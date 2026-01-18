package shell

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"kiki-ai-shell/internal/agent"
	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/history"
	"kiki-ai-shell/internal/llm"
	"kiki-ai-shell/internal/usage"
)

func buildEndpoint(cfg *config.Config) string {
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	}
	return fmt.Sprintf("http://%s:%d/v1/chat/completions", cfg.Host, cfg.Port)
}

func systemPromptWithCtx(cfg *config.Config, st *State, overrideSystem string) string {
	sys := strings.TrimSpace(overrideSystem)
	if sys == "" {
		sys = cfg.SystemPrompt
	}
	if st != nil && st.NoFence {
		sys = strings.TrimSpace(sys) + "\n\n[Output Rules]\n- \uc808\ub300 ``` ... ``` \ud615\ud0dc\uc758 \ub9c8\ud06c\ub2e4\uc6b4 \ucf54\ub4dc\ud39c\uc2a4\ub97c \ucd9c\ub825\ud558\uc9c0 \ub9c8\uc138\uc694 (```yaml \uad6c\ubb38\ub3c4 \uae08\uc9c0).\n- \ud544\uc694\ud558\uba74 \uc21c\uc218 \ud14d\uc2a4\ud2b8\ub85c\ub9cc \ucd9c\ub825\ud558\uc138\uc694.\n"
	}

	if st != nil && len(st.Ctx) > 0 {
		var b strings.Builder
		b.WriteString(sys)
		b.WriteString("\n\n[Context]\n")
		keys := make([]string, 0, len(st.Ctx))
		for k := range st.Ctx {
			keys = append(keys, k)
		}
		// sort keys
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[j] < keys[i] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %s\n", k, st.Ctx[k]))
		}
		return b.String()
	}
	return sys
}

func Ask(cfg *config.Config, st *State, prompt string, overrideSystem string) {
	endpoint := buildEndpoint(cfg)

	userContent, usedFiles, hashes, err := buildUserContent(prompt, st.Files, cfg, st)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attach error:", err)
		return
	}
	sys := systemPromptWithCtx(cfg, st, overrideSystem)

	req := llm.ChatRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temp,
		MaxTokens:   cfg.MaxTokens,
		Stream:      st.Stream,
		Messages: []llm.ChatMessage{
			{Role: "system", Content: sys},
			{Role: "user", Content: userContent},
		},
	}

	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() { <-sigCh; cancel() }()

	now := time.Now().Format(time.RFC3339)
	cwd, _ := os.Getwd()

	// Local agent: if the request is likely bigger than ctx-size, chunk it automatically.
	maxCtx := st.CtxSizeObserved
	if maxCtx <= 0 {
		maxCtx = st.CtxSizeTarget
	}
	if maxCtx > 0 {
		reserve := 768
		// Be a bit conservative for system+overhead.
		if agent.EstimateTokens(userContent) > (maxCtx - reserve) {
			out, err := agent.AskWithAutoChunk(ctx, endpoint, sys, prompt, userContent, agent.AskOpts{
				MaxCtx:    maxCtx,
				Reserve:   reserve,
				Timeout:   timeout,
				Stream:    false, // chunk mode prints as one final answer
				Model:     cfg.Model,
				Temp:      cfg.Temp,
				MaxTokens: cfg.MaxTokens,
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, "LLM error:", err)
				if obs := parseCtxSizeFromError(err); obs > 0 {
					st.CtxSizeObserved = obs
				}
				return
			}
			if st.NoFence {
				out = StripMarkdownFences(out)
			}
			fmt.Println(out)
			st.LastAnswer = out
			if st.Usage != nil {
				cwd, _ := os.Getwd()
				st.Usage.Append(usage.Record{Time: now, User: st.User, Type: "ask", Cwd: cwd, Prompt: prompt, RespPrev: truncateRunes(out, cfg.HistoryPreview)})
				_ = st.RAG.AddText("usage:"+now+":ask", "[ask] "+prompt, 8000)
			}
			if cfg.HistoryEnabled {
				history.Append(cfg.HistoryPath, history.Record{
					Time: now, Endpoint: endpoint, Profile: st.Profile, Model: cfg.Model,
					Temperature: cfg.Temp, MaxTokens: cfg.MaxTokens, Stream: false,
					SystemPrompt: sys, Ctx: st.Ctx, Prompt: prompt, Files: usedFiles,
					FileHashes: hashes, Cwd: cwd, ResponsePrev: truncateRunes(out, cfg.HistoryPreview),
				})
			}
			return
		}
	}

	if st.Stream {
		capLimit := cfg.HistoryPreview
		if cfg.CaptureFull {
			capLimit = cfg.CaptureMax
		}
		captured, err := llm.DoStream(ctx, endpoint, req, capLimit, func(s string) {
			if st.NoFence {
				s = StripFencesFromChunk(s)
			}
			fmt.Print(s)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "LLM stream error:", err)
			if obs := parseCtxSizeFromError(err); obs > 0 {
				st.CtxSizeObserved = obs
			}
			if st.CtxSizeTarget > 0 && st.CtxSizeObserved > 0 && st.CtxSizeTarget > st.CtxSizeObserved {
				fmt.Fprintf(os.Stderr, "hint: server ctx-size=%d. restart llama.cpp server with --ctx-size %d\n", st.CtxSizeObserved, st.CtxSizeTarget)
			}
			return
		}
		if st.NoFence {
			captured = StripMarkdownFences(captured)
		}
		st.LastAnswer = captured
		if st.Usage != nil {
			cwd, _ := os.Getwd()
			st.Usage.Append(usage.Record{Time: now, User: st.User, Type: "ask", Cwd: cwd, Prompt: prompt, RespPrev: truncateRunes(captured, cfg.HistoryPreview)})
			_ = st.RAG.AddText("usage:"+now+":ask", "[ask] "+prompt, 8000)
		}
		if cfg.HistoryEnabled {
			history.Append(cfg.HistoryPath, history.Record{
				Time: now, Endpoint: endpoint, Profile: st.Profile, Model: cfg.Model,
				Temperature: cfg.Temp, MaxTokens: cfg.MaxTokens, Stream: true,
				SystemPrompt: sys, Ctx: st.Ctx, Prompt: prompt, Files: usedFiles,
				FileHashes: hashes, Cwd: cwd, ResponsePrev: truncateRunes(captured, cfg.HistoryPreview),
			})
		}
		return
	}

	out, err := llm.DoNonStream(ctx, endpoint, timeout, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "LLM error:", err)
		if obs := parseCtxSizeFromError(err); obs > 0 {
			st.CtxSizeObserved = obs
		}
		if st.CtxSizeTarget > 0 && st.CtxSizeObserved > 0 && st.CtxSizeTarget > st.CtxSizeObserved {
			fmt.Fprintf(os.Stderr, "hint: server ctx-size=%d. restart llama.cpp server with --ctx-size %d\n", st.CtxSizeObserved, st.CtxSizeTarget)
		}
		return
	}
	if st.NoFence {
		out = StripMarkdownFences(out)
	}
	fmt.Println(out)
	st.LastAnswer = out
	if st.Usage != nil {
		cwd, _ := os.Getwd()
		st.Usage.Append(usage.Record{Time: now, User: st.User, Type: "ask", Cwd: cwd, Prompt: prompt, RespPrev: truncateRunes(out, cfg.HistoryPreview)})
		_ = st.RAG.AddText("usage:"+now+":ask", "[ask] "+prompt, 8000)
	}
	if cfg.HistoryEnabled {
		history.Append(cfg.HistoryPath, history.Record{
			Time: now, Endpoint: endpoint, Profile: st.Profile, Model: cfg.Model,
			Temperature: cfg.Temp, MaxTokens: cfg.MaxTokens, Stream: false,
			SystemPrompt: sys, Ctx: st.Ctx, Prompt: prompt, Files: usedFiles,
			FileHashes: hashes, Cwd: cwd, ResponsePrev: truncateRunes(out, cfg.HistoryPreview),
		})
	}
}
