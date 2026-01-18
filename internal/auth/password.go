package auth

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// readPasswordNoEcho reads a line from stdin with terminal echo disabled.
// It uses `stty` to avoid external Go deps (x/term). If stty fails, caller can fall back.
func readPasswordNoEcho(prompt string) (string, error) {
	if prompt != "" {
		fmt.Fprint(os.Stdout, prompt)
	}
	// best-effort no-echo
	_ = exec.Command("stty", "-echo").Run()
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	_ = exec.Command("stty", "echo").Run()
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
