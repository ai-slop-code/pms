# ADR-004: Stay type reporting semantics

- **Status:** Accepted for PMS 21 staged migration.
- **Date:** 2026-07-13.
- **Deciders:** Engineering + product.
- **Supersedes:** n/a.

## Context

Legacy occupancy rows use closure labels, guest names, and source types to infer whether a range is a real stay, external sale, manual closure, maintenance, or source block. PMS 21 needs explicit stay-type semantics so analytics, availability, cleaning, Nuki, finance, and UI can make consistent decisions.

## Decision

`named_stays.stay_type` is required and has four values:

- `booking_com`: Booking.com guest stay.
- `external`: external/direct guest stay.
- `maintenance`: maintenance stay-like block.
- `personal_use`: owner or personal use stay-like block.

Default cleaning rules:

- `booking_com`: cleaning required by default.
- `external`: cleaning required by default.
- `maintenance`: cleaning not required by default.
- `personal_use`: cleaning not required by default.

Analytics and availability rules:

- Active confirmed `booking_com` named-stay nights count as sold/occupied nights.
- Active confirmed `external` nights count as sold/occupied only when linked finance data exists or manual revenue is entered.
- Active external stays without revenue still reduce bookable availability but do not increase sold-night, occupancy-rate numerator, ADR, RevPAR, or returning-guest revenue metrics.
- `maintenance` and `personal_use` reduce available/bookable nights but do not count as sold/occupied revenue nights.
- `review_status = needs_review` active stays reduce availability but do not count as sold/revenue until confirmed according to stay-type rules.
- `property_availability_blocks` reduce availability but never count as sold/revenue nights.
- Raw booking blocks never count as sold/revenue nights.

## Invariants

- Only active confirmed named stays can contribute sold/revenue nights.
- Cancelled or archived named stays must not leave active named-stay nights behind.
- Manual external revenue is stored on `named_stays` and survives finance resets.
- Revenue reporting uses named stays and finance links, not raw source blocks.

## Consequences

- Analytics must move from `occupancies` to `named_stays` and `named_stay_nights` before the raw block model can become primary.
- External stays without revenue need UI/reporting visibility as action-required rows.
- Legacy `closure_state = closed` rows are not silently converted into maintenance or personal-use stays.
