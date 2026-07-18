# PMS 21 Stage 4 Local Verification

Date: 2026-07-13  
Scope: local/test verification of Stage 4 named-stay service layer.

## Implementation Summary

Stage 4 was implemented additively and legacy-safe:

- Added first-class named-stay store methods in `backend/internal/store/named_stays.go`.
- Added raw-block promotion to `named_stays` with `stay_source_links`.
- Added hard no-overlap enforcement through `named_stay_nights`.
- Preserved raw block nights after partial promotion.
- Added derived legacy `occupancies` compatibility writes and `occupancy_stay_migration_map` links.
- Reconciled legacy `occupancy_nights` for raw-block promotions so named compatibility rows win promoted nights and aggregate raw rows keep leftovers.
- Added Nuki generation badge state persistence on `named_stays`.
- Added Stage 4 API routes in `backend/internal/api/server.go` and handlers in `backend/internal/api/occupancy_named_stay_handlers.go`.
- Added focused tests in `backend/internal/store/named_stays_test.go`.

## Verified Behaviors

| Check | Result |
|---|---|
| Partial raw-block promotion creates a first-class named stay | Pass |
| Raw booking block night rows remain active after promotion | Pass |
| `stay_source_links` records active raw-to-stay linkage | Pass |
| `occupancy_stay_migration_map` records the legacy compatibility occupancy | Pass |
| Legacy `occupancy_nights` are rebuilt for promoted stays and raw leftovers | Pass |
| Editing a promoted stay keeps `stay_source_links` date range aligned | Pass |
| Active named-stay overlap is rejected | Pass |
| Cancelled named stays deactivate active named-stay nights | Pass |
| Reactivating into an occupied range is rejected | Pass |
| Default cleaning rules are applied by stay type | Pass |
| Nuki generation status/error can be persisted for calendar badges | Pass |

## Verification Command

Ran from `backend/`:

```sh
go test ./...
```

Result: pass.

## Constraints Still In Force

- No production gates were enabled.
- No production backfill apply was implemented or run.
- Legacy occupancy writes remain enabled.
- Downstream read cutovers remain disabled.
- Cleaning reconciliation still uses the broad-window legacy path until Stage 6.
- Nuki still generates through legacy occupancy compatibility until Stage 7.

## Conclusion

Stage 4 is locally implemented and verified. The current implementation stage can move to Stage 5: Occupancy Calendar API and UI.
