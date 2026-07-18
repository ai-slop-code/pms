# ADR-005: Occupancy ID compatibility window and public export retirement

- **Status:** Accepted for PMS 21 staged migration.
- **Date:** 2026-07-13.
- **Deciders:** Engineering + product.
- **Supersedes:** n/a.

## Context

Existing APIs, frontend views, Nuki codes, finance bookings, invoices, messages, and cleaning rows use `occupancy_id` as a stay identity. PMS 21 introduces separate identities for raw blocks, named stays, source links, and availability blocks. Removing legacy IDs immediately would break existing consumers and historical references.

Public occupancy export is no longer a target feature. Google Calendar events are the supported external calendar integration path.

## Decision

Keep old occupancy-as-stay APIs temporarily and introduce `occupancy_stay_migration_map` to translate legacy `occupancies.id` to one of:

- `raw_booking_block_id`
- `named_stay_id`
- `availability_block_id`
- `unmapped`

New APIs and frontend work use `stay_id`, `raw_booking_block_id`, and `availability_block_id` where they represent those concepts. Old endpoints are deprecated only after new read models are implemented and backfill/audit results are clean for the affected integration.

Public occupancy export is not replaced with export v2. During early additive stages, existing export behavior remains unchanged. During compatibility cleanup, export routes, token management, export-token storage, and n8n/curl guidance are deprecated or removed behind an explicit `occupancy_export_disabled` gate.

## Invariants

- Compatibility mapping preserves old IDs during staged migration.
- New business behavior must not depend on raw `occupancy_id` once a corresponding named stay exists.
- Legacy closures remain availability-blocking through availability blocks or verified compatibility reads.
- No old columns or legacy write paths are removed until at least one production release cycle has run with all cutover flags enabled and no unresolved migration conflicts.

## Consequences

- New schema remains additive at first.
- API contracts in `spec/openapi.yaml` must be updated before frontend migration.
- Feature gates default to safer legacy behavior until audit, backfill, and integration verification pass.
- Rollback can switch individual integrations back to legacy reads while retaining new tables and links for inspection.
