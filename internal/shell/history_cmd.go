package shell

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"kiki-ai-shell/internal/config"
	"kiki-ai-shell/internal/history"
)

func handleHistory(cfg *config.Config, st *State, args []string) bool {
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
		re, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid regex:", err)
			return true
		}
		recs, err := history.ReadAll(cfg.HistoryPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "history read error:", err)
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
}
