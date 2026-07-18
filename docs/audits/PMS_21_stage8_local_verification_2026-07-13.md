# PMS 21 Stage 8 Local Verification - 2026-07-13

Scope: Finance, payout, and invoice cutover to named stays.

Implemented locally:

- Added migration `000035_finance_invoice_named_stay_cutover` to backfill `finance_bookings.named_stay_id` and `invoices.named_stay_id` while preserving legacy `occupancy_id` values.
- Updated finance booking persistence, list DTOs, and mapping APIs so `named_stay_id` is the primary stay identity and `occupancy_id` remains deprecated compatibility.
- Changed payout import/rematch to deterministic named-stay matching only; finance imports no longer create synthetic stay-like `occupancies`.
- Changed explicit payout create/link action to create or reuse a first-class `named_stays` row and map the finance row to that stay.
- Changed Booking.com finance cancellation status handling to mark linked named stays as `review_status = needs_review` instead of automatically cancelling user-owned stays.
- Updated invoice storage and handlers to persist `invoices.named_stay_id`, resolve deprecated `occupancy_id` through `occupancy_stay_migration_map` where possible, and list named stays as invoice candidates.
- Updated finance reset behavior to delete invoice rows/files linked through finance bookings or named-stay finance links while preserving named stays and manual external revenue.
- Added named-stay finance candidates endpoint and updated Booking payouts / Invoices UI to select and submit `named_stay_id`.
- Added a Booking payouts UI action for entering manual revenue on mapped external named stays.
- Added Stage 8 finance/invoice endpoint and DTO coverage to `spec/openapi.yaml`.

Verification commands:

- `go test ./...` from `backend/`: passed.
- `npm run type-check` from `frontend/`: passed.
- `npm run test` from `frontend/`: passed.
- `npm run build` from `frontend/`: passed.
- `ruby -e 'require "yaml"; YAML.load_file("spec/openapi.yaml"); puts "openapi yaml ok"'` from repo root: passed.

Notes:

- Raw booking blocks are not finance or invoice mapping targets.
- Legacy `finance_bookings.occupancy_id` and `invoices.occupancy_id` remain for compatibility and rollback until later cleanup stages.
- External stays still count in analytics only after Stage 9 moves analytics to named-stay semantics; Stage 8 only adds the finance/manual revenue data path.
- Production rollout remains blocked pending production audit/backfill approval; Stage 8 only implements and locally verifies the cutover behavior.

Remaining Stage 8 rollout/audit work:

- No additional local Stage 8 implementation work remains.
- Run the finance/invoice named-stay backfill migration in production only after production audit approval.
- Review production finance bookings and invoices that still cannot map to exactly one named stay.
- Validate production invoice numbers, invoice files, finance transactions, and payout mappings after relinking.
