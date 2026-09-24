# PMS 30 — Podman cleaning-calendar migration

The main backend image now packages:

- `/app/cleaning-calendar-cleanup prepare|status|run`
- `/app/pms-migrate cleaning-calendar`

Use the **same new backend image** for cleanup, migration, and the server. No
separate maintenance image or generated Go wrapper is required. The `2.6.5`
image does not contain these tools.

The migration command applies only migration 40, requires migration 39 already
installed, and enforces the existing PMS-22 cleanup prerequisite. It is safe to
repeat the migration command after success. Normal server startup still skips
the manual transition.

## 1. Obtain the new main backend image

On the production host, from a checkout containing this change, build the normal
backend Dockerfile:

```bash
IMAGE=localhost/pms-backend:pms-30
podman build --platform linux/amd64 -f deploy/Dockerfile.backend -t "$IMAGE" .
podman run --rm --entrypoint /app/pms-migrate "$IMAGE" --help
```

This is the normal server image. The explicit platform matches the existing
Dockerfile's Linux/amd64 binary build. If using the normal GHCR publishing
workflow instead, set `IMAGE` to the **newly published immutable tag/digest** and
pull it before proceeding; do not use `2.6.5`. No new GHCR tag is claimed by this
document.

Finish the build/pull and help check before stopping production.

## 2. Identify production configuration

In the same Bash session, change to the directory containing the production
`pms.env` used in your original launch command. Then:

```bash
CONTAINER=api.pms.airportlounge.sk
DATA=/mnt/main_storage/containers/data/api.pms.airportlounge.sk
ENV_FILE="$(pwd)/pms.env"
test -f "$ENV_FILE"

podman inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$CONTAINER" |
  grep -E '^(DATABASE_PATH|DATA_DIR|PMS_GOOGLE_SERVICE_ACCOUNT_FILE)='
```

Confirm the database is under `/data` and the credential file, if used, is
available through the existing `/data` mount. Use the same database and Google
settings in `pms.env`. Do not print or paste credentials. Run Podman with the
same user/rootful or rootless mode as the original deployment.

## 3. Stop and back up

Run each step separately and stop on any failure. Stop any additional backend
instances/schedulers if they share the database.

```bash
podman stop --time 60 "$CONTAINER"
podman inspect --format '{{.State.Running}}' "$CONTAINER"
```

The result must be `false`.

```bash
BACKUP="$(pwd)/pms-before-calendar-migration-$(date -u +%Y%m%dT%H%M%SZ).tar"
if [ "$(podman info --format '{{.Host.Security.Rootless}}')" = true ]; then
  podman unshare tar -C "$DATA" -cpf "$BACKUP" .
else
  tar -C "$DATA" -cpf "$BACKUP" .
fi
tar -tf "$BACKUP" >/dev/null
ls -lh "$BACKUP"
```

Keep the backup outside the mounted data directory. It includes SQLite sidecars
if present. Proceed only after successful backup and archive verification.

## 4. Run cleanup using the new backend image

Define this helper to reuse your production container settings:

```bash
pms_operator() {
  local executable="$1"
  shift
  podman run --rm \
    --network internet_enabled \
    --read-only --tmpfs /tmp \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    -v "$DATA:/data:Z" \
    --env-file "$ENV_FILE" \
    --entrypoint "$executable" \
    "$IMAGE" "$@"
}

pms_operator /app/cleaning-calendar-cleanup prepare
pms_operator /app/cleaning-calendar-cleanup status
```

Check the reported calendar and cutoff. Preparation fixes the property-local
date when it runs; do not backdate it to the incident date. Then:

```bash
pms_operator /app/cleaning-calendar-cleanup run
pms_operator /app/cleaning-calendar-cleanup status
```

Every prepared property must report `phase=complete`, `pending=0`, and no error.
This is the existing PMS-22 cleanup: eligible future provisional Google events
are deleted, while past Google events and named-stay events are preserved.

If cleanup fails, keep the backend stopped, correct the reported error, and
retry `run`. Do not change the target calendar or manually mark work complete.

## 5. Apply migration 40

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

## 6. Recreate the backend with that same new image

Only after successful migration, replace the stopped container. This is your
original launch command with the new image:

```bash
podman rm "$CONTAINER"

podman run -d \
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
  "$IMAGE"

podman logs --since 5m "$CONTAINER"
podman healthcheck run "$CONTAINER"
```

Allow startup to finish before checking health. The final `:Z` mount restores
the correct private SELinux labeling for the new server container.

Then open Cleaning for property 1, select September 2026, and confirm the events
load. Click **Reconcile now**, verify the response's `ok` field and latest run
status, and inspect upcoming eligible named-stay events in Google Calendar.
HTTP 200 alone does not establish reconciliation success.

Retain the backup and command output. Restoring a database does not undo remote
Google deletions; use the existing PMS-22 recovery procedure if needed.
