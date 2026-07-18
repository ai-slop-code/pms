# PMS 21 Stage 9 Local Verification - 2026-07-14

Scope: Stage 9 analytics, messages, and dashboard cutover to named-stay semantics.

Implemented locally:

- Analytics primary stay selection now reads active confirmed `named_stays` and excludes raw booking blocks.
- External named stays count as sold/revenue only when linked finance data exists or manual revenue is present.
- Maintenance, personal-use, unfunded external, review-required stays, availability blocks, and legacy closed rows reduce bookable availability without increasing sold nights.
- Finance analytics totals and matched stay IDs use `finance_bookings.named_stay_id`, with manual external revenue included when no finance booking exists.
- Returning guest, demand, gaps, pace, ADR/RevPAR, net-per-stay, cancellation, and night-level performance helpers are named-stay aware.
- Message stay picker and generation use `stay_id`; deprecated `occupancy_id` remains accepted only through compatibility resolution/fallback.
- Cleaning-staff messages read final cleaning-required named stays instead of raw/legacy occupancy rows.
- Dashboard upcoming stays/check-in KPI reads named stays and emits `stay_id` as the primary identity.
- OpenAPI coverage was extended for dashboard and message named-stay response/request shapes.

Compatibility retained:

- Local legacy test fixtures and pre-backfill empty named-stay datasets fall back to legacy reads only when a property has no `named_stays` rows.
- Deprecated message `occupancy_id` remains accepted for compatibility and resolves through `occupancy_stay_migration_map` where available.

Verification run:

- `go test ./...` from `backend/` passed.
- `npm run type-check` from `frontend/` passed.
- `npm run test` from `frontend/` passed: 53 files, 277 tests.
- `npm run build` from `frontend/` passed.

Focused Stage 9 coverage added:

- `TestAnalyticsStage9_NamedStaySemanticsExcludeRawAndUnfundedExternal` verifies raw blocks are excluded, external stays without revenue do not count as sold, manual external revenue counts, and non-sold named stays reduce availability.

Remaining rollout/audit work:

- Production analytics/messages/dashboard cutover remains blocked until production PMS 21 backfills and audit approval are complete.
- Public occupancy export deprecation/removal remains Stage 10 work.
