package ctxuser

import (
	"context"

	"pms/backend/internal/store"
)

type key int

const (
	userKey       key = 1
	mfaPendingKey key = 2
)

func WithUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func From(ctx context.Context) *store.User {
	u, _ := ctx.Value(userKey).(*store.User)
	return u
}

// WithMFAPending marks the request as belonging to a session whose owner
// has enrolled in TOTP but has not yet completed the second-factor
// challenge for this session. Handlers that accept only the verify / logout
// endpoints consult this flag; most protected handlers reject pending
// sessions outright via middleware.
func WithMFAPending(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, mfaPendingKey, u)
}

// MFAPending returns the user attached to a pending-MFA context, or nil.
func MFAPending(ctx context.Context) *store.User {
	u, _ := ctx.Value(mfaPendingKey).(*store.User)
	return u
}
