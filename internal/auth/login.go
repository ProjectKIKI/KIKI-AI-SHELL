package auth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
)

// LoginPAM is used by the shell at startup.
//
// Behavior:
// - If KIKI_LOGIN is not enabled, it simply returns the current OS user.
// - If KIKI_LOGIN=1, it prompts for username/password and validates them
//   using Authenticate(...). (When built with the `pam` build tag on Linux
//   with cgo + libpam, this is real PAM auth. Otherwise it's a permissive stub.)
func LoginPAM() (string, error) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("KIKI_LOGIN"))) != "1" {
		if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
			return u.Username, nil
		}
		return envUserFallback(), nil
	}

	r := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	uname, _ := r.ReadString('\n')
	uname = strings.TrimSpace(uname)
	if uname == "" {
		return "", errors.New("empty username")
	}

	pass, err := readPasswordNoEcho("Password: ")
	if err != nil {
		// fallback (echoed)
		line, _ := r.ReadString('\n')
		pass = strings.TrimSpace(line)
	} else {
		fmt.Println()
	}

	if err := Authenticate(uname, pass); err != nil {
		return "", err
	}
	return uname, nil
}

func envUserFallback() string {
	if v := strings.TrimSpace(os.Getenv("USER")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("USERNAME")); v != "" {
		return v
	}
	return "unknown"
}
