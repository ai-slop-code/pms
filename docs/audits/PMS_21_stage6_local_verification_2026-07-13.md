PMS 21 Stage 6 Local Verification - 2026-07-13

Scope:

- Implemented date-scoped cleaning reconciliation via `ReconcilePropertyDateRange`.
- Added Google Calendar event listing and PMS-owned matching by stored ID, private metadata, and same-date title fallback.
- Added desired-state hashing so unchanged synced events are not patched.
- Wired cleaning event ownership to `named_stay_id`, `raw_booking_block_id`, and deterministic `cleaning_identity` during reconciliation.
- Kept legacy occupancy fallback for ranges without PMS 21 raw/named cleaning sources.

Verification:

- `go test ./...` from `backend/` passed.
- Added tests for unchanged desired-hash no-op reconciliation and PMS 21 raw/named cleaning ownership.

Notes:

- Stage 6 uses the cleaning columns added by Stage 1 migration `000032_raw_booking_blocks_named_stays`.
- Legacy broad entrypoint `ReconcileProperty` now delegates to date-range reconciliation for the existing default window.

Remaining Stage 6 rollout work:

- Validate Google list/match behavior against production calendars before approving rollout.
- Backfill or naturally reconcile existing production cleaning rows to populate PMS 21 ownership fields after production audit approval.
- Review legacy cleaning rows that cannot be matched to a named stay or raw block before any destructive cleanup.
- Keep downstream Nuki, finance, analytics, messages, and cleaning salary/daily-log cutovers in later stages.
