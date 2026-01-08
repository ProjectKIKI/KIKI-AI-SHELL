package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Messages    []ChatMessage `json:"messages"`
}
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
}
type StreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
type APIErrorWrapper struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type HistoryRecord struct {
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

// ---------------- Config ----------------

type Config struct {
	Host         string
	Port         int
	BaseURL      string
	Model        string
	Temp         float64
	MaxTokens    int
	TimeoutSec   int
	SystemPrompt string
	Profile      string
	Stream       bool

	// ctx-size (token context window). NOTE: actual limit is decided by llama.cpp server startup.
	CtxSizeTarget   int // desired/target ctx-size (for header + guidance)
	CtxSizeObserved int // last observed server ctx-size (parsed from errors or env)

	// RAG
	RagEnabled    bool
	RagPath       string
	RagTopK       int
	RagChunkChars int
	RagOverlap    int
	RagMaxChars   int // maximum chars of retrieved context appended to prompt

	HistoryEnabled bool
	HistoryPath    string
	HistoryPreview int

	FileMaxBytes int
	FileMaxChars int

	CaptureFull bool
	CaptureMax  int
}

type UIConfig struct {
	ShowHeader   bool
	ClearOnDraw  bool // legacy full clear
	FixedHeader  bool // pin header via scroll region
	HeaderLines  int  // header height (default 5)
	UseUnicode   bool
	MaxFilesLine int
}

type ShellState struct {
	Files           []string
	Profile         string
	Stream          bool
	LastAnswer      string
	Ctx             map[string]string
	CtxSizeTarget   int
	CtxSizeObserved int
	RagEnabled      bool
	Rag             *RagIndex
	UI              UIConfig
	ApprovalMode    bool
	Pending         []PendingCmd
	NextPendingID   int
}

type PendingCmd struct {
	ID   int    `json:"id"`
	Time string `json:"time"`
	Cmd  string `json:"cmd"`
}

// ---------------- Env helpers ----------------

func envString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func envFloat(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func envBool(key string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		v = strings.ToLower(v)
		if v == "1" || v == "true" || v == "yes" || v == "on" {
			return true
		}
		if v == "0" || v == "false" || v == "no" || v == "off" {
			return false
		}
	}
	return def
}

func defaultHistoryPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kiki")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "history.jsonl")
}

func defaultRagPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kiki")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "rag.json")
}

func buildEndpoint(cfg *Config) string {
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	}
	return fmt.Sprintf("http://%s:%d/v1/chat/completions", cfg.Host, cfg.Port)
}

func applyProfile(cfg *Config) {
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

// ---------------- HTTP ----------------

func makeHTTPClient(timeoutSec int) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if timeoutSec <= 0 {
		return &http.Client{Transport: transport}
	}
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second, Transport: transport}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ---------------- Files ----------------

func readAndFormatFile(path string, maxBytes, maxChars int) (string, string, string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", "", "", errors.New("빈 파일 경로")
	}
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
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

func printFiles(st *ShellState) {
	if st == nil || len(st.Files) == 0 {
		fmt.Println("(no attached files)")
		return
	}
	for i, f := range st.Files {
		fmt.Printf("%d) %s\n", i+1, f)
	}
}

func removeFileAt(st *ShellState, idx int) bool {
	if st == nil || idx < 0 || idx >= len(st.Files) {
		return false
	}
	st.Files = append(st.Files[:idx], st.Files[idx+1:]...)
	return true
}

// ---------------- History ----------------

func appendHistory(path string, rec HistoryRecord) {
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

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------------- ctx helpers ----------------

var reCtxErr = regexp.MustCompile(`available context size \((\d+) tokens\)`)

func parseCtxSizeFromError(err error) int {
	if err == nil {
		return 0
	}
	m := reCtxErr.FindStringSubmatch(err.Error())
	if len(m) == 2 {
		if n, e := strconv.Atoi(m[1]); e == nil {
			return n
		}
	}
	return 0
}

func ctxGet(st *ShellState, key string) string {
	if st == nil || st.Ctx == nil {
		return ""
	}
	return strings.TrimSpace(st.Ctx[key])
}

func ctxSet(st *ShellState, key, val string) {
	if st == nil {
		return
	}
	if st.Ctx == nil {
		st.Ctx = map[string]string{}
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if key == "" {
		return
	}
	st.Ctx[key] = val
}

func ctxClear(st *ShellState) {
	if st == nil {
		return
	}
	st.Ctx = map[string]string{}
}

// ---------------- RAG (local, no external deps) ----------------

// Simple hashed BoW vectors + cosine similarity.
// Stored on disk as JSON for portability.

type RagChunk struct {
	ID     string    `json:"id"`
	File   string    `json:"file"`
	SHA256 string    `json:"sha256"`
	Offset int       `json:"offset"` // rune offset
	Text   string    `json:"text"`
	Vec    []float32 `json:"vec"`
}

type RagIndex struct {
	Dim    int        `json:"dim"`
	Chunks []RagChunk `json:"chunks"`
	Path   string     `json:"-"`
}

type RagHit struct {
	Score float64
	Chunk RagChunk
}

var reTok = regexp.MustCompile(`[\p{L}\p{N}]+`)

func fnv32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func textToVec(dim int, text string) []float32 {
	if dim <= 0 {
		dim = 256
	}
	vec := make([]float32, dim)
	toks := reTok.FindAllString(strings.ToLower(text), -1)
	for _, t := range toks {
		idx := int(fnv32(t) % uint32(dim))
		vec[idx] += 1
	}
	// L2 normalize
	var ss float64
	for _, v := range vec {
		ss += float64(v) * float64(v)
	}
	if ss <= 0 {
		return vec
	}
	inv := float32(1.0 / math.Sqrt(ss))
	for i := range vec {
		vec[i] *= inv
	}
	return vec
}

func cosine(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var s float64
	for i := 0; i < n; i++ {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func loadRagIndex(path string, dim int) *RagIndex {
	ri := &RagIndex{Dim: dim, Chunks: []RagChunk{}, Path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		return ri
	}
	var tmp RagIndex
	if json.Unmarshal(b, &tmp) == nil && tmp.Dim > 0 {
		tmp.Path = path
		if dim > 0 {
			tmp.Dim = dim
		}
		// If dim changed, re-vectorize (best effort).
		if dim > 0 && dim != tmp.Dim {
			for i := range tmp.Chunks {
				tmp.Chunks[i].Vec = textToVec(dim, tmp.Chunks[i].Text)
			}
			tmp.Dim = dim
		}
		return &tmp
	}
	return ri
}

func (ri *RagIndex) save() {
	if ri == nil || strings.TrimSpace(ri.Path) == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(ri.Path), 0o755)
	b, err := json.MarshalIndent(ri, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(ri.Path, b, 0o600)
}

func chunkTextRunes(s string, chunkChars, overlap int) []struct {
	off  int
	text string
} {
	if chunkChars <= 0 {
		chunkChars = 1200
	}
	if overlap < 0 {
		overlap = 0
	}
	r := []rune(s)
	var out []struct {
		off  int
		text string
	}
	for start := 0; start < len(r); {
		end := start + chunkChars
		if end > len(r) {
			end = len(r)
		}
		txt := strings.TrimSpace(string(r[start:end]))
		if txt != "" {
			out = append(out, struct {
				off  int
				text string
			}{off: start, text: txt})
		}
		if end == len(r) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = 0
		}
	}
	return out
}

func (ri *RagIndex) removeFile(file string) {
	if ri == nil {
		return
	}
	file = normalizePath(file)
	n := 0
	for _, c := range ri.Chunks {
		if c.File != file {
			ri.Chunks[n] = c
			n++
		}
	}
	ri.Chunks = ri.Chunks[:n]
}

func (ri *RagIndex) addFile(file string, maxBytes int, chunkChars, overlap int) error {
	if ri == nil {
		return errors.New("rag index nil")
	}
	file = normalizePath(file)
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if maxBytes > 0 && len(b) > maxBytes {
		b = b[:maxBytes]
	}
	sha := hashBytes(b)
	ri.removeFile(file)

	chunks := chunkTextRunes(string(b), chunkChars, overlap)
	for _, ch := range chunks {
		id := fmt.Sprintf("%s:%d:%s", file, ch.off, sha[:12])
		rc := RagChunk{
			ID:     id,
			File:   file,
			SHA256: sha,
			Offset: ch.off,
			Text:   ch.text,
			Vec:    textToVec(ri.Dim, ch.text),
		}
		ri.Chunks = append(ri.Chunks, rc)
	}
	ri.save()
	return nil
}

func (ri *RagIndex) clear() {
	if ri == nil {
		return
	}
	ri.Chunks = []RagChunk{}
	ri.save()
}

func (ri *RagIndex) search(query string, topk int, filterFiles []string) []RagHit {
	if ri == nil || len(ri.Chunks) == 0 {
		return nil
	}
	if topk <= 0 {
		topk = 3
	}
	fset := map[string]bool{}
	for _, f := range filterFiles {
		fset[normalizePath(f)] = true
	}
	qv := textToVec(ri.Dim, query)

	hits := make([]RagHit, 0, topk*2)
	for _, c := range ri.Chunks {
		if len(fset) > 0 && !fset[c.File] {
			continue
		}
		s := cosine(qv, c.Vec)
		if s <= 0 {
			continue
		}
		hits = append(hits, RagHit{Score: s, Chunk: c})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topk {
		hits = hits[:topk]
	}
	return hits
}

func formatRagContext(hits []RagHit, maxChars int) string {
	if len(hits) == 0 {
		return ""
	}
	if maxChars <= 0 {
		maxChars = 4000
	}
	var b strings.Builder
	b.WriteString("\n\n---\n[Retrieved Context - RAG]\n")
	used := 0
	for _, h := range hits {
		head := fmt.Sprintf("### RAG: %s @%d (score=%.3f)\n```\n", h.Chunk.File, h.Chunk.Offset, h.Score)
		tail := "\n```\n"
		txt := h.Chunk.Text
		remain := maxChars - used - len([]rune(head)) - len([]rune(tail))
		if remain <= 0 {
			break
		}
		r := []rune(txt)
		if len(r) > remain {
			txt = string(r[:remain]) + "…"
		}
		seg := head + txt + tail
		b.WriteString(seg)
		used += len([]rune(seg))
		if used >= maxChars {
			break
		}
	}
	return b.String()
}

// ---------------- Prompt assembly ----------------

func buildUserContent(prompt string, files []string, cfg *Config, st *ShellState) (string, []string, []string, error) {
	var buf strings.Builder
	buf.WriteString(strings.TrimSpace(prompt))

	// RAG retrieve
	if st != nil && st.RagEnabled && st.Rag != nil {
		filter := []string{}
		// If files attached, prefer those files only (reduces irrelevant hits).
		if len(files) > 0 {
			filter = append(filter, files...)
		}
		hits := st.Rag.search(prompt, cfg.RagTopK, filter)
		rc := formatRagContext(hits, cfg.RagMaxChars)
		if rc != "" {
			buf.WriteString(rc)
		}
	}

	var used []string
	var hashes []string

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

// ---------------- LLM calls ----------------

func doNonStream(ctx context.Context, cfg *Config, endpoint string, reqPayload ChatRequest) (string, error) {
	client := makeHTTPClient(cfg.TimeoutSec)
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		var ew APIErrorWrapper
		if json.Unmarshal(raw, &ew) == nil && ew.Error.Message != "" {
			return "", fmt.Errorf("API Error: %s", ew.Error.Message)
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var cr ChatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("응답 파싱 실패: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("choices가 비어있음")
	}
	content := cr.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		content = cr.Choices[0].Text
	}
	return strings.TrimSpace(content), nil
}

func doStream(ctx context.Context, cfg *Config, endpoint string, reqPayload ChatRequest) (string, error) {
	client := makeHTTPClient(0)

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var ew APIErrorWrapper
		if json.Unmarshal(raw, &ew) == nil && ew.Error.Message != "" {
			return "", fmt.Errorf("API Error: %s", ew.Error.Message)
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	reader := bufio.NewReader(resp.Body)
	var captured strings.Builder

	capLimit := cfg.HistoryPreview
	if cfg.CaptureFull {
		capLimit = cfg.CaptureMax
		if capLimit <= 0 {
			capLimit = 2_000_000
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return captured.String(), nil
			}
			return captured.String(), err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			fmt.Print("\n")
			return captured.String(), nil
		}

		var chunk StreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		text := chunk.Choices[0].Delta.Content
		if text == "" {
			text = chunk.Choices[0].Message.Content
		}
		if text == "" {
			continue
		}

		fmt.Print(text)

		if capLimit > 0 && captured.Len() < capLimit {
			remain := capLimit - captured.Len()
			if len(text) > remain {
				captured.WriteString(text[:remain])
			} else {
				captured.WriteString(text)
			}
		}
	}
}

func systemPromptWithCtx(cfg *Config, st *ShellState, overrideSystem string) string {
	sys := overrideSystem
	if strings.TrimSpace(sys) == "" {
		sys = cfg.SystemPrompt
	}
	if st != nil && len(st.Ctx) > 0 {
		var b strings.Builder
		b.WriteString(sys)
		b.WriteString("\n\n[Context]\n")
		keys := make([]string, 0, len(st.Ctx))
		for k := range st.Ctx {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %s\n", k, st.Ctx[k]))
		}
		return b.String()
	}
	return sys
}

// ---------------- UI ----------------

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

func termCols() int {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 || ws.Col == 0 {
		if v := strings.TrimSpace(os.Getenv("COLUMNS")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return 80
	}
	return int(ws.Col)
}

func termRows() int {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 || ws.Row == 0 {
		if v := strings.TrimSpace(os.Getenv("LINES")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return 24
	}
	return int(ws.Row)
}

func clearScreen() { fmt.Print("\033[H\033[2J") }

func setScrollRegion(top, bottom int) {
	if top < 1 {
		top = 1
	}
	if bottom < top {
		bottom = top
	}
	fmt.Printf("\033[%d;%dr", top, bottom)
}

func saveCursor()    { fmt.Print("\033[s") }
func restoreCursor() { fmt.Print("\033[u") }
func cursorHome()    { fmt.Print("\033[H") }

func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

func elideMiddle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	head := (max - 1) / 2
	tail := max - head - 1
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

func renderHeader(cfg *Config, st *ShellState) {
	if !st.UI.ShowHeader {
		return
	}
	if st.UI.FixedHeader {
		saveCursor()
		cursorHome()
		lines := st.UI.HeaderLines
		if lines <= 0 {
			lines = 5
		}
		for i := 0; i < lines; i++ {
			fmt.Print("\033[2K")
			if i < lines-1 {
				fmt.Print("\n")
			}
		}
		cursorHome()
	} else if st.UI.ClearOnDraw {
		clearScreen()
	}

	cols := termCols()
	if cols < 50 {
		cols = 50
	}
	inner := cols - 2
	if inner < 10 {
		inner = 10
	}

	title := " KIKI AI SHELL "
	h := "─"
	top := "┌" + h + title
	remain := inner - (1 + len([]rune(title)))
	if remain < 0 {
		remain = 0
	}
	top += strings.Repeat(h, remain) + "┐"

	llm := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if strings.TrimSpace(cfg.BaseURL) != "" {
		llm = cfg.BaseURL
	}

	stream := "off"
	if st.Stream {
		stream = "on"
	}

	rag := "off"
	if st.RagEnabled {
		rag = "on"
	}

	line1 := fmt.Sprintf(" LLM: %s | profile: %s | stream: %s | rag: %s ", llm, st.Profile, stream, rag)

	ctxObs := st.CtxSizeObserved
	ctxTgt := st.CtxSizeTarget
	if ctxObs > 0 || ctxTgt > 0 {
		if ctxObs > 0 && ctxTgt > 0 {
			line1 = strings.TrimRight(line1, " ") + fmt.Sprintf("| ctx: %d/%d ", ctxObs, ctxTgt)
		} else if ctxObs > 0 {
			line1 = strings.TrimRight(line1, " ") + fmt.Sprintf("| ctx: %d ", ctxObs)
		} else {
			line1 = strings.TrimRight(line1, " ") + fmt.Sprintf("| ctx: %d ", ctxTgt)
		}
	}

	cluster := ctxGet(st, "cluster")
	ns := ctxGet(st, "ns")
	k8s := " K8S: (none) "
	if cluster != "" || ns != "" {
		if cluster == "" {
			cluster = "-"
		}
		if ns == "" {
			ns = "-"
		}
		k8s = fmt.Sprintf(" K8S: cluster=%s | ns=%s ", cluster, ns)
	}

	files := "(none)"
	if len(st.Files) > 0 {
		files = strings.Join(st.Files, ", ")
	}
	maxFiles := st.UI.MaxFilesLine
	if maxFiles <= 0 {
		maxFiles = inner - len([]rune(" Files: ")) - 2
		if maxFiles < 10 {
			maxFiles = 10
		}
	}
	files = elideMiddle(files, maxFiles)

	line2 := k8s
	line3 := fmt.Sprintf(" Files: %s ", files)

	line1 = padRight(elideMiddle(line1, inner), inner)
	line2 = padRight(elideMiddle(line2, inner), inner)
	line3 = padRight(elideMiddle(line3, inner), inner)

	fmt.Println(top)
	fmt.Println("│" + line1 + "│")
	fmt.Println("│" + line2 + "│")
	fmt.Println("│" + line3 + "│")
	fmt.Println("└" + strings.Repeat(h, inner) + "┘")

	if st.UI.FixedHeader {
		lines := st.UI.HeaderLines
		if lines <= 0 {
			lines = 5
		}
		rows := termRows()
		setScrollRegion(lines+1, rows)
		restoreCursor()
	}
}

func promptLine(st *ShellState) string {
	cwd, _ := os.Getwd()
	fileCount := len(st.Files)
	stream := "off"
	if st.Stream {
		stream = "on"
	}
	ap := "off"
	if st.ApprovalMode {
		ap = "on"
	}
	return fmt.Sprintf("kiki[%s|stream:%s|files:%d|appr:%s] %s> ", st.Profile, stream, fileCount, ap, cwd)
}

// ---------------- PTY bash ----------------

func runInteractiveBash() error {
	cmd := exec.Command("/bin/bash")
	cmd.Env = append(os.Environ(), "KIKI_INNER_BASH=1")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH

	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, ptmx)
	_ = cmd.Wait()
	return nil
}

// ---------------- Help ----------------

func printHelpAll() {
	fmt.Println(`
KIKI AI SHELL

=== 실행 방식 ===
  kiki                          인터랙티브 쉘
  kiki ask "질문"               단일 질문(원샷)
  kiki "질문"                   ask 단축형
  kiki --help                   도움말(전체)

=== LLM 질문(대화) ===
  ?질문                         LLM 질의 (현재 profile/stream/files/ctx/rag 반영)

=== 내부 명령(:로 시작) ===
  :help [topic]                 도움말 (topic: shell|llm|file|ctx|ctx-size|rag|ui|env)
  :profile fast|deep|none       프로파일 변경
  :stream on|off                스트리밍 출력 on/off
  :ui header on|off             상단 헤더 표시 on/off
  :ui clear on|off              화면 전체 clear(레거시) on/off
  :ctx set key=value            컨텍스트 설정 (예: cluster, ns)
  :ctx show                     컨텍스트 표시
  :ctx clear                    컨텍스트 초기화
  :ctx-size N                   목표 ctx-size 설정 (서버 재시작 필요)
  :pending                      대기 중 명령 목록
  :approve <id>|all              대기 명령 실행 승인(실행)
  :reject <id>|all               대기 명령 폐기
  :approval on|off               승인 워크플로우 토글
  :rag on|off                   RAG 토글
  :rag add /path                파일을 RAG 인덱스에 적재(청킹 후 저장)
  :rag query "text"             RAG 검색 테스트(TopK)
  :rag stats                    RAG 상태
  :rag clear                    RAG 인덱스 초기화
  :bash                         PTY 기반 bash 진입 (exit로 복귀)
  :exit | :quit                 종료

=== 파일 첨부 ===
  -f /path                      (원샷) 파일 첨부 (RAG on이면 자동 인덱싱)
  :file add /path               (쉘) 파일 첨부 (RAG on이면 자동 인덱싱)
  :file list                    (쉘) 첨부 목록
  :file rm N                    (쉘) N번째 제거
  :file clear                   (쉘) 전체 제거

=== 쉘 ===
  cd /path                      디렉터리 이동
  그 외 입력                    /bin/bash -lc 로 실행(결과 누적)

TIP:
  - 헤더 고정: KIKI_UI_FIXED=1 (default)
  - 헤더 줄 수: KIKI_UI_HEADERLINES=5
  - LLM 서버:  LLM_HOST/LLM_PORT 또는 LLM_BASE_URL
`)
}

func printHelpTopic(topic string) {
	t := strings.ToLower(strings.TrimSpace(topic))
	switch t {
	case "shell":
		fmt.Println(`
[help:shell]
  - 인터랙티브 모드: kiki
  - 프롬프트:
      ?질문    -> LLM에게 질문 (RAG/CTX/파일 반영)
  - 내부 명령:
      :bash    -> PTY bash (exit로 복귀)
      :profile -> profile 변경
      :stream  -> 스트림 토글
      :rag     -> RAG 토글/적재/검색
      :ctx     -> cluster/ns 같은 컨텍스트 설정
      :ctx-size-> ctx-size 목표값 설정(서버 재시작 필요)
      :exit    -> 종료
  - 일반 명령 입력은 /bin/bash -lc 로 실행됩니다.
`)
	case "llm":
		fmt.Println(`
[help:llm]
  - LLM 서버(OpenAI 호환 /v1/chat/completions):
      LLM_HOST (default 10.0.2.253)
      LLM_PORT (default 8080)
      LLM_BASE_URL (예: http://10.0.2.253:8080)
  - 튜닝:
      LLM_MODEL (default llama)
      LLM_TEMP
      LLM_MAX_TOKENS
      LLM_TIMEOUT
      LLM_PROFILE (none|fast|deep)
      LLM_STREAM (0|1)
      LLM_SYSTEM_PROMPT
  - 예:
      LLM_PROFILE=deep LLM_STREAM=1 ./kiki ask "원인 분석해줘"
`)
	case "file":
		fmt.Println(`
[help:file]
  - 원샷 첨부:
      ./kiki -f /var/log/messages ask "이 로그 분석"
  - 쉘 첨부:
      :file add /var/log/messages
      :file list
      :file rm 1
      :file clear
  - 제한:
      LLM_FILE_MAX_BYTES (default 256KB)
      LLM_FILE_MAX_CHARS (default 20000)
  - RAG 연동:
      RAG on 상태에서 파일을 add 하면 자동으로 RAG 인덱싱합니다.
`)
	case "ctx":
		fmt.Println(`
[help:ctx]
  - 컨텍스트는 시스템 프롬프트 뒤에 [Context] 섹션으로 붙습니다.
      :ctx set cluster=prod
      :ctx set ns=kube-system
      :ctx show
      :ctx clear
`)
	case "ctx-size":
		fmt.Println(`
[help:ctx-size]
  - ctx-size는 LLM의 컨텍스트 윈도우(토큰 수)입니다.
  - KIKI는 목표값을 저장/표시할 수 있지만, 실제 적용은 llama.cpp 서버를 --ctx-size 로 재시작해야 합니다.

  명령:
    :ctx-size 15000

  환경변수:
    LLM_CTX_TARGET=15000
    LLM_CTX_OBSERVED=8192   (선택: 관측값 강제)
`)
	case "rag":
		fmt.Println(`
[help:rag]
  - 로컬 RAG: 파일을 청킹해서 ~/.kiki/rag.json에 저장한 뒤, 질문 시 관련 청크를 자동으로 붙입니다.
  - 명령:
      :rag on|off
      :rag add /path/to/file
      :rag query "검색어"
      :rag stats
      :rag clear
  - 환경변수:
      LLM_RAG=1|0
      LLM_RAG_PATH=~/.kiki/rag.json
      LLM_RAG_TOPK=3
      LLM_RAG_CHUNK_CHARS=1200
      LLM_RAG_OVERLAP=200
      LLM_RAG_MAXCHARS=4000
`)
	case "ui":
		fmt.Println(`
[help:ui]
  - 헤더 고정(권장):
      KIKI_UI_FIXED=1
      KIKI_UI_HEADERLINES=5
  - 토글:
      :ui header on|off
      :ui clear on|off   (레거시: 전체 clear)
`)
	case "env":
		fmt.Println(`
[help:env]
  UI:
    KIKI_UI_HEADER=1|0
    KIKI_UI_FIXED=1|0
    KIKI_UI_HEADERLINES=5
    KIKI_UI_CLEAR=1|0
    KIKI_UI_MAXFILES=120
    KIKI_APPROVAL=1|0

  LLM:
    LLM_HOST, LLM_PORT, LLM_BASE_URL
    LLM_MODEL, LLM_TEMP, LLM_MAX_TOKENS, LLM_TIMEOUT
    LLM_PROFILE (none|fast|deep)
    LLM_STREAM (0|1)
    LLM_SYSTEM_PROMPT
    LLM_CTX_TARGET, LLM_CTX_OBSERVED

  RAG:
    LLM_RAG, LLM_RAG_PATH, LLM_RAG_TOPK
    LLM_RAG_CHUNK_CHARS, LLM_RAG_OVERLAP, LLM_RAG_MAXCHARS

  History:
    LLM_HISTORY, LLM_HISTORY_PATH, LLM_HISTORY_PREVIEW
    LLM_CAPTURE_FULL, LLM_CAPTURE_MAX
`)
	default:
		printHelpAll()
		fmt.Println("topics: shell | llm | file | ctx | ctx-size | rag | ui | env")
	}
}

func printHelp(args ...string) {
	if len(args) == 0 {
		printHelpAll()
		return
	}
	printHelpTopic(args[0])
}

// ---------------- CLI flags ----------------

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v != "" {
		*m = append(*m, v)
	}
	return nil
}

// ---------------- Ask ----------------

func doAsk(cfg *Config, st *ShellState, prompt string, overrideSystem string) {
	endpoint := buildEndpoint(cfg)

	userContent, usedFiles, hashes, err := buildUserContent(prompt, st.Files, cfg, st)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attach error:", err)
		return
	}
	sys := systemPromptWithCtx(cfg, st, overrideSystem)

	req := ChatRequest{
		Model:       cfg.Model,
		Temperature: cfg.Temp,
		MaxTokens:   cfg.MaxTokens,
		Stream:      st.Stream,
		Messages: []ChatMessage{
			{Role: "system", Content: sys},
			{Role: "user", Content: userContent},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() { <-sigCh; cancel() }()

	now := time.Now().Format(time.RFC3339)
	cwd, _ := os.Getwd()

	if st.Stream {
		captured, err := doStream(ctx, cfg, endpoint, req)
		if err != nil {
			fmt.Fprintln(os.Stderr, "LLM stream error:", err)
			if obs := parseCtxSizeFromError(err); obs > 0 {
				st.CtxSizeObserved = obs
			}
			if st.CtxSizeTarget > 0 && st.CtxSizeObserved > 0 && st.CtxSizeTarget > st.CtxSizeObserved {
				fmt.Fprintf(os.Stderr, "hint: server ctx-size=%d. restart llama.cpp server with --ctx-size %d (or higher).\n", st.CtxSizeObserved, st.CtxSizeTarget)
			}
			return
		}
		st.LastAnswer = captured
		if cfg.HistoryEnabled {
			appendHistory(cfg.HistoryPath, HistoryRecord{
				Time:         now,
				Endpoint:     endpoint,
				Profile:      st.Profile,
				Model:        cfg.Model,
				Temperature:  cfg.Temp,
				MaxTokens:    cfg.MaxTokens,
				Stream:       true,
				SystemPrompt: sys,
				Ctx:          st.Ctx,
				Prompt:       prompt,
				Files:        usedFiles,
				FileHashes:   hashes,
				Cwd:          cwd,
				ResponsePrev: truncateRunes(captured, cfg.HistoryPreview),
			})
		}
		return
	}

	out, err := doNonStream(ctx, cfg, endpoint, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "LLM error:", err)
		if obs := parseCtxSizeFromError(err); obs > 0 {
			st.CtxSizeObserved = obs
		}
		if st.CtxSizeTarget > 0 && st.CtxSizeObserved > 0 && st.CtxSizeTarget > st.CtxSizeObserved {
			fmt.Fprintf(os.Stderr, "hint: server ctx-size=%d. restart llama.cpp server with --ctx-size %d (or higher).\n", st.CtxSizeObserved, st.CtxSizeTarget)
		}
		return
	}
	fmt.Println(out)
	st.LastAnswer = out
	if cfg.HistoryEnabled {
		appendHistory(cfg.HistoryPath, HistoryRecord{
			Time:         now,
			Endpoint:     endpoint,
			Profile:      st.Profile,
			Model:        cfg.Model,
			Temperature:  cfg.Temp,
			MaxTokens:    cfg.MaxTokens,
			Stream:       false,
			SystemPrompt: sys,
			Ctx:          st.Ctx,
			Prompt:       prompt,
			Files:        usedFiles,
			FileHashes:   hashes,
			Cwd:          cwd,
			ResponsePrev: truncateRunes(out, cfg.HistoryPreview),
		})
	}
}

// ---------------- Internal Commands ----------------

func handleInternalCommand(cfg *Config, st *ShellState, line string) bool {
	cmdline := strings.TrimSpace(strings.TrimPrefix(line, ":"))
	if cmdline == "" {
		return true
	}
	parts := strings.Fields(cmdline)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help":
		if len(args) > 0 {
			printHelp(args[0])
		} else {
			printHelp()
		}
		return true

	case "bash":
		fmt.Println("\n[Entering interactive bash] (type 'exit' to return)\n")
		if err := runInteractiveBash(); err != nil {
			fmt.Fprintln(os.Stderr, "pty bash error:", err)
		}
		fmt.Println("\n[Back to KIKI]\n")
		if st.UI.FixedHeader {
			renderHeader(cfg, st)
		}
		return true

	case "profile":
		if len(args) < 1 {
			fmt.Println("usage: :profile fast|deep|none")
			return true
		}
		st.Profile = args[0]
		cfg.Profile = st.Profile
		applyProfile(cfg)
		return true

	case "stream":
		if len(args) < 1 {
			fmt.Println("usage: :stream on|off")
			return true
		}
		v := strings.ToLower(args[0])
		st.Stream = (v == "on" || v == "1" || v == "true")
		cfg.Stream = st.Stream
		return true

	case "ui":
		if len(args) < 2 {
			fmt.Println("usage: :ui header on|off | :ui clear on|off")
			return true
		}
		sub := strings.ToLower(args[0])
		val := strings.ToLower(args[1])
		on := (val == "on" || val == "1" || val == "true")
		if sub == "header" {
			st.UI.ShowHeader = on
			return true
		}
		if sub == "clear" {
			st.UI.ClearOnDraw = on
			return true
		}
		fmt.Println("usage: :ui header on|off | :ui clear on|off")
		return true

	case "ctx":
		// :ctx set key=value | :ctx show | :ctx clear
		if len(args) < 1 {
			fmt.Println("usage: :ctx set key=value | :ctx show | :ctx clear")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "set":
			if len(args) < 2 {
				fmt.Println("usage: :ctx set key=value")
				return true
			}
			kv := strings.Join(args[1:], " ")
			kvp := strings.SplitN(kv, "=", 2)
			if len(kvp) != 2 {
				fmt.Println("usage: :ctx set key=value")
				return true
			}
			k := strings.TrimSpace(kvp[0])
			v := strings.TrimSpace(kvp[1])
			ctxSet(st, k, v)
			fmt.Printf("ctx set: %s=%s\n", k, v)
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true
		case "show":
			if len(st.Ctx) == 0 {
				fmt.Println("(ctx empty)")
				return true
			}
			keys := make([]string, 0, len(st.Ctx))
			for k := range st.Ctx {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%s=%s\n", k, st.Ctx[k])
			}
			return true
		case "clear":
			ctxClear(st)
			fmt.Println("ctx cleared")
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true
		default:
			fmt.Println("usage: :ctx set key=value | :ctx show | :ctx clear")
			return true
		}

	case "ctx-size", "ctxsize":
		if len(args) < 1 {
			fmt.Printf("ctx-size: observed=%d target=%d\n", st.CtxSizeObserved, st.CtxSizeTarget)
			fmt.Println("usage: :ctx-size <N>")
			return true
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 256 {
			fmt.Println("invalid ctx-size:", args[0])
			return true
		}
		st.CtxSizeTarget = n
		cfg.CtxSizeTarget = n
		fmt.Println("ctx-size target set:", n)
		fmt.Println("note: actual ctx-size requires llama.cpp server restart with --ctx-size", n)
		if st.UI.FixedHeader {
			renderHeader(cfg, st)
		}
		return true

	case "rag":
		// :rag on|off | add <path> | query <text> | stats | clear
		if len(args) < 1 {
			fmt.Println("usage: :rag on|off | add <path> | query <text> | stats | clear")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "on":
			st.RagEnabled = true
			cfg.RagEnabled = true
			fmt.Println("rag: on")
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true
		case "off":
			st.RagEnabled = false
			cfg.RagEnabled = false
			fmt.Println("rag: off")
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true
		case "add":
			if len(args) < 2 {
				fmt.Println("usage: :rag add <path>")
				return true
			}
			if st.Rag == nil {
				fmt.Println("rag not initialized")
				return true
			}
			p := normalizePath(strings.Join(args[1:], " "))
			if !fileExists(p) {
				fmt.Println("file not found:", p)
				return true
			}
			if err := st.Rag.addFile(p, cfg.FileMaxBytes, cfg.RagChunkChars, cfg.RagOverlap); err != nil {
				fmt.Println("rag add error:", err)
				return true
			}
			fmt.Println("rag indexed:", p)
			return true
		case "query":
			if st.Rag == nil {
				fmt.Println("rag not initialized")
				return true
			}
			q := strings.TrimSpace(strings.Join(args[1:], " "))
			if q == "" {
				fmt.Println("usage: :rag query <text>")
				return true
			}
			hits := st.Rag.search(q, cfg.RagTopK, nil)
			if len(hits) == 0 {
				fmt.Println("(no hits)")
				return true
			}
			for _, h := range hits {
				fmt.Printf("score=%.3f file=%s off=%d\n", h.Score, h.Chunk.File, h.Chunk.Offset)
				fmt.Println(truncateRunes(h.Chunk.Text, 240))
				fmt.Println()
			}
			return true
		case "stats":
			if st.Rag == nil {
				fmt.Println("rag not initialized")
				return true
			}
			fmt.Printf("rag: enabled=%v path=%s dim=%d chunks=%d topk=%d\n", st.RagEnabled, st.Rag.Path, st.Rag.Dim, len(st.Rag.Chunks), cfg.RagTopK)
			return true
		case "clear":
			if st.Rag == nil {
				fmt.Println("rag not initialized")
				return true
			}
			st.Rag.clear()
			fmt.Println("rag cleared")
			return true
		default:
			fmt.Println("usage: :rag on|off | add <path> | query <text> | stats | clear")
			return true
		}

	case "file":
		if len(args) < 1 {
			printFiles(st)
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "add":
			if len(args) < 2 {
				fmt.Println("usage: :file add <path>")
				return true
			}
			p := normalizePath(strings.Join(args[1:], " "))
			if !fileExists(p) {
				fmt.Println("file not found:", p)
				return true
			}
			for _, f := range st.Files {
				if f == p {
					fmt.Println("already attached:", p)
					return true
				}
			}
			st.Files = append(st.Files, p)
			fmt.Println("attached:", p)

			// file -> rag (auto index)
			if st.RagEnabled && st.Rag != nil {
				_ = st.Rag.addFile(p, cfg.FileMaxBytes, cfg.RagChunkChars, cfg.RagOverlap)
			}
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true

		case "list", "ls":
			printFiles(st)
			return true

		case "rm", "remove", "del":
			if len(args) < 2 {
				fmt.Println("usage: :file rm <N>")
				return true
			}
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				fmt.Println("invalid index:", args[1])
				return true
			}
			if !removeFileAt(st, n-1) {
				fmt.Println("no such file index:", n)
				return true
			}
			fmt.Println("removed:", n)
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true

		case "clear":
			st.Files = nil
			fmt.Println("files cleared")
			if st.UI.FixedHeader {
				renderHeader(cfg, st)
			}
			return true

		default:
			fmt.Println("usage: :file add <path> | :file list | :file rm <N> | :file clear")
			return true
		}

	case "pending":
		if len(st.Pending) == 0 {
			fmt.Println("(no pending commands)")
			return true
		}
		// oldest first
		sort.SliceStable(st.Pending, func(i, j int) bool { return st.Pending[i].ID < st.Pending[j].ID })
		for _, p := range st.Pending {
			fmt.Printf("%d) %s | %s\n", p.ID, p.Time, p.Cmd)
		}
		return true

	case "approve":
		if len(args) < 1 {
			fmt.Println("usage: :approve <id>|all")
			return true
		}
		if strings.ToLower(args[0]) == "all" {
			if len(st.Pending) == 0 {
				fmt.Println("(no pending commands)")
				return true
			}
			sort.SliceStable(st.Pending, func(i, j int) bool { return st.Pending[i].ID < st.Pending[j].ID })
			pending := st.Pending
			st.Pending = []PendingCmd{}
			for _, p := range pending {
				fmt.Printf("\n[approve] #%d: %s\n", p.ID, p.Cmd)
				exit := runBashOnce(p.Cmd)
				fmt.Printf("[exit] %d\n", exit)
			}
			return true
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			fmt.Println("invalid id:", args[0])
			return true
		}
		idx := -1
		for i := range st.Pending {
			if st.Pending[i].ID == n {
				idx = i
				break
			}
		}
		if idx < 0 {
			fmt.Println("no such pending id:", n)
			return true
		}
		p := st.Pending[idx]
		st.Pending = append(st.Pending[:idx], st.Pending[idx+1:]...)
		fmt.Printf("\n[approve] #%d: %s\n", p.ID, p.Cmd)
		exit := runBashOnce(p.Cmd)
		fmt.Printf("[exit] %d\n", exit)
		return true

	case "reject":
		if len(args) < 1 {
			fmt.Println("usage: :reject <id>|all")
			return true
		}
		if strings.ToLower(args[0]) == "all" {
			st.Pending = []PendingCmd{}
			fmt.Println("rejected: all")
			return true
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			fmt.Println("invalid id:", args[0])
			return true
		}
		idx := -1
		for i := range st.Pending {
			if st.Pending[i].ID == n {
				idx = i
				break
			}
		}
		if idx < 0 {
			fmt.Println("no such pending id:", n)
			return true
		}
		st.Pending = append(st.Pending[:idx], st.Pending[idx+1:]...)
		fmt.Println("rejected:", n)
		return true

	case "approval":
		if len(args) < 1 {
			mode := "off"
			if st.ApprovalMode {
				mode = "on"
			}
			fmt.Println("approval mode:", mode)
			fmt.Println("usage: :approval on|off")
			return true
		}
		v := strings.ToLower(args[0])
		st.ApprovalMode = (v == "on" || v == "1" || v == "true")
		mode := "off"
		if st.ApprovalMode {
			mode = "on"
		}
		fmt.Println("approval mode:", mode)
		return true
	case "exit", "quit":
		os.Exit(0)
		return true
	}

	fmt.Println("unknown command. try :help")
	return true
}

// ---------------- Shell execution ----------------

func runBashOnce(cmdline string) int {
	c := exec.Command("/bin/bash", "-lc", cmdline)
	c.Stdout, c.Stderr, c.Stdin = os.Stdout, os.Stderr, os.Stdin
	err := c.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "bash 실행 실패:", err)
	return 127
}

func runREPL(cfg *Config, ui UIConfig, rag *RagIndex, approvalMode bool) {
	st := &ShellState{
		Files:           []string{},
		Profile:         cfg.Profile,
		Stream:          cfg.Stream,
		Ctx:             map[string]string{},
		CtxSizeTarget:   cfg.CtxSizeTarget,
		CtxSizeObserved: cfg.CtxSizeObserved,
		RagEnabled:      cfg.RagEnabled,
		Rag:             rag,
		UI:              ui,
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024), 1024*1024)

	for {
		renderHeader(cfg, st)
		fmt.Print(promptLine(st))
		if !sc.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			handleInternalCommand(cfg, st, line)
			continue
		}
		if strings.HasPrefix(line, "?") {
			q := strings.TrimSpace(strings.TrimPrefix(line, "?"))
			if q != "" {
				doAsk(cfg, st, q, "")
			}
			continue
		}
		if strings.HasPrefix(line, "cd ") || line == "cd" {
			target := strings.TrimSpace(strings.TrimPrefix(line, "cd"))
			if target == "" {
				home, _ := os.UserHomeDir()
				target = home
			}
			if strings.HasPrefix(target, "~") {
				home, _ := os.UserHomeDir()
				target = filepath.Join(home, strings.TrimPrefix(target, "~"))
			}
			if err := os.Chdir(target); err != nil {
				fmt.Fprintln(os.Stderr, "cd error:", err)
			}
			continue
		}

		if st.ApprovalMode {
			id := st.NextPendingID
			st.NextPendingID++
			st.Pending = append(st.Pending, PendingCmd{ID: id, Time: time.Now().Format("15:04:05"), Cmd: line})
			fmt.Printf("queued: #%d (use :approve %d or :reject %d)\n", id, id, id)
			continue
		}
		_ = runBashOnce(line)
	}
}

func main() {
	cfg := &Config{
		Host:         envString("LLM_HOST", "10.0.2.253"),
		Port:         envInt("LLM_PORT", 8080),
		BaseURL:      envString("LLM_BASE_URL", ""),
		Model:        envString("LLM_MODEL", "llama"),
		Temp:         envFloat("LLM_TEMP", 0.2),
		MaxTokens:    envInt("LLM_MAX_TOKENS", 512),
		TimeoutSec:   envInt("LLM_TIMEOUT", 60),
		SystemPrompt: envString("LLM_SYSTEM_PROMPT", "당신은 간결하고 정확하게 답변하는 도우미입니다."),
		Profile:      envString("LLM_PROFILE", "fast"),
		Stream:       envBool("LLM_STREAM", false),

		CtxSizeTarget:   envInt("LLM_CTX_TARGET", 0),
		CtxSizeObserved: envInt("LLM_CTX_OBSERVED", 0),

		// RAG defaults
		RagEnabled:    envBool("LLM_RAG", true),
		RagPath:       envString("LLM_RAG_PATH", defaultRagPath()),
		RagTopK:       envInt("LLM_RAG_TOPK", 3),
		RagChunkChars: envInt("LLM_RAG_CHUNK_CHARS", 1200),
		RagOverlap:    envInt("LLM_RAG_OVERLAP", 200),
		RagMaxChars:   envInt("LLM_RAG_MAXCHARS", 4000),

		HistoryEnabled: envBool("LLM_HISTORY", true),
		HistoryPath:    envString("LLM_HISTORY_PATH", defaultHistoryPath()),
		HistoryPreview: envInt("LLM_HISTORY_PREVIEW", 800),

		FileMaxBytes: envInt("LLM_FILE_MAX_BYTES", 256*1024),
		FileMaxChars: envInt("LLM_FILE_MAX_CHARS", 20000),

		CaptureFull: envBool("LLM_CAPTURE_FULL", false),
		CaptureMax:  envInt("LLM_CAPTURE_MAX", 2_000_000),
	}

	ui := UIConfig{
		ShowHeader:   envBool("KIKI_UI_HEADER", true),
		ClearOnDraw:  envBool("KIKI_UI_CLEAR", false),
		FixedHeader:  envBool("KIKI_UI_FIXED", true),
		HeaderLines:  envInt("KIKI_UI_HEADERLINES", 5),
		UseUnicode:   true,
		MaxFilesLine: envInt("KIKI_UI_MAXFILES", 120),
	}

	approvalMode := envBool("KIKI_APPROVAL", false)

	var files multiFlag
	fHelp := flag.Bool("help", false, "show help")
	fProfile := flag.String("profile", "", "profile none|fast|deep")
	fStream := flag.Bool("stream", false, "stream output")
	fUIHeader := flag.Bool("ui-header", ui.ShowHeader, "show header in interactive shell")
	fUIClear := flag.Bool("ui-clear", ui.ClearOnDraw, "clear screen before drawing header")
	fUIMaxFiles := flag.Int("ui-maxfiles", ui.MaxFilesLine, "max width for files line (runes)")
	fApproval := flag.Bool("approval", false, "require approval before executing shell commands")

	flag.Var(&files, "f", "attach file (repeatable)")
	flag.Parse()

	if *fHelp {
		printHelp()
		return
	}
	if strings.TrimSpace(*fProfile) != "" {
		cfg.Profile = *fProfile
	}
	if *fStream {
		cfg.Stream = true
	}
	ui.ShowHeader = *fUIHeader
	ui.ClearOnDraw = *fUIClear
	ui.MaxFilesLine = *fUIMaxFiles
	if *fApproval {
		approvalMode = true
	}

	applyProfile(cfg)

	// init rag
	rag := loadRagIndex(cfg.RagPath, 256)

	args := flag.Args()
	if len(args) > 0 {
		st := &ShellState{
			Files:           []string(files),
			Profile:         cfg.Profile,
			Stream:          cfg.Stream,
			Ctx:             map[string]string{},
			CtxSizeTarget:   cfg.CtxSizeTarget,
			CtxSizeObserved: cfg.CtxSizeObserved,
			RagEnabled:      cfg.RagEnabled,
			Rag:             rag,
			UI:              ui,
		}

		// oneshot: auto index attached files if rag enabled
		if st.RagEnabled && st.Rag != nil && len(st.Files) > 0 {
			for _, f := range st.Files {
				_ = st.Rag.addFile(f, cfg.FileMaxBytes, cfg.RagChunkChars, cfg.RagOverlap)
			}
		}

		if args[0] == "ask" {
			p := strings.TrimSpace(strings.Join(args[1:], " "))
			if p == "" {
				fmt.Fprintln(os.Stderr, "ask: prompt is empty")
				os.Exit(1)
			}
			doAsk(cfg, st, p, "")
			return
		}
		p := strings.TrimSpace(strings.Join(args, " "))
		if p == "" {
			fmt.Fprintln(os.Stderr, "prompt is empty")
			os.Exit(1)
		}
		doAsk(cfg, st, p, "")
		return
	}

	runREPL(cfg, ui, rag, approvalMode)
}
