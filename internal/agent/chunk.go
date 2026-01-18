package agent

import "strings"

// EstimateTokens is a cheap heuristic (very approximate!).
// For English text it's ~4 chars/token; for Korean it varies, but this works well enough
// to decide whether to chunk before hitting llama.cpp ctx-size errors.
func EstimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// rune-based length is more stable for mixed-language.
	r := []rune(s)
	// 1 token ~= 3.5 runes (roughly). Keep it conservative.
	return (len(r) + 3) / 4
}

func SplitByApproxTokens(s string, maxTokens int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if maxTokens <= 0 {
		return []string{s}
	}
	// If already small, return.
	if EstimateTokens(s) <= maxTokens {
		return []string{s}
	}

	// Split by paragraphs first, then fall back to line-based.
	parts := strings.Split(s, "\n\n")
	chunks := make([]string, 0, 8)
	var cur strings.Builder
	curTok := 0

	flush := func() {
		c := strings.TrimSpace(cur.String())
		if c != "" {
			chunks = append(chunks, c)
		}
		cur.Reset()
		curTok = 0
	}

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pt := EstimateTokens(p)
		if pt > maxTokens {
			// Too big paragraph: split by lines.
			lines := strings.Split(p, "\n")
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if ln == "" {
					continue
				}
				lt := EstimateTokens(ln)
				if curTok+lt+1 > maxTokens && curTok > 0 {
					flush()
				}
				if cur.Len() > 0 {
					cur.WriteString("\n")
					curTok += 1
				}
				cur.WriteString(ln)
				curTok += lt
			}
			cur.WriteString("\n")
			curTok += 1
			continue
		}

		if curTok+pt+2 > maxTokens && curTok > 0 {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
			curTok += 2
		}
		cur.WriteString(p)
		curTok += pt
	}
	flush()
	if len(chunks) == 0 {
		return []string{s}
	}
	return chunks
}
