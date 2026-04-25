package middleware

import (
	"net/http"
	"strings"
)

// ClientHeaderName is the custom request header expected on every
// state-changing API call. Browsers cannot set arbitrary headers on simple
// cross-origin form submissions (plain form / multipart / text/plain), so
// requiring a custom header is a robust CSRF shield for the cookie-based
// session auth used by the PMS API.
const ClientHeaderName = "X-PMS-Client"

// RequireClientHeader rejects state-changing requests that do not carry the
// custom X-PMS-Client header. Safe methods (GET/HEAD/OPTIONS) pass through
// unconditionally so browser navigations and preflights continue to work.
//
// This is the back-compat shim for tests and call sites that don't want
// origin enforcement. Production code should call CSRFGuard with the same
// allowlist used by the CORS middleware.
func RequireClientHeader(next http.Handler) http.Handler {
	return CSRFGuard(nil)(next)
}

// CSRFGuard combines two cheap, browser-enforced CSRF defences:
//
//  1. Custom-header trick — requires X-PMS-Client on state-changing requests.
//     Simple cross-origin form submissions cannot set arbitrary headers.
//  2. Origin allowlist — if the request carries an Origin header and that
//     origin is not in `allowedOrigins`, reject. Browsers always send Origin
//     on state-changing requests; same-origin SPAs send their own origin
//     which must already be in the CORS allowlist for the response to be
//     readable. Mismatches indicate either a misconfigured deployment or an
//     attempted cross-origin attack.
//
// `allowedOrigins == nil` (or empty) skips the Origin check, preserving the
// pre-existing behaviour. Pass the same slice configured for the CORS
// middleware so the two stay in lock-step.
func CSRFGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o != "" && o != "*" {
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get(ClientHeaderName) == "" {
				writeForbidden(w, "missing client header")
				return
			}
			// Origin check: only enforce when the client actually sent one
			// and we have an allowlist configured. Server-to-server callers
			// (curl, integration tests) often omit Origin entirely; we
			// already gate them via X-PMS-Client above.
			if origin := r.Header.Get("Origin"); origin != "" && len(allowed) > 0 {
				if _, ok := allowed[origin]; !ok {
					writeForbidden(w, "origin not allowed")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeForbidden(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"` + reason + `"}`))
}
