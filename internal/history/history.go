package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Record struct {
	Time         string            `json:"time"`
	Endpoint     string            `json:"endpoint"`
	Profile      string            `json:"profile"`
	Model        string            `json:"model"`
	Temperature  float64           `json:"temperature"`
	MaxTokens    int               `json:"max_tokens"`
	Stream       bool              `json:"stream"`
	SystemPrompt string            `json:"system_prompt"`
	Ctx          map[string]string `json:"ctx,omitempty"`
	Prompt       string            `json:"prompt"`
	Files        []string          `json:"files"`
	FileHashes   []string          `json:"file_hashes"`
	Cwd          string            `json:"cwd"`
	ResponsePrev string            `json:"response_preview,omitempty"`
}

func Append(path string, rec Record) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}

func ReadAll(path string) ([]Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	out := make([]Record, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(ln), &r) == nil {
			out = append(out, r)
		}
	}
	return out, nil
}
