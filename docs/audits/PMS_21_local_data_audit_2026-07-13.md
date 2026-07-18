# PMS 21 Local Data Audit

Date: 2026-07-13  
Database: `data/pms.db`  
Scope: read-only local SQLite audit required before additive PMS 21 schema work.

## Schema State

| Metric | Value |
|---|---:|
| Applied schema migrations | 31 |
| Latest migration | `000031_ics_dtstamp` |
| Properties | 1 |
| Active occupancy sources | 1 |
| Disabled occupancy sources | 0 |
| Properties missing Booking.com ICS URL | 0 |

## Occupancy Classification Counts

| Source type | Status | Representation kind | Closure state | Superseded | Count |
|---|---|---|---|---:|---:|
| `booking_ics` | `deleted_from_source` | `legacy_generated_night` | | 0 | 198 |
| `booking_payout` | `active` | `named_stay` | | 0 | 118 |
| `booking_ics` | `active` | `named_stay` | | 0 | 45 |
| `booking_ics` | `deleted_from_source` | `unnamed_block` | | 0 | 17 |
| `booking_statement` | `active` | `named_stay` | | 0 | 11 |
| `booking_ics` | `active` | `unnamed_block` | | 0 | 8 |
| `manual` | `active` | `unnamed_block` | | 1 | 6 |
| `booking_ics` | `deleted_from_source` | `named_stay` | | 0 | 5 |
| `booking_ics` | `active` | `legacy_generated_night` | | 0 | 3 |
| `manual` | `active` | `unnamed_block` | | 0 | 2 |
| `booking_ics` | `deleted_from_source` | `external_sale` | `external_sale` | 0 | 1 |
| `booking_ics` | `deleted_from_source` | `manual_closure` | `closed` | 0 | 1 |
| `booking_ics` | `deleted_from_source` | `unnamed_block` | | 1 | 1 |
| `manual` | `deleted_from_source` | `unnamed_block` | | 1 | 1 |

## Candidate Counts

| Candidate class | Count |
|---|---:|
| Raw block candidate occupancy rows | 238 |
| Named-like candidate occupancy rows | 179 |
| Legacy generated night rows | 201 |
| Manual split rows | 0 |
| Active closed rows | 0 |
| External sale rows | 1 |
| Finance synthetic named-stay rows | 129 |

## Night-Level Checks

| Metric | Value |
|---|---:|
| Active `occupancy_nights`, property 1 | 22 |
| Duplicate active `occupancy_nights` groups | 0 |
| Overlapping active named-stay candidate pairs | 0 |
| Duplicate or overlapping active raw block pairs | 7 |

Representative active raw overlaps, IDs only:

| Property | Raw A | Raw B | A range | B range |
|---:|---:|---:|---|---|
| 1 | 76967 | 77012 | `2026-07-08T00:00:00Z` -> `2026-07-09T00:00:00Z` | `2026-07-08T00:00:00Z` -> `2026-07-09T00:00:00Z` |
| 1 | 75566 | 77034 | `2026-07-15T00:00:00Z` -> `2026-07-17T00:00:00Z` | `2026-07-16T00:00:00Z` -> `2026-07-17T00:00:00Z` |
| 1 | 11 | 76968 | `2026-07-30T00:00:00Z` -> `2026-07-31T00:00:00Z` | `2026-07-30T00:00:00Z` -> `2026-07-31T00:00:00Z` |
| 1 | 12 | 76982 | `2026-08-07T00:00:00Z` -> `2026-08-11T00:00:00Z` | `2026-08-07T00:00:00Z` -> `2026-08-08T00:00:00Z` |
| 1 | 12 | 76983 | `2026-08-07T00:00:00Z` -> `2026-08-11T00:00:00Z` | `2026-08-08T00:00:00Z` -> `2026-08-09T00:00:00Z` |

## Finance, Invoice, And Revenue Mapping

| Metric | Count |
|---|---:|
| Occupancies with `finance_booking_id` | 173 |
| Finance bookings total | 207 |
| Finance bookings with `occupancy_id` | 175 |
| Finance bookings unmatched | 32 |
| Invoices total | 0 |
| Invoices with `occupancy_id` | 0 |
| Invoices with finance booking link through occupancy | 0 |

Unmatched finance bookings are all currently cancelled/no-amount rows in the inspected sample. Later finance cutover must still surface them as unmatched/review rows rather than silently creating stays.

## Nuki Mapping

| Metric | Count |
|---|---:|
| Generated Nuki access codes | 3 |
| Revoked Nuki access codes | 47 |
| Future generated Nuki access codes | 0 |
| Nuki guest daily entries | 37 |
| Nuki codes without named-like occupancy mapping | 1 |

`nuki_guest_daily_entries` for property 1 span `2026-05-15` through `2026-07-04`.

## Cleaning Mapping

| Cleaning kind | Status | Missing Google event ID | Count |
|---|---|---:|---:|
| `named_stay` | `error` | 0 | 15 |
| `named_stay` | `removed` | 0 | 38 |
| `provisional_block` | `error` | 0 | 25 |

| Metric | Count |
|---|---:|
| Cleaning rows without upstream identity and occupancy mapping | 0 |
| Cleaning events without mappable occupancy or raw UID | 0 |

## Export And Frontend/API Consumers

| Metric | Count / status |
|---|---|
| Public occupancy export tokens | 0 |
| Frontend `occupancy_id` references | Present in dashboard, cleaning, payouts, invoices, messages, Nuki, and occupancy views/types |
| Export-token/n8n/curl UI references | Present in `OccupancyView.vue` and `OccupancySyncPanel.vue` |

## Risk Report

- Candidate raw blocks can be created from 238 legacy occupancy rows.
- Candidate named stays can be created from 179 named-like rows.
- No active named-stay overlap was detected in the local data.
- One `external_sale` row exists and requires classification/review in backfill.
- No active `closed` rows were found, so no local closure/off-market availability loss is currently detected.
- 129 finance-synthetic named-stay rows require review-required backfill handling if not matched to an existing named stay.
- 32 finance bookings are unmatched and must remain unmatched or become explicit review/create-link actions.
- One Nuki code cannot be mapped to a named-like occupancy by the simple local audit rule.
- Active overlapping raw blocks exist and provisional cleaning must coalesce them by property/date.
- Frontend and API consumers still depend on `occupancy_id`; no cutover flags should be enabled yet.
- Public export tokens are absent locally, but export-token UI and route code still exist and should be deprecated/removed only in the Stage 10 compatibility step.

## Stage Gate Conclusion

This local audit does not block additive Stage 1 schema implementation. It does block destructive cleanup and downstream cutovers until the listed finance, Nuki, raw-overlap, and compatibility risks are handled by the staged migration.
