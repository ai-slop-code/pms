# PMS 21 - Raw Booking Blocks and Named Stays Migration Plan

Status: Stage 2 guarded apply and the PMS 21 remediation are locally implemented and tested, but no production audit or apply has run. Production backfill, version cutover, safety-gate changes, and destructive cleanup remain blocked pending owner-run production audit approval.
Source request: `spec/PMS_20_Occupancy_code_analysis_and_business_logic.md`  
Goal: provide a staged technical plan for replacing the current overloaded occupancy model with explicit Raw booking date blocks and Named stays.

## Current Stage

Current implementation stage: **Stage 11 - Cleanup**.

Local status:

- Stages 0 and 1 are implemented.
- Stage 2 has local dry-run and guarded apply tooling with shared classification and idempotency tests; production apply has not run.
- Stage 3 is implemented and locally verified behind `PMS21_RAW_BLOCKS_DUAL_WRITE`, which remains default-off.
- Stage 4 is implemented and locally verified with backend tests in `docs/audits/PMS_21_stage4_local_verification_2026-07-13.md`.
- Stage 5 is implemented and locally verified with backend/frontend checks in `docs/audits/PMS_21_stage5_local_verification_2026-07-13.md`.
- Stage 6 is implemented and locally verified with backend checks in `docs/audits/PMS_21_stage6_local_verification_2026-07-13.md`.
- Stage 7 is implemented and locally verified with backend/frontend checks in `docs/audits/PMS_21_stage7_local_verification_2026-07-13.md`.
- Stage 8 is implemented and locally verified with backend/frontend checks in `docs/audits/PMS_21_stage8_local_verification_2026-07-13.md`.
- Stage 9 is implemented and locally verified with backend/frontend checks in `docs/audits/PMS_21_stage9_local_verification_2026-07-14.md`.
- Stage 10 is implemented and locally verified with backend/frontend checks in `docs/audits/PMS_21_stage10_local_verification_2026-07-14.md`.
- Stage 11 non-destructive cleanup gate is implemented and locally verified with backend checks in `docs/audits/PMS_21_stage11_local_verification_2026-07-14.md`.

Stage 5 completion review:

- Combined calendar DTO exposes raw blocks, named stays, availability blocks, current local cleaning event summaries, Nuki badge state, and raw-source warning state.
- Calendar UI can promote raw blocks to named stays and create external, maintenance, personal-use, or manually confirmed Booking.com named stays from empty or raw-covered nights.
- Availability blocks are rendered distinctly and can be created/edited from the calendar through Stage 5 API endpoints.
- Dashboard widgets remain on legacy compatibility data until Stage 9, as allowed by the Stage 5 plan.

Stage 6 completion review:

- Cleaning reconciliation now has a date-scoped entrypoint and the legacy full-property entrypoint delegates to it for the existing default window.
- Desired cleaning state is built from PMS 21 raw blocks and named stays when those sources exist, with legacy occupancy fallback for ranges that have not moved to the PMS 21 model.
- Google Calendar support can list existing events and match PMS-owned events by stored Google ID, PMS private metadata, deterministic cleaning identity, owner metadata, or same-date title fallback.
- Local cleaning events now persist PMS 21 ownership fields during reconciliation: `named_stay_id`, `raw_booking_block_id`, `cleaning_identity`, `desired_hash`, and `last_google_seen_at`.
- Desired-state hashes prevent Google patch calls for unchanged synced events.
- Named-stay create/promote/update/status workflows request cleaning reconciliation only for affected old/new stay ranges.

Stage 6 remaining work:

- Production Google Calendar behavior must be validated against real calendars before rollout is considered approved.
- Existing production cleaning rows still require approved backfill or natural reconciliation to populate PMS 21 ownership fields; production backfill apply remains blocked pending audit approval.
- Legacy local cleaning rows that cannot be matched to a named stay or raw block still need production audit review before any destructive cleanup.
- Stage 8+ downstream consumers still run on legacy compatibility where planned; Stage 6 did not cut over finance, analytics, messages, or cleaning salary/daily logs.

Stage 7 completion review:

- Nuki access code generation/listing now selects active confirmed `named_stays` with Nuki-eligible stay types (`booking_com`, `external`) instead of raw/legacy occupancy rows.
- `nuki_access_codes.named_stay_id` and `nuki_guest_daily_entries.named_stay_id` are backfilled by migration `000034_nuki_named_stay_cutover` from `occupancy_stay_migration_map`, while legacy `occupancy_id` values remain preserved.
- Existing active legacy codes can still be revoked through compatibility cleanup until all production rows are relinked.
- Nuki upcoming-stays API and UI use `stay_id` as the primary identity; deprecated `occupancy_id` remains in API payloads and accepted generation input for compatibility.
- Guest check-in reconciliation and heatmap reads now resolve and query named-stay attribution while keeping historical occupancy attribution fields.
- Nuki stay-name editing updates `named_stays.display_name` rather than mutating legacy occupancy guest fields as business truth.
- Stage 7 Nuki endpoint contracts are covered in `spec/openapi.yaml`.

Stage 7 remaining rollout/audit work:

- No additional local Stage 7 implementation work remains; the implementation is complete and locally verified.

- Production Nuki backfill/apply remains blocked pending audit approval.
- Production rows that cannot map to exactly one named stay still require review before enabling Nuki named-stay rollout.
- Existing production active PINs must be validated after relinking to confirm generated PINs and external Nuki IDs are preserved.

Stage 8 completion review:

- Finance booking and invoice rows now use `named_stay_id` as the primary stay identity, with legacy `occupancy_id` retained only for compatibility and rollback.
- Migration `000035_finance_invoice_named_stay_cutover` backfills `finance_bookings.named_stay_id` and `invoices.named_stay_id` from `occupancy_stay_migration_map` and linked finance bookings.
- Payout import/rematch uses deterministic named-stay matching only; it no longer creates synthetic `occupancies` from finance imports.
- Explicit payout create/link workflow creates or links a first-class `named_stays` row and maps the finance row to that stay.
- Booking.com finance cancellation status marks linked named stays for review instead of automatically cancelling user-owned stays.
- Invoice creation/update and invoice stay candidates use named-stay identity; deprecated occupancy compatibility remains accepted where it can resolve through the migration map.
- Finance reset deletes invoices and invoice files linked through finance booking or named-stay finance links, while preserving named stays and manual external revenue.
- Booking payout UI maps to named stays and exposes manual external revenue entry for mapped external stays.
- Stage 8 finance/invoice endpoint contracts are covered in `spec/openapi.yaml`.

Stage 8 remaining rollout/audit work:

- No additional local Stage 8 implementation work remains; the implementation is complete and locally verified.
- Production finance/invoice named-stay backfill/apply remains blocked pending audit approval.
- Production finance bookings or invoices that cannot map to exactly one named stay still require review before enabling the finance named-stay rollout.
- Existing production invoices and payout mappings must be validated after relinking to confirm invoice numbers, files, and finance transaction links are preserved.
- Stage 11 hard cleanup and legacy storage/drop decisions remain pending.

Stage 9 completion review:

- Analytics stay selection now reads active confirmed `named_stays` / `named_stay_nights` and excludes raw booking blocks from sold/revenue metrics.
- External named stays count as sold/revenue only when linked finance data or manual revenue exists; unfunded external, maintenance, personal-use, review-required, availability-block, and legacy closed nights reduce bookable availability without increasing sold nights.
- Returning guest, demand, heatmap, gaps, ADR, RevPAR, net-per-stay, finance performance, cancellation, and pace helpers are named-stay aware, with legacy fallback only for properties that have no named-stay rows.
- Guest message generation uses `stay_id` and message pickers list message-eligible named stays; deprecated `occupancy_id` remains accepted as compatibility.
- Cleaning-staff messages derive from final cleaning-required named stays rather than raw booking blocks.
- Dashboard upcoming stays and check-in KPI widgets read named stays and expose `stay_id` as primary identity.
- Dashboard/message OpenAPI coverage was added for changed Stage 9 DTOs.

Stage 9 remaining rollout/audit work:

- No additional local Stage 9 implementation work remains; the implementation is complete and locally verified.
- Production analytics/messages/dashboard cutover remains blocked pending PMS 21 production backfill and audit approval.

Stage 10 completion review:

- Legacy occupancy-as-stay API routes now emit explicit `Deprecation` and `Warning` headers while retaining compatibility behavior.
- Public occupancy export and export-token endpoints are documented as deprecated in OpenAPI and backend responses.
- `PMS21_OCCUPANCY_EXPORT_DISABLED` can disable the public export route with `410 Gone` without dropping token storage.
- Occupancy sync UI no longer exposes export-token creation, token listing, curl snippets, or n8n guidance, and it no longer calls token-management endpoints.
- Frontend OpenAPI types were regenerated after documenting the deprecated export/token surface.

Stage 10 remaining rollout/audit work:

- No additional local Stage 10 implementation work remains; the implementation is complete and locally verified.
- Production export disablement requires explicit `PMS21_OCCUPANCY_EXPORT_DISABLED=1` rollout approval.
- Legacy occupancy-as-stay routes remain compatibility endpoints until production data/backfill and caller audits approve hard removal.
- `occupancy_api_tokens` storage remains in place for compatibility/rollback and can be dropped only in Stage 11+ cleanup after a release cycle.

Stage 11 local completion review:

- Added default-off `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` as the explicit cleanup gate for stopping new legacy occupancy compatibility writes.
- Server config now passes the cleanup gate to `store.Store`.
- When the gate is enabled, finance payout/statement matching no longer creates synthetic legacy `occupancies` rows if no legacy match exists, and the legacy generic-ICS supersede helper becomes a no-op.
- When the gate is enabled, named-stay create/update/status workflows no longer create or update derived compatibility `occupancies` rows or new compatibility migration-map rows.
- Default behavior remains unchanged, preserving legacy compatibility writes until production approval.

Stage 11 remaining rollout/audit work:

- Keep `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` disabled in production until production backfill/audit approval and the required release-cycle window are complete.
- Hard removal of old write paths, obsolete columns, token storage, and old occupancy-as-stay routes remains blocked by production verification.
- Production cleanup still requires a clean migration conflict report or documented resolution for every ambiguity.

2026-07-18 remediation review:

- Migration `000036_nuki_named_stay_primary` rebuilds Nuki access-code and guest-entry tables with nullable legacy occupancy identity while preserving row IDs, encrypted PIN values, external Nuki IDs, run links, event-log links, and historical attribution. Local preservation tests pass.
- Migration `000037_finance_evidence_confirms_named_stays` upgrades payout/statement-backed stays whose only review reason is the Stage 2 `legacy_non_reservation_stay` classification; cancellation and other operational review reasons remain intact. Missing historical ICS remains provenance rather than an actionable warning, while true raw-coverage conflicts stay visible.
- Named-stay patch/status flows reconcile Nuki generation/revocation after the database update and retain visible error state when external Nuki calls fail.
- Raw sync recomputes source-link health from the union of active linked raw nights; missing, shrunken, recovered, adjacent, and gap coverage are locally tested without mutating named-stay business fields.
- Calendar sold-night eligibility is backend-provided and follows analytics rules. Analytics active/closed day metrics consume active `named_stay_nights` and divergence tests pass.
- OpenAPI represents the complete registered route inventory, not complete contracts for route-only entries. Touched PMS 21/Nuki/cleaning/dashboard contracts are concrete, generated frontend types are refreshed, and route coverage is tested.

Production status:

- Production audit artifact is absent; expected reviewed artifact name is `docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md` or an equivalent recorded operational artifact.
- Stage 2 apply/backfill is implemented locally but has not run in production.
- Production gate enablement is not approved.
- Legacy occupancy writes must remain enabled until named-stay-primary dependencies, including Nuki, are verified safe.
- Downstream production version cutover is not approved.
- Production version-switch instructions are documented in `docs/pms-21-operations-cutover-runbook.md`; the owner must stop before apply unless the reviewed production audit has zero severe conflicts and explicitly accounts for review-required rows.

## Executive Summary

Booking.com ICS events can now represent a continuous blocked date range instead of one real customer stay. The PMS currently has partial support for this through PMS_19: upstream UID ownership, `representation_kind`, `occupancy_nights`, provisional cleaning events, and named-stay endpoints already exist. However, the implementation still overloads the `occupancies` table as raw ICS block, named stay, manual closure, external sale, synthetic finance stay, analytics source, Nuki source, payout mapping target, and cleaning-control object.

The requested business model should be implemented by introducing explicit domain objects:

- `raw_booking_blocks`: synced from Booking.com ICS, not used for analytics, Nuki, payout truth, or final cleaning truth.
- `named_stays`: user-controlled source of truth for guest stays, external stays, maintenance, personal use, analytics, Nuki, payout mapping, and final cleaning state.
- `named_stay_nights`: night-level capacity and analytics source for named stays.
- `raw_booking_block_nights`: night-level raw coverage source for UI visibility and provisional cleaning.
- `stay_source_links`: relationship between named stays and raw blocks, preserving partial promotion and leftover raw coverage.
- `property_availability_blocks`: non-stay blocked availability migrated from true legacy closures/off-market periods when they are not maintenance or personal-use stays.

Do not implement this as a big rewrite. Add the new model alongside current tables, backfill it, dual-read in safe places, then migrate each integration one at a time.

## Resolved Product Decisions

- Booking.com raw block sync remains controlled by the global `OCCUPANCY_SYNC_INTERVAL_MINUTES` configuration.
- Provisional raw-block cleaning placeholders are created on checkout-placeholder dates. A raw block `2026-07-09 -> 2026-07-12` creates provisional cleanings on `2026-07-10`, `2026-07-11`, and `2026-07-12`.
- Promoting one multi-night named stay removes intermediate provisional cleaning placeholders and keeps or converts only the final checkout cleaning, unless cleaning is disabled for the named stay.
- Partial promotion keeps provisional cleanings for leftover raw-block checkout-placeholder dates that are not covered by a named stay's final cleaning.
- Named stay creation triggers Nuki generation synchronously for Nuki-eligible stay types. Failures must be visible as a small badge in the relevant occupancy calendar cell.
- Named stay overlap is prohibited. Use a hard no-overlap rule for active named stay nights.
- External, maintenance, and personal-use named stays may be created on empty nights or raw-block nights, but may not overlap another active named stay.
- Dashboard, message generation, cleaning daily logs/salary, and Nuki guest daily entries should migrate in the same relevant cutover stages, not as indefinite follow-ups.
- Public occupancy export can be dropped as a feature; Google Calendar events are the external calendar integration path.
- New stay/raw-block endpoints must be added to `spec/openapi.yaml` before frontend implementation. Generated OpenAPI types should become the source of truth instead of hand-written frontend API types where practical.
- PMS will retain `named_stays` names, source references, and copied finance/guest data; no additional named-stay PII redaction/retention behavior is required beyond existing system backup/security controls.
- Named stay operations use the existing Occupancy module permissions.
- Provisional cleaning event title is exactly `Upratovanie`. Final named-stay cleaning uses configured final cleaning title/labels.
- `maintenance` and `personal_use` named stays reduce available/bookable nights and do not count as sold/occupied revenue nights.
- `external` named stays count as sold only when they have linked finance data or a manually entered revenue amount.
- Booking.com cancellation/status changes from finance or statement data must be surfaced for user confirmation before changing a named stay status.
- Raw-source cancellation/missing-source badges are acceptable, and they must clear automatically once the named stay is relinked to active raw block coverage.
- Named stays are soft-deleted only. Use `archived` for removal and `cancelled` for confirmed cancellation; do not hard-delete stays that have finance, invoice, Nuki, cleaning, message, or audit references.
- Legacy true closures/off-market blocks must continue reducing bookable availability after migration, either through a new non-stay availability-block table or a retained legacy compatibility read model. Do not silently convert `closed` rows into maintenance stays.
- Existing finance-synthetic occupancies may be backfilled only as review-required legacy-import named stays when needed to preserve production data. New finance imports must not silently create named stays.

## Current Architecture And Data Flow

### Backend And Frontend Shape

- Backend is Go with SQLite migrations in `backend/internal/migrate`.
- Store layer in `backend/internal/store` owns most persistence and domain logic.
- Service packages exist for integration boundaries: `backend/internal/occupancy`, `backend/internal/cleaningcalendar`, `backend/internal/nuki`.
- API routes are registered in `backend/internal/api/server.go`.
- Frontend is Vue 3 in `frontend/src`, with occupancy UI in `frontend/src/views/OccupancyView.vue` and child components under `frontend/src/views/occupancy`.

### Booking.com ICS Sync

Relevant files:

- `backend/internal/occupancy/parse.go`
- `backend/internal/occupancy/sync.go`
- `backend/internal/store/occupancy.go`
- `backend/internal/store/occupancy_reconciliation.go`
- `backend/internal/store/occupancy_nights.go`
- migrations `000002_occupancy.up.sql`, `000029_booking_ics_reconciliation.up.sql`, `000030_cleaning_provisional_per_night.up.sql`, `000031_ics_dtstamp.up.sql`

Current flow:

1. `occupancy.Service.SyncProperty` fetches a property's Booking.com ICS URL from `property_secrets.booking_ics_url` when `occupancy_sources.active = 1`.
2. `ParseICalendarDetailed` parses VEVENT rows. All-day `DTSTART;VALUE=DATE` and `DTEND;VALUE=DATE` are correctly treated as UTC civil dates with exclusive `DTEND`.
3. Each parsed event is persisted to `occupancy_raw_events` as a per-sync snapshot.
4. Parsed events are converted into `store.DesiredBlock` and passed to `Store.ReconcileBookingICSSync`.
5. `ReconcileBookingICSSync` upserts an aggregate row into `occupancies` using the Booking UID as `source_event_uid`, sets `upstream_source_type = booking_ics`, `upstream_event_uid = UID`, and `representation_kind = unnamed_block`.
6. `reconcileUpstreamCoverageTx` rebuilds `occupancy_nights` for that upstream UID. Existing operator rows, named-stay rows, and closure rows win nights over the aggregate unnamed block.
7. Missing current/future upstream UIDs are marked `deleted_from_source` and their `occupancy_nights` are deactivated.
8. Partial parse failures are safe: the sync run gets `partial_no_mutation` and does not mutate occupancies.

Good current patterns to preserve:

- ICS parsing uses half-open date ranges and avoids local-time shifts.
- Sync has a per-property job lease.
- Raw snapshots are kept in `occupancy_raw_events`.
- Partial parse failures do not cause false deletion.
- `occupancy_nights` provides a useful capacity-one primitive.

Architectural problem:

- The current aggregate row in `occupancies` is both the raw Booking.com block and, sometimes, a stay-like object. `representation_kind` improves this but does not create a clean source-of-truth boundary.

### Current Named Stay Flow

Relevant files:

- `backend/internal/api/occupancy_named_stay_handlers.go`
- `backend/internal/store/occupancy_reconciliation.go`
- `frontend/src/views/OccupancyView.vue`
- `frontend/src/views/occupancy/OccupancyCalendar.vue`

Current flow:

- Frontend can create a named stay using `POST /api/properties/{id}/occupancy-blocks/{upstreamUid}/named-stays`.
- Backend calls `Store.CreateNamedStay`, which inserts a new row into `occupancies` with `source_type = manual`, synthetic UID `named:{upstreamUID}:{YYYYMMDD}`, `guest_display_name`, and `representation_kind = named_stay`.
- Edits and deletes operate through `/occupancies/{occupancyId}/named-stay` and mutate the same `occupancies` row.
- Partial naming works through `occupancy_nights`: the aggregate block keeps unclaimed nights and the named row wins selected nights.

What matches the requested business logic:

- User can promote part of a raw block to a named stay.
- Leftover raw coverage remains via aggregate `unnamed_block` coverage.
- Named stays are preferred over raw block coverage in `occupancy_nights`.

What does not match:

- A named stay is not a first-class object. It is an `occupancies` representation row.
- Stay type is missing. Current labels are closure/external sale, not requested stay types `booking_com`, `external`, `maintenance`, `personal_use`.
- Cleaning preference is stored as `occupancies.cleaning_calendar_excluded`; it does not model default cleaning rules by stay type.
- Nuki, analytics, finance, invoices, messages, cleaning, and UI still use `occupancy_id` as the stay identity.

### Google Calendar Cleaning

Relevant files:

- `backend/internal/cleaningcalendar/service.go`
- `backend/internal/cleaningcalendar/google_client.go`
- `backend/internal/store/cleaning_calendar.go`
- migrations `000023_cleaning_calendar.up.sql`, `000028_cleaning_calendar_exclusion.up.sql`, `000029_booking_ics_reconciliation.up.sql`, `000030_cleaning_provisional_per_night.up.sql`

Current flow:

- `cleaningcalendar.Service.ReconcileProperty` reconciles a fixed window: 30 days past to 365 days future.
- It lists cleaning-eligible `occupancies`, then expands each row through `CleaningCheckoutDatesForOccupancy`.
- `unnamed_block` rows create per-night provisional checkouts with `cleaning_kind = provisional_block`.
- `named_stay` and `external_sale` rows create one checkout event with `cleaning_kind = named_stay`.
- `cleaning_calendar_events` has an identity key `(property_id, upstream_event_uid, checkout_date, cleaning_kind)` where present.
- Google events are created or patched through stored `google_event_id` and include private metadata: `pms_property_id`, `pms_occupancy_id`, `pms_cleaning_event_id`, `pms_managed_event_version`.
- Deletion removes active local cleaning events that are no longer desired.

Important gap against the request:

- Reconciliation is not date-scoped to the currently affected date range. It scans the whole fixed window and may patch every desired event each run.
- The Google client can insert, patch, and delete by stored ID, but it cannot list events for a specific date and match PMS-owned events by metadata or wording fallback.
- Idempotence exists in local DB identity, but Google calls are still made whenever `ReconcileProperty` runs, even if event content is unchanged.
- Provisional title is currently derived from settings with labels like `Upratovanie: Bez Hosta`, not the requested generic provisional `Upratovanie`.

### Nuki Access

Relevant files:

- `backend/internal/nuki/service.go`
- `backend/internal/store/nuki.go`
- migrations `000004_nuki.up.sql`, `000005_nuki_keypad_codes.up.sql`, `000020_nuki_guest_daily_entries.up.sql`
- `frontend/src/views/NukiView.vue`

Current flow:

- `nuki_access_codes.occupancy_id` is a required foreign key to `occupancies`.
- `ListOccupanciesForNukiSync` selects active, non-superseded, non-closure occupancy rows with non-empty `guest_display_name`.
- `GenerateCodes` creates or updates codes for those occupancy rows.
- Upcoming Nuki UI uses `/nuki/upcoming-stays`, also based on `occupancies` and `nuki_access_codes.occupancy_id`.

Current behavior is close to the new business rule because raw blocks generally have no `guest_display_name`, but the rule is implicit and fragile. Nuki should explicitly read `named_stays` and should never know about raw blocks.

### Analytics

Relevant files:

- `backend/internal/store/analytics.go`
- `backend/internal/store/analytics_statement.go`
- `backend/internal/api/analytics_handlers.go`
- frontend analytics under `frontend/src/views/analytics`

Current flow:

- `ListActiveOccupanciesInDateRange` reads `occupancies` where `status IN ('active','updated')` and `closure_state != 'closed'`.
- Monthly, weekly, DOW, gap, demand, returning guest, and performance calculations derive stay nights from `occupancies.start_at` and `occupancies.end_at` or from `occupancy_nights` in some paths.
- `OccupancyMetricNights` already uses `occupancy_nights` and counts `RepresentationNamedStay` or `external_sale` as guest nights.
- Finance revenue metrics join `finance_bookings.occupancy_id` to occupancy rows.

Main problem:

- Analytics still treats raw Booking blocks as active stays in multiple paths, especially when the aggregate `unnamed_block` remains active. This violates the requested rule that raw blocks do not count until promoted to a named stay.

### Finance, Payouts, Statements, Invoices

Relevant files:

- `backend/internal/store/finance_booking_payouts.go`
- `backend/internal/store/finance_bookings_merge.go`
- `backend/internal/api/finance_handlers.go`
- `backend/internal/api/booking_payout_display.go`
- `backend/internal/store/invoices.go`
- `backend/internal/api/invoice_handlers.go`
- `frontend/src/views/BookingPayoutsView.vue`
- `frontend/src/views/InvoicesView.vue`

Current flow:

- `finance_bookings.occupancy_id` points to `occupancies`.
- `occupancies.finance_booking_id` points back to `finance_bookings`.
- Matching is date-based in `FindOccupancyForStayDates` and related functions.
- If no matching occupancy exists, `FindOrCreateOccupancyForPayoutStayDates` or `FindOrCreateOccupancyForStatementStayDates` creates a synthetic `occupancies` row with source type `booking_payout` or `booking_statement`.
- `SupersedeGenericICSBlocksForFinanceStayDates` can mark generic ICS blocks deleted from source when a finance-derived stay is created.
- Invoice candidates use occupancy candidates.

Main problem:

- Finance creates and maps to `occupancies`, so imported payout/reservation data can create a stay-like occupancy without a true named stay. Under the target model, finance should map to `named_stays`, with any automatic creation handled as an explicit imported or suggested named stay with audit trail.

### Manual Closures, External Sales, Stay Outcomes, Cleaning Exclusion

Relevant files:

- `backend/internal/api/occupancy_closure_handlers.go`
- `backend/internal/store/occupancy.go`
- `frontend/src/views/occupancy/closure.ts`
- `frontend/src/views/OccupancyView.vue`
- migrations `000019_occupancy_closure.up.sql`, `000027_stay_outcome_overrides.up.sql`, `000028_cleaning_calendar_exclusion.up.sql`

Current flow:

- Closure and external sale are labels on `occupancies.closure_state`.
- Stay outcomes are on `occupancies.stay_outcome`.
- Cleaning exclusion is on `occupancies.cleaning_calendar_excluded`.

Target direction:

- Maintenance and personal use should be `named_stays.stay_type`, not a closure label.
- External stay should be a first-class named stay type, not `closure_state = external_sale`.
- Closure/off-market may remain a separate concept only if it is not a stay and should reduce bookable availability.

## Current Schema Affected By The Migration

Existing tables directly affected:

- `occupancies`: overloaded current stay/block/manual/finance representation table.
- `occupancy_nights`: current night-level capacity table keyed to `occupancies`.
- `occupancy_raw_events`: per-sync ICS snapshots.
- `occupancy_sources`, `occupancy_sync_runs`: sync source/run metadata.
- `cleaning_calendar_events`: currently references `occupancies` and upstream UID identity.
- `nuki_access_codes`, `nuki_event_logs`: currently keyed by `occupancy_id`.
- `finance_bookings`: currently has `occupancy_id` and is also referenced by `occupancies.finance_booking_id`.
- `invoices`: currently references `occupancy_id` and finance booking ID.
- `finance_transactions`: source references can describe booking payouts and generated cleaning salary.
- `api_audit_logs`: entity names need updates but data can remain.

Existing persisted data to migrate:

- Active `occupancies` rows with `representation_kind = unnamed_block` become `raw_booking_blocks`.
- Active/superseded/deleted `occupancies` rows with `representation_kind = named_stay` or non-empty `guest_display_name` become `named_stays` where safe.
- Active true closure/off-market `occupancies` become `property_availability_blocks` or remain in a legacy availability compatibility read until classified.
- `occupancy_nights` for raw rows become `raw_booking_block_nights`; for named rows become `named_stay_nights`.
- `cleaning_calendar_events` should gain `named_stay_id` and `raw_booking_block_id` while preserving `google_event_id`.
- `nuki_access_codes` should gain `named_stay_id`, backfilled from `occupancy_id` where the occupancy maps to a named stay.
- `finance_bookings` should gain `named_stay_id`, backfilled from current `occupancy_id` mapping where possible.
- `invoices` should gain `named_stay_id`, backfilled from current `occupancy_id` mapping where possible.

## ICS Event Equals Stay Assumptions

The following places currently imply or preserve the old assumption:

- `backend/internal/store/occupancy.go`: `Occupancy` struct contains raw source fields, guest name, closure state, finance mapping, stay outcome, cleaning exclusion, and representation metadata in one type.
- `backend/internal/store/occupancy.go`: `ListOccupancies`, `ListOccupanciesBetween`, `ListOccupanciesForExport`, `ListUpcomingOccupancies` expose `occupancies` as stays.
- `backend/internal/store/occupancy_reconciliation.go`: `CreateNamedStay` creates another `occupancies` row instead of a `named_stays` row.
- `backend/internal/store/occupancy_reconciliation.go`: sync disappearance can mark named-stay representations `deleted_from_source`, which conflicts with the new rule that ICS sync must not delete or mutate named stays.
- `backend/internal/store/cleaning_calendar.go`: cleaning eligibility reads `occupancies` for both provisional and final cleaning.
- `backend/internal/cleaningcalendar/service.go`: reconciliation builds desired final cleaning state from `occupancies` and updates a broad window.
- `backend/internal/store/nuki.go`: Nuki sync and Nuki UI use `occupancies` as stays, filtered by `guest_display_name`.
- `backend/internal/nuki/service.go`: generated code labels and windows are based on `store.Occupancy`.
- `backend/internal/store/analytics.go`: active analytics use `occupancies` in many queries.
- `backend/internal/store/analytics_statement.go`: statement analytics join finance bookings to `occupancies`.
- `backend/internal/store/finance_booking_payouts.go`: date matching and synthetic stay creation create or link `occupancies`.
- `backend/internal/store/finance_bookings_merge.go`: mapping/cancellation links update `occupancies.finance_booking_id`.
- `backend/internal/api/booking_payout_display.go`: payout display exposes occupancy options.
- `backend/internal/api/invoice_handlers.go` and `backend/internal/store/invoices.go`: invoice candidates and invoice data use `occupancy_id`.
- `frontend/src/api/types/occupancy.ts`: frontend type names the primary list item `Occupancy` and treats it as stay/block/closure.
- `frontend/src/views/OccupancyView.vue`: create/edit named stay dialog uses occupancy IDs and upstream UIDs.
- `frontend/src/views/occupancy/OccupancyCalendar.vue`: calendar chips count occupancy rows as occupied nights.
- `frontend/src/views/BookingPayoutsView.vue`, `InvoicesView.vue`, `NukiView.vue`, message views and dashboard components use `occupancy_id` for stay identity.

## Target Domain Model

### Raw Booking Date Block

Purpose:

- Durable synced representation of a Booking.com ICS blocked date range.
- Visible in occupancy calendar.
- Creates provisional cleaning placeholders for every blocked night.
- Not counted in analytics.
- Not visible to Nuki.
- Not directly mapped to payouts or invoices.

Proposed table: `raw_booking_blocks`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `source_type TEXT NOT NULL DEFAULT 'booking_ics'`
- `source_event_uid TEXT NOT NULL`
- `check_in_date TEXT NOT NULL`
- `check_out_date TEXT NOT NULL`
- `status TEXT NOT NULL CHECK(status IN ('active','deleted_from_source','conflict'))`
- `raw_summary TEXT`
- `content_hash TEXT NOT NULL`
- `source_dtstamp TEXT`
- `first_seen_sync_run_id INTEGER REFERENCES occupancy_sync_runs(id) ON DELETE SET NULL`
- `last_sync_run_id INTEGER REFERENCES occupancy_sync_runs(id) ON DELETE SET NULL`
- `imported_at TEXT NOT NULL`
- `last_synced_at TEXT NOT NULL`
- `deleted_from_source_at TEXT`
- `conflict_reason TEXT`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Constraints and indexes:

- `UNIQUE(property_id, source_type, source_event_uid)`.
- Index `(property_id, check_in_date, check_out_date)`.
- Index `(property_id, status)`.
- Index `(property_id, last_synced_at DESC)`.
- Optional check: `check_out_date > check_in_date` enforced in service code because SQLite check date comparisons are text-based but ISO dates are safe.
- Sync updates raw block content in place while keeping `status = active`; there is no raw-block `cancelled` business state because ICS disappearance only means source coverage is missing/deleted.

Proposed table: `raw_booking_block_nights`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `raw_booking_block_id INTEGER NOT NULL REFERENCES raw_booking_blocks(id) ON DELETE CASCADE`
- `local_night_date TEXT NOT NULL`
- `active INTEGER NOT NULL DEFAULT 1`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Constraints and indexes:

- `UNIQUE(property_id, raw_booking_block_id, local_night_date)`.
- Index `(property_id, local_night_date)`.
- Do not enforce capacity one against `named_stay_nights`; raw coverage and named stays intentionally overlap.
- Multiple active raw blocks may cover the same property night because upstream ICS can duplicate, merge, or split ranges. UI and provisional cleaning must coalesce raw coverage by property/date so duplicates do not create duplicate provisional events.

### Named Stay

Purpose:

- User-controlled business truth for stays, maintenance, personal use, analytics, Nuki, finance mapping, invoices, and final cleaning.
- Can cover part of one raw block, all of one raw block, the union of multiple raw blocks when Booking.com splits/merges/changes UIDs, or no raw block for external/personal/maintenance/direct bookings.

Proposed table: `named_stays`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `display_name TEXT NOT NULL`
- `stay_type TEXT NOT NULL CHECK(stay_type IN ('booking_com','external','maintenance','personal_use'))`
- `check_in_date TEXT NOT NULL`
- `check_out_date TEXT NOT NULL`
- `status TEXT NOT NULL CHECK(status IN ('active','cancelled','archived')) DEFAULT 'active'`
- `cleaning_required INTEGER NOT NULL`
- `cleaning_override_reason TEXT`
- `source_channel TEXT`
- `source_reference TEXT`
- `manual_revenue_cents INTEGER`
- `manual_revenue_currency TEXT`
- `manual_revenue_note TEXT`
- `review_status TEXT CHECK(review_status IN ('confirmed','needs_review')) DEFAULT 'confirmed'`
- `review_reason TEXT`
- `stay_outcome TEXT CHECK(stay_outcome IN ('cancelled_non_refundable','no_show'))`
- `stay_outcome_reason TEXT`
- `stay_outcome_marked_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- `stay_outcome_marked_at TEXT`
- `nuki_generation_status TEXT CHECK(nuki_generation_status IN ('not_applicable','pending','generated','error')) DEFAULT 'not_applicable'`
- `nuki_generation_error TEXT`
- `nuki_generation_updated_at TEXT`
- `created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- `updated_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Default cleaning rules:

- `booking_com`: `cleaning_required = 1` by default.
- `external`: `cleaning_required = 1` by default.
- `maintenance`: `cleaning_required = 0` by default.
- `personal_use`: `cleaning_required = 0` by default.

Constraints and indexes:

- Index `(property_id, check_in_date, check_out_date)`.
- Index `(property_id, status, check_in_date)`.
- Index `(property_id, stay_type)`.
- Prevent overlap through `named_stay_nights` unique index, not through row interval constraints.
- `manual_revenue_cents` must be non-negative when present.
- `manual_revenue_currency` is required when `manual_revenue_cents` is present and should match the property/reporting currency unless multi-currency reporting is explicitly added.
- `review_status = needs_review` rows remain visible and availability-blocking if active, but must be excluded from sold-night/revenue KPIs until confirmed or linked according to stay-type rules.
- Canonical finance linkage is `finance_bookings.named_stay_id`. Do not add a second finance FK on `named_stays` unless a later ADR introduces a deliberate one-to-many/primary-booking model.

Status semantics:

- `active`: creates active `named_stay_nights`, blocks availability, may create final cleaning, may generate Nuki for eligible stay types, and may count in analytics according to stay type and revenue rules.
- `cancelled`: keeps the stay record and links for audit, deactivates `named_stay_nights`, does not block availability, revokes or prevents future Nuki access, removes future PMS-owned final cleaning unless explicitly retained by a manual cleaning workflow, and reports any retained finance money in cancellation/no-show revenue buckets rather than sold-night ADR/RevPAR.
- `archived`: soft-delete/removal state. It deactivates nights, hides from default calendar views, revokes future Nuki access, keeps audit/finance/invoice links, and is reversible by an admin/support workflow if needed.
- Hard delete is allowed only for never-integrated draft/test rows with no finance, invoice, Nuki, cleaning, message, or audit references.

Proposed table: `named_stay_nights`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `named_stay_id INTEGER NOT NULL REFERENCES named_stays(id) ON DELETE CASCADE`
- `local_night_date TEXT NOT NULL`
- `active INTEGER NOT NULL DEFAULT 1`
- `created_at TEXT NOT NULL`

Constraints and indexes:

- `UNIQUE(property_id, local_night_date) WHERE active = 1` to prevent overlapping active named stays.
- `UNIQUE(property_id, named_stay_id, local_night_date)`.
- Index `(named_stay_id)`.

Proposed table: `stay_source_links`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `named_stay_id INTEGER NOT NULL REFERENCES named_stays(id) ON DELETE CASCADE`
- `raw_booking_block_id INTEGER REFERENCES raw_booking_blocks(id) ON DELETE SET NULL`
- `source_type TEXT NOT NULL DEFAULT 'booking_ics'`
- `source_event_uid TEXT`
- `linked_check_in_date TEXT NOT NULL`
- `linked_check_out_date TEXT NOT NULL`
- `link_status TEXT NOT NULL CHECK(link_status IN ('active','source_deleted','conflict','manual_unlinked')) DEFAULT 'active'`
- `conflict_reason TEXT`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Ownership rules:

- Raw blocks are owned by sync.
- Named stays are owned by user/business workflows.
- Sync may mark source links as conflict or source_deleted, but must not mutate `named_stays.check_in_date`, `check_out_date`, `display_name`, `stay_type`, `cleaning_required`, Nuki state, or finance mapping automatically.
- Finance may suggest or link named stays but must not silently overwrite a user-created named stay.
- Google cleaning reconciliation owns only rows in `cleaning_calendar_events` that have PMS metadata or stored Google IDs.
- One named stay may have multiple source links when Booking.com splits, merges, or changes UIDs. Source-link validity is based on whether the union of active linked raw-block nights covers the named stay's linked date range.
- Auto-clearing `source_deleted` / `conflict` badges is allowed only when active raw coverage again covers the named stay range. Do not auto-resize, auto-merge, or auto-cancel the named stay.

### Non-Stay Availability Block

Purpose:

- Preserve true closure/off-market periods that reduce bookable availability but are not guest stays, maintenance stays, or personal-use stays.
- Avoid incorrectly converting legacy `closure_state = closed` rows into named stays.
- Keep analytics/gap calculations honest after named stays replace legacy occupancies.

Proposed table: `property_availability_blocks`

Columns:

- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE`
- `block_type TEXT NOT NULL CHECK(block_type IN ('closed','off_market'))`
- `start_date TEXT NOT NULL`
- `end_date TEXT NOT NULL`
- `reason TEXT`
- `source_occupancy_id INTEGER`
- `status TEXT NOT NULL CHECK(status IN ('active','archived')) DEFAULT 'active'`
- `created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- `updated_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`

Rules:

- Active availability blocks reduce available/bookable nights and affect gaps/availability, but never count as sold/occupied/revenue nights.
- They do not generate Nuki codes, guest messages, payouts, invoices, or final cleaning by default.
- Active availability blocks should not overlap active named stays for the same property/night unless an implementation explicitly supports a reviewed conflict state. Backfill must report such overlaps instead of silently choosing one.
- If a legacy closed row is actually maintenance or personal use, migrate only after explicit classification or review.
- If this table is intentionally deferred, legacy closed rows must remain in the analytics/calendar availability read model until an equivalent replacement exists.

### Compatibility Mapping

Add table: `occupancy_stay_migration_map`

Columns:

- `old_occupancy_id INTEGER NOT NULL`
- `property_id INTEGER NOT NULL`
- `raw_booking_block_id INTEGER`
- `named_stay_id INTEGER`
- `availability_block_id INTEGER`
- `migration_kind TEXT NOT NULL CHECK(migration_kind IN ('raw_block','named_stay','availability_block','closure','synthetic_finance','unmapped'))`
- `notes TEXT`
- `created_at TEXT NOT NULL`
- `UNIQUE(old_occupancy_id)`

Purpose:

- Preserve old IDs during staged migration.
- Backfill `named_stay_id` into Nuki, finance, invoices, cleaning without guessing repeatedly.
- Let APIs temporarily accept `occupancy_id` and resolve to `named_stay_id` while the frontend migrates.
- Preserve legacy closure/off-market mapping so availability does not change when analytics stop reading `occupancies`.

## Required Integration Changes

### API

Add new endpoints while preserving old ones temporarily:

- `GET /api/properties/{id}/booking-blocks?month=YYYY-MM`
- `GET /api/properties/{id}/stays?month=YYYY-MM`
- `GET /api/properties/{id}/availability-blocks?month=YYYY-MM`
- `GET /api/properties/{id}/occupancy-calendar?month=YYYY-MM` returning raw blocks, named stays, and non-stay availability blocks in one view model.
- `POST /api/properties/{id}/booking-blocks/{blockId}/promote` with `{display_name, check_in, check_out, stay_type, cleaning_required?}`.
- `POST /api/properties/{id}/stays` for external, maintenance, personal use, and manually confirmed Booking.com stay creation.
- `PATCH /api/properties/{id}/stays/{stayId}`.
- `PATCH /api/properties/{id}/stays/{stayId}/status` for `active`, `cancelled`, or `archived`. Do not expose hard delete as a normal user workflow.
- `POST /api/properties/{id}/availability-blocks` and `PATCH /api/properties/{id}/availability-blocks/{blockId}` for non-stay closure/off-market periods if legacy closure management remains needed.
- `POST /api/properties/{id}/stays/{stayId}/cleaning` for cleaning enable/disable.
- `POST /api/properties/{id}/stays/{stayId}/nuki/generate` or migrate existing Nuki endpoint to `stayId`.
- Finance mapping endpoints should accept `named_stay_id`; keep `occupancy_id` only as deprecated compatibility.
- All new stay/raw-block endpoints must be specified in `spec/openapi.yaml` before frontend work starts. Generated OpenAPI frontend types should be the source of truth where practical.
- OpenAPI coverage must include changed dashboard, messages, Nuki, payout, invoice, analytics, cleaning-calendar, and compatibility/deprecation DTOs before frontend migration, not only the new stay/raw-block endpoints.
- New stay operations use the existing Occupancy module permissions. No separate Stays permission is required for this migration.
- Audit entity types should be explicit: `named_stay`, `raw_booking_block`, `stay_source_link`, `property_availability_block`, `cleaning_event`, and compatibility references to `old_occupancy_id`.

### Background Sync

- `occupancy.Service.SyncProperty` should still parse and store `occupancy_raw_events`.
- Scheduled raw block sync remains controlled by global `OCCUPANCY_SYNC_INTERVAL_MINUTES`; do not introduce per-property sync intervals in this migration.
- Replace `ReconcileBookingICSSync` write target with `raw_booking_blocks` and `raw_booking_block_nights`.
- Existing `occupancies` writes should be disabled only after all consumers have moved.
- Sync should create/update/delete provisional cleaning only for affected raw block dates.
- Sync should detect conflict when a named stay linked to a raw block is no longer covered by any active raw block night. It should create/update `stay_source_links.link_status = conflict` and expose the conflict in UI.
- If a named stay loses raw-block coverage because Booking.com cancelled or removed the block, do not mutate the named stay. Mark the source link `source_deleted` or `conflict`, display the issue in the calendar, and allow the user to relink it if a replacement raw block exists.
- Calendar badge labels for this state may be `Source cancelled` or `Raw block missing`. The badge must clear automatically once the named stay is relinked to active raw block coverage or the raw block reappears and source coverage is valid again.

### UI

Occupancy calendar should render a combined model:

- Empty night: diagonal lines through the cell.
- Raw booking block night: raw badge, distinct neutral/amber style, no analytics count. If multiple raw blocks cover the same date, show one coalesced raw badge with a conflict/detail indicator rather than multiple provisional stay-like chips.
- Named stay: badge with stay name, stay type color, night count, cleaning status.
- Availability block: blocked/off-market badge distinct from maintenance/personal-use named stays, no guest/Nuki/finance affordances.
- Guest stays `booking_com` and `external`: green.
- Maintenance: red.
- Personal use: grey.
- Cleaning indicator: show generated, disabled, provisional, or error.

Promotion dialog changes:

- User clicks a raw-covered cell.
- Dialog defaults to clicked date -> next date for one-night stay.
- User must enter display name, check-in, check-out, stay type.
- Dialog validates range is within the raw block for Booking.com promotion.
- External, maintenance, and personal-use named stays may be created on empty nights or raw-block nights, but they still must not overlap another active named stay.
- Dialog shows leftover raw dates after promotion.
- Calendar cells must show Nuki generation failure as a small badge on the relevant named stay when synchronous Nuki generation fails.
- Dashboard upcoming stays and next-check-in KPIs must read named stays only; dashboard sync-status widgets may continue reading raw ICS sync/source state.
- Remove export-token/n8n guidance UI when public occupancy export is deprecated or removed.

### Google Calendar Reconciliation

Required new behavior:

- Reconcile only an affected date range.
- Build desired state for each affected date from raw blocks and named stays.
- Query current PMS local records for affected dates.
- Query Google Calendar events for affected dates.
- Match PMS-owned events in this order:
- Stored `google_event_id`.
- Google private extended properties: `pms_property_id`, `pms_cleaning_event_id`, future `pms_named_stay_id`, `pms_raw_booking_block_id`, `pms_cleaning_identity`.
- Wording fallback for legacy PMS events, limited to configured cleaning calendar and same date.
- Create, patch, or delete only when desired state differs from current state.
- Never modify non-PMS events.

Required client changes:

- Extend `CalendarClient` with `ListEvents(ctx, calendarID, timeMin, timeMax) ([]GoogleCalendarEvent, error)`.
- Include event metadata, summary, start, end, status, and IDs in `GoogleCalendarEvent`.
- Add deterministic desired-state hash to local `cleaning_calendar_events`, for example `desired_hash TEXT`, so no-op reconciles skip Google patch.

Recommended table changes:

- Add `named_stay_id INTEGER REFERENCES named_stays(id) ON DELETE CASCADE`.
- Add `raw_booking_block_id INTEGER REFERENCES raw_booking_blocks(id) ON DELETE CASCADE`.
- Add `cleaning_identity TEXT` unique per desired cleaning event.
- Add `desired_hash TEXT`.
- Add `last_google_seen_at TEXT`.
- Keep `occupancy_id` nullable for legacy until cutover.

Desired cleaning identities:

- Provisional raw coverage per checkout date: `raw-provisional:{propertyID}:{checkoutDate}`.
- Final named stay checkout: `stay:{propertyID}:{namedStayID}:{checkoutDate}`.

Desired cleaning rules:

- One or more active raw block nights with no named stay covering that night -> exactly one provisional `Upratovanie` checkout on night + 1 for that property/date.
- For raw block `2026-07-09 -> 2026-07-12`, provisional placeholders are on `2026-07-10`, `2026-07-11`, and `2026-07-12`.
- Named stay `booking_com` or `external` with `cleaning_required = 1` -> one final checkout event on check-out date.
- Named stay `booking_com` or `external` with `cleaning_required = 0` -> remove matching provisional/final PMS event in affected range.
- Named stay `maintenance` or `personal_use` with `cleaning_required = 0` -> no final cleaning event.
- Named stay `maintenance` or `personal_use` with `cleaning_required = 1` -> one final checkout event.
- Full promotion of a multi-night raw block to one named stay removes all intermediate provisional placeholders and keeps or converts only the final checkout event.
- Partial promotion removes provisional placeholders covered by the named stay's internal nights but keeps provisional placeholders for unpromoted leftover raw checkout-placeholder dates.
- Provisional event title is exactly `Upratovanie`. Final named-stay cleaning uses configured final cleaning labels.
- Affected-date reconciliation must include the union of old and new checkout-placeholder dates when a raw block or named stay is moved, shrunk, expanded, deleted, archived, cancelled, or has cleaning toggled. Otherwise stale Google events outside the new range can survive.

### Nuki

- Add `nuki_access_codes.named_stay_id` nullable initially.
- Add `nuki_guest_daily_entries.named_stay_id` nullable initially and backfill through `nuki_access_codes` / `occupancy_stay_migration_map`.
- Backfill from `occupancy_stay_migration_map`.
- Change Nuki sync selection to `named_stays` where `status = active`, `review_status = confirmed`, and `stay_type IN ('booking_com','external')`.
- Raw blocks should never appear in Nuki UI.
- Keep revocation compatibility for old `occupancy_id` until all existing active codes have `named_stay_id`.
- Named stay creation must synchronously trigger Nuki generation for Nuki-eligible stay types. If generation fails, the named stay remains created and the failure is stored/displayed as a calendar badge.
- Existing generated PINs and external Nuki IDs must be preserved during migration and relinking.
- Guest check-in heatmap and Nuki guest daily analytics must migrate from `occupancy_id` to `named_stay_id`, preserving historical revoked-code attribution.
- Nuki UI must not implicitly create/promote stays. If it edits a name, decide in implementation whether it updates `named_stays.display_name` through stay permissions or stores a separate Nuki code-label override; it must not mutate legacy `occupancies.guest_display_name` as business truth.

### Analytics

- Move primary analytics source from `occupancies` to `named_stay_nights` joined to `named_stays`.
- Count active named stays by stay type:
- `booking_com` counts as sold/occupied nights.
- `external` counts as sold/occupied nights only when linked finance data exists or `manual_revenue_cents` is entered. External stays without revenue remain visible in the calendar and should be surfaced as needing revenue input before they affect sold-night/revenue KPIs.
- `maintenance` reduces available/bookable nights and does not count as sold/occupied revenue nights.
- `personal_use` reduces available/bookable nights and does not count as sold/occupied revenue nights.
- Raw blocks must not count as sold or revenue nights.
- Active external stays without revenue still reduce available/bookable nights because the property is not bookable, but they must not increase sold nights, occupancy-rate numerator, ADR, RevPAR revenue numerator, or returning-guest revenue metrics until revenue exists.
- Active `review_status = needs_review` named stays reduce availability if their nights are active, but must not count as sold/revenue until confirmed.
- `property_availability_blocks` reduce available/bookable nights and gap availability, but never count as sold/occupied/revenue nights.
- Replace finance matching by `finance_bookings.named_stay_id`.
- Dashboard, message generation, cleaning daily logs/salary, and Nuki guest daily entries must move with the relevant named-stay cutover stages so no consumer keeps treating raw blocks as stays.

### Finance, Payouts, Invoices, Messages

- Add `finance_bookings.named_stay_id` and backfill.
- Add `invoices.named_stay_id` and backfill.
- Update payout matching to use named stays first.
- If statement/payout contains stay dates but no named stay exists, leave it unmatched by default and show a UI action to create/link a named stay. Automatic imported named-stay creation is not part of this migration except for preserving existing legacy synthetic rows during backfill as review-required records.
- Update invoice candidates to list named stays.
- Update message generation to use named stays and Nuki code by named stay.
- Guest message generation should accept `stay_id` and show only message-eligible named stays in pickers.
- Cleaning-staff messages should derive from final cleaning-required named stays or desired cleaning events, not raw booking blocks.
- Booking.com finance/statement status should map into named stay review/outcome where a named stay is linked. Active/OK/modified reservations keep the named stay active. Cancelled reservations create a review action instead of directly setting `named_stays.status = cancelled`. Existing no-show and non-refundable cancellation override data maps to `named_stays.stay_outcome` when confirmed or already explicit.
- Final rule: finance/statement cancellations must be surfaced for user confirmation before PMS changes `named_stays.status`. Even exact linked matches should create a review action rather than automatically cancelling the named stay.
- Finance import must not silently create stay-like legacy `occupancies`. If it can safely match one named stay by reference or exact date/name, link it. Otherwise leave unmatched and show a create/link action.
- External named stays require linked finance data or a manually entered revenue amount before they count as sold/occupied revenue nights in analytics.
- Finance reset/import reset must clear or null finance-derived links on named stays without deleting named stays, preserve manual external revenue, and delete/update invoices through both finance booking and named-stay links so no orphaned invoice candidates remain.

## Old Data Migration Strategy

Run migration in additive phases. Do not drop or rewrite `occupancies` first.

Classification rules:

- `occupancies.representation_kind = unnamed_block` and source `booking_ics` -> `raw_booking_blocks`.
- `occupancies.representation_kind = legacy_generated_night` with no guest name -> raw block coverage or unmapped legacy row. Prefer mapping to raw block by `upstream_event_uid` and date.
- `occupancies.representation_kind = named_stay` -> `named_stays`.
- `occupancies.guest_display_name IS NOT NULL` and no representation kind -> `named_stays`, default `stay_type = booking_com` if upstream source is Booking ICS, otherwise `external` or `booking_com` based on source type.
- Existing `occupancies.source_type IN ('booking_payout','booking_statement')` rows are legacy synthetic finance rows. If they map exactly to one existing named-stay candidate, link finance to that stay. If no exact named stay exists, preserve them as `named_stays` with `stay_type = booking_com`, `source_channel = legacy_finance_import`, `review_status = needs_review`, and `review_reason = synthetic_finance_occupancy`, so production history is not lost but analytics can exclude them from sold-night/revenue KPIs until confirmed.
- `closure_state = external_sale` -> `named_stays.stay_type = external` if it represented a real external guest stay. This is a product-sensitive migration and should produce review rows if uncertain.
- `closure_state = closed` -> do not create named stay by default. Migrate clear true closures to `property_availability_blocks` or keep them in a legacy availability compatibility read. If business wants maintenance or personal use, user must classify it explicitly.
- `stay_outcome` and `cleaning_calendar_excluded` move to named stay where the old row maps to exactly one named stay. Existing `finance_booking_id` relationships move to `finance_bookings.named_stay_id`.
- Active `closure_state = closed` rows must remain availability-blocking after cutover. Analytics and gaps must not accidentally treat those nights as bookable merely because old `occupancies` are no longer the primary read model.

Conflict handling:

- If two old named-like occupancies overlap on the same property night, do not auto-merge. Insert both as `named_stays` with inactive nights for losers or mark migration conflict for manual review.
- If one finance booking points to a raw block but there is no exact named stay, leave `finance_bookings.named_stay_id` null and surface as unmatched.
- If one finance booking points to a review-required synthetic-finance named stay, keep it linked but exclude it from sold-night/revenue KPIs until the review confirms it is a real stay.
- If a cleaning event cannot be mapped to a named stay or raw block, keep `occupancy_id` and mark `migration_kind = unmapped` for manual review.

## Staged Migration Plan

### Stage 0 - ADRs And Acceptance Criteria

Write ADRs before schema work where future agents need consistent rules:

- Source of truth ADR: raw blocks vs named stays.
- Cleaning ownership ADR: provisional vs final events, PMS-owned Google matching, date-scoped reconciliation.
- Stay type semantics ADR: analytics and availability treatment for maintenance and personal use.
- Migration compatibility ADR: how long `occupancy_id` APIs remain supported, how export tokens are retired, and how legacy closures remain availability-blocking.

### Stage 1 - Add New Schema And Mapping

- Add tables `raw_booking_blocks`, `raw_booking_block_nights`, `named_stays`, `named_stay_nights`, `stay_source_links`, `property_availability_blocks`, `occupancy_stay_migration_map`.
- Add nullable `named_stay_id` / `raw_booking_block_id` to affected integration tables.
- Add `named_stay_id` to `nuki_guest_daily_entries` and any cleaning salary/log attribution tables that currently depend on occupancy-derived stay identity.
- Add schema fields needed for calendar badges: Nuki generation status/error and source-link conflict/source-deleted state.
- Do not remove old columns.

### Stage 2 - Backfill Read-Only New Model

- Backfill raw blocks and named stays from current `occupancies`.
- Backfill nights and source links.
- Backfill mapping table.
- Add diagnostic reports for unmapped and conflicting rows.
- No API behavior changes yet.

### Stage 3 - Dual-Write ICS Sync To Raw Blocks

- Keep current `occupancies` write path.
- Also upsert `raw_booking_blocks` and `raw_booking_block_nights` from parsed ICS events.
- Add sync-run counters for raw blocks inserted, updated, deleted, conflicts surfaced.
- Verify parity between raw block coverage and current aggregate unnamed block coverage.

### Stage 4 - Named Stay Service Layer

- Create store/service methods for named stays independent of `occupancies`.
- Implement promotion from raw block to named stay.
- Enforce non-overlap through `named_stay_nights`.
- Preserve leftover raw block nights by design.
- Until `occupancy_legacy_write_disabled` is enabled, named-stay create/edit/status changes must keep legacy stay-like `occupancies` in sync or block legacy consumers behind feature flags. The default safe path is dual-write derived legacy occupancy rows for active named stays so Nuki, finance, messages, dashboard, and analytics do not lose newly created stays before their cutovers.
- The new `named_stays` row is the source of truth during dual-write; legacy occupancy rows are derived compatibility records and must not be edited as independent business truth.
- Reconcile affected cleaning dates only.
- Trigger Nuki generation synchronously for eligible named stay types and store/display calendar badge state when generation fails.
- Allow external, maintenance, and personal-use named stays to be created on empty nights or raw-block nights, subject to hard no-overlap with active named stays.

### Stage 5 - Occupancy Calendar API And UI

- Update `spec/openapi.yaml` for raw-block, named-stay, and combined calendar endpoints before frontend implementation.
- Add combined occupancy calendar DTO with raw blocks, named stays, availability blocks, cleaning status, conflicts.
- Update `OccupancyView.vue` and `OccupancyCalendar.vue` to render raw vs named badges and empty diagonal cells.
- Update dashboard upcoming-stays/check-in widgets to named stays in the same frontend cutover or keep those widgets on legacy compatibility data until Stage 9.
- Add calendar badge state for synchronous Nuki generation failure and raw-source conflict/source-deleted warnings.
- Keep old list/sync tabs during transition.

### Stage 6 - Cleaning Reconciliation Rewrite

- Add date-scoped reconciliation service.
- Add Google `ListEvents` support and PMS-owned matching.
- Add desired hash no-op detection.
- Migrate local cleaning event ownership from `occupancy_id` to `named_stay_id` / `raw_booking_block_id`.
- Stop broad-window patching after date-scoped reconciliation is verified.

### Stage 7 - Nuki Cutover

- Backfill `nuki_access_codes.named_stay_id`.
- Backfill `nuki_guest_daily_entries.named_stay_id` and migrate guest check-in heatmap queries.
- Change Nuki queries and UI to named stays.
- Preserve `occupancy_id` for historical code display until safe removal.

### Stage 8 - Finance, Payout, Invoice Cutover

- Backfill `finance_bookings.named_stay_id` and invoice `named_stay_id`.
- Update payout matching and manual mapping UI to named stays.
- Update finance reset behavior for named-stay links and manual external revenue preservation.
- Stop creating synthetic `occupancies` from finance imports.
- Make unmatched payout/reservation rows link to named stays through explicit UI action or deterministic exact match only.
- Map Booking.com finance/statement cancellation and no-show semantics onto review actions / `named_stays.stay_outcome` without automatically cancelling user-created stays.
- Add UI/API for entering manual revenue on external named stays so they can count as sold/revenue nights.

### Stage 9 - Analytics And Messages Cutover

- Change analytics to `named_stays` and `named_stay_nights`.
- Update returning guest, demand, heatmap, gaps, ADR, RevPAR, and finance performance calculations.
- Update message generation to named stay identity.
- Move dashboard, cleaning salary/daily log consumers, and Nuki guest daily entries to named-stay semantics in the same cutover window.
- Update `spec/openapi.yaml` for changed dashboard/message/analytics response types.

### Stage 10 - Deprecate Old Occupancy-as-Stay APIs

- Mark old endpoints deprecated.
- Keep compatibility translation where needed.
- Deprecate or remove public occupancy export route, export-token storage, export-token UI, and related n8n/curl guidance after confirming no internal callers remain.
- Once all frontend and integrations use named stays, freeze `occupancies` as legacy compatibility or replace with a view.

### Stage 11 - Cleanup

- Drop no-longer-used write paths after a release cycle.
- Remove synthetic finance occupancy creation.
- Remove old cleaning exclusion fields from `occupancies` after all rows are migrated.
- Consider renaming UI from Occupancy to Calendar/Stays if product wants clearer language.

## Migration Tracking Table

| Stage | Change area | Business goal | Technical change | Affected files/modules | Database migration required | Old data migration required | Verification/tests | Risks/dependencies | Implementation status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | ADRs | Lock source-of-truth and stay-type rules | Added ADRs for raw vs named, cleaning ownership, stay type analytics, compatibility/export retirement, and finance import behavior; added readiness doc | `docs/adr/ADR-002-*` through `ADR-006-*`, `docs/pms-21-implementation-readiness.md` | No | No | ADR/readiness artifacts added; local audit artifact saved | Later cutovers still depend on resolving audit risks | Implemented |
| 1 | Schema | Create first-class objects | Added additive raw block, named stay, night, source-link, availability-block, mapping tables, and nullable integration compatibility columns | `backend/internal/migrate/000032_raw_booking_blocks_named_stays.*.sql`, `backend/internal/store/nuki.go` | Yes | No | Applied migration to temp copy of `data/pms.db`, ran FK check, `go test ./...` from `backend/` passes | Additive only; no legacy columns removed and no read/write cutovers enabled | Implemented |
| 2 | Backfill | Preserve existing production data | Added shared dry-run/apply classifier, explicit confirmation and review override, idempotent object creation/mapping, integration relinking, structured applied counts, and severe-conflict refusal | `docs/audits/PMS_21_local_data_audit_2026-07-13.md`, `backend/internal/store/pms21_migration.go`, `backend/cmd/pms21-migration` | No additional schema beyond Stage 1 | Yes, production run pending | Apply-twice, classification, conflict refusal, integration-link, and legacy preservation tests pass locally | Production audit still must resolve actual overlaps/unmapped rows before apply | Locally implemented; production audit/apply absent |
| 3 | ICS sync | Sync raw blocks without changing business truth | Added default-off `PMS21_RAW_BLOCKS_DUAL_WRITE` gate, raw-block/night writes inside the existing ICS reconciliation transaction, and raw-block sync counters | `backend/internal/occupancy/sync.go`, `backend/internal/store/occupancy_reconciliation.go`, `backend/internal/migrate/000033_raw_block_sync_counters.*.sql` | Yes, additive sync counters only | No | Unit tests for default-off safety, raw block/night upsert and shrink rebuild, source deletion; `go test ./...` from `backend/` passes; local verification report saved in `docs/audits/PMS_21_stage3_local_dual_write_verification_2026-07-13.md` | Gate must remain disabled in production until backfill/production audit verification passes | Implemented and locally verified behind default-off gate |
| 4 | Named stay service | User promotes raw coverage to business stay | Added first-class named stay create/update/status/promote store methods, active-night overlap checks, source links, Nuki generation badge state, and derived legacy occupancy compatibility rows | `backend/internal/store/named_stays.go`, `backend/internal/api/occupancy_named_stay_handlers.go`, `backend/internal/api/server.go`, `backend/internal/store/named_stays_test.go` | No additional schema beyond Stage 1 | No | Unit tests for partial promotion, overlap rejection, status lifecycle, raw leftovers, Nuki failure badge state, legacy compatibility mapping; `go test ./...` from `backend/` passes; local verification saved in `docs/audits/PMS_21_stage4_local_verification_2026-07-13.md` | Cleaning still uses broad-window best-effort reconciliation until Stage 6; Nuki still uses legacy occupancy compatibility until Stage 7 | Implemented and locally verified |
| 5 | Calendar API/UI | Show raw blocks, named stays, and availability blocks distinctly | Added OpenAPI contract, generated frontend OpenAPI types, combined calendar endpoint/read model, raw/named/availability Vue rendering, current cleaning status summaries, empty diagonal cells, source/Nuki/cleaning badges, raw-block promotion dialog, empty-night/manual stay creation UI, and availability-block create/edit endpoints/UI; dashboard widgets remain on legacy compatibility data until Stage 9 | `spec/openapi.yaml`, `frontend/src/api/types/generated.ts`, `backend/internal/store/stay_calendar.go`, `backend/internal/api/occupancy_calendar_v2_handlers.go`, `backend/internal/api/server.go`, `frontend/src/api/types/occupancy.ts`, `OccupancyView.vue`, `OccupancyView.spec.ts`, `OccupancyCalendar.vue`, `OccupancyCalendar.spec.ts` | No | No | Backend store tests for combined calendar model, cleaning status, availability-block overlap; frontend tests for combined calendar badges, empty-cell action emission, and Stage 5 endpoint loading; `go test ./...` from `backend/` passes; `npm run type-check`, `npm run test`, and `npm run build` from `frontend/` pass; local verification saved in `docs/audits/PMS_21_stage5_local_verification_2026-07-13.md` | Stage 6 still owns date-scoped/idempotent Google reconciliation; dashboard named-stay semantics intentionally deferred to Stage 9; production rollout still blocked pending audit/backfill approval | Implemented and locally verified |
| 6 | Cleaning | Stop noisy Google updates and finalize cleaning truth | Added date-scoped idempotent reconciliation, Google event list/match support, desired hashes, PMS 21 cleaning identities, and named/raw owner writes with legacy fallback | `cleaningcalendar/service.go`, `google_client.go`, `store/cleaning_calendar.go`, named-stay API cleaning triggers | No additional schema beyond Stage 1 cleaning columns | Ownership is populated on reconcile; production backfill remains blocked pending audit approval | Tests for no-op reconcile and PMS 21 raw/named cleaning ownership; `go test ./...` from `backend/` passes; local verification saved in `docs/audits/PMS_21_stage6_local_verification_2026-07-13.md` | Google API behavior and legacy wording fallback still need production validation before rollout | Implemented and locally verified |
| 7 | Nuki | Generate access only for named stays | Backfilled Nuki named-stay links, switched sync/list/generate/upcoming-stays UI and guest daily attribution to named stays, retained legacy occupancy compatibility for historical display/revocation | `backend/internal/migrate/000034_nuki_named_stay_cutover.*.sql`, `backend/internal/store/nuki.go`, `backend/internal/store/nuki_guest_logs.go`, `backend/internal/nuki/service.go`, `backend/internal/api/nuki_handlers.go`, `frontend/src/views/NukiView.vue`, `frontend/src/views/nuki/NukiUpcomingStays.vue`, `spec/openapi.yaml` | Yes | Yes | Store/service/API tests for named-stay Nuki selection, raw invisibility, revocation compatibility, guest daily named-stay attribution; `go test ./...`, `npm run type-check`, `npm run test`, and `npm run build` pass; local verification saved in `docs/audits/PMS_21_stage7_local_verification_2026-07-13.md` | Production active PIN preservation and unmapped Nuki rows require audit review before rollout | Implemented and locally verified |
| 8 | Finance/payouts/invoices | Map money and invoices to named stays | Backfilled finance/invoice named-stay links, switched import/rematch/manual payout mapping and invoice candidates to named stays, stopped synthetic finance occupancy creation from imports, changed cancellation handling to review-required instead of automatic named-stay cancellation, updated finance reset for named-stay invoice links, and added manual external revenue entry from payout UI | `backend/internal/migrate/000035_finance_invoice_named_stay_cutover.*.sql`, `backend/internal/store/finance_booking_payouts.go`, `backend/internal/store/invoices.go`, `backend/internal/store/finance_reset.go`, `backend/internal/api/finance_handlers.go`, `backend/internal/api/finance_imports_handlers.go`, `backend/internal/api/invoice_handlers.go`, `frontend/src/views/BookingPayoutsView.vue`, `frontend/src/views/InvoicesView.vue`, `spec/openapi.yaml` | Yes | Yes | API test updated for named-stay payout create/link; `go test ./...`, `npm run type-check`, `npm run test`, and `npm run build` pass; local verification saved in `docs/audits/PMS_21_stage8_local_verification_2026-07-13.md` | Production finance/invoice relinking and unmatched rows require audit review before rollout; Stage 9 analytics/messages/dashboard semantics still pending | Implemented and locally verified |
| 9 | Analytics/messages/dashboard | Count only named stays | Switched analytics, message generation/pickers, cleaning-staff messages, dashboard upcoming/check-in widgets, and named-stay night metrics to named-stay semantics; raw blocks excluded from analytics; external-without-revenue and review-required stays block availability without sold/revenue count; legacy fallback remains only for properties with no named-stay rows | `backend/internal/store/analytics*.go`, `backend/internal/store/messages.go`, `backend/internal/store/occupancy_nights.go`, `backend/internal/api/message_handlers.go`, `backend/internal/api/server.go`, `frontend/src/views/MessagesView.vue`, dashboard/message frontend types, `spec/openapi.yaml` | No | Yes via prior stages | Added Stage 9 analytics raw/external/manual-revenue coverage; `go test ./...`, `npm run type-check`, `npm run test`, and `npm run build` pass; local verification saved in `docs/audits/PMS_21_stage9_local_verification_2026-07-14.md` | Production analytics/messages/dashboard cutover requires approved PMS 21 backfill/audit; public occupancy export remains Stage 10 | Implemented and locally verified |
| 10 | Compatibility | Remove confusing old API dependence | Added deprecation headers on legacy occupancy-as-stay endpoints, deprecated public export/token endpoints in OpenAPI/backend, added `PMS21_OCCUPANCY_EXPORT_DISABLED` kill switch, removed export-token/n8n/curl UI and frontend token-management calls | `backend/internal/api/server.go`, `backend/internal/api/occupancy_handlers.go`, `backend/internal/api/deprecation.go`, `backend/internal/config/config.go`, `backend/cmd/server/main.go`, `frontend/src/views/OccupancyView.vue`, `frontend/src/views/occupancy/OccupancySyncPanel.vue`, `spec/openapi.yaml` | No, token table retained for compatibility/rollback | No | API tests for export deprecation/disablement; frontend test that sync tab does not call token APIs or show export/n8n UI; `go test ./...`, `npm run type-check`, `npm run test`, and `npm run build` pass; local verification saved in `docs/audits/PMS_21_stage10_local_verification_2026-07-14.md` | Hard removal and token-table drop must wait for production caller audit/release-cycle cleanup | Implemented and locally verified |
| 11 | Cleanup | Reduce maintenance burden | Added default-off cleanup gate for stopping new legacy occupancy compatibility writes without dropping tables/columns/routes; hard cleanup remains blocked | `backend/internal/config/config.go`, `backend/cmd/server/main.go`, `backend/internal/store/store.go`, `backend/internal/store/finance_booking_payouts.go`, `backend/internal/store/named_stays.go`, store tests | No | No | `go test ./internal/store` and `go test ./...` from `backend/` pass; local verification saved in `docs/audits/PMS_21_stage11_local_verification_2026-07-14.md` | Must wait until production data verified, all cutovers have run for a release cycle, and cleanup gate is explicitly approved before destructive removal | Non-destructive gate implemented and locally verified; destructive cleanup blocked |

## Recommended Tests

Backend unit tests:

- ICS all-day merged block parses as check-in inclusive, check-out exclusive.
- Raw block upsert preserves leftover raw nights after partial promotion.
- Overlapping or duplicate raw blocks coalesce to one provisional cleaning per property/date.
- Named stay overlap rejection for same property nights.
- Named stay can span one or more nights inside a raw block.
- Named stay status transitions deactivate/reactivate nights correctly and trigger the required cleaning/Nuki side effects.
- Named stay of each type gets correct default `cleaning_required`.
- Sync deleting or shrinking raw blocks does not mutate named stay dates or names.
- Sync conflict creates source-link conflict when named stay no longer covered by raw source.
- Source-link conflict clears when active raw coverage again covers the linked named stay range, without resizing the named stay.
- Cleaning date-scoped reconciliation does not patch unchanged Google events.
- Cleaning only deletes PMS-owned events.
- Cleaning reconciliation removes stale events from old checkout-placeholder dates when a raw block or named stay shrinks, moves, is archived/cancelled, or disables cleaning.
- Nuki selection excludes raw blocks and includes only eligible named stays.
- Nuki guest daily entries backfill and query by `named_stay_id` while preserving historical attribution.
- Finance matching links to named stay, not raw block.
- Finance reset clears finance-derived named-stay links without deleting named stays or manual external revenue.
- Analytics excludes raw blocks and includes named stay nights.
- Availability blocks reduce bookable nights and gap availability but do not count as sold/revenue nights.
- Maintenance and personal-use stays reduce available/bookable nights and do not count as sold nights.
- External stays without linked finance or manual revenue do not count as sold/revenue nights and are surfaced as needing revenue input.
- Review-required legacy synthetic finance stays do not count as sold/revenue until confirmed.
- Booking.com finance cancellation creates a user-confirmation action rather than automatically cancelling a named stay.
- Scheduled sync uses global `OCCUPANCY_SYNC_INTERVAL_MINUTES` and respects per-property leases.
- Manual sync and scheduled sync cannot mutate the same property concurrently.
- Disabled occupancy source creates no raw block mutations.
- Partial ICS parse failure remains no-mutation.

Frontend tests:

- Calendar cell shows raw block badge and named stay badge separately.
- Calendar cell shows availability block badge separately from maintenance/personal-use named stays.
- Empty night uses diagonal visual class.
- Named stay colors match stay type.
- Promotion dialog defaults to clicked night and validates check-out exclusive range.
- Cleaning indicator updates after toggling cleaning required.
- Nuki generation failure appears as a small badge on the relevant calendar cell.
- Source-deleted/conflict raw block link state appears in the calendar.
- Dashboard upcoming-stays and check-in KPI widgets exclude raw blocks.
- Nuki view does not show raw blocks.
- Payout mapping UI lists named stays.
- Message stay picker lists message-eligible named stays and sends `stay_id`.
- Export-token UI and n8n/curl guidance disappear when export removal is enabled.

Migration tests:

- Backfill from one raw Booking.com aggregate only.
- Backfill from aggregate plus partial named stay.
- Backfill from legacy generated night rows.
- Backfill from finance synthetic occupancy.
- Backfill from finance synthetic occupancy marks review-required when no exact named stay exists.
- Backfill from legacy closed rows preserves availability blocking through `property_availability_blocks` or compatibility reads.
- Backfill preserves existing Google event IDs.
- Backfill preserves existing generated Nuki code links.
- Backfill preserves Nuki guest daily entry attribution.
- Backfill reports ambiguous overlapping old rows without destructive changes.
- Backfill preserves existing finance cancellation/outcome override semantics when mapping to named stays.

## Production Data Audit

Before implementing Stage 1, run a read-only production data audit. The audit output should be saved as an implementation artifact and reviewed before schema or backfill code is applied to production.

Required audit counts:

- `occupancies` by `source_type`, `status`, `representation_kind`, `closure_state`, `stay_outcome`, and `superseded_at IS NOT NULL`.
- Active `occupancy_nights` count by property and count of duplicate/overlapping active nights.
- Booking.com raw aggregate rows by upstream UID and date range.
- Named-like occupancy rows: `representation_kind = named_stay` or non-empty `guest_display_name`.
- Legacy generated night rows and manual split rows.
- Active `closure_state = closed` rows by property/date range and whether they appear to be true closure, maintenance, or personal use.
- Rows with `finance_booking_id` and `finance_bookings.occupancy_id`.
- `finance_bookings` unmatched count and rows whose date range matches only a raw block.
- `invoices` with `occupancy_id` and/or finance booking links.
- `nuki_access_codes` by status, with active future codes and generated PINs.
- `nuki_guest_daily_entries` by `occupancy_id`, day range, and whether the occupancy maps to exactly one named stay.
- `cleaning_calendar_events` by `cleaning_kind`, status, `google_event_id IS NULL`, and legacy rows without upstream identity.
- Google cleaning events that exist in local DB but no longer exist in Google, if Google listing is available.
- Dashboard/message/export consumers that currently read occupancy IDs.
- Public occupancy export tokens and frontend export-token UI usage, including n8n/curl guidance references.

Required audit risk report:

- Candidate raw blocks to create.
- Candidate named stays to create.
- Overlapping named-stay candidates.
- Ambiguous `external_sale` rows requiring manual classification.
- `closed` rows requiring product decision: closure/block vs maintenance named stay.
- Legacy closure/off-market nights that would become bookable if old occupancies were removed without availability-block replacement.
- Nuki codes that cannot be mapped to exactly one named stay candidate.
- Nuki guest daily entries that cannot be mapped to exactly one named stay candidate.
- Cleaning events that cannot be mapped to exactly one raw block or named stay candidate.
- Finance/invoice rows that cannot be mapped to exactly one named stay candidate.
- Finance synthetic rows that would become review-required named stays.
- Duplicate/overlapping raw ICS blocks that would coalesce for provisional cleaning.
- Properties with no active ICS URL or disabled occupancy source.

No implementation stage should proceed to destructive cleanup until this report is clean or every reported ambiguity has a documented resolution.

## Migration Dry-Run Tooling

Add a dry-run command or admin-only endpoint before old data backfill. It must not write data unless an explicit apply mode is used.

Dry-run output should include:

- Number of `raw_booking_blocks`, `raw_booking_block_nights`, `named_stays`, `named_stay_nights`, and `stay_source_links` that would be created.
- Number of records that would receive `named_stay_id`: Nuki codes, cleaning events, finance bookings, invoices.
- Number of conflicts, unmapped rows, overlap violations, and review-required rows.
- Sample rows for each conflict class, capped to a safe limit.
- Whether the backfill is idempotent compared with any prior dry run.

Recommended implementation shape:

- Add a store-level dry-run planner that returns a structured report.
- Reuse the same planner in apply mode so dry-run and apply cannot diverge.
- Add tests proving that running apply twice is idempotent.

## Feature Flags And Rollout Gates

Owner decision 2026-07-18: do not add the wider runtime gate set. The PMS 21 cutover model is a version/deployment switch after safe migrations and verification, not long-term dual-mode operation of every integration. Documentation must not claim flags exist when code does not expose them.

Implemented runtime safety flags:

- `PMS21_RAW_BLOCKS_DUAL_WRITE`: ICS sync writes both legacy `occupancies` and new raw block tables. Default off.
- `PMS21_OCCUPANCY_EXPORT_DISABLED`: disables deprecated public occupancy export compatibility. Default off.
- `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED`: stops new stay-like legacy occupancy compatibility writes after all named-stay-primary dependencies are safe. Default off.

Collapsed cutovers handled by deploying or rolling back the PMS 21 version:

- Named-stay read model, calendar v2, date-scoped cleaning, Nuki named-stay reads, finance named-stay mapping, analytics named-stay reads, availability-block reads, and message named-stay reads do not have separate runtime flags.
- Rollback for these areas means redeploying the prior binary/frontend while preserving additive tables and columns for inspection or retry.
- Runtime flags are not required for each collapsed cutover because the owner selected a fast version cutover after production audit and Stage 2 verification.

Rollout rules:

- Do not deploy the PMS 21 version that assumes named-stay truth until production audit and Stage 2 apply/backfill verification pass.
- Do not enable `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` until Nuki can generate/list/update/revoke for named stays without requiring newly written legacy occupancy rows.
- Do not enable `PMS21_OCCUPANCY_EXPORT_DISABLED` without explicit production approval for export compatibility removal.
- Stop if the production data audit reports unresolved unmapped active records for any integration required by the cutover.
- Runtime flag state and deployed version should be recorded in the cutover artifact so production behavior can be explained later.

## Rollback Strategy

Each stage needs an explicit rollback expectation.

Additive stages:

- Schema additions can be left in place if rollback is needed.
- Dual-write can be disabled by flag while retaining new tables for inspection.
- Backfilled data can be marked stale or regenerated from legacy data; avoid deleting it immediately.

Cutover stages:

- Cleaning rollback must preserve `google_event_id`, local cleaning event IDs, desired hashes, and removed-event history.
- Nuki rollback must preserve existing PINs, `external_nuki_id`, valid windows, and revocation history. It must not regenerate PINs merely because identity moved from `occupancy_id` to `named_stay_id`.
- Finance rollback must preserve `finance_bookings.occupancy_id`, `finance_bookings.named_stay_id`, invoice links, and transaction links.
- Analytics rollback can switch read queries back to legacy tables, but the discrepancy should be visible in metrics or logs.
- Frontend rollback should continue to function against legacy endpoints until old APIs are intentionally removed.
- Export removal rollback is possible only until the route/token table/UI are deleted; before deletion, disabling `occupancy_export_disabled` should restore legacy export behavior.
- Availability-block rollback must not make legacy closed nights bookable. Keep old closure rows or compatibility reads until replacement behavior is verified.

Cleanup stage:

- No old columns or legacy write paths should be removed until at least one production release cycle has run with all cutover flags enabled and no unresolved migration conflicts.

## Source-Of-Truth Invariants

Implementation agents must preserve these invariants:

- Raw booking blocks are sync-owned.
- Named stays are business-owned.
- Booking.com ICS sync may create, update, or mark raw blocks deleted, but must not delete, resize, rename, or reclassify named stays.
- Raw booking blocks never generate Nuki access codes.
- Raw booking blocks never count in analytics or payout/invoice truth.
- Named stays are the source of truth for analytics, Nuki, payout mapping, invoices, messages, and final cleaning state.
- Only active, confirmed named stays can count as sold/revenue nights, and only according to stay-type/revenue rules.
- Cancelled or archived named stays must not leave active named-stay nights behind.
- Legacy closure/off-market nights must remain unavailable/bookability-reducing until replaced by verified availability blocks.
- Manual external revenue belongs to named stays and must survive finance import resets.
- Provisional cleaning events are temporary operational hints; final cleaning state comes from named stays.
- Google Calendar reconciliation may only create, update, or delete PMS-owned events.
- PMS-owned Google events are identified by stored Google event ID or metadata first; wording/date fallback is only for legacy migration.
- Date ranges use check-in inclusive and check-out exclusive semantics everywhere.
- Property-local ISO dates are the business truth for raw blocks, named stays, and nights.
- UTC timestamps are used for audit/sync/event instants only.

## Concurrency And Idempotency

The implementation must handle concurrent workflows safely:

- ICS sync running while a user promotes or edits a named stay.
- Cleaning reconciliation running while a named stay is created, edited, deleted, or has cleaning toggled.
- Finance import/rematch running while a named stay date range is edited.
- Nuki generation running while named stay dates, type, or display name change.
- Manual sync and scheduled sync for the same property running close together.

Required safeguards:

- Use property-scoped leases for sync/reconciliation jobs where current code already has them, and add similarly scoped protection for new date-scoped cleaning reconciliation if needed.
- Named stay creation/edit and `named_stay_nights` writes must be transactional.
- Overlap checks must happen in the same transaction as night writes.
- Named-stay status changes must update nights, cleaning side effects, Nuki revocation/generation state, and compatibility rows transactionally where local DB state is involved. External API calls may run after commit but must be idempotent and retryable.
- Backfill apply must be idempotent and resumable.
- Google Calendar operations must be idempotent by cleaning identity and desired hash.
- Nuki generation must preserve existing generated PIN and external Nuki ID when the named stay identity is a migration of an old occupancy.
- Finance reset/rematch must be transactional for finance links, invoice links, and named-stay finance cache fields.

## Timezone And Date Policy

The new model should be date-first.

Rules:

- `raw_booking_blocks.check_in_date`, `raw_booking_blocks.check_out_date`, `named_stays.check_in_date`, and `named_stays.check_out_date` store property-local ISO dates, not UTC timestamps.
- Night tables store property-local `YYYY-MM-DD` night dates.
- ICS all-day dates should be interpreted as date values and mapped to property-local date ranges without shifting days.
- Nuki valid-from and valid-until are computed from named stay dates plus property profile check-in/check-out times in the property timezone, then stored/sent as UTC instants.
- Google cleaning event start/end times are computed from checkout date plus property profile cleaning settings in the property timezone, then stored/sent as UTC instants.
- Analytics groups by property-local dates/months.
- Sync/audit fields such as `last_synced_at`, `created_at`, `updated_at`, `source_dtstamp`, and Google/Nuki API timestamps remain UTC instants.

Tests must include properties outside UTC and DST boundary dates.

## External API And Export Compatibility

Public occupancy export should be dropped as a feature. Google Calendar events are the supported external calendar integration path.

Recommended approach:

- Keep current export behavior unchanged during early additive stages to avoid mixing feature removal with schema introduction.
- Do not add export v2.
- Before removing the route, confirm no frontend or internal automation still calls the legacy export endpoint.
- Deprecate or remove the legacy export route, `occupancy_api_tokens` storage, token-management endpoints, export-token UI, and n8n/curl guidance during the compatibility/cleanup stages.
- Update `spec/openapi.yaml` to remove or mark the legacy export endpoint and token-management endpoints deprecated when the backend route changes.

## Operational Observability

Add metrics, logs, and sync counters that make migration behavior auditable.

Recommended counters:

- Raw blocks inserted, updated, unchanged, deleted from source.
- Raw block nights active/inactive.
- Named stays created, updated, archived, and migrated.
- Named stay overlap rejections.
- Source-link conflicts created, resolved, acknowledged.
- Provisional cleaning events created, updated, removed, skipped as no-op.
- Final cleaning events created, updated, removed, skipped as no-op.
- Google events matched by stored ID, metadata, wording fallback, and unmatched.
- Google patch calls avoided by desired hash.
- Nuki codes generated, preserved, relinked, revoked, failed.
- Finance rows matched to named stay, unmatched, ambiguous.
- Finance synthetic rows migrated as review-required named stays.
- Availability-block nights active and migrated from legacy closures.
- Analytics read model in use: legacy vs named stays.
- Public export route/token UI enabled vs disabled.

Recommended logs/audit events:

- Raw block source deletion.
- Named stay promotion from raw block.
- Named stay conflict detected/resolved.
- Cleaning event ownership migration.
- Nuki code relinked from old occupancy to named stay.
- Finance booking mapped/remapped to named stay.
- Finance reset cleared named-stay finance links.
- Public export token created/deprecated/removed.
- Legacy closure migrated to availability block.

## Implementation Readiness Checklist

Do not start Stage 1 implementation until these items are complete or explicitly waived:

- ADR decisions are drafted for source of truth, cleaning ownership, stay type analytics, compatibility, and finance import behavior.
- Production data audit has been run and reviewed.
- Migration dry-run report format is defined.
- Feature flags/rollout gates are named and default behavior is defined.
- Rollback expectations are documented for additive, cutover, and cleanup stages.
- Source-of-truth invariants are agreed and copied into implementation tasks.
- Concurrency/idempotency safeguards are included in implementation tasks.
- Timezone/date policy is confirmed.
- Public export removal strategy is confirmed.
- `spec/openapi.yaml` has been updated for new raw-block, named-stay, availability-block, and calendar endpoints before frontend implementation.
- `spec/openapi.yaml` coverage is updated for changed dashboard, messages, Nuki, payout, invoice, analytics, cleaning-calendar, and compatibility/deprecation DTOs before frontend migration.
- Observability counters/logs are defined.
- Acceptance tests include raw block exclusion from analytics, named stay Nuki eligibility, cleaning no-op reconciliation, finance mapping, and old data backfill.
- No additional named-stay PII redaction/retention work is required for this migration.

## Risks And Edge Cases

- Booking.com may change UID for the same blocked range. Use date coverage and DTSTAMP for diagnostics, but do not auto-merge named stays without user review.
- A raw block can shrink below a named stay. Surface conflict; do not delete or resize the named stay.
- A raw block can merge with another raw block. Preserve named stays and create/update raw coverage accordingly.
- Duplicate or overlapping raw ICS blocks can occur. They must not create duplicate provisional cleaning events or duplicate sold/occupancy metrics.
- A named stay can link to multiple raw blocks after Booking.com UID churn/split/merge. Source-link state must be coverage-based, not single-UID based.
- Same-day check-out/check-in is common. Cleaning reconciliation must handle final event on checkout date and same-day arrival labels.
- Finance data can reveal exact stays before the user promotes raw blocks. Safer behavior is suggestion/review, not silent mutation.
- Finance reset can remove imported booking rows after named stays exist. Reset must not delete named stays or manual revenue.
- Existing Nuki codes must not be regenerated with new PINs during ID migration.
- Legacy Google events without metadata require conservative wording/date fallback. Never delete non-PMS events.
- External stays without linked finance or manual revenue must be easy to find because they remain excluded from sold-night/revenue KPIs until revenue is entered.

## Recommended Unification And Refactoring

Before or during migration:

- Introduce a small stay-domain package or store file set, for example `store/named_stays.go`, `store/raw_booking_blocks.go`, `store/stay_calendar.go`. Do not continue adding new behavior to the already overloaded `occupancy.go`.
- Define canonical date helpers for check-in inclusive/check-out exclusive ranges and reuse them in sync, UI DTOs, analytics, cleaning, Nuki, and finance.
- Replace `guest_display_name` as eligibility signal with explicit `named_stays.stay_type` and `named_stays.status`.
- Replace `cleaning_calendar_excluded` with `named_stays.cleaning_required` plus audit fields.
- Keep integration services (`occupancy`, `cleaningcalendar`, `nuki`) as orchestration boundaries, but move business ownership rules into named stay/raw block store methods.

After migration:

- Rename public API concepts from occupancy IDs to stay IDs where they represent business stays.
- Convert old `occupancies` table into a legacy compatibility table or remove after a deliberate release cycle.
- Remove finance synthetic occupancy creation entirely.
- Consolidate closure/external sale concepts: external sale becomes `named_stays.stay_type = external`; true closure/off-market becomes `property_availability_blocks` or remains in a verified legacy availability compatibility read until fully migrated.

## Remaining Concerns Before Implementation

The original open questions have been folded into the requirements above. The following points remain important for implementation planning:

- Finance cancellation/status mapping needs careful implementation. Active or modified Booking.com rows should keep linked named stays active. Cancellation rows must create a user-confirmation action before changing named stay status. No-show and non-refundable cancellation data should map to `named_stays.stay_outcome` when confirmed or already explicitly marked.
- Raw block lineage does not need a dedicated lineage table in the first implementation. The concrete problem is auditability when Booking.com merges, splits, or changes UIDs. Preserve `occupancy_raw_events`, raw block sync history, and source-link conflict records. Add explicit lineage only if production data shows UID churn that cannot be explained from snapshots and source links.
- Scheduler/manual sync tests should be defined by engineering. At minimum, test scheduled sync, manual sync, disabled source behavior, partial parse no-mutation, per-property sync lease behavior, and date-scoped cleaning reconcile no-op behavior.
- Public occupancy export removal should be planned separately from the additive schema stages so migration risk stays contained.

## Product Question Status

All currently known product questions are answered. New questions should be captured as implementation issues or ADR updates rather than inline answer notes.

## Suggested ADRs

- ADR: Raw booking blocks are sync-owned and named stays are business-owned.
- ADR: Cleaning event ownership and date-scoped reconciliation strategy.
- ADR: Stay type reporting semantics for booking.com, external, maintenance, and personal use.
- ADR: Compatibility window for old `occupancy_id` APIs, migration map usage, and public export removal.
- ADR: Finance import behavior when statement data references a stay that is not yet named.

## Acceptance Criteria For The Full Migration

- Raw Booking.com ICS blocks remain visible in the occupancy calendar but do not count in analytics.
- A user can promote any subrange of a raw block to a named stay and leftover raw dates remain visible.
- Named stays have required type and cleaning-required state.
- True closure/off-market periods remain availability-blocking through availability blocks or verified compatibility reads.
- Maintenance and personal-use named stays reduce available/bookable nights and do not count as sold nights.
- External named stays count as sold only after linked finance data or manual revenue is entered.
- Review-required legacy synthetic finance stays do not count as sold/revenue until confirmed.
- Nuki only sees eligible named stays.
- Nuki guest daily entries and heatmaps preserve historical attribution after moving to named stays.
- Finance bookings and invoices map to named stays.
- Booking.com finance cancellations surface for user confirmation before changing named stay status.
- Duplicate/overlapping raw blocks do not create duplicate provisional cleaning events.
- Google cleaning reconciliation modifies only PMS-owned events and only for affected dates.
- Existing Nuki codes, finance mappings, invoices, and Google event IDs survive migration.
- Public occupancy export and export-token UI are removed or deprecated without introducing export v2.
- Future ICS syncs do not delete or mutate named stays. Conflicts are surfaced for review.
