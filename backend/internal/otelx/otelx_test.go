package otelx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Disabled-mode helpers are the common path in tests and on operators who
// never set OTEL_EXPORTER_OTLP_ENDPOINT. Make sure they stay pure no-ops
// rather than panicking on nil tracer providers.

func TestDisabledByDefault(t *testing.T) {
	if Enabled() {
		t.Fatalf("otelx should not be enabled without OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	// Middleware must pass through requests unchanged.
	var called bool
	h := Middleware("test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if !called {
		t.Fatalf("handler not invoked through middleware")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}

	// HTTPTransport returns base unchanged.
	if got := HTTPTransport(http.DefaultTransport); got != http.DefaultTransport {
		t.Fatalf("HTTPTransport wrapped DefaultTransport when tracing disabled")
	}

	// StartSpan is a no-op. end(nil) and end(err) must not panic.
	_, end := StartSpan(context.Background(), "noop-span")
	end(nil)
	_, end2 := StartSpan(context.Background(), "noop-span-err")
	end2(http.ErrServerClosed)
}
