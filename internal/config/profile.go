package config

import "strings"

func ApplyProfile(cfg *Config) {
	switch strings.ToLower(strings.TrimSpace(cfg.Profile)) {
	case "", "none":
		return
	case "fast":
		if cfg.Temp > 0.2 {
			cfg.Temp = 0.2
		}
		if cfg.MaxTokens <= 0 || cfg.MaxTokens > 384 {
			cfg.MaxTokens = 256
		}
		if strings.TrimSpace(cfg.SystemPrompt) == "" {
			cfg.SystemPrompt = "당신은 간결하고 정확하게 답변하는 도우미입니다. 결론을 먼저 말하고, 필요하면 3개 이하 bullet로만 보충하세요."
		}
	case "deep":
		if cfg.Temp < 0.3 {
			cfg.Temp = 0.3
		}
		if cfg.MaxTokens < 768 {
			cfg.MaxTokens = 1024
		}
		if strings.TrimSpace(cfg.SystemPrompt) == "" {
			cfg.SystemPrompt = "당신은 시니어 SRE/플랫폼 엔지니어입니다. 가정/원인/검증/조치 순으로 구조화해 답변하세요. 불확실한 부분은 불확실하다고 표시하세요."
		}
	}
}
