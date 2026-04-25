package secretbox

import (
	"encoding/base64"
	"strings"
	"testing"
)

func newTestBox(t *testing.T) *Box {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	b, err := New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	b := newTestBox(t)
	for _, pt := range []string{"hello", "a more interesting secret token XYZ-123", ""} {
		ct, err := b.Encrypt(pt)
		if err != nil {
			t.Fatal(err)
		}
		if pt != "" && !strings.HasPrefix(ct, "v1:") {
			t.Fatalf("missing v1: prefix on %q", ct)
		}
		got, err := b.Decrypt(ct)
		if err != nil {
			t.Fatal(err)
		}
		if got != pt {
			t.Fatalf("round-trip mismatch: got %q want %q", got, pt)
		}
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	b := newTestBox(t)
	got, err := b.Decrypt("legacy-plain-token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "legacy-plain-token" {
		t.Fatalf("got %q want legacy-plain-token", got)
	}
}

func TestDecryptTampered(t *testing.T) {
	b := newTestBox(t)
	ct, err := b.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the base64 body.
	body := ct[len("v1:"):]
	raw, _ := base64.StdEncoding.DecodeString(body)
	raw[len(raw)-1] ^= 0x01
	tampered := "v1:" + base64.StdEncoding.EncodeToString(raw)
	if _, err := b.Decrypt(tampered); err == nil {
		t.Fatal("expected error decrypting tampered ciphertext")
	}
}

func TestWrongKeyFails(t *testing.T) {
	a := newTestBox(t)
	bKey := make([]byte, 32)
	for i := range bKey {
		bKey[i] = byte(0xff - i)
	}
	other, err := New(base64.StdEncoding.EncodeToString(bKey))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := a.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(ct); err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestNilReceiverIsNoOp(t *testing.T) {
	var b *Box
	ct, err := b.Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if ct != "hello" {
		t.Fatalf("nil Encrypt returned %q", ct)
	}
	pt, err := b.Decrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if pt != "hello" {
		t.Fatalf("nil Decrypt returned %q", pt)
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error on empty key")
	}
	if _, err := New("not-base64-and-not-32-chars"); err == nil {
		t.Fatal("expected error on short key")
	}
}
