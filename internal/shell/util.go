package shell

import (
	"regexp"
	"strconv"
	"strings"
)

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
