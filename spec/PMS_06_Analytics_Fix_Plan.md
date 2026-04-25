# PMS — Analytics Module Fix Plan

_Date: 2026-04-22_
_Scope: UX polish + data-integrity guards for the Analytics module shipped under `PMS_05_Analytics_Module_Spec.md`._

## 0. Goals

1. Make the Analytics UI legible at a glance (terminology, tab/button affordance, date-picker consistency with the rest of the app).
2. Stop rendering misleading charts when the underlying data window is empty or partial (no flat-line plots, no negative "net per stay" when finance data is missing).
3. Impose a consistent **Money → Occupancy → Cleaning** hierarchy on the Performance tab.
4. Tighten the Demand tab (bucket labels, day-of-week labels, gap/orphan context, returning-guest Top-5 preview).

No backend contract changes are required for most items; the work is predominantly frontend. Three small backend additions are called out in §F.

---

## A. Global UI fixes (apply to all three tabs)

### A1. Terminology legend / glossary
- Add a collapsible **"What do these terms mean?"** panel at the top of `AnalyticsView.vue`, persistent across tabs, collapsed by default.
- Contents (single-sentence definitions, plain English):
  - **ADR** — Average Daily Rate = gross revenue ÷ nights sold.
  - **RevPAR** — Revenue Per Available Night = gross revenue ÷ available nights.
  - **Occupancy rate** — nights sold ÷ available nights.
  - **Effective take rate** — (commission + payment fees) ÷ gross.
  - **Confirmed vs. estimated revenue** — confirmed = payouts already matched to stays in the window; estimated = remaining unmatched unsold nights valued at the trailing 90-day ADR.
  - **Pacing / booking pace** — cumulative number of reservations received for a target window, plotted by booking date.
  - **Gap night** — a vacant night sandwiched between two occupied nights.
  - **Orphan midweek night** — a vacant midweek night (Tue–Thu) adjacent to occupied weekend nights.
- Implementation: single `<details>` element or a dedicated `<section class="glossary">`; no new dependency.

### A2. Tab buttons have invisible background
- Current: inactive tab buttons render with `background: #fff` on a white card → look like they are missing.
- Fix: give inactive tabs a visible light-grey surface (`background: var(--color-surface-muted)` or `#f1f5f9`) with a 1 px border; active tab keeps the solid brand-blue fill.
- Apply the same treatment to any other inline-tab controls on the view (e.g. segmented controls).
- Also audit the page for other buttons affected by the same white-on-white issue (e.g. "Prev / Next" in the returning-guests dialog, "Load more" anywhere).

### A3. Date-picker alignment with Finance module
- Reuse the exact markup/styles used in `FinanceView.vue` for From / To date inputs and the "Year" select — wrap the controls in the same grid/flex container, use identical input heights and gap.
- Applies to Performance tab and Demand tab. Outlook tab has no date picker (window is always the 90-day rolling horizon).
- Extract the block into a small local component (`<AnalyticsDateRange>`) if the markup is repeated, or copy the `.filter-row` class convention used in Finance. Zero new dependencies.

---

## B. Outlook tab

### B1. Booking-pace chart — label, axes, empty state
- Today it is a single blue line in a white box with no context.
- Fix:
  - Add a chart title: **"Booking pace — cumulative reservations received"** plus a sub-title line: _"target window: next 90 days"_.
  - Show x-axis tick labels (booking date, every ~7 days) and y-axis max value.
  - Add a legend entry ("This 90 d"). When LY comparison becomes available (already modelled in backend) render a second line with the label "Same window last year".
  - **Empty-state**: if `pacing_series.length === 0` or all counts are zero, render a muted "Not enough booking activity in this window yet." placeholder instead of a blank SVG.

### B2. Unsold nights — switch from per-night list to date-range roll-up
- Current: one row per unsold night (noisy, repetitive).
- New: roll adjacent unsold nights into ranges and show at most **5 ranges**, sorted by date ascending.
  - Columns: **From**, **To**, **Nights**, **Previous guest → Next guest** (display "—" when unknown).
  - A single isolated night renders as `From = To` with `Nights = 1`.
  - Backend already returns `unsold_nights[]` (date + prev/next guest). Do the roll-up in the frontend: walk the sorted list, start a new range whenever the current date is not `prev + 1 day` OR when prev/next guest context changes.
  - If more than 5 ranges exist, add a muted footer: _"+N more ranges…"_ (no expansion required in this iteration).

### B3. Remove "New bookings" 7-day mini-bars
- The metric (count of `occupancies.imported_at` per day, last 7 days) is ambiguous for an operator — it conflates fresh bookings, updates, and cancellations from the ICS feed and is easily misunderstood.
- Action:
  - Frontend: delete the `new_bookings` card/section from Outlook.
  - Backend: keep the field in the JSON response for now (marked `deprecated: true` in a comment) so we don't break the contract; remove from the spec in a follow-up once all clients drop it. **No DB or SQL change.**
- What the metric _was_ (for the user's benefit):
  > "New bookings / day" counted rows in `occupancies` whose `imported_at` fell within the last N days, grouped per calendar day. It represented how many reservation rows the ICS importer touched that day — which is a mixture of newly-arrived reservations, date-shifted reservations, and cancellations flipping back to active. Not a pure "new bookings taken today" counter and therefore not actionable.

---

## C. Performance tab

### C1. Section ordering — Money → Occupancy → Cleaning
Reorder the Performance tab into three clearly labelled blocks, top-to-bottom:

1. **Money** (must appear first)
   1. Operating cashflow tiles (currently labelled "Yearly finance" — rename to **"Operating cashflow"** and move to the top of the tab).
   2. KPI grid: Gross, Net, Commission, Payment fees, Effective take rate, ADR, RevPAR, Revenue vs. prior period (if YoY data).
   3. Net-per-stay chart (see C5).
2. **Occupancy**
   1. Headline KPIs: Occupancy rate, Nights sold, Available nights.
   2. Monthly trend (see C3).
   3. Seasonality heatmap (see C4).
   4. Day-of-week occupancy (see C6).
3. **Cleaning**
   1. Yearly cleaning counts (migrated bar chart, last).

Introduce three `<h2>` section headings with consistent styling (e.g. a small coloured accent bar) so the hierarchy is obvious without reading.

### C2. Date switcher — reuse Finance style
- Covered by §A3 — apply the same component here.

### C3. Monthly trend — don't stretch when data is sparse
- Current: the SVG viewport is 100 % of the card width regardless of point count, which produces the "flat line then hockey stick" look when only a handful of months have data.
- Fix:
  - Only render data points whose month ≥ earliest month with non-zero `gross_cents` OR non-zero `nights_sold`. Everything earlier is suppressed (not plotted as zero).
  - Cap the SVG width to `points × 48 px` (or similar) with a `max-width: 100%`. Left-align the chart so it looks proportional on small datasets.
  - If fewer than 2 usable points remain, show **"Not enough history for a trend yet."** placeholder.
  - Keep axes clean: tick label only every 2–3 months when there are >12 points.

### C4. Seasonality heatmap — show ISO week numbers
- Add an x-axis strip beneath the heatmap with the ISO week number for every column (or every 4th column to reduce noise, with minor tick marks between).
- Keep the existing year-axis on the left.
- Add a colour-scale legend on the right: `0 %  ─  min ─── max  100 %`.

### C5. Net-per-stay — chart instead of table, skip rows with no finance
- Replace the current `<table>` with a horizontal bar chart (hand-rolled SVG, same approach used elsewhere).
  - X-axis: net amount (EUR).
  - Y-axis: stays ordered by check-in date; label = "dd Mon — guest initials or occupancy id".
  - Colour bars green when net ≥ 0, amber when 0 > net ≥ -X, red when below.
- **Drop any stay whose finance data is absent** — specifically rows where `gross_cents == 0` AND no matched payout exists. The current behaviour surfaces negative nets like `-€25` purely because the cleaner fee was recorded without a payout yet, which is misleading.
  - Guard in the backend (preferred): in `ListNetPerStay`, filter out rows where `gross_cents = 0 AND commission_cents = 0 AND payment_fees_cents = 0` (i.e. no finance data at all). Keep the cleaner fee deduction only for stays that _do_ have payout data.
  - Alternative frontend guard if backend change is deferred: filter in the view before rendering.

### C6. Day-of-week occupancy — start on Monday (configurable)
- Default: week starts on **Monday** (the rest of the app assumes Mon–Sun, and Central European convention).
- Add a per-property preference `week_starts_on` with values `monday` | `sunday`, default `monday`. Plumbing:
  - Backend: extend the `properties` table via a new migration (default `'monday'`), expose the field on `GET /properties/:id`, accept it in property update payload.
  - Frontend: surface a "Week starts on" radio in the Property form view.
  - Analytics: read `propertyStore.current.week_starts_on` and rotate the DOW array before rendering. Do **not** re-query the backend; the existing `dow` 0–6 values (ISO: 1=Mon … 7=Sun) just need re-labelling.
- Label array (Monday default): `['Mon','Tue','Wed','Thu','Fri','Sat','Sun']`.
- Use the same array in §D2 (ADR by DOW) and §D3 (gap / orphan day-of-week annotation).

---

## D. Demand tab

### D1. Date picker alignment
- Covered by §A3.

### D2. Lead-time & Length-of-stay — explicit units on every bucket label
- Current bucket labels render as bare numbers (`0-3`, `4-14`, `1`, `2`, …). Users can't tell what unit this is.
- New labels:
  - Lead time: `0–3 days`, `4–14 days`, `15–45 days`, `46–90 days`, `91+ days`.
  - Length of stay: `1 night`, `2 nights`, `3 nights`, `4–5 nights`, `6–7 nights`, `8–14 nights`, `15+ nights`.
- Also set chart titles: **"Lead time (days between booking and arrival)"** and **"Length of stay (nights per reservation)"**.
- Axis label on the y-axis: "Reservations".

### D3. ADR by day of week — show day name, not number
- Map `dow` integers to localised short names using the same Mon-first array from §C6 (e.g. `Mon`, `Tue`, …). Display below each bar.
- Chart title: **"ADR by day of week"**, subtitle: _"Gross ÷ nights sold, per weekday"_.

### D4. Gap nights & Orphan midweek nights — annotate with weekday
- Each row/card currently shows only a date.
- Append the weekday name to the date column (`2026-05-14 (Thu)`) using the same weekday array.
- For gap nights, additionally show the previous-guest checkout date and next-guest arrival date in a single cell (e.g. `← 05-13 Wed   →   05-15 Fri`) so the operator can see the turnover context at a glance. Backend already returns `prev_stay_id` / `next_stay_id`; the date-lookup can be done client-side against data already loaded, or by extending the backend response to include `prev_checkout` and `next_checkin` dates (preferred — one join, no N+1).
- For orphan midweek nights, label the adjacent occupied nights the same way.

### D5. Returning guests — Top-5 inline, rest behind a button
- Inline: the Demand tab shows **summary count + Top-5 list** (name · total stays · total gross) sorted by `total_stays` desc, then `total_gross_cents` desc.
- A **"Show all returning guests"** button opens the existing paginated dialog (already implemented) with the same 50-per-page behaviour.
- Handle pluralisation ("1 stay" vs. "3 stays").

---

## E. Cross-cutting polish

### E1. Empty-state convention
Adopt a single pattern for empty/insufficient data across all charts:
- Render a centred muted paragraph inside the chart card:
  - "No data for this window yet."
  - "Not enough history for a reliable trend."
  - "Finance data not yet imported for these stays."
- Never render an SVG that is all-zero or a single flat baseline.

### E2. Colour palette consistency
Pin the following CSS variables (or Tailwind tokens) and reuse across every chart:
- Primary series: `--chart-blue` (Occupancy, pace).
- Secondary series: `--chart-amber` (ADR, RevPAR).
- Positive / negative bars: `--chart-green` / `--chart-red` with an `--chart-amber` transitional band for -€1…-€50.

### E3. Button/tab affordance audit
Beyond §A2, sweep `AnalyticsView.vue` for every `<button>` that currently inherits a `background: #fff` and apply the shared button tokens already used by `FinanceView.vue` and `CleaningView.vue`.

---

## F. Backend deltas required

| # | Change | File | Rationale |
|---|---|---|---|
| F1 | Filter out finance-empty rows in `ListNetPerStay` (`WHERE gross_cents > 0 OR commission_cents > 0 OR payment_fees_cents > 0`). | `backend/internal/store/analytics.go` | Feeds §C5 — prevents misleading negative "net per stay" driven solely by cleaner fees. |
| F2 | Include `prev_checkout` / `next_checkin` ISO dates in gap-night rows; include `prev_checkout` / `next_checkin` for orphan rows. | `backend/internal/store/analytics.go` + handler DTO in `analytics_handlers.go` | Feeds §D4 without an N+1 query. |
| F3 | Add `week_starts_on` column to `properties` table (nullable, default `'monday'`), expose via property endpoints. New migration under `backend/internal/migrate/` (next sequential number). | migrations + `properties` store + property handler | Feeds §C6 — per-property configurable week start. |

For all three, write/extend unit tests in `backend/internal/store/analytics_test.go` (or `properties_test.go` for F3).

---

## G. Test updates

### G1. Backend
- `ListNetPerStay` — new test: stays with only cleaner-fee (no gross, no payout) must be excluded from results.
- Gap-night / orphan-night helpers — extend existing tests to assert the new `prev_checkout` / `next_checkin` fields.
- Property store — CRUD test covering `week_starts_on` default and update.

### G2. Frontend (new — `frontend/src/views/analytics.spec.ts`)
- Tab-button styling: active vs. inactive class assertions.
- Unsold-nights roll-up: given a fixture of 12 consecutive unsold nights + 1 gap + 4 more, expect exactly 5 ranges (capped) with correct `from`/`to`/`nights` values.
- DOW relabel: property with `week_starts_on='monday'` renders the first bar as `Mon`; `sunday` renders `Sun`.
- Lead-time / LOS bucket labels include explicit units.
- Returning-guests: top-5 inline preview renders 5 rows max; clicking "Show all" opens dialog.
- Net-per-stay: fixture entries with all finance fields = 0 are filtered out client-side.

---

## H. Documentation

- Update `spec/PMS_05_Analytics_Module_Spec.md` with the renamed "Operating cashflow" section, the Outlook-tab removal of New bookings, the unsold-nights range roll-up, and the per-property `week_starts_on` preference.
- Update `spec/PMS_02_Module_Specifications.md` §Property to document the new `week_starts_on` field.
- Update `spec/PMS_03_Implementation_Checklists.md` with a **Phase 6.1 — Analytics polish** row referencing this document.

---

## I. Rollout order (suggested)

1. **Quick visual fixes** (no backend): A1, A2, A3, B1, D2, D3, D5, C1 ordering, E1/E2/E3.
2. **Frontend data shaping**: B2 roll-up, B3 removal, C3 sparse-data guard, C5 client-side filter.
3. **Backend deltas**: F1, F2, F3 (with migration + tests).
4. **Wire backend-dependent polish**: C5 using F1, D4 using F2, C6/D3 using F3.
5. **Tests**: G1, G2.
6. **Docs**: H + checklist flip.

Each stage should leave the build and test suites green.
