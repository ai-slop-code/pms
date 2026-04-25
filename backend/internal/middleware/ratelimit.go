package middleware

import (
	"sync"

	"golang.org/x/time/rate"
)

// KeyedLimiter provides per-key leaky-bucket rate limiting. Stale entries are
// kept in memory; acceptable for the narrow surfaces this is applied to
// (login, admin backup). For high-cardinality keys, swap to an LRU.
type KeyedLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

// NewKeyedLimiter returns a limiter that allows r events per second with a
// burst of b per distinct key.
func NewKeyedLimiter(r rate.Limit, b int) *KeyedLimiter {
	return &KeyedLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

// Allow returns true if a single event is currently permitted for key.
func (k *KeyedLimiter) Allow(key string) bool {
	k.mu.Lock()
	lim, ok := k.limiters[key]
	if !ok {
		lim = rate.NewLimiter(k.r, k.b)
		k.limiters[key] = lim
	}
	k.mu.Unlock()
	return lim.Allow()
}
