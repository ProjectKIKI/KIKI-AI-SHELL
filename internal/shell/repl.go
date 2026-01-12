package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/ui"
)

func renderHeader(cfg *config.Config, st *State) {
	llmAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if strings.TrimSpace(cfg.BaseURL) != "" {
		llmAddr = cfg.BaseURL
	}
	ui.RenderHeader(st.UI, ui.HeaderData{
		Title:       " KIKI AI SHELL ",
		LLM:         llmAddr,
		Profile:     st.Profile,
		Stream:      st.Stream,
		CtxObserved: st.CtxSizeObserved,
		CtxTarget:   st.CtxSizeTarget,
		Cluster:     ctxGet(st, "cluster"),
		Namespace:   ctxGet(st, "ns"),
		Files:       st.Files,
	})
}

func RunREPL(cfg *config.Config, uicfg ui.Config) {
	st := NewState(cfg, uicfg)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024), 1024*1024)

	for {
		renderHeader(cfg, st)
		fmt.Print(promptLine(st))
		if !sc.Scan() {
			fmt.Println()
			ui.ResetScroll()
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
				Ask(cfg, st, q, "")
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

		_ = runBashOnce(line)
	}
}

func handleInternalCommand(cfg *config.Config, st *State, line string) bool {
	cmdline := strings.TrimSpace(strings.TrimPrefix(line, ":"))
	if cmdline == "" {
		return true
	}
	parts := strings.Fields(cmdline)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help":
		PrintHelp()
		return true
	case "bash":
		fmt.Print("\n[Entering bash] (type 'exit' to return)\n")
		if err := runInteractiveBash(); err != nil {
			fmt.Fprintln(os.Stderr, "bash error:", err)
		}
		fmt.Print("\n[Back to KIKI]\n")
		return true
	case "profile":
		if len(args) < 1 {
			fmt.Println("usage: :profile fast|deep|none")
			return true
		}
		st.Profile = args[0]
		cfg.Profile = st.Profile
		config.ApplyProfile(cfg)
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
	case "timeout":
		if len(args) < 1 {
			fmt.Printf("timeout: %ds\n", cfg.TimeoutSec)
			fmt.Println("usage: :timeout <sec>")
			return true
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			fmt.Println("invalid timeout:", args[0])
			return true
		}
		cfg.TimeoutSec = n
		fmt.Println("timeout set:", n, "sec")
		return true
	case "ui":
		if len(args) < 2 {
			fmt.Println("usage: :ui header on|off")
			return true
		}
		sub := strings.ToLower(args[0])
		val := strings.ToLower(args[1])
		on := (val == "on" || val == "1" || val == "true")
		if sub == "header" {
			st.UI.ShowHeader = on
			return true
		}
		fmt.Println("usage: :ui header on|off")
		return true
	case "ctx":
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
			ctxSet(st, kvp[0], kvp[1])
			fmt.Printf("ctx set: %s=%s\n", strings.TrimSpace(kvp[0]), strings.TrimSpace(kvp[1]))
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
		fmt.Println("note: restart llama.cpp server with --ctx-size", n)
		return true
	case "rag":
		if len(args) < 1 {
			en, docs := st.RAG.Stats()
			fmt.Printf("rag: enabled=%v docs=%d\n", en, docs)
			fmt.Println("usage: :rag on|off | :rag add <path> | :rag stats | :rag clear")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "on":
			st.RAG.Toggle(true)
			cfg.RAGEnabled = true
			fmt.Println("rag enabled")
			return true
		case "off":
			st.RAG.Toggle(false)
			cfg.RAGEnabled = false
			fmt.Println("rag disabled")
			return true
		case "add":
			if len(args) < 2 {
				fmt.Println("usage: :rag add <path>")
				return true
			}
			p := strings.Join(args[1:], " ")
			d, err := st.RAG.AddFile(p, cfg.FileMaxBytes, cfg.FileMaxChars)
			if err != nil {
				fmt.Fprintln(os.Stderr, "rag add error:", err)
				return true
			}
			fmt.Println("rag added:", d.Path)
			return true
		case "stats":
			en, docs := st.RAG.Stats()
			fmt.Printf("rag: enabled=%v docs=%d\n", en, docs)
			return true
		case "clear":
			st.RAG.Clear()
			fmt.Println("rag cleared")
			return true
		default:
			fmt.Println("usage: :rag on|off | :rag add <path> | :rag stats | :rag clear")
			return true
		}
	case "file":
		if len(args) < 1 {
			fmt.Println("usage: :file add|list|rm|clear ...")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "add":
			if len(args) < 2 {
				fmt.Println("usage: :file add /path")
				return true
			}
			p := normalizePath(strings.Join(args[1:], " "))
			if !fileExists(p) {
				fmt.Println("file not found:", p)
				return true
			}
			st.Files = append(st.Files, p)
			fmt.Println("file added:", p)
			// file->rag auto
			if st.RAG != nil && st.RAG.Enabled {
				_, _ = st.RAG.AddFile(p, cfg.FileMaxBytes, cfg.FileMaxChars)
			}
			return true
		case "list":
			if len(st.Files) == 0 {
				fmt.Println("(no attached files)")
				return true
			}
			for i, f := range st.Files {
				fmt.Printf("%d) %s\n", i+1, f)
			}
			return true
		case "rm":
			if len(args) < 2 {
				fmt.Println("usage: :file rm N")
				return true
			}
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 || n > len(st.Files) {
				fmt.Println("invalid index")
				return true
			}
			idx := n - 1
			removed := st.Files[idx]
			st.Files = append(st.Files[:idx], st.Files[idx+1:]...)
			fmt.Println("file removed:", removed)
			return true
		case "clear":
			st.Files = []string{}
			fmt.Println("files cleared")
			return true
		default:
			fmt.Println("usage: :file add|list|rm|clear ...")
			return true
		}
	case "history":
		// :history show [N] | :history search <regex> [N] | :history path
		if len(args) < 1 {
			fmt.Println("usage: :history show [N] | :history search <regex> [N] | :history path")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "path":
			fmt.Println(cfg.HistoryPath)
			return true
		case "show":
			limit := 20
			if len(args) >= 2 {
				if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
					limit = n
				}
			}
			recs, err := history.ReadAll(cfg.HistoryPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "history read error:", err)
				return true
			}
			if len(recs) == 0 {
				fmt.Println("(history empty)")
				return true
			}
			if limit > len(recs) {
				limit = len(recs)
			}
			start := len(recs) - limit
			for i := start; i < len(recs); i++ {
				r := recs[i]
				idx := i + 1
				fmt.Printf("#%d %s | profile=%s | stream=%v | prompt=%s\n", idx, r.Time, r.Profile, r.Stream, truncateRunes(r.Prompt, 120))
				if r.ResponsePrev != "" {
					fmt.Printf("    ↳ %s\n", truncateRunes(r.ResponsePrev, 160))
				}
			}
			return true
		case "search":
			if len(args) < 2 {
				fmt.Println("usage: :history search <regex> [N]")
				return true
			}
			pattern := args[1]
			limit := 20
			if len(args) >= 3 {
				if n, err := strconv.Atoi(args[2]); err == nil && n > 0 {
					limit = n
				}
			}
			recs, err := history.ReadAll(cfg.HistoryPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "history read error:", err)
				return true
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid regex:", err)
				return true
			}
			matches := 0
			for i := len(recs) - 1; i >= 0; i-- {
				r := recs[i]
				if re.MatchString(r.Prompt) || re.MatchString(r.ResponsePrev) {
					idx := i + 1
					fmt.Printf("#%d %s | profile=%s | prompt=%s\n", idx, r.Time, r.Profile, truncateRunes(r.Prompt, 120))
					if r.ResponsePrev != "" {
						fmt.Printf("    ↳ %s\n", truncateRunes(r.ResponsePrev, 160))
					}
					matches++
					if matches >= limit {
						break
					}
				}
			}
			if matches == 0 {
				fmt.Println("(no matches)")
			}
			return true
		default:
			fmt.Println("usage: :history show [N] | :history search <regex> [N] | :history path")
			return true
		}
	case "exit", "quit":
	case "history":
		// :history show [N] | :history search <regex> [N] | :history path
		if len(args) < 1 {
			fmt.Println("usage: :history show [N] | :history search <regex> [N] | :history path")
			return true
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "path":
			fmt.Println(cfg.HistoryPath)
			return true
		case "show":
			limit := 20
			if len(args) >= 2 {
				if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
					limit = n
				}
			}
			recs, err := history.ReadAll(cfg.HistoryPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "history read error:", err)
				return true
			}
			if len(recs) == 0 {
				fmt.Println("(history empty)")
				return true
			}
			if limit > len(recs) {
				limit = len(recs)
			}
			start := len(recs) - limit
			for i := start; i < len(recs); i++ {
				r := recs[i]
				idx := i + 1
				fmt.Printf("#%d %s | profile=%s | stream=%v | prompt=%s\n", idx, r.Time, r.Profile, r.Stream, truncateRunes(r.Prompt, 120))
				if r.ResponsePrev != "" {
					fmt.Printf("    ↳ %s\n", truncateRunes(r.ResponsePrev, 160))
				}
			}
			return true
		case "search":
			if len(args) < 2 {
				fmt.Println("usage: :history search <regex> [N]")
				return true
			}
			pattern := args[1]
			limit := 20
			if len(args) >= 3 {
				if n, err := strconv.Atoi(args[2]); err == nil && n > 0 {
					limit = n
				}
			}
			recs, err := history.ReadAll(cfg.HistoryPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "history read error:", err)
				return true
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid regex:", err)
				return true
			}
			matches := 0
			for i := len(recs) - 1; i >= 0; i-- {
				r := recs[i]
				if re.MatchString(r.Prompt) || re.MatchString(r.ResponsePrev) {
					idx := i + 1
					fmt.Printf("#%d %s | profile=%s | prompt=%s\n", idx, r.Time, r.Profile, truncateRunes(r.Prompt, 120))
					if r.ResponsePrev != "" {
						fmt.Printf("    ↳ %s\n", truncateRunes(r.ResponsePrev, 160))
					}
					matches++
					if matches >= limit {
						break
					}
				}
			}
			if matches == 0 {
				fmt.Println("(no matches)")
			}
			return true
		default:
			fmt.Println("usage: :history show [N] | :history search <regex> [N] | :history path")
			return true
		}
		ui.ResetScroll()
		os.Exit(0)
	}
	fmt.Println("unknown command. try :help")
	return true
}
