package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/auth"
)

func TestGetAdminBackup_SuperAdminGetsTarGzWithDB(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()

	// Seed a super_admin so we can hit the gated endpoint.
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateUser(ctx, "root@example.com", hash, "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	// Provisioning gate (PMS_11 follow-up #6) blocks super_admins without
	// 2FA. Pre-enrol the test account so the existing flow still works.
	if err := st.SetUserTOTP(ctx, root.ID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}

	// Seed data dir layout so the archive contains more than just the DB.
	dataDir := t.TempDir()
	for _, rel := range []string{"invoices/1", "attachments/1/42"} {
		if err := os.MkdirAll(filepath.Join(dataDir, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dataDir, "invoices/1/sample.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "attachments/1/42/receipt.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: dataDir, TOTPDevBypass: true}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "root@example.com", "secret123")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/backup", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("backup status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type=%q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "pms-backup-") {
		t.Fatalf("content-disposition=%q", cd)
	}

	gz, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	found := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		found[hdr.Name] = true
		if _, err := io.Copy(io.Discard, tr); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"pms.db", "invoices/1/sample.pdf", "attachments/1/42/receipt.txt"} {
		if !found[want] {
			t.Errorf("archive missing %q; got %v", want, keysOf(found))
		}
	}
}

func TestGetAdminBackup_NonAdminForbidden(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	if _, err := st.CreateUser(ctx, "owner@example.com", hash, "owner"); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "owner@example.com", "secret123")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin/backup", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
