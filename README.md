# PMS — Property Management System

PMS is a self-hosted, multi-tenant management system for short-term rental
properties. It consolidates the day-to-day operations that an owner or
co-host otherwise spreads across spreadsheets, calendars, locker apps and
ad-hoc notes:

- **Occupancy** — pulls iCal feeds from Booking.com / Airbnb / direct
  channels and reconciles them into a single timeline per property.
- **Access management** — issues, rotates and revokes Nuki keypad codes
  per stay, with automatic expiry windows.
- **Cleaning** — schedules, logs and analyses cleaner activity, plus
  per-property fees, adjustments and payouts.
- **Finance** — tracks bookings, payouts, expenses and platform fees;
  reconciles imported booking-payout reports against actual stays.
- **Invoicing** — generates compliant PDF invoices with sequential numbers,
  configurable templates and per-property branding.
- **Messaging** — produces guest-ready message templates from stay data
  (check-in, parking, Wi-Fi, cleaning fees, etc.).
- **Analytics** — occupancy %, ADR, RevPAR, cleaning load and revenue
  breakdowns by property / month / channel.

The reference deployment runs as three small containers (Caddy → nginx →
Go API) on a single VPS. SQLite is the default datastore; the schema and
queries are written so a future migration to PostgreSQL is mechanical.

**Highlights**

- Cookie sessions with TOTP 2FA (mandatory for `super_admin`).
- Field-level encryption for OAuth / API tokens stored in the database.
- Forced password rotation for the bootstrap account.
- CSRF defence-in-depth (header + Origin allowlist).
- Per-IP rate limiting that works correctly behind a reverse proxy.
- Automatic nightly encrypted backups + on-demand admin backup endpoint.
- Structured JSON logs, Prometheus `/metrics`, optional OpenTelemetry
  traces and Sentry error reporting.

## Repository layout

```
backend/    Go API server (chi, modernc.org/sqlite, no CGO)
frontend/   Vue 3 + Vite SPA (TypeScript, Pinia)
deploy/     Dockerfiles, docker-compose, Caddyfile, .env.example, systemd unit
docs/       Architecture decision records and the deployment runbook
e2e/        Playwright end-to-end suite
spec/       Product specification documents
```

## Quick start (Docker Compose)

Prerequisites: Docker ≥ 24, a domain pointing at the host, ports 80/443
open. The full procedure (first-boot checklist, secret generation,
disaster-recovery drill, secret rotation, monitoring) is in
[docs/deployment/README.md](docs/deployment/README.md).

```bash
git clone <repo> pms && cd pms

# 1. Generate strong secrets and write them into deploy/.env
cp deploy/.env.example deploy/.env
echo "SESSION_SECRET=$(openssl rand -hex 32)"     >> deploy/.env
echo "DB_ENCRYPTION_KEY=$(openssl rand -base64 32)" >> deploy/.env
# edit deploy/.env to set PMS_DOMAIN, CORS_ORIGINS, FIRST_SUPERADMIN_*

# 2. Build and boot
cd deploy && docker compose up -d --build

# 3. Wait for the backend to report healthy
docker compose ps
```

On first login PMS forces the bootstrap super-admin to rotate the
temporary password and enrol in 2FA before any other endpoint becomes
reachable. Follow the **First-boot checklist** in
[docs/deployment/README.md](docs/deployment/README.md) for the full
sequence.

For other deployment shapes — systemd binary, static frontend + your own
backend host, or a fully manual build — see the same document.

## Container images

Pre-built multi-arch (`linux/amd64`, `linux/arm64`) images are published to
GitHub Container Registry on every push to `main` and on every `vX.Y.Z`
tag by the [`Publish images`](.github/workflows/publish-images.yml)
workflow:

- `ghcr.io/<owner>/<repo>-backend:<tag>`
- `ghcr.io/<owner>/<repo>-frontend:<tag>`

Available tags:

| Tag                | Source                                    |
| ------------------ | ----------------------------------------- |
| `latest`           | latest push to `main` and latest `vX.Y.Z` |
| `main`             | every push to `main`                      |
| `sha-<short>`      | every commit                              |
| `vX.Y.Z` / `vX.Y` / `vX` | semver tag pushes                   |

To consume the published images instead of building locally, replace the
`build:` blocks in [`deploy/docker-compose.yml`](deploy/docker-compose.yml)
with `image: ghcr.io/<owner>/<repo>-backend:latest` (and the same for
the frontend), then `docker compose pull && docker compose up -d`.

### One-time repository setup

1. **Make the repository public**, or grant the deployment host a PAT
   with the `read:packages` scope. By default GHCR images inherit the
   repository's visibility — pushing from a private repo creates private
   packages.
2. **Allow GitHub Actions to write packages**: *Settings → Actions →
   General → Workflow permissions → Read and write permissions*. The
   workflow already requests `packages: write`, but the repo-level
   toggle must permit it.
3. **(Optional) Make the package public** after the first successful
   run: open https://github.com/users/&lt;owner&gt;/packages/container/&lt;repo&gt;-backend
   → *Package settings* → *Change visibility* → *Public*. This lets
   `docker pull` work without authentication.

### Cutting a release

```bash
git tag v1.2.3
git push origin v1.2.3
```

The workflow picks up the tag, runs both image builds in parallel
(`linux/amd64` + `linux/arm64`), pushes to GHCR with provenance + SBOM
attestations and updates the `latest`, `1`, `1.2`, `1.2.3` tags.

## Static frontend bundle

The same workflow also packages the built SPA as
`pms-frontend-<version>.tar.gz` (with a `.sha256` checksum). Use this if
you want to host the frontend on S3, Cloudflare Pages, GitHub Pages,
nginx, Apache, or any other static host while running the backend
elsewhere.

- **On tagged releases** (`vX.Y.Z`): the tarball is attached to the
  GitHub Release page automatically.
- **On every push to `main`**: it is uploaded as a workflow artefact
  (retained for 30 days). Open the run in *Actions → Publish images →
  Frontend static bundle* and download `pms-frontend-<branch>-<sha>`.

Deploying the bundle:

```bash
curl -LO https://github.com/<owner>/<repo>/releases/download/v1.2.3/pms-frontend-v1.2.3.tar.gz
curl -LO https://github.com/<owner>/<repo>/releases/download/v1.2.3/pms-frontend-v1.2.3.tar.gz.sha256
sha256sum -c pms-frontend-v1.2.3.tar.gz.sha256
tar -xzf pms-frontend-v1.2.3.tar.gz       # extracts to ./dist/
# (optional) point the SPA at a backend on a different origin
#   $EDITOR dist/config.js   # set apiBaseUrl: 'https://api.example.com'
# upload ./dist/* to your static host
```

The bundle ships with a `dist/config.js` that holds the runtime
configuration (currently just `apiBaseUrl`). Leave it empty for the
recommended same-origin setup (the static host reverse-proxies `/api/*`
to the backend) or edit it in place to point at a backend on a
different origin — no rebuild required, the change takes effect on the
next browser refresh. Cross-origin deployments also need
`CORS_ORIGINS`, `PMS_COOKIE_SAMESITE=none`, and `PMS_COOKIE_SECURE=true`
on the backend; see
[docs/deployment/README.md](docs/deployment/README.md#3-static-frontend--your-own-infra)
for the full checklist.

## Development

### Prerequisites

- Go 1.26+ (go.mod pins the minimum; `go.work` not used)
- Node.js 20+ and npm 10+
- `make` (every common task is wrapped)
- SQLite is bundled via `modernc.org/sqlite`; no system library needed.

### One-time setup

```bash
make setup            # `npm install` + `go mod tidy`
```

### Run the stack locally

In two terminals:

```bash
# Terminal 1 — API on :8080 (creates ./data/pms.db on first run)
FIRST_SUPERADMIN_EMAIL=dev@example.com \
FIRST_SUPERADMIN_PASSWORD=dev-password-1234 \
SESSION_SECRET=$(openssl rand -hex 32) \
DB_ENCRYPTION_KEY=$(openssl rand -base64 32) \
make backend-run

# Terminal 2 — SPA dev server on :5173, proxies /api/* to :8080
make frontend-dev
```

Visit http://localhost:5173.

For a faster developer loop you can set `PMS_2FA_DEV_BYPASS=true` (only
honoured when `PMS_ENV` ∈ `{dev,development,test}`).

### Common make targets

| Target              | What it does                                  |
| ------------------- | --------------------------------------------- |
| `make setup`        | Install all deps                              |
| `make backend-run`  | Run the Go API server with SQLite on `:8080` |
| `make backend-test` | `go test ./...`                              |
| `make backend-build`| Build a static binary into `./bin/pms-server`|
| `make frontend-dev` | Vite dev server on `:5173` with API proxy    |
| `make frontend-test`| Vitest run (~250 specs)                      |
| `make frontend-build`| Production SPA bundle into `frontend/dist/` |
| `make test`         | Backend + frontend tests                     |
| `make build`        | Backend binary + frontend bundle             |
| `make fmt`          | `go fmt ./...`                               |

## Testing

Run everything before opening a PR:

```bash
make test                                # unit + component tests
cd e2e && npm install && npm test        # Playwright (requires the SPA)
cd backend && $(go env GOPATH)/bin/govulncheck ./...   # Go vuln scan
cd frontend && npm audit --omit=dev      # SPA prod-tree audit
```

CI runs the same matrix on every PR (`.github/workflows/ci.yml`).

## Contributing

1. **Branch from `main`** and keep PRs small and topical.
2. **Read the spec** for the area you are touching:
   - Architecture and cross-cutting rules: [spec/PMS_01_Architecture_and_Global_Spec.md](spec/PMS_01_Architecture_and_Global_Spec.md)
   - Per-module behaviour: [spec/PMS_02_Module_Specifications.md](spec/PMS_02_Module_Specifications.md)
   - Implementation checklists: [spec/PMS_03_Implementation_Checklists.md](spec/PMS_03_Implementation_Checklists.md)
   - Architecture decisions: [docs/adr/](docs/adr/)
3. **Add a migration** for any schema change. Migrations live in
   [backend/internal/migrate/](backend/internal/migrate/) as numbered
   pairs (`NNNNNN_name.up.sql` / `.down.sql`). Never edit a shipped
   migration; add a new one.
4. **Write tests** alongside the code. Backend handlers must have
   unit + integration coverage; the frontend uses Vitest with Vue Test
   Utils. End-to-end coverage for cross-page flows belongs in `e2e/`.
5. **Run** `make test` and `make fmt` before pushing. Lint failures
   block CI.
6. **Update the docs** when behaviour changes — the deployment runbook
   in particular is a release-blocker for ops-visible changes.
7. **Commit messages**: short imperative subject, optional body
   explaining *why* (not *what*).

### Reporting security issues

Please do **not** open a public issue for security findings. Email the
maintainer (see repository metadata) with reproduction steps and we will
coordinate disclosure.

## License

Released under the [MIT License](LICENSE).
