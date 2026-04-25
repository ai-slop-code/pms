// Package secretbox provides authenticated symmetric encryption for secrets
// stored in the PMS database. Ciphertexts are framed with a version prefix
// so the format can evolve without schema changes.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// prefix identifies a v1 ciphertext. Values without this prefix are treated
// as legacy plaintext so existing rows remain readable across the rollout
// window until a backfill re-encrypts them.
const prefix = "v1:"

// Box wraps an AES-256-GCM AEAD for a fixed 32-byte master key.
type Box struct {
	gcm cipher.AEAD
}

// New parses a 32-byte master key (base64 or raw) and returns a Box.
func New(masterKey string) (*Box, error) {
	key, err := parseKey(masterKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: g}, nil
}

func parseKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("master key is empty")
	}
	if k, err := base64.StdEncoding.DecodeString(raw); err == nil && len(k) == 32 {
		return k, nil
	}
	if k, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(k) == 32 {
		return k, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("master key must decode to 32 bytes (got %d chars)", len(raw))
}

// Encrypt returns a v1:-prefixed, base64-encoded ciphertext. Empty plaintext
// is returned unchanged (nothing to protect, preserves NULL semantics in
// callers).
func (b *Box) Encrypt(plain string) (string, error) {
	if b == nil || plain == "" {
		return plain, nil
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := b.gcm.Seal(nil, nonce, []byte(plain), nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt. Values without the v1 prefix are returned
// verbatim to support rows that predate encryption; callers should schedule
// a backfill to remove this legacy path over time.
func (b *Box) Decrypt(value string) (string, error) {
	if b == nil || value == "" {
		return value, nil
	}
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(value[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("secretbox: invalid base64: %w", err)
	}
	ns := b.gcm.NonceSize()
	if len(raw) < ns+b.gcm.Overhead() {
		return "", errors.New("secretbox: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: open: %w", err)
	}
	return string(pt), nil
}

// IsEncrypted reports whether value has the v1 prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}
