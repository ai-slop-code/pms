package testutil

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func FastPasswordHash(t *testing.T, plain string) string {
	t.Helper()
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
