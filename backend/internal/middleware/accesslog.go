package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// sensitiveQueryKeys are query-string keys whose values must never end up in
// access logs (e.g. Booking/ICS tokens, OAuth/bearer-style secrets). Lowercase.
var sensitiveQueryKeys = map[string]struct{}{
	"token":        {},
	"access_token": {},
	"api_key":      {},
	"apikey":       {},
	"secret":       {},
	"password":     {},
}

// sensitiveHeaderKeys are request headers whose values must never end up in
// structured access logs (the access logger does not currently emit headers,
// but the redaction helper is shared with other logging sites).
var sensitiveHeaderKeys = map[string]struct{}{
	"authorization":  {},
	"cookie":         {},
	"x-export-token": {},
}

// RedactURLQuery returns a copy of rawQuery with values for sensitive keys
// replaced by "REDACTED". The original rawQuery is returned unchanged when it
// contains no sensitive keys, so no allocations happen in the hot path.
func RedactURLQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	if !containsSensitiveKey(rawQuery) {
		return rawQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "REDACTED"
	}
	for k, vs := range values {
		if _, ok := sensitiveQueryKeys[strings.ToLower(k)]; !ok {
			continue
		}
		for i := range vs {
			if vs[i] != "" {
				vs[i] = "REDACTED"
			}
		}
		values[k] = vs
	}
	return values.Encode()
}

// IsSensitiveHeader reports whether a header key should be redacted in logs.
func IsSensitiveHeader(key string) bool {
	_, ok := sensitiveHeaderKeys[strings.ToLower(key)]
	return ok
}

func containsSensitiveKey(rawQuery string) bool {
	lower := strings.ToLower(rawQuery)
	for k := range sensitiveQueryKeys {
		if strings.Contains(lower, k+"=") {
			return true
		}
	}
	return false
}

// statusRecorder is a minimal wrapper that captures the HTTP status code
// written by downstream handlers so the access logger can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// accessObserver is an optional hook called after each request. It is populated
// by the metrics package during server bootstrap so request counts and
// latencies can be tracked without creating a circular dependency between
// middleware and the metrics registry.
var accessObserver func(method string, status int, elapsed time.Duration)

// SetAccessObserver wires a callback that will be invoked once per request.
// Passing nil removes the hook.
func SetAccessObserver(fn func(method string, status int, elapsed time.Duration)) {
	accessObserver = fn
}

// AccessLog is a minimal request logger that mirrors chi's default format but
// never logs sensitive query-string values. It honours the PMS_ACCESS_LOG_FORMAT
// environment variable: when set to "json" it emits one JSON object per request
// (preferred for production log aggregators); otherwise it falls back to the
// legacy text format. It is a drop-in replacement for chi's
// `middleware.Logger`.
func AccessLog(next http.Handler) http.Handler {
	jsonMode := strings.EqualFold(strings.TrimSpace(os.Getenv("PMS_ACCESS_LOG_FORMAT")), "json")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)
		redactedQuery := RedactURLQuery(r.URL.RawQuery)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		if obs := accessObserver; obs != nil {
			obs(r.Method, status, elapsed)
		}
		if jsonMode {
			entry := map[string]any{
				"ts":          time.Now().UTC().Format(time.RFC3339Nano),
				"level":       "info",
				"event":       "http_request",
				"method":      r.Method,
				"path":        r.URL.Path,
				"query":       redactedQuery,
				"status":      status,
				"bytes":       rec.bytes,
				"duration_ms": elapsed.Milliseconds(),
				"remote":      r.RemoteAddr,
			}
			if rid := r.Header.Get("X-Request-Id"); rid != "" {
				entry["request_id"] = rid
			}
			buf, err := json.Marshal(entry)
			if err == nil {
				log.Println(string(buf))
				return
			}
			// fall through to text format on marshal failure
		}
		path := r.URL.Path
		if redactedQuery != "" {
			path = path + "?" + redactedQuery
		}
		log.Printf("%s %s %d %dB %s", r.Method, path, status, rec.bytes, elapsed)
	})
}
