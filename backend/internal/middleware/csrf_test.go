package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireClientHeader_AllowsSafeMethods(t *testing.T) {
	h := RequireClientHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(m, "/x", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("method %s: status=%d want 200", m, rec.Code)
		}
	}
}

func TestRequireClientHeader_BlocksMutationWithoutHeader(t *testing.T) {
	h := RequireClientHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(m, "/x", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("method %s: status=%d want 403", m, rec.Code)
		}
	}
}

func TestRequireClientHeader_AllowsMutationWithHeader(t *testing.T) {
	h := RequireClientHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.Header.Set(ClientHeaderName, "web")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

func TestCSRFGuard_RejectsDisallowedOrigin(t *testing.T) {
	guard := CSRFGuard([]string{"https://pms.example.com"})
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.Header.Set(ClientHeaderName, "web")
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "origin not allowed") {
		t.Fatalf("body=%q expected origin error", rec.Body.String())
	}
}

func TestCSRFGuard_AllowsMatchingOrigin(t *testing.T) {
	guard := CSRFGuard([]string{"https://pms.example.com"})
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.Header.Set(ClientHeaderName, "web")
	req.Header.Set("Origin", "https://pms.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

func TestCSRFGuard_NoOriginPasses(t *testing.T) {
	// Non-browser callers (curl, integration tests) often omit Origin.
	// They are still gated by the X-PMS-Client header so this is safe.
	guard := CSRFGuard([]string{"https://pms.example.com"})
	h := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.Header.Set(ClientHeaderName, "web")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}
