package agent

import (
	"context"
	"fmt"
	"strings"

	"kiki-ai-shell/internal/llm"
)

type AskOpts struct {
	MaxCtx   int
	Reserve  int
	Timeout  int
	Stream   bool
	Model    string
	Temp     float64
	MaxTokens int
}

// AskWithAutoChunk splits oversized user content and performs a "running summary" pass,
// then asks the final question using the condensed summary.
//
// This is a local agent (no extra server). It reduces ctx-size explosions for CPU-only llama.cpp.
func AskWithAutoChunk(ctx context.Context, endpoint string, systemPrompt string, userQuestion string, userContent string, opts AskOpts) (string, error) {
	maxCtx := opts.MaxCtx
	if maxCtx <= 0 {
		// No ctx info: just do normal request.
		return single(ctx, endpoint, systemPrompt, userContent, opts)
	}
	reserve := opts.Reserve
	if reserve <= 0 {
		reserve = 512
	}
	// Determine chunk size.
	chunkMax := maxCtx - reserve
	if chunkMax < 512 {
		chunkMax = 512
	}

	// If it's within budget, do normal request.
	if EstimateTokens(userContent) <= chunkMax {
		return single(ctx, endpoint, systemPrompt, userContent, opts)
	}

	chunks := SplitByApproxTokens(userContent, chunkMax)
	if len(chunks) <= 1 {
		return single(ctx, endpoint, systemPrompt, userContent, opts)
	}

	// Phase 1: iterative summarization
	running := ""
	for i, c := range chunks {
		msg := buildChunkMessage(i+1, len(chunks), c, running)
		out, err := single(ctx, endpoint, systemPrompt, msg, AskOpts{MaxCtx: opts.MaxCtx, Reserve: opts.Reserve, Timeout: opts.Timeout, Stream: false, Model: opts.Model, Temp: 0.2, MaxTokens: 512})
		if err != nil {
			return "", err
		}
		running = strings.TrimSpace(out)
		if running == "" {
			running = "(empty summary)"
		}
	}

	// Phase 2: final answer from summary
	finalUser := fmt.Sprintf("아래는 긴 입력을 여러 조각으로 요약한 결과입니다.\n\n[SUMMARY]\n%s\n\n이 요약을 바탕으로 사용자의 원 질문에 답하세요:\n%s\n", running, strings.TrimSpace(userQuestion))
	return single(ctx, endpoint, systemPrompt, finalUser, opts)
}

func buildChunkMessage(idx, total int, chunk, running string) string {
	chunk = strings.TrimSpace(chunk)
	running = strings.TrimSpace(running)
	if running == "" {
		return fmt.Sprintf("[PART %d/%d]\n%s\n\n당신의 임무: 위 내용을 읽고 핵심 사실/지표/오류/원인 후보/조치 후보를 10줄 이내로 요약하세요. (설명 금지, 요약만)\n", idx, total, chunk)
	}
	return fmt.Sprintf("[PART %d/%d]\n%s\n\n[CURRENT SUMMARY]\n%s\n\n당신의 임무: 기존 요약을 유지하되, 새 내용이 추가되면 덧붙이고, 중복은 제거해서 12줄 이내로 업데이트하세요. (설명 금지, 요약만)\n", idx, total, chunk, running)
}

func single(ctx context.Context, endpoint, systemPrompt, userContent string, opts AskOpts) (string, error) {
	req := llm.ChatRequest{
		Model:       opts.Model,
		Temperature: opts.Temp,
		MaxTokens:   opts.MaxTokens,
		Stream:      opts.Stream,
		Messages: []llm.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
	}
	if opts.Stream {
		// Stream tokens to stdout, and also capture enough to keep the last answer/history.
		return llm.DoStream(ctx, endpoint, req, 2_000_000, func(s string) { fmt.Print(s) })
	}
	return llm.DoNonStream(ctx, endpoint, opts.Timeout, req)
}
