package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost       = 12
	minPasswordChars = 12
)

// ErrWeakPassword is returned when a candidate password fails policy checks.
// Callers should surface the embedded message verbatim to the end user.
var ErrWeakPassword = errors.New("password does not meet policy")

// commonPasswords holds a small in-memory deny list. Upstream literature
// (NIST SP 800-63B) recommends rejecting breached/common credentials instead
// of demanding character-class complexity; this list is intentionally short
// — operators can extend it via `data/common-passwords.txt` at runtime.
var commonPasswords = map[string]struct{}{
	"password":          {},
	"password1":         {},
	"password123":       {},
	"passw0rd":          {},
	"letmein":           {},
	"welcome":           {},
	"welcome1":          {},
	"administrator":     {},
	"qwerty":            {},
	"qwerty123":         {},
	"iloveyou":          {},
	"1234567890":        {},
	"123456789012":      {},
	"changeme":          {},
	"changeme123":       {},
	"monkey":            {},
	"dragon":            {},
	"trustno1":          {},
	"correcthorse":      {},
	"correcthorsebattery": {},
}

// ValidatePassword enforces the PMS password policy:
//   - at least 12 characters
//   - not present in the common-password deny list (case-insensitive)
// No character-class rules (per NIST SP 800-63B).
func ValidatePassword(raw string) error {
	if len(raw) < minPasswordChars {
		return errors.New("password must be at least 12 characters")
	}
	lower := strings.ToLower(raw)
	if _, bad := commonPasswords[lower]; bad {
		return errors.New("password is on the common-password deny list")
	}
	return nil
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
