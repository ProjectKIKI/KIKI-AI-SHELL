package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Record struct {
	Time     string `json:"time"`
	User     string `json:"user"`
	Type     string `json:"type"` // "cmd" | "ask" | "llm"
	Cwd      string `json:"cwd,omitempty"`
	Command  string `json:"command,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	RespPrev string `json:"resp_prev,omitempty"`
}

type Logger struct {
	User string
	Path string
}

func New(baseDir, user string) *Logger {
	if strings.TrimSpace(user) == "" {
		user = "unknown"
	}
	dir := filepath.Join(baseDir, user)
	_ = os.MkdirAll(dir, 0o700)
	return &Logger{
		User: user,
		Path: filepath.Join(dir, "usage.jsonl"),
	}
}

// NewLogger is kept for backward compatibility with earlier refactors.
func NewLogger(baseDir, user string) *Logger { return New(baseDir, user) }

func (l *Logger) Append(rec Record) {
	if l == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.Path), 0o700)
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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

func LoadAll(path string) ([]Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	out := make([]Record, 0, 1024)
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
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

func LoadRecent(path string, days int, max int) ([]Record, error) {
	all, err := LoadAll(path)
	if err != nil {
		return nil, err
	}
	if days <= 0 {
		days = 30
	}
	cut := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	out := make([]Record, 0, len(all))
	for _, r := range all {
		t, err := time.Parse(time.RFC3339, r.Time)
		if err != nil {
			continue
		}
		if t.After(cut) {
			out = append(out, r)
		}
	}
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out, nil
}

func Summary(records []Record) string {
	if len(records) == 0 {
		return "(no records)"
	}
	cmdCount := 0
	askCount := 0
	top := map[string]int{}
	for _, r := range records {
		switch r.Type {
		case "cmd":
			cmdCount++
			c := strings.TrimSpace(r.Command)
			if c == "" {
				continue
			}
			// aggregate by first token (command name)
			fields := strings.Fields(c)
			if len(fields) > 0 {
				top[fields[0]]++
			}
		case "ask":
			askCount++
		}
	}
	// crude top-5
	type kv struct {
		K string
		V int
	}
	var pairs []kv
	for k, v := range top {
		pairs = append(pairs, kv{k, v})
	}
	// sort desc
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].V > pairs[i].V {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	if len(pairs) > 5 {
		pairs = pairs[:5]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "records=%d (cmd=%d, ask=%d)\n", len(records), cmdCount, askCount)
	if len(pairs) > 0 {
		b.WriteString("top cmds: ")
		for i, p := range pairs {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s(%d)", p.K, p.V)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
