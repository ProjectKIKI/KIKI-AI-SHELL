package shell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kiki-ai-shell/internal/auth"
	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/ui"
	"kiki-ai-shell/internal/usage"
)

func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func renderHeader(cfg *config.Config, st *State, uicfg ui.Config) {
	if !uicfg.ShowHeader {
		return
	}

	llm := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if strings.TrimSpace(cfg.BaseURL) != "" {
		llm = cfg.BaseURL
	}

	// ui.RenderHeader will format stream/files; we pass raw values.

	cluster := ctxGet(st, "cluster")
	ns := ctxGet(st, "ns")

	h := ui.HeaderData{
		Title:       " KIKI AI SHELL ",
		User:        st.User,
		LLM:         llm,
		Profile:     st.Profile,
		Stream:      st.Stream,
		Files:       st.Files,
		NoFence:     st.NoFence,
		CtxObserved: st.CtxSizeObserved,
		CtxTarget:   st.CtxSizeTarget,
		Cluster:     cluster,
		Namespace:   ns,
		PCP:         st.PCP.Display(),
	}
	ui.RenderHeader(uicfg, h)
}

// RunREPL starts interactive shell mode.
func RunREPL(cfg *config.Config, uicfg ui.Config) {
	st := NewState(cfg, uicfg)

	// --- auth (PAM) ---
	if cfg.AuthPAM {
		user, err := auth.LoginPAM()
		if err != nil {
			fmt.Fprintln(os.Stderr, "login failed:", err)
			return
		}
		st.User = user
		st.Usage = usage.New(cfg.UsageBaseDir, user)

		// preload recent usage into RAG
		recs, _ := usage.LoadRecent(st.Usage.Path, cfg.UsageLoadDays, cfg.UsageLoadMax)
		for _, r := range recs {
			text := fmt.Sprintf("[%s] %s | %s", r.Type, r.Cwd, firstNonEmpty(r.Command, r.Prompt))
			_ = st.RAG.AddText("usage:"+r.Time, text, 4000)
		}
	}

	for {
		renderHeader(cfg, st, uicfg)
		line, err := ui.ReadLineRaw(promptLine(st), completeLine)
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// internal commands
		if strings.HasPrefix(line, ":") {
			handleInternalCommand(cfg, st, uicfg, line)
			continue
		}

		// LLM ask
		if strings.HasPrefix(line, "?") {
			q := strings.TrimSpace(strings.TrimPrefix(line, "?"))
			if q != "" {
				Ask(cfg, st, q, "")
			}
			continue
		}

		// gen <path> <prompt...>  (generate code only and optionally save)
		if strings.HasPrefix(line, "gen ") {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				fmt.Println("usage: gen <output_path> <prompt...>")
				continue
			}
			outPath := parts[1]
			prompt := strings.TrimSpace(strings.TrimPrefix(line, "gen "+outPath))
			if err := Gen(cfg, st, outPath, prompt); err != nil {
				fmt.Fprintln(os.Stderr, "gen error:", err)
			}
			continue
		}

		// cd
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

		// bash once + usage log
		exitCode := runBashOnce(line)
		if st.Usage != nil {
			now := time.Now().Format(time.RFC3339)
			cwd, _ := os.Getwd()
			st.Usage.Append(usage.Record{Time: now, User: st.User, Type: "cmd", Cwd: cwd, Command: line})
			_ = st.RAG.AddText("usage:"+now+":cmd", "[cmd] "+cwd+" $ "+line+" (exit="+strconv.Itoa(exitCode)+")", 8000)
		}
	}
}

func handleInternalCommand(cfg *config.Config, st *State, uicfg ui.Config, line string) {
	cmdline := strings.TrimSpace(strings.TrimPrefix(line, ":"))
	if cmdline == "" {
		return
	}
	parts := strings.Fields(cmdline)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help":
		if len(args) > 0 {
			PrintHelp(args[0])
		} else {
			PrintHelp("")
		}
		return

	case "llm":
		// :llm show | :llm set <base_url> | :llm clear
		if len(args) < 1 {
			fmt.Println("usage: :llm show | :llm set <base_url> | :llm clear")
			return
		}
		sub := strings.ToLower(strings.TrimSpace(args[0]))
		switch sub {
		case "show":
			if strings.TrimSpace(cfg.BaseURL) == "" {
				fmt.Printf("LLM_BASE_URL: (empty) -> using http://%s:%d\n", cfg.Host, cfg.Port)
			} else {
				fmt.Printf("LLM_BASE_URL: %s\n", cfg.BaseURL)
			}
			return
		case "clear":
			cfg.BaseURL = ""
			fmt.Println("LLM_BASE_URL cleared (fallback to host:port)")
			if uicfg.FixedHeader {
				renderHeader(cfg, st, uicfg)
			}
			return
		case "set":
			if len(args) < 2 {
				fmt.Println("usage: :llm set <base_url>")
				return
			}
			url := strings.TrimSpace(args[1])
			// allow: :llm set http://host:port  OR :llm set host:port
			if url != "" && !strings.Contains(url, "://") {
				url = "http://" + url
			}
			cfg.BaseURL = strings.TrimRight(url, "/")
			fmt.Println("LLM_BASE_URL set:", cfg.BaseURL)
			if uicfg.FixedHeader {
				renderHeader(cfg, st, uicfg)
			}
			return
		default:
			// shorthand: :llm http://...
			url := strings.TrimSpace(args[0])
			if url != "" && (strings.Contains(url, "://") || strings.Contains(url, ":")) {
				if !strings.Contains(url, "://") {
					url = "http://" + url
				}
				cfg.BaseURL = strings.TrimRight(url, "/")
				fmt.Println("LLM_BASE_URL set:", cfg.BaseURL)
				if uicfg.FixedHeader {
					renderHeader(cfg, st, uicfg)
				}
				return
			}
			fmt.Println("usage: :llm show | :llm set <base_url> | :llm clear")
			return
		}

	case "gen":
		// :gen <path> <prompt...>
		if len(args) < 2 {
			fmt.Println("usage: :gen <path> <prompt...>")
			return
		}
		out := args[0]
		p := strings.TrimSpace(strings.Join(args[1:], " "))
		if err := Gen(cfg, st, out, p); err != nil {
			fmt.Fprintln(os.Stderr, "gen error:", err)
		}
		if uicfg.FixedHeader {
			renderHeader(cfg, st, uicfg)
		}
		return

	case "pcp":
		// :pcp show | :pcp host <host|local> | :pcp cpu | :pcp mem | :pcp load | :pcp raw <metric...>
		if len(args) < 1 {
			fmt.Println("usage: :pcp show | :pcp host <host|local> | :pcp cpu|mem|load | :pcp raw <metric...>")
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "show":
			fmt.Println(st.PCP.Show())
			return
		case "host":
			if len(args) < 2 {
				fmt.Println("usage: :pcp host <host|local>")
				return
			}
			st.PCP.SetHost(args[1])
			fmt.Println("pcp host set:", st.PCP.HostLabel())
			if uicfg.FixedHeader {
				renderHeader(cfg, st, uicfg)
			}
			return
		case "cpu":
			out, err := st.PCP.CPUOnce()
			if err != nil {
				fmt.Fprintln(os.Stderr, "pcp error:", err)
				return
			}
			fmt.Println(out)
			return
		case "mem":
			out, err := st.PCP.MemOnce()
			if err != nil {
				fmt.Fprintln(os.Stderr, "pcp error:", err)
				return
			}
			fmt.Println(out)
			return
		case "load":
			out, err := st.PCP.LoadOnce()
			if err != nil {
				fmt.Fprintln(os.Stderr, "pcp error:", err)
				return
			}
			fmt.Println(out)
			return
		case "raw":
			if len(args) < 2 {
				fmt.Println("usage: :pcp raw <metric...>")
				return
			}
			out, err := st.PCP.RawOnce(args[1:])
			if err != nil {
				fmt.Fprintln(os.Stderr, "pcp error:", err)
				return
			}
			fmt.Println(out)
			return
		default:
			// shorthand: :pcp <host>
			if len(args) == 1 {
				st.PCP.SetHost(args[0])
				fmt.Println("pcp host set:", st.PCP.HostLabel())
				if uicfg.FixedHeader {
					renderHeader(cfg, st, uicfg)
				}
				return
			}
			fmt.Println("usage: :pcp show | :pcp host <host|local> | :pcp cpu|mem|load | :pcp raw <metric...>")
			return
		}

	case "bash":
		fmt.Println("\n[Entering interactive bash] (type 'exit' to return)\n")
		if err := runInteractiveBash(); err != nil {
			fmt.Fprintln(os.Stderr, "pty bash error:", err)
		}
		fmt.Println("\n[Back to KIKI]\n")
		if uicfg.FixedHeader {
			renderHeader(cfg, st, uicfg)
		}
		return

	case "profile":
		if len(args) < 1 {
			fmt.Println("usage: :profile fast|deep|none")
			return
		}
		st.Profile = args[0]
		cfg.Profile = st.Profile
		config.ApplyProfile(cfg)
		return

	case "stream":
		if len(args) < 1 {
			fmt.Println("usage: :stream on|off")
			return
		}
		v := strings.ToLower(args[0])
		st.Stream = (v == "on" || v == "1" || v == "true")
		cfg.Stream = st.Stream
		return

	case "nofence":
		if len(args) < 1 {
			state := "off"
			if st.NoFence {
				state = "on"
			}
			fmt.Println("nofence:", state)
			fmt.Println("usage: :nofence on|off")
			return
		}
		v := strings.ToLower(args[0])
		on := (v == "on" || v == "1" || v == "true")
		st.NoFence = on
		cfg.NoFence = on
		return

	case "ui":
		if len(args) < 2 {
			fmt.Println("usage: :ui header on|off | :ui clear on|off")
			return
		}
		sub := strings.ToLower(args[0])
		val := strings.ToLower(args[1])
		on := (val == "on" || val == "1" || val == "true")
		switch sub {
		case "header":
			uicfg.ShowHeader = on
			st.UI.ShowHeader = on
		case "clear":
			uicfg.ClearOnDraw = on
			st.UI.ClearOnDraw = on
		default:
			fmt.Println("usage: :ui header on|off | :ui clear on|off")
		}
		return

	case "ctx":
		if len(args) < 1 {
			fmt.Println("usage: :ctx set key=value | :ctx show | :ctx clear")
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "set":
			if len(args) < 2 {
				fmt.Println("usage: :ctx set key=value")
				return
			}
			kv := strings.Join(args[1:], " ")
			parts2 := strings.SplitN(kv, "=", 2)
			if len(parts2) != 2 {
				fmt.Println("usage: :ctx set key=value")
				return
			}
			k := strings.TrimSpace(parts2[0])
			v := strings.TrimSpace(parts2[1])
			ctxSet(st, k, v)
			fmt.Printf("ctx set: %s=%s\n", k, v)
			return
		case "show":
			if len(st.Ctx) == 0 {
				fmt.Println("(ctx empty)")
				return
			}
			keys := make([]string, 0, len(st.Ctx))
			for k := range st.Ctx {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%s=%s\n", k, st.Ctx[k])
			}
			return
		case "clear":
			ctxClear(st)
			fmt.Println("ctx cleared")
			return
		default:
			fmt.Println("usage: :ctx set key=value | :ctx show | :ctx clear")
			return
		}

	case "ctx-size", "ctxsize":
		if len(args) < 1 {
			fmt.Printf("ctx-size: observed=%d target=%d\n", st.CtxSizeObserved, st.CtxSizeTarget)
			fmt.Println("usage: :ctx-size <N>")
			return
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 256 {
			fmt.Println("invalid ctx-size:", args[0])
			return
		}
		st.CtxSizeTarget = n
		cfg.CtxSizeTarget = n
		fmt.Println("ctx-size target set:", n)
		fmt.Println("note: actual ctx-size requires llama.cpp server restart with --ctx-size", n)
		return

	case "rag":
		if len(args) < 1 {
			fmt.Printf("rag: enabled=%v docs=%d\n", st.RAG.Enabled, len(st.RAG.Docs))
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "on":
			st.RAG.Enabled = true
			fmt.Println("rag enabled")
		case "off":
			st.RAG.Enabled = false
			fmt.Println("rag disabled")
		case "clear":
			st.RAG.Docs = nil
			fmt.Println("rag cleared")
		case "stats":
			fmt.Printf("rag docs=%d\n", len(st.RAG.Docs))
		default:
			fmt.Println("usage: :rag on|off | :rag clear | :rag stats")
		}
		return

	case "file":
		if len(args) < 1 {
			fmt.Println("usage: :file add /path | :file list | :file rm N | :file clear")
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "add":
			if len(args) < 2 {
				fmt.Println("usage: :file add /path")
				return
			}
			p := normalizePath(strings.Join(args[1:], " "))
			if !fileExists(p) {
				fmt.Println("file not found:", p)
				return
			}
			st.Files = append(st.Files, p)
			fmt.Println("file added:", p)
		case "list":
			if len(st.Files) == 0 {
				fmt.Println("(no attached files)")
				return
			}
			for i, f := range st.Files {
				fmt.Printf("%d) %s\n", i+1, f)
			}
		case "rm":
			if len(args) < 2 {
				fmt.Println("usage: :file rm N")
				return
			}
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 || n > len(st.Files) {
				fmt.Println("invalid index")
				return
			}
			idx := n - 1
			removed := st.Files[idx]
			st.Files = append(st.Files[:idx], st.Files[idx+1:]...)
			fmt.Println("file removed:", removed)
		case "clear":
			st.Files = nil
			fmt.Println("files cleared")
		default:
			fmt.Println("usage: :file add /path | :file list | :file rm N | :file clear")
		}
		return

	case "history":
		handleHistory(cfg, st, args)
		return

	case "usage":
		if st.Usage == nil {
			fmt.Println("usage: (not enabled)")
			return
		}
		days := cfg.UsageLoadDays
		if len(args) >= 1 {
			if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
				days = n
			}
		}
		recs, err := usage.LoadRecent(st.Usage.Path, days, cfg.UsageLoadMax)
		if err != nil {
			fmt.Fprintln(os.Stderr, "usage load error:", err)
			return
		}
		fmt.Println(usage.Summary(recs))
		return

	case "exit", "quit":
		ui.ResetScroll()
		os.Exit(0)
	}
	fmt.Println("unknown command. try :help")
}
