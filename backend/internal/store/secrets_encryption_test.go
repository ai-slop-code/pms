package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"strings"
	"testing"

	"pms/backend/internal/crypto/secretbox"
	"pms/backend/internal/testutil"
)

func newTestBox(t *testing.T) *secretbox.Box {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	b, err := secretbox.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPropertySecrets_EncryptionRoundTrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	st := &Store{DB: db, Crypto: newTestBox(t)}
	ctx := context.Background()
	pid := setupFinanceProperty(t, st)

	token := "nuki-live-abcdef-123456"
	ics := "https://example.com/calendar.ics"
	lock := "lock-42"
	if err := st.UpdatePropertySecrets(ctx, pid, &ics, &token, &lock); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Raw database row must be encrypted (v1: prefix) — never plaintext.
	var rawToken, rawICS sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT nuki_api_token, booking_ics_url FROM property_secrets WHERE property_id = ?`, pid).Scan(&rawToken, &rawICS); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rawToken.String, "v1:") {
		t.Fatalf("expected encrypted token, got %q", rawToken.String)
	}
	if !strings.HasPrefix(rawICS.String, "v1:") {
		t.Fatalf("expected encrypted ICS url, got %q", rawICS.String)
	}
	if strings.Contains(rawToken.String, token) || strings.Contains(rawICS.String, ics) {
		t.Fatal("ciphertext leaks plaintext")
	}

	// Reading back via Store must return plaintext.
	sec, err := st.GetPropertySecrets(ctx, pid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sec.NukiAPIToken.String != token {
		t.Fatalf("token mismatch: got %q", sec.NukiAPIToken.String)
	}
	if sec.BookingICSURL.String != ics {
		t.Fatalf("ics mismatch: got %q", sec.BookingICSURL.String)
	}
}

func TestPropertySecrets_LegacyPlaintextReadsThrough(t *testing.T) {
	db := testutil.OpenTestDB(t)
	st := &Store{DB: db, Crypto: newTestBox(t)}
	ctx := context.Background()
	pid := setupFinanceProperty(t, st)

	// Simulate a row written before encryption was enabled.
	legacy := "legacy-plain-token"
	if _, err := db.ExecContext(ctx,
		`UPDATE property_secrets SET nuki_api_token = ?, updated_at = datetime('now') WHERE property_id = ?`,
		legacy, pid); err != nil {
		t.Fatal(err)
	}
	sec, err := st.GetPropertySecrets(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if sec.NukiAPIToken.String != legacy {
		t.Fatalf("legacy plaintext not returned verbatim: got %q", sec.NukiAPIToken.String)
	}
}
