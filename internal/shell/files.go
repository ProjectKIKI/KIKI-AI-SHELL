package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kiki-ai-shell/internal/config"
)

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}
func fileExists(p string) bool {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !st.IsDir()
}
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readAndFormatFile(path string, maxBytes, maxChars int) (string, string, string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", "", "", errors.New("빈 파일 경로")
	}
	p = normalizePath(p)
	b, err := os.ReadFile(p)
	if err != nil {
		return "", "", "", err
	}
	origHash := hashBytes(b)
	if maxBytes > 0 && len(b) > maxBytes {
		b = b[:maxBytes]
	}
	txt := string(b)
	if maxChars > 0 && len([]rune(txt)) > maxChars {
		r := []rune(txt)
		txt = string(r[:maxChars])
	}
	block := fmt.Sprintf("### FILE: %s (sha256:%s)\n```\n%s\n```\n", p, origHash, txt)
	return p, origHash, block, nil
}

func buildUserContent(prompt string, files []string, cfg *config.Config, st *State) (string, []string, []string, error) {
	var buf strings.Builder
	buf.WriteString(strings.TrimSpace(prompt))

	// RAG snippets first
	if st != nil && st.RAG != nil && st.RAG.Enabled {
		snippets := st.RAG.Search(prompt, cfg.RAGTopK, cfg.RAGMaxChars)
		if len(snippets) > 0 {
			buf.WriteString("\n\n---\n아래는 RAG 검색 결과(참고 발췌)입니다.\n\n")
			for _, s := range snippets {
				buf.WriteString(s)
				buf.WriteString("\n\n")
			}
		}
	}

	used := []string{}
	hashes := []string{}
	if len(files) > 0 {
		buf.WriteString("\n\n---\n아래는 첨부 파일 내용입니다. 파일 내용을 근거로 분석/답변하세요.\n\n")
		for _, f := range files {
			p, h, block, err := readAndFormatFile(f, cfg.FileMaxBytes, cfg.FileMaxChars)
			if err != nil {
				return "", nil, nil, fmt.Errorf("파일 읽기 실패 (%s): %w", f, err)
			}
			used = append(used, p)
			hashes = append(hashes, h)
			buf.WriteString(block)
			buf.WriteString("\n")
		}
	}
	return buf.String(), used, hashes, nil
}
