# ADR-003: Cleaning event ownership and date-scoped reconciliation

- **Status:** Accepted for PMS 21 staged migration.
- **Date:** 2026-07-13.
- **Deciders:** Engineering + product.
- **Supersedes:** n/a.

## Context

The current Google cleaning reconciliation scans a broad fixed window and derives both provisional and final cleaning events from `occupancies`. This causes unnecessary patching and keeps cleaning identity tied to the overloaded legacy table.

PMS 21 separates raw source blocks from named stays. Cleaning must therefore distinguish provisional raw-block placeholders from final named-stay cleaning truth.

## Decision

Use date-scoped desired-state reconciliation for cleaning.

Desired cleaning identities are deterministic:

- Provisional raw coverage checkout: `raw-provisional:{propertyID}:{checkoutDate}`.
- Final named stay checkout: `stay:{propertyID}:{namedStayID}:{checkoutDate}`.

Cleaning ownership rules:

- Active raw block nights with no active named stay covering that night create exactly one coalesced provisional checkout placeholder for that property checkout date.
- Provisional event title is exactly `Upratovanie`.
- Active named stays create final cleaning on check-out only when `cleaning_required = 1`.
- Full promotion removes intermediate provisional placeholders and leaves only the final named-stay checkout cleaning when required.
- Partial promotion keeps provisional placeholders for leftover raw nights.

Google reconciliation must only create, patch, or delete PMS-owned events. Ownership matching order is stored Google event ID, private extended properties, then conservative wording/date fallback for legacy PMS events in the configured calendar.

## Invariants

- Cleaning reconciliation operates on affected date ranges, including old and new checkout-placeholder dates when ranges move or shrink.
- Local `cleaning_calendar_events` rows preserve `google_event_id` across ownership migration.
- Desired state hashes are stored so no-op reconciles avoid Google patch calls.
- Non-PMS Google events are never modified.
- Final cleaning state comes from named stays; provisional raw events are operational hints only.

## Consequences

- `cleaning_calendar_events` gains nullable `named_stay_id`, `raw_booking_block_id`, deterministic `cleaning_identity`, desired hash, and last-seen metadata.
- `occupancy_id` remains nullable during compatibility and historical display.
- The Google client must support listing events over a date range before the broad-window reconciler is disabled.
