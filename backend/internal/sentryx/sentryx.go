// Package sentryx wraps github.com/getsentry/sentry-go behind a feature flag.
//
// The Sentry SDK is only initialised when SENTRY_DSN is set. When disabled,
// all helpers in this package are safe no-ops so call sites don't need
// nil checks. Sensitive headers (Authorization, Cookie, X-Export-Token,
// X-PMS-Client) are scrubbed via BeforeSend before any event leaves the
// process.
package sentryx

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

var enabled bool

// Init initialises Sentry when SENTRY_DSN is set. env and release propagate
// to Sentry so events can be filtered in the UI. Call once at startup.
func Init(env, release string) error {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		return nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          release,
		AttachStacktrace: true,
		// Conservative sample rate by default. Can be overridden through
		// SENTRY_TRACES_SAMPLE_RATE, but traces aren't emitted unless the
		// integration is wired explicitly.
		TracesSampleRate: 0.0,
		BeforeSend:       scrub,
	})
	if err != nil {
		return err
	}
	enabled = true
	return nil
}

// Flush drains pending events before process exit.
func Flush() {
	if enabled {
		sentry.Flush(2 * time.Second)
	}
}

// CaptureException forwards err to Sentry when configured.
func CaptureException(err error) {
	if !enabled || err == nil {
		return
	}
	sentry.CaptureException(err)
}

// Recoverer returns an HTTP middleware that converts panics into Sentry
// events. It should be placed AFTER chimw.Recoverer so chi's default
// response (500) is still produced; this wrapper only records.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				if enabled {
					sentry.CurrentHub().Recover(rv)
					sentry.Flush(500 * time.Millisecond)
				}
				panic(rv) // re-raise so chimw.Recoverer handles the 500
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// sensitiveHeaders lists headers scrubbed from every outgoing event.
var sensitiveHeaders = []string{
	"Authorization",
	"Cookie",
	"Set-Cookie",
	"X-Export-Token",
	"X-Pms-Client",
	"X-Csrf-Token",
}

// scrub strips sensitive request/response headers before the event leaves
// the process. Runs on the Sentry pipeline so any non-standard capture path
// also benefits.
func scrub(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return event
	}
	if event.Request != nil && event.Request.Headers != nil {
		for _, h := range sensitiveHeaders {
			if _, ok := event.Request.Headers[h]; ok {
				event.Request.Headers[h] = "[redacted]"
			}
		}
		// Strip any query string values that could contain tokens.
		if event.Request.QueryString != "" && strings.Contains(strings.ToLower(event.Request.QueryString), "token=") {
			event.Request.QueryString = "[redacted]"
		}
	}
	return event
}
