# Database Backup & Restore Runbook

PMS ships with an **in-process snapshot scheduler** (Option B from
`spec/PMS_11`). The server periodically writes a consistent SQLite file to
`${PMS_BACKUP_DIR:-$DATA_DIR/backups}` using `VACUUM INTO`, which holds only a
reader lock against the live DB and produces a self-contained copy safe to
copy off-host.

Production runs as the standalone Podman container
`api.pms.airportlounge.sk`, with host data at
`/mnt/main_storage/containers/data/api.pms.airportlounge.sk`, mounted at
`/data:Z`. It does not use Compose or a host `pms.service` systemd unit. The
examples below use that topology and assume `DATABASE_PATH=/data/pms.db`;
substitute the exact path recorded in `pms.env` when different.

Off-host replication is intentionally **out of scope for the app** — the
operator is responsible for an `rsync`/`rclone`/`restic` job that ships the
contents of the backup directory somewhere durable.

## What the scheduler does

| Step           | Detail                                                                           |
| -------------- | -------------------------------------------------------------------------------- |
| Trigger        | Every `PMS_BACKUP_INTERVAL_MINUTES` (default 60). Disable with `0`.              |
| Mechanism      | `VACUUM INTO '<dir>/pms-YYYYMMDDTHHMMSSZ.db.part'` then atomic `rename` to `.db`.|
| Retention      | Keeps 24 most-recent hourly snapshots + one per day for 7 UTC days.              |
| Observability  | Prometheus gauge `pms_last_successful_backup_unixtime` stamps each success.      |
| Failure        | Logged at `ERROR`, counted via `pms_scheduler_runs_total{job="backup_snapshot",outcome="error"}`. |

## Configuration

```
PMS_BACKUP_INTERVAL_MINUTES=60       # 0 disables the scheduler
PMS_BACKUP_DIR=                      # default: $DATA_DIR/backups
```

These variables belong in the container's `pms.env`. The application creates
the directory with `0750` under the mounted data tree.

## Verifying a snapshot

```bash
# 1. Integrity and foreign-key checks
sqlite3 /mnt/main_storage/containers/data/api.pms.airportlounge.sk/backups/<SNAPSHOT>.db \
  'PRAGMA integrity_check; PRAGMA foreign_key_check;'
# → "ok"

# 2. Final-model row-count sanity checks
sqlite3 /mnt/main_storage/containers/data/api.pms.airportlounge.sk/pms.db \
  'SELECT COUNT(*) FROM named_stays; SELECT COUNT(*) FROM named_stay_nights; SELECT COUNT(*) FROM raw_booking_blocks; SELECT COUNT(*) FROM finance_transactions;'
sqlite3 /mnt/main_storage/containers/data/api.pms.airportlounge.sk/backups/<SNAPSHOT>.db \
  'SELECT COUNT(*) FROM named_stays; SELECT COUNT(*) FROM named_stay_nights; SELECT COUNT(*) FROM raw_booking_blocks; SELECT COUNT(*) FROM finance_transactions;'

# 3. Record immutable evidence
sha256sum /mnt/main_storage/containers/data/api.pms.airportlounge.sk/backups/<SNAPSHOT>.db
stat /mnt/main_storage/containers/data/api.pms.airportlounge.sk/backups/<SNAPSHOT>.db
```

## Off-host replication (recommended)

The simplest robust option is `rclone` with a server-side-encrypted remote:

```bash
# Install rclone, configure a remote named "offsite" once.
rclone sync /mnt/main_storage/containers/data/api.pms.airportlounge.sk/backups offsite:pms-backups \
  --links --fast-list --transfers=2 \
  --log-file=/var/log/pms/rclone.log \
  --min-age=5m
```

Run on a 15-minute timer via systemd or cron. The `--min-age=5m` flag skips
files still being written.

## Restore Drill

A restore drill is an isolated rehearsal, not a claim that production was
restored. Record `<DRILL_DATE>`, `<OPERATOR>`, `<SNAPSHOT_SHA256>`,
`<IMAGE_DIGEST>`, `<SCHEMA_BEFORE>`, `<SCHEMA_AFTER>`, `<START_RESULT>`,
`<INTEGRITY_RESULT>`, `<RTO_SECONDS>`, and the restricted evidence path.

1. Resolve the application image to the immutable digest compatible with the
   chosen snapshot. Do not use `latest` as drill evidence.
2. Create an isolated host directory `<DRILL_DATA_DIR>` outside the live data
   directory and copy the chosen `.db` snapshot to `<DRILL_DATA_DIR>/pms.db`.
3. Use a drill-only environment file `<DRILL_ENV_FILE>`. It must point to
   `/data/pms.db`, use non-production secrets, disable schedulers/integrations
   that could contact real Nuki/Google/ICS endpoints, and bind no public
   production route.
4. Verify before startup:

   ```bash
   sha256sum <DRILL_DATA_DIR>/pms.db
   sqlite3 <DRILL_DATA_DIR>/pms.db \
     'PRAGMA integrity_check; PRAGMA foreign_key_check;'
   ```

5. Start an isolated disposable container with the digest-pinned image. Add
   only the networking required for the health check; never attach the
   production route or reuse the production container name.

   ```bash
   APP_IMAGE='ghcr.io/ai-slop-code/pms-backend@sha256:<APPROVED_DIGEST>'
   podman run -d \
     --name pms-backup-restore-drill \
     --network none \
     --read-only \
     --tmpfs /tmp \
     --cap-drop=ALL \
     --security-opt=no-new-privileges \
     -v <DRILL_DATA_DIR>:/data:Z \
     --env-file <DRILL_ENV_FILE> \
     --health-cmd '/app/pms-healthcheck' \
     "$APP_IMAGE"
   podman healthcheck run pms-backup-restore-drill
   podman logs pms-backup-restore-drill
   ```

6. Verify application startup, schema migration if intentionally included in
   the drill, final-model row counts, representative reads, invoice/data-file
   access when the full archive is under test, and another integrity/FK check.
7. Record measured RTO and results, then remove the disposable container and
   drill directory under the operator's normal secure-data procedure.

No successful restore-drill record is currently asserted by this document.

## Emergency Production Restore

Use only after owner/rollback approval and record the write-loss boundary.

1. Record and stop the standalone API: `podman stop api.pms.airportlounge.sk`.
2. Preserve the current database, WAL, and SHM files as restricted incident
   evidence; do not overwrite them before checksums are captured.
3. Copy the approved snapshot to the configured host database path, remove
   stale WAL/SHM files only after the incident copy is secured, and restore the
   recorded ownership/mode.
4. Run `PRAGMA integrity_check` and `PRAGMA foreign_key_check`.
5. Recreate the API with the compatible immutable digest and exact standalone
   `podman run` options recorded in
   `docs/pms-21-operations-cutover-runbook.md`.
6. Run `podman healthcheck run api.pms.airportlounge.sk` and inspect
   `podman logs api.pms.airportlounge.sk` before traffic resumes.

### RPO / RTO

- **RPO objective:** snapshot interval plus measured off-host replication lag;
  record the actual last durable snapshot timestamp at incident time.
- **RTO objective:** `<APPROVED_RTO>`. Replace this placeholder only with a
  measured restore-drill result for the real data size and compatible image.

## PMS 21 Cleanup Backup

Release C/D requires more than the existence of a scheduled snapshot:

- Stop the API before using the production volume for cleanup audit/apply.
- Record WAL/SHM disposition and use a SQLite-consistent snapshot method.
- Preserve the full required data set, including invoice/attachment files, or
  pair the DB snapshot with a checksum inventory and approved filesystem
  snapshot/archive.
- Record backup checksum, size, database fingerprint, schema version, image
  digest, free disk, restore image, operator, and timestamps.
- Complete and record the isolated restore/migrate/application-start drill
  before destructive approval.
- Pin the pre-cleanup backup through Release C/D acceptance, then return it to
  the normal retention/security policy.

## Alerting

Configure your metrics scraper with a rule like:

```
pms_last_successful_backup_unixtime < (time() - 3 * 3600)
```

to page when no successful snapshot has been recorded for 3 hours.
