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

	SystemPrompt    string
	GenSystemPrompt string
	Profile         string
	Stream          bool

	AuthPAM       bool
	UsageEnabled  bool
	UsageBaseDir  string
	UsageLoadDays int
	UsageLoadMax  int

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

	// PCP (Performance Co-Pilot)
	PCPHost string // "local" or remote host (requires pmcd on target)

	// Output formatting
	NoFence bool // strip markdown code fences like ```yaml ... ```
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

func defaultUsageBaseDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kiki-ai-shell", "usage")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func Default() *Config {
	return &Config{
		// NOTE: 주소 설정은 env에서 LLM_BASE_URL만 사용합니다.
		// LLM_HOST/LLM_PORT는 더 이상 env로 읽지 않습니다. (쉘 내 :llm 로 변경)
		Host:       "10.0.2.253",
		Port:       8080,
		BaseURL:    envString("LLM_BASE_URL", ""),
		Model:      envString("LLM_MODEL", "llama"),
		Temp:       envFloat("LLM_TEMP", 0.2),
		MaxTokens:  envInt("LLM_MAX_TOKENS", 512),
		TimeoutSec: envInt("LLM_TIMEOUT", 60),

		SystemPrompt:    envString("LLM_SYSTEM_PROMPT", "당신은 간결하고 정확하게 답변하는 도우미입니다."),
		GenSystemPrompt: envString("LLM_GEN_SYSTEM_PROMPT", "\ub2f9\uc2e0\uc740 \ucf54\ub4dc \uc0dd\uc131\uae30\uc785\ub2c8\ub2e4. \uc124\uba85\uc740 \uc4f0\uc9c0 \ub9d0\uace0, \uc694\uccad\ud55c \uacb0\uacfc\ub97c \uadf8\ub300\ub85c \uc6d0\ubcf8 \ucf54\ub4dc\ub9cc \ucd9c\ub825\ud558\uc138\uc694. \ub9c8\ud06c\ub2e4\uc6b4 \ucf54\ub4dc\ud39c\uc2a4(``` ... ```)\ub098 \ubc31\ud2f1(`)\uc744 \uc808\ub300 \ud3ec\ud568\ud558\uc9c0 \ub9c8\uc138\uc694. \ud14d\uc2a4\ud2b8 \uc124\uba85, \uc8fc\uc11d, \ucd94\uac00 \ubb38\uc7a5\ub3c4 \uc808\ub300 \ud3ec\ud568\ud558\uc9c0 \ub9c8\uc138\uc694."),
		Profile:         envString("LLM_PROFILE", "fast"),
		Stream:          envBool("LLM_STREAM", false),

		AuthPAM:       envBool("KIKI_AUTH_PAM", true),
		UsageEnabled:  envBool("KIKI_USAGE", true),
		UsageBaseDir:  envString("KIKI_USAGE_BASE", defaultUsageBaseDir()),
		UsageLoadDays: envInt("KIKI_USAGE_LOAD_DAYS", 30),
		UsageLoadMax:  envInt("KIKI_USAGE_LOAD_MAX", 5000),

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

		PCPHost: envString("KIKI_PCP_HOST", "local"),

		PCPHost: envString("KIKI_PCP_HOST", "local"),

		// If true, the shell will remove markdown fences like ```yaml / ``` from model outputs.
		NoFence: envBool("KIKI_NOFENCE", true),
	}
}
