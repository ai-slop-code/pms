# PMS 21 Coding Agent Remediation Instructions - 2026-07-18

Status: Draft analyst handoff for a coding AI agent.

Source audit: `spec/PMS_21_Codebase_Divergence_Analysis_2026-07-18.md`.

Primary reference: `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`.

Purpose: convert the divergence audit into actionable, challenge-aware coding instructions. This is not an approval to enable production gates, run production backfills, or remove legacy storage.

## Operating Rules For The Coding Agent

- Do not modify application code until you have read this file, the divergence analysis, and the current main PMS 21 migration plan.
- Treat the worktree as possibly dirty. Do not revert user changes. Inspect diffs before editing files you touch.
- Keep changes small and staged by workstream. Do not attempt a single mega-patch across migration, Nuki, frontend, OpenAPI, and analytics.
- Preserve existing data unless a migration explicitly copies it and has tests proving preservation. This is especially important for Nuki PINs, external Nuki IDs, invoices, finance links, Google event IDs, and message history.
- Do not fabricate production audit artifacts. If a production audit cannot be run in the coding environment, add code/docs that make the required command clear and leave the artifact absent.
- Do not enable production feature gates by default. New gates must default to legacy-safe behavior unless the product owner explicitly approves otherwise.
- Prefer additive or compatibility-preserving migrations. Hard deletion/drop work remains out of scope until production backfill and release-cycle verification are complete.
- Every behavioral change must include tests or a written reason why tests cannot be added in this pass.
- Update `spec/openapi.yaml` and frontend DTOs together when changing API contracts, unless the decision is to explicitly keep OpenAPI documentation-only for that surface.

## High-Level Verdict

The divergence audit is mostly directionally correct, but several findings need to be challenged before implementation. The largest confirmed blockers are Stage 2 apply/backfill, Nuki's remaining dependency on legacy occupancy identity, missing source-link status recomputation, incorrect frontend cleaning defaults, and contract drift. The largest decision risks are rollout-gate scope, how complete OpenAPI must be, whether analytics must be strictly night-table-primary everywhere, and how much relinking UI is required for multi-raw-block coverage.

The coding agent should implement only the pieces that are mechanically required and validated. Where the spec demands a product or rollout decision, stop and ask instead of inventing policy.

## Owner Decisions Captured 2026-07-18

- Rollout gates: do not add the wider runtime flag set. Document these cutovers as version/deployment rollback. The owner wants to ship the new version as soon as safe migrations and verification allow, not run a long dual-mode legacy app.
- Stage 2 apply: payout/reservation-derived historical bookings should automatically become named stays when the source data is sufficient and unambiguous. Other non-payout/non-reservation stay candidates may support explicit override mode for review-required rows.
- Stage 2 classification detail: "payout/reservation-derived" means Booking.com payout rows and imported reservation statements. Legacy ICS guest-name occupancies do not count as reservation data for auto-confirmation.
- Stage 2 review status: historical rows from Booking.com payout rows or imported reservation statements may become confirmed named stays. Everything else must go to review. Override-created non-reservation rows default to `review_status = needs_review`.
- Nuki: must be fully named-stay-primary before `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` is considered safe. The deployment will switch to the new model rather than continue running the legacy app.
- Nuki schema migration: rebuilding `nuki_access_codes` is acceptable if row IDs and all generated PIN/external Nuki data are preserved.
- Source-link repair/relinking: automatic backend warnings are required for Booking.com-linked named stays. Example: raw booking data is synced, promoted into a named stay, then the guest cancels and the raw source disappears. The named stay should remain user-controlled, but the UI should warn that the Booking.com source is no longer active. Do not build manual relink API/UI by default unless a concrete workflow needs it.
- OpenAPI: cover everything, not only PMS 21 touched surfaces.
- Frontend types: keeping both generated OpenAPI types and hand-authored/domain DTOs can make sense, but the boundary must be explicit. Generated types should represent the API contract; hand-authored types may remain as UI/domain adapters where they add value.
- Calendar KPI: sold-night counts must match backend analytics semantics.
- Analytics: analytics must strictly read from `named_stay_nights`, not direct `named_stays` date intervals.
- Frontend lifecycle: include full named-stay edit, status, archive, cancel, and cleaning controls.
- Operations cutover: the owner will run the production audit/migration, but the coding agent must provide exact operator instructions for moving from the old binary to the new PMS 21 version safely.
- Legacy cleanup: destructive legacy code/data removal is desired later, but only after a successful PMS 21 release window. The cutover instructions must outline when and how cleanup becomes eligible.

## Challenges To The Divergence Analysis

### Challenge 1 - Do Not Treat The Main Plan's Stage Completion Text As Ground Truth

The main plan says several stages are complete and locally verified, while the divergence audit shows concrete contradictions in code. Use code and tests as truth for implementation readiness. When the plan says "complete" but code still depends on legacy occupancy identity, the plan must be corrected or narrowed.

Coding implication: when a workstream fixes a contradicted area, update the plan's completion notes to distinguish "locally implemented" from "production-rollout complete" and from "legacy-independent complete".

### Challenge 2 - Production Artifact Absence Is Not A Coding Bug

`P0-02` is a rollout blocker, but it cannot be fixed by writing code alone. The coding agent may add or improve the audit command, output schema, and documentation. It must not create a fake production audit report.

Coding implication: acceptance is a documented command and artifact template/location, not a production report unless the user explicitly provides production access and asks the agent to run it.

### Challenge 3 - Rollout Gates Need A Decision Before Broad Refactoring

The readiness doc lists many gates, but code currently has only three PMS 21 flags. Adding every missing gate may create many branches around already-merged code and introduce new inconsistencies. Collapsing gates into deployment/version control may be acceptable only if documented rollback behavior is explicit.

Owner decision: use documented deployment/version rollback for the wider cutover set. Do not add many runtime config flags just to match the readiness checklist. Keep only safety gates that still protect migration/destructive cleanup or externally exposed compatibility behavior.

### Challenge 4 - Stage 2 Apply Is Dangerous Enough To Need Dry-Run Parity

The dry-run planner classifies rows. Apply mode must reuse the same classification logic, not reimplement a second classifier that can drift. Applying data migration while unresolved conflict classes exist should be blocked unless an explicit `--allow-review-required` or equivalent is intentionally designed.

Coding implication: the first implementation target is an idempotent local apply with parity tests, not a production rollout.

### Challenge 5 - Nuki Table Constraint Changes Are A SQLite Migration Risk

`nuki_access_codes.occupancy_id` is `NOT NULL` and `UNIQUE(property_id, occupancy_id)` in the original table. SQLite cannot simply drop those constraints in place. A migration likely needs a table rebuild or an additive replacement strategy.

Coding implication: do not casually alter Nuki schema. Write a migration that preserves all existing rows, IDs if possible, encrypted PIN data, external IDs, valid windows, revocation timestamps, run links, and event-log references. Add migration tests or integration tests that prove preservation.

### Challenge 6 - Multi-Raw-Block Relinking May Be Backend-Only At First

The spec supports multiple `stay_source_links`, but it is not clear whether users need manual relink UI immediately or whether automatic recomputation is enough for the first production-safe iteration.

Owner decision: implement backend union coverage and automatic source-link status recomputation. Booking.com-linked named stays must show automatic warnings when the active raw source disappears, shrinks, or no longer covers the stay. Do not build manual relink API/UI unless automatic maintenance is insufficient for a concrete workflow.

### Challenge 7 - Calendar KPI Sold-Night Semantics Need Product Confirmation

The audit assumes the frontend monthly KPI must match backend analytics sold-night rules. It may instead be an operational calendar counter. The plan says external stays count as sold only with finance/manual revenue, so the safer default is to align with backend rules, but changing UI metrics may surprise users.

Owner decision: calendar KPI sold-night counts must match backend analytics rules. Add backend-provided semantics if needed; do not keep a frontend-only approximation.

### Challenge 8 - Analytics Night-Table Strictness May Be Over-Specified

The plan says analytics should primarily use `named_stay_nights`, but direct `named_stays` date ranges can be correct if night rows are maintained transactionally. Strict night-table use is strongest for overlap, capacity, and day-level metrics.

Owner decision: analytics must strictly read from `named_stay_nights`. Add tests around row/night divergence and update helper queries accordingly.

### Challenge 9 - OpenAPI Completeness Needs Scope Control

`spec/openapi.yaml` is behind the route table. Fully documenting every backend route may be large and distract from PMS 21 blockers. However, PMS 21 changed surfaces must not remain false.

Owner decision: OpenAPI should cover the full backend route table. This can be implemented in phases, but the target is complete API coverage, not PMS 21-only coverage.

### Challenge 10 - Generated Frontend Types Should Be Phased

The plan says generated OpenAPI types should become source of truth where practical. A repo-wide type migration is not required to fix PMS 21 behavior and can create churn.

Owner decision: both generated and hand-authored types may coexist. Generated types should define the API contract; hand-authored types should be explicit UI/domain adapters, not contradictory duplicate contracts.

## Recommended Implementation Order

1. Document the version/deployment rollback policy and remove false claims that wider runtime gates exist.
2. Fix Stage 2 dry-run/apply foundations and artifact documentation.
3. Make Nuki named-stay-primary enough that `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED=1` does not break new eligible stays.
4. Add named-stay patch/status side effects for Nuki generation/revocation.
5. Recompute source-link status and implement union coverage semantics; do not build manual relink UI unless a concrete need appears.
6. Implement full frontend named-stay lifecycle UI and correct cleaning defaults.
7. Reconcile complete OpenAPI route coverage and define generated-vs-domain frontend type boundaries.
8. Make analytics and calendar KPI semantics strictly `named_stay_nights`-based and consistent.
9. Clean up stale docs and latent legacy helper risks.

Do not start with OpenAPI-only cleanup unless the task is explicitly narrowed to contracts. The production blockers are data migration and legacy identity dependencies.

## Workstream A - Preflight And Baseline Verification

Goal: understand current local state and avoid overwriting concurrent work.

Files to read first:

- `spec/PMS_21_Codebase_Divergence_Analysis_2026-07-18.md`
- `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`
- `docs/pms-21-implementation-readiness.md`
- `backend/cmd/pms21-migration/main.go`
- `backend/internal/store/pms21_migration.go`
- `backend/internal/config/config.go`
- `backend/internal/migrate/000032_raw_booking_blocks_named_stays.up.sql`

Required actions:

- Run `git status --short` and inspect diffs for files you plan to touch.
- Find latest migration number before adding migrations.
- Identify backend and frontend test commands from existing project files. Do not guess if scripts already exist.
- Write down which divergence findings are in scope for the current coding session.

Acceptance:

- The agent can state which files are dirty before edits.
- The agent can state which findings are being fixed and which are deferred as product/rollout decisions.

## Workstream B - Stage 2 Apply And Production Audit Foundation

Related audit findings: `P0-01`, `P0-02`, `P2-18`.

Confirmed evidence:

- `backend/internal/store/pms21_migration.go` exposes a dry-run planner with `ApplyImplemented: false`.
- `backend/cmd/pms21-migration/main.go` refuses to run without `--dry-run`.
- No production audit artifact was found under `docs/audits/`.

Goal: implement an idempotent, resumable local apply path for safe rows and make production artifact expectations explicit. Do not run production apply.

Owner classification rules:

- Historical bookings derived from Booking.com payout rows or imported reservation statements should automatically become confirmed named stays.
- Legacy ICS guest-name occupancies are not reservation data for auto-confirmation.
- Candidates not backed by Booking.com payout rows or imported reservation statements must be review-required and should support explicit override mode.
- Override mode must be auditable and explicit; it must not silently classify ambiguous rows as confirmed named stays.

Implementation instructions:

- Refactor dry-run classification into reusable row-level classification functions. The dry-run report and apply path must use the same classifier.
- Add an apply command mode to `backend/cmd/pms21-migration/main.go`, for example `--apply`, while preserving `--dry-run` as the safe default.
- Require explicit mode selection. Do not make apply the default.
- Add guardrails before apply:
- Refuse apply if additive PMS 21 tables do not exist.
- Refuse apply if severe conflict counts are non-zero unless an explicit override is designed, documented, and logged in the output report.
- Refuse apply if the database path is empty or ambiguous.
- Print a summary before apply. If interactive prompts are unsuitable for automation, require a flag such as `--confirm-apply`.
- Insert or upsert rows idempotently for safe classifications:
- `raw_booking_blocks`
- `raw_booking_block_nights`
- `named_stays`
- `named_stay_nights`
- `stay_source_links`
- `property_availability_blocks`
- `occupancy_stay_migration_map`
- Backfill integration links only when mapping is unambiguous:
- `nuki_access_codes.named_stay_id`
- `nuki_guest_daily_entries.named_stay_id`
- `cleaning_calendar_events.named_stay_id`
- `cleaning_calendar_events.raw_booking_block_id`
- `finance_bookings.named_stay_id`
- `invoices.named_stay_id`
- Preserve legacy IDs in mapping rows. Do not delete or null legacy columns in this workstream.
- Make the report include actual created/updated/skipped counts, conflict counts, and samples after apply.
- Make the report distinguish auto-confirmed named stays from review-required override-created named stays.
- Ensure apply can be run twice with no duplicate rows and no changed business data on the second run.

Tests to add:

- Dry-run followed by apply creates expected rows from representative legacy raw block, named stay, availability block, and synthetic finance cases.
- Apply twice is idempotent.
- Apply refuses unresolved overlap/conflict cases.
- Booking.com payout rows and imported reservation statements become confirmed named stays.
- Legacy ICS guest-name occupancies do not become confirmed named stays solely because they have guest-like data.
- Non-payout/non-reservation review-required rows require explicit override and are marked `review_status = needs_review`.
- Apply backfills Nuki, cleaning, finance, and invoice links only when mapping is unique.
- Apply preserves legacy `occupancies` and existing integration rows.

Documentation updates:

- Add an artifact naming convention, for example `docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md` for reviewed production audit output.
- Update the main plan and readiness doc to say production audit is still absent unless the artifact is actually created from production.
- Add exact dry-run and apply command examples.

Do not:

- Create a fake production audit report.
- Enable downstream cutover gates.
- Delete legacy occupancy data.

## Workstream C - Rollout Gate Policy

Related audit finding: `P0-03`.

Confirmed evidence:

- Config currently exposes `PMS21_RAW_BLOCKS_DUAL_WRITE`, `PMS21_OCCUPANCY_EXPORT_DISABLED`, and `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED`.
- Readiness docs list many more gates.

Owner decision:

- Use version/deployment rollback for the wider stage cutovers.
- Do not add the full listed runtime gate set.
- The product direction is to switch to the new model after required migrations and verification, not run the legacy app indefinitely.

Implementation instructions:

- Update `docs/pms-21-implementation-readiness.md` and the main plan to remove or qualify the wider gate list.
- For every collapsed gate, document rollback behavior and why a runtime flag is not required.
- Keep existing safety gates where still useful, especially export disablement and legacy-write cleanup until named-stay-primary dependencies are safe.
- Add a release checklist that states migrations/backfill must run before deploying the version that assumes PMS 21 named-stay truth.
- Document that rollback means deploying the prior version plus preserving additive tables, not flipping per-area flags.

Do not:

- Add unused flags just to match a list.
- Leave docs claiming gates exist when they do not.

## Workstream D - Nuki Named-Stay-Primary Storage And Generation

Related audit findings: `P1-04`, `P1-05`, `P1-12`, `P2-16`.

Confirmed evidence:

- `nuki_access_codes.occupancy_id` is non-null and unique by `(property_id, occupancy_id)`.
- `nuki_access_codes.named_stay_id` was added later, but `UpsertNukiCode` still conflicts on legacy occupancy identity.
- Nuki generation marks `legacy_occupancy_missing` if a named stay has no migration-map occupancy.
- Guest daily entries still upsert by `(property_id, occupancy_id, day_date)`.
- Nuki dashboard/upcoming DTOs still require legacy occupancy ID in places.

Goal: eligible active confirmed named stays can generate, update, revoke, list, and report Nuki data without requiring a new legacy occupancy row.

Schema instructions:

- Add a new migration after the current latest migration.
- Decide whether to rebuild `nuki_access_codes` or introduce an additive companion table. Prefer table rebuild only if tests can prove preservation.
- Make `occupancy_id` nullable for `nuki_access_codes`, or otherwise remove the requirement from new named-stay-primary writes.
- Ensure uniqueness for new rows is by `(property_id, named_stay_id)` when `named_stay_id IS NOT NULL`.
- Preserve or adapt uniqueness for legacy-only rows. If SQLite allows multiple NULLs under a unique index, verify behavior with tests.
- Ensure `nuki_event_logs.nuki_access_code_id` references remain valid. If rebuilding the table, preserve `id` values or migrate references.
- Make `nuki_guest_daily_entries.occupancy_id` nullable if new named-stay-primary guest entries are expected.
- Use `(property_id, named_stay_id, day_date)` as the conflict identity when `named_stay_id` is present.
- Preserve legacy `(property_id, occupancy_id, day_date)` behavior for rows that do not have a named stay.

Store/service instructions:

- Update `NukiAccessCode` and related scan/insert code so `OccupancyID` can be optional where needed. Avoid representing missing occupancy as zero in public contracts.
- Change `UpsertNukiCode` to choose named-stay conflict identity when `NamedStayID.Valid` is true.
- Change Nuki generation to stop failing solely because `LegacyOccupancyID` is missing.
- Keep legacy lookup fallback for existing codes that only have occupancy identity.
- Update `ListNukiCodes`, dashboard active code queries, upcoming-stay queries, and keypad reconciliation to not require an inner join to `occupancies` when `named_stay_id` exists.
- Use `stay_id` or `nuki_code_id` as frontend identity keys. Keep `occupancy_id` optional/deprecated only for historical compatibility.
- Preserve PIN encryption/decryption behavior exactly.
- Preserve existing generated PINs and external Nuki IDs during relinking.

API/OpenAPI/frontend instructions:

- Make `occupancy_id` optional/deprecated in Nuki upcoming-stay and active-code DTOs.
- Add or confirm `stay_id` is required for named-stay-primary Nuki actions.
- Update frontend types and components to key rows by `stay_id` or `generated_code_id`/`nuki_code_id`, not mandatory `occupancy_id`.
- Update `spec/openapi.yaml` for all touched Nuki DTOs and routes.

Tests to add:

- New named stay with no migration-map occupancy generates a Nuki code.
- Existing legacy code with both IDs is updated without losing PIN/external ID.
- Existing legacy code can still be revoked.
- Cancelled, archived, needs-review, maintenance, and personal-use stays are revoked or excluded as appropriate.
- Guest daily entry upsert uses named-stay identity when available and remains idempotent.
- Dashboard/Nuki frontend type checks pass with missing `occupancy_id`.

Do not:

- Generate raw booking block codes.
- Require `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED=0` for new eligible named stays.
- Break historical Nuki rows that have only legacy occupancy attribution.

## Workstream E - Named-Stay Patch/Status Side Effects

Related audit finding: `P1-06`.

Confirmed evidence:

- Create/promote calls trigger Nuki generation.
- Patch/status flows reconcile cleaning but do not trigger Nuki generation or revocation.

Goal: Nuki and cleaning side effects stay consistent when named-stay fields change.

Implementation instructions:

- After `PATCH /stays/{stayId}`, compare old and new values for check-in, check-out, display name, stay type, review status if editable, cleaning, and manual revenue only where relevant.
- Trigger Nuki regeneration/update if an eligible stay's dates or display name changed.
- Trigger Nuki revocation if a stay becomes ineligible by status, review status, stay outcome, or stay type.
- After `PATCH /stays/{stayId}/status`, revoke for cancelled/archived and regenerate if reactivated and eligible.
- Store visible Nuki error state on failure using existing `nuki_generation_status`/`nuki_generation_error` semantics.
- Keep local DB transaction boundaries sane: database stay update should not be rolled back because an external Nuki API call fails, but the failure must be visible.
- Continue date-scoped cleaning reconciliation for old and new ranges.

Tests to add:

- Date change updates Nuki valid window.
- Name change updates code label or marks update error.
- Stay type changed from `booking_com` to `maintenance` revokes or marks code ineligible.
- Cancellation/archive revokes generated code.
- Re-activate eligible stay regenerates or reuses code.
- Nuki client failure leaves stay updated and stores error state.

## Workstream F - Source-Link Status And Multi-Raw-Block Coverage

Related audit findings: `P1-07`, `P1-08`.

Confirmed evidence:

- Calendar reads `stay_source_links.link_status` and `conflict_reason`.
- ICS raw blocks are upserted/deleted, but no matching source-link recomputation path was found.
- Update validation requires one raw block to fully cover the named-stay interval instead of union coverage.

Goal: raw-source warnings reflect active raw coverage, and linked stays can be backed by multiple raw blocks.

Implementation instructions:

- Add a store function to recompute source-link health for impacted property/stay IDs after raw block upsert, deletion, shrink, or status change.
- Recompute based on the union of active linked raw-block nights, not a single interval.
- Mark links/stays active when active linked raw coverage fully covers the named stay's nights.
- Mark `source_deleted` when linked raw blocks are deleted from source or inactive and no active coverage remains.
- Mark `conflict` when active linked raw coverage exists but does not cover the named stay's full range, or when linked date ranges contradict raw block dates.
- Keep the named stay business fields untouched when raw source warnings are raised. Cancellation from missing raw data is a warning/review signal, not an automatic named-stay cancellation.
- Set deterministic `conflict_reason` values suitable for UI badges, for example `raw_source_missing`, `raw_coverage_gap`, `raw_coverage_shrunk`, or `raw_link_ambiguous`.
- Clear `conflict_reason` automatically when coverage becomes valid again.
- Increment or populate sync conflict counters if the sync report exposes raw source conflicts.
- Update `ensureNamedStayWithinActiveLinksTx` to validate night-union coverage.
- If no active links exist for a manual external/maintenance/personal stay, do not block updates on raw coverage.
- Consider an internal relink function that can attach additional raw blocks by UID/date. Ask before exposing a manual relink API/UI.

Tests to add:

- Raw block disappears -> linked stay source warning becomes `source_deleted`.
- Raw block shrinks below stay -> warning becomes `conflict`.
- Raw block reappears/covers range -> warning clears.
- Two adjacent raw blocks cover one stay -> update is allowed by union coverage.
- One-night gap across linked raw blocks -> update is rejected or warning remains conflict.
- UID split/merge scenario does not mutate named-stay business fields.

Do not:

- Let ICS sync overwrite named-stay display name, stay type, cleaning, revenue, or status.
- Treat raw blocks as Nuki/analytics/finance truth.

## Workstream G - Frontend Named-Stay Lifecycle And Cleaning Defaults

Related audit findings: `P1-09`, `P1-10`, `P2-14`.

Confirmed evidence:

- Backend defaults cleaning to true only for `booking_com` and `external`.
- Frontend create/promote dialogs initialize cleaning to true and always send `cleaning_required`.
- Calendar day details display named-stay state but do not expose full edit/status/cleaning lifecycle actions.
- Calendar KPI treats every active external stay as sold.

Goal: frontend UI should not override backend semantics and should allow basic first-class named-stay lifecycle management without falling back to deprecated occupancy-as-stay flows.

Cleaning default instructions:

- Add a frontend helper matching backend defaults: cleaning is true for `booking_com` and `external`, false for `maintenance` and `personal_use`.
- When opening create/promote dialogs, initialize cleaning from selected stay type.
- When stay type changes and the user has not explicitly overridden cleaning, update cleaning to the default for the new type.
- Consider omitting `cleaning_required` from POST when no explicit override was made, letting backend default be authoritative.
- If always sending `cleaning_required`, ensure the sent value tracks the selected stay type default unless user changed it.

Lifecycle UI instructions:

- Add named-stay edit action from calendar day details.
- Support editing display name, date range, stay type, and cleaning required.
- Support status action for cancel, archive, and reactivate as allowed by backend endpoints.
- Include the full lifecycle in this pass; do not stop after cleaning defaults.
- Show Nuki error state and raw-source warning details already exposed by calendar DTOs.
- Prefer using PMS 21 `/stays/{stayId}` endpoints over legacy occupancy routes.
- Keep legacy occupancy UI only where functionality is not yet covered, and mark it as compatibility if user-facing text exists.

Calendar KPI instructions:

- Do not compute sold nights from `stay_type` alone.
- Add backend DTO fields such as `counts_as_sold`, `has_revenue`, or `revenue_status` to named stays in the calendar response if the frontend otherwise lacks enough information.
- Exclude `review_status = needs_review` from sold/revenue counts.
- Count external stays as sold only when linked finance data or manual revenue exists.

Tests to add:

- Create maintenance defaults cleaning false.
- Create personal use defaults cleaning false.
- Create external and Booking.com default cleaning true.
- Changing stay type updates default unless user manually changed cleaning.
- Edit named stay calls `PATCH /stays/{stayId}` with expected payload.
- Cancel/archive calls status endpoint and refreshes calendar.
- KPI excludes unfunded external and needs-review stays.

## Workstream H - OpenAPI And Frontend Type Contract

Related audit findings: `P1-11`, `P2-15`, plus DTO findings in Nuki/dashboard/cleaning.

Confirmed evidence:

- `spec/openapi.yaml` has path/query mistakes for availability blocks.
- `NamedStayPatchRequest` omits manual revenue fields accepted by backend.
- Deprecated headers are emitted by backend but not consistently documented.
- Hand-authored frontend DTOs remain effective source of truth.

Goal: `spec/openapi.yaml` should become truthful for the full backend route table, while prioritizing PMS 21 blockers first if the work must be phased.

Owner decision: OpenAPI should cover every backend route. It is acceptable to phase the implementation, but do not document PMS 21-only coverage as the final target.

Implementation instructions:

- Fix `/properties/{id}/availability-blocks` so month query applies to GET only, not POST.
- Add manual revenue fields to `NamedStayPatchRequest`.
- Document `Deprecation` and `Warning` headers on deprecated occupancy/export/token endpoints that emit them.
- Update OpenAPI schemas for Nuki, dashboard active codes, upcoming stays, cleaning calendar, named stays, availability blocks, finance/invoice DTOs only when those surfaces are touched.
- Add missing paths for the full backend route table, including invoice, message, Nuki, finance rematch/import, analytics, cleaning calendar, occupancy compatibility, and system/admin routes.
- Regenerate frontend OpenAPI types if that is the repo practice.
- Update `frontend/src/api/types/README.md` to define the boundary: generated OpenAPI types are API contract types; hand-authored types are allowed only as UI/domain adapters or compatibility shims.
- Avoid a repo-wide frontend type rewrite unless needed for contract correctness, but eliminate contradictory duplicate type definitions when touching a surface.

Acceptance:

- OpenAPI no longer contradicts backend for touched endpoints.
- OpenAPI has a route-table coverage checklist for any routes not completed in the first pass.
- Frontend type checks do not rely on mandatory `occupancy_id` for named-stay-primary rows.
- The README clearly states generated types are API contract types and hand-authored types are UI/domain adapters or compatibility shims.

## Workstream I - Cleaning Calendar Public DTO Identity

Related audit finding: `P2-17`.

Confirmed evidence:

- Store has PMS 21 ownership fields for cleaning calendar rows.
- API DTO still presents legacy occupancy identity as primary.

Goal: public cleaning event DTOs can represent named-stay-owned and raw-block-owned events.

Implementation instructions:

- Add `named_stay_id`, `raw_booking_block_id`, and `cleaning_identity` to cleaning event response DTOs if this endpoint is a PMS 21 contract.
- Make `occupancy_id` optional/deprecated in API schema and frontend types where possible.
- Ensure JSON omits missing legacy IDs rather than serializing zero.
- Preserve existing frontend behavior for legacy rows.

Tests to add:

- Named-stay final cleaning serializes named stay identity.
- Raw provisional cleaning serializes raw block identity.
- Legacy-only cleaning row still serializes compat identity.

## Workstream J - Analytics And Messages Semantics

Related audit finding: `P2-13` and parts of `P2-14`.

Goal: analytics and calendar counters should not disagree silently.

Owner decision: analytics must strictly read from `named_stay_nights`.

Implementation instructions:

- Add tests that intentionally create divergence between `named_stays` date range and `named_stay_nights` rows.
- Update active/closed occupancy helper queries and all day-level analytics to join active `named_stay_nights`.
- Preserve legacy fallback only for properties with no PMS 21 named-stay rows, if that is still the intended behavior.
- Ensure `review_status = needs_review` and unfunded external stays do not count as sold/revenue where backend analytics semantics require that.

Tests to add:

- Active named stay with missing night row does not inflate day-level analytics if night-table strictness is chosen.
- Unfunded external stay reduces availability but not sold/revenue metrics.
- Review-required stay reduces availability but not sold/revenue metrics.
- Maintenance/personal use reduce bookable availability without sold/revenue metrics.

## Workstream K - Legacy Finance Synthetic Helper Risk

Related audit finding: `P3-19`.

Confirmed evidence:

- Legacy finance helper still exists and inserts synthetic legacy occupancies when cleanup gate is false.
- No non-test call sites were found in the divergence audit.

Goal: prevent accidental reuse of legacy synthetic occupancy creation.

Implementation instructions:

- Rename helper or add comments/tests making it explicitly legacy-only if it remains.
- Add tests proving current finance import/rematch paths do not call it.
- Consider making helper unexported if no legitimate external store caller exists.
- Do not remove it if tests or rollback paths still require it.

Acceptance:

- Future agents are less likely to call synthetic legacy occupancy creation by mistake.
- Cleanup gate behavior remains unchanged unless explicitly changed in a Nuki/Stage 11 workstream.

## Workstream L - Documentation And Artifact Alignment

Related audit finding: `P2-18`.

Goal: docs should describe current reality without overstating readiness.

Implementation instructions:

- Update `docs/pms-21-implementation-readiness.md` if it remains active; otherwise mark it superseded by the main plan and point to the current plan.
- Add missing Stage 1/2 artifact references only if real artifacts exist.
- Do not claim Stage 2 apply is complete until apply exists, tests pass, and artifact instructions are updated.
- Update the main plan's completion sections for any area where this remediation changes readiness.
- Add an operations cutover runbook for the production switch from the old binary to the new PMS 21 binary.
- Add a post-cutover destructive cleanup plan that is explicitly gated by successful production operation on the new model.

Acceptance:

- A future analyst can find the source of truth for stage status.
- Stage status distinguishes local verification, production audit, production apply, and destructive cleanup readiness.
- An operator can follow the documented runbook without guessing command order, expected artifacts, verification queries, rollback points, or cleanup timing.

## Workstream M - Operations Cutover Runbook

Goal: provide exact production operations instructions for switching from the current old binary to the new PMS 21 version.

Owner decision:

- The owner will run the production audit and migration.
- The codebase must provide clear operations instructions before the owner does that work.
- The intended deployment model is a version switch to PMS 21, not long-term dual operation of old and new app behavior.

Required runbook contents:

- Preconditions:
- Confirm the exact old binary/version currently running.
- Confirm the exact new binary/version/commit to deploy.
- Confirm database backup location and restore procedure.
- Confirm maintenance window or reduced-traffic period.
- Confirm no other migration or sync jobs are running.
- Confirm Nuki, Booking.com ICS sync, Google cleaning calendar, finance imports, invoice generation, and message jobs are paused or safe to run during migration.
- Confirm latest migrations included in the new binary.
- Confirm `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` and any remaining safety gates are set as intended.
- Confirm OpenAPI/frontend assets correspond to the deployed backend version.

- Backup steps:
- Stop or pause the old app/jobs as required.
- Take a database backup before applying migrations or running PMS 21 apply.
- Record backup path, timestamp, database size/checksum if practical, old binary version, and new binary version.
- Verify the backup can be opened or restored in a safe environment if practical.

- Dry-run/audit steps:
- Run the PMS 21 production dry-run/audit command against the production database backup or production database in read-only mode.
- Save the artifact under the documented `docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md` or equivalent operational artifact location.
- Review counts for auto-confirmed named stays, review-required rows, conflicts, unmapped integration rows, and destructive-cleanup blockers.
- Stop if severe conflict counts are non-zero and no owner-approved override exists.

- Apply steps:
- Run additive database migrations required by the new binary.
- Run Stage 2 PMS 21 apply with explicit confirmation flags.
- Save the apply output artifact, including created/updated/skipped counts and review-required rows.
- Run apply a second time or run an idempotency check if the command supports it; the second run must report no duplicate creation and no unintended business-data mutation.

- Deployment steps:
- Deploy the new PMS 21 binary/frontend after successful migration apply and verification.
- Resume jobs in a safe order, starting with read-only/listing checks before write-heavy syncs where possible.
- Run Booking.com ICS sync and verify raw blocks/source-link warnings behave as expected.
- Run or trigger Nuki sync and verify named-stay-primary code generation/listing without legacy occupancy dependency.
- Run cleaning reconciliation for a narrow range and verify named-stay/raw-block ownership fields.

- Verification steps:
- Verify representative Booking.com payout/reservation-derived historical rows became confirmed named stays.
- Verify non-payout/non-reservation stay-like rows are review-required, not silently confirmed.
- Verify `named_stay_nights` exists and drives analytics results.
- Verify calendar KPI matches backend analytics semantics for sold nights.
- Verify frontend can create, edit, cancel, archive, reactivate, and toggle cleaning for named stays.
- Verify Nuki upcoming stays, active codes, dashboard widget, and guest daily entries do not require new legacy occupancy rows.
- Verify invoices, finance mappings, messages, cleaning calendar, and dashboard rows reference named stays where expected.
- Verify deprecated legacy endpoints either still work as compatibility or are intentionally disabled according to the release plan.

- Rollback points:
- Before migrations/apply: restore old binary and continue with original database.
- After additive migrations but before apply: old binary may continue if additive tables/columns are compatible; document exact caveats.
- After apply but before new binary traffic: prefer restoring the pre-apply backup if rollback is required.
- After new binary traffic: rollback requires a decision. Because the new version may write named-stay-primary data, restoring the old database backup can lose post-cutover writes. Document this as an explicit operational risk.

- Post-cutover monitoring:
- Monitor Nuki generation errors, source-link conflicts, cleaning reconciliation errors, finance/import errors, invoice creation, message generation, and analytics/dashboard mismatches.
- Review all `needs_review` named stays created by migration.
- Keep database backups from before and after cutover.

- Legacy cleanup eligibility:
- Do not drop legacy occupancy columns/tables/routes/token storage immediately after cutover.
- Cleanup becomes eligible only after the new PMS 21 version runs successfully for an agreed release window and no rollback to the old binary is expected.
- Before destructive cleanup, run a cleanup readiness audit proving no required production behavior still depends on legacy-only data.
- Cleanup instructions must list exact objects to remove, backup requirements, restore implications, and tests to run after removal.

Acceptance:

- The runbook includes exact commands once the final command names/flags exist.
- The runbook tells the operator when to stop.
- The runbook describes rollback consequences at each phase.
- The runbook includes a later destructive cleanup path without authorizing immediate deletion.

## Cross-Cutting Test Commands

The coding agent must discover exact project commands from repo files before running them. Likely categories:

- Backend Go tests for changed packages.
- Migration tests or database integration tests around schema changes.
- Frontend typecheck/tests for changed Vue/API DTO code.
- OpenAPI generation/validation if scripts exist.

Minimum expectation by workstream:

- Backend store/API changes: run targeted Go tests for touched packages, then broader backend tests if feasible.
- Migration changes: run migration tests or create a temporary database migration test path.
- Frontend changes: run typecheck and relevant frontend tests.
- OpenAPI changes: regenerate generated types and verify no unreviewed generated churn.

## Stop-And-Ask Decision Points

Ask the user/product owner before implementing any of these:

- Running anything against production data.
- Creating manual source relink UI/API beyond automatic source-link recomputation.
- Rebuilding Nuki tables if preserving `id` values cannot be guaranteed.
- Removing or hard-deleting legacy occupancy/token/storage/routes.
- Performing a repo-wide frontend migration to generated OpenAPI types beyond defining generated contract types plus hand-authored domain adapters.
- Classifying review-required non-payout/non-reservation historical rows as confirmed named stays without an explicit owner-approved override rule.

## Definition Of Done For A Safe Remediation Pass

- Stage 2 apply either exists with idempotency tests or remains explicitly blocked with no false completion claims.
- Production audit absence is documented honestly, with commands and artifact location ready.
- Nuki can operate for named stays without requiring newly written legacy occupancy rows, or `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` remains documented as unsafe for Nuki.
- Named-stay updates/status changes reconcile cleaning and Nuki side effects.
- Source-link badges are maintained by sync/recompute logic and clear automatically.
- Frontend creation/promote flows respect stay-type cleaning defaults.
- First-class named-stay edit/status/cleaning lifecycle paths exist or are explicitly deferred with legacy dependency documented.
- OpenAPI is truthful for all touched PMS 21 routes and DTOs.
- Docs no longer claim production readiness where production audit/backfill is missing.
