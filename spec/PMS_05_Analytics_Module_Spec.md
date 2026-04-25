# PMS Analytics Module — Business Spec

## Purpose

Give the property owner a **business-intelligence layer** over the data the PMS already captures, answering three kinds of question:

1. **Forward-looking** — "How is the next month shaping up vs last year? What's on the books?"
2. **Retrospective** — "How did last month / last quarter / last year actually perform?"
3. **Pricing & demand** — "When do guests book, how far ahead, for how long, at what price?"

This module is distinct from the existing **Dashboard Module (#8)**, which is an operational landing page ("who is arriving today, did the sync run, current-month totals"). Analytics is the **strategic** layer; Dashboard is the **operational** layer. They are read-side-only and share the same underlying data.

**Source of truth for available data:** [`PMS_04_Analytics_Data_Inventory.md`](PMS_04_Analytics_Data_Inventory.md).

---

## Scoping assumptions (locked)

These decisions bound the spec. Revisit only on explicit owner request.

| Decision | Value |
|---|---|
| Portfolio scope | Single property. No cross-property comparison UI, no property filter dropdown. Multi-tenancy plumbing remains backend-only. |
| Booking channels | Booking.com only (ICS + payouts CSV). No channel-mix widgets. |
| History depth | 2+ years available → all YoY comparisons valid. |
| Payouts CSV cadence | Imported monthly. Revenue-side metrics (ADR, RevPAR, commission, net) are authoritative but **lag up to ~30 days** behind arrivals. UI must surface this freshness explicitly. |
| Lead time basis | `imported_at → start_at` is accepted as a close proxy for booking creation time (sync is frequent enough). |
| Returning guests | Fuzzy match on normalized `guest_name` from Booking.com payouts. Always labelled as "likely returning" with a disclaimer. |
| Currency | EUR only. No FX. |

---

## Data caveats the UI must acknowledge

These are non-negotiable product disclosures so the owner trusts the numbers:

1. **Revenue metrics lag arrivals by up to ~30 days** because they depend on the next Booking.com payout CSV import. Every revenue tile shows "revenue data through: <date>" derived from `MAX(finance_booking_payouts.payout_date)`.
2. **Forward revenue is a blend**: bookings with a payout already matched contribute their actual gross; bookings without a payout yet contribute an **estimate = property-wide trailing-12-months ADR × nights**. Estimated vs confirmed portions must be visually distinct.
3. **Returning-guest count is fuzzy**. The UI labels it as "likely returning" and offers a drill-down list so the owner can eyeball false positives.
4. **Cancelled stays** are excluded from occupancy, ADR, RevPAR, and revenue totals, but counted in cancellation metrics.
5. **Gap-night analysis** only considers nights between two *confirmed active* stays.

---

## Metric catalog (with definitions)

Precision matters. Every formula below is implementable against the tables listed in `PMS_04`.

### Nightly / calendar primitives

- **Night** — a calendar date `d` such that `start_at_date ≤ d < end_at_date`. A stay from 2026-06-10 → 2026-06-13 contributes 3 nights: the 10th, 11th, 12th.
- **Available nights in period P** — count of calendar dates in P. No blocking calendar is modelled; every night is assumed sellable.
- **Nights sold in period P** — sum over all *active* (non-cancelled, non-deleted) stays of nights that fall inside P.
- **Active stay** — `occupancies.status IN ('active', 'updated')`. Cancellations are `cancelled` or `deleted_from_source`.

### Core performance metrics

| Metric | Formula | Notes |
|---|---|---|
| Occupancy rate | nights sold / available nights | Per period, per month, per day-of-week. |
| ADR (Average Daily Rate) | Σ `amount_cents` (gross from payouts) / nights sold — **for stays that have a matched payout** | Denominator matches numerator: never divide payout revenue by all nights sold. |
| RevPAR | Σ gross / available nights | Equivalent to ADR × occupancy (on the same stay set). |
| Net revenue per night | Σ `net_cents` / nights sold (matched stays only) | Already net of Booking.com commission and payment fees. |
| Booking.com effective take-rate | Σ (commission + payment_service_fee) / Σ gross | Trended monthly. Alerts if it creeps up. |
| Average length of stay | nights sold / distinct active stays in period | |
| Gross booking revenue | Σ `amount_cents` | Period defined by `check_in_date` (arrival cohort), not `payout_date`. |
| Net booking revenue | Σ `net_cents` | Same cohort definition as gross. |

### Booking behavior metrics

- **Lead time** — `DATE(start_at) − DATE(imported_at)` in days, per stay. Bucketed as: **0–3 / 4–14 / 15–45 / 46–90 / 91+**.
- **Booking pace curve** — for a given *future arrival window* (e.g. "stays arriving in July 2026"), for each integer `T` from 0 to 180: count of bookings for that window already received at `T` days before the window's *start*. Plotted as a cumulative curve, with last year's curve for the same-named window overlaid.
- **Day-of-week occupancy** — nights sold per weekday / available nights per weekday, across the selected period.
- **Seasonality heatmap** — occupancy rate per ISO week × year. Colour scale goes dark at ≤30%, hot at ≥90%.
- **Length-of-stay distribution** — histogram, buckets **1 / 2 / 3 / 4–5 / 6–7 / 8–14 / 15+ nights**.

### Forward-looking metrics

- **On-the-books nights, next 30/60/90 days** — nights sold for arrival dates in `[today, today + N)`.
- **Forward occupancy, next 30/60/90 days** — on-the-books nights / available nights in the window.
- **On-the-books revenue, next 30/60/90 days** — Σ gross for matched stays + Σ (trailing-12m ADR × nights) for unmatched stays, split visually.
- **Pace vs same time last year** — on-the-books nights for calendar-window `[today, today+30)` **today** vs on-the-books nights for `[today −1y, today −1y +30)` as-of `today −1y`. Needs a point-in-time reconstruction: for each day in history, the set of bookings with `imported_at ≤ D`. Sqlite can reconstruct this from `occupancies.imported_at` + `last_synced_at`.
- **Unsold nights in next 14 days** — list view with dates and adjacent-stay context, to trigger last-minute pricing action.

### Cancellation metrics

- **Cancellation rate** — cancelled stays in period / (cancelled + active) stays in period, cohorted by **arrival date**. A cancellation is detected when status transitions to `cancelled` or `deleted_from_source` (tracked via `content_hash` churn and final status).
- **Cancellation lead time distribution** — `DATE(start_at) − DATE(cancelled_at)` in days; `cancelled_at` inferred from `last_synced_at` on the final status-change. Buckets **0–3 / 4–14 / 15–45 / 46+**. Late cancellations are the painful ones; highlight them.

### Gap & efficiency metrics

- **Gap night** — any available night `d` such that `d − 1` was the checkout night of stay A and `d + 1` is the check-in night of stay B, both active. Single-night gap between two back-to-back bookings.
- **Orphan midweek** — 1–2 consecutive unsold nights that fall Mon–Thu with booked weekends on both sides. Prime candidate for midweek promo.

### Guest recognition (fuzzy, disclosed)

- **Returning guest** — a stay whose `normalize(guest_name)` already appears on an **earlier** stay. `normalize = lowercase + NFD unicode strip + trim + collapse spaces`. Minimum match length 6 characters to avoid "Anna" colliding with "Anna".
- **Returning-guest rate** — returning stays / total active stays in period.
- **Drill-down** — flat list of (normalized name, stay count, first stay, last stay) so the owner can spot duplicates and false matches by eye.

### ADR diagnostics (pricing lens)

- **ADR by month** — seasonality pricing curve (12 monthly points, compared YoY).
- **ADR by day-of-week** — weekend premium visualisation.
- **ADR by lead-time bucket** — answers "do last-minute bookers pay more or less than early planners?" Useful for deciding how aggressively to discount the unsold tail.

### Margin roll-up (already partly computed)

- **Net per stay** — gross − commission − payment_service_fee − allocated cleaning cost (cleaning cost of the stay's checkout day from `cleaning_daily_logs` + `cleaner_fee_history`).
- **Cleaner margin %** — reuse the already-computed figure from `/finance/summary`.
- **Cost per booked night** — (cleaning + recurring outgoing) / nights sold, per month.

---

## Functional requirements

### Page layout

Single Analytics page at `/analytics`, three tabs:

1. **Outlook** (default — the morning-glance view)
2. **Performance** (retrospective)
3. **Demand & Pricing**

A slim **freshness bar** sits above all tabs showing: last ICS sync timestamp, last payout CSV import date, count of unmatched payouts (payouts with no linked occupancy).

### Tab 1 — Outlook (forward-looking)

- Hero row: three KPI tiles — forward occupancy **30 / 60 / 90 days**, each with YoY delta in %.
- Hero row: on-the-books gross revenue **30 / 60 / 90 days**, split confirmed vs estimated, with YoY delta.
- Pacing chart: cumulative on-the-books nights for the next 90-day arrival window, overlaid with the same-day-last-year curve.
- Upcoming unsold-nights table: every sellable night in the next 14 days that isn't booked, with adjacent-stay context (what's booked before / after).
- "New bookings received in the last 7 days" sparkline (cheap to compute from `imported_at`).

### Tab 2 — Performance (retrospective)

- Period selector: current month / last 12 months rolling / calendar year / custom range, each with a YoY toggle.
- KPI grid: occupancy rate, ADR, RevPAR, gross revenue, net revenue, Booking.com effective take-rate. Each tile shows current, prior period, YoY delta.
- Monthly trend chart: occupancy + ADR dual-axis line, 24 months.
- Seasonality heatmap (ISO week × year).
- Day-of-week occupancy heatmap for the selected period.
- Cancellation panel: cancellation rate, cancellation lead-time histogram.
- Net-per-stay panel: table of stays in period with gross / commission / fees / cleaning cost / net, sortable.

### Tab 3 — Demand & Pricing

- Lead-time distribution histogram (with median + mean annotations).
- Booking pace curve for the next three month-windows (this month, +1, +2), each overlaid with last year's curve.
- Length-of-stay distribution histogram.
- ADR-by-month curve (seasonality).
- ADR-by-day-of-week bar chart.
- ADR-by-lead-time-bucket bar chart.
- Gap-nights and orphan-midweek tally with dates.
- Likely-returning-guests panel: count, rate, drill-down list.

---

## Business rules

- **Cancelled stays are excluded** from occupancy / ADR / RevPAR / revenue totals; included in cancellation metrics only.
- **Revenue metrics cohort by arrival date** (`check_in_date` on the payout, or `start_at` on the occupancy), not by payout date. A payout received in March for a stay in February belongs to February revenue.
- **When a stay has no matched payout**, it contributes to occupancy-side metrics but not to revenue-side metrics. Forward-revenue estimation is the only exception and must be visually distinguished.
- **YoY comparisons** anchor on calendar period, not on weekday alignment. July 2026 compares to July 2025.
- **Pace vs LY** uses the "as-of offset" technique: today's position in 2026 vs the equivalent day in 2025 (`today − interval '1 year'`).
- **Returning-guest** detection requires minimum 6 normalized characters and must never influence any revenue or occupancy metric — it is a descriptive statistic only.
- **Freshness warnings**: if `MAX(payout_date) < today − 45 days`, show a yellow banner on revenue widgets. At >75 days, show red.

---

## Suggested API endpoints

All endpoints are read-only, scoped by the property resolved from auth context (no `property_id` in URL needed given single-property assumption — but include it for future-proofing).

- `GET /api/analytics/outlook?property_id=…` — forward KPIs, pacing series, unsold-nights list, new-bookings sparkline.
- `GET /api/analytics/performance?property_id=…&from=…&to=…&yoy=true` — retrospective KPI grid, monthly trends, heatmaps, cancellation stats, net-per-stay stay list.
- `GET /api/analytics/demand?property_id=…&from=…&to=…` — lead-time, length-of-stay, ADR-by-X breakdowns, gap-nights, returning-guests.
- `GET /api/analytics/pace?property_id=…&window=YYYY-MM` — pace curve for one arrival window, with LY overlay series.
- `GET /api/analytics/returning-guests?property_id=…&limit=…&offset=…` — paginated drill-down list for the fuzzy-match panel.
- `GET /api/analytics/freshness?property_id=…` — tiny endpoint powering the freshness bar.

All list responses include a `generated_at` timestamp and the input filters echoed back.

---

## Suggested database entities

**No new tables required.** All metrics derive from the existing schema documented in `PMS_04`:

- `occupancies` + `occupancy_sync_runs` → nights, occupancy, lead time, cancellations, pace, gaps.
- `finance_booking_payouts` → ADR, RevPAR, gross / net / commission / fees, returning-guest name source.
- `finance_transactions` + `cleaning_monthly_summaries` → net-per-stay cleaning allocation, cost-per-night.

**Optional future addition** (flagged, not included in v1):

- `analytics_snapshots` — a nightly materialized cache `(property_id, metric_code, period_key, value_cents_or_ratio, computed_at)`. Only add if query latency is unacceptable on live data. Keep v1 live-computed.

---

## Frontend screens

- Single `/analytics` page with tabbed navigation (Outlook / Performance / Demand & Pricing).
- Persistent freshness bar across all tabs.
- Period selector on Performance and Demand & Pricing tabs.
- All charts must degrade gracefully when a period has insufficient data (e.g. "Not enough history for YoY").
- All revenue-annotated widgets display the freshness disclaimer on hover.
- Returning-guests panel includes a visible "fuzzy name match — review for accuracy" caption with a link to the drill-down.

### Chart rendering

- **Library:** [Chart.js](https://www.chartjs.org/) 4.x via [`vue-chartjs`](https://vue-chartjs.org/) 5.x. MIT-licensed, tree-shakeable controller registration, Vue 3 + TypeScript native, honours `prefers-reduced-motion`, ≈ 55 kB gz after tree-shake of the Line/Bar controllers.
- **Wrapper:** `frontend/src/components/charts/UiLineChart.vue` (shared primitive, see `PMS_08_UI_UX_Polish_Spec.md` §12.1). Every chart in Analytics must render through a wrapper — no direct `Chart.js` imports in view files.
- **Palette:** chart colours come exclusively from `--viz-1…--viz-8` / `--color-text-muted` / `--color-border` tokens. The library's own theme JSON must not be forked; token changes win automatically.
- **Accessibility:** every wrapper sets `role="img"` + `aria-label` on the canvas and emits a `.sr-only` `<table>` fallback so screen readers and print surfaces still receive the data.
- **Migration status (2026-04-23):** monthly-trend and pacing-series line charts render exclusively via `UiLineChart`; hand-rolled SVG fallbacks removed. Bar-style charts (net-per-stay, yearly cleaning, DOW occupancy) remain as HTML/CSS bars — simpler and cheaper there; migrate only if interactivity (tooltips, zoom) is needed.

---

## Test focus

- Cohorting correctness: a payout received in March for a February stay lands in February revenue.
- Occupancy denominator: 28-day February computes correctly, as do DST boundaries (stays stored in UTC).
- Cancelled stays excluded from performance metrics, included in cancellation stats.
- Forward-revenue split: confirmed vs estimated portions sum to the total.
- Pace-vs-LY "as-of" reconstruction produces monotonic cumulative curves.
- Returning-guest normalization handles diacritics (`Novák` == `novak`), trims whitespace, rejects ≤5-char names.
- Gap-night detection on back-to-back-bookings with same-day checkout/check-in correctly reports **zero** gap (not one).
- Freshness thresholds (45 / 75 days) render the correct banner colour.
- YoY gracefully disables when <13 months of data exist.

---

## Metrics migrated from other modules

To avoid duplicated "yearly" roll-ups across the UI, the following existing figures **move out of their current home and become Analytics-only** in v1:

| Migrated metric | Current home | New home in Analytics |
|---|---|---|
| Yearly cleaning count (12 monthly bars, selectable year) | Cleaning module — "Yearly Cleaning Stats" card on the cleaning page | **Performance tab** — new "Cleaning activity" panel (bars unchanged, same SVG style) |
| Yearly incoming / outgoing / net balance tiles (selectable year) | Finance module — "Yearly Overview" cards at the top of the finance page | **Performance tab** — new "Operating cashflow (year)" KPI row next to the occupancy/ADR/RevPAR grid |

Rules for the migration:

- The old UI affordances (the Yearly Overview cards in Finance, the Yearly Cleaning Stats card in Cleaning) are **removed**, not duplicated. The only remaining yearly surfaces in Finance and Cleaning are their existing monthly views.
- The backing endpoints `GET /api/properties/{id}/cleaning/yearly-stats?year=…` and the `yearly_incoming_cents` / `yearly_outgoing_cents` / `yearly_net_cents` fields returned by `GET /api/properties/{id}/finance/summary` remain available during the migration window but are **deprecated**. The Analytics implementation either consumes them internally or replaces them with fresh `/api/properties/{id}/analytics/...` reads — the frontend must not call them any more once the migration lands.
- Access control flips from `cleaning:read` / `finance:read` to `analytics:read`. A user who previously only had cleaning or finance read access will lose visibility into yearly stats unless they are granted analytics read too; this is intentional and consistent with the rest of the Analytics module's permission model.
- Yearly ranges follow the **calendar year in property timezone**, matching today's behaviour.

These roll-ups surface inside the existing Performance-tab layout; no new tab is required. The Demand & Pricing tab already covers forward-looking pricing and remains untouched.

---



- **Channel mix** — only relevant once Airbnb or a direct channel is added.
- **Competitor / market benchmarking** — no external data feed.
- **VAT / tax analytics** — requires schema extension (see `PMS_04` gap 3).
- **Outbound-message analytics** — no send log exists (gap 4).
- **Multi-property portfolio view** — locked to single property per scoping decision.
- **Guest geography from invoice VAT prefix** — too sparse and noisy to be useful.
- **Materialized snapshot cache** — add only if live computation becomes slow.
- **Day-of-week a booking was made** (as opposed to arrival weekday) — low operational value.

---

## Open questions to revisit post-v1

1. Should gap-night detection trigger an actionable prompt (e.g. "offer a discount for this orphan night")? Currently read-only.
2. Should the "estimated forward revenue" use trailing-12m ADR, or the same-month-last-year ADR (more seasonal, less sample)?
3. Is there appetite to start capturing guest email at check-in to move returning-guest detection from fuzzy to precise? (Separate module change; answer locks whether we ever upgrade this metric.)
