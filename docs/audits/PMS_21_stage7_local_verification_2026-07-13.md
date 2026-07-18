# PMS 21 Stage 7 Local Verification - 2026-07-13

Scope: Nuki cutover to named stays.

Implemented locally:

- Added migration `000034_nuki_named_stay_cutover` to backfill `nuki_access_codes.named_stay_id` and `nuki_guest_daily_entries.named_stay_id` from `occupancy_stay_migration_map` while preserving legacy `occupancy_id` values.
- Switched Nuki code generation/listing selection to active confirmed `named_stays` with `stay_type IN ('booking_com', 'external')`.
- Kept legacy occupancy revocation compatibility for existing active codes that are not yet relinked.
- Updated Nuki access code persistence and keypad linking to preserve and use `named_stay_id` where available.
- Updated guest check-in reconciliation and heatmap reads to resolve and persist `named_stay_id` while retaining `occupancy_id` for historical attribution.
- Updated Nuki upcoming-stays API and UI to use `stay_id` as the primary identity, with `occupancy_id` retained as deprecated compatibility output/input.
- Updated Nuki stay-name editing to mutate `named_stays.display_name` rather than legacy occupancy guest fields.
- Added Stage 7 Nuki endpoint schemas to `spec/openapi.yaml`.

Verification commands:

- `go test ./...` from `backend/`: passed.
- `npm run type-check` from `frontend/`: passed.
- `npm run test` from `frontend/`: passed.
- `npm run build` from `frontend/`: passed.

Notes:

- Raw booking blocks are not selected for Nuki generation or the Nuki upcoming-stays UI.
- `nuki_access_codes.occupancy_id` and `nuki_guest_daily_entries.occupancy_id` remain populated for compatibility and historical display until later cleanup stages.
- Production rollout remains blocked pending production audit/backfill approval; Stage 7 only implements and locally verifies the cutover behavior.

Remaining Stage 7 rollout/audit work:

- No additional local Stage 7 implementation work remains.

- Run the Nuki backfill migration in production only after production audit approval.
- Review any production Nuki codes or guest daily entries that still cannot map to exactly one named stay.
- Validate that existing active production PINs keep their PIN/external Nuki IDs after relinking.
