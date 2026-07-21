# PMS 21 - Legacy Occupancy Removal Specification

Status: Draft cleanup specification. This document defines future work; it does not authorize destructive production cleanup.

Primary source of truth: `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`

Related decisions:

- `docs/adr/ADR-002-raw-booking-blocks-and-named-stays.md`
- `docs/adr/ADR-003-cleaning-event-ownership.md`
- `docs/adr/ADR-004-stay-type-reporting-semantics.md`
- `docs/adr/ADR-005-occupancy-compatibility-window.md`
- `docs/adr/ADR-006-finance-import-named-stay-behavior.md`

Operational companion: `docs/pms-21-operations-cutover-runbook.md`

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

These artifacts establish that the backfill ran and was idempotent for the recorded database. They do not establish cleanup eligibility. The following evidence is still absent or unresolved in the repository:

- A reviewed production audit Markdown artifact approving the JSON findings.
- A recorded resolution for the eight unmapped cleaning rows.
- A recorded resolution for the external-sale conflict.
- A recorded review policy and disposition for the 50 `needs_review` stays.
- A cleanup-readiness audit proving zero runtime and data dependencies.
- A recorded successful release window with legacy writes disabled.
- A record of exact deployed backend/frontend versions and PMS 21 flag values.
- A caller inventory proving deprecated APIs and public export are unused.

Therefore, destructive cleanup is blocked at the time this specification is written.

## 3. Scope

This specification covers:

- Removing old occupancy writes, reads, fallback queries, identity adapters, routes, DTO fields, UI flows, tests, and one-off repair tooling.
- Removing the public occupancy export and token-management feature.
- Removing rollback-only migration-map lookups from runtime paths.
- Rebuilding integration tables without legacy `occupancy_id` foreign keys.
- Dropping `occupancies`, `occupancy_nights`, and `occupancy_api_tokens` after dependency proof.
- Retaining required Booking.com source configuration, raw source evidence, operational history, and integration history.
- Aligning documentation, OpenAPI, generated types, and naming with the final model.

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
- Historical old IDs may remain in a read-only archive or audit payload only when clearly named `legacy_occupancy_id` and never used for behavior.

### 5.3 Dates And Status

- Stay and block ranges remain half-open: check-in inclusive, check-out exclusive.
- Property-local ISO dates remain the business-date representation.
- At most one active named stay may own a property night.
- Cancelled and archived stays have no active named-stay nights.
- `needs_review`, maintenance, personal use, and unfunded external stays reduce availability according to PMS 21 rules but do not inflate sold/revenue metrics.

### 5.4 Data Preservation

- Existing Nuki row IDs, PIN ciphertext/plain storage values as currently persisted, external Nuki IDs, valid windows, statuses, revocation timestamps, run links, and event-log references are preserved.
- Existing cleaning row IDs, Google Calendar IDs, Google event IDs, deterministic cleaning identities, desired hashes, statuses, warnings, errors, and event logs are preserved.
- Existing finance booking IDs, raw import evidence, transaction links, reset/import history, and named-stay links are preserved.
- Existing invoice IDs, invoice numbers, sequence values, snapshots, files, and finance/named-stay links are preserved.
- Existing audit logs and migration artifacts are preserved.
- Applied SQL migration files remain immutable and present in the repository.

## 6. Cleanup Eligibility Gates

Every gate in this section is mandatory. A coding agent must not interpret passing one group as permission to skip another.

### 6.1 Data Gates

- `PRAGMA foreign_key_check` returns no rows before cleanup.
- Every current Nuki access code has `named_stay_id`, or has an owner-approved archival disposition and is not needed by active behavior.
- Every Nuki guest daily entry has `named_stay_id`, or has an approved historical archive disposition.
- Every finance booking requiring a stay association has `named_stay_id`.
- Every invoice requiring a stay association has `named_stay_id` directly or through its finance booking according to the canonical invoice contract.
- Every active or historically managed cleaning row has `named_stay_id` or `raw_booking_block_id`, or has a documented archival disposition.
- No cleaning row requires `next_occupancy_id` to preserve same-day-turnover behavior.
- Every active legacy closure maps to an availability block or an explicitly verified new-model equivalent.
- Every external-sale legacy row maps to a named stay or an explicitly approved non-stay disposition.
- Every stay outcome and required outcome metadata is represented on `named_stays` before the old fields are dropped.
- No migration-map row remains `migration_kind = 'unmapped'` without explicit owner approval.
- All `needs_review` stays have an approved production handling policy; cleanup must not silently change them to confirmed.

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

### 6.3 API And Caller Gates

- Access logs or an equivalent caller audit show no use of deprecated occupancy-as-stay routes for the agreed observation period.
- No automation uses `/api/properties/{id}/occupancy-export`.
- No active token exists in `occupancy_api_tokens`, or every token owner has approved retirement.
- OpenAPI contains no legacy route or compatibility field scheduled for removal.
- Generated frontend API types have been regenerated from the cleaned OpenAPI contract.
- Repository-wide searches find no behavior-bearing `occupancy_id`, `legacy_occupancy_id`, or `old_occupancy_id` use outside historical migrations, archives, audit documents, and explicitly retained provenance types.

### 6.4 Operational Gates

- The exact PMS 21 backend image, frontend version, database backup, and migration artifacts are recorded.
- The cleanup release window has a defined start, end, acceptance metrics, and owner approval.
- The release window ran with no new legacy occupancy writes.
- Rollback to the pre-PMS-21 binary is formally retired.
- Pre-cutover and pre-cleanup backups are retained outside normal short backup rotation.
- A restore drill or verified database-open check has been recorded for the cleanup backup.
- The standalone Podman deployment procedure in `docs/pms-21-operations-cutover-runbook.md` matches the real `api.pms.airportlounge.sk` container, bind mount, database path, environment file, network, and image.

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

The eight unmapped cleaning rows, one external-sale conflict, and 50 `needs_review` stays must receive explicit dispositions. Valid outcomes include correction, new-model linking, read-only archival, or documented deletion when the row is proven to be disposable test data. Absence of a mapping is not itself permission to delete.

### 7.4 Preserve Legacy-Only Metadata

Before dropping `occupancies`, audit whether the new model contains all business-relevant metadata currently stored only in old columns:

- Stay outcome reason, actor, and timestamp.
- Closure category, reason, actor, and timestamp.
- External-sale channel and reason metadata.
- Cleaning-exclusion reason, actor, and timestamp.
- Source/supersession provenance needed to explain historical events.

Required result:

- Copy relevant business metadata into canonical new-model fields or a read-only archive.
- Record fields intentionally discarded and the owner approval for each class.
- Do not add compatibility columns to `named_stays` merely to mirror every obsolete implementation detail.

## 8. Required Delivery Sequence

Cleanup must be delivered in separate, reviewable releases. Combining all phases into one migration is prohibited.

### 8.1 Release A - Stop Creating Legacy Dependencies

Release A is non-destructive. Legacy tables and columns remain available for observation and emergency diagnosis.

Required changes:

- Make raw-block reconciliation unconditional and stop all ICS writes to `occupancies` and `occupancy_nights`.
- Stop named-stay compatibility occupancy writes and migration-map creation.
- Remove synthetic finance occupancy creation from every active import/rematch path.
- Add any missing named-stay outcome and review operations.
- Resolve or archive known production exceptions.
- Reconcile Google Calendar over an approved range so active managed events have new-model ownership metadata.
- Enable export disablement for the release window if it has not already been enabled.
- Record database write monitoring proving no legacy rows are created or changed by normal operation.

Release A acceptance:

- Row hashes or equivalent snapshots show no runtime mutation of `occupancies`, `occupancy_nights`, `occupancy_api_tokens`, or `occupancy_stay_migration_map` during normal workflows.
- All new-model functional suites pass.
- Production monitoring shows no integration requiring a newly created legacy occupancy.

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

Release B acceptance:

- A production-like test database with legacy tables renamed or access-denied can run the complete application test suite, except migration-history tests that intentionally construct historical schemas.
- Runtime SQL tracing records no statement against `occupancies`, `occupancy_nights`, `occupancy_api_tokens`, or `occupancy_stay_migration_map`.
- Repository searches show no active API or UI dependency on old IDs.

### 8.3 Release C - Destructive Forward Migration

Release C adds a new numbered migration. Never edit migrations `000001` through `000037` or any later migration already applied outside a disposable local database.

Required migration order:

1. Create a durable archive of approved legacy identity/provenance if the owner chooses archival retention.
2. Rebuild integration tables to remove foreign keys and columns referencing `occupancies`.
3. Verify copied row counts, primary keys, unique constraints, child references, and critical values.
4. Drop `occupancy_nights`.
5. Drop `occupancy_api_tokens`.
6. Drop `occupancies`.
7. Remove `property_availability_blocks.source_occupancy_id` only after equivalent provenance is archived or explicitly discarded.
8. Archive or drop `occupancy_stay_migration_map` according to the approved retention decision.
9. Remove obsolete indexes and triggers.
10. Run `PRAGMA foreign_key_check` and latest-schema scans before commit.

Release C acceptance:

- A database migrated from migration `000001` to latest reaches the clean schema.
- A production-shaped database preserves every required row and value through table rebuilds.
- No latest-schema foreign key references `occupancies`.
- No latest-schema column uses `occupancy_id` as a current business identity.
- The application starts and all supported workflows pass with no legacy tables present.

### 8.4 Release D - Remove Transitional Tooling And Align Documentation

Release D may be combined with Release C only when the tooling cannot be needed to inspect or recover the cleanup migration.

Required changes:

- Remove the PMS 21 migration CLI after final audit artifacts are captured and no supported environment still needs the backfill.
- Remove the occupancy repair CLI and UI if all supported recovery is raw-block-based.
- Remove PMS 21 migration CLI packaging from `deploy/Dockerfile.backend`.
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

### 9.2 Legacy Occupancy Store

`backend/internal/store/occupancy.go` currently mixes source configuration, sync-run history, raw snapshots, legacy stay CRUD, closure behavior, finance mappings, export-token storage, and public export reads.

Required refactor and removal:

- Retain occupancy source configuration, property ICS secret handling, sync-run records, and raw-event snapshots.
- Move retained source/sync concerns into narrowly named files if needed; do not preserve a monolithic `occupancy.go` solely for historical naming.
- Remove the legacy `Occupancy` stay/block representation type after all callers migrate.
- Remove legacy occupancy insert, upsert, status, list, calendar, upcoming, export, closure, split, reopen, outcome, cleaning-exclusion, and finance-mapping methods.
- Remove token create/list/delete/authentication methods.
- Remove old representation constants and closure constants when no retained archive parser needs them.

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

Final analytics must operate from named-stay nights, availability blocks, named-stay finance links, and retained sync/source history only.

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
- Legacy wording fallback only if real Google events still require it; this is event-discovery compatibility, not occupancy-model compatibility.
- Date-scoped desired-state reconciliation and desired hashes.
- Cleaning event and event-log history.

### 9.8 Finance And Invoices

Remove from `backend/internal/store/finance_booking_payouts.go` and `backend/internal/store/finance_bookings_merge.go`:

- `legacyFindOrCreateOccupancyForPayoutStayDates` and statement equivalent.
- Synthetic occupancy insertion for payout/statement data.
- Generic ICS occupancy supersession caused by finance matching.
- Migration-map and occupancy fallback matching.
- Writes to `occupancies.finance_booking_id` or `finance_bookings.occupancy_id`.

Remove from `backend/internal/store/invoices.go` and invoice handlers:

- Occupancy candidate queries.
- `occupancy_id` request aliases and response fields.
- Occupancy fallback when a named-stay ID is absent.

Required naming alignment:

- Rename `/properties/{id}/invoices/occupancy-candidates` to a stay-oriented route.
- Name internal form/request fields `named_stay_id`, not `occupancy_id` holding a stay ID.
- Preserve the canonical finance booking and named-stay linkage without adding a reverse compatibility FK.

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
- A request to a removed route returns the normal router `404`, not a permanent compatibility `410` shim.
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

### 11.4 Invoice Naming

Update:

- `frontend/src/views/InvoicesView.vue`
- `frontend/src/views/invoices/InvoiceEditorForm.vue`

Required result:

- Form state and payload use `named_stay_id`.
- Candidate endpoint and response names are stay-oriented.
- No variable named `occupancy_id` stores a named-stay ID.

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
| `occupancies` | All integration FKs removed; metadata archived/migrated; zero runtime query. |

### 12.3 Migration Map Disposition

`occupancy_stay_migration_map` is not a business source of truth. It must not remain in runtime queries after Release B.

Before Release C, choose and document one disposition:

- Retain it as a read-only historical table with a name that clearly signals archival purpose.
- Copy it into a generic migration/audit archive and drop the live table.
- Export it with the reviewed cleanup artifact and drop it if retention outside the live database satisfies audit requirements.

Dropping it without preserving old-to-new attribution is prohibited while audit logs, support records, or external artifacts refer to old occupancy IDs.

### 12.4 Integration Table Rebuilds

SQLite table rebuilds must preserve primary keys and child relationships.

| Table | Remove | Preserve |
| --- | --- | --- |
| `nuki_access_codes` | `occupancy_id`, legacy occupancy unique index, legacy check branch | IDs, `named_stay_id`, PIN data, external IDs, validity, status, errors, sync links, timestamps, revocation |
| `nuki_guest_daily_entries` | `occupancy_id`, legacy unique index, legacy check branch | IDs, `named_stay_id`, day, first entry, event reference, timestamps |
| `finance_bookings` | `occupancy_id` and occupancy indexes | IDs, `named_stay_id`, source evidence, amounts, dates, transactions, import state |
| `invoices` | `occupancy_id` and occupancy unique index | IDs, `named_stay_id`, finance link, numbering, snapshots, totals, files |
| `cleaning_calendar_events` | `occupancy_id`, `next_occupancy_id`, occupancy indexes/uniqueness | IDs, new ownership, Google IDs, identity, desired hash, dates, status, warning/error state |

Required preservation checks:

- `nuki_event_logs.nuki_access_code_id` still resolves to the same code ID.
- Invoice files still resolve to the same invoice ID.
- Cleaning event logs still resolve to the same cleaning event ID.
- Finance transaction and merge relationships remain unchanged.
- No copied row violates new-model uniqueness.

### 12.5 Applied Migration Files

Retain every historical migration file, including those that create or alter legacy tables. Fresh databases must replay the historical sequence and then apply the new cleanup migration. Do not rewrite history to make old tables appear never to have existed.

## 13. Cleanup Readiness Queries

The implementation must provide a repeatable read-only command or report containing equivalent checks. The exact SQL may evolve with schema changes, but every condition must be represented.

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
WHERE occupancy_id IS NOT NULL
  AND named_stay_id IS NULL;

SELECT COUNT(*) AS invoices_without_stay
FROM invoices
WHERE occupancy_id IS NOT NULL
  AND named_stay_id IS NULL;

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

SELECT COUNT(*) AS active_export_tokens
FROM occupancy_api_tokens;
```

Before dropping `occupancies`, also compare old and new values for stay outcomes, date ranges, statuses, display names, finance links, and required provenance. A count-only report is insufficient when values can differ.

After the destructive migration:

```sql
PRAGMA foreign_key_check;

SELECT name, sql
FROM sqlite_schema
WHERE lower(COALESCE(sql, '')) LIKE '%references occupancies%'
   OR lower(COALESCE(sql, '')) LIKE '% occupancy_id%';
```

Every returned row must be either eliminated or explicitly approved as historical archive schema that cannot participate in runtime behavior.

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
- Removed APIs return `404` and no hidden compatibility alias remains.
- New requests containing `occupancy_id` are rejected rather than silently interpreted.
- A fresh database migrates from `000001` through the cleanup migration.
- A populated pre-cleanup database preserves IDs and critical values across every table rebuild.
- `PRAGMA foreign_key_check` is clean after migration.
- Latest schema has no foreign key to `occupancies`.
- Repository route/OpenAPI coverage remains complete after route removal.
- Public occupancy export and token management are absent from backend and frontend.

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
- `docs/pms-21-operations-cutover-runbook.md`: retain standalone Podman commands only for the actual production topology.

Historical artifacts must remain intact:

- Stage verification documents under `docs/audits/`.
- Production audit/apply/idempotency JSON.
- Divergence and remediation documents, marked historical or superseded rather than rewritten as if their findings never existed.
- ADRs, amended or superseded through normal ADR practice when decisions changed.

## 16. Podman Operational Requirements

Production PMS runs as the standalone Podman container `api.pms.airportlounge.sk` without Compose. It uses network `internet_enabled`, image `ghcr.io/ai-slop-code/pms-backend:latest`, environment file `./pms.env`, and the bind mount `/mnt/main_storage/containers/data/api.pms.airportlounge.sk:/data:Z`. Cleanup instructions must follow this topology.

Required operational properties:

- Record the exact image ID behind `ghcr.io/ai-slop-code/pms-backend:latest` before the maintenance window.
- Stop the backend before audit/apply when the production SQLite volume is used directly.
- Take and verify a backup before migration.
- Stop the backend, mount `/mnt/main_storage/containers/data/api.pms.airportlounge.sk` directly into each one-off migration container, then remove and recreate the stopped API afterward with its normal `podman run` command.
- Mount the data directory at `/data:ro,Z` for audit and `/data:Z` for approved apply/cleanup.
- Pass `/data/pms.db` explicitly to migration tools, or the exact alternative `DATABASE_PATH` configured in `pms.env`.
- Redirect audit output to a host path outside the application volume.
- Recreate the application using the existing direct `podman run` command; do not introduce Compose, systemd, Quadlet, or `--volumes-from` as cleanup dependencies.
- Record image, container, volume, database path, environment source, command output, and timestamps.

The exact command templates belong in `docs/pms-21-operations-cutover-runbook.md`, not duplicated across implementation code.

## 17. Rollback Rules

### 17.1 Before Release C

- Release A and B may roll back by deploying the prior application image while preserving additive schema.
- Do not run historical down migrations as an operational rollback.
- Keep the pre-cutover backup pinned beyond normal retention.

### 17.2 After Release C

- The old binary is unsupported because its required tables and columns no longer exist.
- Rollback requires restoring the pre-cleanup database backup together with the compatible prior application image.
- Restoring that backup discards writes made after cleanup unless a separately designed forward recovery is performed.
- This consequence must be acknowledged in the cleanup approval record.

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
- Migration-map provenance is preserved according to the approved retention decision.
- Required historical and integration data is preserved.
- Fresh and upgraded database migration tests pass.
- Backend, frontend, OpenAPI, and end-to-end verification pass.
- Active documentation describes one model rather than compatibility and target models side by side.
- The cleanup audit, owner approval, backup reference, deployed image, and verification results are retained.

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
