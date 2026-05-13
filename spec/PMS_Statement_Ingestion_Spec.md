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
| Channels | Booking.com only today, but the schema carries `source_channel` from day 1 (default `'booking_com'`) so future channels (Airbnb, direct) are an additive change, not a migration. |
| Currency | EUR only. Any row with `Currency != 'EUR'` is rejected at parse time. |
| Storage | One canonical row per reservation per property. Sources are tracked via flags + raw JSON for audit. Reference numbers are stored as `TEXT` (never numeric) to preserve any future leading-zero or alphanumeric formats. |
| Merge key | `(property_id, source_channel, reference_number)`. Booking.com files share the reservation reference (`Reference number` / `Reservation number`). |
| Property mapping | Admin picks the target property on every upload (Q1=a). On first upload for a property we also capture the observed `Hotel id` into `properties.booking_hotel_id` so Phase 3 auto-route is one config check away. Statements are filtered by this captured `Hotel id` — rows belonging to other hotels are skipped and counted separately in the import summary (see N1). |
| Sign convention | Costs (commission, payment service fee) stored as **positive** integers. Payout parser flips sign on import to match statement convention. |
| Time zone | `Booked on` is interpreted as `Europe/Bratislava` and stored as `TIMESTAMPTZ`. All lead-time / cancellation-window math is done on the resulting UTC instant. |
| File ingestion | Manual CSV upload. This is the **long-term solution**; Booking.com API is not on the roadmap (Q17). |
| Permissions | Same role as today's payout upload (Finance). PII added by the statement file (`Booker name`, `Guest request`, `Country`) is governed by the existing GDPR delete-on-request flow, **including redaction inside `raw_*_json` blobs**. |

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
| `source_channel` | `'booking_com'` (constant) | `'booking_com'` (constant) | constant |
| `status_bucket` (derived) | from `Reservation status` | from `Status` | computed on read: `active` / `cancelled` / `other` |
| `raw_payout_row_json` | full row | — | last-write-wins; PII redacted on GDPR delete |
| `raw_statement_row_json` | — | full row | last-write-wins; PII redacted on GDPR delete |

### `status_bucket` derivation rules

| Source value | Bucket |
|---|---|
| `OK` / `ok` | `active` |
| `CANCELLED` | `cancelled` |
| Anything else (`MODIFIED`, `NO_SHOW`, `REFUSED_BY_HOTEL`, future unknowns) | `other` |

Raw values are stored verbatim per Q16; the bucket is computed on read so adding new mappings does not require a backfill.

### Auxiliary table — `finance_imports`

Every upload writes one row regardless of outcome:

```
id (uuid)
property_id
source_type           -- 'payout' | 'statement'
source_channel        -- 'booking_com'
hotel_id              -- from statement header rows; NULL for payout
invoice_number        -- from statement; NULL for payout
period_start, period_end -- min/max arrival or payout date observed
uploaded_by, uploaded_at
file_sha256           -- for quick "is this the same file?" hint
row_count_total
row_count_inserted
row_count_updated
row_count_unchanged
row_count_skipped_other_hotel
row_count_rejected
```

This table is the audit anchor for both idempotence checks and the merge-audit log.

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

Idempotence is **row-hash-based**, not file-hash-based. For each parsed row we compute a stable hash over the canonical fields the row provides; if the hash matches what's already on the canonical row for that source, it counts as `unchanged`. This keeps re-uploads silent even when Booking re-exports the same logical data with cosmetic CSV differences (re-quoted, re-ordered columns, BOM toggled).

File-level `file_sha256` is recorded on `finance_imports` purely as a UI hint ("this exact file was already uploaded on …").

---

## Derived metrics enabled by the statement file

| Metric | Definition | Where it lives |
|---|---|---|
| Lead time per stay | `DATE(arrival) − DATE(booked_on AT TIME ZONE 'Europe/Bratislava')`, in days, computed on read | Demand tab. Replaces the ICS-derived series for Booking.com rows; ICS-derived series is retained for non-Booking sources so the chart never regresses (see Q11/N2). |
| Cancellation rate (trend, booking-cohort) | `cancelled / (cancelled + active)` grouped by `booked_on` month. `other` excluded from both numerator and denominator. | Performance tab KPI + trend chart |
| Cancellation rate (operational, arrival-cohort) | same numerator/denominator but grouped by `arrival` month | Performance tab KPI |
| Persons distribution | count of `active` stays grouped by `persons` (NULL/0 excluded) | Demand tab — bar chart |
| ADR by persons | `Σ final_amount_cents / Σ room_nights` grouped by `persons`, active stays only | Demand tab — table or grouped bars |
| Average commission rate | weighted = `Σ commission_amount / Σ final_amount` over `active` stays only — cancelled rows have `final_amount = 0` and would dilute the rate. | Performance tab KPI |
| Commission per stay | bar chart, one bar per `active` stay, mirrors "Net per stay" | Performance tab |

All metrics use the same "active stay" filter as today (cancelled and `other` excluded), and respect the existing freshness disclaimer.

### Bucket coverage in chart UX

For any time bucket where `has_statement_data = false` for **all** rows in the bucket, statement-derived metrics render as a `"no statement data — metric unavailable"` state instead of `0%` / empty bars. This avoids the misleading "0% cancellation" reading on months that pre-date the first statement upload (relevant because Q9 elects no historical backfill).

### Data sources for analytics

- **Occupancies remain payout-driven.** Statement-only rows (future arrivals not yet paid out) do **not** create `occupancies` rows. Analytics that need future-looking statement data (lead time, cancellation rate, persons mix) read directly from `finance_bookings`. This keeps occupancy/ADR/RevPAR stable and cash-basis as today.
- **Cancellation reconciliation** (Q6) updates an existing `occupancies` row only when one already exists from a prior payout import.

---

## Open questions — please fill in answers below

> Format: each question has an **A:** line. Replace `<TBD>` with your answer (free text). Questions tagged **(blocker)** must be answered before any code is written.

### File handling

**Q1 — Property identification.** Statement carries `Hotel id` (e.g. `13452548`). On upload should we (a) require admin to pick the property like today, (b) auto-route by `Hotel id` if a mapping exists, or (c) both, with a confirm step?

A: let's do a)

**Q2 — File-type detection.** Single upload endpoint with header-based auto-detect, or keep two distinct upload buttons (`Upload Payout` / `Upload Statement`)?

A: single upload

**Q3 — Encoding.** Sample payout shows mojibake (`VojtÄch`) while statement is clean UTF-8. Should we (a) require UTF-8 only and reject otherwise, (b) attempt cp1252→utf-8 fallback, or (c) accept and display as-is?

A: b)

**Q4 — Date format stability.** Payout uses `"4 Sept 2025"`; statement uses `2025-12-31` and `2025-12-16T23:41:28`. Have you ever seen Slovak month names or US-format dates from Booking.com? If yes, list the variants we must accept.

A: No

### Merge semantics — precedence (blockers)

**Q5 — Per-field precedence on conflict between payout and statement.** Confirm or amend the table below.

| Field | Payout value | Statement value | Recommended winner | Your choice |
|---|---|---|---|---|
| `amount_cents` (gross) | `Amount` | `Final amount` | statement | <TBD> |
| `commission_cents` | `Commission` (negative sign in payout) | `Commission amount` | statement | <TBD> |
| `payment_service_fee_cents` | `Payments Service Fee` | `Payment fee` | statement | <TBD> |
| `check_in_date` / `check_out_date` | `Check-in` / `Checkout` | `Arrival` / `Departure` | statement | <TBD> |
| `guest_name` | `Guest name` | `Guest name` (separate `Booker name`) | statement | <TBD> |

A (any extra notes): confirm

**Q6 — Cancellation reconciliation (blocker).** If a payout was previously imported (creating an occupancy) and a statement later marks the reservation `CANCELLED`, what should happen?
- (a) hard-delete the occupancy
- (b) flag the occupancy `cancelled` and keep, exclude from occupancy/ADR/RevPAR by default
- (c) refuse the merge and ask the user to resolve

Recommendation: **(b)**.

A: b)

**Q7 — Re-uploading the same file.** Idempotent silent re-import (recommended), or treat each upload as an immutable batch with its own audit row even when nothing changed?

A: Idempotent silent re-import

### Schema / migration (blocker)

**Q8 — Rename `finance_booking_payouts` → `finance_bookings`?** Or keep the existing name and document that it now covers all booking states?

A: rename

**Q9 — Historical backfill.** When this ships, should we (a) leave existing payout-only data as-is (statement-only fields stay NULL until next monthly statement), or (b) prompt admins to re-upload statements covering the same months?

A: a) leave as is

**Q10 — PII / retention.** `Booker name`, `Guest request`, `Country` — are these subject to the same retention rules as today's `Guest name`? Confirm GDPR delete-on-request flow already covers them once added.

A: same retention rules.

### Analytics scope (blocker for Q11–Q12)

**Q11 — Where does each new chart live?**
- Lead-time histogram → **(a)** replace the existing ICS-derived chart in Demand tab, or **(b)** add a new "Booking lead time (statement)" alongside?
- Persons distribution + ADR-by-persons → Demand tab? New tab?
- Commission rate trend + commission-per-stay → Performance tab next to "Net per stay"?
- Cancellation rate KPI → Outlook tab? Performance tab?

A: 
- lead-time histogram -> a)
- Persons distribution + ADR-by-persons -> demand tab
- Commission rate trend + commission-per-stay -> performance tab
- Cancellation rate KPI -> Performance tab

**Q12 — Definition of cancellation rate.** `cancelled / (cancelled + ok)` over the **booked-on window** (booking-cohort) or over the **arrival window** (arrival-cohort)? They tell very different stories.

Recommendation: booking-cohort for the trend chart, arrival-cohort for the operational "this month's cancels" KPI — show both.

A: implement recommendation.

**Q13 — Revenue treatment of cancellations.** Confirm cancellations are excluded from ADR/RevPAR (industry standard). Should the cancelled night be counted as **available** in RevPAR (i.e. zero revenue, full denominator) or excluded entirely?

A: excluded entirely

### Operations

**Q14 — Permissions.** Same role as today's payout upload, or split (statement carries booker name + country)?

A: same permissions

**Q15 — Validation & error UX.** On a malformed row: (a) reject the whole file, (b) skip the row and report it, (c) preview-then-commit with diff. What's today's payout flow doing? Should we keep parity?

A: c, keep parity

**Q16 — Out-of-band statuses.** Sample shows `OK` and `CANCELLED`. Booking.com is also known to emit `MODIFIED`, `NO_SHOW`, `REFUSED_BY_HOTEL`. Whitelist + reject unknown, or store as-is and surface in UI?

A: store as is

### Long-term strategy

**Q17 — API ingestion roadmap.** Are CSV uploads a stop-gap toward a Booking.com API integration, or the long-term solution? Affects investment in upload UX (drag-drop, multi-file, scheduling).

A: long term solution, we can' integrate to booking.com API

**Q18 — Multi-channel.** Will Airbnb / Direct bookings need a similar dual-document model? If yes, the schema should generalise (`source_channel`, `source_document_type`) rather than hardcoding `payout` / `statement`.

A: at some point we will do multi channel, tho not so soon. → schema therefore carries `source_channel` (default `'booking_com'`) from day 1; no changes to UI or analytics until a second channel actually arrives.

---

## Second-round questions (recommended defaults — flag exceptions only)

All defaults below are written into the spec body above and assumed locked unless the answer here is different. Push back on any line and I'll revise.

**N1 — Multi-hotel statement files.** Statement exports for accounts with >1 property typically contain rows for all hotels in one file. With Q1=manual property pick, what should we do with rows whose `Hotel id` differs from the selected property's captured `booking_hotel_id`?
- (a) reject the entire upload
- (b) split rows by hotel and ask the admin to confirm a mapping per hotel id
- (c) **filter** — process only rows matching the selected property, count the rest as `row_count_skipped_other_hotel` in the import summary, surface a warning

Default: **(c)**. Simplest, no surprises, no silent mis-attribution.

A: c)

**N2 — Lead-time chart shape.** Replacing the ICS-derived chart (Q11) with a Booking-only series would silently drop Airbnb/direct bookings from the lead-time view.
- (a) replace (current Q11 answer; regression for non-Booking)
- (b) two separate charts on the Demand tab
- (c) **one chart, two series**: "precise (statement)" + "approximate (calendar)" overlaid

Default: **(c)**.

A: c)

**N3 — `source_channel` column from day 1.** Cheap insurance against a painful migration when channel #2 lands.

Default: **yes, add it** with `DEFAULT 'booking_com'`.

A: yes

**N4 — Capture `Hotel id` on `properties` from first upload.** Adds `properties.booking_hotel_id NULL`. Phase 3 auto-route becomes a one-line check.

Default: **yes**.

A: yes

**N5 — `finance_imports` table from Phase 1.** Required for the audit log + idempotence machinery anyway; promoting it from "polish" to "foundation".

Default: **yes, Phase 1**.

A: yes

**N6 — Existing payout upload flow — preview-then-commit or upload-and-commit?** Q15 says "keep parity with payout flow". Need to verify what payout actually does today before locking the answer.
- If payout is upload-and-commit: keep that, add a per-row reject report and a post-import diff summary.
- If payout already previews: extend the same UX to statement.

Default: **I'll verify in code and propose; the answer here drives Phase 1 UX scope**.

A: Verify the defefault

**Resolution (FEAT-04, 2026-05-08):** Pre-FEAT-04 the payout flow was
**upload-and-commit** (single `POST .../booking-payouts/import`
multipart endpoint that parsed and persisted in one shot). Per the
user directive *"upgrade both"*, FEAT-04 introduced a unified
`POST .../finance/imports/preview` + `POST .../finance/imports/commit`
pair that handles **both** payout and statement uploads through the
same preview-then-commit UX. The legacy single-step endpoint has been
removed and the Finance page now renders a single
"Upload Booking.com CSV" button that auto-detects the format and opens
the merge-plan dialog.

**N7 — Bucketing of `MODIFIED` / `NO_SHOW` / `REFUSED_BY_HOTEL`.** With Q16=store-as-is, the bucketing is a read-time concern.
- (a) treat as `active` (counts as a denominated stay, dilutes cancellation rate)
- (b) **`other`** — excluded from cancellation-rate numerator and denominator, surfaced as a separate "non-standard outcomes" KPI
- (c) treat as `cancelled`

Default: **(b)**.

A: b)

**N8 — Future-arrival rows from statements.** A statement uploaded in October contains January arrivals. Should we create `occupancies` rows for them now?
- (a) yes — occupancy charts get future visibility
- (b) **no, occupancies stay payout-driven**; lead-time / cancellation / persons-mix analytics read from `finance_bookings` directly

Default: **(b)**. Keeps ADR/RevPAR cash-basis as today; avoids occupancy chart "jumping" when a statement is uploaded.

A: b)

**N9 — Cancellation timestamp.** Neither file carries the moment of cancellation. We can infer it as "first statement import where status flipped to CANCELLED" if needed for a "cancellations within 14 days of arrival" metric. Out of scope for Phase 2 unless you want it.

A: Cancelation time should not be counted from these. E.g if we see cancelation there, let's just do some fuzzy logic and pair the cancelation to data from ics, however I think further analysis on this is needed.

**N10 — Booker name surfacing in UI.** New PII field. Default: visible only on the booking detail view, omitted from list views and analytics.

A: agree.

**N11 — Cancellation-rate KPI window.** Performance tab KPI default window: last 12 months on `booked_on` for the trend, current month on `arrival` for the operational KPI. Adjust if existing Performance KPIs use a different window.

A: See my answere for N9.

**N12 — Occupancy ↔ booking link key.** For Q6 cancellation reconciliation we need a deterministic FK from `occupancies` to `finance_bookings`. Today's link to `finance_booking_payouts` is implicit (same `reference_number` + `property_id`). Phase 1 should formalise this as an explicit FK column on `occupancies`. Default: **add `occupancies.finance_booking_id` FK during the same migration that renames the table**.

A: Go by recommendation.

---

## Suggested phased delivery

### Phase 1 — Ingestion + reconciliation (no UX rework)

- Migration: rename `finance_booking_payouts` → `finance_bookings`; add canonical columns, `source_channel`, `has_payout_data` / `has_statement_data` flags, raw JSON per source, `properties.booking_hotel_id`, `occupancies.finance_booking_id` FK.
- New `finance_imports` table.
- Merge-audit log table (`finance_booking_merges`: booking_id, import_id, source_type, changed_fields_json, occurred_at).
- Statement parser, file-type auto-detect (header signature), merge upsert with precedence rules, sign normalisation, timezone normalisation, currency guard.
- Multi-hotel statement filtering + skip count (N1).
- Cancellation flow (statement → existing occupancy via FK).
- GDPR redaction extended to `raw_*_json` blobs.
- Tests:
  - statement-then-payout, payout-then-statement (both orders),
  - cancel-after-payout, payout-after-cancel,
  - idempotent re-upload (row-hash-based, including byte-different-but-logically-identical re-export),
  - corrupt encoding fallback (cp1252 → utf-8),
  - unknown status handling (`MODIFIED`, `NO_SHOW`, `REFUSED_BY_HOTEL`) → `status_bucket='other'`,
  - multi-hotel statement → only matching `Hotel id` rows ingested,
  - currency != EUR → row rejected with clear error,
  - sign convention → commission stored positive regardless of source,
  - negative-precedence cases (existing non-null never overwritten with NULL),
  - GDPR delete → canonical PII columns nulled AND `raw_*_json` PII redacted.

### Phase 2 — Analytics

- Cancellation rate KPI + trend (booking-cohort + arrival-cohort, per Q12).
- Lead-time histogram: dual-series (statement-precise + ICS-approximate), per N2.
- Persons distribution + ADR-by-persons.
- Commission rate trend + commission-per-stay chart (active stays only, per G8).
- "No statement data" rendering for buckets pre-dating first statement upload.
- Freshness disclaimer extended ("revenue through <payout date>; bookings through <last statement date>").

### Phase 3 — Polish

- Auto-route by `Hotel id`.
- Multi-file / multi-property single upload.
- Re-upload preview-and-diff UX.
- Reconciled-view export (CSV with both sources merged).

---

## Acceptance criteria (for the implementation agent)

A run is acceptable when:

1. Either CSV (payout or statement) can be uploaded through the existing Finance UI; file type is auto-detected by header signature.
2. After uploading both files for the same month in any order, every reservation has `has_payout_data = true` AND `has_statement_data = true` where applicable, with merged values per the precedence table and signs/timezone normalised.
3. Multi-hotel statement files ingest only rows matching the selected property's `booking_hotel_id`; the rest are reported as skipped in the import summary (N1).
4. Cancelled reservations from the statement file are visible in the Finance UI, the linked occupancy (if any) is flagged `cancelled` (not deleted), excluded from occupancy/ADR/RevPAR, and counted in cancellation metrics.
5. `MODIFIED` / `NO_SHOW` / `REFUSED_BY_HOTEL` rows ingest, are bucketed `other`, and are excluded from cancellation-rate numerator and denominator (N7).
6. New analytics (lead time dual-series, persons mix, ADR-by-persons, commission rate, commission per stay, cancellation rate booking-cohort + arrival-cohort) render with values that match hand-computed expectations on the September sample files; pre-statement buckets render the explicit "no statement data" state.
7. Re-uploading either file is a no-op when row hashes match; changes produce a merge-audit row and bump `row_count_updated` on `finance_imports`.
8. GDPR delete-on-request nulls canonical PII columns AND redacts PII fields inside `raw_*_json` for both sources.
9. All blockers in §Open questions and §Second-round questions have answers committed in this file before code is written.
