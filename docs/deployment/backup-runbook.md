# Database Backup & Restore Runbook

PMS ships with an **in-process snapshot scheduler** (Option B from
`spec/PMS_11`). The server periodically writes a consistent SQLite file to
`${PMS_BACKUP_DIR:-$DATA_DIR/backups}` using `VACUUM INTO`, which holds only a
reader lock against the live DB and produces a self-contained copy safe to
copy off-host.

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

Run as the same Unix user that owns the main SQLite file. The directory is
created with `0750`.

## Verifying a snapshot

```bash
# 1. Integrity check
sqlite3 /srv/pms/data/backups/pms-20251013T130500Z.db 'PRAGMA integrity_check;'
# → "ok"

# 2. Row count sanity check vs. live DB
for t in users properties occupancies finance_transactions; do
  echo -n "$t live: ";   sqlite3 /srv/pms/data/pms.db       "SELECT COUNT(*) FROM $t;"
  echo -n "$t backup: "; sqlite3 /srv/pms/data/backups/pms-20251013T130500Z.db "SELECT COUNT(*) FROM $t;"
done
```

## Off-host replication (recommended)

The simplest robust option is `rclone` with a server-side-encrypted remote:

```bash
# Install rclone, configure a remote named "offsite" once.
rclone sync /srv/pms/data/backups offsite:pms-backups \
  --links --fast-list --transfers=2 \
  --log-file=/var/log/pms/rclone.log \
  --min-age=5m
```

Run on a 15-minute timer via systemd or cron. The `--min-age=5m` flag skips
files still being written.

## Restore drill

1. Stop the PMS service: `systemctl stop pms`
2. Move the current DB aside:
   ```bash
   mv /srv/pms/data/pms.db /srv/pms/data/pms.db.broken
   rm -f /srv/pms/data/pms.db-wal /srv/pms/data/pms.db-shm
   ```
3. Copy the chosen snapshot into place and fix permissions:
   ```bash
   cp /srv/pms/data/backups/pms-20251013T130500Z.db /srv/pms/data/pms.db
   chown pms:pms /srv/pms/data/pms.db
   chmod 0640   /srv/pms/data/pms.db
   ```
4. (Optional) Verify integrity:
   ```bash
   sudo -u pms sqlite3 /srv/pms/data/pms.db 'PRAGMA integrity_check;'
   ```
5. Start the service: `systemctl start pms`
6. Watch the first few requests: `journalctl -fu pms`.

### RPO / RTO

- **RPO:** ≤ `PMS_BACKUP_INTERVAL_MINUTES` + off-host replication lag.
- **RTO:** ~2 minutes (file copy + `systemctl start` + warm-up).

## Alerting

Configure your metrics scraper with a rule like:

```
pms_last_successful_backup_unixtime < (time() - 3 * 3600)
```

to page when no successful snapshot has been recorded for 3 hours.
