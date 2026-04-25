package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractExportToken(t *testing.T) {
	cases := []struct {
		name      string
		header    map[string]string
		rawQuery  string
		wantToken string
		wantSrc   exportTokenSource
	}{
		{
			name:      "authorization bearer wins",
			header:    map[string]string{"Authorization": "Bearer abc123", "X-Export-Token": "xyz"},
			rawQuery:  "token=query",
			wantToken: "abc123",
			wantSrc:   exportTokenSourceAuthorizationBearer,
		},
		{
			name:      "x-export-token when no authorization",
			header:    map[string]string{"X-Export-Token": "headertok"},
			rawQuery:  "token=query",
			wantToken: "headertok",
			wantSrc:   exportTokenSourceHeader,
		},
		{
			name:    "query is rejected (PMS_11/T2.6 removed legacy path)",
			rawQuery: "token=legacy",
			wantSrc:  exportTokenSourceNone,
		},
		{
			name:    "missing",
			wantSrc: exportTokenSourceNone,
		},
		{
			name:    "authorization non-bearer is ignored",
			header:  map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			wantSrc: exportTokenSourceNone,
		},
		{
			name:      "authorization case-insensitive bearer",
			header:    map[string]string{"Authorization": "bearer low"},
			wantToken: "low",
			wantSrc:   exportTokenSourceAuthorizationBearer,
		},
		{
			name:      "whitespace-only header value falls through to next source",
			header:    map[string]string{"Authorization": "Bearer    ", "X-Export-Token": "next"},
			wantToken: "next",
			wantSrc:   exportTokenSourceHeader,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/properties/1/occupancy-export?"+c.rawQuery, nil)
			for k, v := range c.header {
				req.Header.Set(k, v)
			}
			tok, src := extractExportToken(req)
			if tok != c.wantToken {
				t.Fatalf("token = %q, want %q", tok, c.wantToken)
			}
			if src != c.wantSrc {
				t.Fatalf("source = %d, want %d", src, c.wantSrc)
			}
		})
	}
}
