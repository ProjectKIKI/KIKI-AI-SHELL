package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ShowHeader   bool
	ClearOnDraw  bool
	FixedHeader  bool
	HeaderLines  int
	MaxFilesLine int
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
func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func DefaultConfig() Config {
	return Config{
		ShowHeader:   envBool("KIKI_UI_HEADER", true),
		ClearOnDraw:  envBool("KIKI_UI_CLEAR", false),
		FixedHeader:  envBool("KIKI_UI_FIXED", true),
		HeaderLines:  envInt("KIKI_UI_HEADERLINES", 5),
		MaxFilesLine: envInt("KIKI_UI_MAXFILES", 120),
	}
}

func ResetScroll() { fmt.Print("\033[r") }

type HeaderData struct {
	Title       string
	LLM         string
	Profile     string
	Stream      bool
	CtxObserved int
	CtxTarget   int
	Cluster     string
	Namespace   string
	Files       []string
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
func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

func RenderHeader(cfg Config, hd HeaderData) {
	if !cfg.ShowHeader {
		return
	}
	if cfg.ClearOnDraw {
		fmt.Print("\033[H\033[2J")
	}

	cols := 100
	inner := cols - 2

	title := hd.Title
	if strings.TrimSpace(title) == "" {
		title = " KIKI AI SHELL "
	}

	top := "┌" + strings.Repeat("─", 1) + title
	remain := inner - (1 + len([]rune(title)))
	if remain < 0 {
		remain = 0
	}
	top += strings.Repeat("─", remain) + "┐"

	stream := "off"
	if hd.Stream {
		stream = "on"
	}
	line1 := fmt.Sprintf(" LLM: %s | profile: %s | stream: %s ", hd.LLM, hd.Profile, stream)
	if hd.CtxObserved > 0 || hd.CtxTarget > 0 {
		if hd.CtxObserved > 0 && hd.CtxTarget > 0 {
			line1 += fmt.Sprintf("| ctx: %d/%d ", hd.CtxObserved, hd.CtxTarget)
		}
		if hd.CtxObserved > 0 && hd.CtxTarget == 0 {
			line1 += fmt.Sprintf("| ctx: %d ", hd.CtxObserved)
		}
		if hd.CtxObserved == 0 && hd.CtxTarget > 0 {
			line1 += fmt.Sprintf("| ctx: %d ", hd.CtxTarget)
		}
	}

	k8s := " K8S: (none) "
	if strings.TrimSpace(hd.Cluster) != "" || strings.TrimSpace(hd.Namespace) != "" {
		c := hd.Cluster
		if c == "" {
			c = "-"
		}
		n := hd.Namespace
		if n == "" {
			n = "-"
		}
		k8s = fmt.Sprintf(" K8S: cluster=%s | ns=%s ", c, n)
	}

	files := "(none)"
	if len(hd.Files) > 0 {
		files = strings.Join(hd.Files, ", ")
	}
	maxFiles := cfg.MaxFilesLine
	if maxFiles <= 0 {
		maxFiles = 120
	}
	files = elideMiddle(files, maxFiles)
	line3 := fmt.Sprintf(" Files: %s ", files)

	line1 = padRight(elideMiddle(line1, inner), inner)
	line2 := padRight(elideMiddle(k8s, inner), inner)
	line3 = padRight(elideMiddle(line3, inner), inner)

	fmt.Println(top)
	fmt.Println("│" + line1 + "│")
	fmt.Println("│" + line2 + "│")
	fmt.Println("│" + line3 + "│")
	fmt.Println("└" + strings.Repeat("─", inner) + "┘")
}
