// Package totp wraps github.com/pquerna/otp for TOTP-based 2FA.
//
// Secrets are generated with the default SHA1/30s/6-digit parameters used
// by every mainstream authenticator app (Google Authenticator, 1Password,
// Authy, Bitwarden). The raw base32 secret is returned so the caller can
// encrypt it at rest via store.Crypto — this package never touches the DB.
//
// Recovery codes are independent of TOTP: they are random 10-character
// base32 strings, stored as SHA-256 hashes, consumed single-use to
// re-authenticate when the user loses access to their authenticator app.
package totp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// EnrolmentKey is what the caller needs to display the QR code and verify
// the first code before persisting the enrolment.
type EnrolmentKey struct {
	// Secret is the base32-encoded TOTP secret (no padding, no spaces).
	// Persist encrypted; show once to the user as a fallback.
	Secret string
	// OTPAuthURL is the `otpauth://totp/...` URI the authenticator app scans.
	OTPAuthURL string
}

// Generate produces a fresh TOTP secret for (issuer, accountEmail).
// The issuer appears as the label in the authenticator app.
func Generate(issuer, accountEmail string) (EnrolmentKey, error) {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = "PMS"
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountEmail,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return EnrolmentKey{}, err
	}
	return EnrolmentKey{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

// Verify returns true if the 6-digit code matches the secret at `at`.
// A ±1-period window (30s each side) is accepted to tolerate clock skew
// between server and authenticator device.
func Verify(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, at.UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes returns n plaintext codes (shown to the user once)
// and their SHA-256 hashes (persisted in user_recovery_codes.code_hash).
func GenerateRecoveryCodes(n int) (plain []string, hashes []string, err error) {
	if n <= 0 {
		return nil, nil, fmt.Errorf("totp: recovery code count must be positive")
	}
	plain = make([]string, n)
	hashes = make([]string, n)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	for i := 0; i < n; i++ {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, nil, err
		}
		raw := enc.EncodeToString(buf[:])[:10]
		// Format as two 5-character groups for readability: ABCDE-12345.
		code := strings.ToUpper(raw[:5] + "-" + raw[5:10])
		plain[i] = code
		hashes[i] = HashRecoveryCode(code)
	}
	return plain, hashes, nil
}

// HashRecoveryCode normalises (uppercase, strip whitespace and hyphens) and
// hashes with SHA-256. Normalisation lets users paste codes with or without
// the dash separator and in any case.
func HashRecoveryCode(code string) string {
	norm := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' {
			return -1
		}
		if r >= 'a' && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}, code)
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
