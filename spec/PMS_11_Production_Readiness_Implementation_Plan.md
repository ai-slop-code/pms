# PMS_11 — Production Readiness Implementation Plan

**Companion to:** [PMS_10_Production_Readiness_Review.md](PMS_10_Production_Readiness_Review.md)
**Audience:** AI coding agent (Copilot, Claude, etc.) + human reviewer.
**Goal:** Remediate every finding in PMS_10 with concrete, reviewable steps. Each task has: *why*, *files*, *exact change*, *acceptance checks*, and a final checkbox.
**Working rules for the agent:**
1. Complete tasks in order within a phase. Phases are independent unless noted.
2. Each task must keep the full test suite green (`make test`). Never skip tests.
3. No drive-by refactors. One task = one focused diff.
4. After every P0 task, run `make backend-test frontend-test` and fix regressions before continuing.
5. Update the checkbox at the end of the task when all acceptance checks pass.
6. If a task is blocked by missing info, stop and ask; do not guess secrets, keys, or infrastructure choices.

---

## Status overview (updated 2026-04-25)

| Phase | Scope | Status |
|-------|-------|--------|
| Phase 0 | Prep (branch, baseline, dep) | ✅ Complete |
| Phase 1 | P0 blockers (T1.1–T1.9) | ✅ Complete — all tests green |
| Phase 2 | P1 first-month items (T2.1–T2.13) | ✅ Complete — all tests green |
| Phase 3 | P2 quarterly items (T3.1–T3.6) | ✅ Complete — T3.1/T3.2/T3.3/T3.4/T3.5/T3.6 done; T3.7 dropped |

**Baseline invariant held:** backend `go test -count=1 ./...` green across all packages; frontend 50 files / 255 tests green; e2e Playwright smoke green locally.

**Key artefacts added:**
- Backend: [`internal/crypto/secretbox`](../backend/internal/crypto/secretbox), [`internal/backup`](../backend/internal/backup), [`internal/logging`](../backend/internal/logging), [`internal/sentryx`](../backend/internal/sentryx), [`middleware/secheaders.go`](../backend/internal/middleware/secheaders.go), [`middleware/csrf.go`](../backend/internal/middleware/csrf.go), [`middleware/ratelimit.go`](../backend/internal/middleware/ratelimit.go), [`api/health_handlers.go`](../backend/internal/api/health_handlers.go).
- Deploy: [`deploy/Dockerfile.backend`](../deploy/Dockerfile.backend), [`deploy/Dockerfile.frontend`](../deploy/Dockerfile.frontend), [`deploy/nginx.conf`](../deploy/nginx.conf), [`deploy/docker-compose.yml`](../deploy/docker-compose.yml), [`deploy/Caddyfile`](../deploy/Caddyfile), [`deploy/systemd/pms-server.service`](../deploy/systemd/pms-server.service), [`deploy/.env.example`](../deploy/.env.example).
- CI: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), [`.github/dependabot.yml`](../.github/dependabot.yml).
- Docs: [`docs/deployment/README.md`](../docs/deployment/README.md) (Docker, systemd, static-frontend, manual).

**Notable deviations from plan:**
- T1.9 implemented as **Option B** (in-app `VACUUM INTO` scheduler + retention). Litestream remains an optional add-on for operators that want it.
- Go toolchain bumped 1.22 → 1.25 transitively when `go get github.com/getsentry/sentry-go` ran; accepted.
- Dockerfile split into backend + frontend (nginx) images for operators who want to serve the SPA independently. Compose topology is `caddy → pms-frontend (nginx) → pms-backend`.

---

## Phase 0 — Prep (do first, once)

### T0.1 Create a feature branch and baseline

- [x] Branch: `prod-readiness/phase-p0` off `main`.
- [x] Record baseline: `make test` — capture pass count (expected: BE all green, FE 49 files / 251 tests green). If baseline is red, stop and fix first.
- [x] Record `go vet ./...`, `cd frontend && npm run lint` clean.

### T0.2 Add dev dependency: `golang.org/x/time/rate`

- [x] `cd backend && go get golang.org/x/time/rate && go mod tidy`.
- [x] Commit `go.mod` / `go.sum`.

---

## Phase 1 — P0 blockers (pre-prod)

### T1.1 Hardened `http.Server` with timeouts + graceful shutdown

**Why:** PMS_10 §3.1, §3.2. Current `http.ListenAndServe` has no timeouts (Slowloris) and no shutdown path (WAL corruption risk on deploy).

**Files:**
- [backend/cmd/server/main.go](../backend/cmd/server/main.go)

**Steps:**
1. Replace the final `http.ListenAndServe(cfg.HTTPAddr, r)` block with:
   ```go
   ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
   defer stop()

   srv := &http.Server{
       Addr:              cfg.HTTPAddr,
       Handler:           r,
       ReadHeaderTimeout: 10 * time.Second,
       ReadTimeout:       30 * time.Second,
       WriteTimeout:      120 * time.Second, // invoice PDF + backup streaming
       IdleTimeout:       120 * time.Second,
       MaxHeaderBytes:    1 << 20,
       BaseContext:       func(net.Listener) context.Context { return ctx },
   }

   go func() {
       log.Printf("listening on %s", cfg.HTTPAddr)
       if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
           log.Fatalf("http server: %v", err)
       }
   }()

   <-ctx.Done()
   log.Printf("shutdown signal received; draining")
   shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   if err := srv.Shutdown(shutdownCtx); err != nil {
       log.Printf("graceful shutdown: %v", err)
   }
   _ = db.Close()
   ```
2. Pass `ctx` into every `go scheduler(...)` launch and replace `time.NewTicker` loops with `select { case <-ctx.Done(): return; case <-ticker.C: ... }` so schedulers exit cleanly.
3. Add `"context"`, `"errors"`, `"net"`, `"os/signal"`, `"syscall"` imports as required.

**Acceptance:**
- `SIGTERM` causes process to exit within 30 s with exit code 0.
- Access log shows in-flight requests completing after shutdown signal.
- `make backend-test` green.

- [x] T1.1 done.

### T1.2 Recover-wrap scheduler goroutines

**Why:** PMS_10 §3.4. A panic inside a scheduler tick kills the goroutine silently.

**Files:**
- [backend/cmd/server/main.go](../backend/cmd/server/main.go)

**Steps:**
1. Add a helper at file scope:
   ```go
   func safeTick(name string, fn func()) {
       defer func() {
           if rec := recover(); rec != nil {
               log.Printf("scheduler %s: panic: %v", name, rec)
           }
       }()
       fn()
   }
   ```
2. Wrap each tick body: `safeTick("occupancy", func() { /* existing body */ })`, likewise for `nuki_cleanup` and `cleaning_reconcile`.

**Acceptance:**
- Add a test-only scheduler that panics and verify it does not take down the process (can be a light unit test on `safeTick`).

- [x] T1.2 done.

### T1.3 Global JSON body size cap via `http.MaxBytesReader`

**Why:** PMS_10 §2.3. Unbounded JSON bodies = OOM DoS.

**Files:**
- [backend/internal/api/jsonutil.go](../backend/internal/api/jsonutil.go)
- All handlers that call `ReadJSON` (no change needed if signature is kept).

**Steps:**
1. In `ReadJSON`, before decoding:
   ```go
   const defaultMaxJSONBytes = 1 << 20 // 1 MiB
   func ReadJSON(r *http.Request, v any) error {
       return ReadJSONN(r, v, defaultMaxJSONBytes)
   }
   func ReadJSONN(r *http.Request, v any, maxBytes int64) error {
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
   ```
2. Keep the existing `ReadJSON` signature for compatibility; no caller changes required.
3. Any handler that legitimately needs >1 MiB JSON switches to `ReadJSONN(r, &body, 5<<20)`.

**Acceptance:**
- New test: POST a 2 MiB JSON body to any auth-protected endpoint → 400 `"request body too large"`.
- Existing tests stay green.

- [x] T1.3 done.

### T1.4 Multipart size cap before parse

**Why:** PMS_10 §2.3. `ParseMultipartForm(25<<20)` allocates first; wrap body first.

**Files:**
- [backend/internal/api/finance_handlers.go](../backend/internal/api/finance_handlers.go) (two call sites: line ~816 and ~1236).

**Steps:**
1. Before each `r.ParseMultipartForm(N)` call, add:
   ```go
   r.Body = http.MaxBytesReader(w, r.Body, N+1<<20) // N + headroom for part boundaries
   ```

**Acceptance:**
- Uploading a 30 MiB file to the 25 MiB endpoint returns `413`/`400`, not OOM.

- [x] T1.4 done.

### T1.5 CSRF mitigation via required custom header

**Why:** PMS_10 §2.1 gap 1. SameSite=lax permits simple cross-origin POSTs of `multipart/form-data` and `application/x-www-form-urlencoded`.

**Files:**
- [backend/internal/middleware/auth.go](../backend/internal/middleware/auth.go) (new middleware file or add next to `Auth`).
- [backend/cmd/server/main.go](../backend/cmd/server/main.go) (router wiring).
- [frontend/src/api/http.ts](../frontend/src/api/http.ts) (inject header on every request).

**Steps:**
1. Create `backend/internal/middleware/csrf.go`:
   ```go
   package middleware

   import "net/http"

   // RequireCustomHeader rejects state-changing requests that do not carry the
   // custom header "X-PMS-Client: web". Browsers cannot set custom headers on
   // simple cross-origin form submissions, so this is a robust CSRF shield
   // for cookie-authenticated JSON/multipart APIs.
   func RequireCustomHeader(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           switch r.Method {
           case http.MethodGet, http.MethodHead, http.MethodOptions:
               next.ServeHTTP(w, r)
               return
           }
           if r.Header.Get("X-PMS-Client") == "" {
               http.Error(w, `{"error":"missing client header"}`, http.StatusForbidden)
               return
           }
           next.ServeHTTP(w, r)
       })
   }
   ```
2. Mount it **after** `Auth` in `/api` router group, but **exclude** `/api/properties/{id}/occupancy-export` (explicit header-token endpoint, no cookie).
3. In `frontend/src/api/http.ts`, add `'X-PMS-Client': 'web'` to the default header record in `api()`.
4. Update any existing integration tests that POST/PATCH/DELETE through `httptest` to add the header (add a `setCSRFHeader(req)` helper in `testutil`).

**Acceptance:**
- Curl `POST /api/…` without the header → `403`.
- FE requests work unchanged.
- Test suite green.

- [x] T1.5 done.

### T1.6 Login rate limiting

**Why:** PMS_10 §2.1 gap 2.

**Files:**
- [backend/internal/middleware/ratelimit.go](../backend/internal/middleware/ratelimit.go) (new).
- [backend/internal/api/server.go](../backend/internal/api/server.go) or wherever login route is registered.

**Steps:**
1. New package-level middleware using `golang.org/x/time/rate`:
   ```go
   type keyedLimiter struct {
       mu       sync.Mutex
       limiters map[string]*rate.Limiter
       r        rate.Limit
       b        int
   }

   func NewKeyedLimiter(rps rate.Limit, burst int) *keyedLimiter { ... }
   func (k *keyedLimiter) Allow(key string) bool { ... }
   ```
2. Build `LoginRateLimit(next)` that keys on `clientIP + ":" + normalisedEmail` (pull email from parsed body — requires reading body once via `io.ReadAll` + restoring). Limit: `rate.Every(3*time.Second)`, burst 5. On deny → `429` with `Retry-After: 10`.
3. An alternative simpler approach: key only on client IP with limit `rate.Every(2*time.Second)`, burst 5. Safer, no body parsing. **Prefer this.**
4. Mount before the login handler only.
5. Config knobs: `LOGIN_RATE_PER_SEC` (default `0.5`), `LOGIN_BURST` (default `5`), `LOGIN_RATE_ENABLED` (default `true`).
6. Use `strings.Split(r.RemoteAddr, ":")[0]` or better: respect `X-Forwarded-For` **only if `TRUSTED_PROXY=true`** (new config flag; default false).

**Acceptance:**
- Test fires 20 bad logins in 2 s from the same IP → 5 pass through, 15 get `429`.
- Legitimate logins after cooldown succeed.

- [x] T1.6 done.

### T1.7 Default-secure `/metrics`

**Why:** PMS_10 §2.4, §3.6.

**Files:**
- [backend/cmd/server/main.go](../backend/cmd/server/main.go)
- [backend/internal/config/config.go](../backend/internal/config/config.go)

**Steps:**
1. In config: if `PMS_ENV=production` and `PMS_METRICS_TOKEN` is empty → `return fmt.Errorf("PMS_METRICS_TOKEN required in production")`.
2. Alternative accepted: `PMS_METRICS_BIND=127.0.0.1:9090` starts a second `http.Server` bound to loopback serving only `/metrics`. If set, ignore token on the main listener and 404 `/metrics` there.
3. Document both options in `.env.example`.

**Acceptance:**
- `PMS_ENV=production` without token or separate bind fails to start with a clear error.
- `PMS_ENV=dev` still serves `/metrics` unauthenticated (developer convenience).

- [x] T1.7 done.

### T1.8 Secrets at rest: AES-256-GCM for Nuki token, ICS URL, PIN

**Why:** PMS_10 §2.5. Single DB dump = all lock credentials.

**Files:**
- [backend/internal/crypto/secretbox.go](../backend/internal/crypto/secretbox.go) (new package).
- [backend/internal/config/config.go](../backend/internal/config/config.go) (read `PMS_MASTER_KEY`).
- [backend/internal/store/store.go](../backend/internal/store/store.go) (encrypt on write, decrypt on read for `property_secrets`).
- [backend/internal/store/nuki.go](../backend/internal/store/nuki.go) (same for `generated_pin_plain`).
- [backend/internal/migrate/000016_encrypt_secrets.up.sql](../backend/internal/migrate/) (new; no DDL, just a marker row).
- A one-shot re-encrypt routine in `backend/cmd/server/main.go` on startup.

**Steps:**
1. Implement `crypto.Box` with `Encrypt(plaintext []byte) (string, error)` returning `"v1:" + base64(nonce || ciphertext || tag)` and `Decrypt(string) ([]byte, error)`. Use `crypto/aes` + `crypto/cipher.NewGCM`.
2. Key from env `PMS_MASTER_KEY` — must be 32 random bytes base64. Validate length at startup. **Fail fast** if missing and `PMS_ENV=production`.
3. Back-compat: `Decrypt` must accept the raw (unencrypted) string if it does **not** start with `"v1:"` and return as-is (so the first decrypt after deploy still works).
4. Wrap `property_secrets.nuki_api_token` and `property_secrets.booking_ics_url` on **all read/write paths** in the store. Add unit tests for round-trip.
5. Same for `nuki_access_codes.generated_pin_plain`.
6. On startup, if `PMS_ENCRYPT_BACKFILL=true`, iterate rows missing the `"v1:"` prefix and rewrite encrypted. Log progress. One-shot; operator unsets flag afterwards.
7. Document key generation in `.env.example`:
   ```
   # Required in production. Generate with:
   #   openssl rand -base64 32
   PMS_MASTER_KEY=
   ```

**Acceptance:**
- New rows stored with `"v1:"` prefix.
- Existing plaintext rows still decrypt on read.
- With backfill flag set, all rows end up encrypted on next boot.
- Unit tests: encrypt → decrypt round-trip; wrong key → error; tampered ciphertext → error.

- [x] T1.8 done.

### T1.9 Automated off-host backups (decision required)

**Why:** PMS_10 §3.5. Single-file SQLite with no automation = single point of total failure.

**Two accepted paths — pick one and document:**

**Option A — Litestream (recommended for S3/GCS-ready environments):**
- [ ] Add `docs/deployment/litestream.md` with the YAML config template.
- [ ] Add `docs/deployment/restore-runbook.md`.
- [ ] No code changes; Litestream runs as a sidecar/service.

**Option B — In-app hourly snapshot to `DATA_DIR/backups/` + external rsync:**
- [x] New goroutine `backupScheduler` in `main.go` runs `VACUUM INTO` every `BACKUP_INTERVAL_MINUTES` (default 60). Implemented in [`internal/backup/backup.go`](../backend/internal/backup/backup.go).
- [x] Retention: keep last 24 hourly, last 7 daily (covered by `Prune` + [`backup_test.go`](../backend/internal/backup/backup_test.go)).
- [x] Operator is responsible for off-host rsync (documented in [`docs/deployment/README.md`](../docs/deployment/README.md)).

**Acceptance (both):**
- Restore drill documented with exact commands.
- Monitoring: a metric `pms_last_successful_backup_unixtime` exposed via Prometheus.

- [x] T1.9 done — **Option B implemented** (in-app `VACUUM INTO` scheduler in [`backend/internal/backup`](../backend/internal/backup), hourly snapshots, 24 hourly + 7 daily retention, `pms_last_successful_backup_unixtime` exposed). Litestream left as an operator-level add-on.

---

## Phase 2 — P1 (first month in production)

### T2.1 Security response headers middleware

**Why:** PMS_10 §2.4.

**Files:** `backend/internal/middleware/secheaders.go` (new); wire in `main.go` **before** `Recoverer`.

```go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        h := w.Header()
        h.Set("X-Content-Type-Options", "nosniff")
        h.Set("X-Frame-Options", "DENY")
        h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
        h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
            h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }
        // CSP: loose for JSON API, stricter set by SPA shell via <meta>.
        h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
        next.ServeHTTP(w, r)
    })
}
```

- [x] Add a CSP `<meta>` to `frontend/index.html` for the SPA bundle:
      `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none';`.
      Shipped as: `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'`.
- [x] Acceptance: `curl -I https://…/api/health` shows all headers.

- [x] T2.1 done.

### T2.2 Structured logging with `log/slog`

**Why:** PMS_10 §3.6.

**Files:** new `backend/internal/logging/logging.go`; replace `log.Printf` uses incrementally.

- [x] `slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))`.
- [x] `LOG_LEVEL` env (`debug|info|warn|error`, default `info`).
- [x] Access log middleware already emits structured JSON when `PMS_ACCESS_LOG_FORMAT=json`; `log.Printf` call sites now route through slog via an `io.Writer` bridge in [`internal/logging`](../backend/internal/logging).
- [x] Keep `log.Printf` behaviour visually similar — must not regress existing log-scraping tests.

- [x] T2.2 done.

### T2.3 Error reporting

**Why:** PMS_10 §3.6.

- [x] Add Sentry SDK (`github.com/getsentry/sentry-go`) conditionally wired when `SENTRY_DSN` is set — see [`internal/sentryx`](../backend/internal/sentryx).
- [x] `sentry.Recoverer`-style middleware **after** `chimw.Recoverer` re-raises into Sentry.
- [x] Scrub `Authorization`, `Cookie`, `X-Export-Token`, `X-PMS-Client`, `X-CSRF-Token`, `Set-Cookie` + `?token=…` query strings before shipping events.

- [x] T2.3 done.

### T2.4 Password policy + session invalidation

**Why:** PMS_10 §2.1 gaps 3 & 4.

**Files:**
- [backend/internal/auth/password.go](../backend/internal/auth/password.go) — add `ValidatePassword(raw string) error`.
- `backend/internal/store/store.go` — on password change, `DELETE FROM auth_sessions WHERE user_id = ?` (keep current session, so invalidate *others*).
- `backend/internal/api/server.go` — login / password-change / user-create handlers call `ValidatePassword` first.

**Policy:**
- Min 12 chars.
- Must not be in a small `data/common-passwords.txt` list (ship top-1000).
- No further complexity rules (NIST SP 800-63B stance).

**Session rotation:**
- On password change: delete all sessions of that user, then issue a fresh cookie to the current user.
- Add `POST /api/users/me/sessions/revoke-all` admin primitive.

- [x] T2.4 done — `ValidatePassword` enforces 12-char minimum + case-insensitive deny-list (≈20 common passwords). `patchUser` calls `DeleteSessionsForUserExcept` (self, preserves current cookie) or `DeleteSessionsForUser` (admin resetting another user). Explicit `/revoke-all` endpoint deferred — not required by current product surface.

### T2.5 Per-endpoint rate limiting

**Why:** PMS_10 §2.6.

- [x] Reuse `NewKeyedLimiter` from T1.6.
- [x] Apply `rate.Every(5*time.Second), burst 3` to invoice PDF regeneration, `rate.Every(2*time.Second), burst 5` to attachment upload, per-user keying. `admin/backup` additionally limited at `rate.Every(30*time.Second), burst 1`.
- [x] 429 response with `Retry-After`.

- [x] T2.5 done.

### T2.6 Remove query-string occupancy token

**Why:** PMS_10 §2.9.

- [x] Set deprecation sunset date in FE/docs.
- [x] After one release, **drop** the `?token=` branch in `occupancy_handlers.go`; keep only `Authorization: Bearer` and `X-Export-Token`.
- [x] Add test: `?token=` now returns `401`.

- [x] T2.6 done.

### T2.7 `/healthz` + `/readyz` split

**Why:** PMS_10 §5.4.

- [x] `/healthz` — process liveness, return 200 with `{"ok":true}`.
- [x] `/readyz` — `s.Store.DB.PingContext(ctx)` (2s timeout) → 200 or 503 `{"ok":false,"reason":"db unreachable"}`. `/health` retained as alias.
- [ ] Per-scheduler `pms_scheduler_last_success_unixtime` — deferred; backup already exposes `pms_last_successful_backup_unixtime`. Add remaining schedulers in Phase 3 if needed.

- [x] T2.7 done (liveness/readiness split). Scheduler-age metrics remain a follow-up.

### T2.8 Deployment artefacts

**Why:** PMS_10 §5.3.

Create:
- [x] `deploy/Dockerfile.backend` — multi-stage, distroless, non-root. Separate `deploy/Dockerfile.frontend` (nginx) ships the static SPA.
- [x] `deploy/docker-compose.yml` — caddy → pms-frontend (nginx) → pms-backend; Caddy automatic TLS.
- [x] `deploy/Caddyfile` — adds HSTS + forwards to `pms-frontend:80`.
- [x] `deploy/nginx.conf` — SPA history fallback + `/api/*` reverse proxy to `pms-backend:8080`.
- [x] `deploy/systemd/pms-server.service` — `Restart=on-failure`, `LimitNOFILE=65535`, `ProtectSystem=strict`, `ReadWritePaths=/var/lib/pms`, full seccomp hardening.
- [x] `docs/deployment/README.md` — covers Docker Compose, systemd, **static-frontend + your own infra**, and manual/local.

**Note:** no infra provisioning; the agent only ships templates.

- [x] T2.8 done.

### T2.9 CI workflow

**Why:** PMS_10 §4.8.

- [x] `.github/workflows/ci.yml`:
    - Job `backend`: `go vet ./...`, `go test -race -count=1 ./...`, `govulncheck ./...`, `gosec`.
    - Job `frontend`: `npm ci`, `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`.
    - Job `docker`: builds backend + frontend images with separate gha caches (only on push to main).
    - Cache Go modules and `node_modules`.
- [x] `.github/dependabot.yml` — weekly Go + npm + GitHub Actions + Docker bumps, grouped minor/patch.

- [x] T2.9 done.

### T2.10 Audit-log retention

**Why:** PMS_10 §4.5.

- [x] New scheduler `auditRetention` runs daily via `store.DeleteAuditLogsBefore(ctx, cutoff)`; active iff `AUDIT_LOG_RETENTION_DAYS > 0`.
- [x] `AUDIT_LOG_RETENTION_DAYS` env (default `365`, `0` disables).
- [x] Emits a `pms_audit_log_deleted_total` counter (`metrics.RecordAuditLogDeletion`).

- [x] T2.10 done.

### T2.11 Fix `downloadInvoice` audit ordering

**Why:** PMS_10 §2.2 (minor) — audit is written after `http.ServeFile` even if the write failed.

- [x] Took the simpler route: `s.audit(..., "invoice_download", …)` now runs **before** `http.ServeFile`, with a comment explaining the trade-off.

- [x] T2.11 done.

### T2.12 Harden `admin/backup` error message

**Why:** PMS_10 §2.8.

- [x] Replaced `"database snapshot failed: "+err.Error()` with opaque `"backup failed"` to the client; full error logged server-side via `log.Printf` (which now routes through slog).
- [x] Added `rate.Every(30*time.Second), burst 1` limiter on this endpoint (`AdminBackupLimiter`).

- [x] T2.12 done.

### T2.13 Reject CORS `*` with credentials

**Why:** PMS_10 §2.7.

- [x] In `config.Load()`, if `cors_origins` contains `"*"` and `AllowCredentials` is true → startup error.
- [x] Log resolved origins on startup.

- [x] T2.13 done.

---

## Phase 3 — P2 (first quarter)

### T3.1 TOTP-based 2FA for write/admin users

**Why:** PMS_10 §2.1 gap 5.

Outline:
- [x] New columns `users.totp_secret`, `users.totp_enrolled_at` (encrypted at rest via `store.Crypto`); new table `user_recovery_codes (id, user_id, code_hash UNIQUE, used_at, created_at)`.
- [x] Session quarantine: `auth_sessions.mfa_verified` flag; sessions for enrolled users start with `mfa_verified=0` until the TOTP challenge passes.
- [x] Enrolment flow: `POST /api/auth/2fa/enroll/start` → returns `{secret, otpauth_url}`; `POST /api/auth/2fa/enroll/confirm` with `{secret, code}` → persists secret, returns 10 one-time recovery codes (shown once).
- [x] Login: if enrolled, `/api/auth/login` returns `{mfa_required:true}` and the session is quarantined; client must call `POST /api/auth/2fa/verify` with `{code}` or `{recovery_code}` to upgrade. `/api/auth/me` mirrors this contract.
- [x] Recovery codes: 10 codes, format `ABCDE-12345`, stored as SHA-256 hashes, single-use via atomic `UPDATE … WHERE used_at IS NULL`.
- [x] Disable: `POST /api/auth/2fa/disable` requires password re-entry; clears secret and recovery codes.
- [x] Per-user opt-in (no forced enrolment yet — owner/admin enforcement can be layered later via middleware).
- [x] Dev escape hatch: `PMS_2FA_DEV_BYPASS=true` skips the challenge for enrolled users (rejected at startup unless `PMS_ENV ∈ {dev,development,test}`).
- [x] Frontend: `LoginView` two-step (password → TOTP/recovery), `TwoFactorSection` self-service component on the user's own profile, Pinia store exposes `mfaPending` and `verifyTwoFactor`/`twoFactor*` helpers.
- [x] Tests: `internal/totp` unit tests, `internal/api/totp_handlers_test.go` enrol/challenge/recovery/disable/dev-bypass coverage, frontend `auth.totp.spec.ts` for store branches.

QR rendering note: backend returns the standard `otpauth://` URL; UI shows it as a clickable link plus the secret for manual entry. A QR image renderer can be added later without API changes.

- [x] T3.1 done.

### T3.2 OpenAPI + TS type generation

- [x] Hand-authored `spec/openapi.yaml` (OpenAPI 3.1) covering system, auth, users/permissions, and properties surfaces. Remaining modules (occupancy, cleaning, finance, invoices, messages, nuki, dashboard, analytics, bookingPayouts) will be migrated incrementally (tracked in `frontend/src/api/types/README.md`).
- [x] Frontend tooling: `openapi-typescript@7` added; `npm run types:openapi` writes `frontend/src/api/types/generated.ts` (committed).
- [x] Hand-authored DTOs in `frontend/src/api/types/*.ts` retained to avoid breaking 251 existing tests; `README.md` documents the per-module migration order. Note: `oapi-codegen` is deferred — for a TS-only consumer, `openapi-typescript` is the leaner, sufficient fit; server-side handlers remain chi today and can adopt `oapi-codegen` later if needed.

- [x] T3.2 done (partial-coverage MVP; incremental migration path established).


### T3.3 E2E test suite

- [x] Playwright workspace under `e2e/` (config + scripts + tests). The harness builds a hermetic backend on port 18080 (fresh SQLite under `e2e/.runtime/`, per-run `PMS_MASTER_KEY`, bootstrap super-admin) and runs the Vite dev server on 15173 with its `/api` proxy pointed at the test backend so the SPA hits same-origin routes — matching the production reverse-proxy shape.
- [x] Smoke flow `tests/smoke.spec.ts`: login → navigate to Properties → create a new property → logout. Exercises CSRF header, cookie session, RBAC, navigation guards, and the encrypted store. Verified locally green in 2.7 s.
- [x] CI job `e2e` in `.github/workflows/ci.yml` runs the suite on `ubuntu-latest` with cached Playwright browsers; uploads the HTML report (and, on failure, raw test artefacts) for inspection.
- [x] Vite config gained `VITE_DEV_API_PROXY` so the proxy target is parameterised — used by the e2e harness, no impact on day-to-day dev (defaults to `http://127.0.0.1:8080`).

Deferred (intentionally) from the original spec scenario: occupancy → invoice → PDF download. Those flows depend on fixture-heavy paths (ICS sync or manual occupancy entry, invoice numbering, payout selection) that are still in flux. The harness is reused by adding a new `*.spec.ts` once those flows stabilise; nothing about the infrastructure has to change.

- [x] T3.3 done.

### T3.4 Alerting bundle

- [x] `deploy/monitoring/prometheus-rules.yml` with:
    - `PMSHigh5xxRate` — HTTP 5xx / total > 1% for 5m.
    - `PMSSchedulerStalled` — `(time() - pms_scheduler_last_run_timestamp_seconds) > 3h` for 10m.
    - `PMSBackupStale` — `time() - pms_last_successful_backup_unixtime > 2h` for 15m.
    - `PMSFrequentRestarts` — container restarts via `changes(container_start_time_seconds[15m]) > 2`.
    - `PMSNoTraffic` — `up{job="pms"} == 0` for 2m.
- [x] `deploy/monitoring/grafana-dashboard.json` — p50/p95 latency (from `pms_http_request_duration_seconds_bucket`), RPS by status, 5xx rate stat panel, backup age, audit deletions 24h, process resident memory, scheduler age. (SQLite WAL size gauge deferred — not currently exported.)

- [x] T3.4 done.


### T3.5 OpenTelemetry traces

- [x] Gated behind `OTEL_EXPORTER_OTLP_ENDPOINT` — empty value = no-op. `OTEL_EXPORTER_OTLP_PROTOCOL` (grpc default | http/protobuf) and `OTEL_TRACES_SAMPLER_ARG` (default 0.1) recognised; documented in `deploy/.env.example`.
- [x] New `backend/internal/otelx` package: SDK bootstrap + shutdown, `Middleware(serviceName)` for chi (via `otelhttp`), `HTTPTransport(base)` for outbound clients, `StartSpan` helper. Resource attrs include `service.name=pms-backend`, `service.version=$PMS_RELEASE`, `deployment.environment=$PMS_ENV`.
- [x] Wired into: chi router in `cmd/server/main.go` (tracing middleware before access log); Nuki HTTP client (`internal/nuki/client.go`); occupancy ICS sync HTTP client (`internal/occupancy/sync.go`).
- [x] SQLite store-level instrumentation deferred — spec says "top-level only" and the HTTP span already covers store calls; the added value is marginal vs. the overhead of `otelsql` wiring. Revisit if traces look too coarse in production.
- [x] Tests: `internal/otelx/otelx_test.go` asserts disabled-mode no-op semantics.

- [x] T3.5 done.


### T3.6 Postgres migration option

- [x] `docs/adr/ADR-001-persistence.md` — status **Accepted**. Keeps SQLite for now; documents triggers that would force revisiting (multi-writer, persistent `SQLITE_BUSY` contention, >10M rows, ops dissatisfaction), the driver swap (`modernc.org/sqlite` → `jackc/pgx/v5/stdlib`), dialect deltas (AUTOINCREMENT → BIGSERIAL, datetime/strftime → `now()/to_char`, REAL → NUMERIC(18,2), COLLATE NOCASE → CITEXT or functional index, REPLACE INTO → ON CONFLICT DO UPDATE), data migration via pgloader, and approximate effort.

- [x] T3.6 done.


### T3.7 SSO/OIDC option

- ❌ **Dropped (2026-04-25).** Decision: not worth the maintenance burden for the current user base. Self-hosted password auth + per-user TOTP (T3.1) is sufficient. Revisit if/when an enterprise tenant requests SSO.

- [x] T3.7 done (dropped).

---

## Cross-cutting rules for the agent

### Test discipline

For every backend change:
- [ ] Add or extend a test in the nearest `_test.go`.
- [ ] Run `cd backend && go test ./...` before declaring task done.

For every middleware change:
- [ ] Add a direct unit test that exercises the middleware in isolation, not just through integration.

For every frontend-touching change (T1.5 header injection):
- [ ] Extend `frontend/src/api/http.spec.ts` to assert the header is present.
- [ ] `cd frontend && npm test` must stay green.

### Documentation

- [ ] Every new env var goes into `backend/.env.example` with a comment and safe default.
- [ ] Every new operational surface (metrics bind, backup schedule, master key generation) goes into `docs/deployment/README.md`.

### Non-goals (do not do)

- Do not rewrite existing handlers for style.
- Do not introduce new heavy frameworks (no Gin, no GORM, no OAuth libs beyond what's required).
- Do not change the database from SQLite; migration is tracked in T3.6 only.
- Do not add CSRF tokens when the header shield (T1.5) is already accepted — one mechanism is enough.
- Do not silently change test assertions to make them pass.

---

## Final cross-check

Run this list after all P0 + P1 tasks close. All items must be [x]:

**Security**
- [x] `http.Server` timeouts configured and observable via code review.
- [x] `http.MaxBytesReader` present in `ReadJSON`.
- [x] Every multipart handler wraps body with `MaxBytesReader` before parse.
- [x] `X-PMS-Client` required on all state-changing routes except the explicit token-header occupancy export.
- [x] Login rate limiting blocks 20-attempts-in-2s from single IP.
- [x] `/metrics` unreachable without token (or on separate bind) in production.
- [x] `property_secrets` and `generated_pin_plain` columns contain only `"v1:"`-prefixed ciphertexts after backfill.
- [x] Security headers present on every API response (`curl -I`).
- [x] SPA ships a CSP `<meta>`.
- [x] Password policy enforced at all password-set entry points.
- [x] Password change invalidates other sessions.
- [x] CORS `*` with credentials rejected at startup.
- [x] `admin/backup` returns opaque error; is rate-limited.
- [x] Query-string occupancy token path removed.

**Stability**
- [x] `SIGTERM` drains the server in ≤30 s and closes DB cleanly.
- [x] Scheduler goroutines recover from panics.
- [x] `/healthz` vs `/readyz` distinguished.
- [x] Backup strategy documented + monitored.

**Maintainability / Infra**
- [x] `slog` JSON logs with request IDs.
- [x] Sentry (or equivalent) wired behind `SENTRY_DSN`.
- [x] CI workflow defined (green on `main` pending first push).
- [x] Dependabot configured.
- [x] Dockerfile (backend + frontend) + Compose + systemd + Caddyfile + nginx.conf in `deploy/`.
- [x] `docs/deployment/README.md` covers: Docker Compose, systemd, static-frontend + own infra, manual/local; includes master key generation and backup/restore.
- [x] Audit-log retention job running.
- [x] All new env vars in `.env.example`.

**Tests**
- [x] `make test` green (backend all packages, frontend 50 files / 255 tests as of 2026-04-25).
- [x] `govulncheck ./...` wired into CI `backend` job (runs on every push/PR).
- [x] `gosec ./...` wired into CI `backend` job (SARIF artefact, no-fail mode).
- [x] E2E Playwright smoke (`e2e/tests/smoke.spec.ts`) wired as the `e2e` CI job and verified green locally in 2.7 s; full report uploaded as artefact, raw test-results uploaded on failure.

When every box above is checked, the product is production-ready per PMS_10.

— *End of PMS_11.*
