package middleware

import "net/http"

// SecurityHeaders sets conservative defaults on every HTTP response. These
// protect the JSON API and the static SPA shell from a handful of common
// browser-side attacks (clickjacking, MIME sniffing, referrer leakage, and
// permissions abuse). HSTS is emitted only when the request is already TLS
// (or arrived via a trusted reverse proxy that set `X-Forwarded-Proto`), so
// local HTTP development is unaffected.
//
// Mount this BEFORE `chimw.Recoverer`: if a downstream panics before headers
// are written, the Recoverer still serves a 500 but our headers travel with
// the response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		// The JSON API serves no HTML, so a very tight CSP is safe here.
		// The SPA shell ships its own, looser CSP via a <meta> tag.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
