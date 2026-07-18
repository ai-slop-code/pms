# PMS 21 Stage 5 Local Verification - 2026-07-13

Scope: Occupancy calendar API and UI local implementation.

Implemented locally:

- Added `spec/openapi.yaml` contracts for raw booking blocks, named stays, availability blocks, availability-block mutation, promotion, named-stay mutation responses, cleaning status summaries, and the combined occupancy calendar endpoint.
- Regenerated frontend OpenAPI types in `frontend/src/api/types/generated.ts`.
- Added backend combined calendar read model in `backend/internal/store/stay_calendar.go`.
- Added read endpoints:
  - `GET /api/properties/{id}/booking-blocks?month=YYYY-MM`
  - `GET /api/properties/{id}/stays?month=YYYY-MM`
  - `GET /api/properties/{id}/availability-blocks?month=YYYY-MM`
  - `GET /api/properties/{id}/occupancy-calendar?month=YYYY-MM`
- Added availability-block mutation endpoints:
  - `POST /api/properties/{id}/availability-blocks`
  - `PATCH /api/properties/{id}/availability-blocks/{blockId}`
- Updated `OccupancyView.vue` and `OccupancyCalendar.vue` to load/render raw blocks, named stays, availability blocks, cleaning status, raw-source warnings, Nuki error badges, and diagonal empty cells.
- Added raw-block promotion from the combined calendar UI through the existing Stage 4 promotion endpoint.
- Added empty-night/manual named-stay creation from the combined calendar UI through `POST /api/properties/{id}/stays`.
- Added availability-block create/edit UI from the combined calendar details dialog.
- Added frontend unit tests for combined calendar badges and empty-cell action emission.
- Kept legacy list and sync/export tabs unchanged for compatibility.
- Dashboard widgets remain on legacy compatibility data, which is allowed until the Stage 9 cutover.

Verification commands:

- `go test ./...` from `backend/`: passed.
- `npm run type-check` from `frontend/`: passed.
- `npm run test` from `frontend/`: passed.
- `npm run build` from `frontend/`: passed.

Notes:

- Stage 5 is still additive. It does not enable production rollout, production backfill apply, downstream integration cutovers, or legacy occupancy write disablement.
- The new calendar UI reads the combined model directly; old occupancy-as-stay actions remain available only in the legacy list tab and legacy compatibility paths.

Stage 5 coverage check:

| Requirement | Local status | Notes |
| --- | --- | --- |
| OpenAPI contract before frontend implementation | Covered | `spec/openapi.yaml` now includes raw block, named stay, availability block, availability-block mutation, cleaning status summary, promotion, and combined calendar schemas/endpoints; generated frontend OpenAPI types were refreshed. |
| Combined calendar DTO | Covered | DTO includes raw blocks, named stays, availability blocks, cleaning event summaries, Nuki badge state, and source-link conflict/source-deleted status. |
| Raw vs named calendar rendering | Covered | Calendar cells render raw badges separately from named stay badges; duplicate raw blocks on the same date are coalesced into one raw chip with a count. |
| Empty diagonal cells | Covered | Empty nights in the combined calendar view use a diagonal background style. |
| Availability block rendering and mutation | Covered | Availability blocks render as distinct blocked/off-market entries; create/update endpoints and calendar UI are implemented. |
| Nuki generation failure badge | Covered | Named stays with `nuki_generation_status = error` show a cell badge and row state. |
| Raw-source warning badge | Covered | Active named stays with source-link `conflict` or `source_deleted` show a raw-source issue badge. |
| Promotion from raw block | Covered | Raw-block details allow promotion through `POST /booking-blocks/{blockId}/promote`. |
| Empty-night named stay creation | Covered | Empty cells are clickable in the combined calendar and can create external, maintenance, personal-use, or manually confirmed Booking.com named stays. |
| Dashboard upcoming/check-in widgets | Deferred by design | Dashboard remains on legacy compatibility data until the Stage 9 dashboard/analytics/message cutover. |
| Old list/sync tabs retained | Covered | Legacy list and sync/export tabs remain available during transition. |

Stage 5 downstream notes:

- Stage 6 still owns rewriting cleaning reconciliation to date-scoped idempotent Google operations; Stage 5 only exposes current local cleaning event status in the combined calendar model.
- Dashboard remains on legacy compatibility data until Stage 9, which is one of the allowed Stage 5 outcomes.
- Extend OpenAPI coverage for downstream dashboard, messages, Nuki, payout, invoice, analytics, and cleaning-calendar DTOs in their respective cutover stages.

Test coverage added/updated:

- `backend/internal/store/stay_calendar_test.go`: combined calendar read model, cleaning status attachment, availability-block overlap rejection, invalid month validation.
- `frontend/src/views/occupancy/OccupancyCalendar.spec.ts`: raw/named/availability/source/Nuki/cleaning badge rendering and empty-cell action emission.
- `frontend/src/views/OccupancyView.spec.ts`: initial calendar load now expects the Stage 5 `/occupancy-calendar` endpoint.
