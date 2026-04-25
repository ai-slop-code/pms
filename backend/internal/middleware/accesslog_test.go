package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactURLQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"month=2026-04", "month=2026-04"},
		{"token=abc123", "token=REDACTED"},
		{"month=2026-04&token=abc&foo=bar", "foo=bar&month=2026-04&token=REDACTED"},
		{"TOKEN=abc", "TOKEN=REDACTED"},
		{"access_token=abc&api_key=xyz&secret=s", "access_token=REDACTED&api_key=REDACTED&secret=REDACTED"},
		{"password=hunter2&keep=yes", "keep=yes&password=REDACTED"},
		{"token=&month=01", "month=01&token="},
	}
	for _, c := range cases {
		got := RedactURLQuery(c.in)
		if got != c.want {
			t.Errorf("RedactURLQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAccessLogRedactsSensitiveQueryParams(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/properties/1/occupancy-export?token=supersecret&month=2026-04", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := buf.String()
	if strings.Contains(out, "supersecret") {
		t.Fatalf("access log leaked sensitive token: %q", out)
	}
	if !strings.Contains(out, "token=REDACTED") {
		t.Fatalf("access log missing REDACTED marker: %q", out)
	}
	if !strings.Contains(out, "month=2026-04") {
		t.Fatalf("access log dropped non-sensitive param: %q", out)
	}
}
