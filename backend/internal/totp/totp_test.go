package totp

import (
	"strings"
	"testing"
	"time"

	pquernaotp "github.com/pquerna/otp/totp"
)

func TestGenerateProducesUsableSecret(t *testing.T) {
	key, err := Generate("PMS-Test", "alice@example.com")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if key.Secret == "" {
		t.Fatal("empty secret")
	}
	if !strings.HasPrefix(key.OTPAuthURL, "otpauth://totp/") {
		t.Fatalf("unexpected otpauth URL: %q", key.OTPAuthURL)
	}
	if !strings.Contains(key.OTPAuthURL, "issuer=PMS-Test") {
		t.Fatalf("missing issuer in url: %q", key.OTPAuthURL)
	}
	// A code generated against the secret must verify.
	now := time.Now().UTC()
	code, err := pquernaotp.GenerateCode(key.Secret, now)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !Verify(key.Secret, code, now) {
		t.Fatal("Verify rejected a freshly generated code")
	}
}

func TestVerifyRejectsBadCode(t *testing.T) {
	key, err := Generate("", "bob@example.com")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if Verify(key.Secret, "000000", time.Now()) {
		t.Fatal("Verify accepted 000000")
	}
	if Verify(key.Secret, "", time.Now()) {
		t.Fatal("Verify accepted empty code")
	}
	if Verify(key.Secret, "12345", time.Now()) {
		t.Fatal("Verify accepted 5-digit code")
	}
}

func TestVerifyAcceptsSkew(t *testing.T) {
	key, err := Generate("PMS", "carol@example.com")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Code generated 30s in the past must still validate with skew=1.
	past := time.Now().UTC().Add(-30 * time.Second)
	code, err := pquernaotp.GenerateCode(key.Secret, past)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !Verify(key.Secret, code, time.Now().UTC()) {
		t.Fatal("Verify rejected code from -30s window")
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	plain, hashes, err := GenerateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(plain) != 10 || len(hashes) != 10 {
		t.Fatalf("len: plain=%d hashes=%d", len(plain), len(hashes))
	}
	seen := map[string]bool{}
	for i, code := range plain {
		if len(code) != 11 || code[5] != '-' {
			t.Fatalf("code %d: unexpected shape %q", i, code)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true
		if HashRecoveryCode(code) != hashes[i] {
			t.Fatalf("hash mismatch for %q", code)
		}
	}
}

func TestHashRecoveryCodeNormalisation(t *testing.T) {
	// Dashes, whitespace and case must not affect the hash — users may
	// paste the code in any form their authenticator app copies it.
	code := "ABCDE-12345"
	variants := []string{
		"ABCDE-12345",
		"abcde-12345",
		"ABCDE12345",
		"abcde 12345",
		"  ABCDE-12345  ",
		"abcde\t12345",
	}
	want := HashRecoveryCode(code)
	for _, v := range variants {
		if HashRecoveryCode(v) != want {
			t.Fatalf("hash mismatch for variant %q", v)
		}
	}
}

func TestGenerateRecoveryCodesZero(t *testing.T) {
	if _, _, err := GenerateRecoveryCodes(0); err == nil {
		t.Fatal("expected error for n=0")
	}
}
