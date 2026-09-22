# PMS 29 — Finance: Short-term vs long-term rental comparison

## 1. Status and objective

- **Date:** 2026-09-15.
- **Status:** Business rules confirmed by the product owner through two clarification rounds. Ready for implementation; API/schema details below are codebase-grounded engineering proposals.
- **Audience:** AI coding agent, implementing engineer, reviewer, and QA.
- **Evidence baseline:** Current local working tree, including existing uncommitted Finance/recognition/import changes. This document does not establish production deployment status.
- **Deliverable:** Feature specification. Application implementation and test execution are future work.

**User story:** As a property administrator, I want to configure a hypothetical monthly long-term rent and see its net result beside Revenue recognition metrics in Finance Overview, so I can see whether short-term rental performed better and by how many euros.

For selected property `P` and calendar month `M`:

> Long-term net = applicable monthly rent − eligible monthly outgoing.
>
> Short-term difference = existing Recognized net − long-term net.

Positive difference means short-term is ahead; negative means behind; zero means equal.

This is a deliberately simple management benchmark. Rent includes utilities, and actual recorded non-cleaning expenses are reused as the hypothetical long-term expenses. It is not a separately modeled long-term rental budget.

## 2. Confirmed decisions

| ID | Product-owner decision | Required consequence |
|---|---|---|
| D01 | The €900 rent / €250 non-cleaning / €100 cleaning / €1,100 booking-net example is exactly the desired model. | Long-term net €650; Recognized net €750; short-term ahead €100. |
| D02 | Rent includes utilities; no further utilities breakdown. | Deduct recorded utility expenses along with other eligible outgoing. |
| D03 | Every non-cleaning outgoing reduces the estimate, subject to the booking-payout exclusion. | Include manual, recurring, uncategorized, tax, equipment, loan, and short-term-specific expenses. No configurable expense inclusion list. |
| D04 | Exclude negative booking payouts from long-term deductions. | Exclude outgoing transactions with `source_type = 'booking_payout'`. They already affect short-term booking recognition. |
| D05 | Cleaning exclusion is generated salary plus manual/recurring transactions in the cleaning-salary category. | Exclude by generated source OR cleaning-salary category, without subtracting overlapping matches twice. |
| D06 | Compare against existing Recognized net exactly. | Owner contributions/reimbursements and other incoming remain included on the short-term side according to PMS 28. |
| D07 | Rent has effective-from-month history. | A new rate changes its effective month onward until the next rate, preserving earlier rates. |
| D08 | Initial setup starts from an explicitly chosen month, prefilled with the selected Finance month. | Earlier months are unconfigured, not zero-rent months. No automatic backfill. |
| D09 | Historical and future rates may be added; entries may be corrected/deleted. | Recalculate affected comparisons; deleting a rate extends its predecessor, if any, to the next remaining rate. |
| D10 | Rent is always positive because it is a benchmark. | Accept positive cent-precision values; reject zero/negative rent. No stop-comparing schedule is introduced. |
| D11 | Manage rent from an action beside the Finance Overview card. | Dialog with amount, effective month, and history. |
| D12 | Property owners, super-admins, and Finance admins may manage rates. Finance readers may see the comparison and assumption. | Finance write alone does not authorize benchmark edits. |
| D13 | Display long-term net and the signed comparison beneath it, in euros only. | No percentage metric. Include compact rent-minus-expenses explanation. |
| D14 | Negative results compare mathematically. | Short-term −€100 versus long-term −€300 is ahead by €200. Do not imply either is profitable. |
| D15 | Current months use full monthly rent with currently stored data. | No proration or month-to-date conversion; show Month in progress. |
| D16 | Future months also show currently stored data. | Show numeric comparison when configured, with future-month qualification. No prediction of missing revenue/expenses. |
| D17 | Unsynced months show numeric results with a provisional indication. | Reuse existing generated-entry sync state; reads never generate transactions. |
| D18 | Finance reset preserves rent history. | Recalculate from remaining financial data after reset. |

### Challenges explicitly accepted

1. Common expenses deducted on both sides cancel in the difference. The card still shows the useful long-term net amount, while the difference largely measures whether booking proceeds and other incoming, after cleaning, beat rent.
2. Short-term-specific non-cleaning expenses also reduce hypothetical long-term net under this simple rule.
3. Owner funding recorded as incoming can improve the reported short-term result. This feature does not redefine Recognized net.
4. Full monthly rent is compared to incomplete data in current/future months. Labels must explain the context rather than silently prorating or forecasting.
5. Historical comparisons remain dynamic after corrected transactions, payout recognition, classification, or rent-history edits. Rate history is not a frozen results ledger.

## 3. Verified codebase findings

### Finance presentation and calculation

- `frontend/src/views/FinanceView.vue` owns selected property/month, loads Finance data, and passes recognition totals to Overview. The current working tree includes a `loadSequence` mechanism and recognition availability state; preserve/extend these protections.
- `frontend/src/views/finance/FinanceOverviewTab.vue` already has **Recognized gross** and **Recognized net** cards in **Revenue recognition**, a responsive `kpi-grid`, and an unsynced provisional note.
- `backend/internal/api/finance_handlers.go` exposes recognition totals including `recognized_booking_net_cents`, `other_incoming_cents`, `other_outgoing_cents`, and `recognized_net_cents`.
- `backend/internal/store/finance_revenue_recognition.go` computes payout-backed stay recognition. `ComputeFinanceOtherMovements` aggregates non-booking-payout transactions using `substr(transaction_date, 1, 7)` and propagates query errors.
- PMS 28 defines Recognized net as recognized booking net + other incoming − other outgoing. Reuse the implemented result and its timing/eligibility rules; do not reproduce booking recognition independently.

### Cleaning classification trap

- `backend/internal/store/finance.go`, `ComputeFinanceSummary`, calculates existing `CleanerExpenseCents` using source `cleaning_salary` and `source_reference_id = month`.
- That summary is **not** the required cleaning exclusion: it misses manual/recurring cleaning-category entries and does not use the same transaction-month predicate.
- `backend/internal/migrate/000009_finance.up.sql` seeds category code `cleaning_salary`. Transaction provenance and category are separate fields.
- Implement the new exclusion using source/category codes, not displayed category title, notes, or the existing Cleaner expense KPI.

### Settings, permissions, and lifecycle

- `store.Property` has no rent benchmark field/history. A single new property amount would not satisfy effective-month history.
- `backend/internal/permissions/modules.go` defines Finance read/write/admin levels. `store.UserCan` grants owners admin-equivalent access and active super-admins access, otherwise evaluates module grants.
- `backend/internal/api/property_access.go`, `requirePropertyModuleAccess`, is the existing property-scoped authorization helper.
- `backend/internal/api/server.go` registers property-scoped Finance routes.
- `backend/internal/store/finance_reset.go` implements reset and cleaning preservation. New benchmark history must survive this operation.
- Follow `spec/PMS_13_Coding_Conventions.md`, including sequential up/down migrations, integer cents, typed APIs, design-system UI, and auditing privileged actions.

## 4. Calculation contract

### FR01 — Select the applicable rent

For property `P`, choose the rate with the greatest `effective_from_month` such that `effective_from_month <= M`.

- Effective months are canonical `YYYY-MM` calendar months, inclusive.
- Rate applies to the entire month regardless of length, leap day, occupancy, closed nights, or when it was entered.
- No qualifying rate: comparison state is **not configured**. Do not fall back to the earliest future rate or use zero.
- No rent assumptions are seeded for existing properties on migration.
- Month navigation never creates rate entries automatically.

### FR02 — Eligible outgoing

Consider transactions for `P` satisfying all of:

1. `direction = 'outgoing'`;
2. `substr(transaction_date, 1, 7) = M`, matching current Finance month semantics;
3. `source_type <> 'booking_payout'`;
4. `source_type <> 'cleaning_salary'`;
5. Referenced category does not have `code = 'cleaning_salary'`.

Sum stored `amount_cents` as `eligible_outgoing_cents`. Empty set is zero.

- Uncategorized transactions remain eligible. Use a left join or equivalent null-safe category lookup; do not lose uncategorized entries through an inner join or SQL NULL comparison.
- Cleaning exclusion applies regardless of source for a cleaning-category outgoing, and regardless of category for generated cleaning-salary outgoing.
- Inactive categories still classify their historical transactions. Do not filter on category activity or `counts_toward_property_income`.
- Category identity means exact machine code `cleaning_salary`, including global/property-specific category records using that code. A custom category merely titled “Cleaning” is insufficient.
- Each transaction is considered once. A generated salary row with the cleaning category is one exclusion, not two.
- All incoming transactions are ignored by the long-term calculation, including incoming cleaning reimbursements. Their effect on existing Recognized net remains unchanged.
- Expense timing is the stored transaction month, not a bill service period, source-reference month, payout recognition month, or newly converted timezone month.

### FR03 — Derived totals and outcome

All monetary arithmetic uses integer cents:

```text
long_term_net_cents = monthly_rent_cents - eligible_outgoing_cents
short_term_difference_cents = recognized_net_cents - long_term_net_cents

outcome = ahead  if difference > 0
          behind if difference < 0
          equal  if difference = 0
```

- Do not clamp either result at zero or use absolute values before deciding the outcome.
- Display the magnitude of the difference after “ahead by” / “behind by”.
- No tolerance threshold: one cent ahead is ahead, and only exact cent equality is equal.
- Rent is hypothetical configuration: saving it must not create incoming Finance transactions or recurring rules.
- Platform fees/commission/refunds must not be deducted again from Recognized net.

For valid current source semantics, a useful test identity is:

```text
difference = recognized_booking_net + other_incoming
             - excluded_cleaning_outgoing - monthly_rent
```

Here `excluded_cleaning_outgoing` is the subset of PMS 28 other outgoing excluded by FR02's cleaning rule, using the same transaction month. This is a reconciliation aid; the implementation should expose the direct long-term calculation rather than depend on the existing Cleaner expense summary.

### FR04 — Recalculation and data freshness

- Compute on read from current stored transactions, current Recognized net, and applicable rate. Do not persist derived monthly results.
- Reload after successful rate mutation, transaction mutation, Finance import, generated-entry sync, or reset through the existing Finance lifecycle.
- Historical payout corrections follow PMS 28 recognition; current category reassignment/code changes can alter the benchmark expense classification on subsequent reads.
- Reads must not sync entries, estimate missing recurring bills, or forecast booking income.
- Use the existing `FinanceSummary.generated_entry_sync` state for provisional display. Synced is not an assertion that all data is complete or frozen.
- Determine current/past/future month using the selected property's timezone and existing timezone fallback conventions. Preserve the different, existing stored-timestamp rule for expense membership.

## 5. Rent-history management

### Required interactions

- **Manage long-term rent** opens a property-scoped dialog from Finance Overview, including when the selected month is unconfigured.
- Show all existing rates with effective-from month and EUR monthly amount; order chronologically and identify the rate applicable to the selected month.
- Add form has monthly rent and effective-from month. Prefill the month from the active Finance selection; require an explicit amount.
- Allow past/future additions and edits to amount/effective month. Editing an entry loads its existing values.
- Allow deletion using the existing confirmation interaction, explaining that the preceding rate may apply instead.
- Save/delete successfully before closing or refreshing; validation/network errors preserve form data and show actionable feedback.
- Changing property while a dialog/request is open must never submit a rate to a different property or refresh the wrong property's result. Capture request property identity and close/reset context as appropriate.

### Validation and effective-month uniqueness

Engineering contract proposals implementing D07–D10:

- Amount is positive, integral cents. Accept common existing EUR decimal-input conventions, including decimal comma/point where supported, with at most two decimal places. Reject invalid/nonfinite input, unsupported precision, and amounts outside the backend's supported safe integer range; do not silently round.
- API uses integer cents, never a floating EUR value. Check arithmetic and JSON/TypeScript-safe range boundaries; frontend-only validation is insufficient.
- Effective month must be canonical `YYYY-MM`, using the supported Finance year range (currently 2000–3000). Reject malformed values such as `2026-1`.
- At most one rate per property/effective month. A conflicting create or edit is rejected with a field-level conflict, not silently treated as replacement. Edit the existing row explicitly.
- Editing/moving a rate recomputes timeline selection; no persisted end-month or overlapping interval repair is needed.
- Deleting the earliest rate leaves months before the next remaining entry unconfigured. Deleting all rates leaves the benchmark unconfigured for every month.
- No separate disable flag, zero-rent sentinel, or scheduled stop action is required.

### Authorization and audit

| Operation | Required access |
|---|---|
| View comparison, applied assumption, history | Finance read or higher |
| Add, edit, delete benchmark rates | Finance admin |

Existing active property owners and super-admins inherit access through `UserCan`. Finance write without admin must receive 403 on mutations. Property-settings admin alone is not a substitute for Finance admin.

Enforce permissions server-side for every request and property-scope every rate lookup/mutation. Audit successful privileged mutations through the existing audit facility with rate/property identity and meaningful before/after data. Use established error and audit conventions; no new approval workflow is required.

## 6. Overview presentation

### Card and copy

Location: **Finance → Overview → Revenue recognition**. Order: Recognized gross, Recognized net, **Long-term net**. Preserve order when cards stack on small screens.

Approved example content:

```text
Long-term net
€650.00
Short-term ahead by €100.00
€900.00 rent − €250.00 eligible expenses.
```

- Difference below the long-term amount is essential, not only a hover tooltip.
- Use existing EUR formatting, design-system cards/buttons/dialogs, and spacing tokens. The snippet illustrates content, not a required currency-symbol locale layout.
- Other outcome strings: **Short-term behind by €100.00**; **Same result as long-term rent**.
- Positive/negative/equal comparison may use success/danger/neutral styling. Meaning must also be in text; a positive difference does not mean positive profit.
- Compact explanation shows applied rent and deducted eligible outgoing. Supporting help explains: “Monthly rent less recorded outgoing, excluding booking payouts and cleaning salary/category expenses. Compared with Recognized net.”
- Show Manage long-term rent only to authorized managers, using backend-derived capability or the established permission mechanism; do not infer permission from role name alone.
- Readers still see the applied assumption; rate history may be exposed read-only through the same dialog without mutation controls.
- No additional Revenue-tab, Dashboard, annual total, percentage, export, or booking-level profit presentation is required.

### Display states

| State | Behavior |
|---|---|
| Configured and loaded | Show signed long-term net, outcome text, difference magnitude, and calculation explanation. |
| No applicable rate | Show **Long-term rent not configured**; suppress monetary comparison/outcome rather than showing €0. Managers can configure it. |
| Current month | Show numeric result plus **Month in progress**. Full monthly rent applies. |
| Future month | Show numeric result plus **Future month — based on currently recorded data**. Full monthly rent applies. |
| Generated entries not synced | Also show **Provisional — generated entries have not been synced.** Explain recurring/cleaning entries may be missing or outdated. |
| Past synced month | Show results without current/future/unsynced labels; do not call it final or audited. |
| Loading or changed property/month | Loading/unavailable presentation; do not attribute the previous selection's values to the new selection. |
| Required request fails or required fields missing | Show unavailable/error, not zero, not not-configured, and not merely provisional. |
| No selected property | Follow the existing property-selection empty state. |

Time-period and sync labels are independent: a future unsynced month has both qualifications. Existing shared recognition notes may be reused if their applicability to both net cards is unambiguous. Unconfigured states do not show an ahead/behind verdict even if financial data exists.

## 7. Proposed backend/API design

The following is a concrete implementation proposal, not a claim these routes or fields already exist. Engineering may refine organization while preserving the business contract and keeping OpenAPI/types aligned.

### 7.1 Persistence

Add a property-owned `finance_long_term_rent_rates` table in a new sequential up/down migration. Recheck the next migration number: the working tree already contains migration 000041.

Suggested columns:

| Column | Purpose |
|---|---|
| `id` | Integer primary key. |
| `property_id` | Required property FK, cascade on property deletion. |
| `effective_from_month` | Required canonical `YYYY-MM`. |
| `monthly_rent_cents` | Required positive integer, DB check `> 0`. |
| `currency` | Required EUR for this feature, consistent with existing Finance presentation; no FX behavior. |
| `created_at`, `updated_at` | UTC RFC3339 metadata. |

Unique index on `(property_id, effective_from_month)` supports uniqueness and applicable-rate lookup. Actor information is recorded through existing audit conventions; optional creator/updater columns are engineering detail.

- Do not add this table to Finance-reset deletions. Verify reset preserves it in both preview/execution semantics.
- An up migration creates schema only; no hardcoded €900 defaults, transaction backfill, or synthetic rates.
- Down migration drops this feature's schema using normal repository migration conventions.

### 7.2 Rate endpoints

Suggested routes under `/api/properties/{id}/finance/long-term-rent-rates`:

| Method/path | Request/result |
|---|---|
| `GET` collection | `{ rates: Rate[], can_manage: boolean }`, Finance read. |
| `POST` collection | `{ effective_from_month, monthly_rent_cents }`; return created Rate, Finance admin. |
| `PATCH /{rateId}` | Update amount and/or effective month; return saved Rate, Finance admin. |
| `DELETE /{rateId}` | Delete the property-scoped rate; use standard success envelope, Finance admin. |

Rate representation: `id`, `effective_from_month`, `monthly_rent_cents`, `currency`, `created_at`, `updated_at`.

Use existing HTTP conventions: 400 invalid input/month, 403 insufficient access, 404 rate absent for authorized property, 409 duplicate effective month, 500 persistence failure. No cross-property row disclosure through supplied rate IDs. Empty collection is a successful configured-history read, not an error.

### 7.3 Additive recognition response

Extend the existing endpoint:

`GET /api/properties/{id}/finance/revenue-recognition?month=YYYY-MM`

Add a required `long_term_comparison` object:

```json
{
  "status": "configured",
  "rate_id": 12,
  "effective_from_month": "2026-09",
  "monthly_rent_cents": 90000,
  "eligible_outgoing_cents": 25000,
  "long_term_net_cents": 65000,
  "short_term_difference_cents": 10000,
  "outcome": "ahead"
}
```

- `status`: `configured` or `not_configured`.
- Configured state requires all shown fields and correct integer-cent values. `outcome` is `ahead | behind | equal`.
- Not-configured state returns the same keys with rate/calculation/outcome fields null. Successful lack of configuration must be distinguishable from request failure or an old backend missing the object.
- Existing top-level `recognized_net_cents` remains the exact short-term operand; no alternative short-term profit field/calculation.
- Keep current recognition response fields and booking rows compatible.
- Compose benchmark calculations in the Finance endpoint/store layer, not the shared gross-recognition calculation used by Dashboard.
- Compute eligible outgoing in a bounded aggregate with proper error propagation. Rate lookup/aggregate failures fail the required calculation, never silently become unconfigured or zero expenses.
- Prefer one ledger aggregate for existing other movements plus new eligible outgoing, keeping both operands on the same ledger read. Keep rate/recognition composition internally coherent using repository-compatible read transaction/snapshot organization as needed. No per-booking expense queries.
- Reuse Finance read access and existing strict month validation. No mutation during GET.

### 7.4 Frontend integration and rollout

- Extend handwritten Finance interfaces and OpenAPI; regenerate `frontend/src/api/types/generated.ts` via the existing script.
- FinanceView must normalize/validate the new discriminated state, pass data to Overview, and preserve request sequence/selection identity guards.
- Load the capability/history for the active property and refresh it after successful management changes. Read-only users must not encounter failed admin requests simply by opening Overview.
- Mutations reload both the active comparison and rate history. A late response from a previous property/month must not overwrite the active display.
- Reuse generated-entry sync metadata already fetched by FinanceView. Do not introduce a second persistent sync state.
- Deploy migrated backend before or together with frontend. Missing new response fields mean unavailable, not a fabricated amount.

## 8. Implementation impact map

| Area | Expected work |
|---|---|
| `backend/internal/migrate/` | New sequential rate-history up/down migration and uniqueness/check constraints. |
| `backend/internal/store/finance_long_term_rent.go` (proposed new file) | Property-scoped rate CRUD, effective-month lookup, comparison composition/helpers. |
| `backend/internal/store/finance_revenue_recognition.go` | Extend/reuse source-filtered ledger aggregate; preserve booking recognition and existing totals. |
| `backend/internal/api/finance_handlers.go` or dedicated Finance benchmark handler file | Add comparison DTO mapping and rate endpoints, validation, permissions, audit. |
| `backend/internal/api/server.go` | Register rate-management routes. |
| `backend/internal/store/finance_reset.go` | Review integration; rate settings survive. No change to existing reset accounting required. |
| `spec/openapi.yaml` | New endpoints, rate schema, nullable/not-configured comparison contract. |
| `frontend/src/api/types/finance.ts` | New typed rates/comparison response models. |
| `frontend/src/api/types/generated.ts` | Regenerate from OpenAPI, do not hand-edit. |
| `frontend/src/views/FinanceView.vue` | Load/refresh integration, permissions/capability, state and identity handling. |
| `frontend/src/views/finance/FinanceOverviewTab.vue` | New card, placement, outcome and state copy, management entry point. |
| `frontend/src/views/finance/FinanceLongTermRentDialog.vue` (proposed new file) | Rate-history UI and validated add/edit/delete workflow. |
| Store/API/Vue tests alongside affected files | Acceptance cases below, migration and reset regression coverage. |

Recheck current signatures before implementation; there is concurrent uncommitted work. PMS 28 governs Recognized net. This feature consumes that result and does not change its definition, existing Cleaner expense, Monthly net, Dashboard, or revenue recognition allocation.

## 9. Acceptance criteria

Synthetic examples below use EUR with assertions in integer cents. Unless specified otherwise, rent is configured and calculations use a single selected property/month.

| ID | Scenario | Expected |
|---|---|---|
| AC01 | Rent €900; recognized booking net €1,100; other incoming €0; eligible outgoing €250; generated cleaning €100. | Recognized net €750; long-term net **€650**; **ahead €100**. |
| AC02 | AC01 plus an incoming booking-payout ledger row €1,100. | All three results unchanged; no payout double count. |
| AC03 | Rent €900; recognized booking net −€50; outgoing booking-payout €50; no other movements. | Recognized net −€50; eligible outgoing €0; long-term net €900; **behind €950**. Refund does not reduce hypothetical long-term rent. |
| AC04 | Rent €900; booking net €1,100; generated cleaning €100, manual cleaning-category €40, recurring cleaning-category €20, eligible outgoing €250. | Recognized net €690; long-term net €650; **ahead €40**. All three cleaning sources are excluded from long-term deductions. |
| AC05 | Generated cleaning row also has cleaning category; another generated cleaning row has no category. | Each excluded once. Category absence does not make generated salary eligible. |
| AC06 | Outgoing €30 uncategorized, €20 in inactive non-cleaning category, €10 in category titled “Cleaning” with another code. | Eligible outgoing **€60**. Inactive `cleaning_salary` category would still be excluded. |
| AC07 | AC01 plus owner contribution incoming €1,000. | Recognized net €1,750; long-term net €650; **ahead €1,100**, per accepted existing net semantics. |
| AC08 | Rent €900; no bookings or transactions. | Recognized net €0; long-term net €900; **behind €900**. Provisional/future labels apply independently. |
| AC09 | Rent €900; eligible outgoing €1,200; booking net €1,100; no other transactions. | Long-term net **−€300**; Recognized net **−€100**; **ahead €200**, despite both being negative. |
| AC10 | Rent €900; booking net €1,000; cleaning €100; eligible outgoing €250. Then separately raise booking net one cent or lower it one cent. | Base: both €650, **Same result as long-term rent**. Variants: ahead €0.01 / behind €0.01. |
| AC11 | An August-service bill €40 is recorded in September; another timestamp is near a timezone month boundary. | Deduction follows stored `YYYY-MM` exactly, consistent with Finance, not service month or converted local month. |
| AC12 | Generated salary source-reference August has transaction date in September; incoming cleaning reimbursement €20. | Salary is excluded from September long-term outgoing by source. Reimbursement never affects long-term net, but affects Recognized net under existing rules. |
| AC13 | First rate €900 effective September 2026; second €1,000 effective January 2027. | August unconfigured; September–December use €900; January onward uses €1,000. No daily proration in February or leap years. |
| AC14 | Change September rate to €950; then delete January rate. | September–December recompute using €950; initially January remains €1,000. After deletion, January onward uses €950. |
| AC15 | Delete earliest rate while a later rate remains; separately delete all rates. | Before later rate: unconfigured. With no rates: every month unconfigured, no €0 assumption/verdict. |
| AC16 | Add/move a rate onto an existing effective month; invalid month `2026-1`; zero/negative/fractional-cent rent. | Validation/conflict response, no partial mutation; UI retains inputs with error. Different properties may use the same month. |
| AC17 | Save €900.50 using supported decimal input. | Store/API amount **90050 cents** and display exact currency amount. No floating-point drift or silent precision rounding. |
| AC18 | Current month halfway through; future month; rate €900 and no stored financial data. | Both use €900 full rent and show behind €900; current/future qualification is correct. No forecast or proration. |
| AC19 | Not synced: booking net €1,100, outgoing €250, no stored cleaning. Sync adds cleaning €100 and non-cleaning recurring €50. | Before: short-term €850, long-term €650, ahead €200, Provisional. After: short-term €700, long-term €600, ahead €100, synced status. GET creates no entries. |
| AC20 | Correct historical recognized booking net, edit/delete outgoing, or reclassify eligible expense as cleaning salary category. | Next read recomputes affected property/month. Reclassifying does not change Recognized net but increases long-term net, reducing the difference. |
| AC21 | Finance reader / writer / admin, property owner, super-admin, and unrelated-property user call read and mutation endpoints. | Reader/writer can read only; admin/owner/super-admin can manage per existing active-user checks; unauthorized access rejected. IDs cannot cross property boundaries. |
| AC22 | Rapid month/property changes with reversed response order; save response arrives after leaving property; comparison request fails or omits fields. | No stale/misattributed amounts, mutations, or capability state. Failure shows unavailable rather than zero/not-configured. |
| AC23 | Finance reset preserves €25 cleaning and existing €900 rent rate, removing bookings/other transactions. | History retained; Recognized net −€25; long-term net €900; **behind €925**. Sync label follows reset's returned state. |
| AC24 | Eligible-outgoing aggregate or rate lookup fails. | Error returned; no successful result using default €0 expenses or not-configured status. |
| AC25 | Add/edit/delete rates while monitoring finance transactions and audit records. | No synthetic rent transactions; successful changes audited; comparison/history refresh after success. |
| AC26 | View desktop and supported mobile widths; reader and manager views. | Card after Recognized net, readable signed values, difference underneath, calculation explanation and applicable state labels; management controls only for managers. |
| AC27 | Regression checks of Recognized gross/net, Monthly net, Cleaner expense, Revenue tab, Dashboard. | Their pre-feature meanings and existing results are preserved. No duplicate cleaning deduction or new net calculation. |
| AC28 | Two properties contain identical month/rate values and distinct expenses; move a rate's effective month earlier/later. | Isolation holds; selected rate always greatest effective month <= selected month; affected intervals update immediately. |

## 10. Verification and definition of done

### Implementation verification

1. Store tests: exact-cent formulas, source/category union exclusion, null/inactive categories, transaction-month timing, effective-rate selection, edits/deletes/uniqueness, property isolation, query failure behavior, and reset preservation.
2. API tests: contract states, integer/month validation, permissions, property-scoped IDs, conflicts, audit integration, and read-only behavior.
3. Vue tests: card content/order, form workflows and errors, role capability display, exact amount input, configured/unconfigured/unavailable states, month labels, provisional state, and stale-response protection.
4. Migration verification: new schema up/down through existing test conventions, no default rates or changes to existing Finance data.
5. Manual desktop/mobile review: amount readability, dialog/history usability, equal/negative cases, and selection/sync refresh behavior.

Relevant existing commands for the implementing agent:

- From `backend/`: `go test ./internal/store ./internal/api` (plus migration-focused checks for the new migration).
- From `frontend/`: `npm run types:openapi`, `npm run type-check`, and `npm test -- src/views/FinanceView.spec.ts`, adding the new dialog/component test paths.
- Run applicable repository lint/build checks for the implementation diff. These commands are a future verification plan, not claimed passing results.

### Definition of done

- Confirmed D01–D18 and FR01–FR04 are implemented with exact-cent acceptance coverage.
- Effective-month history, management permissions, audit behavior, and Finance-reset preservation work end to end.
- New Overview card compares against the exact existing Recognized net and excludes cleaning through both approved classification routes.
- Current/future/unsynced states present stored-data results honestly; missing data/errors cannot masquerade as zeros.
- Schema, backend responses, OpenAPI, handwritten types, and generated types are synchronized.
- Relevant tests and UI review pass, with existing financial metrics protected by regressions.

## 11. Handoff boundaries and residual engineering choices

No unresolved product question blocks this confirmed scope. Currency formatting follows existing EUR Finance behavior; no currency conversion or multi-currency consolidation is added. No expense modeling, scheduled disabling, percentage comparison, forecasting, accounting-period lock, annual report, or additional dashboard surface is part of this request.

Remaining choices are engineering details: exact helper/component organization, bounded safe-money validation consistent with repository conventions, read-snapshot implementation, API DTO organization, and concise supporting copy that preserves the approved meaning. The proposed contract provides an implementation starting point; these choices must not redefine the expense exclusions, permission threshold, rate timeline, or compared net value.
