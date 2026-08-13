# PMS 21 - Legacy Occupancy Removal Specification

Status: Final cleanup specification. Production data backfill is reported complete. This document defines future code and schema cleanup; production execution still requires every eligibility gate and operational approval below.

Authority for final cleanup: this specification. `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md` remains the historical staged-migration source and must not override this document where its pre-production status, compatibility, flag, or rollback language is stale.

Related decisions:

- `docs/adr/ADR-002-raw-booking-blocks-and-named-stays.md`
- `docs/adr/ADR-003-cleaning-event-ownership.md`
- `docs/adr/ADR-004-stay-type-reporting-semantics.md`
- `docs/adr/ADR-005-occupancy-compatibility-window.md`
- `docs/adr/ADR-006-finance-import-named-stay-behavior.md`

Operational companion: convert `docs/pms-21-operations-cutover-runbook.md` from the initial-cutover procedure into the active Release A-D cleanup runbook while preserving the historical cutover evidence it references.

## 1. Purpose

PMS 21 introduced first-class raw Booking.com blocks, named stays, named-stay nights, raw-block nights, source links, and non-stay availability blocks. The old `occupancies` model remains in the repository as a compatibility layer for migration, rollback, deprecated APIs, historical integrations, and old frontend workflows.

This specification defines how to remove that compatibility layer so the codebase has one coherent model:

- Booking.com ICS synchronization writes `raw_booking_blocks` and `raw_booking_block_nights`.
- User-controlled stay truth lives in `named_stays` and `named_stay_nights`.
- Booking.com provenance is represented by `stay_source_links`.
- Non-stay closures live in `property_availability_blocks`.
- Nuki, cleaning, finance, invoices, messages, dashboard, and analytics use new-model identities only.
- No active API or frontend contract exposes `occupancy_id` as a stay identity.
- No runtime code reads or writes the legacy `occupancies` or `occupancy_nights` tables.

The objective is removal, not another long-lived dual-mode period. Compatibility is removed in controlled releases, followed by one destructive forward migration after runtime independence is proven.

## 2. Current Evidence And Cleanup Status

The repository contains PMS 21 production artifacts dated 2026-07-19:

- `docs/audits/PMS_21_production_data_audit_2026-07-19.json`
- `docs/audits/PMS_21_production_apply_2026-07-19.json`
- `docs/audits/PMS_21_production_apply_idempotency_2026-07-19.json`

The apply artifact records:

- 211 created named stays.
- 418 legacy-to-new migration-map rows.
- 161 auto-confirmed named stays.
- 50 `needs_review` named stays.
- 49 relinked Nuki code rows.
- 37 relinked Nuki guest-entry rows.
- 68 relinked cleaning rows: 35 named-stay-owned and 33 raw-block-owned.
- 206 relinked finance rows.
- 8 cleaning rows with no new-model ownership candidate.
- 1 external-sale classification conflict.
- Zero named-stay overlap conflicts.
- Zero creates and zero link updates on the recorded second apply.

The owner reports as of 2026-07-22 that production, including its data, has been fully migrated and normal operation appears healthy. This is the starting operational state for this cleanup definition. It does not replace the reproducible cleanup-readiness evidence required below.

These artifacts establish that the backfill ran and was idempotent for the recorded database. They do not establish cleanup eligibility. The following evidence is still absent or unresolved in the repository:

- A reviewed production audit Markdown artifact approving the JSON findings.
- A recorded resolution for the eight unmapped cleaning rows.
- A recorded resolution for the external-sale conflict.
- Row-level resolution evidence for the 50 `needs_review` stays.
- A cleanup-readiness audit proving zero runtime and data dependencies.
- A recorded successful release window with legacy writes disabled.
- A record of exact deployed backend/frontend versions and PMS 21 flag values.
- A caller inventory proving deprecated APIs and public export are unused.
- A reconciliation of audit/apply counter differences, including raw-block create/skip counts, Nuki and finance link forecasts versus updates, and dry-run unmatched finance rows versus apply results.
- A row-level exception register identifying every approved non-zero exception, its disposition, approver, approval time, and evidence.
- A value-level preservation report for Nuki, cleaning/Google, finance, invoices/files, analytics, and legacy-only metadata.

Therefore, repository evidence does not yet establish destructive-cleanup eligibility even though the production backfill is complete and operation appears healthy.

## 3. Scope

This specification covers:

- Removing old occupancy writes, reads, fallback queries, identity adapters, routes, DTO fields, UI flows, tests, and one-off repair tooling.
- Removing legacy access from server startup, schedulers, health/readiness paths, admin/support commands, migration helpers, and rarely used workflows.
- Removing the public occupancy export and token-management feature.
- Removing rollback-only migration-map lookups from runtime paths.
- Rebuilding integration tables without legacy `occupancy_id` foreign keys.
- Dropping `occupancies`, `occupancy_nights`, and `occupancy_api_tokens` after dependency proof.
- Retaining required Booking.com source configuration, raw source evidence, operational history, and integration history.
- Aligning documentation, OpenAPI, generated types, and naming with the final model.
- Reconciling remote Google and Nuki state where local legacy identity removal affects discovery or verification.
- Defining backup, archive, restore, evidence-retention, and destructive-migration verification requirements.

This specification does not cover:

- Rewriting historical migration files.
- Deleting audit, finance, invoice, Nuki, Google Calendar, or message history.
- Hard-deleting named stays that have business or integration references.
- Replacing SQLite.
- Adding another set of long-lived runtime cutover flags.
- Running commands against production as part of implementation.

## 4. Terminology

| Term | Meaning |
| --- | --- |
| Legacy occupancy | A row in `occupancies`, or code that treats `occupancies.id` as the identity of a stay, raw block, closure, or finance-derived booking. |
| New model | `raw_booking_blocks`, `raw_booking_block_nights`, `named_stays`, `named_stay_nights`, `stay_source_links`, and `property_availability_blocks`. |
| Compatibility code | Runtime behavior that writes, reads, translates, or exposes old occupancy identities only to support migration or rollback. |
| Historical attribution | An old identifier retained as immutable provenance, not used to execute current business behavior. |
| Cleanup readiness audit | A read-only report produced immediately before destructive cleanup that proves every required zero-count and records approved exceptions. |
| Release window | The owner-approved production observation period during which the new model runs without creating or requiring legacy occupancy data. |
| Exception register | A row-level, reviewable artifact containing the source identity, exception class, disposition, approver, approval time, and supporting evidence for every approved non-zero gate. |
| Inert legacy storage | Legacy tables that remain physically present but are not queried or mutated by application startup, runtime, jobs, APIs, tools, or external consumers. |

## 5. Non-Negotiable Final Invariants

### 5.1 Domain Ownership

- ICS sync owns raw booking blocks and raw-block nights only.
- ICS sync may update source-link health but must not resize, rename, classify, cancel, archive, or delete a named stay.
- Named stays are the only stay identity used by Nuki, finance, invoices, messages, dashboard stay widgets, analytics, and final cleaning.
- Raw blocks never count as sold/revenue nights and never generate Nuki access.
- Non-stay closures reduce bookable availability through `property_availability_blocks` and never masquerade as stays.
- `named_stay_nights` is the capacity and day-level analytics source of truth.

### 5.2 Identity

- New runtime request and response contracts use `stay_id`, `named_stay_id`, `raw_booking_block_id`, `availability_block_id`, or integration row IDs as appropriate.
- No API accepts `occupancy_id` as an alias for `named_stay_id`.
- No frontend state variable uses `occupancy_id` to hold a named-stay ID.
- No runtime query joins through `occupancy_stay_migration_map` to find current business truth.
- Historical old IDs may remain in existing audit payloads only when clearly named `legacy_occupancy_id` and never used for behavior. No live old-to-new attribution archive is retained.

### 5.3 Dates And Status

- Stay and block ranges remain half-open: check-in inclusive, check-out exclusive.
- Property-local ISO dates remain the business-date representation.
- At most one active named stay may own a property night.
- Cancelled and archived stays have no active named-stay nights.
- `needs_review`, maintenance, personal use, and unfunded external stays reduce availability according to PMS 21 rules but do not inflate sold/revenue metrics.
- `cancelled_non_refundable` and `no_show` follow PMS 17: they count as sold/occupied, retain actual imported revenue, and are excluded from the normal cancellation-rate numerator and denominator.
- A canonical per-stay first-known/booking timestamp and required cancellation-effective timestamp preserve pace, lead-time, and cancellation statistics; migration-time `named_stays.created_at` is not an acceptable substitute for historical `occupancies.imported_at`.

### 5.4 Data Preservation

- Existing Nuki row IDs, exact stored PIN values, external Nuki IDs, valid windows, statuses, revocation timestamps, run links, and event-log references are preserved. PIN encryption/backfill and key-restore validation are not cleanup gates. Occupancy cleanup must not decrypt/re-encrypt, regenerate, or discard PINs.
- Existing cleaning row IDs, Google Calendar IDs, Google event IDs, deterministic cleaning identities, desired hashes, statuses, warnings, errors, and event logs are preserved.
- Existing finance booking IDs, raw import evidence, transaction links, reset/import history, and named-stay links are preserved.
- Existing invoice IDs, invoice numbers, sequence values, snapshots, files, and finance/named-stay links are preserved.
- Existing audit logs and migration artifacts are preserved.
- Applied SQL migration files remain immutable and present in the repository.
- Repository artifacts contain no real production PINs, token secrets, secret-bearing or live ICS URLs, real guest names, credentials, or unrestricted production payloads. Synthetic fixtures and clearly redacted examples are permitted; raw evidence is stored in an approved restricted location and repository summaries are redacted.

### 5.5 Authorization And Tenant Integrity

- Replacement APIs preserve or strengthen the existing Occupancy-module authorization rules; identity renaming must not widen access.
- Every integration-to-stay, integration-to-block, source-link, and night attribution belongs to the same property as its owner.
- Final schema constraints and runtime validation reject cross-property ownership even where the current single-column foreign keys do not. Composite property/ID foreign keys are the preferred database enforcement; triggers are allowed only when SQLite cannot express an equivalent declarative constraint.

## 6. Cleanup Eligibility Gates

Every gate in this section is mandatory. A coding agent must not interpret passing one group as permission to skip another.

### 6.1 Data Gates

- `PRAGMA foreign_key_check` returns no rows and `PRAGMA integrity_check` returns `ok` before cleanup.
- Every Nuki access code has a non-null same-property `named_stay_id`.
- Every Nuki guest daily entry has a non-null same-property `named_stay_id`.
- Every finance booking has a non-null same-property `named_stay_id`; unmatched import evidence remains in import staging/rejection evidence and is not committed as an ownerless `finance_bookings` row.
- Every invoice has a non-null same-property `named_stay_id`. Its finance-booking link is optional, but when present it belongs to the same property and references the same named stay.
- Every cleaning row has exactly one same-property `named_stay_id` or `raw_booking_block_id`, appropriate to its cleaning kind; rows with both owners, neither owner, or a cross-property owner are zero before cleanup.
- No cleaning row requires `next_occupancy_id` to preserve same-day-turnover behavior.
- Every active legacy closure maps to an availability block or an explicitly verified new-model equivalent.
- Every external-sale legacy row maps to a named stay or an explicitly approved non-stay disposition.
- Every stay outcome required for behavior/statistics is represented on `named_stays` before old fields are dropped; legacy reason/actor/audit metadata is intentionally discarded unless already canonical.
- No migration-map row remains `migration_kind = 'unmapped'`.
- Every migration-map row has exactly the target cardinality required by `migration_kind` and references an existing same-property target before the map is dropped.
- Every relevant legacy occupancy has a same-property target; existence of a map row alone is insufficient.
- Every `needs_review` stay is individually resolved before Release C; cleanup must not bulk-confirm rows merely to reach zero.
- Active named stays have exactly the expected half-open set of active `named_stay_nights`; cancelled and archived stays have none.
- Raw block ranges, statuses, UIDs, and active night coverage match retained source evidence or have an approved exception.
- Availability-block mappings preserve exact property and date coverage, not only the existence of a target row.
- Stay-source links reference valid same-property rows and their status and linked range agree with raw-night coverage.
- No finance booking or invoice is unlinked from a named stay.
- Pre-cleanup comparisons prove field-level parity for all statistics-bearing fields and effective operational state. Legacy-only reasons, actors, timestamps not used by retained statistics, representation/supersession details, and old IDs are intentionally discarded after parity is established.

### 6.2 Runtime Independence Gates

- Booking.com sync can run repeatedly without inserting, updating, deleting, or querying `occupancies` or `occupancy_nights` for reconciliation.
- Named-stay create, edit, status, cleaning, outcome, review, Nuki, finance, and invoice workflows do not write compatibility occupancy rows or migration-map rows.
- Analytics has no legacy fallback branch.
- Cleaning desired-state generation has no legacy occupancy fallback.
- Nuki generation, listing, synchronization, revocation, keypad reconciliation, dashboard display, and guest daily logs have no occupancy fallback.
- Finance import, rematch, explicit link, cancellation review, and reset have no synthetic occupancy creation path.
- Invoice creation, editing, candidates, regeneration, and download metadata have no occupancy fallback.
- Message generation and pickers do not accept or resolve occupancy IDs.
- Dashboard stay and Nuki DTOs work when all legacy occupancy IDs are absent.
- The frontend provides every supported lifecycle action through new-model APIs.
- Server startup performs no legacy backfill, query, or write; `Store.BackfillUpstreamOwnership` or any replacement startup repair is retired from normal startup.
- Schedulers, health/readiness checks, manual jobs, admin/support tools, migration helpers, and rarely used routes intended to remain supported after Release B execute without legacy access. One-shot PMS 21 backfill and legacy-repair executables are explicitly quarantined transitional tools: they are never invoked by startup or normal production operation, are excluded from this access-denial gate through Release B, and are removed in the combined Release C/D.
- Calendar v2 and cleaning attachment queries do not use the migration map or legacy IDs.
- No normal operation writes legacy integration columns, including `occupancy_id`, `next_occupancy_id`, `source_occupancy_id`, or Google private metadata `pms_occupancy_id`.
- Runtime SQL tracing covers reads and writes across representative startup, scheduled, manual, and user workflows for every configured property and integration.

### 6.3 API And Caller Gates

- Available access logs and repository/deployment caller inventory show no use of deprecated occupancy-as-stay routes. The owner has confirmed there are no consumers; no additional deprecation waiting period is required.
- No automation uses `/api/properties/{id}/occupancy-export`.
- All occupancy export tokens are retired; token owners do not require notification or a grace period.
- OpenAPI contains no legacy route or compatibility field scheduled for removal.
- Generated frontend API types have been regenerated from the cleaned OpenAPI contract.
- All affected operations have concrete request and response contracts; `x-contract-status: route-only` is not accepted as proof that an undocumented legacy alias was removed.
- Repository-wide searches find no behavior-bearing `occupancy_id`, `legacy_occupancy_id`, or `old_occupancy_id` use outside historical migrations, archives, audit documents, and explicitly retained provenance types.
- Runtime contract tests prove removed request fields are rejected and removed response fields are absent, independently of OpenAPI route coverage.
- Caller evidence includes public traffic, internal automation, support scripts, reporting jobs, cached/older frontend builds, and all deployed workers or containers.

### 6.4 Operational Gates

- The exact PMS 21 backend image, frontend version, database backup, and migration artifacts are recorded.
- Release A and Release B each complete a minimum 48-hour production observation window.
- Each window includes a server restart, repeated ICS sync, named-stay lifecycle change, cleaning reconciliation, Nuki reconciliation, finance import/rematch, invoice create/regenerate, message generation, and analytics/dashboard verification. A controlled smoke operation may satisfy a workflow that does not occur naturally.
- Each window has zero unexplained new-model errors, zero legacy reads/writes where prohibited by that release, and no statistics variance outside explicitly approved rounding. A relevant incident, rollback, or code change restarts that release's 48-hour window.
- Rollback to the pre-PMS-21 binary is formally retired.
- Pre-cutover and pre-cleanup backups remain pinned until Release C/D acceptance, then return to the existing backup retention and security policy; no separate long-term legacy archive is required.
- A restore drill has been recorded for the cleanup backup.
- The cleanup backup passes `PRAGMA integrity_check`, can be restored, migrated, and used to start the compatible application in a safe environment.
- The standalone Podman deployment procedure in `docs/pms-21-operations-cutover-runbook.md` matches the real `api.pms.airportlounge.sk` container, bind mount, database path, environment file, network, and image.
- Cleanup uses an immutable image digest and records the image digest, commit, frontend build, schema version, database fingerprint, effective flag values, operator, commands, and timestamps.
- SQLite WAL/SHM handling, checkpoint/consistent snapshot method, free-disk requirement, expected migration duration, lock duration, and maintenance-window fit are recorded and verified.
- Production has no older API container, worker, cron job, support binary, or external reporter that can access or recreate legacy data.
- Remote Google and Nuki verification is complete, including orphan detection, stable identifiers, and confirmation that no non-PMS Google event was modified.
- Release-window acceptance defines duration, representative workload, metrics, thresholds, incident restart rules, and named approver.

## 7. Prerequisites That Must Be Closed Before Removal

### 7.1 Make ICS Sync New-Model Only

Current sync calls `Store.ReconcileBookingICSSync`, which still reconciles legacy `occupancies` and conditionally adds raw-block dual-write. `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` does not disable this main legacy ICS write path.

Required result:

- Raw-block upsert, raw-night replacement, disappearance handling, and source-link health recomputation become the sole reconciliation path.
- Existing safe parsing, raw-event snapshots, partial-parse protection, per-property leases, and sync-run reporting remain.
- Sync-run counters and naming stop presenting `occupancies_upserted` as the primary result; replace or reinterpret it through a deliberate contract migration.
- `PMS21_RAW_BLOCKS_DUAL_WRITE` is removed after raw-block writes are unconditional.

Primary files:

- `backend/internal/occupancy/sync.go`
- `backend/internal/store/occupancy_reconciliation.go`
- `backend/internal/store/occupancy.go`
- `backend/internal/store/occupancy_nights.go`
- `backend/internal/store/named_stays.go`
- `backend/internal/store/occupancy_reconciliation_test.go`

### 7.2 Replace Remaining Legacy-Only Mutations

Before deprecated occupancy routes can disappear, named stays must support all business mutations still available only through old routes.

Required result:

- A named-stay outcome operation sets and clears `cancelled_non_refundable` and `no_show`, including reason, actor, and timestamp.
- A named-stay review operation confirms or otherwise resolves `needs_review` without direct database edits.
- Existing cleaning and Nuki side effects run for these state changes according to PMS 21 rules.
- Audit logs use `named_stay` entity identity.

Primary files:

- `backend/internal/api/occupancy_named_stay_handlers.go`
- `backend/internal/api/occupancy_closure_handlers.go`
- `backend/internal/store/named_stays.go`
- `frontend/src/views/OccupancyView.vue`
- `spec/openapi.yaml`

### 7.3 Resolve Production Exceptions

The eight unmapped cleaning rows and one external-sale conflict require row-level correction, new-model linking, or documented deletion when proven disposable test data. All 50 `needs_review` stays must be individually resolved. Absence of a mapping is not permission to delete data, and review rows must not be bulk-confirmed merely to satisfy the gate.

### 7.4 Preserve Statistics-Bearing Data

The owner does not require a legacy metadata archive. Before dropping `occupancies`, a field-level crosswalk must prove that the final model preserves:

- Property, half-open stay/block ranges, and exact active night sets.
- Named-stay display name, stay type, status, review status, and stay outcome.
- Availability-block type, range, and status.
- Finance-to-stay links, imported amounts/currency/status, manual external revenue/currency, and invoice-to-stay links.
- Effective cleaning-required state needed by current operation.
- Raw source UID/range/status/timestamps in retained raw blocks, source links, raw events, and sync runs.
- A canonical historical first-known/booking timestamp and required cancellation-effective timestamp for pace, lead-time, cancellation, and ADR-by-lead-time statistics. `named_stays.created_at` set during migration is not sufficient.

After this parity is proven, cleanup intentionally discards legacy-only closure/external-sale/outcome/cleaning reasons and actors, historical action timestamps not required by retained statistics, `external_channel` when not already canonical, representation/supersession metadata, old occupancy IDs, and duplicate source fields from `occupancies`. Do not add compatibility columns merely to retain discarded implementation details.

The current Stage 2 loader did not copy every statistics-bearing timestamp. Successful backfill alone is not evidence of statistical parity.

### 7.5 Retire Startup And Background Compatibility

Normal server startup currently invokes `Store.BackfillUpstreamOwnership`, and background/manual paths can still touch legacy storage. Before Release B:

- Remove legacy data repair from normal server startup.
- Inventory and exercise all schedulers, manual triggers, support commands, health/readiness checks, and background integrations.
- Confirm `finance-repair` and any other retained support command works on the final schema or retire it before Release C.
- Confirm no old binary, worker, cron job, or externally deployed script can write legacy rows after Release A begins.

### 7.6 Reconcile Existing Migration Evidence

Before using the 2026-07-19 artifacts as cleanup evidence:

- Explain raw-block dry-run/apply create and skip counter differences.
- Explain forecast-versus-update differences for Nuki and finance links.
- Account row by row for finance rows reported unmatched by dry run but not by apply.
- Record row-level dispositions for cleaning exceptions, the external-sale conflict, and review-required stays.
- Produce a reviewed Markdown summary that identifies the database/tool/image used and links to restricted detail where repository redaction is required.

## 8. Required Delivery Sequence

Release A and Release B are separate, reviewable, non-destructive releases. Per the owner decision, Release D tooling/documentation cleanup is combined with Release C after required diagnostics and artifacts are captured; combining A or B with the destructive release is prohibited.

### 8.1 Release A - Stop Creating Legacy Dependencies

Release A is non-destructive. Legacy tables and columns remain available for observation and emergency diagnosis.

Required changes:

- Make raw-block reconciliation unconditional and stop all ICS writes to `occupancies` and `occupancy_nights`.
- Stop named-stay compatibility occupancy writes and migration-map creation.
- Remove synthetic finance occupancy creation from every active import/rematch path.
- Remove startup `BackfillUpstreamOwnership` and any other automatic repair that mutates legacy occupancy data.
- Add any missing named-stay outcome and review operations.
- Resolve known production exceptions and every `needs_review` stay.
- Reconcile Google Calendar over an approved range so active managed events have new-model ownership metadata.
- Stop writing `pms_occupancy_id` to Google private metadata and stop writing legacy identity columns in Nuki, cleaning, finance, invoice, and availability-block flows.
- Disable public export and occupancy-token create, list, and delete routes for the release window while retaining access-log evidence of attempted callers. Do not add another long-lived runtime flag, and preserve existing token rows unchanged until their approved Release C disposition.
- Record database write monitoring proving no legacy rows are created or changed by normal operation.

Release A acceptance:

- Row hashes or equivalent snapshots show no runtime mutation of `occupancies`, `occupancy_nights`, `occupancy_api_tokens`, `occupancy_stay_migration_map`, or any legacy identity column retained in integration tables during normal workflows.
- All new-model functional suites pass.
- Production monitoring shows no integration requiring a newly created legacy occupancy.
- Baseline analytics and availability results are captured for the approved comparison ranges before legacy reads are removed.
- Release A satisfies the 48-hour workload and acceptance contract in Section 6.4.

### 8.2 Release B - Remove Runtime Compatibility

Release B removes application-level compatibility while retaining legacy tables as inert data for one final verification period.

Required changes:

- Remove deprecated routes, handlers, request aliases, response aliases, and deprecation middleware usage that exists only for those routes.
- Remove legacy store methods, fallbacks, migration-map joins, DTO fields, and frontend controls.
- Remove public export and token management.
- Remove one-off occupancy repair UI and API after raw-only sync is proven.
- Rename stay-oriented APIs and UI state that still use occupancy terminology.
- Regenerate OpenAPI client types.
- Keep legacy tables read-only and outside all runtime query plans.
- Reconcile remote Google events again after legacy metadata writes have stopped, and record local/remote orphan results.
- Verify replacement-route permissions and property scoping match the removed operations.

Release B acceptance:

- A production-like test database with each legacy object renamed or access-denied can start the server and run the complete application test suite, except migration-history tests that intentionally construct historical schemas.
- Runtime SQL tracing records no statement against `occupancies`, `occupancy_nights`, `occupancy_api_tokens`, or `occupancy_stay_migration_map`.
- Repository searches show no active API or UI dependency on old IDs.
- Scheduled jobs, manual jobs, retained support tools, and all configured integrations pass the same access-denial test. Each explicitly quarantined transitional tool is either absent or proven fail-closed and unreachable from normal production operation.
- Runtime request/response tests prove undocumented aliases are absent even for route-only OpenAPI operations.
- Release B satisfies the 48-hour workload and acceptance contract in Section 6.4.

### 8.3 Release C - Destructive Forward Migration

Release C adds a new numbered migration. Never edit migrations `000001` through `000037` or any later migration already applied outside a disposable local database.

Required migration order:

1. Capture pre-migration row counts, critical-field hashes, child foreign-key values, file checksums, uniqueness conflicts, statistics baselines, and `sqlite_sequence` values for every affected table.
2. Materialize the exact parent/child dependency graph, including the circular `occupancies`/`finance_bookings` relationship and children of Nuki codes, cleaning events, finance bookings, and invoices.
3. Stage and rebuild affected child and parent tables in an explicitly reviewed order that preserves IDs and references. `PRAGMA foreign_keys=OFF` inside the existing transaction-based migrator is not a valid strategy; the migration must be foreign-key-safe under the actual runner or use a separately approved migration mechanism.
4. Rebuild `nuki_access_codes`, `nuki_guest_daily_entries`, `finance_bookings`, `invoices`, and `cleaning_calendar_events` without legacy columns, checks, foreign keys, or indexes, recreating every non-legacy constraint and index deliberately.
5. Rebuild `property_availability_blocks` without `source_occupancy_id`; legacy occupancy provenance is intentionally discarded.
6. Apply the final constraints in Section 20. The dependency graph and rebuild plan include every additional new-model parent or child required, including as applicable `named_stays`, `named_stay_nights`, `raw_booking_blocks`, `raw_booking_block_nights`, and `stay_source_links`; the tables named above are a minimum inventory, not an exhaustive limit.
7. Verify copied row counts, primary keys, child references, critical values, statistics, uniqueness, property ownership, and next AUTOINCREMENT behavior before dropping source tables.
8. Drop `occupancy_nights`.
9. Drop `occupancy_api_tokens`.
10. Drop `occupancies`.
11. Drop `occupancy_stay_migration_map` without archive/export after all parity and target checks pass. Existing audit-log IDs may remain as opaque historical values.
12. Remove obsolete indexes and triggers and verify no view, trigger, index, table, or column retains behavior-bearing legacy references.
13. Capture the final migration/repair diagnostics, then remove the transitional CLIs, compatibility flags, packaging, and obsolete documentation paths in the same Release C/D delivery.
14. Run `PRAGMA foreign_key_check`, `PRAGMA integrity_check`, latest-schema scans, and value-preservation checks before commit or operational acceptance as appropriate to the migration mechanism.

Release C acceptance:

- A database migrated from migration `000001` to latest reaches the clean schema.
- A production-shaped database preserves every required row and value through table rebuilds.
- No latest-schema foreign key references `occupancies`.
- No latest-schema column uses `occupancy_id` as a current business identity.
- The application starts and all supported workflows pass with no legacy tables present.
- All expected indexes, checks, unique constraints, foreign-key actions, and AUTOINCREMENT sequences are present and tested after rebuild.
- Nuki event logs, cleaning event logs, finance merges/transactions, invoices, and invoice files retain the same parent IDs and critical values.
- The migration fits the measured maintenance window and free-disk budget and fails atomically under an injected failure test.

### 8.4 Release D - Combined Transitional Tooling And Documentation Cleanup

Release D is combined with Release C. Required audit, migration, repair, and diagnostic outputs must be captured before the tooling is removed. Recovery after deployment uses the verified backup and migration artifacts rather than retaining executable compatibility tooling.

Required changes:

- Remove the PMS 21 migration CLI after final audit artifacts are captured and no supported environment still needs the backfill.
- Remove the occupancy repair CLI and UI if all supported recovery is raw-block-based.
- Remove PMS 21 migration CLI packaging from `deploy/Dockerfile.backend`.
- Remove or update `finance-repair` and any support tooling that cannot operate on the final schema.
- Remove the three PMS 21 runtime flags after their call sites are gone.
- Mark historical implementation/audit documents as historical or superseded without deleting their evidence.
- Update general architecture, module, analytics, cleaning, and deployment docs that still present `occupancies` or public export as current behavior.

## 9. Backend Removal Inventory

### 9.1 Configuration And Wiring

Remove after their corresponding branches are gone:

| File | Remove | Final state |
| --- | --- | --- |
| `backend/internal/config/config.go` | `RawBlocksDualWrite`, `OccupancyExportDisabled`, `OccupancyLegacyWriteDisabled` and `PMS21_*` parsing | No PMS 21 compatibility flags remain. |
| `backend/cmd/server/main.go` | Flag propagation into store, occupancy service, and API server | New-model services are unconditional. |
| `backend/internal/store/store.go` | `Store.OccupancyLegacyWriteDisabled` | Store has no compatibility-write mode. |
| `backend/internal/occupancy/sync.go` | `Service.RawBlocksDualWrite` and related counter wiring | Sync always reconciles raw blocks. |
| `backend/internal/api/server.go` | `Server.OccupancyExportDisabled` | Export route no longer exists. |

Also remove the unconditional `Store.BackfillUpstreamOwnership` startup call after its one-time purpose is proven complete; a latest-schema server must start without querying legacy tables.

### 9.2 Legacy Occupancy Store

`backend/internal/store/occupancy.go` currently mixes source configuration, sync-run history, raw snapshots, legacy stay CRUD, closure behavior, finance mappings, export-token storage, and public export reads.

Required refactor and removal:

- Retain occupancy source configuration, property ICS secret handling, sync-run records, and raw-event snapshots.
- Move retained source/sync concerns into narrowly named files if needed; do not preserve a monolithic `occupancy.go` solely for historical naming.
- Remove the legacy `Occupancy` stay/block representation type after all callers migrate.
- Remove legacy occupancy insert, upsert, status, list, calendar, upcoming, export, closure, split, reopen, outcome, cleaning-exclusion, and finance-mapping methods.
- Remove token create/list/delete/authentication methods.
- Remove old representation constants and closure constants after all current callers are gone.

### 9.3 ICS Reconciliation And Repair

| File | Remove or rewrite |
| --- | --- |
| `backend/internal/store/occupancy_reconciliation.go` | Remove aggregate occupancy reconciliation, legacy representation arbitration, legacy named-stay rows, supersession writes, and occupancy-night rebuilding. Keep only raw-block/source-link reconciliation in an appropriately named file. |
| `backend/internal/store/occupancy_nights.go` | Remove occupancy-night ownership, metrics, and repair helpers. Preserve only logic that has a true named-stay-night or availability-block equivalent, moved to those domains. |
| `backend/internal/store/occupancy_repair.go` | Remove PMS-19 repair planning/apply after production no longer contains active legacy conflicts. |
| `backend/cmd/occupancy-repair` | Remove one-off repair executable after repair retirement is approved. |
| `backend/internal/api/occupancy_repair_handlers.go` | Remove repair HTTP handlers. |

Do not remove:

- ICS parser correctness.
- Raw component snapshots in `occupancy_raw_events`.
- Sync leases and partial-parse no-mutation behavior.
- `occupancy_sources` and `occupancy_sync_runs`, despite their legacy names.

### 9.4 Named-Stay Compatibility

Remove from `backend/internal/store/named_stays.go`:

- `NamedStay.LegacyOccupancyID` from current domain/API models.
- Lookups through `occupancy_stay_migration_map` used to resolve current requests.
- Joins to `occupancies` used only to expose old IDs for raw blocks or named stays.
- `legacyNamedStayRow`.
- `upsertLegacyOccupancyForNamedStayTx`.
- `reconcileLegacyRawCoverageForNamedStayTx`.
- `upsertOccupancyStayMigrationMapTx` from normal create/update/status flows.
- Conditional branches controlled by `OccupancyLegacyWriteDisabled`.

Keep:

- Named-stay overlap enforcement.
- Named-stay-night replacement/deactivation.
- Source-link union coverage and warning recomputation.
- Cleaning and Nuki side-effect requests.
- Soft-delete semantics.

### 9.5 Analytics

Remove from `backend/internal/store/analytics.go` and related analytics files:

- `legacyListActiveOccupanciesInDateRange`.
- `legacyListClosedOccupanciesInDateRange`.
- `legacySumPayoutGrossNetForStays`.
- `legacyListOccupanciesByIDs`.
- `legacyListReturningGuests`.
- `legacyReturningGuestCount`.
- Every branch selected because a property has no named stays.
- Every append of old closed occupancy rows.
- Metrics sourced from `occupancies.imported_at` when a new-model booking/source event exists.

Final analytics must operate from named-stay nights, availability blocks, named-stay finance links, retained sync/source history, and canonical first-known/booking and cancellation timestamps only.

Also remove direct legacy pace queries from `backend/internal/api/analytics_handlers.go`; store-level fallback removal alone is insufficient.

Required semantic correction and parity:

- Backfill a canonical historical first-known/booking timestamp rather than treating Stage 2 migration-time `named_stays.created_at` as booking time.
- Preserve cancellation-effective timing needed by cancellation and lead-time statistics.
- Make sold-night, revenue, ADR, RevPAR, and cancellation calculations for `cancelled_non_refundable` and `no_show` conform to PMS 17.
- Compare historical pace, lead-time, cancellation, sold-night, occupancy-rate, ADR, RevPAR, returning-guest, and availability results before removing legacy reads.

### 9.6 Nuki

Remove from `backend/internal/store/nuki.go`, `backend/internal/store/nuki_guest_logs.go`, and `backend/internal/nuki/service.go`:

- Legacy occupancy selection methods.
- Joins to `occupancies` for names, dates, status, or active-night checks.
- Migration-map lookups for `old_occupancy_id`.
- Occupancy-based code lookup, generation, relinking, revocation, and guest-entry conflict targets.
- `LegacyOccupancyID` fields in store/service structs.
- Request acceptance and response serialization of `occupancy_id`.

Keep:

- Named-stay eligibility rules.
- Stable Nuki code IDs.
- Existing PINs and external IDs.
- Named-stay-keyed uniqueness and guest-entry idempotency.
- Nuki run/event history.

### 9.7 Cleaning Calendar

Remove from `backend/internal/store/cleaning_calendar.go`, `backend/internal/cleaningcalendar/service.go`, `backend/internal/cleaningcalendar/google_client.go`, and API DTOs:

- Legacy occupancy candidate fallback.
- `occupancy_id` and `next_occupancy_id` ownership and matching.
- Google private metadata written only as `pms_occupancy_id`.
- Same-day-arrival resolution through old occupancies.
- Public DTO compatibility fields for occupancy identity.

Keep:

- `named_stay_id`, `raw_booking_block_id`, and `cleaning_identity` ownership.
- Stored Google IDs and deterministic identity matching.
- Remove legacy Google wording/date and `pms_occupancy_id` discovery fallback in Release B after successful new-identity reconciliation and orphan verification; retain only stored Google ID, deterministic cleaning identity, and new-model metadata matching.
- Date-scoped desired-state reconciliation and desired hashes.
- Cleaning event and event-log history.

### 9.8 Finance And Invoices

Remove from `backend/internal/store/finance.go`, `backend/internal/store/finance_booking_payouts.go`, and `backend/internal/store/finance_bookings_merge.go`:

- `legacyFindOrCreateOccupancyForPayoutStayDates` and statement equivalent.
- Synthetic occupancy insertion for payout/statement data.
- Generic ICS occupancy supersession caused by finance matching.
- Migration-map and occupancy fallback matching.
- Writes to `occupancies.finance_booking_id` or `finance_bookings.occupancy_id`.
- `COALESCE(named_stay_id, occupancy_id)` mapped-state logic and legacy display fallbacks.

Remove from `backend/internal/store/invoices.go` and invoice handlers:

- Occupancy candidate queries.
- `occupancy_id` request aliases and response fields.
- Occupancy fallback when a named-stay ID is absent.

Required naming alignment:

- Rename `/properties/{id}/invoices/occupancy-candidates` to a stay-oriented route.
- Name internal form/request fields `named_stay_id`, not `occupancy_id` holding a stay ID.
- Preserve the canonical finance booking and named-stay linkage without adding a reverse compatibility FK.

Final finance and invoice contract:

- `finance_bookings.named_stay_id` is mandatory. Finance import may retain unmatched raw evidence in staging/rejection output, but no canonical finance-booking row is committed without an explicitly selected named stay. This supersedes ADR-006's temporary unmatched-row behavior.
- `invoices.named_stay_id` is mandatory and is the canonical invoice-to-stay identity.
- An invoice's finance-booking link is optional provenance and amount-refresh evidence. If present, the finance booking belongs to the same property and has the same `named_stay_id` as the invoice.
- Standalone invoices without a named stay are not supported after cleanup. Existing standalone invoices must be linked before Release C.
- Clearing or changing an invoice stay while a finance link remains must validate the complete resulting row and reject disagreement.

### 9.9 Messages And Dashboard

Remove:

- Message generation query parameter `occupancy_id` and its migration-map resolution.
- Message picker fields that expose occupancy identity.
- Dashboard DTO `occupancy_id` fields for upcoming stays and Nuki codes.
- Legacy list keys or links built from occupancy identity.

Keep:

- `stay_id` for stay-facing records.
- Nuki code ID for code-facing records.
- Sync freshness sourced from retained occupancy sync runs.

### 9.10 Combined Calendar

Remove from `backend/internal/store/stay_calendar.go` and related handlers/DTOs:

- Migration-map joins used to attach legacy occupancy IDs to raw blocks, named stays, or cleaning summaries.
- `LegacyOccupancyID` fields in calendar objects.
- Cleaning attachment through `cleaning_calendar_events.occupancy_id`.
- Any fallback that chooses old IDs when block IDs, stay IDs, or cleaning IDs exist.

### 9.11 API Handler Compatibility

Store cleanup is not sufficient. Remove undocumented aliases and legacy serialization in:

- `backend/internal/api/invoice_handlers.go`.
- `backend/internal/api/message_handlers.go`.
- Nuki, cleaning, dashboard, calendar, finance, and occupancy handlers that accept or emit compatibility IDs.

Every changed operation must have concrete OpenAPI request and response schemas before generated types are treated as proof of contract cleanup.

## 10. API Route Removal Inventory

Remove the following routes from `backend/internal/api/server.go` and `spec/openapi.yaml` after caller gates pass:

| Route | Replacement or disposition |
| --- | --- |
| `GET /api/properties/{id}/occupancies` | `GET /api/properties/{id}/stays` and `GET /api/properties/{id}/occupancy-calendar` |
| `GET /api/properties/{id}/occupancies/calendar` | `GET /api/properties/{id}/occupancy-calendar` |
| `POST /api/properties/{id}/occupancies/{occupancyId}/close` | Availability-block or maintenance/personal-use stay operation |
| `POST /api/properties/{id}/occupancies/{occupancyId}/external-sale` | Named-stay create/update |
| `POST /api/properties/{id}/occupancies/{occupancyId}/split-nights` | Named-stay range update |
| `POST /api/properties/{id}/occupancies/{occupancyId}/reopen` | Named-stay or availability-block status operation |
| `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/cancelled-non-refundable` | Named-stay outcome operation |
| `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/no-show` | Named-stay outcome operation |
| `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/clear` | Named-stay outcome operation |
| `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/exclude` | Named-stay cleaning control |
| `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/include` | Named-stay cleaning control |
| `POST /api/properties/{id}/occupancy-blocks/{upstreamUid}/named-stays` | `POST /api/properties/{id}/booking-blocks/{blockId}/promote` |
| `PATCH /api/properties/{id}/occupancies/{occupancyId}/named-stay` | `PATCH /api/properties/{id}/stays/{stayId}` |
| `DELETE /api/properties/{id}/occupancies/{occupancyId}/named-stay` | `PATCH /api/properties/{id}/stays/{stayId}/status` |
| `POST /api/properties/{id}/occupancy-repair/ics-reconciliation/dry-run` | Retire after repair window |
| `POST /api/properties/{id}/occupancy-repair/ics-reconciliation/apply` | Retire after repair window |
| `GET /api/properties/{id}/occupancy-export` | Remove without v2 replacement |
| `POST /api/properties/{id}/occupancy-api-tokens` | Remove |
| `GET /api/properties/{id}/occupancy-api-tokens` | Remove |
| `DELETE /api/properties/{id}/occupancy-api-tokens/{tokenId}` | Remove |

After route removal:

- Delete route-specific deprecation wrappers, warnings, tests, and OpenAPI headers when no other deprecated route uses them.
- Removed routes return the normal router `404` immediately in Release B. There is no `410`, grace period, or compatibility shim because the owner confirms there are no consumers.
- Do not retain hidden aliases that continue accepting occupancy identity.

## 11. Frontend Removal Inventory

### 11.1 Occupancy View

Remove from `frontend/src/views/OccupancyView.vue`:

- Legacy occupancy list loading and state.
- Close, external-sale, split, reopen, old outcome, old cleaning exclusion, old promotion, old named-stay edit, and hard-delete request flows.
- Compatibility selection based on occupancy IDs or upstream UIDs where block IDs/stay IDs exist.
- Any duplicate controls superseded by combined calendar named-stay and availability-block lifecycle actions.

Delete after usage is removed:

- `frontend/src/views/occupancy/OccupancyStayList.vue`
- `frontend/src/views/occupancy/OccupancyStayList.spec.ts`
- `frontend/src/views/occupancy/OccupancyClosureDialog.vue`

Also update `frontend/src/views/occupancy/OccupancyCalendar.vue` so it has no legacy `occupancies` fallback and keys every object by its real new-model identity.

Review rather than delete wholesale:

- `frontend/src/views/occupancy/closure.ts`, because payout and stay-outcome labels may still be reusable. Split or rename retained outcome-formatting helpers so a legacy closure module is not a permanent shared dependency.
- `frontend/src/views/occupancy/status.ts`, retaining only status behavior valid for new-model objects.

### 11.2 Sync Panel

Remove from `frontend/src/views/occupancy/OccupancySyncPanel.vue`:

- Legacy ICS repair dry-run/apply controls.
- Repair report types containing winner/loser occupancy IDs.

Keep:

- Occupancy source enablement and URL configuration.
- Manual raw sync trigger.
- Sync-run history, status, parse errors, and raw-source health.

### 11.3 DTOs And Adapters

Remove legacy fields from hand-authored frontend types:

- `frontend/src/api/types/occupancy.ts`
- `frontend/src/api/types/nuki.ts`
- `frontend/src/api/types/dashboard.ts`
- `frontend/src/api/types/cleaning.ts`
- `frontend/src/api/types/messages.ts`
- `frontend/src/api/types/bookingPayouts.ts`
- `frontend/src/api/types/invoice.ts`

Required field removals include:

- `occupancy_id` aliases.
- `legacy_occupancy_id` display/lookup fields.
- Repair-only `winner_occupancy_id` and `loser_occupancy_ids`.
- Cleaning `next_occupancy_id`.
- Stay-facing `occupancy_start_at`, `occupancy_end_at`, and `occupancy_summary` names where they describe a named stay rather than retained source evidence.

### 11.4 Invoice Naming

Update:

- `frontend/src/views/InvoicesView.vue`
- `frontend/src/views/invoices/InvoiceEditorForm.vue`

Required result:

- Form state and payload use `named_stay_id`.
- Candidate endpoint and response names are stay-oriented.
- No variable named `occupancy_id` stores a named-stay ID.

Delete or rename `frontend/src/views/messages/OccupancyCombobox.vue` after confirming it has no retained caller; unused compatibility components must not remain as an apparent supported path.

### 11.5 Generated Contract

After OpenAPI cleanup:

- Regenerate `frontend/src/api/types/generated.ts` using the repository's normal generator.
- Review generated deletions rather than hand-editing the generated file.
- Update mocks in occupancy, Nuki, dashboard, cleaning, messages, payouts, and invoices so tests prove old IDs are not required.

## 12. Database Cleanup Specification

### 12.1 Tables To Retain

The following tables remain even though some names contain `occupancy`:

| Table | Reason |
| --- | --- |
| `occupancy_sources` | Booking.com ICS source configuration. |
| `occupancy_sync_runs` | Operational sync history and freshness. |
| `occupancy_raw_events` | Immutable/raw upstream evidence for each sync run. |
| `raw_booking_blocks` | Current sync-owned blocked ranges. |
| `raw_booking_block_nights` | Raw source coverage by property-local night. |
| `named_stays` | Current user/business stay truth. |
| `named_stay_nights` | Capacity and analytics truth. |
| `stay_source_links` | Booking.com provenance and source health. |
| `property_availability_blocks` | Non-stay availability reductions. |
| Nuki run/event tables | Integration and security history. |
| Cleaning event/log tables | Local and Google reconciliation history. |
| Finance/import/reset tables | Financial evidence and audit history. |
| Invoice/file tables | Legal/business records. |
| API audit logs | Security and change history. |

### 12.2 Tables To Drop

Drop only in Release C:

| Table | Preconditions |
| --- | --- |
| `occupancy_nights` | No runtime query; named-stay nights and raw-block nights fully cover current behavior. |
| `occupancy_api_tokens` | Export route and token APIs removed; zero approved active consumers. |
| `occupancies` | All integration FKs removed; statistics-bearing data migrated; approved legacy-only metadata discarded; zero runtime query. |
| `occupancy_stay_migration_map` | All target/parity checks passed; runtime lookups removed; owner approved removal without archive/export. |

### 12.3 Migration Map Disposition

`occupancy_stay_migration_map` is not a business source of truth. Runtime use ends in Release B. Release C validates every row and target, then drops the table without archive or export. The owner accepts loss of old-to-new attribution; retained audit logs may contain opaque old occupancy IDs that are no longer resolvable.

### 12.4 Integration Table Rebuilds

SQLite table rebuilds must preserve primary keys and child relationships.

| Table | Remove | Preserve |
| --- | --- | --- |
| `nuki_access_codes` | `occupancy_id`, legacy occupancy unique index, legacy check branch | IDs, `named_stay_id`, PIN data, external IDs, validity, status, errors, sync links, timestamps, revocation |
| `nuki_guest_daily_entries` | `occupancy_id`, legacy unique index, legacy check branch | IDs, `named_stay_id`, day, first entry, event reference, timestamps |
| `finance_bookings` | `occupancy_id` and occupancy indexes | IDs, `named_stay_id`, source evidence, amounts, dates, transactions, import state |
| `invoices` | `occupancy_id` and occupancy unique index | IDs, `named_stay_id`, finance link, numbering, snapshots, totals, files |
| `cleaning_calendar_events` | `occupancy_id`, `next_occupancy_id`, occupancy indexes/uniqueness | IDs, new ownership, Google IDs, identity, desired hash, dates, status, warning/error state |
| `property_availability_blocks` | `source_occupancy_id` | IDs, property/date range, block type, status, canonical reason if already present, actors/timestamps already canonical |

The following child tables do not own legacy columns but must be included in the rebuild dependency plan because dropping/rebuilding their parents can null or cascade their links:

| Child table | Parent link that must remain identical |
| --- | --- |
| `nuki_event_logs` | `nuki_access_code_id` |
| `cleaning_calendar_event_logs` | `cleaning_calendar_event_id` |
| `invoice_files` | `invoice_id` |
| `finance_booking_merges` | `booking_id` |
| `invoices` | `finance_booking_payout_id` / canonical finance-booking link |

The `occupancies.finance_booking_id` and `finance_bookings.occupancy_id` relationship is circular in the pre-cleanup schema. The migration design must break that cycle without invoking `SET NULL`/`CASCADE` side effects on retained rows.

Required preservation checks:

- `nuki_event_logs.nuki_access_code_id` still resolves to the same code ID.
- Invoice files still resolve to the same invoice ID.
- Cleaning event logs still resolve to the same cleaning event ID.
- Finance transaction and merge relationships remain unchanged.
- No copied row violates new-model uniqueness.
- Every rebuilt table preserves all non-legacy checks, indexes, implicit unique constraints, foreign-key actions, and property scoping deliberately; relying on `CREATE TABLE AS` is prohibited for the final table.
- `sqlite_sequence` or equivalent next-ID behavior is preserved so later inserts cannot collide or unexpectedly reuse identities.

Final owner constraints:

- `nuki_access_codes.named_stay_id` and `nuki_guest_daily_entries.named_stay_id` are `NOT NULL`, same-property references with `ON DELETE RESTRICT`.
- `finance_bookings.named_stay_id` and `invoices.named_stay_id` are `NOT NULL`, same-property references with `ON DELETE RESTRICT`.
- An optional invoice finance-booking reference is same-property and must resolve to the invoice's named stay; retain one-invoice-per-stay and one-invoice-per-finance-booking uniqueness.
- `cleaning_calendar_events` has exactly one of `named_stay_id` and `raw_booking_block_id`, with same-property ownership and `ON DELETE RESTRICT` for retained history.
- `named_stay_nights` and `raw_booking_block_nights` use same-property composite ownership; derived nights may retain `ON DELETE CASCADE`.
- `stay_source_links` enforces same-property named-stay ownership and, when a raw block is present, same-property raw-block ownership.
- Required parent `UNIQUE(property_id, id)` keys or equivalent declarative support are added for composite foreign keys.

### 12.5 Applied Migration Files

Retain every historical migration file, including those that create or alter legacy tables. Fresh databases must replay the historical sequence and then apply the new cleanup migration. Do not rewrite history to make old tables appear never to have existed.

## 13. Cleanup Readiness Queries

The implementation must provide a repeatable read-only command or report containing equivalent checks. The exact SQL may evolve with schema changes, but every condition must be represented.

Each report records the database fingerprint and size, schema migration version, tool commit and immutable image digest, effective PMS 21 flags, command arguments, UTC start/end timestamps, operator, and report checksum. It separates zero-count results from approved exceptions and links every exception to the row-level register. Pre- and post-cleanup reports use the same stable check identifiers so results can be compared mechanically.

```sql
PRAGMA foreign_key_check;

SELECT migration_kind, COUNT(*)
FROM occupancy_stay_migration_map
GROUP BY migration_kind;

SELECT COUNT(*) AS unmapped_migration_rows
FROM occupancy_stay_migration_map
WHERE migration_kind = 'unmapped';

SELECT COUNT(*) AS nuki_codes_without_stay
FROM nuki_access_codes
WHERE named_stay_id IS NULL;

SELECT COUNT(*) AS nuki_entries_without_stay
FROM nuki_guest_daily_entries
WHERE named_stay_id IS NULL;

SELECT COUNT(*) AS finance_bookings_without_stay
FROM finance_bookings
WHERE named_stay_id IS NULL;

SELECT COUNT(*) AS invoices_without_stay
FROM invoices
WHERE named_stay_id IS NULL;

SELECT COUNT(*) AS invoice_finance_stay_mismatches
FROM invoices i
JOIN finance_bookings fb
  ON fb.id = i.finance_booking_payout_id
WHERE i.property_id != fb.property_id
   OR i.named_stay_id != fb.named_stay_id;

SELECT COUNT(*) AS cleaning_events_without_new_owner
FROM cleaning_calendar_events
WHERE named_stay_id IS NULL
  AND raw_booking_block_id IS NULL;

SELECT COUNT(*) AS cleaning_events_with_legacy_next_stay
FROM cleaning_calendar_events
WHERE next_occupancy_id IS NOT NULL;

SELECT COUNT(*) AS review_required_stays
FROM named_stays
WHERE review_status = 'needs_review';

SELECT COUNT(*) AS active_closures_without_availability_block
FROM occupancies o
LEFT JOIN occupancy_stay_migration_map m
  ON m.old_occupancy_id = o.id
WHERE o.closure_state = 'closed'
  AND o.status NOT IN ('deleted_from_source', 'cancelled')
  AND m.availability_block_id IS NULL;

SELECT COUNT(*) AS external_sales_without_named_stay
FROM occupancies o
LEFT JOIN occupancy_stay_migration_map m
  ON m.old_occupancy_id = o.id
WHERE o.closure_state = 'external_sale'
  AND m.named_stay_id IS NULL;

SELECT COUNT(*) AS export_token_rows_remaining
FROM occupancy_api_tokens;

SELECT COUNT(*) AS availability_blocks_with_legacy_source
FROM property_availability_blocks
WHERE source_occupancy_id IS NOT NULL;

SELECT COUNT(*) AS cleaning_events_with_invalid_owner_cardinality
FROM cleaning_calendar_events
WHERE (named_stay_id IS NULL) = (raw_booking_block_id IS NULL);

SELECT COUNT(*) AS invalid_migration_map_target_cardinality
FROM occupancy_stay_migration_map
WHERE (raw_booking_block_id IS NOT NULL)
    + (named_stay_id IS NOT NULL)
    + (availability_block_id IS NOT NULL) !=
      CASE WHEN migration_kind = 'unmapped' THEN 0 ELSE 1 END;
```

The implementation may express the boolean/cardinality check differently if required by the supported SQLite version. It must additionally report:

- Cross-property references for every new-model owner and migration-map target.
- Dangling targets and migration-kind/target mismatches.
- Named-stay and raw-block active-night range parity, including no active nights for cancelled/archived stays.
- Source-link target, property, range, and status consistency.
- Cleaning rows with both/no owner, invalid owner kind, invalid next-stay replacement, local/remote Google orphans, and legacy-only Google metadata.
- Finance and invoice rows without a named stay, conflicting direct/finance-derived stay IDs, or cross-property links; every count must be zero.
- Nuki critical-value parity and orphan event-log links.
- Child-reference, file checksum, uniqueness, and AUTOINCREMENT baselines for all rebuilt tables.
- Canonical first-known/booking and cancellation-effective timestamp coverage for every stay included in pace, lead-time, or cancellation statistics.
- Availability, sold-night, revenue, ADR, RevPAR, pace, lead-time, gap, cancellation/no-show, external-unfunded, maintenance, personal-use, and review-required analytics over all available history, with focused spot checks for current and prior-year ranges. Outcome calculations must match PMS 17 even where that intentionally differs from the current PMS 21 implementation.

Before dropping `occupancies`, also compare old and new values for statistics-bearing outcomes, timestamps, date ranges, statuses, display names, and finance links. A count-only report is insufficient when values can differ. Every approved non-zero result must link to the exception register.

After the destructive migration:

```sql
PRAGMA foreign_key_check;
PRAGMA integrity_check;

SELECT name, sql
FROM sqlite_schema
WHERE lower(COALESCE(sql, '')) LIKE '%references occupancies%'
   OR lower(COALESCE(sql, '')) LIKE '% occupancy_id%'
   OR lower(COALESCE(sql, '')) LIKE '%next_occupancy_id%'
   OR lower(COALESCE(sql, '')) LIKE '%source_occupancy_id%'
   OR lower(COALESCE(sql, '')) LIKE '%old_occupancy_id%';

SELECT name
FROM sqlite_schema
WHERE type = 'table'
  AND name IN ('occupancies', 'occupancy_nights', 'occupancy_api_tokens', 'occupancy_stay_migration_map');
```

`PRAGMA table_info` checks must also enumerate columns of every latest-schema table rather than relying only on SQL text matching. Every returned legacy table or behavior-bearing legacy column must be eliminated; no migration-map/archive exception remains in the live schema.

## 14. Test Cleanup And Required Coverage

### 14.1 Tests To Delete Or Rewrite

- Delete tests whose sole purpose is proving raw-block dual-write defaults off.
- Rewrite ICS reconciliation tests to assert raw blocks, raw nights, source links, source warnings, and no legacy table writes.
- Delete legacy occupancy representation arbitration and repair expectations after repair retirement.
- Delete named-stay tests that expect derived occupancy rows or migration-map rows.
- Delete finance tests for synthetic occupancy creation.
- Delete analytics tests that expect fallback to `occupancies`.
- Delete Nuki tests that generate or find codes by occupancy identity.
- Delete API tests for removed occupancy, export-token, and repair routes.
- Delete `OccupancyStayList.spec.ts` with its component.
- Rewrite frontend mocks that include old IDs only because current DTOs require them.

### 14.2 Tests To Retain And Strengthen

- Raw sync parsing, leases, partial-no-mutation, disappearance, shrink, recovery, split/merge, and source warning tests.
- Named-stay overlap, lifecycle, status, review, outcome, cleaning, Nuki, revenue, and source-link tests.
- Named-stay-night strict analytics and sold-night semantics.
- Nuki row preservation, named-stay uniqueness, revocation, and guest-entry idempotency.
- Cleaning ownership, Google matching, no-op hash, and non-PMS event protection.
- Finance and invoice preservation, matching, rematch, cancellation review, reset, and file tests.
- OpenAPI route coverage and generated type checks.

### 14.3 New Required Tests

- Normal operation against a latest database never writes legacy tables during Release A.
- Normal operation succeeds when legacy tables are inaccessible during Release B.
- Removed APIs return ordinary `404` and no hidden compatibility alias remains.
- New requests containing `occupancy_id` are rejected rather than silently interpreted.
- A fresh database migrates from `000001` through the cleanup migration.
- Every supported historical upgrade starting point reaches the same latest schema.
- A populated pre-cleanup database preserves IDs and critical values across every table rebuild.
- `PRAGMA foreign_key_check` is clean after migration.
- Latest schema has no foreign key to `occupancies`.
- Repository route/OpenAPI coverage remains complete after route removal.
- Public occupancy export and token management are absent from backend and frontend.
- Server startup, every scheduler, manual job, and retained support command succeeds with legacy objects inaccessible.
- Runtime request tests reject undocumented `occupancy_id` input and response tests prove no legacy field is serialized, including route-only operations.
- Replacement routes preserve authorization and reject cross-property IDs.
- Current and cached previous frontend builds have an explicit compatibility outcome for the cleaned backend.
- Analytics and availability parity tests cover every stay type and review/outcome state.
- Property-local dates, non-UTC zones, DST boundaries, half-open ranges, and concurrent sync/cleaning/Nuki/finance operations retain their PMS 21 behavior.
- Migration failure injection proves no partially rebuilt schema or lost child relationship can be committed.

## 15. Documentation Cleanup

Update active documents so the final architecture is unambiguous:

- `README.md`: stop presenting public occupancy export or old occupancy-as-stay behavior as current functionality.
- `spec/README.md`: add PMS 21 source-of-truth and cleanup documents; supersede the old n8n/export scope note.
- `spec/PMS_01_Architecture_and_Global_Spec.md`: update schema and identity references.
- `spec/PMS_02_Module_Specifications.md`: replace occupancy-as-stay and `occupancy_id` API descriptions.
- `spec/PMS_03_Implementation_Checklists.md`: remove completed claims for token export and occupancy-based message generation.
- `spec/PMS_04_Analytics_Data_Inventory.md`: document named-stay-night and new finance linkage.
- `spec/PMS_05_Analytics_Module_Spec.md`: remove occupancy-table analytics authority.
- `spec/PMS_12_v1.1_Implementation_Plan.md`: mark occupancy-keyed Nuki details historical.
- `spec/PMS_15_Google_Calendar_Cleaning_Events_Spec.md`: replace occupancy ownership with PMS 21 cleaning ownership.
- `spec/PMS_16_Finance_Reset_Preserve_Cleaning_Salary_Spec.md`: replace synthetic occupancy preservation with named-stay preservation.
- `spec/PMS_19_Booking_ICS_Reconciliation_Spec.md`: mark PMS 21 as superseding the overloaded representation model.
- `docs/pms-21-implementation-readiness.md`: reflect real audit/apply status and cleanup gates.
- `docs/pms-21-operations-cutover-runbook.md`: convert it into the active combined Release A-D cleanup runbook, preserving links to initial-cutover evidence and retaining standalone Podman commands only for the actual production topology.
- `docs/deployment/backup-runbook.md`: replace stale systemd paths/commands and occupancy-table checks with the actual standalone Podman topology and final-schema checks.
- `docs/adr/ADR-005-occupancy-compatibility-window.md`: supersede it through normal ADR practice; retain it unchanged as the historical compatibility-window decision.
- `docs/adr/ADR-006-finance-import-named-stay-behavior.md`: supersede its temporary unmatched canonical finance-booking behavior. Raw unmatched evidence may remain staged, but every committed `finance_bookings` row requires a named stay.
- `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md` and `docs/pms-21-implementation-readiness.md`: mark stale pre-production status statements historical and point final cleanup authority to this specification.

Historical artifacts must remain intact:

- Stage verification documents under `docs/audits/`.
- Production audit/apply/idempotency JSON.
- Divergence and remediation documents, marked historical or superseded rather than rewritten as if their findings never existed.
- ADRs, amended or superseded through normal ADR practice when decisions changed.

## 16. Podman Operational Requirements

Production PMS runs as the standalone Podman container `api.pms.airportlounge.sk` without Compose. It uses network `internet_enabled`, image `ghcr.io/ai-slop-code/pms-backend:latest`, environment file `./pms.env`, and the bind mount `/mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z`. Cleanup instructions must follow this topology.

Required operational properties:

- Record the exact image ID behind `ghcr.io/ai-slop-code/pms-backend:latest` before the maintenance window.
- Resolve that image to an immutable digest and use the same approved digest for readiness, migration, and verification; the mutable `latest` tag alone is not sufficient evidence.
- Stop the backend before audit/apply when the production SQLite volume is used directly.
- Take and verify a backup before migration.
- Use a SQLite-consistent backup method that accounts for WAL/SHM state, record its checksum and size, and complete a restore/migrate/application-start drill.
- Stop the backend, mount `/mnt/main_storage/containers/data/api.pms.airportlounge.sk` directly into each one-off migration container, then remove and recreate the stopped API afterward with its normal `podman run` command.
- Mount the data directory at `/data:ro,Z` for audit and `/data:Z` for approved apply/cleanup.
- Pass `/data/pms.db` explicitly to migration tools, or the exact alternative `DATABASE_PATH` configured in `pms.env`.
- Redirect audit output to a host path outside the application volume.
- Recreate the application using the existing direct `podman run` command; do not introduce Compose, systemd, Quadlet, or `--volumes-from` as cleanup dependencies.
- Record image, container, volume, database path, environment source, command output, and timestamps.
- Record available disk, expected temporary-copy size, measured duration, maintenance-window limit, operator, approvals, and cleanup migration checksum.
- Keep restricted raw command output outside the repository when it contains production identifiers or secrets; retain a redacted, checksum-linked approval summary in the repository.

The exact command templates belong in `docs/pms-21-operations-cutover-runbook.md`, not duplicated across implementation code.

## 17. Rollback Rules

### 17.1 Before Release C

- Before Release A has produced new-model-only writes, rollback may deploy the prior application image while preserving additive schema if that exact combination is tested.
- After legacy writes stop, redeploying a prior binary is not automatically safe because it may read stale legacy state. Rollback requires either restoring the matching pre-release database backup or a separately tested forward-reconciliation procedure, together with the compatible backend/frontend image.
- Do not run historical down migrations as an operational rollback.
- Keep the pre-cutover backup pinned through Release C/D acceptance, then apply the existing retention policy.
- Releases A and B use the compatible pre-release backup/image pair when rollback is required; the operational objective is restoration before further traffic rather than dual-mode reconciliation.

### 17.2 After Release C

- The old binary is unsupported because its required tables and columns no longer exist.
- Production is quiesced for backup, migration, and verification. If verification fails before traffic resumes, restore the pre-cleanup database and compatible image; because no production writes occurred during the window, the target RPO is zero and target RTO is 30 minutes.
- Once traffic resumes, cleanup is operationally irreversible and the default is fix-forward. Restoring the pre-cleanup backup requires explicit owner approval and knowingly discards all later writes unless a separately designed forward recovery is available.
- The owner is the final go/no-go and rollback authority. The runbook records the traffic-resume decision and any external Nuki/Google operations performed during maintenance.

## 18. Definition Of Done

Cleanup is complete only when all statements are true:

- ICS sync writes raw blocks directly and never writes legacy occupancies.
- All current stay behavior uses named stays and named-stay nights.
- All non-stay availability behavior uses availability blocks.
- All integrations use new-model identities without fallback.
- All legacy routes, export/token routes, and repair routes are removed.
- All frontend legacy occupancy flows and fields are removed.
- OpenAPI and generated types expose no occupancy-as-stay compatibility contract.
- Runtime code does not query `occupancies`, `occupancy_nights`, `occupancy_api_tokens`, or `occupancy_stay_migration_map`.
- Integration tables have no current-business `occupancy_id` columns.
- `occupancies`, `occupancy_nights`, and `occupancy_api_tokens` are absent from latest schema.
- `occupancy_stay_migration_map` and `source_occupancy_id` are removed without archive/export after parity checks; old IDs remaining in audit logs are intentionally opaque.
- Required statistics-bearing and integration data is preserved.
- Fresh and upgraded database migration tests pass.
- Backend, frontend, OpenAPI, and end-to-end verification pass.
- Server startup, schedulers, post-cleanup support commands, and integrations pass with legacy objects absent.
- Authorization, same-property ownership, date/DST behavior, concurrency behavior, analytics parity, and remote Google/Nuki verification pass.
- Every rebuilt parent/child relationship, critical value, file checksum, constraint, index, and next-ID behavior is preserved.
- Active documentation describes one model rather than compatibility and target models side by side.
- The cleanup audit, owner approval, backup reference, deployed image, and verification results are retained.
- Every production exception is resolved or documented as disposable test data, and implementation conforms to the resolved decisions in Section 20.

## 19. Explicitly Prohibited Shortcuts

- Do not drop legacy tables first and fix failing consumers afterward.
- Do not edit an applied migration.
- Do not classify ambiguous data by guessing from names or date overlap.
- Do not confirm `needs_review` rows merely to reach a zero count.
- Do not delete unmapped cleaning rows without checking Google event ownership and history.
- Do not regenerate Nuki PINs or external IDs because identity changed.
- Do not renumber invoices or recreate invoice files.
- Do not keep `occupancy_id` aliases under a different undocumented route.
- Do not retain a permanent fallback because tests are easier to satisfy with old data.
- Do not delete `occupancy_sources`, `occupancy_sync_runs`, or `occupancy_raw_events`; they are retained source configuration and evidence, not the obsolete stay model.
- Do not use Compose commands in production PMS 21 cleanup instructions.
- Do not treat successful Stage 2 backfill, count-only idempotency, or apparently healthy production behavior as destructive-cleanup proof.
- Do not use `PRAGMA foreign_keys=OFF` inside the current transaction-based migrator as a substitute for a foreign-key-safe rebuild order.
- Do not rebuild a parent table without preserving and verifying all child references that can be nulled or cascaded.
- Do not redeploy a legacy-reading binary after legacy writes stop unless its database state and reconciliation behavior are explicitly tested.
- Do not use a mutable image tag as the identity of the audited or migration binary.
- Do not put real production secrets, PINs, guest data, token values, live secret-bearing URLs, or unrestricted raw audit payloads in the repository.

## 20. Resolved Owner Decisions

Decisions recorded 2026-07-22:

| # | Decision |
| --- | --- |
| 1 | Drop `occupancy_stay_migration_map` without archive or export after parity checks. |
| 2 | Preserve canonical data required for financial and occupancy statistics; discard legacy-only reasons, actors, old IDs, and provenance details after validation. |
| 3 | Resolve every `needs_review` stay individually before Release C. |
| 4 | Every committed finance booking has a named stay; unmatched input remains staging/rejection evidence only. |
| 5 | Every invoice has a canonical `named_stay_id`; an optional finance link must reference the same stay. |
| 6 | Every Nuki code and guest daily entry has a named stay. |
| 7 | Every cleaning event has exactly one named-stay or raw-block owner. |
| 8 | Enforce same-property composite ownership, mandatory owner nullability, exactly-one cleaning ownership, `RESTRICT` for retained integration/history rows, and `CASCADE` only for derived night rows. |
| 9 | Release A and B each run for at least 48 hours with the representative workload and zero-error/zero-legacy-access acceptance contract in Section 6.4. |
| 10 | Remove deprecated APIs, export, and token management immediately in Release B with ordinary `404`; no grace period or `410` shim. |
| 11 | Before traffic resumes, failed cleanup restores the quiesced backup with target RPO zero and RTO 30 minutes. After traffic resumes, fix-forward is the default and restore requires explicit owner acceptance of write loss. |
| 12 | Existing backup/security/retention controls are sufficient; pin cleanup backups only through Release C/D acceptance and create no additional long-term legacy archive. |
| 13 | PIN encryption/backfill and key-restore validation are not cleanup gates; preserve exact stored PIN values and behavior without transformation. |
| 14 | Retire legacy Google wording/date and `pms_occupancy_id` fallback in Release B after successful new-identity reconciliation and orphan verification. |
| 15 | Convert the existing cutover runbook into the active cleanup runbook while preserving historical evidence references. |
| 16 | Supersede ADR-005 rather than rewriting it. Supersede ADR-006 where it permits ownerless canonical finance bookings. |
| 17 | Combine Release D with Release C after diagnostic artifacts are captured. |
| 18 | Follow PMS 17 for `cancelled_non_refundable` and `no_show`: count sold/occupied nights, retain actual revenue, and exclude them from normal cancellation-rate numerator and denominator. |
