package shell

import (
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// completeLine returns candidates for TAB completion.
// We keep it simple: internal commands (:...), help topics, and cd path.
func completeLine(line string) []string {
    s := strings.TrimSpace(line)
    if s == "" {
        return nil
    }

    // cd path completion
    if strings.HasPrefix(s, "cd ") {
        p := strings.TrimSpace(strings.TrimPrefix(s, "cd "))
        if p == "" {
            p = "./"
        }
        // expand ~
        if strings.HasPrefix(p, "~") {
            home, _ := os.UserHomeDir()
            p = filepath.Join(home, strings.TrimPrefix(p, "~"))
        }
        glob := p + "*"
        matches, _ := filepath.Glob(glob)
        out := make([]string, 0, len(matches))
        for _, m := range matches {
            st, err := os.Stat(m)
            if err == nil && st.IsDir() {
                out = append(out, "cd "+m)
            }
        }
        sort.Strings(out)
        return out
    }

    // internal commands
    if strings.HasPrefix(s, ":") {
        // tokenization
        parts := strings.Fields(strings.TrimPrefix(s, ":"))
        if len(parts) == 0 {
            return prefixMatches(s, []string{":help", ":profile", ":stream", ":ui", ":file", ":ctx", ":ctx-size", ":llm", ":gen", ":bash", ":exit", ":quit"})
        }
        cmd := strings.ToLower(parts[0])
        // completing the command itself
        if len(parts) == 1 && !strings.HasSuffix(s, " ") {
            return prefixMatches(":"+parts[0], []string{":help", ":profile", ":stream", ":ui", ":file", ":ctx", ":ctx-size", ":llm", ":gen", ":bash", ":exit", ":quit"})
        }

        // completing subcommands/args
        switch cmd {
        case "help":
            topics := []string{"shell", "llm", "file", "ctx", "ui", "env", "gen", "llmset"}
            return completeSecondToken(s, ":help", topics)
        case "profile":
            vals := []string{"none", "fast", "deep"}
            return completeSecondToken(s, ":profile", vals)
        case "stream":
            vals := []string{"on", "off"}
            return completeSecondToken(s, ":stream", vals)
        case "ui":
            vals := []string{"header", "clear", "fence"}
            return completeSecondToken(s, ":ui", vals)
        case "file":
            vals := []string{"add", "list", "rm", "clear"}
            return completeSecondToken(s, ":file", vals)
        case "ctx":
            vals := []string{"set", "show", "clear"}
            return completeSecondToken(s, ":ctx", vals)
        case "llm":
            vals := []string{"set", "show", "clear"}
            return completeSecondToken(s, ":llm", vals)
        case "gen":
            vals := []string{"sh", "yaml", "ansible", "tf", "k8s"}
            return completeSecondToken(s, ":gen", vals)
        }
        return nil
    }

    return nil
}

func prefixMatches(prefix string, items []string) []string {
    out := make([]string, 0, len(items))
    for _, it := range items {
        if strings.HasPrefix(strings.ToLower(it), strings.ToLower(prefix)) {
            out = append(out, it)
        }
    }
    sort.Strings(out)
    return out
}

func completeSecondToken(fullLine, base string, options []string) []string {
    // fullLine like ":cmd ..."
    // We only complete the *next* token after base.
    s := strings.TrimSpace(fullLine)
    if !strings.HasPrefix(s, base) {
        return nil
    }
    rest := strings.TrimSpace(strings.TrimPrefix(s, base))
    if rest == "" {
        out := make([]string, 0, len(options))
        for _, o := range options {
            out = append(out, base+" "+o)
        }
        return out
    }
    // already has something typed
    parts := strings.Fields(rest)
    if len(parts) == 0 {
        return nil
    }
    if len(parts) == 1 && !strings.HasSuffix(s, " ") {
        pref := parts[0]
        out := []string{}
        for _, o := range options {
            if strings.HasPrefix(strings.ToLower(o), strings.ToLower(pref)) {
                out = append(out, base+" "+o)
            }
        }
        sort.Strings(out)
        return out
    }
    return nil
}
