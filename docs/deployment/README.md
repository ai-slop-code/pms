# PMS deployment guide

Four supported layouts, in order of simplicity:

1. **Docker Compose + Caddy** — one host, automatic TLS, zero config beyond DNS. Recommended.
2. **systemd binary** — single host, no container runtime.
3. **Static frontend + your own infra** — ship `frontend/dist` to any static host (S3, nginx, Apache, Cloudflare Pages, GitHub Pages behind a proxy, …) and point it at a separately hosted backend.
4. **Manual Go build** — local development or custom orchestration.

All layouts share the same env vars; see [`deploy/.env.example`](../../deploy/.env.example).

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

## 3. Static frontend + your own infra

If you already have a way to serve static files (S3 + CloudFront, nginx,
Apache, Caddy on a shared host, Cloudflare Pages, etc.), you only need
the built SPA bundle and a reverse-proxy rule that forwards `/api/*` to a
running backend. All frontend code calls same-origin `/api/...`, so the
only requirement is that the static host and the API share an origin
(either the same domain, or the static host proxies `/api/*` to the
backend).

### Build the bundle

```bash
cd frontend
npm ci
npm run build
# output: frontend/dist/  (static HTML/JS/CSS only, no Node runtime needed)
```

Or from the repo root:

```bash
make frontend-build
```

Upload the contents of `frontend/dist/` to your static host.

### Reverse-proxy rule

Point `/api/*` at a `pms-server` binary running on any reachable host.
Minimal nginx snippet:

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

Caddyfile equivalent:

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

Run the backend using layout #2 (systemd) or any process supervisor you
prefer. The static host does not need Node at runtime.

> **Cross-origin note:** if the static bundle is served from a _different_
> origin than the API (e.g. `app.example.com` for the SPA, `api.example.com`
> for the backend), set `CORS_ORIGINS=https://app.example.com` in the
> backend env and ensure your reverse proxy does not strip the session
> cookie. Same-origin hosting avoids this entirely.

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
