package shell

import (
	"regexp"
	"strings"
)

// StripMarkdownFences removes common markdown code fences from a full text output.
// It targets patterns like:
//   ```yaml
//   ...
//   ```
// and returns the inner content.
func StripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "```") {
			// drop opening/closing fence lines entirely
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

var reFenceInline = regexp.MustCompile("```[a-zA-Z0-9_-]*")

// StripFencesFromChunk removes fence tokens that may appear during streaming.
// This is best-effort; it avoids printing ``` / ```yaml etc.
func StripFencesFromChunk(chunk string) string {
	if chunk == "" {
		return chunk
	}
	chunk = reFenceInline.ReplaceAllString(chunk, "")
	return chunk
}
