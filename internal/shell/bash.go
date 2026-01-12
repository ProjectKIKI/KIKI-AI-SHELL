package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func runInteractiveBash() error {
	cmd := exec.Command("/bin/bash")
	cmd.Env = append(os.Environ(), "KIKI_INNER_BASH=1")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

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
