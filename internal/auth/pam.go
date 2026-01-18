//go:build pam
// +build pam

package auth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
)

// LoginPAM prompts for username/password when KIKI_LOGIN=1 and authenticates via PAM.
//
// Enable with: go build -tags pam
// (Linux + CGO required. Ubuntu/Debian: sudo apt-get install -y libpam0g-dev)
func LoginPAM() (string, error) {
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("KIKI_LOGIN")))
	if enabled == "" || enabled == "0" || enabled == "false" || enabled == "off" || enabled == "no" {
		u, err := user.Current()
		if err != nil {
			return "", nil
		}
		return u.Username, nil
	}

	r := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	userIn, _ := r.ReadString('\n')
	username := strings.TrimSpace(userIn)
	if username == "" {
		return "", errors.New("empty username")
	}

	pw, err := readPasswordNoEcho("Password: ")
	if err != nil {
		return "", err
	}

	service := strings.TrimSpace(os.Getenv("KIKI_PAM_SERVICE"))
	if service == "" {
		service = "login"
	}

	if err := Authenticate(username, pw); err != nil {
		return "", fmt.Errorf("PAM auth failed: %w", err)
	}
	return username, nil
}
