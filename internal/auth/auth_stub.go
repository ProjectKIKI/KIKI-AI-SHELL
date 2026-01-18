//go:build !pam

package auth

// Authenticate validates username/password.
//
// Stub implementation (default build): always returns nil.
// Build with `-tags pam` (linux+cgo+libpam) to enable real PAM authentication.
func Authenticate(username, password string) error {
	return nil
}
