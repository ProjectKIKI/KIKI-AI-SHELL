package app

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/shell"
	"kiki-ai-shell/internal/ui"
)

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v != "" {
		*m = append(*m, v)
	}
	return nil
}

func Main() {
	cfg := config.Default()
	uicfg := ui.DefaultConfig()

	var files multiFlag
	fHelp := flag.Bool("help", false, "show help")
	fProfile := flag.String("profile", "", "profile none|fast|deep")
	fStream := flag.Bool("stream", false, "stream output")
	fUIHeader := flag.Bool("ui-header", uicfg.ShowHeader, "show header in interactive shell")
	fUIClear := flag.Bool("ui-clear", uicfg.ClearOnDraw, "clear screen before drawing header")
	fUIMaxFiles := flag.Int("ui-maxfiles", uicfg.MaxFilesLine, "max width for files line (runes)")
	flag.Var(&files, "f", "attach file (repeatable)")
	flag.Parse()

	if *fHelp {
		shell.PrintHelp("")
		return
	}
	if strings.TrimSpace(*fProfile) != "" {
		cfg.Profile = *fProfile
	}
	if *fStream {
		cfg.Stream = true
	}
	uicfg.ShowHeader = *fUIHeader
	uicfg.ClearOnDraw = *fUIClear
	uicfg.MaxFilesLine = *fUIMaxFiles

	config.ApplyProfile(cfg)

	args := flag.Args()
	if len(args) > 0 {
		st := shell.NewState(cfg, uicfg)
		st.Files = append(st.Files, []string(files)...)
		if args[0] == "gen" {
			if len(args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: kiki-ai-shell gen <path> <prompt...>")
				os.Exit(1)
			}
			out := args[1]
			p := strings.TrimSpace(strings.Join(args[2:], " "))
			if p == "" {
				fmt.Fprintln(os.Stderr, "gen: prompt is empty")
				os.Exit(1)
			}
			if err := shell.RunGen(cfg, st, out, p); err != nil {
				fmt.Fprintln(os.Stderr, "gen error:", err)
				os.Exit(1)
			}
			return
		}
		if args[0] == "ask" {
			p := strings.TrimSpace(strings.Join(args[1:], " "))
			if p == "" {
				fmt.Fprintln(os.Stderr, "ask: prompt is empty")
				os.Exit(1)
			}
			shell.Ask(cfg, st, p, "")
			return
		}
		p := strings.TrimSpace(strings.Join(args, " "))
		if p == "" {
			fmt.Fprintln(os.Stderr, "prompt is empty")
			os.Exit(1)
		}
		shell.Ask(cfg, st, p, "")
		return
	}

	shell.RunREPL(cfg, uicfg)
}
