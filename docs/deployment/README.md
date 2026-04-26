# PMS deployment guide

Four supported layouts, in order of simplicity:

1. **Docker Compose + Caddy** — one host, automatic TLS, zero config beyond DNS. Recommended.
2. **systemd binary** — single host, no container runtime.
2b. **Standalone backend container** (`podman run` / `docker run`) — just the API, no frontend, no compose. Useful when the SPA lives elsewhere.
3. **Static frontend + your own infra** — ship `frontend/dist` to any static host (S3, nginx, Apache, Cloudflare Pages, GitHub Pages behind a proxy, …) and point it at a separately hosted backend.
4. **Manual Go build** — local development or custom orchestration.

All layouts share the same env vars. The canonical, exhaustive list with
defaults and inline comments lives in
[`deploy/.env.example`](../../deploy/.env.example) — copy it and edit in
place rather than rebuilding the file from snippets in this doc. Sections
below only highlight the variables that *change meaning* between layouts
(e.g. `PMS_TRUSTED_PROXY`, `PMS_API_BASE_URL`, `PMS_COOKIE_SAMESITE`).

## 1. Docker Compose (recommended)

Prerequisites: Docker ≥ 24, a domain pointing at the host, ports 80/443 open.

The stack is three containers:

- `pms-backend` — Go binary serving the JSON API on :8080 (internal only).
- `pms-frontend` — nginx serving the built SPA on :80 and reverse-proxying `/api/*` to `pms-backend`.
- `caddy` — TLS termination and HSTS, reverse-proxying to `pms-frontend`.

```bash
git clone <repo> pms && cd pms
cp deploy/.env.example deploy/.env
# edit deploy/.env with real values (secrets ≥12 chars, base64 key, etc.)
cd deploy
docker compose build
docker compose up -d
```

Caddy will obtain a TLS certificate on first request. Logs:

```bash
docker compose logs -f pms-backend pms-frontend caddy
```

You can also build the two images independently:

```bash
docker build -f deploy/Dockerfile.backend  -t pms-backend  .
docker build -f deploy/Dockerfile.frontend -t pms-frontend .
```

Data lives in the `pms-data` named volume. Back it up with:

```bash
docker run --rm -v pms_pms-data:/data -v $PWD:/backup alpine \
    tar czf /backup/pms-$(date +%F).tgz -C /data .
```

## 2. systemd

Prerequisites: Linux host, Go 1.25 build toolchain on a build box (or CI
artefact), ports 80/443 behind a reverse proxy (nginx, Caddy, ELB, …).

```bash
# on a build host
cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o pms-server ./cmd/server
scp pms-server user@host:/tmp/

# on the target
sudo useradd -r -s /usr/sbin/nologin -d /var/lib/pms pms
sudo install -d -o pms -g pms -m 750 /var/lib/pms /etc/pms
sudo install -m 640 -o root -g pms deploy/.env.example /etc/pms/pms.env
sudo $EDITOR /etc/pms/pms.env   # fill in real values
sudo install -m 755 /tmp/pms-server /usr/local/bin/pms-server
sudo cp deploy/systemd/pms-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pms-server
sudo journalctl -u pms-server -f
```

The unit is locked down (`ProtectSystem=strict`, `NoNewPrivileges`, seccomp
filter). Writable paths are limited to `/var/lib/pms`.

## 2b. Standalone backend container (Podman / `docker run`)

Use this when you only want the API container — for example, you already
host the SPA elsewhere (S3, Cloudflare Pages, the static tarball on
nginx) and you just need somewhere to run `pms-server` with a persistent
volume. No compose, no frontend container, no Caddy.

The published image is a distroless static binary running as `nonroot`
(uid/gid 65532), so it works the same with rootless Podman and Docker.

```bash
# 1. Pull the published image (or build locally with podman build -f deploy/Dockerfile.backend).
podman pull ghcr.io/<owner>/<repo>-backend:latest

# 2. Persistent volume for the SQLite DB, audit logs, invoice PDFs.
podman volume create pms-data

# 3. Start from the canonical env template and edit in place. This file
#    contains every supported variable (secrets, observability, OTel,
#    Nuki, audit retention, 2FA issuer, …) with comments — do NOT
#    cherry-pick a hand-rolled subset.
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/deploy/.env.example -o pms.env
# Or, if you cloned the repo: cp deploy/.env.example pms.env
$EDITOR pms.env
chmod 600 pms.env

# 4. Generate the secrets the file asks for:
openssl rand -hex 32       # paste into SESSION_SECRET
openssl rand -base64 32    # paste into DB_ENCRYPTION_KEY

# 5. For this topology specifically, set:
#    PMS_TRUSTED_PROXY=true   if a reverse proxy fronts the API
#                              (so per-IP rate limiting keys on X-Forwarded-For)
#    PMS_TRUSTED_PROXY=false  if the container is the public ingress
#    PMS_API_BASE_URL is not used by the backend itself (it's read by
#    the frontend container); ignore it here.
#    PMS_COOKIE_SAMESITE=none + PMS_COOKIE_SECURE=true if the SPA lives
#    on a different origin (see section 3b).

# 6. Run.
podman run -d --name pms-backend \
    --restart=unless-stopped \
    --read-only --tmpfs /tmp \
    --cap-drop=ALL --security-opt=no-new-privileges \
    -p 127.0.0.1:8080:8080 \
    -v pms-data:/data:Z \
    --env-file ./pms.env \
    --health-cmd '/app/pms-healthcheck' \
    --health-interval=30s --health-timeout=5s --health-retries=3 \
    ghcr.io/<owner>/<repo>-backend:latest
```

A few container-specific notes:

- `-p 127.0.0.1:8080:8080` keeps the API loopback-only. Put nginx, Caddy,
  or an ALB in front to terminate TLS and forward `/api/*` (or all paths)
  to it. If you want to expose it directly, use `-p 8080:8080` and run
  the backend behind something that handles HTTPS — never publish the
  raw HTTP port to the internet.
- `:Z` on the volume mount is the SELinux relabel hint and is harmless
  on systems without SELinux (Docker on Ubuntu/macOS). On Podman with
  SELinux enabled it is mandatory.
- The image already declares a `HEALTHCHECK` running `/app/pms-healthcheck`,
  but `podman run --health-cmd` re-asserts it for clarity and so
  `podman healthcheck run pms-backend` works.
- The first start runs migrations and creates the bootstrap super-admin
  account. The user must change the password and (for super-admins)
  enrol TOTP on first login — see the *First-boot checklist* below.

To replace the binary on a new release:

```bash
podman pull ghcr.io/<owner>/<repo>-backend:latest
podman stop pms-backend && podman rm pms-backend
# re-run the `podman run` command above
```

The data volume is reused; migrations run on every boot and are idempotent.

For systemd-managed Podman, generate a unit with:

```bash
podman generate systemd --new --name pms-backend > ~/.config/systemd/user/pms-backend.service
systemctl --user enable --now pms-backend.service
loginctl enable-linger $USER   # keeps the unit running after logout
```

## 3. Static frontend + your own infra

If you already have a way to serve static files (S3 + CloudFront, nginx,
Apache, Caddy on a shared host, Cloudflare Pages, etc.), you only need
the built SPA bundle and somewhere to run the `pms-server` binary.

There are **two supported topologies** — pick whichever matches your
infrastructure. The choice is purely operational; both are equally
secure when configured correctly.

| Mode | SPA host | API host | Cookie scope | Browser sees |
| ---- | -------- | -------- | ------------ | ------------ |
| **3a — Same-origin** (recommended) | `pms.example.com` | `pms.example.com` (proxied to backend) | `SameSite=Lax` | one origin |
| **3b — Cross-origin** | `app.example.com` | `api.example.com` | `SameSite=None; Secure` | two origins, CORS preflight |

### Build the bundle

The SPA reads its API base URL from `dist/config.js` at runtime, so a
**single build works for both topologies**. You only need to override
`VITE_API_BASE_URL` at build time if you want to bake a default into the
bundle (e.g. for an immutable CDN deployment where editing files
post-upload is awkward).

```bash
cd frontend
npm ci
npm run build
# output: frontend/dist/  (static HTML/JS/CSS only, no Node runtime needed)
```

Or, from the repo root:

```bash
make frontend-build
```

After uploading, point the SPA at the right backend by editing
`dist/config.js` on the static host:

```js
// dist/config.js
window.__PMS_CONFIG__ = {
  apiBaseUrl: '',                            // mode 3a: same-origin (default)
  // apiBaseUrl: 'https://api.example.com',  // mode 3b: cross-origin
}
```

The change takes effect on the next browser refresh. If you prefer to
bake the value in at build time instead, set
`VITE_API_BASE_URL=https://api.example.com` before `npm run build`;
runtime config (`config.js`) still wins if both are set.

Upload the contents of `frontend/dist/` to your static host.

### 3a. Same-origin (SPA and API share a hostname)

The static host serves the SPA and proxies `/api/*` to the backend on
the same hostname. Browsers see a single origin, so the session cookie
remains `SameSite=Lax` and no CORS preflight is required. This is the
simplest option and the default the SPA assumes.

Backend env:

```dotenv
CORS_ORIGINS=https://pms.example.com
# PMS_COOKIE_SAMESITE defaults to "lax" — leave unset.
# PMS_COOKIE_SECURE   defaults to true in production — leave unset.
```

Reverse-proxy snippets:

```nginx
server {
    listen 443 ssl;
    server_name pms.example.com;

    root /var/www/pms;
    index index.html;

    # Long-cache hashed assets; never cache index.html.
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    location = /index.html {
        add_header Cache-Control "no-store";
    }

    # API
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 25m;
    }

    # SPA history fallback
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

```caddy
pms.example.com {
    root * /var/www/pms
    try_files {path} /index.html
    file_server
    handle /api/* {
        reverse_proxy 127.0.0.1:8080
    }
}
```

Run the backend using layout #2 (systemd) or any process supervisor.
The static host does not need Node at runtime.

### 3b. Cross-origin (SPA and API on different domains)

Use this when the API runs on its own hostname — typically `app.example.com`
serving the SPA and `api.example.com` serving the backend. The SPA reads
the API origin from `dist/config.js` at runtime; alternatively you can
bake it in at build time via `VITE_API_BASE_URL=https://api.example.com`.

Required configuration:

| Setting | Value | Why |
| ------- | ----- | --- |
| `dist/config.js` `apiBaseUrl` | `https://api.example.com` | Sends API calls to the right origin (or use `VITE_API_BASE_URL` at build time). The bundled CSP is built from this value at runtime, so `connect-src` no longer has to be edited by hand. |
| `CORS_ORIGINS` | `https://app.example.com` | Backend allows the SPA's origin and emits the CORS headers needed for credentialed requests. |
| `PMS_COOKIE_SAMESITE` | `none` | Browsers refuse to send `SameSite=Lax` cookies on cross-site fetches. |
| `PMS_COOKIE_SECURE` | `true` | Required by browsers whenever `SameSite=None`. Both hosts MUST be HTTPS. |

Backend env (`.env` on the API host):

```dotenv
CORS_ORIGINS=https://app.example.com
PMS_COOKIE_SAMESITE=none
PMS_COOKIE_SECURE=true
```

The SPA host is just a static-file host — no `/api/*` proxy needed.
A minimal nginx config for `app.example.com`:

```nginx
server {
    listen 443 ssl;
    server_name app.example.com;

    root /var/www/pms;
    index index.html;

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    location = /index.html {
        add_header Cache-Control "no-store";
    }
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

The API host (`api.example.com`) terminates TLS and forwards to
`pms-server:8080` using whichever proxy you prefer; no additional rules
are required beyond preserving the `Host`, `X-Forwarded-For` and
`X-Forwarded-Proto` headers (the same headers used in 3a).

> **Why mode 3a is recommended.** Cross-origin deployment works, but it
> exposes the cookie to a wider browser/network surface (a misconfigured
> proxy that strips `Set-Cookie` on cross-site responses, third-party
> cookie blocking in some browsers, the need for both hosts to be HTTPS).
> Same-origin avoids all of this.

## 4. Manual / local development

See the repo root `Makefile`:

```bash
make backend-test frontend-test
make backend-run       # listens on :8080
make frontend-dev      # Vite dev server on :5173 with /api proxy
```

## Health probes

- `GET /healthz` — always 200, does not touch the database.
- `GET /readyz` — 200 when the database is pingable, 503 otherwise (2s timeout).

Point container/kubelet liveness probes at `/healthz` and readiness at `/readyz`.

## Secret rotation

- `SESSION_SECRET`: bump the value and redeploy — all sessions invalidated.
- `DB_ENCRYPTION_KEY`: **never rotate in place**; migrate through a script (out of scope).
- `FIRST_SUPERADMIN_PASSWORD`: only used during bootstrap. Change the password through the UI afterwards.

## Audit log retention

Controlled by `AUDIT_LOG_RETENTION_DAYS` (default 365). Rows older than the
window are deleted by the in-process scheduler once per day. Set `0` or a
negative value to disable pruning.

## Monitoring

- `/metrics` serves Prometheus-style counters.
- Structured JSON logs are emitted on stdout when `PMS_LOG_FORMAT=json`.
- Set `SENTRY_DSN` to forward panics and captured errors to Sentry. Headers
  `Authorization`, `Cookie`, `X-Export-Token`, `X-PMS-Client` are scrubbed
  before any event leaves the process.

## First-boot checklist

Run through this once after the first `docker compose up -d`. Each step
should take less than a minute.

1. **Generate strong secrets** before editing `.env`:
   ```bash
   openssl rand -hex 32        # SESSION_SECRET
   openssl rand -base64 32     # DB_ENCRYPTION_KEY
   openssl rand -base64 24     # FIRST_SUPERADMIN_PASSWORD (temporary)
   ```
2. **Populate** `deploy/.env` with `PMS_DOMAIN`, `CORS_ORIGINS=https://<domain>`,
   the three secrets above, and `FIRST_SUPERADMIN_EMAIL`.
3. **Boot**: `cd deploy && docker compose up -d`. Wait for
   `docker compose ps` to show `pms-backend` as `healthy` (≤30 s).
4. **Verify probes** from the host:
   ```bash
   curl -fsS https://<domain>/api/../healthz   # via Caddy → 200
   docker compose exec pms-backend /app/pms-healthcheck && echo OK
   ```
5. **First login**: visit `https://<domain>`, sign in with
   `FIRST_SUPERADMIN_EMAIL` + the temporary password. The API will refuse
   every other action with `403 password_change_required` until you rotate.
6. **Rotate the bootstrap password** through the profile screen. The
   forced-change flag clears automatically and other sessions are
   invalidated.
7. **Enrol in 2FA** — super_admin accounts are blocked from every
   non-enrolment endpoint with `403 two_factor_enrolment_required` until
   TOTP is set up. Store the recovery codes offline.
8. **Confirm a backup exists**:
   ```bash
   docker compose exec pms-backend ls -lh /var/lib/pms/backups/ | tail -3
   ```
   The scheduler runs nightly; you can also trigger one with
   `POST /api/admin/backup` (super_admin, returns the tar.gz inline).
9. **Scrape metrics**: `curl -fsS -H "Authorization: Bearer $METRICS_TOKEN"
   http://<host>/metrics | head` (when `METRICS_TOKEN` is set).
10. **Wipe the temporary `FIRST_SUPERADMIN_PASSWORD`** value from `.env`
    once rotation succeeded — it is only consulted on a database with
    zero users.

## Disaster-recovery drill

Run this drill at least quarterly. It validates that the encrypted backup
is restorable and that the operator knows the muscle-memory steps.

1. **Pick a snapshot** from `/var/lib/pms/backups/` (e.g. the most recent).
2. **Stop the backend** so SQLite has no open writers:
   ```bash
   docker compose stop pms-backend
   ```
3. **Move the live DB aside** (do not delete; you may need to roll back):
   ```bash
   docker compose run --rm --entrypoint sh pms-backend -c \
     "mv /var/lib/pms/pms.db /var/lib/pms/pms.db.predrill"
   ```
4. **Restore** the snapshot in place:
   ```bash
   docker compose run --rm --entrypoint sh pms-backend -c \
     "tar -xzf /var/lib/pms/backups/<snapshot>.tar.gz -C / \
      && chown nonroot:nonroot /var/lib/pms/pms.db"
   ```
5. **Start** the backend and watch logs for migration replay:
   ```bash
   docker compose up -d pms-backend
   docker compose logs -f pms-backend
   ```
6. **Verify**: log in, confirm the most recent occupancy / invoice / audit
   log entries match what you expect from the snapshot date.
7. **Roll back** if the drill fails: stop the backend, swap
   `pms.db.predrill` back into place, restart.
8. **Record** the drill date and outcome in
   [`docs/deployment/backup-runbook.md`](backup-runbook.md).

If the drill ever fails to complete step 6, the backups are not actually
usable. Treat that as a P0 — fix the backup pipeline before any further
production change.
