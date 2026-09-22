# PMS 28 — Finance Overview: Recognized net

## 1. Document status and purpose

- **Date:** 2026-09-14.
- **Status:** Business calculation and display scope confirmed through three rounds of clarification. Technical design below is a codebase-grounded implementation proposal, not implemented functionality.
- **Audience:** Product owner, implementing engineer, reviewer, and QA.
- **Deliverable:** Specification only. This analysis did not change application code, database schema, or data, and did not execute imports or implementation tests.
- **Evidence baseline:** Current local working tree, including pre-existing uncommitted Finance, import, API-contract, and UI changes. Findings do not establish what is deployed in production. Older specifications are supporting context; inspected code takes precedence when describing current behavior.

### Business objective

Allow a Finance user to see a selected property's monthly management result beside **Recognized gross**, without manually combining stay-based booking proceeds with Finance transactions.

**User story:** As a user with Finance read access, I want a **Recognized net** KPI beside **Recognized gross** in Finance Overview, so I can assess the selected month's result after platform deductions and the other recorded incoming/outgoing movements.

### Approved definition

For property `P` and selected month `M`, in integer cents:

**Recognized net = recognized booking net + other incoming − other outgoing.**

- **Recognized booking net:** Current canonical payout-backed booking net, allocated using the same eligibility, dates, exception rules, and exact-cent algorithm as recognized gross.
- **Other incoming:** All incoming Finance transactions in `M` for `P`, except `source_type = 'booking_payout'`.
- **Other outgoing:** All outgoing Finance transactions in `M` for `P`, except `source_type = 'booking_payout'`.

This is the user's explicitly approved **hybrid management balance**: stay-period recognition for booking proceeds plus existing Finance transaction-month recognition for other movements. It is not a complete accrual profit-and-loss calculation.

## 2. Requirement challenges and confirmed decisions

The initial request used “net income,” which could mean net booking proceeds, income after commission only, or profit after operating expenses. The user first chose profit after operating expenses, then explicitly approved the broader all-transactions formula after its consequences were explained.

| ID | Decision confirmed by user | Consequence |
|---|---|---|
| D01 | Display label is **Recognized net**. | Do not label it merely “Net payout” or substitute existing Monthly net. |
| D02 | Display in **Finance Overview only**, beside Recognized gross. | No new net KPI or net columns in the Revenue tab or Dashboard. |
| D03 | Use the existing gross-recognition eligibility and timing. | Payout-backed only; checkout-exclusive nights; include unmatched records; cancellations/no-shows in scheduled check-in month; invalid windows excluded. |
| D04 | Use recorded `finance_bookings.net_cents`. | Do not calculate net again from gross less commission/fee fields. |
| D05 | Allocate net independently with the existing exact-cent algorithm, including signed negative values and real zeros. | Gross and net each conserve their own booking total across months. |
| D06 | Recalculate original stay months after corrections. | Current historical results can change after a late payout, commission correction, refund, or corrected canonical booking data. |
| D07 | Include **all non-booking_payout outgoing transactions**. | Includes manual, recurring, cleaning salary, uncategorized entries, and any recorded tax, equipment, or loan payments. No operating-expense category filter. |
| D08 | Include **all non-booking_payout incoming transactions**. | Includes direct receipts, reimbursements, owner contributions, and incoming recurring entries if recorded. |
| D09 | Use transaction month for these other movements. | An August electricity bill recorded with a September transaction date reduces September, not August. |
| D10 | Match existing Finance month membership exactly. | Use the stored timestamp's `YYYY-MM`, consistent with current list/summary queries; do not reinterpret these transactions in property-local time. |
| D11 | For a Not synced month, show a provisional numeric result based on stored entries. | No automatic generation or estimation of missing recurring/cleaning transactions on read. |
| D12 | Accept the hybrid-balance consequences explicitly. | Owner funding increases this KPI; loan principal/equipment purchases decrease it; a month without recognized bookings can still have a nonzero result. |

**Terminology resolution:** The earlier suggested hint “After platform deductions; before operating expenses” is inapplicable after D07–D12. UI copy must explain that other Finance movements are included. “Synced” must not be presented as audited, complete, or locked profit.

## 3. Verified current-state analysis

### 3.1 Presentation and request flow

1. `frontend/src/views/FinanceView.vue` owns the property/month selection and `loadAll()`.
2. `loadAll()` concurrently requests categories, transactions, summary, recurring rules, and `GET /api/properties/{id}/finance/revenue-recognition?month=YYYY-MM`.
3. The recognition response currently contains `month`, `gross_revenue_cents`, `bookings`, and `excluded_bookings`. FinanceView initializes/normalizes these values and passes only the gross total to Overview.
4. `frontend/src/views/finance/FinanceOverviewTab.vue` renders a **Revenue recognition** section with a single **Recognized gross** `UiKpiCard`. Its existing responsive grid supports a second card.
5. The same view separately renders **Monthly net**, defined by the cash ledger, and shows generated-entry sync status in the parent toolbar.
6. `frontend/src/views/finance/FinanceRevenueTab.vue` displays a gross total, gross booking detail, and invalid-window issues. The Dashboard also consumes recognized gross, through a separate Dashboard contract.

The Overview description currently says gross is allocated over “occupied nights.” That omits the existing cancellation/no-show exception. The proposed description in section 6 describes both the existing gross and the new hybrid result more accurately.

### 3.2 Gross-recognition calculation to reuse

`backend/internal/store/finance_revenue_recognition.go`, `ComputeFinanceRevenueRecognition`:

- Selects `finance_bookings` scoped to property with `has_payout_data = 1`.
- Reads canonical `amount_cents` as gross, treating a missing gross as zero.
- Uses finance-booking check-in/check-out dates, not payout date or a substituted named-stay date.
- Does not require a linked transaction or named stay. A missing named-stay link is reported as unmatched.
- Does not additionally filter `source_channel` in this query. Preserve the existing actual cohort rather than silently introducing a new channel filter based on its Booking.com UI wording.
- Validates the stay window before processing any status exception. Missing/invalid dates and non-positive windows are excluded.
- Uses canonical status, falling back to reservation status when blank, plus the existing outcome-override/named-stay outcome resolution.
- Recognizes cancellation/no-show amounts entirely in scheduled check-in month, with zero recognized occupied nights.
- Otherwise allocates across property-local calendar nights in `[check-in, checkout)`, intersected with the selected month. It counts calendar dates rather than dividing elapsed hours by 24.
- `allocateFinanceGross` splits integer cents evenly and assigns signed remainder cents to earliest nights. It already supports negative amounts.
- Collects invalid-window issues from the property-wide payout-backed population, even if an invalid record cannot be assigned to the selected month.
- Reads current canonical values on each request; there is no frozen recognition snapshot.

### 3.3 Ledger and generated entries

`backend/internal/store/finance.go`:

- `ListFinanceTransactions` and `ComputeFinanceSummary` select the month with `substr(transaction_date, 1, 7)`.
- Monthly net is all incoming minus all outgoing, including `booking_payout` entries.
- `FinanceTransaction` provides property, date, direction, amount, category, and source type. It has no separate expense service-period field.
- `FinanceCategory` has `counts_toward_property_income`, but no operating-profit inclusion flag. That existing income flag must not be reused to narrow the approved all-transactions calculation.
- Generated recurring and cleaning entries use `source_type = 'recurring_rule'` and `source_type = 'cleaning_salary'`.
- `GetFinanceGeneratedEntrySync` exposes existing month sync state; `SyncFinanceGeneratedEntriesForMonth` persists/reconciles generated entries.
- Some summary aggregate errors are currently ignored. The new KPI computation must propagate failures rather than inherit that behavior and display a misleading zero.

`backend/internal/store/finance_bookings_merge.go`, `UpsertBookingFinanceTransaction`, represents a positive payout as incoming and a negative payout as outgoing with an absolute stored amount. Both directions must be excluded from the new ledger adjustment.

### 3.4 Canonical data and ongoing import work

- The current finance-bookings schema in `backend/internal/migrate/000039_legacy_occupancy_removal.up.sql` defines `net_cents INTEGER NOT NULL`. A zero value is valid; `has_payout_data` establishes payout-backed eligibility rather than `net_cents > 0`.
- `backend/internal/finance/statements/merge.go` currently contains commission-adjustment and refund branches that modify canonical Net. Normal payout merge writes Net, while statement merge can independently update commission/fees and dates.
- Therefore gross minus currently stored commission/fee fields is not a guaranteed substitute for canonical Net. Deducting platform VAT/commission again would also double-count costs already included in Net.
- `spec/PMS_27_Booking_Payout_Commission_Adjustments_and_Refunds_Spec.md` provides related correction requirements. Its current-state description predates some code now present in the working tree. This analysis verifies the current merge branches, not the completeness of durable replay/correction handling across the entire importer.
- This feature consumes the effective canonical values. It cannot make an incorrect upstream payout correction or repeated import financially correct by recalculating the KPI differently.

## 4. Scope and boundaries

### Required outcomes

1. Calculate the approved result for the selected property/month in the backend.
2. Expose an additive Finance API contract for the total and its monetary components.
3. Render the new Overview KPI directly after Recognized gross, with explanatory and provisional-state copy.
4. Refresh it through the existing Finance loading/mutation/sync lifecycle.
5. Verify exact-cent allocation, source exclusions, historical changes, authorization, and UI states.

### Explicit scope boundaries

- Revenue-tab presentation, Dashboard presentation, existing Monthly net, recognized gross, property income, and cleaner margin retain their existing meanings.
- No category-selection workflow, service-period accounting, tax calculation, depreciation, loan classification, closed-month snapshot, export, or new report page is part of this feature.
- No booking-level “profit after operating expenses” is defined. Other ledger movements apply at property/month level, not to individual booking rows.
- No schema migration or historical backfill is required by the confirmed formula; its inputs already exist. Source-data repair and adjustment-import correctness are separate work.
- Existing EUR Finance formatting is reused. No exchange-rate or multi-currency consolidation behavior is introduced by this specification.

## 5. Detailed calculation requirements

### FR01 — Eligible recognized booking net

For each booking included by the existing gross computation for `P/M`, allocate its current signed `net_cents`:

- Normal stay: use the same total nights, overlap offsets, and signed remainder allocation as gross, independently applied to net.
- Cancellation/no-show: use the entire net in scheduled check-in month only.
- Unmatched payout-backed booking: include according to the same date/status rules.
- Statement-only booking: exclude even if a stored amount or net-like default exists.
- Invalid stay window: exclude from both recognized booking gross and net under existing issue reasons.

Sum the booking contributions to obtain `recognized_booking_net_cents`. Do not cap net at zero or gross, infer a commission percentage, or subtract commission, payment fee, or platform VAT again.

### FR02 — Other monthly incoming/outgoing

Select `finance_transactions` for the same property where:

- `substr(transaction_date, 1, 7) = M`; and
- `source_type <> 'booking_payout'`.

Sum stored `amount_cents` by direction, following existing Finance transaction semantics:

- Incoming contributes to `other_incoming_cents`.
- Outgoing contributes to `other_outgoing_cents`.
- Empty sets contribute zero.

The inclusion rule is independent of category, category activity, `counts_toward_property_income`, stay mapping, and auto-generated status. Do not inner-join categories in a way that drops uncategorized entries.

Exclude **every** booking_payout transaction in both directions, including unmatched/orphaned entries and payouts whose booking is excluded from recognition. Do not add those back as “other income” to compensate for invalid stay data.

### FR03 — Final total and double-counting controls

`recognized_net_cents = recognized_booking_net_cents + other_incoming_cents - other_outgoing_cents`

- Do not use `monthly_net_cents` as the new value.
- Do not add full Monthly net to recognized booking net: that would include booking payouts a second time.
- Do not deduct full `monthly_outgoing_cents`: that would double-count negative booking payouts already reflected in canonical recognized net.
- Do not deduct `cleaner_expense_cents` separately: stored cleaning salary already participates in other outgoing.
- Do not generate expense rows during this read operation.

For consistent ledger data, an audit identity is:

`Recognized net = Monthly net - signed booking_payout ledger total for M + recognized booking net for M`.

This identity is a reconciliation aid; the direct source-filtered formula is the proposed implementation to avoid dependencies on separately loaded totals.

### FR04 — Timing and historical recalculation

- Property-local stay-month boundaries continue to apply to booking recognition.
- Stored timestamp month continues to apply to other transactions, matching the current Finance list/summary even near timezone boundaries.
- Later canonical booking net corrections reallocate the full corrected net to original stay months; no correction-month posting is introduced in this KPI.
- Existing changes to canonical dates/status/outcome may change the cohort in exactly the way gross currently changes.
- Editing/deleting/creating a qualifying transaction changes its Finance-month component on the next successful load.
- A payout received after the stay can introduce recognized net into an earlier stay month once payout-backed data exists. This is not a forecast of unimported payouts.

### FR05 — Empty and negative results

- No eligible bookings and no other movements: display EUR zero.
- No eligible bookings but €50 other outgoing: display −€50, not an empty state.
- No eligible bookings but €500 owner contribution: display €500 under D08/D12.
- Negative canonical booking net reduces the result under the same stay allocation.
- Net may exceed recognized gross because other incoming movements are included. This is not intrinsically an error.

### FR06 — Provisional state

When existing generated-entry status is `not_synced`, display the stored-data numeric result and mark it **Provisional**. Explain that generated recurring and cleaning entries may be missing or outdated.

After successful explicit sync and reload, use the updated persisted movements and clear the Not synced provisional indication when the returned status is `synced`.

Sync state records the existing workflow status; it does not certify that all invoices/imports/expenses are present or that inputs have not changed since the last sync. Do not introduce a new stale-state detector or an accounting “final” state under this feature.

## 6. UI and interaction specification

### Layout

- Location: **Finance → Overview → Revenue recognition**.
- Order: existing **Recognized gross**, then new **Recognized net**.
- Use existing `UiKpiCard`, EUR formatting, spacing, and responsive grid. On narrow screens, cards may stack, preserving gross-before-net order.
- Keep amounts fully readable at supported mobile widths. Convey negative/provisional states through text/value, not color alone.

### Proposed copy

- Section description: **“Booking revenue follows stay-recognition rules. Recognized net also includes other Finance movements in the selected month.”**
- New KPI label: **“Recognized net”**.
- New KPI hint: **“Stay-based booking net + other incoming − other outgoing.”**
- Provisional text: **“Provisional — generated entries have not been synced for this month.”**
- Accessible supporting text, where needed: **“Uses currently stored entries. Sync generated entries to update recurring and cleaning amounts.”**

These strings are implementation copy proposals expressing the confirmed decisions, not a claim that the user approved each word.

### Display states

| State | Required behavior |
|---|---|
| Loaded positive/negative/zero result | Render the exact signed EUR value; use existing appropriate KPI tones. Do not describe it as accounting profit. |
| Loaded Not synced month | Render value plus visible Provisional text. |
| No selected property | Follow existing Pick a property state; do not show a fabricated result. |
| Initial load / property or month change | Do not show zero or the prior selection's result as the newly selected month's computed value. Use a loading/unavailable presentation. |
| Failed required request | Use existing error presentation and make the new result unavailable; failure is not zero and is not merely a provisional result. |
| Missing required new API fields | Treat as unavailable/incompatible data, not `0` through a falsy fallback. |
| Refresh after mutation or successful sync | Update amount and provisional status together for the active property/month. |

`FinanceView.loadAll()` currently permits overlapping loads and retains old data during requests/errors. Integration must ensure a late response for a previous selection cannot overwrite the active new KPI, for example through request identity checks. These are correctness requirements for the new display, not a request for a new global loading framework.

No drilldown or additional net columns are required. Backend monetary components support verification without assigning property-wide expenses to individual bookings.

## 7. Proposed API and technical design

### 7.1 Additive response extension

Extend the existing Finance recognition endpoint:

`GET /api/properties/{id}/finance/revenue-recognition?month=YYYY-MM`

Keep existing fields and gross booking-row semantics. Proposed new required fields:

| JSON field | Type | Meaning |
|---|---|---|
| `recognized_booking_net_cents` | integer, int64 in OpenAPI | Sum of recognized canonical booking net before other movements. |
| `other_incoming_cents` | integer, int64 in OpenAPI | Selected-month incoming excluding booking_payout. |
| `other_outgoing_cents` | integer, int64 in OpenAPI | Selected-month outgoing excluding booking_payout. |
| `recognized_net_cents` | integer, int64 in OpenAPI | Final approved hybrid result. |

Successful responses include explicit zero values where appropriate. Avoid calling the final field `net_revenue_cents`: it includes owner contributions and ledger expenses, not just booking revenue.

For provisional display, reuse the existing `FinanceSummary.generated_entry_sync` already fetched by FinanceView. A second persisted sync state or duplicate recognition-response status is unnecessary. Keep active property/month identity aligned between the two responses. Exact transaction/snapshot organization is an engineering design choice; do not assemble one KPI from different selected months or knowingly inconsistent financial snapshots.

No new per-booking public net fields are necessary for the confirmed Overview-only scope. Internal booking-net contributions can be retained for calculation/testing without expanding the gross-only detail UI.

### 7.2 Backend responsibilities

- Extend the existing gross-recognition read to retrieve canonical Net and allocate it in the same loop. Reuse/generalize the signed cent allocator without changing existing gross results.
- Add a property/month/source-filtered ledger aggregate for other incoming/outgoing; calculate the final result in the backend.
- Prefer a small composition layer for the Finance-specific ledger adjustment so Dashboard's shared gross call need not acquire an unrelated expense-query dependency.
- Propagate query/scan failures. Use one bounded aggregate rather than one expense query per booking.
- Reuse Finance read access, property isolation, existing month validation, timezone fallback, and error conventions.
- A read request must not persist transactions, sync month state, alter bookings, or rewrite invoices.

### 7.3 Contract and rollout

- Update `spec/openapi.yaml` and handwritten Finance TypeScript interfaces together with the backend response.
- Regenerate `frontend/src/api/types/generated.ts` with `npm run types:openapi` from `frontend/` during implementation; do not hand-edit generated types.
- Update FinanceView's initial/load state and Overview props for final net and availability/provisional behavior.
- Existing consumers may ignore additive response fields. Deploy the updated backend before or together with the updated frontend; a new frontend receiving the old response must show unavailable rather than falsely reporting €0.
- No persisted recognition totals, new table, migration, backfill, or reset handling is necessary for this derived result.

## 8. Implementation impact map

Paths are relative to repository root. “Required” describes future implementation work, not edits performed in this documentation task.

| Area / file | Impact |
|---|---|
| `backend/internal/store/finance_revenue_recognition.go` | **Required:** Read/allocate canonical Net alongside gross; expose booking-net total; preserve all existing cohort and rounding rules. |
| `backend/internal/store/finance.go` or a narrowly scoped Finance recognition store helper | **Required:** Add reliable non-booking_payout monthly incoming/outgoing aggregation and compose final net; reuse current month semantics. Exact helper placement is an engineering choice. |
| `backend/internal/api/finance_handlers.go` | **Required:** Extend recognition response type and handler mapping/composition. Preserve Finance read authorization and validation. |
| `spec/openapi.yaml` | **Required:** Document required new integer-cent totals and their definitions in FinanceRevenueRecognitionResponse. |
| `frontend/src/api/types/finance.ts` | **Required:** Extend recognition response interface. |
| `frontend/src/api/types/generated.ts` | **Required:** Regenerate from OpenAPI. |
| `frontend/src/views/FinanceView.vue` | **Required:** Preserve new response fields, pass net/state to Overview, prevent misleading fallback/stale selection values, reload through existing actions. |
| `frontend/src/views/finance/FinanceOverviewTab.vue` | **Required:** Add net KPI, correct explanatory copy, render provisional/availability states. |
| `backend/internal/store/finance_revenue_recognition_test.go` | **Required verification:** Net allocation, signed/zero values, eligibility, status exceptions, conservation, and correction recalculation. Existing helper currently sets net equal to gross; new fixtures must deliberately differ. |
| `backend/internal/store/finance_test.go` and `finance_sync_test.go` | **Required verification:** Source-filtered ledger adjustments, transaction month, empty results, sync effects, isolation, and no duplicate cleaning deduction. |
| `backend/internal/api/finance_revenue_recognition_test.go` | **Required verification:** New totals, additive response, permissions/property scope, malformed month, failures and read-only behavior. Existing API fixture already has gross 10000 and net 8000. |
| `frontend/src/views/FinanceView.spec.ts` | **Required verification:** Overview placement, exact value, provisional/loaded/error states, month/property switching and response mocks. |
| `backend/internal/api/server.go`, `server_response_types.go`, `server_test.go` | **Shared-consumer regression review:** Dashboard uses the gross store calculation. Preserve its existing values and response; no new Dashboard net field requested. |
| `frontend/src/views/finance/FinanceRevenueTab.vue`, `frontend/src/views/dashboard/DashboardHeroKpis.vue`, `frontend/src/api/types/dashboard.ts` | **Regression only:** Presentation/contracts for these surfaces are not expanded. |
| `backend/internal/finance/statements/merge.go`, `backend/internal/api/finance_imports_handlers.go`, `backend/internal/store/finance_bookings_merge.go` | **Upstream dependency verification:** Effective Net/correction and booking_payout source tagging must feed the new result accurately. Importer redesign is not a prerequisite created by this spec. |
| `backend/internal/store/finance_reset.go` | **Regression scenario:** Reset preserves cleaning salary; the derived new KPI can therefore be negative with zero recognized gross afterward. No new stored KPI to delete. |

## 9. Acceptance criteria and exact examples

Fixtures below are synthetic, deterministic examples, not assertions about local or production balances. All amounts are EUR and must be tested as integer cents. “Other” always excludes booking_payout.

| ID | Given / when | Expected result |
|---|---|---|
| AC01 | One one-night payout-backed booking: gross €100, recorded Net €78. Same month: manual outgoing €15, recurring outgoing €10, cleaning outgoing €5, other incoming €12. | Recognized gross €100; recognized booking net €78; other incoming €12; other outgoing €30; **Recognized net €60**. Cleaning is counted once. |
| AC02 | AC01 also has an incoming booking_payout ledger entry of €78 in that month. | Result remains **€60**, not €138. |
| AC03 | A one-night booking has canonical Net −€15.14 and an outgoing booking_payout ledger entry €15.14 in its recognition month; no other movements. | Recognized net **−€15.14**, not −€30.28. |
| AC04 | Stay Jan 31–Feb 3, 2026: gross 10000 cents, Net 8000 cents; payout in March. No other movements. | January gross 3334, booking/final net **2667**; February gross 6666, booking/final net **5333**; March recognition zero. Net across stay months sums to 8000. |
| AC05 | AC04 with Net −100 cents. | January net **−34**; February net **−66**; total −100. A separate true-zero Net booking contributes zero without being treated as missing evidence. |
| AC06 | Cancellation Apr 30–May 2 with gross €25 and Net €20; no other movements. Equivalent no-show fixture. | April net **€20**, May booking net zero. Existing exception flags and zero occupied recognized nights remain consistent with gross. |
| AC07 | Same amounts in a statement-only booking; separate payout-backed unmatched booking; separate invalid-window payout-backed booking. | Statement-only excluded; unmatched valid booking included; invalid-window booking excluded from gross and booking net with existing issue reason. Its payout ledger entry is not added as other income. |
| AC08 | An August electricity expense €40 has stored transaction date in September. | September other outgoing increases €40; August unaffected. A timestamp near a local-time month boundary follows the existing stored `YYYY-MM` exactly. |
| AC09 | No eligible bookings. Owner contribution incoming €500; loan/equipment outgoing €200; both non-booking_payout. | Recognized gross zero, **Recognized net €300**. Category, uncategorized status, and property-income flags do not change this result. |
| AC10 | No bookings/other movements; separately, no bookings and other outgoing €50. | First case numeric **€0**; second case **−€50**, not an empty-bookings suppression. Provisional labeling follows sync status independently. |
| AC11 | Before sync: recognized booking net €100, stored outgoing €10, month Not synced. Sync then creates additional recurring €20 and cleaning €5. | Before: **€90, Provisional**. After successful sync/reload: **€65**, returned synced status, no Not synced provisional indication. Repeated reads create no rows. |
| AC12 | AC04 Net is later corrected from 8000 to 7700 cents, with stay dates unchanged. | Recomputed January **2567**, February **5133**; total 7700. No new correction-month recognition. Gross remains 10000 if correction changes only Net. |
| AC13 | A later refund changes both canonical gross and Net. | Both recognized measures follow their current canonical values over the existing recognition periods; do not deduct the refund again from other movements tagged booking_payout. |
| AC14 | Change property/month quickly; an older request resolves last. A separate load fails or omits new response fields. | New KPI is never assigned to the wrong selection. Failed/incompatible load shows unavailable/error, not previous-selection net or zero. A valid numeric zero still displays zero. |
| AC15 | User without Finance read access requests endpoint; authorized user switches between two properties containing identical references/transaction dates. | Existing authorization is enforced; no cross-property amounts or issue data leak. Malformed month retains existing 400 behavior. |
| AC16 | Same scenario queried through existing Revenue tab, Dashboard, and Monthly net consumers. | Existing gross totals/rows and cash formulas remain correct. No net presentation is added to those other surfaces. |
| AC17 | Finance reset removes bookings and other transactions but preserves €25 cleaning salary for a month. | Next load reports gross zero and **Recognized net −€25**, with provisional state determined by the returned month sync state. |
| AC18 | Net allocation crosses leap day, year end, or a daylight-saving transition. | Checkout-exclusive calendar-night allocation conserves signed net exactly; elapsed-hour changes do not alter night count. |
| AC19 | Non-booking aggregate query fails while booking recognition succeeds. | Endpoint returns an error; it does not emit a successful final net based on zero/default expense components. |
| AC20 | A qualifying other transaction is edited, deleted, or added through the existing Finance UI. | Successful reload updates the correct monthly component and final KPI without changing the recognized booking-net allocation. |

### Verification plan for implementation

1. Store-level tests establish independent net allocation, ledger source filtering, exact monetary identities, sync behavior, and failure propagation.
2. API tests establish payload totals, permission/month behavior, additive compatibility, and read-only requests.
3. Vue tests establish the Overview-only KPI and provisional/loading/error/selection behavior. Use different gross, booking net, and final net values so tests cannot accidentally accept the wrong metric.
4. Regression tests protect shared gross computation, Dashboard gross, Monthly net, correction consumers, and existing reset/sync behavior.
5. Manual review at desktop/mobile widths checks adjacent card order, readable signed values, explanatory copy, and the Not synced → sync → reload transition.

Relevant existing commands, to run during implementation rather than this documentation task:

- From `backend/`: `go test ./internal/store ./internal/api`.
- From `frontend/`: `npm run types:openapi`, `npm run type-check`, and `npm test -- src/views/FinanceView.spec.ts src/views/DashboardView.spec.ts`.
- Run repository-required lint/build or broader checks as appropriate to the eventual implementation diff. The commands above are a starting verification scope, not a claim of passing results.

## 10. Dependencies, limitations, and readiness

### Documented data dependencies

1. **Canonical payout accuracy:** Recognized booking net is only as accurate as current imported/corrected `net_cents`. Verify correction fixtures against the actual implementation state of PMS 27.
2. **Transaction provenance:** Double-counting prevention uses `source_type`, not notes or category names. If an operator manually records an imported payout again with source `manual`, the approved formula includes that manual entry. No duplicate inference is specified.
3. **Generated-entry freshness:** Provisional state follows existing sync metadata, not a new completeness detector. A synced month can still lack a late import or manually omitted expense.
4. **Currency:** Current recognition does not expose currency grouping/conversion and Finance presents EUR. This feature must not claim to reconcile mixed-currency data; a multi-currency requirement would need its own rules.
5. **Working-tree drift:** Finance/import code is already modified locally. Recheck function signatures/contracts before implementation and preserve concurrent work.

### Readiness and residual decisions

No unanswered business question remains for the explicitly confirmed formula, inclusion rules, date basis, historical behavior, label, provisional behavior, or Overview-only scope.

Remaining engineering choices are implementation details, not permission to redefine the metric: helper composition, read-snapshot organization, internal allocator naming, UI loading representation, and final concise helper wording. The proposed API field names and impact map provide a concrete starting design for review.

### Definition of done for the future feature

- D01–D12 and FR01–FR06 are reflected in implementation and contract documentation.
- AC01–AC20 are covered at appropriate test/review levels, with exact-cent expected results.
- Finance Overview shows the final hybrid value, not merely booking net or Monthly net.
- Required fields/types are synchronized and failures never masquerade as zero.
- Shared gross/cash behavior and property authorization remain verified.
- No unrequested reporting surface, accounting-period model, or persistent KPI storage is introduced.

This document is the sole new artifact produced for the current request.
