# PMS 38 — Finance monthly cashflow and revenue corrections

## 1. Status and objective

- **Date:** 2026-10-24.
- **Status:** Implemented and automated checks verified; manual desktop/mobile browser inspection remains unverified. No deployment is claimed.
- **Audience:** AI developer agent, reviewer, and QA.
- **Evidence:** Current repository code and existing Finance specifications, inspected while preparing this specification. No production balances were verified.

Correct the existing Finance figures to distinguish payout-date cashflow from calendar-month stay revenue, use payout Net consistently, count payout deductions once, and explain every affected KPI through an info icon. Apply the corrections throughout Finance reporting and the Dashboard Finance widgets.

Reuse the existing booking data, transactions, recognition allocator, endpoints, UI components, formatting, and tests. Follow `PMS_13_Coding_Conventions.md`.

### Authority

This specification supersedes conflicting definitions of Property income, reported incoming/outgoing totals, Recognized net, and gross-based recognition presentation in earlier Finance specifications, including PMS 28. It supersedes PMS 27's incoming-only Property income reporting and treatment of negative payout balances as reported outgoing; **PMS 27's original-payout-date correction behavior remains**.

PMS 29's rent history and eligible-expense rules and PMS 31's Long-term net presentation remain authoritative. Their Recognized net input now uses this specification. Earlier restrictions limiting changes to Overview or preserving gross presentation on Dashboard/Revenue are superseded by the confirmed cross-surface scope.

## 2. Confirmed product decisions

| ID | Decision |
|---|---|
| D01 | Rename **This month** to **Monthly CashFlow**. Property income uses payout **Net**, selected by **Payout date**, without filtering by **Type**. |
| D02 | Commission adjustments and refunds continue to update the original booking's balance and retain its original payout date. A September correction to an August payout revises August cashflow. |
| D03 | Negative payout Net reduces income only. It must not also be deducted as outgoing. |
| D04 | Monthly incoming equals Property income plus other incoming transactions, with payouts counted once. Monthly net equals Monthly incoming minus Monthly outgoing. |
| D05 | Ordinary outgoing transactions and the Cleaner expense calculation continue to count as before. D03 is the exception to the former direction-only outgoing total. |
| D06 | Rename **Revenue recognition** to **Monthly Revenue**, and **Recognized gross** to **Recognized incoming**. Use Net allocated across calendar-month stay nights. |
| D07 | Recognized net equals Recognized incoming minus Monthly outgoing. Other incoming transactions are not added to this result. |
| D08 | Continue checkout-exclusive night allocation, existing rounding, scheduled-check-in-month recognition for paid cancellations/no-shows, and existing exclusion/reporting of invalid stay dates. |
| D09 | Reuse current stored booking check-in/check-out dates and their existing import precedence. Do not switch to the original payout CSV's dates when stored dates differ. |
| D10 | Keep Long-term net's current formula and presentation. Its amount may change through the corrected Recognized net input. |
| D11 | Keep Cleaner margin's current formula and zero/nonpositive-income behavior, using corrected Property income. |
| D12 | Apply the corrections to all affected displays, including Finance Revenue and Dashboard Finance widgets, rather than Overview alone. |
| D13 | Provide hidden generic calculation help through an info icon on all Finance KPI tiles, including Cleaner margin and Lifetime totals. Use hover/focus and click/tap interaction. |
| D14 | Use the two accepted footnotes verbatim, as recorded in section 6. |

## 3. Verified implementation and reuse map

| Existing file | Relevant behavior / reuse |
|---|---|
| `backend/internal/store/finance.go` | `ComputeFinanceSummary` currently sums transaction directions; Property income additionally requires the category's `counts_toward_property_income` flag. Cleaner expense uses generated cleaning-salary rows; Cleaner margin divides that expense by monthly Property income. Lifetime totals and category breakdown are returned here too. |
| `backend/internal/store/finance_revenue_recognition.go` | Already reads payout-backed bookings, allocates both gross and Net over stored stay dates, and handles signed cents, calendar nights, cancellation/no-show exceptions, and invalid dates. `ComputeFinanceOtherMovements` already sums transaction-month movements excluding `booking_payout`. |
| `backend/internal/finance/statements/merge.go` | Commission adjustments/refunds update canonical balances while retaining original payout context. Statements may replace canonical stay dates. |
| `backend/internal/api/finance_imports_handlers.go` | Import side effects use the merged canonical Net and payout date. |
| `backend/internal/store/finance_bookings_merge.go` | `UpsertBookingFinanceTransaction` represents negative Net as an outgoing row with an absolute amount. One linked payout transaction is maintained per booking reference. |
| `backend/internal/api/finance_handlers.go` | Recognition response already exposes `recognized_booking_net_cents`. It currently adds other incoming to Recognized net. Per-booking Net is computed by the store but not exposed in the response. |
| `backend/internal/store/finance_long_term_rent.go` | Existing eligible outgoing excludes booking payouts, generated cleaning salary, and the cleaning-salary category. |
| `backend/internal/api/server.go`, `server_response_types.go` | Dashboard uses the same Finance summary and recognition store functions, but currently exposes recognized gross. |
| `frontend/src/views/FinanceView.vue` | Loads existing summary/recognition endpoints, validates response values, and supplies Overview/Revenue. Already handles month/property changes and stale Finance responses. |
| `frontend/src/views/finance/FinanceOverviewTab.vue` | Contains six monthly cashflow cards, three revenue cards, two lifetime cards, and the existing Long-term net presentation. |
| `frontend/src/views/finance/FinanceRevenueTab.vue` | Gross-based recognition KPI and per-booking table; must consume the existing Net calculation instead. |
| `frontend/src/views/finance/FinanceBreakdownTab.vue` | Displays incoming/outgoing by category and an outgoing-only chart. Must reconcile with corrected summary totals. |
| `frontend/src/views/dashboard/DashboardHeroKpis.vue` | Displays Finance net cashflow and recognized gross beside unrelated Dashboard KPIs. |
| `frontend/src/components/ui/UiKpiCard.vue`, `UiSection.vue` | Reusable card/section primitives. Cards support visible hints but currently have no hidden-help capability. |
| `frontend/src/api/types/finance.ts`, `dashboard.ts`, `spec/openapi.yaml` | Existing API contracts to extend only where required; regenerate `frontend/src/api/types/generated.ts`. |

## 4. Calculation contract

### 4.1 Sources, dates, and signed amounts

Use integer cents for money and existing property scoping and month-selection conventions.

**Payout income source:** `finance_bookings` with `has_payout_data = 1`, using its current canonical `net_cents` and `payout_date`. Include all row types and positive, zero, and negative Net. Do not filter by category income flags, reservation status, or whether a linked cash transaction exists. Statement-only bookings are not payout income.

**Other movements source:** `finance_transactions` with `source_type <> 'booking_payout'`, using transaction date, direction, and stored amount. Include manual, stored recurring, cleaning, categorized, and uncategorized entries according to direction. Category property-income flags do not decide whether an ordinary incoming transaction is counted.

Payout transactions are a representation of the booking balance, not an additional income source. Exclude them from both other incoming and outgoing aggregations, even when their stored direction is outgoing. Classify by source type, not notes, category title, or amount sign.

The existing storage representation may remain: this reporting correction does not require converting negative payout transactions to negative incoming rows or deleting linked transactions. The Transactions tab remains a view of stored records; it is not the authoritative direction-only summation for the corrected KPIs.

Existing bookings with applied corrections must produce corrected reports immediately. No re-import or new generated-entry sync is required to adopt the reporting rules. Do not replay raw correction rows on top of an already corrected canonical Net.

### 4.2 Monthly CashFlow

For the selected property and month `M`:

```text
P(M) = sum(signed canonical payout Net whose payout date is in M)
I(M) = sum(incoming non-booking_payout transactions dated in M)
O(M) = sum(outgoing non-booking_payout transactions dated in M)

Property income  = P(M)
Monthly incoming = P(M) + I(M)
Monthly outgoing = O(M)
Monthly net      = P(M) + I(M) - O(M)
```

Reuse `ComputeFinanceOtherMovements` for `I(M)` and `O(M)` or factor its existing predicate into a shared store helper used by both reports. The summary and recognition paths must not implement different outgoing eligibility rules.

An ordinary incoming transaction marked as property income contributes to `I(M)`, not `P(M)`. It is still counted once in Monthly incoming. Keep the category configuration intact; this specification changes the reporting definition, not the category editor.

Cleaner expense retains its current generated-cleaning-salary calculation. Cleaner margin remains:

```text
if P(M) > 0: Cleaner margin = Cleaner expense / P(M)
otherwise:  Cleaner margin = 0
```

The UI continues to display this ratio as a percentage using its existing precision and tone rules. It is an expense-to-income ratio despite the existing label.

### 4.3 Lifetime totals and category breakdown

Apply the same source and sign rules to the other existing Finance summary outputs so the reporting surfaces agree:

- `property_income_cents`: all-time signed payout-backed canonical Net.
- `total_incoming_cents`: all-time Property income plus all-time non-payout incoming transactions.
- `total_outgoing_cents`: all-time outgoing transactions excluding `booking_payout`.
- Monthly breakdown: signed payout Net is an incoming contribution in its payout month; other movements use their transaction month and direction. Negative payout amounts reduce the incoming column and never enter the outgoing chart.

Reuse the linked payout transaction's category for attribution where available. If no usable linked category exists, use the existing booking-income category lookup, falling back to the existing uncategorized bucket if necessary. Ordinary transactions retain their categories. Do not create categories for this feature or lose bookings because a category/transaction is absent.

Each payout booking must contribute once, including when building joins for category attribution. Breakdown incoming and outgoing sums must equal Monthly incoming and Monthly outgoing respectively. Signed incoming totals, including negative totals, must remain signed in the UI.

### 4.4 Monthly Revenue

Let `A(M)` be the existing `RecognizedBookingNetCents` calculation for the selected month:

```text
Recognized incoming = A(M)
Recognized net      = A(M) - O(M)
```

Other incoming `I(M)` contributes to cashflow only. For example, an owner contribution with no payout-backed bookings does not create Recognized incoming or increase Recognized net.

Reuse the existing allocator rather than writing a second average-rate implementation:

1. Use the current canonical booking check-in and check-out dates. Keep existing statement-over-payout date precedence.
2. For normal stays, count property-local calendar nights from check-in inclusive to checkout exclusive. Allocate Net evenly over all stay nights, taking only nights within the selected calendar month. Include overlaps from stays starting in earlier months.
3. Keep integer-cent remainder allocation to the earliest nights, including the existing symmetric handling of negative amounts. All monthly allocations across a complete stay must sum exactly to its Net.
4. Paid cancellations and no-shows retain full-Net recognition in the scheduled check-in month, after the existing stay-window validation. They are not spread across scheduled nights.
5. Missing/invalid stay windows remain excluded from recognition and visible in the existing Revenue data-issues display. Such a booking can still contribute to cashflow if it has valid payout data/date.
6. Keep the `has_payout_data` eligibility and current status/outcome handling. Neither payout month nor a Type whitelist restricts recognition.

Net already includes platform deductions. Do not subtract commission, VAT, or payment-service fees again. Later corrections affect the stored Net and therefore the original stay's recognized months, as well as its original payout month in cashflow.

### 4.5 Long-term net

Preserve the existing rent history, expense exclusions, full-month rent, and PMS 31 display:

```text
L = applicable monthly rent - existing eligible outgoing
N = corrected Recognized net

main card value = -abs(L) + max(N, 0)
subtitle       = "Long term rent: {formatted L}"
```

Preserve zero normalization, sign-based tone, unavailable/not-configured states, rent-management permissions, and current/future/provisional labels. The existing API's `short_term_difference_cents` and `outcome` must use corrected `N` too, even though PMS 31 does not use them to render the main card.

### 4.6 Generated entries and loading

Use currently stored recurring and cleaning entries and the existing explicit sync workflow. Preserve the provisional explanation when the month has not been synced. An empty valid result is zero; a failed request or unavailable required input is not a valid zero. Retain existing loading, error, and stale-response protections and propagate errors from any changed aggregate query rather than silently returning invented zero totals.

## 5. API and cross-surface contract

Use the existing Finance summary, revenue-recognition, and Dashboard endpoints.

### Finance summary

Keep the existing response fields and apply section 4's semantics. No additional cashflow endpoint or persisted summary is needed.

### Revenue recognition

- Use existing `recognized_booking_net_cents` as the Overview and Revenue KPI's **Recognized incoming** source.
- Change `recognized_net_cents` to `recognized_booking_net_cents - other_outgoing_cents`.
- Retain `other_incoming_cents` as informational cash movements, but do not add it to Recognized net.
- `other_outgoing_cents` must equal the summary's `monthly_outgoing_cents` for the same property/month and stored data.
- Expose the store's existing per-booking `NetCents` and `RecognizedNetCents` as required integer fields `net_cents` and `recognized_net_cents` on recognition booking rows.
- Existing gross API fields, if retained, must remain genuine gross values. Do not put Net into a field named gross or rename the underlying guest-paid booking Amount to Net.

### Dashboard

- Continue sourcing monthly incoming/outgoing/net from the shared Finance summary.
- Add `recognized_incoming` to the existing Finance-month widget, populated from `RecognizedBookingNetCents`.
- Change the Finance KPI to **Recognized incoming (month)** and consume that field. An existing `recognized_gross` field may remain for compatibility but retains its gross meaning and is no longer used for this KPI.
- Dashboard's current-property-local-month Finance values must match Finance for that property/month.

Update handler DTOs, handwritten frontend types, OpenAPI required fields/descriptions, fixtures, and generated types together. Do not add a separate calculation on the client or fall back from a missing Net field to gross. Keep existing currency formatting and access checks.

The cross-surface scope is the existing Finance metrics: Overview, Revenue, summary-backed Lifetime totals/Breakdown, and Dashboard Finance widgets. It does not redefine independently named gross/commission metrics in other analytics or invoice guest charges.

## 6. Presentation and hidden calculation help

### 6.1 Overview

Keep the existing card order and responsive layout. Use these section titles exactly:

- **Monthly CashFlow**
- **Monthly Revenue**

Render the accepted footnotes beneath their respective card groups:

> Property income is based on payout date. Other incoming and outgoing transactions use their transaction date.

> Income is allocated across stay nights within the selected calendar month. Expenses use their transaction date.

Replace **Recognized gross** with **Recognized incoming** and update contradictory descriptions and visible formula hints. Retain useful dynamic hints such as profitability, availability, provisional state, and the Long-term net subtitle. Generic calculation explanations belong in the hidden help rather than being repeated permanently below every value.

### 6.2 Revenue tab and Dashboard

In `FinanceRevenueTab.vue`:

- Rename the gross-based section to **Monthly Revenue** and its KPI to **Recognized incoming**.
- Use the accepted Monthly Revenue footnote.
- Replace table **Stay gross** with **Stay net**, sourced from `net_cents`.
- Replace table **Recognized gross** with **Recognized incoming**, sourced from the booking row's `recognized_net_cents`.
- Keep dates, recognized nights, status badges, cancellation/no-show wording, and the data-issues display. Update descriptions/empty states from gross to incoming where applicable.
- The table's recognized amounts must sum to its KPI and match Overview.

Update the Dashboard Finance KPI and help to the same definitions. Avoid the old claim that all recognized revenue follows occupied nights, since paid cancellation/no-show exceptions remain.

### 6.3 Info-icon behavior

Extend the existing `UiKpiCard` with an optional calculation-help string (for example `calculationHelp`). Use the existing Lucide icon dependency and design tokens; no tooltip library is needed.

- Place a small info-icon button beside the label when help is supplied.
- Explanation is visually hidden by default and available on pointer hover, keyboard focus, and click/tap.
- Use a descriptive accessible name such as **How Monthly incoming is calculated**, a unique associated help ID, and appropriate accessible description/expanded-state attributes for the chosen interaction.
- Enter/Space must operate the button. Escape dismisses help; click/tap can close it. Handle focus/hover consistently so moving to read the explanation does not immediately hide it.
- Touch users must be able to reveal and dismiss the text without hover. Do not rely on a browser `title` attribute alone.
- Keep help within the viewport at mobile widths and avoid clipping by card/grid containers.
- Keep generic help available for zero, negative, unavailable, and unconfigured values. It explains a formula, not a fabricated result.

Supply help for all eleven Overview KPI cards, the Revenue KPI, and both Dashboard Finance KPIs. The optional component capability must preserve existing behavior for unrelated cards that do not supply help.

### 6.4 Calculation-help content

Use concise text with the following meaning; minor wording adjustments are allowed without changing the rules:

| Tile | Generic explanation |
|---|---|
| Property income | Signed Net from all payout-backed bookings with a payout date in the selected month, regardless of Type. Adjustments and refunds revise the original payout month and reduce income. |
| Monthly incoming | Property income plus other incoming transactions dated in the selected month. Imported payouts are counted once. |
| Monthly outgoing | Outgoing transactions dated in the selected month, including stored recurring and cleaning expenses. Payout deductions reduce income instead and are excluded here. |
| Monthly net / Dashboard Net cashflow | Monthly incoming minus Monthly outgoing. Payout deductions are counted once as reduced income. |
| Cleaner expense | Stored generated cleaning-salary entries for the selected cleaning month, including the salary adjustments reflected by the existing sync process. |
| Cleaner margin | Cleaner expense divided by Property income, shown as a percentage. Displays zero when Property income is zero or negative. |
| Recognized incoming, on every surface | Payout Net allocated over check-in-inclusive, checkout-exclusive stay nights in the calendar month. Paid cancellations and no-shows count in the scheduled check-in month; invalid stay dates are excluded. |
| Recognized net | Recognized incoming minus Monthly outgoing. Other incoming transactions are not added; payout deductions are already included in recognized income. |
| Long-term net | Start with the negative absolute value of monthly long-term rent minus eligible expenses, then add positive Recognized net. Eligible expenses exclude booking payouts and cleaning salary/source-category expenses. The subtitle shows rent minus eligible expenses. |
| Total incoming | All-time signed payout Net plus all other incoming transactions. Payout deductions reduce income and payouts are counted once. |
| Total outgoing | All-time outgoing transactions excluding booking payouts, whose deductions reduce income instead. |

## 7. Implementation sequence for the developer agent

### Step 1 — Correct the shared reporting sources

In `backend/internal/store/finance.go`, replace category-qualified Property income with canonical payout-backed signed Net. Reuse the existing non-payout movement query for monthly incoming/outgoing, then derive Monthly net and Cleaner margin. Align lifetime totals and breakdown attribution with the same sources and signs. Keep property isolation and booking deduplication explicit in SQL.

The reporting change uses existing schema and recorded balances. Do not introduce a payout-movement table, new import format, data migration, or automatic replay/repair of historical adjustments for this feature.

### Step 2 — Reuse recognition Net and update dependent responses

Keep `ComputeFinanceRevenueRecognition`'s allocation/eligibility rules. Update the API calculation of Recognized net and the Long-term comparison's dependent values. Expose per-booking Net fields and the Dashboard recognized-incoming field. Align OpenAPI/types with those response changes.

### Step 3 — Bind all affected displays

Update `FinanceView.vue`, `FinanceOverviewTab.vue`, `FinanceRevenueTab.vue`, and Dashboard Finance bindings/copy. Verify Breakdown and Lifetime totals agree with the shared summary. Retain current rent-management, sync, selection, permission, and availability flows.

### Step 4 — Add reusable hidden help

Extend `UiKpiCard.vue` once and supply generic explanations to the cards listed in section 6. Implement the two section footnotes with existing section/layout primitives.

### Step 5 — Verify behavior

Extend existing store/API Finance and Dashboard tests, `FinanceView.spec.ts`, `FinanceOverviewTab.spec.ts`, and `DashboardView.spec.ts`. Add focused recognition-table/help interaction coverage where the existing suites do not exercise the required behavior. Use acceptance cases below rather than tests that merely repeat implementation arithmetic.

## 8. Acceptance criteria

All monetary examples are EUR; tests should assert integer cents. Each example is independent unless stated otherwise.

| ID | Scenario | Expected result |
|---|---|---|
| AC01 | Stay and payout in the same month: Amount €120, Net €100; manual incoming €20, recurring incoming €5, ordinary outgoing €30, generated cleaning €10. | Property income €100; Monthly incoming €125; outgoing €40; Monthly net €85; Recognized incoming €100; Recognized net €60; Cleaner expense €10; Cleaner margin 10.0%. |
| AC02 | Net €300, stay Aug 30–Sep 2, payout Sep 5. | August recognized incoming €200 and September €100; September Property income €300, August €0. Checkout contributes no night. |
| AC03 | Net €100.00 over Jan 31–Feb 3; repeat with Net −€100.00. | Positive allocation €33.34 in January and €66.66 in February; negative allocation −€33.34 and −€66.66. Each complete stay sums exactly to its Net. |
| AC04 | August payout €100 revised by a September −€10 commission adjustment; repeat independently with a −€10 refund. Stay entirely in August. | Canonical Net €90 retains August payout date. August Property income and Recognized incoming €90; September gets no extra −€10 movement. No second commission/fee deduction. |
| AC05 | Payout Net −€15, linked stored outgoing booking-payout transaction €15, plus ordinary outgoing €20; stay and payout in the same month. | Property income and Monthly incoming −€15; Monthly outgoing €20; Monthly net −€35; Recognized incoming −€15; Recognized net −€35. Lifetime/breakdown also treat €15 as reduced income only. |
| AC06 | Net €80 canonical payout and linked incoming payout transaction €80, plus other incoming €10. | Property income €80 and Monthly incoming €90, not €170. Recognition is €80 for an eligible same-month stay; other incoming does not increase it. |
| AC07 | Payout-backed bookings of different Type values; category flags false or absent. A statement-only booking has an apparent value too. | All canonical payout Net values count once regardless of Type/category flag. Statement-only data contributes no payout income or recognition. |
| AC08 | A €25 manual incoming transaction has a category flagged as property income; no payouts. | Property income €0; Monthly incoming €25; Recognized incoming €0. With no expenses, Recognized net €0. |
| AC09 | No eligible bookings, owner contribution €500, ordinary outgoing €200. | Monthly incoming €500; outgoing €200; Monthly net €300; Recognized incoming €0; Recognized net −€200. |
| AC10 | Paid cancellation/no-show Net €90, scheduled Aug 31–Sep 3. | Full €90 recognized in August, zero in September. Existing validation and status handling apply. |
| AC11 | Payout-backed booking has invalid/missing stay dates but a valid payout date and Net €50. | Cashflow includes €50 in the payout month. Recognition excludes it and the existing issues display explains why. |
| AC12 | Statement import has changed the canonical stay dates relative to the original payout CSV. | Recognition follows the current stored dates, preserving existing import precedence. |
| AC13 | Canonical payout Net €70 with no linked transaction; another payout has zero Net and an obsolete linked nonzero payout transaction. | Payout income uses €70 plus €0. No missing/extra amount from linked transaction presence or stale payout transaction values. |
| AC14 | Property income €0 or negative, Cleaner expense €10. | Cleaner expense remains €10; Cleaner margin is 0.0%, never NaN or infinity. |
| AC15 | Rent €900, eligible expenses €500, corrected Recognized net €100. Then change Recognized net to −€50. | Long-term main figure initially −€300, then −€400; subtitle remains Long term rent: €400. Existing tone/availability rules apply. API difference uses the corrected Recognized net. |
| AC16 | Mixed payout corrections, categorized/uncategorized other movements, multiple properties/months. | Breakdown columns reconcile to their monthly totals; outgoing chart excludes payout deductions. Monthly outputs exclude other months; lifetime outputs cover all months for the selected property. Every output excludes other properties. |
| AC17 | Same current property/month loaded on Overview, Revenue, and Dashboard. | All recognized-incoming figures agree and use Net. Revenue recognized rows sum to the KPI. Dashboard cash net equals Overview Monthly net. |
| AC18 | A different property/month is selected or a transaction/rent/sync reload completes. | Existing refresh/availability behavior recomputes the affected figures without stale selection results or gross fallback. Unsynced generated entries retain provisional messaging. |
| AC19 | Inspect all eleven Overview cards, Revenue KPI, and both Dashboard Finance KPIs. | Each has calculation help; section/card/table labels and footnotes match section 6. No remaining affected gross label is bound to Net. |
| AC20 | Keyboard, mouse, and touch interaction with KPI help, including mobile viewport and multiple cards. | Hidden by default, accessible labeled trigger, hover/focus reveal, click/tap and Enter/Space operation, Escape dismissal, unique associations, readable unclipped text. |

### Verification commands

Run targeted tests during development, followed by the repository's required completion checks. From `backend/`:

```sh
go test ./internal/store ./internal/api ./internal/finance/statements
go test ./...
go build ./...
```

From `frontend/`:

```sh
npm run types:openapi
npm test
npm run type-check
npm run lint
npm run build
```

Inspect desktop/mobile Finance Overview, Revenue, Breakdown, and Dashboard with positive, zero, and negative results. Check the payout-versus-stay-month example and help with keyboard/touch. Report executed checks and any pre-existing failures accurately. Completion means the corrected definitions and explanations agree across the affected surfaces, not merely that a tile was renamed.
