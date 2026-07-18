# PMS 21 Stage 11 Local Verification - 2026-07-14

## Scope

- Added default-off `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` cleanup gate.
- Wired the gate into server config and `store.Store`.
- When enabled, finance payout/statement date matching no longer creates new synthetic legacy `occupancies` rows when no legacy match exists, and the legacy generic-ICS supersede helper becomes a no-op.
- When enabled, named-stay create/update/status workflows no longer write derived compatibility `occupancies` rows or new compatibility migration-map rows.
- Kept the default behavior unchanged because production audit/backfill approval and the required release-cycle cleanup window are not complete.

## Verification

| Check | Result |
| --- | --- |
| `go test ./internal/store` from `backend/` | Passed |
| `go test ./...` from `backend/` | Passed |

## Notes

- This is non-destructive Stage 11 cleanup preparation, not hard cleanup.
- No legacy tables, columns, routes, or token storage were dropped.
- `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` must remain disabled in production until production backfill/audit approval and at least one production release cycle with cutover behavior are complete.
- Full removal of old write paths and obsolete `occupancies` fields remains blocked by the Stage 11 production gates.
