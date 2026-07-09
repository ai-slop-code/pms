package api

import (
	"testing"

	"pms/backend/internal/testutil"
)

func testPasswordHash(t *testing.T, plain string) string {
	t.Helper()
	return testutil.FastPasswordHash(t, plain)
}
