# PMS_10 — Production Readiness Review

**Author role:** Staff software architect · application security specialist · pen tester · infrastructure architect
**Scope:** Full stack — Go backend (`backend/`), Vue 3 SPA (`frontend/`), persistence (SQLite), deployment surface, observability.
**Method:** Static review of the working tree against the OWASP ASVS L2 baseline, Go production HTTP server guidelines, and typical 12-factor / container deployment expectations. No code changes made.
**Verdict:** **Not production-ready.** The product is functionally complete and internally well-tested (251 FE tests, large BE suite), and the core crypto primitives are chosen correctly (bcrypt cost 12, SHA-256 session hash, parameterised SQL everywhere), but the HTTP edge, secrets handling, and deployment story have several pre-prod blockers.

---

## 1. Executive summary

### 1.1 Pre-production blockers (P0 — must fix before any internet-facing deployment)

| # | Area | Finding | Severity |
|---|------|---------|----------|
| P0-1 | HTTP server | Bare `http.ListenAndServe` in [backend/cmd/server/main.go](backend/cmd/server/main.go#L200) — no `ReadTimeout`, `ReadHeaderTimeout`, `WriteTimeout`, or `IdleTimeout`. Trivial Slowloris / slow-POST DoS against a single-process Go binary. | **Critical — Stability/DoS** |
| P0-2 | HTTP server | No graceful shutdown, no `context.Context` cancellation, no `SIGTERM` handling. In-flight requests and SQLite writes can be killed mid-transaction on deploy/rollout, and the three scheduler goroutines (occupancy, nuki cleanup, cleaning reconcile) leak on shutdown. | **Critical — Stability** |
| P0-3 | HTTP body limits | `ReadJSON` in [backend/internal/api/jsonutil.go](backend/internal/api/jsonutil.go) uses `DisallowUnknownFields` but **no `http.MaxBytesReader`**. An authenticated (or unauthenticated on `/api/auth/login`) client can POST gigabytes of JSON and exhaust memory/disk. | **Critical — DoS** |
| P0-4 | Auth / CSRF | No CSRF token. Cookie is `HttpOnly` + configurable `SameSite`, but default is `lax`, which still permits top-level `POST` form submissions and any request initiated by a user-gesture navigation to mount cross-origin state-changing attacks against JSON endpoints if the browser ever coerces a form-encoded submission. Combined with `AllowCredentials: true` in CORS, any lapse in `CORS_ORIGINS` config is immediately exploitable. | **High — Auth** |
| P0-5 | Auth brute-force | Login endpoint has **no rate limiting, no account lockout, no IP throttling, no captcha, no exponential backoff**. Bcrypt cost 12 slows an attacker but does not prevent credential stuffing against a known admin email. | **High — Auth** |
| P0-6 | Secrets at rest | Per-property Nuki API tokens, Booking.com ICS URLs (which embed the token), and Nuki plaintext PINs (`nuki_access_codes.generated_pin_plain`) are stored **in cleartext** in SQLite. A single file read (backup, stolen disk, misconfigured `/admin/backup` endpoint) leaks all smart-lock credentials and every active guest PIN. The existing audit ([PMS_04_Audit_Report_2026-04-13.md](spec/PMS_04_Audit_Report_2026-04-13.md)) already flagged this; remains unresolved. | **High — Confidentiality** |
| P0-7 | Metrics exposure | `/metrics` is served **without auth by default** (`PMS_METRICS_TOKEN` is optional). In production this leaks request volumes, paths, user-agent strings, scheduler timings, and runtime internals to any unauthenticated caller. | **High — Info disclosure** |
| P0-8 | Backups | Single-file SQLite at `./data/pms.db` with no scheduled off-host backup, no PITR, no replication. `/api/admin/backup` exists but is a **manual pull** by a super-admin. Loss of the volume = loss of the business. | **High — Durability** |

### 1.2 Top residual risks (P1)

- Missing security response headers (CSP, HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy) on both the API and the SPA shell.
- No rate limit on any endpoint (invoice PDF generation, attachment upload, occupancy export token probing).
- `/api/properties/{id}/occupancy-export` still accepts the token in the query string (deprecated with `Warning` header, but the surface remains) — tokens end up in proxy logs, browser history, and `Referer` headers.
- Admin backup endpoint leaks raw DB error strings to the client (`"database snapshot failed: "+err.Error()`).
- No password policy: `FIRST_SUPERADMIN_PASSWORD=change-me` in `.env.example` is valid at bootstrap; there is no minimum length, complexity, breach-corpus check, or rotation cadence.
- No session rotation on password change or privilege escalation; no "sign out all devices" primitive.
- No MFA / 2FA, no SSO/OIDC/SAML integration path.
- No structured JSON logging and no log level control — `log.Printf` to stdout only.
- No distinct liveness vs readiness probes (only `/healthz`); Kubernetes rollouts cannot gate on DB availability.
- No deployment artefacts in the repo: no Dockerfile, no Compose file, no systemd unit, no Helm chart, no CI workflow. The project ships as `make build` producing `bin/pms-server` + `frontend/dist`.

### 1.3 Top positive findings (keep doing)

- 100% parameterised SQL — grep for `fmt.Sprintf.*WHERE|ORDER|SELECT` returns zero hot spots.
- Access log middleware redacts sensitive query keys (`token|access_token|api_key|apikey|secret|password`) and sensitive headers (`Authorization`, `Cookie`, `X-Export-Token`) — see [backend/internal/middleware/accesslog.go](backend/internal/middleware/accesslog.go).
- Path-traversal guarded in invoice download via `resolveDataFilePath` → `filepath.Clean` + absolute-prefix check.
- SQLite is configured correctly for a single-writer app: WAL, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL`, `MaxOpenConns=8`.
- Scheduler goroutines use `store.TryAcquireJobLease(instanceID)` — safe to run multiple replicas.
- JSON decoder uses `DisallowUnknownFields`, surfacing typos and protecting against field-smuggling.
- `chimw.Recoverer` is installed, so a single panicking handler will not crash the process.
- Frontend never writes auth tokens to `localStorage` or `sessionStorage`; relies on `HttpOnly` cookie + `credentials: 'include'` — see [frontend/src/api/http.ts](frontend/src/api/http.ts).
- No `v-html`, `innerHTML`, or `eval` in the SPA — searched and clean.
- Nuki plaintext PIN is no longer returned on the list endpoint; reveal is gated on write-level permission and audited (H3 in `PMS_03`).
- Multipart uploads have per-handler caps (`25 MiB` / `20 MiB`).

---

## 2. Security findings (detailed)

### 2.1 Authentication & session management

**Observed:**
- Password hashing: `bcrypt` cost 12 ([backend/internal/auth/password.go](backend/internal/auth/password.go)). Good.
- Session token: 32 random bytes, SHA-256 hashed for storage ([backend/internal/auth/sessiontoken.go](backend/internal/auth/sessiontoken.go)). Good.
- Cookie: `HttpOnly=true`, `SameSite` configurable, `Secure` defaults to `true` outside dev/test; config validates `SameSite=none ⇒ Secure=true`. Good.
- Default TTL 168h (7 days). Acceptable.

**Gaps:**
1. **No CSRF defence other than SameSite.** Cookie auth + `CORS AllowCredentials: true` + JSON body (`application/json`) normally escapes classical CSRF because a cross-origin `fetch` with credentials is blocked by CORS preflight. However, the endpoints also accept `multipart/form-data` and `application/x-www-form-urlencoded` (finance attachments), which are *simple requests* and can be submitted cross-origin without preflight. **Recommendation:** either (a) issue a double-submit CSRF token for any state-changing request, (b) require a custom header such as `X-Requested-With: pms` on every mutating endpoint and reject requests without it (trivial CSRF mitigation), or (c) force `SameSite=strict` in production.
2. **No login throttling.** An attacker can iterate at ~1 attempt/s per core against bcrypt-12 from a single IP. At scale with a botnet, credential-stuffing is viable. **Recommendation:** per-IP + per-account leaky-bucket limiter (e.g. 5 attempts / 15 min), plus exponential backoff, plus audit-log alerting on N failures.
3. **No password policy and no breach-corpus check.** Any string is accepted at user-create / password-change. The bootstrap example ships `change-me`. **Recommendation:** enforce min 12 chars, one of: HaveIBeenPwned k-anonymity lookup, or zxcvbn score ≥ 3.
4. **No session rotation** on password change, role change, or privilege escalation. The previous cookie remains valid until TTL. **Recommendation:** invalidate all sessions on password change (`DELETE FROM auth_sessions WHERE user_id = ?`), and expose a "sign out everywhere" admin action.
5. **No MFA.** Single-factor, password-only admin plane. For a product that holds lock credentials and PINs — significant risk. **Recommendation:** TOTP-based 2FA (RFC 6238) for any user with write access to Nuki or admin/backup.
6. **`FIRST_SUPERADMIN_PASSWORD` via env.** OK for bootstrap if rotated immediately. Document this in the deployment runbook and force a password change on first login.
7. **Session cookie domain not set.** Defaults to host-only. Fine, but document it in the runbook so that subdomain rollouts do not silently share the cookie.

### 2.2 Authorisation

**Observed:** `permissions` package implements role + per-property module ACLs; `requirePropertyModuleAccess` is consistently called in handlers. `super_admin` bypasses property-level checks (correct). `property_access.go` centralises enforcement — good.

**Gaps:**
1. **No authorisation tests at the router level** for *every* handler. Rely on per-handler `requirePropertyModuleAccess` call — any future handler that forgets to call it will silently authorise. **Recommendation:** a table-driven test that, for every route under `/api/properties/{id}/…`, verifies that a user without that property's permission gets 403.
2. **Super-admin escape hatch for `/api/admin/backup`** returns raw SQLite error to the client on failure. Information leak. **Recommendation:** return generic `500 internal error`, log detail server-side.

### 2.3 Input validation / injection

- **SQL injection:** Not found. All store functions use `?` placeholders. Continue this discipline.
- **JSON:** `DisallowUnknownFields` + typed structs is good. **Missing:** `http.MaxBytesReader(r.Body, N)` on every `ReadJSON`. Add a 1 MiB cap globally; override where higher is needed.
- **Multipart:** 20–25 MiB cap per-handler. OK, but the cap is applied *after* `ParseMultipartForm` has already allocated. Prefer `r.Body = http.MaxBytesReader(w, r.Body, 26<<20)` before the parse.
- **Path traversal:** Invoice download path is cleaned + prefix-checked against `DATA_DIR`. Good. Verify the same pattern is applied to finance attachment downloads.
- **Attachment filename:** `saveFinanceAttachment` rejects `..` and `/`. Good, but does not re-encode Unicode right-to-left overrides or zero-width characters in filenames — low risk.
- **HTML injection / XSS on frontend:** No `v-html` or `innerHTML` found — Vue template interpolation auto-escapes. Safe.
- **Open redirect:** No `Location` header built from user input grep-hits — looks clean.

### 2.4 Transport & headers

**Observed:** No TLS in-process. Deployment presumably terminates TLS at a reverse proxy or load balancer. The audit log middleware redacts secrets. `Cache-Control: no-store` set on JSON responses. Good.

**Missing, all P1:**
- `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload`
- `Content-Security-Policy` (even a permissive `default-src 'self'` on the SPA shell is a meaningful upgrade)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY` (or CSP `frame-ancestors 'none'`)
- `Referrer-Policy: strict-origin-when-cross-origin` (or `no-referrer` for admin surfaces)
- `Permissions-Policy: camera=(), microphone=(), geolocation=()`

Add a single `securityHeaders` middleware wired in [backend/cmd/server/main.go](backend/cmd/server/main.go) before `chimw.Recoverer`.

### 2.5 Secrets management

**Observed:** App reads credentials from env vars. Per-property secrets stored in SQLite in plaintext.

**Gaps:**
1. **Nuki API token in cleartext.** One SQLite dump = all smart-lock credentials across all properties.
2. **Booking ICS URL in cleartext.** The URL contains the ICS export secret.
3. **Generated PIN in cleartext** (`nuki_access_codes.generated_pin_plain`). Already flagged in `PMS_03` and `PMS_04` as a tracked follow-up. For a property-management product with physical-access implications, this is **the single most important security issue** to resolve.

**Recommendation:**
- Introduce a master key (env `PMS_MASTER_KEY`, 32 random bytes, base64) and encrypt `nuki_api_token`, `booking_ics_url`, and `generated_pin_plain` with AES-256-GCM. Store key version with each ciphertext. Rotate via re-encrypt job.
- Better: use an HSM-backed KMS (cloud KMS, AWS KMS, GCP KMS, Vault Transit). The app calls `Encrypt`/`Decrypt` on each secret access.
- PINs should be stored **only as long as needed to surface once to the operator**, then purged.

### 2.6 Rate limiting / abuse protection

Nothing observed. All endpoints are unbounded:
- `/api/auth/login` — credential stuffing.
- `/api/properties/{id}/occupancy-export?token=…` — token-guessing brute force (opaque token, but no lockout).
- `/api/properties/{id}/invoices/…` — PDF generation is CPU-heavy (see `invoicepdf` package). An authenticated low-priv user could spin it in a loop.
- `/api/admin/backup` — `VACUUM INTO` copies the whole DB under a lock.

**Recommendation:** per-route token-bucket middleware (e.g. `golang.org/x/time/rate`) with keys `user_id`/`ip`/`property_id` as appropriate.

### 2.7 CORS

`AllowCredentials: true` + `AllowedOrigins: cfg.CORSOrigins` (default `http://localhost:5173`). A misconfigured `CORS_ORIGINS="*"` or a typo including `http://` vs `https://` would be immediately exploitable because credentials are included.

**Recommendation:** Reject `*` at config-parse time when `AllowCredentials=true`. Log the resolved origins on startup for audit.

### 2.8 Admin / backup endpoint

`/api/admin/backup` is super-admin-only, streams a tar.gz via `VACUUM INTO`. Concerns:
- Leaks raw DB error string on failure.
- Has no rate limiting → trivial disk-I/O DoS.
- Output is un-encrypted → anyone with a stolen `pms_session` cookie of a super_admin can exfil the whole DB (including secrets in cleartext — see §2.5).

### 2.9 Occupancy export token

- Cookie-less endpoint at `/api/properties/{id}/occupancy-export` accepts token via `Authorization: Bearer`, `X-Export-Token`, or query param (deprecated, emits `Warning` header).
- Token is SHA-256 hashed at rest. Good.
- **Missing:** per-token rate limit, per-token last-used timestamp display in UI (probably exists — verify), and enforced rotation cadence.

---

## 3. Stability findings (detailed)

### 3.1 HTTP server hardening (P0)

```go
// current — backend/cmd/server/main.go:200
if err := http.ListenAndServe(cfg.HTTPAddr, r); err != nil { … }
```

Required for production:

```go
srv := &http.Server{
    Addr:              cfg.HTTPAddr,
    Handler:           r,
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      60 * time.Second, // longer for invoice PDF
    IdleTimeout:       120 * time.Second,
    MaxHeaderBytes:    1 << 20,
}
go func() { _ = srv.ListenAndServe() }()
<-ctx.Done() // SIGTERM/SIGINT
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx)
```

And `signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)` wrapping the scheduler goroutines so they stop cleanly and `db.Close()` is called.

### 3.2 Graceful shutdown (P0)

Without it:
- SQLite WAL checkpoints can be interrupted, leaving a growing `-wal` file.
- Outbound Nuki API calls mid-flight return `context.Canceled` on next start-up look-up.
- Attachment upload can be killed mid-`io.Copy`, leaving half-written files on disk with a DB row pointing to them.

### 3.3 Process supervision

No systemd unit, no Docker `HEALTHCHECK`, no Kubernetes manifest. If the process crashes (e.g. out-of-memory from §2.3), nothing restarts it. **Recommendation:** ship a minimal systemd unit (`Restart=on-failure`, `RestartSec=5s`, `LimitNOFILE=65535`) **or** a Dockerfile with `CMD ["/pms-server"]` and document `--restart=unless-stopped`.

### 3.4 Panic handling

`chimw.Recoverer` handles per-request panics. **But scheduler goroutines in `main.go` have no recover** — a panic in the occupancy sync, nuki cleanup, or cleaning reconcile tick kills the scheduler goroutine silently until next process restart. Wrap each tick body in `func() { defer func() { if r := recover(); r != nil { log.Printf(...) } }(); … }()`.

### 3.5 Single point of failure — SQLite

Single file, single host. For the current scale this is a pragmatic choice, but:
- No high-availability option beyond restoring a backup.
- No read replicas.
- No online backup without `VACUUM INTO` lock window.

Options in increasing order of effort:
- **Cheap:** cron `sqlite3 pms.db '.backup /backup/pms-$(date).db'` every 15 min, rsync to off-host storage. Retain 30 days.
- **Better:** [Litestream](https://litestream.io) — streaming replication to S3/GCS, point-in-time restore with ~second granularity.
- **Enterprise:** migrate to Postgres when concurrent-writer contention or HA becomes a real constraint.

### 3.6 Observability

- Prometheus `/metrics` exposed (public by default — see §2). Good instrumentation primitive.
- Logs are unstructured `log.Printf` stdout. No request correlation ID, no JSON, no level. **Recommendation:** `log/slog` with JSON handler, include `request_id` from `chimw.RequestID`, per-request line enriched by the access-log middleware.
- No error reporting integration (Sentry, Rollbar, GlitchTip). Panics after recover are invisible.
- No distributed tracing (OpenTelemetry). Nuki/ICS outbound calls are not easily diagnosable.
- No alerting rules bundled. **Recommendation:** ship a starter `prometheus-alerts.yml` with: `http_requests_total 5xx rate > 1%`, `scheduler_last_success_age > 3 * interval`, `process_restart_count`, `sqlite_wal_size_bytes`.

### 3.7 Long-running requests

- Invoice PDF generation blocks the request goroutine. No timeout beyond server defaults (none configured — see §3.1). A slow client can tie up a handler until it disconnects.
- Occupancy ICS sync happens via scheduler and has `defaultSyncHTTPTimeout = 60s` + `maxICSBodyBytes = 20 MiB`. Good.
- Nuki client is bounded (`LimitReader(res.Body, 1<<20)`). Good.

### 3.8 Database connection pool

`MaxOpenConns=8` with SQLite WAL is fine for reads; writes serialise through the single writer. Verify that long-running read transactions (e.g. analytics queries) do not hold the writer via snapshot isolation under heavy write load.

---

## 4. Maintainability findings (detailed)

### 4.1 Configuration

`backend/internal/config/config.go` parses env vars ad-hoc. Validation is inline (`SameSite=none ⇒ Secure=true`). There is no machine-readable schema.

**Recommendations:**
- Add a `Validate()` method that fails fast on: `CORS_ORIGINS=*` with credentials, empty `DATA_DIR`, `SESSION_TTL_HOURS < 1`, weak `FIRST_SUPERADMIN_PASSWORD`, `PMS_MASTER_KEY` length.
- Emit the effective (redacted) config to the log on startup.
- Consider [`github.com/kelseyhightower/envconfig`](https://github.com/kelseyhightower/envconfig) or [`github.com/caarlos0/env`](https://github.com/caarlos0/env) for a single tagged struct.

### 4.2 API contract

No OpenAPI / Swagger definition in the repo. The frontend types under `frontend/src/api/types/` are hand-maintained. Drift is only caught by integration tests.

**Recommendations:** generate OpenAPI via [`github.com/swaggo/swag`](https://github.com/swaggo/swag) or [`github.com/oapi-codegen/oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen), wire code-gen for the TS types.

### 4.3 Error handling

- Handlers return `"database error"`, `"update failed"` as user-facing strings. Fine.
- A few places (`admin/backup`) leak raw error strings. Fix.
- There is no central error type, no `errors.Is`/`errors.As` hierarchy beyond `sql.ErrNoRows` matching. For a codebase of this size, it's acceptable. Watch for growth.

### 4.4 Logging

- Single-level, single-destination (`log.Printf` to stdout).
- Access log is good but tangled with business log.
- No `LOG_LEVEL` toggle.

### 4.5 Audit log

- Audit rows inserted via `s.audit(...)` in most mutating paths. Good coverage.
- `api_audit_logs` table has no retention policy. Unbounded growth.
- No external SIEM sink.

**Recommendations:** retention job (`DELETE FROM api_audit_logs WHERE created_at < now - 365 days`), plus an optional ship-to-syslog or ship-to-S3 export.

### 4.6 Migrations

`backend/internal/migrate/migrate.go` is a clean embed-fs forward-only runner with `schema_migrations` table. Down-migrations are present but not executed. Good minimal choice. No version-lock on concurrent startup — with multiple replicas, first-writer wins via SQLite file lock; acceptable.

**Concern:** there is no transaction around the `INSERT INTO schema_migrations` versus the migration DDL for SQLite — actually there *is* a `tx := db.Begin()`. Good. But SQLite DDL is auto-committed in some dialects — verify with `modernc.org/sqlite`.

### 4.7 Test coverage

- Backend: substantial — server_test.go, per-feature tests, integration-style through `httptest`. Good.
- Frontend: 49 files / 251 tests. Strong component-level coverage. No end-to-end (Playwright/Cypress) tests — the SPA + backend integration is only covered by unit tests.

**Recommendation:** add a lightweight E2E suite covering the critical login → create property → book → generate invoice → download PDF path. One headless Chromium run in CI.

### 4.8 CI/CD

No `.github/workflows`, no `.gitlab-ci.yml`, no `Jenkinsfile`. Build is manual (`make build`). Deploy is undocumented.

**Recommendations:**
- CI: lint (`go vet`, `staticcheck`, `gosec`, `eslint`), test (`go test ./...`, `npm test`), build, run `govulncheck`.
- CD: tag-triggered container build, SBOM generation (`syft`), signed artefacts (`cosign`).

### 4.9 Dependency hygiene

- Go module is clean (`modernc.org/sqlite` is pure-Go → no CGO → easy static build).
- Frontend pins Vite 6, Vue 3.5, Pinia 2, Vitest 2.1.9. Reasonable.
- No automated vulnerability scanning (`govulncheck`, `npm audit`, Dependabot/Renovate).

---

## 5. Infrastructure findings

### 5.1 TLS

Assumed to be terminated upstream (nginx / Caddy / Cloudflare / k8s Ingress). The app itself has no `ListenAndServeTLS` path. Acceptable, but **document it in a `docs/deployment.md`** and ensure:
- HSTS header is added (either by the app or the proxy).
- The app trusts `X-Forwarded-For` / `X-Forwarded-Proto` only from the proxy (for correct `r.RemoteAddr` in access log and `Secure` cookie detection in dev-over-proxy).

### 5.2 Filesystem layout

`DATA_DIR` holds `pms.db`, `invoices/<property_id>/…`, `attachments/<property_id>/<transaction_id>/…`. Single volume. Permissions not enforced by the app.

**Recommendations:**
- Document required mode 0700 on `DATA_DIR`.
- Document that `DATA_DIR` must be on the same filesystem as the process writable tmp (SQLite WAL).
- Mount as a separate volume in container deployment so that `docker rm` does not wipe the DB.

### 5.3 Container story

No Dockerfile. A minimal production-ready one (for reference):

```dockerfile
# stage 1: build backend
FROM golang:1.22-alpine AS be
WORKDIR /src
COPY backend/ .
RUN go build -trimpath -ldflags="-s -w" -o /out/pms-server ./cmd/server/

# stage 2: build frontend
FROM node:20-alpine AS fe
WORKDIR /src
COPY frontend/ .
RUN npm ci && npm run build

# stage 3: runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=be /out/pms-server /pms-server
COPY --from=fe /src/dist /ui
VOLUME ["/data"]
ENV DATA_DIR=/data DATABASE_PATH=/data/pms.db
USER nonroot
ENTRYPOINT ["/pms-server"]
```

Ship this + a Compose file that fronts with Caddy for automatic TLS.

### 5.4 Probes

`/healthz` returns `200 OK` unconditionally (verify). Kubernetes needs:
- **Liveness:** `/healthz` — process up.
- **Readiness:** `/readyz` — DB ping OK + last scheduler run within N × interval.

### 5.5 Multi-replica

Job leases via `TryAcquireJobLease(instanceID)` make scheduler multi-replica-safe. However:
- Session cookie is not sticky — fine, stateless after the cookie.
- SQLite does not support multi-writer from multiple processes across a network filesystem. **Multi-replica is only safe on the same host sharing the volume** (which defeats the point). Acknowledge: current architecture is effectively single-node. If HA is required, migrate persistence.

### 5.6 Secrets in env

- `.env.example` is fine as a template.
- `FIRST_SUPERADMIN_PASSWORD` in shell env: remember that `ps -ef` and `/proc/<pid>/environ` leak it to any user on the host. Prefer a one-shot bootstrap flag or a file mount.
- `PMS_METRICS_TOKEN`, future `PMS_MASTER_KEY` — same caveat. Use Docker/Kubernetes secrets, not plain env.

### 5.7 Time & timezone

Per-property `time.LoadLocation(prop.Timezone)` with fallback to UTC. Handlers that fail to load a zone fall back silently — OK. Ensure the container image has `tzdata` (distroless base does by default).

---

## 6. Frontend-specific findings

1. **No CSP.** `index.html` ships no `<meta http-equiv="Content-Security-Policy">`. Add a strict policy (`default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:;`) or set it from the server.
2. **No Subresource Integrity.** If/when assets are CDN-hosted, add SRI hashes to the built HTML.
3. **No service-worker, no PWA concerns** — simplifies the threat model. Good.
4. **Cookie `Secure` in dev.** `PMS_ENV=dev` flips `Secure=false`. Correct. Ensure prod CI gates on `PMS_ENV=production`.
5. **Error handling in `api/http.ts`** swallows non-JSON error bodies as `{ error: text }`. Minor: an HTML error page from an upstream reverse proxy will surface as a giant error string in the UI. Harmless but ugly.
6. **No explicit logout on 401.** Verify that the Pinia auth store redirects to `/login` and clears user state when `api` throws `401 Unauthorized`. (Not inspected in this review.)

---

## 7. Prioritised remediation roadmap

### P0 — Pre-production blockers

- [ ] **Wrap `http.ListenAndServe` in `http.Server` with timeouts + graceful shutdown** (§3.1, §3.2).
- [ ] **Add `http.MaxBytesReader` globally** in `ReadJSON` (1 MiB default, override per-route) (§2.3).
- [ ] **Require a custom header (`X-Requested-With: pms`) on every mutating JSON endpoint** *or* force `SameSite=strict` in production (§2.1 gap 1).
- [ ] **Add login rate limiting** (per-IP + per-email leaky bucket) (§2.1 gap 2).
- [ ] **Encrypt at rest: Nuki API token, Booking ICS URL, `generated_pin_plain`** using a master key (env or KMS) (§2.5).
- [ ] **Default-secure `/metrics`** — require `PMS_METRICS_TOKEN` *or* bind it to a separate internal listener (§2.4, §3.6).
- [ ] **Automated off-host backups** (Litestream → S3 or `sqlite3 .backup` + rsync + retention) (§3.5).
- [ ] **Recover-wrap scheduler goroutines** (§3.4).

### P1 — Within first month in production

- [ ] **Security response headers middleware** (HSTS, CSP, X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy) (§2.4).
- [ ] **Structured JSON logging with `slog`**, request_id propagation, LOG_LEVEL env (§3.6).
- [ ] **Error reporting integration** (Sentry/GlitchTip) (§3.6).
- [ ] **Password policy** (min 12 chars + breach check) (§2.1 gap 3).
- [ ] **Session rotation on password/role change + "sign out everywhere"** (§2.1 gap 4).
- [ ] **Rate limiting on invoice-generation and attachment endpoints** (§2.6).
- [ ] **Remove the query-string occupancy token** after SDKs have migrated (§2.9).
- [ ] **`/readyz` distinct from `/healthz`** with DB ping + scheduler freshness (§5.4).
- [ ] **Dockerfile + Compose + TLS proxy + systemd unit** in the repo (§5.3).
- [ ] **CI workflow** with lint / test / vuln-scan / build (§4.8).
- [ ] **`api_audit_logs` retention job** (§4.5).
- [ ] **`downloadInvoice` audit ordering bug check:** audit is called *after* `http.ServeFile` writes the response — if the write fails, the audit still records `"success"`. Minor.

### P2 — Within first quarter

- [ ] **MFA (TOTP)** for write-level and admin users (§2.1 gap 5).
- [ ] **OpenAPI spec + generated TS types** (§4.2).
- [ ] **E2E test suite** (Playwright) covering the critical path (§4.7).
- [ ] **Alerting rules bundle** + runbook (§3.6).
- [ ] **OpenTelemetry traces** on outbound Nuki and ICS calls (§3.6).
- [ ] **Renovate/Dependabot** config (§4.9).
- [ ] **Consider Postgres migration** if concurrency/HA becomes a constraint (§3.5).
- [ ] **SSO/OIDC** option for enterprise customers.

---

## 8. Appendix — Files inspected

- [backend/cmd/server/main.go](backend/cmd/server/main.go)
- [backend/internal/config/config.go](backend/internal/config/config.go)
- [backend/internal/auth/password.go](backend/internal/auth/password.go)
- [backend/internal/auth/sessiontoken.go](backend/internal/auth/sessiontoken.go)
- [backend/internal/middleware/auth.go](backend/internal/middleware/auth.go)
- [backend/internal/middleware/accesslog.go](backend/internal/middleware/accesslog.go)
- [backend/internal/api/server.go](backend/internal/api/server.go)
- [backend/internal/api/jsonutil.go](backend/internal/api/jsonutil.go)
- [backend/internal/api/invoice_handlers.go](backend/internal/api/invoice_handlers.go)
- [backend/internal/api/finance_handlers.go](backend/internal/api/finance_handlers.go)
- [backend/internal/api/nuki_handlers.go](backend/internal/api/nuki_handlers.go)
- [backend/internal/api/admin_backup.go](backend/internal/api/admin_backup.go)
- [backend/internal/api/property_access.go](backend/internal/api/property_access.go)
- [backend/internal/api/occupancy_handlers.go](backend/internal/api/occupancy_handlers.go)
- [backend/internal/dbconn/dbconn.go](backend/internal/dbconn/dbconn.go)
- [backend/internal/migrate/migrate.go](backend/internal/migrate/migrate.go)
- [backend/internal/store/store.go](backend/internal/store/store.go)
- [backend/internal/store/nuki.go](backend/internal/store/nuki.go)
- [backend/internal/nuki/client.go](backend/internal/nuki/client.go)
- [backend/internal/nuki/service.go](backend/internal/nuki/service.go)
- [backend/internal/occupancy/sync.go](backend/internal/occupancy/sync.go)
- [backend/.env.example](backend/.env.example)
- [Makefile](Makefile)
- [frontend/src/api/http.ts](frontend/src/api/http.ts)
- [frontend/index.html](frontend/index.html)

Complementary grep sweeps:
- `ListenAndServe|ReadTimeout|WriteTimeout|Shutdown` — one hit, no timeouts configured.
- `MaxBytesReader|ParseMultipartForm` — `MaxBytesReader` absent; multipart caps per-handler.
- `X-Frame-Options|Content-Security-Policy|X-Content-Type-Options|Strict-Transport-Security` — zero hits.
- `CSRF|csrf` — zero hits.
- `rate.?limit|ratelimit` — zero hits.
- `fmt.Sprintf(...WHERE|ORDER|SELECT|...)` — zero hits (no SQLi hot spots).
- `v-html|innerHTML|eval\(` — zero hits in `frontend/`.
- `localStorage|sessionStorage.*(token|secret|pin)` — zero hits.

— *End of PMS_10.*
