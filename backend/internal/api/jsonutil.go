package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// DefaultMaxJSONBytes caps the size of JSON request bodies accepted by
// ReadJSON. Handlers that legitimately need larger payloads should call
// ReadJSONN explicitly.
const DefaultMaxJSONBytes int64 = 1 << 20 // 1 MiB

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// JSON API responses reflect mutable server state; prevent browsers and
	// intermediaries from serving stale bodies after writes.
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// ReadJSON decodes a JSON request body into v, enforcing DefaultMaxJSONBytes.
func ReadJSON(r *http.Request, v interface{}) error {
	return ReadJSONN(r, v, DefaultMaxJSONBytes)
}

// ReadJSONN decodes a JSON request body into v, rejecting unknown fields and
// capping the body size at maxBytes. A MaxBytesError is surfaced to callers as
// a "request body too large" error.
func ReadJSONN(r *http.Request, v interface{}, maxBytes int64) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return fmt.Errorf("request body too large")
		}
		return err
	}
	return nil
}
