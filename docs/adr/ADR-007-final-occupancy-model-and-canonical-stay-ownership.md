# ADR-007: Final occupancy model and canonical stay ownership

- **Status:** Accepted.
- **Date:** 2026-08-11.
- **Deciders:** Engineering + product.
- **Supersedes:** ADR-005 and ADR-006.

## Context

ADR-005 authorized a temporary compatibility window in which legacy
`occupancy_id` identities, export routes, migration-map lookups, and
integration fallbacks could coexist with PMS 21. ADR-006 allowed unmatched
canonical finance-booking rows during that transition. Those decisions were
appropriate for staged migration but are not the final architecture.

PMS 21 now defines distinct source, business, and availability concepts. The
remaining compatibility layer must be removed only through the eligibility,
observation, backup, and verification gates in
`spec/PMS_21_Legacy_Occupancy_Removal_Spec.md`. Accepting this ADR does not
assert that Release A, Release B, destructive cleanup, or any production
restore drill has run.

## Decision

The final model is:

- Booking.com ICS reconciliation owns `raw_booking_blocks` and
  `raw_booking_block_nights`.
- User/business stay truth lives in `named_stays` and
  `named_stay_nights`.
- Booking.com provenance is represented by `stay_source_links`.
- Non-stay closures live in `property_availability_blocks`.
- Nuki, cleaning, finance, invoices, messages, dashboard, and analytics use
  new-model identities only.
- Public occupancy export, occupancy API tokens, occupancy-as-stay routes,
  migration-map runtime lookups, and legacy identity aliases have no final
  replacement and are removed after the caller and cleanup gates pass.
- Every committed `finance_bookings` row has a same-property
  `named_stay_id`. Unmatched import input may remain in staging, rejection,
  preview, or audit evidence, but not as an ownerless canonical booking.
- Every invoice has a same-property `named_stay_id`; an optional finance link
  must identify the same stay.
- Existing integration and business history is preserved while legacy
  behavior-bearing IDs, columns, and tables are removed by a verified forward
  migration.

## Invariants

- No active request treats `occupancy_id` as an alias for a named-stay ID.
- Raw blocks never generate Nuki access or count as sold/revenue nights.
- `named_stay_nights` is the capacity and stay-night analytics truth.
- Runtime code does not use `occupancy_stay_migration_map` after compatibility
  removal.
- Same-property ownership and required new-model links are enforced in the
  final schema.
- Cleanup cannot proceed on the strength of this ADR alone; every gate in the
  PMS 21 cleanup specification and runbook remains mandatory.

## Consequences

- ADR-005 remains immutable historical evidence of the staged compatibility
  decision, but its release-cycle, per-area rollback, export-gate, and legacy
  fallback rules are no longer the target architecture.
- ADR-006 remains immutable historical evidence of transitional finance
  behavior, but its permission for unmatched canonical finance bookings is
  superseded.
- Releases A and B are non-destructive proof periods. Release C performs the
  destructive forward migration and includes Release D tooling/documentation
  retirement only after required diagnostics are captured.
- A pre-cleanup database plus its compatible image is the rollback unit.
  Historical down migrations and an old binary against the cleaned schema are
  unsupported.

## Operational Status

This ADR records architecture, not execution. Production Release A and Release
B windows, exception resolution, cleanup-readiness approval, restore-drill
evidence, and destructive migration evidence remain unrecorded until an
operator creates and approves the artifacts required by the active runbook.
