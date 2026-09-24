# PMS 30 — Cleaning calendar database error fix plan

## 1. Purpose and status

Investigated on 2026-09-24 against the current working tree, the user-supplied access log, and read-only queries of `data/pms.db`. The user confirmed that the available code version and database match production. Audience: the coding agent implementing the incident fix.

**Status: root cause confirmed; packaged repair tooling implemented and locally built.** The event-list query was reproduced with `sqlite3 -readonly`. Stored sync-run evidence confirms the same failure in scheduled synchronization. Google Calendar was not inspected and the supplied database was not modified. The initial specification-only investigation was followed by implementation at the user's request; see section 7 for changes and verification.

Follow `PMS_13_Coding_Conventions.md`. `PMS_22_Named_Stay_Only_Cleaning_Calendar_Spec.md` remains the authority for cleaning ownership, eligibility, creation horizon, retry, and deployment cleanup. This incident does not supply new business requirements.

## 2. Incident evidence

The user reports calendar sync appears not to work and the UI displays `database error`. Supplied log:

```json
{"time":"2026-09-24T18:03:42.183074+02:00","level":"INFO","msg":"GET /api/properties/1/cleaning-calendar/events?month=2026-09 500 27B 618.708µs","source":"log_printf"}
```

This establishes an HTTP 500 on the event-list request. It does not contain the underlying database error or establish the result of a Google synchronization attempt. Request duration is not sufficient to identify the cause.

### 2.1 Confirmed source path

| Area | Source | Finding |
|---|---|---|
| Event-list handler | `backend/internal/api/cleaning_calendar_handlers.go`, `listCleaningCalendarEvents` | After property access, property lookup, and month parsing, calls `ListCleaningCalendarEventsForMonth`. On error returns HTTP 500 with `database error`, without logging the error. Does not call Google. |
| Month query | `backend/internal/store/cleaning_calendar.go`, `ListCleaningCalendarEventsForMonth` | Selects `cleaningCalendarColumns` from `cleaning_calendar_events`, filtered by property and `substr(cleaning_date, 1, 7)`, ordered by date/start. |
| Shared scanner | Same file, `scanCleaningCalendarEvents` | Propagates query, row-scan, and row-iteration errors. RFC3339 parse errors are currently ignored, so those parse errors do not explain this returned database error. |
| Schema requirement | Same file, `cleaningCalendarColumns`; `backend/internal/migrate/000040_named_stay_only_cleaning_calendar.up.sql` | Query requires `pending_action` and `schedule_hash`; migration 40 introduces these columns. The projection and scanner each contain 23 entries and align by source inspection. |
| Startup migration path | `backend/cmd/server/main.go`; `backend/internal/migrate/migrate.go` | Server calls `UpStartup`. Initialized databases use `UpAutomatic`, which explicitly skips migrations 39 and 40. Fresh databases use `Up`. |
| Cleanup prerequisite | `backend/internal/migrate/migrate.go`, `ensurePMS22CleanupReady` | Migration 40 checks the existing PMS-22 cleanup state for databases containing cleaning events. |
| Reconciliation read | `backend/internal/cleaningcalendar/service.go`, call to `ListActiveCleaningCalendarEvents` | Reconciliation also uses the shared event projection/scanner. A schema mismatch could affect both listing and sync; this is not proof of the reported sync outcome. |
| Logging | `backend/internal/logging/logging.go` | Existing process-wide `slog` configuration bridges `log.Printf` at INFO level with `source=log_printf`. The supplied INFO level does not imply the request succeeded. |

### 2.2 Confirmed root cause and database evidence

The current code is running against a database that has not completed the PMS-22 schema transition. Read-only inspection established:

- `schema_migrations` contains migration 39, 41, and 42, but **not** `000040_named_stay_only_cleaning_calendar`. Checking the highest migration number would miss this gap.
- `PRAGMA table_info(cleaning_calendar_events)` shows neither `pending_action` nor `schedule_hash`; the legacy `raw_booking_block_id` column remains.
- Executing the exact 23-column monthly SELECT from `ListCleaningCalendarEventsForMonth` for property `1`, month `2026-09`, fails during SQL preparation with **`no such column: pending_action`**. No row scan or Google request is needed to reproduce it.
- Latest recorded run, ID `10815`, has `status=failure`, `trigger=scheduled`, and `error_message=SQL logic error: no such column: pending_action (1)`. This independently establishes a synchronization failure with the same cause. Earlier success/running entries do not negate it.
- Property 1 has **95 named-stay cleaning rows** and **70 provisional-block cleaning rows**. There are **27 total cleaning rows** for September 2026 before cleanup; this is an inventory count, not the required post-cleanup count.
- No tables matching `cleaning_calendar_cleanup%` exist. The database therefore has no current recorded cleanup work state, and migration 40 is not recorded as complete. This does not establish whether anyone previously attempted remote cleanup.

The intentional automatic-migration skip explains why later ordinary migrations can be installed while the required manual transition remains absent. The repair is the existing cleanup/schema alignment procedure, with focused diagnostic logging to expose future failures.

## 3. Evidence collection and execution boundary

The user's confirmation resolves the earlier question about whether the local database/code represent production. No further user input is needed to identify this cause or select the repair plan.

Read-only inspection used `sqlite3 -readonly data/pms.db` for migration history, table shape, the exact monthly SELECT, aggregate cleaning counts, cleanup-table presence, and the latest five sync-run statuses/errors. Credentials and guest records were not read.

Implementation must recheck the migration state before executing the repair because the installation may change after this investigation. Google access, actual remote inventory, and cleanup completion must be established through the existing PMS-22 tooling during the deployment procedure. This specification records a plan; cleanup, remote deletions, migration, and reconciliation were not executed during investigation.

## 4. Coding-agent implementation sequence

### Step 1 — Make the failing operation diagnosable

- Inspect the then-current diff and deployed/source revision relationship.
- In `listCleaningCalendarEvents`, log the returned error at error level using the existing process-wide logger. Include operation, property ID, and parsed month so the failure can be correlated with the supplied request.
- Keep the existing HTTP 500 and generic client error contract. Log the technical cause server-side without adding guest records, tokens, or credentials to the message.
- If the raw driver error lacks enough context, add focused query/scan/iteration context in the shared scanner using wrapped errors (`%w`). Preserve the underlying error.
- Add regression coverage using the confirmed missing-column failure. The raw SQL and stored sync error already establish the cause; further diagnostic deployment is not a prerequisite to selecting the repair. Logging alone does not constitute the incident fix.

### Step 2 — Complete the confirmed missing PMS-22 transition

**Required repair for the inspected database:**

- Confirm the full recorded migration state and actual table shape, including migration 39, rather than checking only the highest version.
- Follow the existing offline cleanup and deployment procedure in PMS 22 and its current tooling (`backend/cmd/cleaning-calendar-cleanup/`). Establish the actual cleanup status and deployment context with the operator.
- Treat this as a deployment/schema alignment repair unless a reproducible defect in existing tooling is identified. Do not invent a replacement migration or modify migration history merely to silence the query.
- Preserve the intentional manual-migration boundary and cleanup prerequisite. Do not make migrations 39/40 automatic, add just the missing columns ad hoc, or clear calendar rows as a workaround.
- Follow PMS 22 section 9.4: stop backend instances/schedulers and writes, take the existing deployment backup, prepare cleanup, inspect status, run cleanup to completion, apply the existing forward migration, verify named-row/Google-reference/log preservation and foreign-key integrity, then restart and verify synchronization. Use the production-configured calendar and database. Do not infer the remote deletion count from the 70 local provisional rows; the existing cleanup discovery and cutoff rules determine remote scope.
- Verify that migration 40 is now recorded, both required columns exist, named events are retained, and provisional local rows are removed according to the existing specification. Resolve cleanup errors through the existing resumable tooling rather than bypassing its checks.

**Only if execution-time evidence differs from this inspection (migration 40 recorded but schema differs):**

- Record the precise discrepancy and deployment history with the operator. Investigate against a test fixture/copy before choosing a correction. A migration record alone is not proof that the live table matches it.
- Specify a corrective migration only after the discrepancy and preservation requirements are understood; follow repository migration conventions. Do not rewrite an already-applied migration or fabricate a migration record.

**Only if a different error remains after alignment:**

- Use the actual query/scan/iteration error to reproduce the failure in a minimal fixture, then implement the smallest repair in the responsible layer.
- Do not add speculative NULL coercion, empty-result fallbacks, retries, Google credential changes, or frontend rewrites. Ask for missing evidence if the cause cannot be reproduced or explained.

Update this document with repair execution results before declaring the incident resolved.

### Step 3 — Verify listing and synchronization separately

- Reissue `GET /api/properties/1/cleaning-calendar/events?month=2026-09` on the repaired installation and verify HTTP 200 with the existing `{month, events}` response shape and the expected persisted events. An empty month must return an empty array, not an error.
- Verify the actual sync outcome using the existing reconcile endpoint/run history and expected Google event state. Record the response body as well as HTTP status: `runCleaningCalendarReconcile` currently returns HTTP 200 with `ok: false` on service failure.
- Use eligible named-stay fixtures within PMS-22's current creation horizon for sync verification. Listing September history does not authorize recreation of past cleaning events.

## 5. Regression coverage and checks

| Coverage | Required result |
|---|---|
| Monthly store read on current migrated schema | Seed a named-stay-owned event; verify property/month filtering, field scanning, and an empty-month result through the real database query. |
| Event-list API | Verify authenticated authorized access returns expected DTOs and an empty array for no events. Use existing API test setup. |
| Error diagnostics | Induce a deterministic event-store failure in an isolated fixture; verify generic HTTP 500 and a server-side error containing operation/property/month and the underlying cause. Do not satisfy this with only an access-log assertion. |
| Confirmed incident regression | Add a fixture reproducing the confirmed error before the fix. If migration mismatch is confirmed, cover the relevant existing-database startup/manual-migration path and successful read after the prescribed transition. Preserve cleanup prerequisites. |
| Sync regression | Reuse `backend/internal/cleaningcalendar/service_test.go` and its fake Google client to verify shared reads and idempotent reconciliation still work. No live Google call is needed in automated tests. |

Expected implementation areas are `backend/internal/api/cleaning_calendar_handlers.go`, focused API/store tests, and, only when supported by evidence, the responsible store, migration, or deployment tooling. `backend/internal/api/cleaning_calendar_dto_test.go` is existing DTO coverage; it does not establish the monthly database read works.

Run from `backend/` after implementation:

```sh
go test ./internal/store ./internal/api ./internal/cleaningcalendar ./internal/migrate
go test ./...
```

Record outcomes, the deployed revision/schema evidence, and operational verification. Tests passing against a fresh schema do not prove an existing installation completed the manual transition.

## 6. Acceptance and completion

- [x] Underlying incident error and deployed/schema context are recorded; missing migration 40 is confirmed by schema inspection, exact-query reproduction, and sync-run evidence.
- [ ] The confirmed cause is repaired with existing PMS-22 business and deployment rules preserved.
- [ ] The reported property/month request succeeds with correct data.
- [ ] Future event-list database failures produce actionable server-side logs while retaining the API error contract.
- [ ] Relevant automated regressions pass.
- [ ] Actual synchronization outcome is independently verified; any separate failure is reported with its own evidence.

The plan is ready for implementation and the existing PMS-22 deployment repair. Root-cause investigation is complete; repair execution and post-repair verification remain outstanding.

## 7. Confirmed production deployment and executable runbook

The user supplied the production launch command: standalone Podman container
`api.pms.airportlounge.sk`, image `ghcr.io/ai-slop-code/pms-backend:2.6.5`, network
`internet_enabled`, host directory
`/mnt/main_storage/containers/data/api.pms.airportlounge.sk` mounted at `/data:Z`,
and `--env-file ./pms.env`. The container runs read-only with `/tmp` tmpfs,
dropped capabilities, and no-new-privileges.

The original 2.6.5 image packages neither `cleaning-calendar-cleanup` nor a
standalone PMS-22 migration command. The user requested that these tools ship in
the main backend image rather than building a separate maintenance image.

### 7.1 Implemented changes

- `deploy/Dockerfile.backend` now builds and packages
  `/app/cleaning-calendar-cleanup` and `/app/pms-migrate` beside the server.
- `backend/cmd/migrate/main.go` provides `pms-migrate cleaning-calendar`. It uses
  the backend environment and requires an explicitly configured existing
  database file. It does not silently create a fresh database at a wrong path.
- `migrate.UpCleaningCalendar` requires migration 39 and applies only migration
  40 through the existing transactional runner. It retains the cleanup readiness
  checks and does not apply unrelated migrations. Repeating it after success is
  a no-op. Startup's manual-migration exclusions remain in place.
- The production-shaped regression fixture uncovered a second concrete bug:
  migration 40's parent/child table rebuild failed with `FOREIGN KEY constraint
  failed` when named-event logs existed. The runner now uses transaction-scoped
  `PRAGMA defer_foreign_keys = ON` for migration 40, enforcing foreign keys at
  commit after both rebuilt tables exist. No applied SQL migration was rewritten.
- Tests reproduce the missing-column failure with later ordinary migrations
  installed, require cleanup completion/current calendar/no pending work, and
  verify named-event IDs/Google references/logs survive while provisional
  records/logs are removed. They also cover repeat execution, migration scope,
  the PMS-21 prerequisite, and CLI argument/path handling.

The replacement production procedure is
`docs/deployment/pms-30-podman-calendar-repair.md`. All operator commands and the
replacement server use the same new main backend image. The temporary-image and
generated-wrapper procedure is superseded and removed from the runbook.

### 7.2 Verification and delivery status

- `go test ./...` passed from `backend/`.
- The normal Dockerfile built successfully with Podman for Linux/amd64, tagged
  locally as `localhost/pms-backend:pms-30`.
- The image's `/app/pms-migrate --help` ran successfully; the packaged cleanup
  binary ran and returned its expected usage/exit code with no arguments.
- No production database migration, Google cleanup, server replacement, commit,
  release tag, or image publication was performed. The local image is not a new
  GHCR release. Production execution and the separate event-list diagnostic
  logging task remain outstanding.
