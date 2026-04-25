package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONN_RejectsOversizedBody(t *testing.T) {
	big := strings.Repeat("a", 2048)
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"x":"`+big+`"}`))
	var v struct {
		X string `json:"x"`
	}
	err := ReadJSONN(req, &v, 1024)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "request body too large" {
		t.Fatalf("err=%q want %q", err.Error(), "request body too large")
	}
}

func TestReadJSONN_AcceptsBodyUnderLimit(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"x":"ok"}`))
	var v struct {
		X string `json:"x"`
	}
	if err := ReadJSONN(req, &v, 1024); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.X != "ok" {
		t.Fatalf("x=%q want ok", v.X)
	}
}

func TestReadJSON_UsesDefaultLimit(t *testing.T) {
	// Body larger than default 1 MiB.
	big := strings.Repeat("a", int(DefaultMaxJSONBytes)+16)
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"x":"`+big+`"}`))
	var v struct {
		X string `json:"x"`
	}
	err := ReadJSON(req, &v)
	if err == nil || err.Error() != "request body too large" {
		t.Fatalf("err=%v want request body too large", err)
	}
}
