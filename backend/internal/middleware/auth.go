package middleware

import (
	"net/http"

	"pms/backend/internal/auth"
	"pms/backend/internal/ctxuser"
	"pms/backend/internal/store"
)

const SessionCookieName = "pms_session"

func Auth(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookieName)
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			hash := auth.HashSessionToken(c.Value)
			sess, err := st.LookupSessionByHash(r.Context(), hash)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			u, err := st.GetUserByID(r.Context(), sess.UserID)
			if err != nil || !u.Active {
				next.ServeHTTP(w, r)
				return
			}
			// A session that has not yet cleared its 2FA challenge is
			// attached to a separate context key; only the
			// /auth/2fa/verify and /auth/logout handlers consult that
			// key. Normal protected routes keep using ctxuser.From() and
			// therefore reject the request via RequireAuthJSON.
			ctx := r.Context()
			if sess.MFAVerified {
				ctx = ctxuser.WithUser(ctx, u)
			} else {
				ctx = ctxuser.WithMFAPending(ctx, u)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuthJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ctxuser.From(r.Context()) == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
