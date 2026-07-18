# ADR-002: Raw booking blocks are sync-owned and named stays are business-owned

- **Status:** Accepted for PMS 21 staged migration.
- **Date:** 2026-07-13.
- **Deciders:** Engineering + product.
- **Supersedes:** n/a.

## Context

Booking.com ICS events may represent a continuous blocked date range rather than one real customer stay. The legacy `occupancies` table currently mixes raw source blocks, user-named stays, closures, finance-derived synthetic rows, analytics source data, Nuki source data, payout mapping, and cleaning state.

This makes source ownership ambiguous: an upstream ICS disappearance can mark stay-like rows as deleted, while finance and Nuki can still treat raw blocks as business stays.

## Decision

Introduce first-class domain objects alongside the legacy tables:

- `raw_booking_blocks`: synced Booking.com ICS block ranges, owned only by sync.
- `raw_booking_block_nights`: date coverage for raw blocks, allowed to overlap named stays and other raw blocks.
- `named_stays`: user/business-owned stays and stay-like uses.
- `named_stay_nights`: active named stay capacity, with one active named stay per property night.
- `stay_source_links`: relationship and conflict state between named stays and raw source coverage.
- `property_availability_blocks`: non-stay availability reductions such as true closure/off-market periods.
- `occupancy_stay_migration_map`: compatibility mapping from old `occupancies.id` to the new model.

Booking.com ICS sync may create, update, or mark raw blocks as `deleted_from_source`. It must not resize, rename, reclassify, delete, cancel, archive, or otherwise mutate `named_stays` business truth.

## Invariants

- Date ranges use check-in inclusive and check-out exclusive semantics.
- Raw booking blocks are visible operational source data, not analytics, Nuki, payout, invoice, message, or final-cleaning truth.
- Named stays are the source of truth for analytics, Nuki, payout mapping, invoices, messages, and final cleaning state.
- Raw source loss is represented as source-link `source_deleted` or `conflict`, surfaced to users, and auto-cleared only when active raw coverage again covers the linked range.
- Legacy closure/off-market nights must remain availability-blocking until replaced by verified availability blocks or a compatibility read model.

## Consequences

- The migration is additive first. Legacy `occupancies` writes remain during early stages for compatibility.
- New named-stay operations dual-write derived legacy occupancy rows until downstream consumers have cut over or `occupancy_legacy_write_disabled` is enabled.
- Backfill and sync must be idempotent and resumable.
- UI and analytics must explicitly distinguish raw source coverage from named business stays.
