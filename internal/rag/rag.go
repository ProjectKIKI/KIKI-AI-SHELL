package rag

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Doc struct {
	ID   string
	Path string
	Text string
}

type Store struct {
	Enabled bool
	Docs    []Doc
}

func New(enabled bool) *Store   { return &Store{Enabled: enabled, Docs: []Doc{}} }
func (s *Store) Toggle(on bool) { s.Enabled = on }
func (s *Store) Clear()         { s.Docs = []Doc{} }

func trimRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func (s *Store) AddFile(path string, maxBytes, maxChars int) (Doc, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return Doc{}, errors.New("empty path")
	}
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Doc{}, err
	}
	if maxBytes > 0 && len(b) > maxBytes {
		b = b[:maxBytes]
	}
	txt := string(b)
	if maxChars > 0 {
		txt = trimRunes(txt, maxChars)
	}
	sum := sha256.Sum256([]byte(p + ":" + txt))
	d := Doc{ID: hex.EncodeToString(sum[:]), Path: p, Text: txt}
	for i := range s.Docs {
		if s.Docs[i].Path == p {
			s.Docs[i] = d
			return d, nil
		}
	}
	s.Docs = append(s.Docs, d)
	return d, nil
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_-]+`)

type hit struct {
	doc     Doc
	score   int
	excerpt string
}

func buildExcerpt(text string, qWords []string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 800
	}
	lower := strings.ToLower(text)
	best := -1
	for _, w := range qWords {
		if w == "" {
			continue
		}
		idx := strings.Index(lower, w)
		if idx >= 0 && (best == -1 || idx < best) {
			best = idx
		}
	}
	if best == -1 {
		return trimRunes(text, maxChars)
	}
	start := best - maxChars/3
	if start < 0 {
		start = 0
	}
	end := start + maxChars
	if end > len(text) {
		end = len(text)
	}
	sn := text[start:end]
	if start > 0 {
		sn = "…" + sn
	}
	if end < len(text) {
		sn = sn + "…"
	}
	return sn
}

func (s *Store) Search(query string, topK int, excerptChars int) []string {
	if !s.Enabled || len(s.Docs) == 0 {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	qWords := wordRe.FindAllString(q, -1)
	if len(qWords) == 0 {
		return nil
	}
	hits := make([]hit, 0, len(s.Docs))
	for _, d := range s.Docs {
		tl := strings.ToLower(d.Text)
		score := 0
		for _, w := range qWords {
			score += strings.Count(tl, w)
		}
		if score <= 0 {
			continue
		}
		hits = append(hits, hit{doc: d, score: score, excerpt: buildExcerpt(d.Text, qWords, excerptChars)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score == hits[j].score {
			return hits[i].doc.Path < hits[j].doc.Path
		}
		return hits[i].score > hits[j].score
	})
	if topK <= 0 {
		topK = 3
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, "### RAG: "+h.doc.Path+"\n```\n"+h.excerpt+"\n```")
	}
	return out
}

func (s *Store) Stats() (bool, int) { return s.Enabled, len(s.Docs) }
