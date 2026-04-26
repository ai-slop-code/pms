# PMS Booking Statement Ingestion — Business Spec

## Purpose

Extend the Finance module so it can ingest **two complementary CSV exports** from Booking.com — the **Payout Info** file (already supported, cash-basis) and the **Statement** file (new, accrual / commercial). Merging both views per reservation gives the PMS the full booking lifecycle: created → confirmed → cancelled / paid.

This unlocks lead-time, cancellation, persons-mix and commission-trend analytics that the payout file alone cannot support.

---

## Strategic context

Today the PMS is anchored on **cash-basis** data: payouts → occupancies → analytics. The Statement file is **accrual-basis** — it represents the booking as a commercial event regardless of whether money has moved yet, and it carries fields the payout file does not (`Booked on`, `Status`, `Persons`, `Booker name`, `Original/Final amount`, `Commission %`, `Invoice number`, `Hotel id`, …).

Adding it is a meaningful step forward and worth doing properly:

1. The PMS becomes **lifecycle-aware** (booking, modification, cancellation, payout — all in one record).
2. Analytics gain a true booking-creation timeline → lead time, pacing, cancellation rate.
3. The same merge primitive (one canonical row, multiple source documents) generalises later to Airbnb, direct bookings, and a future Booking.com API integration.

---

## Scoping assumptions (locked unless explicitly revisited)

| Decision | Value |
|---|---|
| Channels | Booking.com only (Payout Info + Statement). Other channels out of scope for this spec. |
| Currency | EUR only. |
| Storage | One canonical row per reservation per property. Sources are tracked via flags + raw JSON for audit. |
| Merge key | `(property_id, reference_number)`. Both files share the reservation reference (`Reference number` / `Reservation number`). |
| File ingestion | Manual CSV upload (no API integration in this phase). |
| Permissions | Same role as today's payout upload (Finance). Revisit if PII expands. |

---

## Source files — column inventory

### Payout Info (existing, cash-basis)

```
Type, Reference number, Check-in, Checkout, Guest name,
Reservation status, Currency, Payment status, Amount, Commission,
Payments Service Fee, Net, Payout date, Payout ID
```

Date format: `"4 Sept 2025"` (English month abbreviations).
Excludes cancellations and unsold nights.

### Statement (new, accrual / commercial)

```
Reservation number, Invoice number, Booked on, Arrival, Departure,
Booker name, Guest name, Rooms, Persons, Room nights, Commission %,
Original amount, Final amount, Commission amount, Payment fee,
Status, Guest request, Currency, Hotel id, Property name, City, Country
```

Date format: `2025-12-31` (ISO) for `Arrival`/`Departure`; `2025-12-16T23:41:28` (ISO datetime) for `Booked on`.
Includes cancellations (`Status='CANCELLED'`, `Final amount=0`).
Includes one row per reservation, regardless of payout status.

### Field-level value mapping (proposed)

| Canonical column | Payout source | Statement source | Precedence on conflict |
|---|---|---|---|
| `reference_number` | `Reference number` | `Reservation number` | merge key |
| `booked_on` (DATETIME) | — | `Booked on` | statement only |
| `check_in_date` | `Check-in` | `Arrival` | **see Q5** |
| `check_out_date` | `Checkout` | `Departure` | **see Q5** |
| `room_nights` | `(checkout - checkin)` derived | `Room nights` | statement (handles partial cancels) |
| `persons` | — | `Persons` | statement only |
| `rooms` | — | `Rooms` | statement only |
| `guest_name` | `Guest name` | `Guest name` | **see Q5** |
| `booker_name` | — | `Booker name` | statement only |
| `guest_request` | — | `Guest request` | statement only |
| `status` | `Reservation status` (`ok`) | `Status` (`OK` / `CANCELLED` / …) | statement (richer states) |
| `payment_status` | `Payment status` (`by_booking`) | — | payout only |
| `amount_cents` (gross) | `Amount` | `Final amount` | **see Q5** |
| `original_amount_cents` | — | `Original amount` | statement only |
| `commission_cents` | `Commission` | `Commission amount` | **see Q5** |
| `commission_pct` | derived `Commission/Amount` | `Commission %` | statement (explicit) |
| `payment_service_fee_cents` | `Payments Service Fee` | `Payment fee` | **see Q5** |
| `net_cents` | `Net` | derived `Final − Commission − Payment fee` | payout (explicit) |
| `currency` | `Currency` | `Currency` | first writer wins |
| `payout_id` | `Payout ID` | — | payout only |
| `payout_date` | `Payout date` | — | payout only |
| `invoice_number` | — | `Invoice number` | statement only |
| `hotel_id` | — | `Hotel id` | statement only |
| `property_label` | — | `Property name`/`City`/`Country` | statement only (display only) |
| `has_payout_data` | true on payout import | — | OR |
| `has_statement_data` | — | true on statement import | OR |
| `raw_payout_row_json` | full row | — | last-write-wins |
| `raw_statement_row_json` | — | full row | last-write-wins |

---

## Recommended business logic

### Storage

**Option A (recommended): one canonical table.** Extend `finance_booking_payouts` with the new columns, add `has_payout_data` / `has_statement_data` flags and a separate raw JSON per source. Each upload upserts into the same row.

**Option B:** keep `finance_booking_payouts` untouched; add `finance_statements` joined by `(property_id, reference_number)`.

Recommendation: **A**, plus rename the table to `finance_bookings` so the name reflects the broader semantics.

### File-type detection

Auto-detect by header signature:

- `Payout date` + `Payout ID` ⇒ payout file
- `Booked on` + `Persons` + `Status` ⇒ statement file
- otherwise reject with a clear message

A single upload endpoint accepts either, and the parser dispatches.

### Merge rules (per row)

1. Find row by `(property_id, reference_number)`.
2. **No row** → INSERT, populate every column the file provides, set the matching source flag, store the raw row JSON.
3. **Row exists**:
   - For columns the new source has but the existing row doesn't → fill them.
   - For columns both sources have → apply the precedence table above.
   - Never overwrite a non-null value with NULL.
   - Set the matching source flag, replace the raw JSON for that source.
   - Append a row to a merge-audit log capturing changed fields, old/new, source, upload id.

### Cancellation handling

A statement row with `Status='CANCELLED'` and `Final amount=0`:

- Set canonical `status='cancelled'`.
- If the previous payout import had created an `occupancies` row, mark that occupancy cancelled (do **not** hard-delete) so historical analytics can still show it under cancellation metrics. Exclude cancelled occupancies from occupancy/ADR/RevPAR by default.
- See Q6 for confirmation.

### Idempotence

Re-uploading the same file is a **no-op** for unchanged rows. Changed fields are merged per the rules above and audited.

---

## Derived metrics enabled by the statement file

| Metric | Definition | Where it lives |
|---|---|---|
| Lead time per stay | `DATE(arrival) − DATE(booked_on)`, in days, computed on read | Demand tab (replaces ICS-derived proxy) |
| Cancellation rate | `cancelled / (cancelled + ok)` over the **booked-on** window (Q12) | Performance tab KPI |
| Persons distribution | count of stays grouped by `persons` | Demand tab — bar chart |
| ADR by persons | `Σ amount_cents / Σ room_nights` grouped by `persons`, active stays only | Demand tab — table or grouped bars |
| Average commission rate | weighted = `Σ commission_amount / Σ original_amount` | Performance tab KPI |
| Commission per stay | bar chart, one bar per stay, mirrors "Net per stay" | Performance tab |

All metrics use the same "active stay" filter as today (cancelled excluded), and respect the existing freshness disclaimer.

---

## Open questions — please fill in answers below

> Format: each question has an **A:** line. Replace `<TBD>` with your answer (free text). Questions tagged **(blocker)** must be answered before any code is written.

### File handling

**Q1 — Property identification.** Statement carries `Hotel id` (e.g. `13452548`). On upload should we (a) require admin to pick the property like today, (b) auto-route by `Hotel id` if a mapping exists, or (c) both, with a confirm step?

A: <TBD>

**Q2 — File-type detection.** Single upload endpoint with header-based auto-detect, or keep two distinct upload buttons (`Upload Payout` / `Upload Statement`)?

A: <TBD>

**Q3 — Encoding.** Sample payout shows mojibake (`VojtÄch`) while statement is clean UTF-8. Should we (a) require UTF-8 only and reject otherwise, (b) attempt cp1252→utf-8 fallback, or (c) accept and display as-is?

A: <TBD>

**Q4 — Date format stability.** Payout uses `"4 Sept 2025"`; statement uses `2025-12-31` and `2025-12-16T23:41:28`. Have you ever seen Slovak month names or US-format dates from Booking.com? If yes, list the variants we must accept.

A: <TBD>

### Merge semantics — precedence (blockers)

**Q5 — Per-field precedence on conflict between payout and statement.** Confirm or amend the table below.

| Field | Payout value | Statement value | Recommended winner | Your choice |
|---|---|---|---|---|
| `amount_cents` (gross) | `Amount` | `Final amount` | statement | <TBD> |
| `commission_cents` | `Commission` (negative sign in payout) | `Commission amount` | statement | <TBD> |
| `payment_service_fee_cents` | `Payments Service Fee` | `Payment fee` | statement | <TBD> |
| `check_in_date` / `check_out_date` | `Check-in` / `Checkout` | `Arrival` / `Departure` | statement | <TBD> |
| `guest_name` | `Guest name` | `Guest name` (separate `Booker name`) | statement | <TBD> |

A (any extra notes): <TBD>

**Q6 — Cancellation reconciliation (blocker).** If a payout was previously imported (creating an occupancy) and a statement later marks the reservation `CANCELLED`, what should happen?
- (a) hard-delete the occupancy
- (b) flag the occupancy `cancelled` and keep, exclude from occupancy/ADR/RevPAR by default
- (c) refuse the merge and ask the user to resolve

Recommendation: **(b)**.

A: <TBD>

**Q7 — Re-uploading the same file.** Idempotent silent re-import (recommended), or treat each upload as an immutable batch with its own audit row even when nothing changed?

A: <TBD>

### Schema / migration (blocker)

**Q8 — Rename `finance_booking_payouts` → `finance_bookings`?** Or keep the existing name and document that it now covers all booking states?

A: <TBD>

**Q9 — Historical backfill.** When this ships, should we (a) leave existing payout-only data as-is (statement-only fields stay NULL until next monthly statement), or (b) prompt admins to re-upload statements covering the same months?

A: <TBD>

**Q10 — PII / retention.** `Booker name`, `Guest request`, `Country` — are these subject to the same retention rules as today's `Guest name`? Confirm GDPR delete-on-request flow already covers them once added.

A: <TBD>

### Analytics scope (blocker for Q11–Q12)

**Q11 — Where does each new chart live?**
- Lead-time histogram → **(a)** replace the existing ICS-derived chart in Demand tab, or **(b)** add a new "Booking lead time (statement)" alongside?
- Persons distribution + ADR-by-persons → Demand tab? New tab?
- Commission rate trend + commission-per-stay → Performance tab next to "Net per stay"?
- Cancellation rate KPI → Outlook tab? Performance tab?

A: <TBD>

**Q12 — Definition of cancellation rate.** `cancelled / (cancelled + ok)` over the **booked-on window** (booking-cohort) or over the **arrival window** (arrival-cohort)? They tell very different stories.

Recommendation: booking-cohort for the trend chart, arrival-cohort for the operational "this month's cancels" KPI — show both.

A: <TBD>

**Q13 — Revenue treatment of cancellations.** Confirm cancellations are excluded from ADR/RevPAR (industry standard). Should the cancelled night be counted as **available** in RevPAR (i.e. zero revenue, full denominator) or excluded entirely?

A: <TBD>

### Operations

**Q14 — Permissions.** Same role as today's payout upload, or split (statement carries booker name + country)?

A: <TBD>

**Q15 — Validation & error UX.** On a malformed row: (a) reject the whole file, (b) skip the row and report it, (c) preview-then-commit with diff. What's today's payout flow doing? Should we keep parity?

A: <TBD>

**Q16 — Out-of-band statuses.** Sample shows `OK` and `CANCELLED`. Booking.com is also known to emit `MODIFIED`, `NO_SHOW`, `REFUSED_BY_HOTEL`. Whitelist + reject unknown, or store as-is and surface in UI?

A: <TBD>

### Long-term strategy

**Q17 — API ingestion roadmap.** Are CSV uploads a stop-gap toward a Booking.com API integration, or the long-term solution? Affects investment in upload UX (drag-drop, multi-file, scheduling).

A: <TBD>

**Q18 — Multi-channel.** Will Airbnb / Direct bookings need a similar dual-document model? If yes, the schema should generalise (`source_channel`, `source_document_type`) rather than hardcoding `payout` / `statement`.

A: <TBD>

---

## Suggested phased delivery

### Phase 1 — Ingestion + reconciliation (no UX rework)

- Migration extending the canonical table with the new columns + source flags + raw JSON per source.
- Statement parser, file-type auto-detect, merge upsert with precedence rules.
- Merge audit log.
- Cancellation flow (statement → existing occupancy).
- Tests:
  - statement-then-payout, payout-then-statement (both orders),
  - cancel-after-payout, payout-after-cancel,
  - idempotent re-upload,
  - corrupt encoding fallback,
  - unknown status handling,
  - negative-precedence cases (existing non-null never overwritten with NULL).

### Phase 2 — Analytics

- Cancellation rate KPI + trend.
- Lead-time histogram switched to / supplemented by statement data.
- Persons distribution + ADR-by-persons.
- Commission rate trend + commission-per-stay chart.
- Freshness disclaimer extended ("revenue through <payout date>; bookings through <last statement date>").

### Phase 3 — Polish

- Auto-route by `Hotel id`.
- Multi-file / multi-property single upload.
- Re-upload preview-and-diff UX.
- Reconciled-view export (CSV with both sources merged).

---

## Acceptance criteria (for the implementation agent)

A run is acceptable when:

1. Either CSV (payout or statement) can be uploaded through the existing Finance UI; file type is auto-detected.
2. After uploading both files for the same month in any order, every reservation has `has_payout_data = true` AND `has_statement_data = true` where applicable, with merged values per the precedence table.
3. Cancelled reservations from the statement file are visible in the Finance UI, excluded from occupancy/ADR/RevPAR, and counted in cancellation metrics.
4. New analytics (lead time, persons mix, ADR-by-persons, commission rate, commission per stay, cancellation rate) render with values that match hand-computed expectations on the September sample files.
5. Re-uploading either file is a no-op when content is unchanged; changes produce a merge-audit row.
6. All blockers in §Open questions have answers committed in this file before code is written.
