// Tiny static healthcheck binary baked into the distroless backend image.
// Exits 0 when GET / on the configured URL returns 2xx, non-zero otherwise.
// Used by Docker HEALTHCHECK because the runtime image has no shell.
package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	url := os.Getenv("PMS_HEALTHCHECK_URL")
	if url == "" {
		url = "http://127.0.0.1:8080/readyz"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		os.Exit(1)
	}
}
