package middleware

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestKeyedLimiter_BurstThenDeny(t *testing.T) {
	l := NewKeyedLimiter(rate.Every(time.Hour), 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("ip1") {
			t.Fatalf("iteration %d should allow", i)
		}
	}
	if l.Allow("ip1") {
		t.Fatal("4th call should deny")
	}
	// Different key has its own bucket.
	if !l.Allow("ip2") {
		t.Fatal("ip2 should allow")
	}
}
