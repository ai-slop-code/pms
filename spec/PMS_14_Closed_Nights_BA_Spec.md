# PMS_14 — Closed & Externally-Sold Nights: Business Analyst View & Implementation Order

> Audience: product / property manager + implementing engineer.
> Scope: companion analysis for the v1.1 task **"Occupancy — manually
> classify a Booking.com block as 'closed' or 'externally sold'"**
> ([PMS_12](PMS_12_v1.1_Implementation_Plan.md#2-occupancy--manually-classify-a-bookingcom-block-as-closed-or-externally-sold)).
> This document is the *why* and the *order*; PMS_12 is the *what*.

---

## 1. Problem framing

### 1.1 What the user observes

Hosts on Booking.com routinely **close a date for sale** without a paying
Booking.com guest behind it. There are two operationally distinct
reasons this happens, and the published iCal feed flattens both into
the same `VEVENT`:

1. **Genuinely off the market** — the owner or family stays in the
   apartment for a few nights; the unit is taken offline for cleaning,
   repairs, deep maintenance, or a contractor visit; the host
   soft-blocks weekends ahead of a price change to avoid late bookings
   at the old rate.
2. **Sold through a different channel** — the operator already accepted
   a direct booking (phone, repeat guest, walk-in) or a reservation on
   another OTA (Airbnb, etc.) that PMS does not yet ingest, and blocks
   the date on Booking.com only to prevent a double-booking. The night
   *is* sold and *does* generate revenue; PMS just doesn't know about
   the payout because there is no Booking reservation behind it.

In the Booking.com extranet both actions look like a manual block.
**In the published iCal feed, however, both rows appear as a regular
`VEVENT`** with no flag that distinguishes them from each other or
from a guest reservation.

PMS imports the feed, normalises every event into the `occupancies` table,
and treats it as a sold night across every downstream module:

- Occupancy calendar shows the night as occupied.
- Analytics counts the night in `nights_sold` (numerator of occupancy
  rate) and ignores the absence of a payout when computing ADR / RevPAR.
- Nuki code generation may emit a code that is never used.

For case (1) this overstates occupancy. For case (2) it gets occupancy
right by accident, but ADR and RevPAR are still wrong because the
revenue from the off-platform booking is invisible to PMS.

### 1.2 Why this matters to the business

| Symptom | Concrete impact |
|---|---|
| Inflated **occupancy rate** | Pricing decisions look healthier than reality. Yield-management assumptions become wrong: "we sold 92 % so let's raise prices" when the true paid occupancy was 78 %. |
| Distorted **ADR** | ADR is `gross_revenue / matched_nights`. Closed nights sit in the denominator (`nights_sold`) but generate no payout, so they pull ADR down only if matching is loose, or — in the current code path — leave ADR unchanged but inflate occupancy. Either way the revenue-per-available-room (**RevPAR = ADR × occupancy**) is not internally consistent. |
| Misleading **demand** signals | The seasonality heatmap and DOW occupancy show "demand" peaks that are really maintenance windows. Marketing campaigns get misaimed. |
| Trust in the dashboard | Once an operator notices a single wrong number, every other number on the page becomes suspect. Closed nights are the most common, most visible source of that distrust. |

### 1.3 Why we cannot fix it at the source

- Booking.com does not expose the closure-vs-reservation distinction in
  the public iCal feed.
- The Booking.com Connectivity API would, but PMS's stated long-term
  ingestion strategy is **manual upload, no API integration**
  ([statement-ingestion spec](statement_processing/PMS_Statement_Ingestion_Spec.md), Q17).
- Heuristics ("event whose summary is `CLOSED - Not available`") work in
  some locales and break in others. The English heuristic specifically
  was tried by other PMS vendors and abandoned because hosts customise
  the block text.

The honest path is: **let the operator label the row inside PMS**, with
two distinct labels — one for each of the cases above.

---

## 2. Definitions

A small set of definitions that PMS_12 and the code can refer back to:

- **Closure**: the operator's act of marking a night as "not for sale".
  A closure is always *user-asserted*; the system never infers it.
- **Closed night**: a calendar night that, in the property timezone, has
  at least one closure overlapping it. Closed nights are **excluded
  from both numerator and denominator** of the occupancy rate.
- **External sale**: the operator's act of marking a Booking.com block
  as a real sold night that originated outside Booking, and recording
  the **net amount** received for that stay.
- **Externally-sold night**: a calendar night covered by a row labelled
  *external sale*. Externally-sold nights are counted as **sold** and
  as **available**; they contribute their pro-rata share of
  `external_net_amount` to gross revenue.
- **Bookable night**: an available night that is **not** closed.
  (Externally-sold nights are bookable — they were sold.)
- **Sold night**: a bookable night with either a non-cancelled,
  non-closed Booking-imported occupancy overlapping it, *or* an
  external-sale label.

The arithmetic that follows from those definitions:

$$ \text{occupancy rate} = \frac{\text{sold nights}}{\text{bookable nights}} $$

Closed nights drop out of both sides — exactly the behaviour the operator
expects from "I'm not selling that night". Externally-sold nights stay
in both sides and additionally feed gross revenue.

ADR keeps its existing definition (`gross / matched_paid_nights`) but the
`gross` term now also includes `external_net_amount` from externally-sold
rows, and `matched_paid_nights` includes their nights. RevPAR follows
ADR and the new occupancy definition automatically.

---

## 3. Stakeholder questions answered up-front

These are the questions a business analyst typically asks before
greenlighting the schema change. Answering them here saves a round-trip
during implementation.

### 3.1 Should closures and external sales be separate entities or flags on `occupancies`?

**Decision: flags on `occupancies`** (Phase 1). Both labels share the
same `closure_state` column with values `closed | external_sale | NULL`,
plus three columns dedicated to the external-sale case
(`external_net_amount`, `external_currency`, `external_channel`).

Pros:
- The data already exists in the table — every closure shows up as an
  imported ICS event today. Adding columns is one migration.
- One source of truth for "what's on the calendar that night".
- Re-import idempotency is preserved: ICS sync upserts on
  `(property_id, source_event_uid)` and we explicitly skip closure
  fields in the upsert (see PMS_12 backend bullet).

Cons (and how Phase 2 addresses them):
- Some closures have no upstream ICS row (e.g. the host blocks a night
  inside PMS without ever touching Booking.com). For those we need a
  synthetic row. Phase 2 introduces a `source_type = 'manual_closure'`
  occupancy that the operator creates from a date range. It still lives
  in `occupancies` so analytics queries don't fork.

### 3.2 Should closures be retroactive?

Yes. Operators discover the problem *after* the fact ("our March
occupancy looks wrong") so the UI must let them backfill closures for
past dates. Audit log captures who closed what and when.

### 3.3 What if the upstream event later disappears?

Booking.com sometimes drops cancelled/blocked events from the ICS feed.
Today the sync flips status to `deleted_from_source`. The closure flag
**must survive that transition**, otherwise analytics for a closed past
period would silently revert to the wrong numbers when the operator
removes the manual block in Booking. PMS_12 backend bullet enforces this.

### 3.4 What about Nuki?

Nuki code generation already keys off `status IN ('active', 'updated')`.
We extend the predicate to also require `closure_state IS DISTINCT FROM
'closed'`. **Closed** nights produce no Nuki code, no message, no
anything downstream. **Externally-sold** nights are real guests — but
those guests came through a different channel, so the operator hands
them an entry code through that channel's flow, not through PMS's
Booking-derived code generator. Phase 1 therefore also suppresses Nuki
for `closure_state = 'external_sale'`; if a future channel integration
needs the opposite behaviour we revisit then.

### 3.5 What about the iCal export PMS publishes?

The exported feed (`/api/properties/{id}/occupancy-export`) is consumed
by external tools that don't know about our labels. Decision: **still
export both** — they really are unavailable nights as far as a downstream
calendar is concerned — but tag them so a downstream tool that cares
can filter:

- Closed nights: `STATUS:CONFIRMED`, `CATEGORIES:PMS-CLOSURE`.
- Externally-sold nights: `STATUS:CONFIRMED`,
  `CATEGORIES:PMS-EXTERNAL-SALE` (no amount/channel leaks into the
  public feed).

(Phase 1.)

### 3.6 What about cleaning?

Cleaning currently triggers off occupancy end dates. A closure for
maintenance often *implies* a cleaning afterwards, but conflating the
two is wrong (owner stays don't trigger guest cleaning). Externally-sold
nights, on the other hand, **do** correspond to a real guest checkout
and should keep the existing cleaning trigger. Decision: cleaning logic
is unchanged in Phase 1 — it already keys off the underlying stay row,
which exists for both labels — and we explicitly skip the trigger only
for `closure_state = 'closed'`. We re-evaluate the maintenance case
after we have data.

### 3.7 Granularity: per-night vs per-stay?

Booking.com closures come as multi-night events. The operator's mental
model is a **range** ("close 3rd–5th"). Phase 1 stores the closure or
external-sale label on the imported stay row; if a stay spans both
real-guest and labelled nights (rare, only happens when the host edits
an existing stay), the operator splits it into two events in Booking.com
first. Phase 2 adds a date-range tool that can carve either label out of
any range.

### 3.8 What net amount is the operator expected to enter for an external sale?

**Net** in the same sense PMS uses everywhere else: the amount the
operator actually receives for the stay, after the third-party
platform's commission and after taxes that are remitted by the platform
on the operator's behalf, but **before** the operator's own income tax.
This matches how Booking payouts are stored, so the resulting ADR /
RevPAR figures stay comparable across channels.

The entered amount applies to the **whole stay** (not per-night). When
ADR and RevPAR aggregate, externally-sold nights contribute
`external_net_amount / nights_in_stay` per night, exactly the same
proration rule already used for matched Booking payouts.

### 3.9 Currency

The entered amount is interpreted in the property's default currency
unless the request body specifies otherwise. Mixed-currency reporting is
out of scope for v1.1; the API still records the currency so a future
FX-aware aggregation has the data it needs.

### 3.10 Can the same row be both closed and externally sold?

No. The two labels are mutually exclusive — `closure_state` holds at
most one of them. Switching labels goes through `reopen` first (or the
backend performs an in-place transition that emits both audit events).
This keeps every analytics query a clean two-way split.

---

## 4. Implementation order

The phases below are ordered by **value delivered per unit of risk**.
Phase 1 alone fixes the headline complaint and is low-risk; later phases
are optional and additive.

### Phase 1 — Closure & external-sale flags on imported stays (target: v1.1.0)

Goal: an operator can mark any imported occupancy as **closed** or as
**externally sold** with a net amount; analytics recompute correctly
for both.

1. **Migration `000019_occupancy_closure.up.sql`**
   - Add to `occupancies`:
     - `closure_state TEXT NULL CHECK (closure_state IN ('closed','external_sale') OR closure_state IS NULL)`
     - `closure_reason TEXT`, `closure_category TEXT`,
       `closed_by_user_id INTEGER`, `closed_at TEXT`
     - `external_net_amount NUMERIC NULL`, `external_currency TEXT NULL`,
       `external_channel TEXT NULL`
     - CHECK: the three `external_*` columns are NULL unless
       `closure_state = 'external_sale'`; `external_net_amount >= 0`
       when present.
   - Index `(property_id, closure_state)` to keep analytics scans cheap.

2. **Sync preservation**
   - Update the ICS upsert to omit *all* of the new columns from the
     conflict update set, so re-import never clears either label or the
     entered amount.

3. **Store helpers**
   - Replace `analyticsActiveStatus` with `analyticsBookableStatus`
     (`status IN ('active','updated') AND closure_state IS DISTINCT FROM 'closed'`)
     in every aggregation query.
   - `NightsSoldInRange` counts active stays plus rows with
     `closure_state = 'external_sale'`, minus rows with
     `closure_state = 'closed'`.
   - `GrossRevenueInRange` adds `COALESCE(external_net_amount, 0)` for
     externally-sold rows on top of the existing payout matching.
   - Add `CountClosedNightsInRange(propertyID, from, to)` and
     `CountExternalSaleNightsInRange(propertyID, from, to)`; switch
     `AvailableNightsInRange` to subtract only the closed ones from the
     calendar count.

4. **API**
   - `POST /api/properties/{id}/occupancies/{occupancyId}/close`
     `{ reason, category }` — body validation: `category` ∈
     `{owner_stay, maintenance, soft_block, other}`, `reason` ≤ 500 chars.
   - `POST /api/properties/{id}/occupancies/{occupancyId}/external-sale`
     `{ net_amount, currency?, channel?, reason? }` —
     `net_amount >= 0`; `channel` ∈ `{airbnb, direct, walk_in, other}`;
     `currency` defaults to property default; `reason` ≤ 500 chars.
   - `POST /api/properties/{id}/occupancies/{occupancyId}/reopen`
     clears either label and nulls the associated columns.
   - Audited as `occupancy_close`, `occupancy_mark_external_sale`,
     `occupancy_reopen`. The external-sale audit entry includes the
     amount, currency and channel for traceability.

5. **Frontend**
   - Per-stay overflow menu with **Mark closed**, **Mark as externally
     sold**, **Reopen** actions in stay list and calendar.
   - Closed nights rendered in neutral grey with a category badge.
   - Externally-sold nights rendered in an accent colour with a
     `€ <amount>` chip and channel label.
   - "Mark as externally sold" dialog: net amount (required), channel
     (select), note (textarea); inline validation for non-negative
     numeric input.
   - Performance KPIs in Analytics get an "(excl. closed)" footnote and
     two small chips showing **N closed nights** and **N externally-sold
     nights** in range.

6. **Tests**
   - Unit: `NightsSoldInRange`, `AvailableNightsInRange` and
     `GrossRevenueInRange` honour both labels.
   - Integration:
     - Closing a stay reduces both numerator and denominator and removes
       its (zero) revenue contribution.
     - Marking a stay externally sold leaves occupancy unchanged vs the
       pre-fix value and increases ADR / RevPAR by
       `external_net_amount / matched_nights_in_range`.
     - Reopening reverts both, in either order.
     - Reopening a stay that became `deleted_from_source` upstream still
       works for both labels.
     - Negative `net_amount` returns HTTP 400.
   - Frontend vitest: button visibility / dialog wiring / amount
     validation.

7. **Spec & docs**
   - Update PMS_02 (Occupancy + Analytics) with the new fields and
     endpoints.
   - Update PMS_04 metric definitions to reflect the new
     `nights_sold` and `gross_revenue` formulas.

### Phase 2 — Manual closures and external sales without an upstream event (target: v1.2.0)

Goal: operator can label a date range that has no Booking.com event,
e.g. a future block decided in PMS first, or a direct booking that
never touched Booking at all.

1. **Schema**: add `source_type` values `manual_closure` and
   `manual_external_sale`; no schema change required (`source_type` is
   already free-text).
2. **API**:
   - `POST /api/properties/{id}/closures`
     `{ start_date, end_date, category, reason }` creates a synthetic
     occupancy with `closure_state = 'closed'`,
     `source_event_uid = "manual:<uuid>"`, status `'active'`,
     raw_summary `"Manual closure"`.
   - `POST /api/properties/{id}/external-sales`
     `{ start_date, end_date, net_amount, currency?, channel?, reason? }`
     creates a synthetic occupancy with `closure_state = 'external_sale'`,
     `source_event_uid = "manual:<uuid>"`, status `'active'`,
     raw_summary `"External sale"`, and the three `external_*`
     columns populated.
   - ICS sync ignores rows where `source_type` ∈
     `{manual_closure, manual_external_sale}`.
3. **Frontend**: range-picker dialog above the calendar with a
   label-type toggle (closure / external sale); the external-sale tab
   exposes the same amount / channel / note inputs as the per-stay
   dialog.
4. **Export**: manual rows appear in the iCal export with the same
   `CATEGORIES:PMS-CLOSURE` / `CATEGORIES:PMS-EXTERNAL-SALE` tags as
   imported ones.

### Phase 3 — Closure templates & recurrence (target: opportunistic)

Goal: reduce friction for repeating patterns ("close every Monday in
February for renovations").

1. Predefined templates per property (`weekly_off`, `seasonal_break`).
2. UI to apply a template to a date range.
3. No new analytics behaviour — just shorthand for Phase 2 closures.

This phase is *only* worth doing if Phase 2 telemetry shows operators
creating closures with the same shape repeatedly.

### Phase 4 — Reporting drill-down (target: opportunistic)

Goal: surface closures and external sales as first-class analytics
signals.

1. New tile on Analytics → Performance: **Closed nights breakdown** by
   category, month-over-month.
2. New tile: **External sales breakdown** by channel, with revenue
   share next to Booking.com revenue, so the operator sees how much of
   the business runs off-platform.
3. Compare *theoretical* occupancy (counting closed nights as sold) vs
   *paid* occupancy (the current Phase-1 number) so operators can see
   how much of their downtime is voluntary.

Out of scope for v1.1; documented here so the schema choices in Phase 1
don't paint us into a corner.

---

## 5. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Operators misuse closures to "fix" cancellations or no-shows. | Audit log + read-only category in reports. Phase 4 breakdown surfaces over-closure. |
| Operators inflate ADR by entering wishful `external_net_amount`. | Audit log captures every value; Phase 4 breakdown shows external-sale revenue split out so the operator (and any reviewer) can sanity-check it. |
| Re-import accidentally clears closure flag or external amount. | Explicit upsert exclusion of *all* new columns + integration test for both labels. |
| Past analytics numbers move when an operator backfills a label. | Acceptable — the new number is the correct one. The change is logged in audit. We do **not** snapshot historical KPIs. |
| Performance regression from extra predicate. | Index on `(property_id, closure_state)`; the column is `NULL` for the vast majority of rows so the index stays selective. |
| Confusion between "closure", "external sale" and Booking's "cancellation". | Distinct value set on `closure_state`; cancellations remain governed by `status` not `closure_state`; UI uses different colour treatments and copy. |
| Currency mismatch between `external_net_amount` and matched Booking payouts in mixed-currency reporting. | v1.1 records the currency but reports assume the property default; future FX-aware aggregation has the data it needs. Documented in Q3.9. |

---

## 6. Definition of done (Phase 1)

- All Phase 1 acceptance criteria in
  [PMS_12 §2](PMS_12_v1.1_Implementation_Plan.md#2-occupancy--manually-classify-a-bookingcom-block-as-closed-or-externally-sold)
  pass.
- A property with at least one **closed** night shows different
  occupancy / ADR / RevPAR numbers on Analytics → Performance compared
  to before the closure.
- A property with at least one **externally-sold** night shows
  unchanged occupancy and *higher* ADR / RevPAR compared to before the
  label, with the increase equal to the entered net amount distributed
  pro-rata across the stay's nights.
- An operator can flip a stay through closed → open → externally sold
  → open → closed without any background sync rewriting their input or
  losing the entered amount until an explicit reopen.
- Documentation in `PMS_02` and `PMS_04` reflects the new metric
  definitions verbatim, including the `external_net_amount`
  contribution to `gross_revenue`.
