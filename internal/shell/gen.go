package shell

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/llm"
)

// Gen runs the "gen" workflow:
//  1) Ask the LLM for CODE ONLY output (no explanations)
//  2) Prompt the user before saving to file
//  3) Store generated code into local RAG (so later you can ask "what did I generate?")
func Gen(cfg *config.Config, st *State, outPath, prompt string) error {
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		return fmt.Errorf("gen: output path is empty")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fmt.Errorf("gen: prompt is empty")
	}

	code, err := genOnce(cfg, st, prompt)
	if err != nil {
		return err
	}

	// Make it visible immediately.
	fmt.Println(code)

	// Confirm save.
	if !confirmSave(outPath) {
		fmt.Println("(cancelled)")
		return nil
	}

	if err := writeFile(outPath, code); err != nil {
		return err
	}
	fmt.Println("saved:", outPath)

	// RAG store: generated artifacts.
	if st != nil && st.RAG != nil {
		_ = st.RAG.AddText("gen:"+outPath, code, cfg.RAGMaxChars)
	}

	return nil
}

func genOnce(cfg *config.Config, st *State, prompt string) (string, error) {
	endpoint := buildEndpoint(cfg)

	// Strong guardrail: code only.
	overrideSystem := strings.TrimSpace(cfg.GenSystemPrompt)
	if overrideSystem == "" {
		overrideSystem = "당신은 코드 생성기입니다. 사용자의 요구에 맞는 코드만 출력하세요. 설명/해설/주석 외 문장 금지. 마크다운 코드블록도 금지. 오직 원문 코드만 출력."
	}

	// If ctx has k8s hints, include them in system prompt.
	sys := systemPromptWithCtx(cfg, st, overrideSystem)

	req := llm.ChatRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temp,
		MaxTokens:   cfg.MaxTokens,
		Stream:      false,
		Messages: []llm.ChatMessage{
			{Role: "system", Content: sys},
			{Role: "user", Content: prompt},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()

	out, err := llm.DoNonStream(ctx, endpoint, cfg.TimeoutSec, req)
	if err != nil {
		// try to update observed ctx size
		if st != nil {
			if obs := parseCtxSizeFromError(err); obs > 0 {
				st.CtxSizeObserved = obs
			}
		}
		return "", err
	}
	code := strings.TrimSpace(out)
	if st != nil && st.NoFence {
		code = StripMarkdownFences(code)
	}
	return code, nil
}

func confirmSave(path string) bool {
	// Non-interactive: refuse to save without explicit confirmation.
	// (prevents accidental overwrites when piped)
	fi, _ := os.Stdin.Stat()
	interactive := (fi.Mode() & os.ModeCharDevice) != 0
	if !interactive {
		fmt.Fprintln(os.Stderr, "gen: non-interactive stdin. not saving without confirmation")
		return false
	}

	r := bufio.NewReader(os.Stdin)
	fmt.Printf("save to %s ? [y/N] ", path)
	ans, _ := r.ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes"
}

func writeFile(path, content string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("empty output path")
	}
	if strings.HasPrefix(path, "~") {
		h, _ := os.UserHomeDir()
		path = filepath.Join(h, strings.TrimPrefix(path, "~"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func RunGen(cfg *config.Config, st *State, outPath, prompt string) error {
	return Gen(cfg, st, outPath, prompt)
}
