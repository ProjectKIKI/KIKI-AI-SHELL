package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Host       string
	Port       int
	BaseURL    string
	Model      string
	Temp       float64
	MaxTokens  int
	TimeoutSec int

	SystemPrompt string
	Profile      string
	Stream       bool

	CtxSizeTarget   int
	CtxSizeObserved int

	HistoryEnabled bool
	HistoryPath    string
	HistoryPreview int

	FileMaxBytes int
	FileMaxChars int

	CaptureFull bool
	CaptureMax  int

	RAGEnabled  bool
	RAGTopK     int
	RAGMaxChars int
}

func envString(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
func envFloat(k string, d float64) float64 {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return d
}
func envBool(k string, d bool) bool {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		v = strings.ToLower(v)
		if v == "1" || v == "true" || v == "yes" || v == "on" {
			return true
		}
		if v == "0" || v == "false" || v == "no" || v == "off" {
			return false
		}
	}
	return d
}

func defaultHistoryPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kiki")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "history.jsonl")
}

func Default() *Config {
	return &Config{
		Host:       envString("LLM_HOST", "10.0.2.253"),
		Port:       envInt("LLM_PORT", 8080),
		BaseURL:    envString("LLM_BASE_URL", ""),
		Model:      envString("LLM_MODEL", "llama"),
		Temp:       envFloat("LLM_TEMP", 0.2),
		MaxTokens:  envInt("LLM_MAX_TOKENS", 512),
		TimeoutSec: envInt("LLM_TIMEOUT", 60),

		SystemPrompt: envString("LLM_SYSTEM_PROMPT", "당신은 간결하고 정확하게 답변하는 도우미입니다."),
		Profile:      envString("LLM_PROFILE", "fast"),
		Stream:       envBool("LLM_STREAM", false),

		CtxSizeTarget:   envInt("LLM_CTX_TARGET", 0),
		CtxSizeObserved: envInt("LLM_CTX_OBSERVED", 0),

		HistoryEnabled: envBool("LLM_HISTORY", true),
		HistoryPath:    envString("LLM_HISTORY_PATH", defaultHistoryPath()),
		HistoryPreview: envInt("LLM_HISTORY_PREVIEW", 800),

		FileMaxBytes: envInt("LLM_FILE_MAX_BYTES", 256*1024),
		FileMaxChars: envInt("LLM_FILE_MAX_CHARS", 20000),

		CaptureFull: envBool("LLM_CAPTURE_FULL", false),
		CaptureMax:  envInt("LLM_CAPTURE_MAX", 2_000_000),

		RAGEnabled:  envBool("LLM_RAG", false),
		RAGTopK:     envInt("LLM_RAG_TOPK", 3),
		RAGMaxChars: envInt("LLM_RAG_MAX_CHARS", 2500),
	}
}
