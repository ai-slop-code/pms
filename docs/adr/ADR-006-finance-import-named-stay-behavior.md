# ADR-006: Finance import behavior for unmatched stays

- **Status:** Accepted for PMS 21 staged migration.
- **Date:** 2026-07-13.
- **Deciders:** Engineering + product.
- **Supersedes:** n/a.

## Context

Finance payout and statement imports currently link to `occupancies` and can create synthetic stay-like occupancy rows. Under PMS 21, finance must not silently create or mutate business stays because named stays are user/business-owned truth.

Booking.com finance and statement status can also indicate cancellation or no-show after a named stay exists. Automatically changing stay status from imported finance data would violate the ownership boundary.

## Decision

Finance mappings use `finance_bookings.named_stay_id` as the canonical stay link.

Import and matching behavior:

- If finance data exactly matches one confirmed named stay by deterministic reference or date/name rules, link it.
- If no exact named stay exists, leave the finance booking unmatched and show a create/link action.
- New finance imports must not silently create legacy `occupancies` or named stays.
- Existing legacy synthetic finance occupancies may be backfilled as `named_stays` only to preserve production history, with `stay_type = booking_com`, `source_channel = legacy_finance_import`, `review_status = needs_review`, and `review_reason = synthetic_finance_occupancy` when no exact named stay exists.
- Review-required synthetic finance stays remain availability-blocking if active but are excluded from sold-night/revenue KPIs until confirmed according to stay-type rules.

Status behavior:

- Active, OK, and modified Booking.com rows keep linked named stays active.
- Cancelled finance/statement rows create a user-confirmation review action before changing `named_stays.status`.
- No-show and non-refundable cancellation data maps to `named_stays.stay_outcome` only when confirmed or already explicitly marked.

## Invariants

- Finance may suggest or link named stays but must not silently overwrite user-created stay truth.
- Finance reset/import reset clears finance-derived links without deleting named stays.
- Manual external revenue on named stays survives finance resets.
- Invoices and payout mapping move to named stays while retaining legacy `occupancy_id` compatibility until cutover is complete.

## Consequences

- Finance matching, reset, invoice candidates, payout display, and manual mapping UI need named-stay support before finance cutover.
- Cancellation review workflow is required before imported cancellation data can change stay status.
- Backfill must report ambiguous finance/invoice mappings and synthetic finance rows requiring review.
