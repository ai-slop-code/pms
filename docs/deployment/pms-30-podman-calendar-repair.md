# PMS 30 — Podman cleaning-calendar migration

The main backend image now packages:

- `/app/cleaning-calendar-cleanup prepare|status|run`
- `/app/pms-migrate cleaning-calendar`

This runbook uses the user-confirmed deployed image
`ghcr.io/ai-slop-code/pms-backend:2.6.7` for cleanup, migration, and the server.
It uses the exact production container name, network, data mount, and launch
options supplied by the user. No source checkout or image build is needed.

The migration command applies only migration 40, requires migration 39 already
installed, and enforces the existing PMS-22 cleanup prerequisite. It is safe to
repeat the migration command after success. Normal server startup still skips
the manual transition.

## 1. Set production values and check the packaged command

Run the blocks below **in order, in the same Bash session**, on the production
host, from the directory containing the `pms.env` used in your launch command.
Use the same account that launched Podman (do not switch between rootful and
rootless Podman). All values are filled in; no image/path placeholders remain.
Stop if any command fails.

```bash
IMAGE=ghcr.io/ai-slop-code/pms-backend:2.6.7
CONTAINER=api.pms.airportlounge.sk
DATA=/mnt/main_storage/containers/data/api.pms.airportlounge.sk
ENV_FILE="$(pwd)/pms.env"
test -f "$ENV_FILE" &&
  podman run --rm --pull=never --entrypoint /app/pms-migrate "$IMAGE" --help

podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$CONTAINER" |
  grep -E '^(DATABASE_PATH|DATA_DIR|PMS_GOOGLE_SERVICE_ACCOUNT_FILE)='
```

The help command must succeed before stopping production. `--pull=never` uses
the image already deployed on this host. Confirm the database is under `/data`
and the credential file, if used, is available through that mount. Keep the same
database and Google settings in `pms.env`; do not print or paste credentials.

## 2. Stop and back up

Run each step separately and stop on any failure. Stop any additional backend
instances/schedulers if they share the database.

```bash
podman stop --time 60 "$CONTAINER" &&
  test "$(podman inspect --format '{{.State.Running}}' "$CONTAINER")" = false
```

Both commands must succeed before the backup.

```bash
BACKUP="${DATA}.before-calendar-migration-$(date -u +%Y%m%dT%H%M%SZ).tar"
if [ "$(podman info --format '{{.Host.Security.Rootless}}')" = true ]; then
  podman unshare tar -C "$DATA" -cpf "$BACKUP" .
else
  tar -C "$DATA" -cpf "$BACKUP" .
fi
tar -tf "$BACKUP" >/dev/null
ls -lh "$BACKUP"
```

The archive is stored beside the data directory, outside the mount. It includes
SQLite sidecars if present. Proceed only after successful backup and archive
verification. If your account cannot write in the parent directory, stop and
use a writable backup location outside the data directory.

## 3. Run cleanup using the deployed 2.6.7 image

Define this helper to reuse your production container settings:

```bash
pms_operator() {
  local executable="$1"
  shift
  podman run --rm --pull=never \
    --network internet_enabled \
    --read-only --tmpfs /tmp \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    -v "$DATA:/data:Z" \
    --env-file "$ENV_FILE" \
    --entrypoint "$executable" \
    "$IMAGE" "$@"
}

pms_operator /app/cleaning-calendar-cleanup prepare &&
  pms_operator /app/cleaning-calendar-cleanup status
```

Check the reported calendar and cutoff. Preparation fixes the property-local
date when it runs; do not backdate it to the incident date. Then:

```bash
pms_operator /app/cleaning-calendar-cleanup run &&
  pms_operator /app/cleaning-calendar-cleanup status
```

Every prepared property must report `phase=complete`, `pending=0`, and no error.
This is the existing PMS-22 cleanup: eligible future provisional Google events
are deleted, while past Google events and named-stay events are preserved.

If cleanup fails, keep the backend stopped, correct the reported error, and
retry `run`. Do not change the target calendar or manually mark work complete.

## 4. Apply migration 40

```bash
pms_operator /app/pms-migrate cleaning-calendar
```

Expected result:

```text
cleaning-calendar migration 000040 is complete
```

The migration removes local provisional history and preserves named events,
their IDs, Google references, and logs. Foreign keys for the table rebuild are
deferred until transaction commit, so named-event logs do not block the rebuild
and referential integrity remains enforced. If it fails, keep the server
stopped and retain the error output. Do not rerun cleanup after migration 40;
its legacy source columns have been removed.

## 5. Recreate the backend with 2.6.7

Only after successful migration, replace the stopped container. This uses your
original launch settings and the same `2.6.7` image. Recreating the container
also restores its private SELinux mount label after the one-shot commands:

```bash
podman rm "$CONTAINER" &&
podman run -d --pull=never \
  --name api.pms.airportlounge.sk \
  --hostname api.pms.airportlounge.sk \
  --network internet_enabled \
  --restart=unless-stopped \
  --read-only \
  --tmpfs /tmp \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z \
  --env-file "$ENV_FILE" \
  --health-cmd '/app/pms-healthcheck' \
  --health-interval=30s \
  --health-timeout=5s \
  --health-retries=3 \
  ghcr.io/ai-slop-code/pms-backend:2.6.7
```

Allow startup to finish, then check logs and health:

```bash
podman logs --since 5m api.pms.airportlounge.sk
podman healthcheck run api.pms.airportlounge.sk
```

Then open Cleaning for property 1, select September 2026, and confirm the events
load. Click **Reconcile now**, verify the response's `ok` field and latest run
status, and inspect upcoming eligible named-stay events in Google Calendar.
HTTP 200 alone does not establish reconciliation success.

Retain the backup and command output. Restoring a database does not undo remote
Google deletions; use the existing PMS-22 recovery procedure if needed.
