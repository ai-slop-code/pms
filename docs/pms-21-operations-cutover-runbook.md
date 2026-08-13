# PMS 21 Operations Cutover Runbook

Status: active operator runbook for PMS 21 Releases A-C/D on the standalone
Podman production topology. PMS does not use Compose in production. No release
window, exception resolution, restore drill, or destructive execution is
claimed complete by this document.

Authority: `spec/PMS_21_Legacy_Occupancy_Removal_Spec.md`, ADR-007, and
`docs/deployment/backup-runbook.md`. The 2026-07-19 Stage 2 JSON artifacts are
historical inputs only and do not authorize cleanup.

## Copy-Paste Local Rehearsal

This section validates a local production data copy without changing the source
database. It builds the cleanup command directly from the current worktree, adds
migration `000038` only to a disposable copy, runs the read-only audit, and
allows migration `000039` only when every database and file gate passes.

Prerequisites on the local machine:

```bash
command -v go
command -v git
command -v sqlite3
command -v rsync
command -v jq
```

Run the following variable block first. Change `REPO_ROOT`, `SOURCE_DATA`, and
`OPERATOR` only when the local paths or operator differ:

```bash
set -eu

REPO_ROOT="/Users/lubosbabjak/Personal/infrastructure/pms"
SOURCE_DATA="$REPO_ROOT/data"
SOURCE_DB="$SOURCE_DATA/pms.db"
OPERATOR="lubosbabjak"

WORK_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/pms21-rehearsal.XXXXXX")"
REHEARSAL_DATA="$WORK_ROOT/data"
PREPARED_DB="$REHEARSAL_DATA/pms-prepared.db"
AUDIT_DB="$REHEARSAL_DATA/pms-audit.db"
AUDIT_REPORT="$WORK_ROOT/PMS_21_cleanup_readiness_local.json"
APPLY_REPORT="$WORK_ROOT/PMS_21_cleanup_apply_local.json"
TOOL="$WORK_ROOT/pms21-cleanup"

mkdir -p "$REHEARSAL_DATA"
printf 'Rehearsal directory: %s\n' "$WORK_ROOT"
```

### 1. Verify And Copy The Source

These commands read the source database and copy the application data. The
SQLite backup API creates a consistent disposable database even when the source
copy was originally produced from a WAL-mode database.

```bash
test -f "$SOURCE_DB"

SOURCE_SHA256="$(shasum -a 256 "$SOURCE_DB" | cut -d ' ' -f 1)"
printf 'Source SHA-256: %s\n' "$SOURCE_SHA256"

sqlite3 "file:$SOURCE_DB?mode=ro&immutable=1" \
  "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1; PRAGMA integrity_check; PRAGMA foreign_key_check;"

rsync -a --exclude 'pms.db*' "$SOURCE_DATA/" "$REHEARSAL_DATA/"
sqlite3 "$SOURCE_DB" ".backup '$PREPARED_DB'"

sqlite3 "$PREPARED_DB" \
  "PRAGMA integrity_check; PRAGMA foreign_key_check; SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
```

Expected output includes `ok`, no foreign-key rows, and a latest version of
`000037_finance_evidence_confirms_named_stays` or
`000038_named_stay_lifecycle_metadata`. Stop for any earlier or unexpected
version.

### 2. Apply Additive Migration 000038 To The Disposable Copy

An existing production database receives `000038` during Release A/B server
startup. A copy still at `000037` needs the same additive migration before the
Release C readiness audit. The following transaction modifies only
`$PREPARED_DB`:

```bash
CURRENT_VERSION="$(sqlite3 "$PREPARED_DB" \
  "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;")"

case "$CURRENT_VERSION" in
  000037_finance_evidence_confirms_named_stays)
    sqlite3 "$PREPARED_DB" \
      ".bail on" \
      "PRAGMA foreign_keys=ON;" \
      "BEGIN IMMEDIATE;" \
      ".read $REPO_ROOT/backend/internal/migrate/000038_named_stay_lifecycle_metadata.up.sql" \
      "INSERT INTO schema_migrations(version) VALUES ('000038_named_stay_lifecycle_metadata');" \
      "COMMIT;"
    ;;
  000038_named_stay_lifecycle_metadata)
    printf 'Migration 000038 is already present.\n'
    ;;
  000039_legacy_occupancy_removal)
    printf 'This copy is already cleaned; create a new pre-cleanup rehearsal copy.\n' >&2
    exit 1
    ;;
  *)
    printf 'Unsupported starting migration: %s\n' "$CURRENT_VERSION" >&2
    exit 1
    ;;
esac

sqlite3 "$PREPARED_DB" \
  "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1; PRAGMA integrity_check; PRAGMA foreign_key_check;"
```

### 3. Create A Sidecar-Free Audit Snapshot

Do not delete WAL or SHM files. Create a second SQLite backup after `000038` and
audit that clean snapshot:

```bash
sqlite3 "$PREPARED_DB" ".backup '$AUDIT_DB'"

test -f "$AUDIT_DB"
test ! -e "$AUDIT_DB-wal"
test ! -e "$AUDIT_DB-shm"

sqlite3 "file:$AUDIT_DB?mode=ro&immutable=1" \
  "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1; PRAGMA integrity_check; PRAGMA foreign_key_check;"
```

### 4. Build The Cleanup Tool

The local binary SHA-256 is used as the immutable rehearsal artifact identity.
It is not a substitute for the production container digest.

```bash
(
  cd "$REPO_ROOT/backend"
  go build -o "$TOOL" ./cmd/pms21-cleanup
)

TOOL_SHA256="$(shasum -a 256 "$TOOL" | cut -d ' ' -f 1)"
IMAGE_DIGEST="sha256:$TOOL_SHA256"
BACKEND_COMMIT="$(git -C "$REPO_ROOT" rev-parse HEAD)-local-worktree"
FRONTEND_BUILD="local-rehearsal"

printf 'Cleanup tool SHA-256: %s\n' "$TOOL_SHA256"
```

### 5. Run The Read-Only Audit

The `LOCAL_REHEARSAL_ONLY` references deliberately mark this report as
non-production evidence. Replace them with approved, checksum-linked references
only during the real production procedure.

```bash
EXCEPTION_REGISTER_REFERENCE="LOCAL_REHEARSAL_ONLY:not-production-approved"
APPROVED_EXCEPTIONS_REFERENCE="LOCAL_REHEARSAL_ONLY:not-production-approved"
ANALYTICS_PARITY_REFERENCE="LOCAL_REHEARSAL_ONLY:not-production-approved"
REMOTE_VERIFICATION_REFERENCE="LOCAL_REHEARSAL_ONLY:not-production-approved"
CALLER_INVENTORY_REFERENCE="LOCAL_REHEARSAL_ONLY:not-production-approved"

set +e
"$TOOL" \
  --audit \
  --db "$AUDIT_DB" \
  --data-root "$REHEARSAL_DATA" \
  --image-digest "$IMAGE_DIGEST" \
  --commit "$BACKEND_COMMIT" \
  --frontend-build "$FRONTEND_BUILD" \
  --operator "$OPERATOR" \
  --exception-register-reference "$EXCEPTION_REGISTER_REFERENCE" \
  --approved-exceptions-reference "$APPROVED_EXCEPTIONS_REFERENCE" \
  --analytics-parity-reference "$ANALYTICS_PARITY_REFERENCE" \
  --remote-verification-reference "$REMOTE_VERIFICATION_REFERENCE" \
  --caller-inventory-reference "$CALLER_INVENTORY_REFERENCE" \
  > "$AUDIT_REPORT"
AUDIT_EXIT=$?
set -e

printf 'Audit exit code: %s\n' "$AUDIT_EXIT"
jq '{
  passed,
  failed_checks: [.checks[] | select(.passed == false)],
  errors,
  database_before,
  report_checksum
}' "$AUDIT_REPORT"
printf 'Audit report: %s\n' "$AUDIT_REPORT"
```

An exit code of `1` is expected while data gates remain unresolved. Correct the
reported data in an appropriate Release A/B environment, create a new source
copy, and repeat from step 1. Never edit rows merely to make the count zero.

### 6. Rehearse Destructive Cleanup Only After A Passing Audit

The next command refuses to run unless the report passed, its checksum is
valid, and the database fingerprint and every command identity are unchanged.
It modifies only `$AUDIT_DB`.

```bash
jq -e '.passed == true' "$AUDIT_REPORT"

"$TOOL" \
  --apply \
  --db "$AUDIT_DB" \
  --data-root "$REHEARSAL_DATA" \
  --image-digest "$IMAGE_DIGEST" \
  --commit "$BACKEND_COMMIT" \
  --frontend-build "$FRONTEND_BUILD" \
  --operator "$OPERATOR" \
  --exception-register-reference "$EXCEPTION_REGISTER_REFERENCE" \
  --approved-exceptions-reference "$APPROVED_EXCEPTIONS_REFERENCE" \
  --analytics-parity-reference "$ANALYTICS_PARITY_REFERENCE" \
  --remote-verification-reference "$REMOTE_VERIFICATION_REFERENCE" \
  --caller-inventory-reference "$CALLER_INVENTORY_REFERENCE" \
  --confirm-destructive-cleanup \
  --pre-report "$AUDIT_REPORT" \
  > "$APPLY_REPORT"

jq '{
  passed,
  failed_checks: [.checks[] | select(.passed == false)],
  errors,
  database_before,
  database_after,
  report_checksum
}' "$APPLY_REPORT"
printf 'Apply report: %s\n' "$APPLY_REPORT"
```

### 7. Verify The Cleaned Rehearsal Database

```bash
sqlite3 "$AUDIT_DB" "PRAGMA foreign_key_check; PRAGMA integrity_check;"

sqlite3 "$AUDIT_DB" "
SELECT name
FROM sqlite_schema
WHERE type = 'table'
  AND name IN (
    'occupancies',
    'occupancy_nights',
    'occupancy_api_tokens',
    'occupancy_stay_migration_map'
  );
"

sqlite3 "$AUDIT_DB" "
SELECT s.name AS table_name, c.name AS column_name
FROM sqlite_schema s, pragma_table_info(s.name) c
WHERE s.type = 'table'
  AND c.name IN (
    'occupancy_id',
    'next_occupancy_id',
    'source_occupancy_id',
    'old_occupancy_id'
  );
"

sqlite3 "$AUDIT_DB" \
  "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
```

Expected results after a successful rehearsal apply:

- `PRAGMA foreign_key_check` returns no rows.
- `PRAGMA integrity_check` returns `ok`.
- Both legacy schema queries return no rows.
- Latest migration is `000039_legacy_occupancy_removal`.
- The apply report has `"passed": true`.

The source database at `$SOURCE_DB` is never an apply target in this procedure.
Confirm that it remains unchanged:

```bash
test "$(shasum -a 256 "$SOURCE_DB" | cut -d ' ' -f 1)" = "$SOURCE_SHA256"
printf 'Source database remained unchanged.\n'
```

## Production Topology

- Container: `api.pms.airportlounge.sk`
- Network: `internet_enabled`
- Mutable repository name: `ghcr.io/ai-slop-code/pms-backend`
- Environment file: `./pms.env`
- Host data: `/mnt/main_storage/containers/data/api.pms.airportlounge.sk`
- Container data: `/data:Z`
- Expected database: `/data/pms.db`, unless `DATABASE_PATH` in `pms.env`
  records another path under `/data`

All cleanup evidence and one-off containers must use an immutable image
reference of the form:

```text
ghcr.io/ai-slop-code/pms-backend@sha256:<APPROVED_DIGEST>
```

`latest`, `main`, and release tags may be used to discover an image, but never
as the recorded audit, migration, verification, or recreated API identity.

## Required Record

Create a restricted operations record and a redacted checksum-linked summary.
Fill every placeholder; `TBD` is a stop condition for Release C/D.

- Operator and owner/go-no-go approver: `<OPERATOR>`, `<APPROVER>`
- Maintenance window and traffic-resume deadline: `<START_UTC>`, `<END_UTC>`
- Release A digest/window/evidence: `<DIGEST_A>`, `<A_START>`, `<A_END>`, `<A_EVIDENCE>`
- Release B digest/window/evidence: `<DIGEST_B>`, `<B_START>`, `<B_END>`, `<B_EVIDENCE>`
- Cleanup digest/commit/frontend build: `<C_DIGEST>`, `<COMMIT>`, `<FRONTEND_BUILD>`
- Database path/fingerprint/schema version: `<DB_PATH>`, `<DB_SHA256>`, `<SCHEMA_VERSION>`
- Effective PMS 21 flags: `none`
- Exception register and approved-exceptions decision: `<EXCEPTION_REGISTER>`, `<APPROVED_EXCEPTIONS>`
- Approved analytics/availability parity: `<ANALYTICS_PARITY>`
- Approved caller/runtime inventory: `<CALLER_INVENTORY>`
- Backup/archive/checksum/size: `<BACKUP_PATH>`, `<BACKUP_SHA256>`, `<BACKUP_SIZE>`
- Restore-drill evidence and compatible image: `<RESTORE_DRILL>`, `<RESTORE_IMAGE>`
- Free disk/temporary-copy budget: `<FREE_BYTES>`, `<REQUIRED_BYTES>`
- Measured migration and lock duration: `<MIGRATION_DURATION>`, `<LOCK_DURATION>`
- Cleanup migration/tool checksum: `<MIGRATION_CHECKSUM>`
- Pre/post readiness and verification reports: `<PRE_REPORT>`, `<POST_REPORT>`
- Remote Google/Nuki verification: `<REMOTE_EVIDENCE>`
- Rollback authority and traffic-resume approval: `<ROLLBACK_APPROVER>`, `<RESUME_APPROVAL>`

Raw production output containing guest data, PINs, tokens, credentials, or
secret-bearing URLs stays outside the repository.

## Resolve The Image Digest

Run from the directory containing `pms.env`. Record the current container
before changing anything:

```bash
podman inspect api.pms.airportlounge.sk > /absolute/restricted/path/api-before.json
podman image inspect ghcr.io/ai-slop-code/pms-backend:<APPROVED_TAG>
podman pull ghcr.io/ai-slop-code/pms-backend:<APPROVED_TAG>
podman image inspect --format '{{json .RepoDigests}}' \
  ghcr.io/ai-slop-code/pms-backend:<APPROVED_TAG>
```

Select and record the approved repository digest, then use only this exact
reference in the remaining commands:

```bash
APP_IMAGE='ghcr.io/ai-slop-code/pms-backend@sha256:<APPROVED_DIGEST>'
podman image inspect "$APP_IMAGE"
```

If the digest cannot be resolved or does not match the approved commit/build,
stop.

## Release A

Release A is non-destructive. It stops all legacy writes while retaining
legacy storage for observation.

Before deployment:

- Resolve every known cleaning, external-sale, review-required, finance,
  invoice, Nuki, and cross-property exception, with row-level evidence.
- Verify the Release A build stops ICS/named-stay/finance/integration legacy
  writes, startup repair, migration-map creation, and public export/token use.
- Capture baseline row hashes, legacy-column hashes, analytics, availability,
  Google ownership, and Nuki values.
- Confirm no older API, worker, cron job, support binary, or external writer can
  recreate legacy data.

Recreate the API using the approved Release A digest and the existing direct
Podman topology. Preserve any additional options from the recorded production
`podman inspect`; the template below is the minimum known contract:

```bash
podman stop api.pms.airportlounge.sk
podman rm api.pms.airportlounge.sk
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
  --env-file ./pms.env \
  --health-cmd '/app/pms-healthcheck' \
  --health-interval=30s \
  --health-timeout=5s \
  --health-retries=3 \
  "$APP_IMAGE"
podman healthcheck run api.pms.airportlounge.sk
podman logs api.pms.airportlounge.sk
```

Release A acceptance requires an approved minimum 48-hour production window
including restart, repeated ICS sync, named-stay lifecycle/cleaning change,
cleaning and Nuki reconciliation, finance import/rematch, invoice
create/regenerate, message generation, and analytics/dashboard verification.
Any relevant incident, rollback, or code change restarts the window. Record
zero prohibited writes and no unexplained metric variance. Do not infer this
window from elapsed calendar time alone.

## Release B

Release B is non-destructive. It removes runtime compatibility while legacy
tables remain inert.

Before deployment:

- Approve caller inventory for public traffic, automation, support scripts,
  reporting, cached/older frontends, workers, and containers.
- Confirm deprecated occupancy routes, export/token APIs, repair APIs, request
  aliases, response fields, frontend controls, store fallbacks, and migration
  map lookups are absent.
- Pass startup, scheduler, manual-job, support-tool, API, and integration tests
  with legacy objects renamed or access-denied.
- Reconcile Google metadata to new identities and record local/remote orphan
  results without changing non-PMS events.

Deploy by repeating the direct `podman stop`, `podman rm`, and `podman run`
template above with the approved Release B digest. Release B requires its own
minimum 48-hour window and representative workload. Runtime SQL tracing must
show zero access to `occupancies`, `occupancy_nights`,
`occupancy_api_tokens`, and `occupancy_stay_migration_map`. A relevant incident,
rollback, or code change restarts the window.

## Release C/D Preconditions

Do not begin the maintenance window until all PMS 21 cleanup gates pass and
the Required Record has no placeholders. In particular:

- Release A and B windows are separately approved.
- All exception and `needs_review` rows are individually resolved.
- Data/value parity, same-property ownership, caller inventory, runtime
  independence, remote Google/Nuki checks, and migration failure tests pass.
- Backup method, WAL/SHM handling, free disk, measured migration/lock duration,
  RPO/RTO, compatible restore image, and traffic-resume authority are approved.
- Transitional diagnostics to retain before combined C/D tooling removal are
  identified.

## Cleanup Backup And Restore Gate

Follow `docs/deployment/backup-runbook.md`. Before stopping, create the normal
full application archive if that path is used operationally. Then stop the API
and create a SQLite-consistent cleanup snapshot from the quiesced database,
recording WAL/SHM disposition, checksum, size, integrity result, data-file
checksums, and the compatible digest.

```bash
podman stop api.pms.airportlounge.sk
```

Do not continue until `<RESTORE_DRILL>` proves the chosen backup can be
restored, migrated, and used to start the compatible application in an
isolated environment. This runbook does not claim that drill has occurred.

## Cleanup Readiness Audit

The Release C image supplies `/app/pms21-cleanup`. Set the release identities
to the exact values in the Required Record, mount production data read-only,
and redirect output outside the app volume:

```bash
C_DIGEST='sha256:<APPROVED_DIGEST>'
BACKEND_COMMIT='<COMMIT>'
FRONTEND_BUILD='<FRONTEND_BUILD>'
OPERATOR='<OPERATOR>'
EXCEPTION_REGISTER_REFERENCE='<EXCEPTION_REGISTER>#sha256:<SHA256>'
APPROVED_EXCEPTIONS_REFERENCE='<APPROVED_EXCEPTIONS>#sha256:<SHA256>'
ANALYTICS_PARITY_REFERENCE='<ANALYTICS_PARITY>#sha256:<SHA256>'
REMOTE_VERIFICATION_REFERENCE='<REMOTE_EVIDENCE>#sha256:<SHA256>'
CALLER_INVENTORY_REFERENCE='<CALLER_INVENTORY>#sha256:<SHA256>'
podman run --rm \
  --name pms21-cleanup-readiness \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:ro,Z \
  --entrypoint /app/pms21-cleanup \
  "$APP_IMAGE" \
  --audit \
  --db /data/pms.db \
  --data-root /data \
  --image-digest "$C_DIGEST" \
  --commit "$BACKEND_COMMIT" \
  --frontend-build "$FRONTEND_BUILD" \
  --operator "$OPERATOR" \
  --exception-register-reference "$EXCEPTION_REGISTER_REFERENCE" \
  --approved-exceptions-reference "$APPROVED_EXCEPTIONS_REFERENCE" \
  --analytics-parity-reference "$ANALYTICS_PARITY_REFERENCE" \
  --remote-verification-reference "$REMOTE_VERIFICATION_REFERENCE" \
  --caller-inventory-reference "$CALLER_INVENTORY_REFERENCE" \
  > /absolute/restricted/path/PMS_21_cleanup_readiness_YYYY-MM-DD.json
```

The stopped API must have checkpointed and closed SQLite cleanly. Both
`/data/pms.db-wal` and `/data/pms.db-shm` must be absent; do not delete either
sidecar by hand. The audit rejects either sidecar, records their absence in the
database fingerprint, verifies invoice file contents under `/data`, and reports
the effective PMS 21 flags as `none`. The external references above are
approved evidence inputs; SQL checks do not prove analytics, remote-system, or
caller/runtime parity.

Stop on any unapproved non-zero gate, digest mismatch, schema mismatch,
cross-property reference, missing owner, unresolved review row, statistics
variance, caller, remote orphan, or failed integrity check. A Stage 2 dry run
is not a cleanup-readiness report.

## Destructive Forward Migration

Release C and combined D execute only after explicit go/no-go approval. Keep
the API stopped. Use the same digest and exact database path as readiness:

```bash
podman run --rm \
  --name pms21-cleanup-apply \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z \
  -v /absolute/restricted/path:/evidence:ro,Z \
  --entrypoint /app/pms21-cleanup \
  "$APP_IMAGE" \
  --apply \
  --db /data/pms.db \
  --data-root /data \
  --image-digest "$C_DIGEST" \
  --commit "$BACKEND_COMMIT" \
  --frontend-build "$FRONTEND_BUILD" \
  --operator "$OPERATOR" \
  --exception-register-reference "$EXCEPTION_REGISTER_REFERENCE" \
  --approved-exceptions-reference "$APPROVED_EXCEPTIONS_REFERENCE" \
  --analytics-parity-reference "$ANALYTICS_PARITY_REFERENCE" \
  --remote-verification-reference "$REMOTE_VERIFICATION_REFERENCE" \
  --caller-inventory-reference "$CALLER_INVENTORY_REFERENCE" \
  --confirm-destructive-cleanup \
  --pre-report /evidence/PMS_21_cleanup_readiness_YYYY-MM-DD.json \
  > /absolute/restricted/path/PMS_21_cleanup_apply_YYYY-MM-DD.json
```

The apply command verifies that the pre-report passed, its checksum is valid,
its release, tool, operator, exception, and external-evidence identities match
these flags, and its database and invoice-file fingerprints still match. It
rechecks WAL/SHM absence and the database fingerprint immediately before the
writable open. Capture required migration and repair diagnostics before the
same release removes transitional CLIs and flags.

## Post-Migration Verification

With the API still stopped:

- Run `PRAGMA foreign_key_check` and `PRAGMA integrity_check`.
- Verify legacy tables/columns/FKs are absent and retained source/sync tables
  remain.
- Compare row counts, critical-value hashes, child references, invoice/data
  file checksums, constraints, indexes, and AUTOINCREMENT behavior.
- Verify analytics/availability/pace/lead-time/cancellation/revenue parity.
- Verify Nuki IDs, exact stored PIN values, external IDs, windows, statuses,
  event links, and guest-entry links.
- Verify cleaning IDs, Google calendar/event IDs, deterministic identities,
  desired hashes, logs, and new-model owner cardinality.
- Verify finance booking, transaction, import/reset, invoice, sequence, and
  file relationships.

Recreate the API with the same digest using the direct `podman run` template,
then run health, logs, startup, scheduler, manual job, named-stay lifecycle,
ICS, cleaning, Nuki, finance, invoice, message, analytics, dashboard, API 404,
authorization, and cross-property checks. Complete remote Google/Nuki orphan
verification before traffic resumes.

Only `<RESUME_APPROVAL>` may authorize traffic. Record the exact timestamp.

## Rollback

- Release A/B: restore the tested compatible pre-release image/database pair.
  Do not assume an old binary is safe against legacy state that stopped
  receiving writes.
- Release C before traffic resumes: keep production quiesced, restore the
  verified pre-cleanup backup and compatible image, and target RPO 0/RTO 30
  minutes.
- Release C after traffic resumes: fix-forward is the default. Restoring the
  pre-cleanup backup requires owner approval and explicit acceptance of all
  later write loss unless a separately approved forward recovery exists.
- Never use historical down migrations as operational rollback.

## Historical Initial-Cutover Procedure

The remaining sections preserve the original Stage 2 cutover procedure as
historical evidence. **Do not run these mutable-`latest` Stage 2 commands for
Release A-D cleanup.** The referenced `pms21-migration` binary has been removed;
the commands remain documentation of the historical process only. Use the
active digest-pinned procedure above.

### Historical Preconditions

- Record the exact old binary/version currently running.
- Record the exact new PMS 21 binary/version/commit to deploy.
- Record the matching frontend build/version for the new backend.
- Confirm the database backup location and restore procedure.
- Schedule a maintenance window or reduced-traffic period.
- Confirm no other migration, import, sync, invoice, message, cleaning, or Nuki job is running.
- Confirm Nuki, Booking.com ICS sync, Google cleaning calendar, finance imports, invoice generation, and message jobs are paused or safe to run during migration.
- Confirm latest migrations included in the new binary. Current latest local migration number is `000037`; add later migrations only through the normal migration path.
- Confirm `PMS21_RAW_BLOCKS_DUAL_WRITE`, `PMS21_OCCUPANCY_EXPORT_DISABLED`, and `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` are set as intended.
- Confirm OpenAPI and frontend assets correspond to the deployed backend version.
- Confirm `DATABASE_PATH=/data/pms.db` in `pms.env`. If the file uses a different path under `/data`, use that exact path for every `--db` argument below.
- Confirm the production bind mount remains `/mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z`.
- Confirm `ghcr.io/ai-slop-code/pms-backend:latest` contains the approved PMS 21 version before stopping production.

Record the current container and image details before stopping it:

```bash
podman inspect api.pms.airportlounge.sk
podman image inspect ghcr.io/ai-slop-code/pms-backend:latest
```

### Historical Backup Steps

- Stop or pause the old app/jobs as required by the maintenance plan.
- Take a database backup before applying migrations or running PMS 21 apply.
- Record backup path, timestamp, database size/checksum if practical, old binary version, and new binary version.
- Verify the backup can be opened or restored in a safe environment if practical.

### Historical Dry-Run And Audit Steps

Run the read-only PMS 21 dry-run/audit command against a verified, quiesced production backup, or against the production volume only after the backend is fully stopped and a backup has been taken. The CLI opens the file with SQLite immutable read-only mode, so never point it at a database file that can still change.

Run these commands from the directory containing `pms.env`. Stop the API so each temporary migration container can mount the same host data directory directly. The API is recreated with its normal command after apply:

```bash
podman stop api.pms.airportlounge.sk
podman run --rm \
  --name pms21-migration-audit \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:ro,Z \
  --entrypoint /app/pms21-migration \
  ghcr.io/ai-slop-code/pms-backend:latest \
  --db /data/pms.db \
  --dry-run \
  --sample-limit 25 \
  > /absolute/path/to/PMS_21_production_data_audit_YYYY-MM-DD.json
```

Do not recreate the API between dry-run and apply.

Review the saved artifact for:

- Auto-confirmable named-stay candidates.
- Review-required rows.
- Severe conflicts.
- Unmapped Nuki, cleaning, finance, invoice, message, dashboard, or export dependencies.
- Destructive-cleanup blockers.

Write reviewed notes to `docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md`, referencing the raw JSON. Stop if `named_stay_overlap_pairs` is non-zero. No override exists for severe overlaps. Do not fabricate either artifact.

### Historical Apply Steps

Run apply only after the reviewed production dry run is approved. Reuse the exact image, database path, and host bind mount from the dry run:

```bash
podman run --rm \
  --name pms21-migration-apply \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z \
  --entrypoint /app/pms21-migration \
  ghcr.io/ai-slop-code/pms-backend:latest \
  --db /data/pms.db \
  --apply \
  --confirm-apply \
  --allow-review-required \
  --sample-limit 25 \
  > /absolute/path/to/PMS_21_production_apply_YYYY-MM-DD.json
```

The guarded apply runs pending embedded additive schema migrations before Stage 2 data changes. Run it a second time and save the idempotency artifact:

```bash
podman run --rm \
  --name pms21-migration-idempotency \
  -v /mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z \
  --entrypoint /app/pms21-migration \
  ghcr.io/ai-slop-code/pms-backend:latest \
  --db /data/pms.db \
  --apply \
  --confirm-apply \
  --allow-review-required \
  --sample-limit 25 \
  > /absolute/path/to/PMS_21_production_apply_idempotency_YYYY-MM-DD.json
```

All created and updated-link counts in the second artifact must be zero.

After the second apply passes, recreate the normal API container with the existing production command:

```bash
podman rm api.pms.airportlounge.sk
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
  --env-file ./pms.env \
  --health-cmd '/app/pms-healthcheck' \
  --health-interval=30s \
  --health-timeout=5s \
  --health-retries=3 \
  ghcr.io/ai-slop-code/pms-backend:latest
```

`--apply` requires an explicit absolute `--db` path and will not accept `DATABASE_PATH`. Omit `--allow-review-required` only if the dry run has zero review-required candidates. When supplied, those candidates are created as `needs_review`, never silently confirmed.

Apply checks:

- Run additive database migrations required by the new binary before apply. The guarded `pms21-migration --apply` command now performs this step automatically after explicit confirmation.
- Save the apply output artifact with created, updated, skipped, conflict, and review-required counts.
- Run the same apply command a second time, saving `PMS_21_production_apply_idempotency_YYYY-MM-DD.json`; created and updated-link counts must be zero and no business data may change.
- Stop if Nuki PINs, external Nuki IDs, invoices, finance links, Google event IDs, or message history would be lost.

### Historical Deployment Steps

- Deploy the new PMS 21 backend/frontend only after successful migration apply and verification.
- Recreate the backend with the production `podman run` command above; do not introduce another deployment mechanism for this cutover.
- Verify the recreated container uses `ghcr.io/ai-slop-code/pms-backend:latest` and the `/mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z` bind mount.
- Run `podman healthcheck run api.pms.airportlounge.sk` and inspect `podman logs api.pms.airportlounge.sk` before resuming write-heavy jobs.
- Resume jobs in a safe order, starting with read-only/listing checks before write-heavy syncs where possible.
- Run Booking.com ICS sync and verify raw blocks and source-link warnings behave as expected.
- Run or trigger Nuki sync and verify named-stay-primary code generation/listing without legacy occupancy dependency.
- Run cleaning reconciliation for a narrow date range and verify named-stay/raw-block ownership fields.

### Historical Verification Steps

- Verify representative Booking.com payout/reservation-derived historical rows became confirmed named stays.
- Verify non-payout/non-reservation stay-like rows are review-required, not silently confirmed.
- Verify `named_stay_nights` exists and drives analytics results.
- Verify calendar KPI matches backend analytics semantics for sold nights.
- Verify frontend can create, edit, cancel, archive, reactivate, and toggle cleaning for named stays.
- Verify Nuki upcoming stays, active codes, dashboard widget, and guest daily entries do not require new legacy occupancy rows.
- Verify invoices, finance mappings, messages, cleaning calendar, and dashboard rows reference named stays where expected.
- Verify deprecated legacy endpoints either still work as compatibility or are intentionally disabled according to the release plan.

### Historical Rollback Points

- Before migrations/apply: restore old binary and continue with the original database.
- After additive migrations but before apply: old binary may continue if additive tables/columns are compatible; keep additive objects in place unless a tested restore is chosen.
- After apply but before new binary traffic: prefer restoring the pre-apply backup if rollback is required.
- After new binary traffic: rollback requires an owner decision. The new version may write named-stay-primary data; restoring the old database backup can lose post-cutover writes.

Collapsed cutovers roll back by deploying the prior version. Do not attempt to flip undocumented per-area flags.

### Historical Post-Cutover Monitoring

- Monitor Nuki generation errors.
- Monitor source-link conflicts and source-deleted warnings.
- Monitor cleaning reconciliation errors.
- Monitor finance import/rematch errors.
- Monitor invoice creation/regeneration.
- Monitor message generation.
- Monitor analytics/dashboard mismatches.
- Review all `needs_review` named stays created by migration.
- Keep database backups from before and after cutover.

### Historical Legacy Cleanup Eligibility

- Do not drop legacy occupancy columns/tables/routes/token storage immediately after cutover.
- Cleanup becomes eligible only after the PMS 21 version runs successfully for an agreed release window and no rollback to the old binary is expected.
- Before destructive cleanup, run a cleanup readiness audit proving no required production behavior still depends on legacy-only data.
- Cleanup instructions must list exact objects to remove, backup requirements, restore implications, and tests to run after removal.
