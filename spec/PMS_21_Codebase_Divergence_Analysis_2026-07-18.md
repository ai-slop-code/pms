# PMS 21 Codebase Divergence Analysis - 2026-07-18

Status: Draft strict audit against `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md` and current workspace code.

Purpose: capture concrete divergences, rollout blockers, and spec-tightening questions before using this as input for a follow-up analyst pass.

Scope reviewed:

- Main PMS 21 plan: `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`.
- Stage audit/readiness artifacts under `docs/audits/` and `docs/pms-21-implementation-readiness.md`.
- Backend migrations, store, services, and API registration under `backend/internal`.
- Frontend PMS 21 surfaces under `frontend/src/views`, `frontend/src/api/types`, and `spec/openapi.yaml`.

Important audit constraint:

- This review was performed against a dirty workspace where the PMS 21 implementation files are modified/untracked. Treat this document as an audit of the current local codebase state, not necessarily committed `main`.

## Executive Verdict

The current implementation has the broad PMS 21 shape: first-class raw booking blocks, named stays, night tables, source links, availability blocks, combined calendar endpoints, date-scoped cleaning reconciliation, named-stay-aware finance/analytics/messages paths, API deprecation headers, and a default-off legacy-write cleanup gate.

However, it is not strictly complete against the PMS 21 plan. The highest-risk gaps are:

- Stage 2 apply/backfill is still absent, and no production audit artifact was found.
- Only three PMS 21 rollout gates are implemented in config, while the plan/readiness docs list many independent cutover gates.
- Nuki still requires a legacy `occupancies` row and non-null `nuki_access_codes.occupancy_id` for new named-stay code generation.
- Raw-source conflict/source-deleted status for `stay_source_links` is exposed by the calendar model but does not appear to be maintained by ICS sync.
- The frontend calendar can create/promote stays, but it still sends incorrect cleaning defaults for maintenance/personal-use stays and lacks new named-stay edit/status/cleaning workflows.
- `spec/openapi.yaml` is materially behind the actual API surface and the hand-authored frontend DTOs remain the effective source of truth.

Severity key:

- P0: blocks safe production rollout or cleanup.
- P1: violates a source-of-truth invariant or can cause incorrect live behavior after a gate/flow is used.
- P2: contract, UI, documentation, or test gap that can mislead users/agents or weaken verification.
- P3: cleanup/refactor concern with limited immediate behavior risk.

## Findings

### P0-01 - Stage 2 Backfill Apply Is Not Implemented

Spec expectation:

- Stage 2 must backfill raw blocks, named stays, nights, source links, mapping table, and integration `named_stay_id` links before downstream production cutovers.
- Backfill apply must be idempotent and resumable.

Current code evidence:

- `backend/internal/store/pms21_migration.go:81-91` implements a read-only planner and explicitly sets `ApplyImplemented: false`.
- `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md:14` says Stage 2 has local audit/dry-run planning only; apply mode is not implemented or run.

Risk:

- Production cannot safely enable raw-block dual-write, downstream named-stay cutovers, or Stage 11 cleanup without actual data migration.
- Stage 7/8 migrations backfill from `occupancy_stay_migration_map`; without a populated map, many rows remain unmapped.

Required resolution:

- Implement apply mode using the same planner classification logic.
- Add idempotency tests for apply twice.
- Save production dry-run and apply artifacts before enabling downstream gates.

### P0-02 - Required Production Audit Artifact Was Not Found

Spec expectation:

- Production data audit must be saved and reviewed before production schema application, backfill, cutover, or destructive cleanup.
- No destructive cleanup should proceed until the audit report is clean or every ambiguity is documented.

Current artifact evidence:

- `docs/pms-21-implementation-readiness.md:29-39` requires a production audit artifact.
- `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md:992-1030` defines required production audit counts and risk classes.
- Only local audit/verification docs were found under `docs/audits/`; no production/prod audit artifact was found.
- `docs/audits/PMS_21_local_data_audit_2026-07-13.md:117-128` still reports local risks: 238 raw-block candidates, 179 named-like candidates, 129 finance-synthetic rows, 32 unmatched finance bookings, one Nuki mapping issue, one external-sale classification issue, and overlapping raw blocks.

Risk:

- The implementation may be locally coherent but production data ambiguity remains unknown.
- Production cleanup decisions would be unauditable.

Required resolution:

- Run and commit/save the production read-only audit artifact before any production gate enablement.
- Carry forward every unresolved risk into explicit production rollout decision records.

### P0-03 - Rollout Gates In Code Are Much Narrower Than The Plan

Spec expectation:

- Recommended gates include `raw_blocks_dual_write`, `named_stays_read_model`, `occupancy_calendar_v2`, `cleaning_date_scoped_reconcile`, `nuki_named_stays`, `finance_named_stays`, `analytics_named_stays`, `availability_blocks_read_model`, `messages_named_stays`, `occupancy_export_disabled`, and `occupancy_legacy_write_disabled`.
- Every flag should default to safer legacy behavior until its backfill/verification passes.

Current code evidence:

- `backend/internal/config/config.go:38-40` exposes only `RawBlocksDualWrite`, `OccupancyExportDisabled`, and `OccupancyLegacyWriteDisabled`.
- `backend/internal/config/config.go:203-214` reads only `PMS21_RAW_BLOCKS_DUAL_WRITE`, `PMS21_OCCUPANCY_EXPORT_DISABLED`, and `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED`.
- `docs/pms-21-implementation-readiness.md:13-27` lists the wider gate set, but those gates are not visible in config.

Risk:

- Downstream named-stay behavior is effectively deployed as code-path changes rather than independently rollable cutovers.
- Rollback cannot isolate Nuki, finance, analytics, messages, cleaning, and availability behavior as described by the plan.

Required resolution:

- Either implement the missing gates or amend the spec/readiness docs to state that those gates were intentionally collapsed into deployment/version control.
- If collapsed, document exact rollback behavior for each cutover area without config flags.

### P1-04 - Nuki Generation Still Requires Legacy Occupancy Rows

Spec expectation:

- Nuki selection and generation should use active confirmed `named_stays` with eligible stay types.
- Raw blocks should never appear in Nuki.
- Stage 11 `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` should be able to stop new legacy compatibility writes after cutover.

Current code evidence:

- `backend/internal/store/nuki.go:384-400` selects eligible named stays, but also selects `osm.old_occupancy_id` as `LegacyOccupancyID`.
- `backend/internal/nuki/service.go:218-223` fails generation and marks the named stay error `legacy_occupancy_missing` if the selected stay has no legacy occupancy ID.
- `backend/internal/nuki/service.go:247-288` creates/links `nuki_access_codes` with `OccupancyID: stay.LegacyOccupancyID.Int64` even when `NamedStayID` is set.
- `backend/internal/migrate/000004_nuki.up.sql:19-35` defines `nuki_access_codes.occupancy_id INTEGER NOT NULL` and `UNIQUE(property_id, occupancy_id)`.
- `backend/internal/migrate/000032_raw_booking_blocks_named_stays.up.sql:183-188` adds nullable `named_stay_id`, but does not make `occupancy_id` nullable.

Risk:

- New eligible named stays created while `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED=1` cannot generate Nuki codes.
- Stage 11 non-destructive gate can stop legacy writes before Nuki is actually independent of legacy occupancy storage.

Required resolution:

- Add a migration that makes `nuki_access_codes.occupancy_id` nullable or introduces a new named-stay-primary code table/key strategy.
- Change `UpsertNukiCode` and Nuki generation to key by `(property_id, named_stay_id)` when available.
- Keep legacy `occupancy_id` only as optional historical attribution.

### P1-05 - Nuki Guest Daily Entries Still Upsert By Legacy Occupancy Identity

Spec expectation:

- `nuki_guest_daily_entries` should migrate to `named_stay_id` attribution while preserving historical occupancy attribution.

Current code evidence:

- `backend/internal/migrate/000020_nuki_guest_daily_entries.up.sql` creates rows keyed by `(property_id, occupancy_id, day_date)`.
- `backend/internal/migrate/000032_raw_booking_blocks_named_stays.up.sql:190-195` adds a partial unique index on `(property_id, named_stay_id, day_date)` but does not remove the legacy uniqueness.
- `backend/internal/store/nuki_guest_logs.go:23-55` still documents and writes `ON CONFLICT(property_id, occupancy_id, day_date)`.
- `backend/internal/store/nuki_guest_logs.go:59-80` reads by `named_stay_id`, so write identity and read identity are now mixed.

Risk:

- A named-stay-primary Nuki future cannot write guest entries without an occupancy ID.
- Duplicate/merge behavior may be governed by the wrong uniqueness key after relinking.

Required resolution:

- Change the writer conflict target to named-stay identity when `named_stay_id` is present.
- Make `occupancy_id` optional for new rows after compatibility validation.

### P1-06 - Named-Stay Update And Status Flows Do Not Trigger Nuki Update/Revoke

Spec expectation:

- Named-stay status changes must update nights, cleaning side effects, Nuki revocation/generation state, and compatibility rows transactionally where local DB state is involved.
- Nuki generation should reflect date, type, display name, status, and review eligibility changes.

Current code evidence:

- `backend/internal/api/occupancy_named_stay_handlers.go:171-172` triggers Nuki generation on raw-block promotion.
- `backend/internal/api/occupancy_named_stay_handlers.go:202-203` triggers Nuki generation on manual named-stay creation.
- `backend/internal/api/occupancy_named_stay_handlers.go:229-246` patch flow updates the stay and only reconciles cleaning.
- `backend/internal/api/occupancy_named_stay_handlers.go:266-273` status flow updates the stay and only reconciles cleaning.
- `backend/internal/store/named_stays.go:404-470` deactivates/reactivates nights and legacy compatibility rows, but does not revoke/update Nuki codes.

Risk:

- Changing check-in/check-out, display name, stay type, review status, cancelled, or archived status may leave stale active Nuki access windows or labels until a separate Nuki sync runs.
- Cancelled/archived stays may retain generated codes longer than intended.

Required resolution:

- Trigger Nuki sync/revocation after patch/status changes for impacted stays.
- Store visible Nuki error state if an update/revoke fails, similar to create/promote.
- Add tests for date change, name change, type change to non-eligible, cancellation, and archive.

### P1-07 - Raw Source Conflict And Source-Deleted Link Maintenance Appears Missing

Spec expectation:

- ICS sync may mark source links as `conflict` or `source_deleted`, but must not mutate named-stay business fields.
- Raw-source warning badges should clear automatically when active raw coverage again covers the named stay range.
- `stay_source_links` validity should be based on active raw coverage.

Current code evidence:

- `backend/internal/store/stay_calendar.go:258-284` reads and exposes `stay_source_links.link_status` and `conflict_reason` for calendar warnings.
- `backend/internal/store/occupancy_reconciliation.go:669-680` dual-writes raw block changes during ICS sync.
- `backend/internal/store/occupancy_reconciliation.go:895-908` marks raw booking blocks `deleted_from_source` and deactivates raw nights.
- No corresponding sync path was found that updates `stay_source_links.link_status` to `source_deleted`/`conflict`, increments `raw_block_conflicts`, or clears link status when coverage returns.

Risk:

- Calendar raw-source issue badges may never appear for a named stay whose source raw block disappears or shrinks.
- Users may believe a Booking.com-sourced named stay is still backed by active raw coverage when it is not.

Required resolution:

- Add a sync transaction step that recomputes linked named-stay coverage after raw block upsert/delete/shrink.
- Update `stay_source_links.link_status`, `conflict_reason`, and sync counters deterministically.
- Add tests for raw disappearance, shrink below stay, reappearance, UID split/merge, and auto-clear.

### P1-08 - Multi-Raw-Block Coverage Rule Is Not Implemented For Linked Stay Updates

Spec expectation:

- One named stay may link to multiple raw blocks when Booking.com splits, merges, or changes UIDs.
- Source-link validity should be based on the union of active linked raw-block nights.

Current code evidence:

- Schema supports multiple links: `backend/internal/migrate/000032_raw_booking_blocks_named_stays.up.sql:104-127`.
- `backend/internal/store/named_stays.go:675-695` allows updates only if at least one active linked raw block fully covers the new stay interval.
- The validation query checks one `raw_booking_blocks` interval with `rb.check_in_date <= ? AND rb.check_out_date >= ?`; it does not verify union coverage across multiple linked raw blocks.
- `backend/internal/store/named_stays.go:135-145` promotion creates from a single raw block; no relink/multi-link API was found.

Risk:

- A legitimate named stay backed by split Booking.com raw blocks cannot be edited across the split even though the union covers it.
- Conflict auto-clear cannot be implemented correctly without union coverage semantics.

Required resolution:

- Add source-link/relink API or store workflow for multiple raw blocks.
- Validate coverage by night union, not by a single raw interval.

### P1-09 - Frontend Overrides Correct Backend Cleaning Defaults For Maintenance And Personal Use

Spec expectation:

- Default cleaning rules: `booking_com` and `external` default to cleaning required; `maintenance` and `personal_use` default to no cleaning.

Current code evidence:

- Backend default is correct: `backend/internal/store/named_stays.go:631-633` returns true only for `booking_com` and `external`.
- Frontend manual stay dialog initializes `manualStayCleaningRequired = true`: `frontend/src/views/OccupancyView.vue:293-296`.
- Frontend promote dialog initializes `promoteCleaningRequired = true`: `frontend/src/views/OccupancyView.vue:287-288` and `frontend/src/views/OccupancyView.vue:334-335`.
- Frontend always sends `cleaning_required` on create/promote: `frontend/src/views/OccupancyView.vue:376-382` and `frontend/src/views/OccupancyView.vue:484-490`.

Risk:

- Creating a maintenance or personal-use named stay from the UI creates cleaning-required stays unless the user manually toggles the checkbox.
- This violates stay-type semantics and can create unwanted cleaning events.

Required resolution:

- Update UI defaults when stay type changes.
- Either omit `cleaning_required` unless explicitly overridden or reset it according to selected stay type.
- Add frontend tests for each stay type default.

### P1-10 - New Calendar UI Lacks Named-Stay Edit, Status, Archive/Cancel, And Cleaning Controls

Spec expectation:

- API includes `PATCH /stays/{stayId}`, `PATCH /stays/{stayId}/status`, and cleaning enable/disable behavior.
- Calendar UI should support named-stay management after creation/promotion, including cleaning status controls.

Current code evidence:

- Backend has new stay patch/status routes: `backend/internal/api/server.go:169-171`.
- Day details only display named-stay badges and cleaning summary: `frontend/src/views/OccupancyView.vue:1259-1283`.
- No edit/status/archive/cancel/toggle-cleaning action was found in the new calendar details section.
- Deprecated legacy occupancy list actions remain elsewhere: `backend/internal/api/server.go:156-167` and old occupancy UI flows in `frontend/src/views/OccupancyView.vue`.

Risk:

- Users can create/promote first-class named stays but must rely on legacy occupancy-as-stay workflows or missing UI paths to manage them.
- Stage 10 deprecation is weakened because old UI concepts remain necessary for normal lifecycle operations.

Required resolution:

- Add calendar named-stay detail/edit/status/cleaning actions using PMS 21 endpoints.
- Keep legacy actions only as compatibility until the new flow covers normal operations.

### P1-11 - OpenAPI Contract Is Incomplete And In Places Incorrect

Spec expectation:

- New stay/raw-block endpoints must be specified before frontend implementation.
- OpenAPI coverage must include changed dashboard, messages, Nuki, payout, invoice, analytics, cleaning-calendar, and compatibility/deprecation DTOs.
- Deprecated response headers should be explicit when routes emit them.

Current evidence:

- Backend routes include many endpoints not represented in `spec/openapi.yaml`, for example invoice CRUD/download/regenerate routes at `backend/internal/api/server.go:230-238`, message template and cleaning message routes at `backend/internal/api/server.go:239-245`, Nuki code/runs/sync/revoke/keypad routes at `backend/internal/api/server.go:179-188`, and finance rematch at `backend/internal/api/server.go:213`.
- `spec/openapi.yaml:445-448` puts `MonthQuery` at the path level for `/properties/{id}/availability-blocks`, making generated POST types require `month`; frontend POST does not send it at `frontend/src/views/OccupancyView.vue:425-436`.
- `spec/openapi.yaml:1437-1451` defines `NamedStayPatchRequest` without `manual_revenue_cents`, `manual_revenue_currency`, or `manual_revenue_note`, while backend accepts them at `backend/internal/api/occupancy_named_stay_handlers.go:41-51` and frontend sends them from Booking payouts.
- `spec/openapi.yaml:536-607` marks export/token endpoints deprecated but does not model `Deprecation` or `Warning` headers emitted by `backend/internal/api/deprecation.go:5-13`.
- Legacy occupancy-as-stay endpoints wrapped by deprecation handlers at `backend/internal/api/server.go:146-167` are not comprehensively documented as deprecated OpenAPI paths.

Risk:

- Generated clients and analysts receive a false contract.
- Frontend/backend contract drift is already visible in generated types and hand-authored types.

Required resolution:

- Reconcile OpenAPI with the actual route table.
- Move `MonthQuery` from path level to GET only for availability blocks.
- Add manual revenue fields to `NamedStayPatchRequest`.
- Add response headers for deprecated endpoints.
- Decide whether every route must be in OpenAPI or explicitly document allowed omissions.

### P1-12 - Dashboard Active Nuki Codes Still Use Occupancy Identity

Spec expectation:

- Dashboard upcoming stays and Nuki-related consumers should use named-stay identity after Stage 9/7 cutovers.

Current code evidence:

- Upcoming stays widget uses `stay_id`: `backend/internal/api/server_response_types.go:113-120` and `backend/internal/api/server.go:952-970`.
- Active Nuki code widget is still `occupancy_id` keyed: `backend/internal/api/server_response_types.go:122-131`.
- Dashboard builds active Nuki codes with `OccupancyID: row.OccupancyID`: `backend/internal/api/server.go:973-998`.
- Frontend type requires `occupancy_id`: `frontend/src/api/types/dashboard.ts:17-27`.
- Frontend uses `code.occupancy_id` as list key: `frontend/src/views/dashboard/DashboardNukiCodes.vue:30-32`.

Risk:

- Dashboard Nuki widget remains incompatible with a future named-stay-only Nuki code model.
- Legacy identity remains the only stable frontend key for generated Nuki dashboard rows.

Required resolution:

- Add `stay_id` and/or `nuki_code_id` to dashboard active code DTOs.
- Retain deprecated `occupancy_id` only as optional compatibility.

### P2-13 - Analytics Is Named-Stay Aware But Not Consistently Night-Table Primary

Spec expectation:

- Primary analytics source should be `named_stay_nights` joined to `named_stays`.

Current code evidence:

- `backend/internal/store/occupancy_nights.go:180-249` implements `OccupancyMetricNights` using `named_stay_nights` and availability blocks.
- `backend/internal/store/analytics.go:188-240` `ListActiveOccupanciesInDateRange` reads direct `named_stays` date ranges, not `named_stay_nights`.
- `backend/internal/store/analytics.go:398-499` `ListClosedOccupanciesInDateRange` also reads direct `named_stays` intervals and availability-block intervals, with legacy closed-row fallback.

Risk:

- If `named_stay_nights` becomes the hard overlap/source-of-truth table, stale or missing night rows may not affect all analytics consistently.
- Current behavior can still be correct when stay rows and night rows are perfectly maintained, but it is weaker than the spec invariant.

Required resolution:

- Decide whether analytics must strictly use night tables everywhere or whether row intervals are acceptable for derived helpers.
- If strict, update helper queries and tests to fail when row intervals and night rows diverge.

### P2-14 - Calendar KPI Counts External And Review-Required Stays As Sold

Spec expectation:

- External named stays count as sold only with linked finance data or manual revenue.
- `review_status = needs_review` rows reduce availability but must not count as sold/revenue.

Current code evidence:

- Calendar monthly KPI treats any active `booking_com` or `external` named stay as sold: `frontend/src/views/occupancy/OccupancyCalendar.vue:277-285`.
- The calendar DTO used by the component does not expose manual revenue or finance-linked status, so the UI cannot apply the full rule.

Risk:

- Calendar KPI can disagree with backend analytics.
- Users may see overstated named guest nights/occupancy for external stays without revenue or review-required stays.

Required resolution:

- Either remove sold-night KPI from frontend calendar or add `counts_as_sold`/`has_revenue`/`review_status` semantics from backend.
- Add frontend tests for external-without-revenue and needs-review stays.

### P2-15 - Generated OpenAPI Types Are Not The Frontend Source Of Truth

Spec expectation:

- Generated OpenAPI frontend types should become the source of truth where practical.

Current code evidence:

- `frontend/src/api/types/generated.ts` exists and is regenerated.
- `frontend/src/api/types/index.ts:7-15` exports hand-authored domain DTOs, not generated types.
- `frontend/src/api/types/README.md:5-15` says hand-authored types remain the current source of truth and generated types currently cover only system/auth/users/properties, which is stale.
- Application code imports hand-authored DTO modules such as `frontend/src/api/types/nuki.ts`, `dashboard.ts`, `bookingPayouts.ts`, `invoice.ts`, `messages.ts`, and `occupancy.ts`.

Risk:

- OpenAPI drift will not break frontend type-checks.
- Analysts may assume generated types enforce PMS 21 contracts when they do not.

Required resolution:

- Update README to current reality.
- Either migrate consumers to generated types module-by-module or explicitly document that OpenAPI is documentation-only for now.

### P2-16 - Nuki Upcoming-Stay DTO Still Requires Deprecated Occupancy ID

Spec expectation:

- Nuki upcoming-stays API and UI should use `stay_id` as the primary identity; deprecated `occupancy_id` can remain compatibility.

Current code evidence:

- Backend DTO includes required `OccupancyID int64`: `backend/internal/api/nuki_handlers.go:37-56`.
- Handler always emits `OccupancyID: row.OccupancyID`: `backend/internal/api/nuki_handlers.go:278-288`.
- Frontend type requires `occupancy_id`: `frontend/src/api/types/nuki.ts:15-25`.
- Generated OpenAPI type also requires deprecated `occupancy_id` in the upcoming stay schema.

Risk:

- The API contract cannot represent a named stay with no legacy occupancy row.
- This reinforces the Nuki legacy-write-disabled blocker.

Required resolution:

- Make `occupancy_id` optional/deprecated in backend DTO, frontend type, and OpenAPI.
- Use `stay_id` for UI keys and generate actions.

### P2-17 - Cleaning Calendar API DTO Still Presents Legacy Occupancy As Primary

Spec expectation:

- Cleaning local rows should gain `named_stay_id` and `raw_booking_block_id` ownership while preserving `occupancy_id` as nullable legacy compatibility.

Current code evidence:

- Store includes PMS 21 ownership fields and identities: `backend/internal/store/cleaning_calendar.go:70-72`, `backend/internal/store/cleaning_calendar.go:700-738`.
- Handler DTO still exposes `OccupancyID int64` and `NextOccupancyID` in `backend/internal/api/cleaning_calendar_handlers.go:33-40`.

Risk:

- API consumers may keep treating cleaning events as occupancy-owned.
- Future named-stay-only events may not serialize cleanly if `occupancy_id` is absent or zero.

Required resolution:

- Add `named_stay_id`, `raw_booking_block_id`, and `cleaning_identity` to the public cleaning event DTO if this endpoint is a PMS 21 contract.
- Mark `occupancy_id` as deprecated/optional in OpenAPI and frontend types.

### P2-18 - Stage Artifact Documentation Is Stale Or Incomplete

Spec expectation:

- Stage tracking table claims local verification artifacts for Stage 1 and Stage 2 dry-run activity.
- User request mentioned separate stage spec files; no separate stage spec files were found under `spec/`.

Current evidence:

- `docs/pms-21-implementation-readiness.md:3` stops at Stage 4, while the main plan says Stage 11 is current.
- No Stage 1 local schema verification artifact was found under `docs/audits/`, despite the tracking table claiming migration/FK/go-test verification.
- No Stage 2 dry-run verification report artifact was found under `docs/audits/`; only `PMS_21_local_data_audit_2026-07-13.md` was found.
- Stage 3 through Stage 11 local verification artifacts were found.

Risk:

- Future analysts may rely on tracking-table claims without reproducible artifacts.
- The phrase "stage spec files" is ambiguous because the repo currently has verification artifacts, not separate stage specs.

Required resolution:

- Update readiness doc to Stage 11 status or mark it superseded by the main plan.
- Add missing Stage 1/2 artifacts or amend the tracking table to point to the actual evidence.

### P3-19 - Legacy Finance Synthetic Occupancy Creation Still Exists As A Compatibility Function

Spec expectation:

- New finance imports must not silently create stay-like legacy `occupancies`.
- Stage 11 cleanup removes synthetic finance occupancy creation after compatibility window.

Current code evidence:

- `backend/internal/store/finance_booking_payouts.go:434-452` still exposes `FindOrCreateOccupancyForPayoutStayDates` and `FindOrCreateOccupancyForStatementStayDates`.
- `backend/internal/store/finance_booking_payouts.go:504-559` still inserts synthetic legacy `occupancies` when no legacy match exists and `OccupancyLegacyWriteDisabled` is false.
- No non-test call sites were found in the current tree, so this appears latent compatibility code.

Risk:

- If future code reuses these helpers while the default gate remains off, new synthetic occupancies can re-enter the system.
- The helper name invites use by finance agents despite Stage 8 intent.

Required resolution:

- Rename or restrict the helper to make legacy-only behavior explicit.
- Add a guard/test proving finance import/rematch code paths cannot call it.
- Remove after the approved cleanup window.

## Areas That Look Substantially Aligned

These are not findings, but they reduce ambiguity for the next analyst:

- Stage 1 additive schema broadly matches the target tables and integration columns in `backend/internal/migrate/000032_raw_booking_blocks_named_stays.up.sql`.
- Backend named-stay overlap prevention uses `named_stay_nights` transactionally in `backend/internal/store/named_stays.go:639-672`.
- Backend default cleaning rules are correct in `backend/internal/store/named_stays.go:631-633`.
- Cleaning reconciliation is date-scoped and delegates broad-window reconcile to date range in `backend/internal/cleaningcalendar/service.go:85-92`.
- Cleaning reconciliation lists Google events for the affected range in `backend/internal/cleaningcalendar/service.go:144-153`.
- Raw provisional cleaning coalesces by checkout date in `backend/internal/store/cleaning_calendar.go:700-716`.
- Google event metadata includes `pms_named_stay_id`, `pms_raw_booking_block_id`, and `pms_cleaning_identity` in `backend/internal/cleaningcalendar/google_client.go:179-193`.
- Backend deprecation headers are implemented by `backend/internal/api/deprecation.go:5-13` and applied to many legacy routes in `backend/internal/api/server.go:146-178`.
- Export-token/n8n frontend UI appears removed from active occupancy UI, though the OpenAPI/header documentation still needs tightening.

## Recommended Spec Tightening Questions

1. Are all recommended rollout gates mandatory, or may locally implemented cutovers ship without per-area feature flags?
2. Is `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` allowed to be enabled before Nuki code storage no longer requires `occupancy_id`?
3. Should Nuki code rows become named-stay-primary before Stage 11 is considered complete, or is that part of destructive cleanup?
4. Must analytics strictly use `named_stay_nights` everywhere, or are direct `named_stays` date ranges acceptable if night rows are maintained transactionally?
5. Should calendar KPI values mirror backend analytics sold-night rules, or are they only operational calendar counters?
6. Is OpenAPI intended to be complete for all backend routes, or only the PMS 21 changed surfaces? The current plan says changed dashboard/messages/Nuki/payout/invoice/analytics/cleaning-calendar/compatibility DTOs must be covered, which is stricter than current YAML.
7. Should `stay_source_links` support explicit user relinking to multiple raw blocks in the first migration, or only conflict display?
8. What artifact naming/location should be used for Stage 1 and Stage 2 verification, given no separate stage spec files were found under `spec/`?

## Production Gate Checklist Before Any Cleanup

- Production audit artifact exists and is reviewed.
- Stage 2 apply/backfill exists, is idempotent, and has run or has an approved rollout plan.
- `occupancy_stay_migration_map` is populated for all safe legacy named/raw/availability candidates.
- Nuki codes and guest daily entries can be written for named stays without requiring new legacy occupancy rows.
- Source-link conflict/source-deleted recomputation is implemented and tested.
- Dashboard, Nuki, cleaning-calendar, invoice, message, and finance DTOs can represent named-stay-primary identities with optional legacy `occupancy_id`.
- OpenAPI matches the actual production API contract or documented omissions are accepted.
- Frontend named-stay lifecycle actions exist without requiring deprecated occupancy-as-stay workflows.
- A release cycle has run with all approved cutover behavior before any destructive removal.
