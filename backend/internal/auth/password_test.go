package auth

import "testing"

func TestValidatePassword_RejectsShort(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("expected error for short password")
	}
	if err := ValidatePassword("exactly11ch"); err == nil {
		t.Fatal("expected error for 11-char password")
	}
}

func TestValidatePassword_RejectsCommon(t *testing.T) {
	// Entries must be ≥12 chars (min-length check runs first) AND match the
	// deny list verbatim (case-insensitive) to be rejected.
	for _, p := range []string{"correcthorse", "CORRECTHORSEBATTERY", "administrator", "123456789012"} {
		if err := ValidatePassword(p); err == nil {
			t.Fatalf("expected error for common password %q", p)
		}
	}
}

func TestValidatePassword_AcceptsStrong(t *testing.T) {
	for _, p := range []string{"correct horse battery staple", "Zx7!kQw#p2Lm", "this-is-a-long-enough-phrase"} {
		if err := ValidatePassword(p); err != nil {
			t.Fatalf("valid password %q rejected: %v", p, err)
		}
	}
}
