# PMS 31 — Finance: Long-term net card redesign

## 1. Status and objective

- **Date:** 2026-09-24.
- **Status:** Business rules and presentation confirmed by the product owner through clarification. Implementation is future work.
- **Audience:** AI coding agent, implementing engineer, reviewer, and QA.
- **Evidence baseline:** Repository inspected during specification preparation; this is not a statement of deployment status.
- **Location:** Finance → Overview → Revenue recognition → Long-term net card.

Change the card's main figure to a negative baseline that improves with positive Recognized net, never worsening below that baseline when Recognized net is negative. Show the actual rent-minus-eligible-expenses result in the smaller line underneath.

This document supersedes PMS 29's main-card value, outcome-based colour, ahead/behind subtitle, and duplicate numerical explanation requirements. PMS 29's underlying rent history, expense eligibility, API calculations, permissions, and month semantics continue to apply. PMS 28 continues to define Recognized net.

## 2. Confirmed product decisions

| ID | Decision |
|---|---|
| D01 | Keep the title **LONG-TERM NET** and its existing location. |
| D02 | The baseline is the negative absolute value of rent minus eligible expenses. €900 − €572.72 produces a baseline of −€327.28. |
| D03 | Expenses exceeding rent also produce a negative baseline: €900 − €1,200 produces a baseline of −€300. |
| D04 | Add positive Recognized net to the baseline. Negative Recognized net contributes zero and cannot push the figure below its baseline. |
| D05 | Colour the main figure red below zero, normal dark text at exactly zero, and green above zero. |
| D06 | Replace the short-term ahead/behind/equal subtitle with **Long term rent: {rent-minus-expenses result}**. Show only the result, not the equation or operand labels. |
| D07 | The subtitle preserves the mathematical sign: €327.28 for €900 − €572.72; −€300.00 for €900 − €1,200. |
| D08 | Existing underlying rent, eligible-expense, and Recognized net inputs remain. |
| D09 | Use existing currency formatting. Positive figures have no explicit plus sign; zero uses the normal text colour, not a hard-coded black colour. |
| D10 | Remove the duplicate rent-minus-expenses numerical explanation below the cards. |

Approved example, when Recognized net is zero or negative:

```text
LONG-TERM NET
−€327.28
Long term rent: €327.28
```

Currency placement, separators, and negative-sign glyph follow the existing formatter; this example illustrates content rather than mandating a new locale.

## 3. Verified implementation findings

| File | Current responsibility / finding |
|---|---|
| `frontend/src/views/finance/FinanceOverviewTab.vue` | Receives Recognized net, its availability flag, and `longTermComparison`. Currently renders `long_term_net_cents` as the main value; chooses colour from `outcome`; builds the ahead/behind/equal subtitle with `outcomeText`. Contains the separate numerical explanation, rent-management button, supporting help, and period/provisional labels. |
| `frontend/src/views/FinanceView.vue` | Loads recognition and rent-history data, validates recognition fields, and passes values to Overview. Uses `loadSequence` and captured property/month identities to reject stale results. Passes a null comparison when recognition is unavailable. |
| `frontend/src/components/ui/UiKpiCard.vue` | Already supports `value`, `hint`, and `default`/`danger`/`success` tones. Its hint is smaller than the main figure. The label is uppercased through CSS. Default value colour is `var(--color-text)`. |
| `frontend/src/utils/format.ts` | `formatEuros` formats integer cents using existing locale conventions and supports ordinary formatting without a forced plus sign. |
| `frontend/src/api/types/finance.ts` | `FinanceLongTermComparison` already provides nullable rent, eligible outgoing, raw long-term net, short-term difference, and outcome fields. |
| `backend/internal/api/finance_handlers.go` | Recognition handler computes `long_term_net_cents = monthly_rent_cents - eligible_outgoing_cents`, and the existing signed short-term difference/outcome. Also returns `recognized_net_cents`. |
| `backend/internal/store/finance_long_term_rent.go` | Resolves applicable rent history and computes eligible outgoing using property, transaction month, and source/category exclusions. |
| `spec/openapi.yaml` and `frontend/src/api/types/generated.ts` | Already describe the inputs needed by this presentation. |
| `frontend/src/views/FinanceView.spec.ts` | Existing Vitest/Vue Test Utils integration coverage for Finance loading, recognition, and sync presentation. |

The existing card can show a positive raw rent-minus-expenses value in red because its colour currently comes from a different value: the short-term difference. The new contract makes main-value colour depend on the displayed main value itself.

## 4. Calculation contract

All calculations use integer cents. Define:

```text
R = applicable full monthly rent
E = existing eligible outgoing for the selected property/month
N = existing Recognized net for the selected property/month

L = R - E
B = -abs(L)
D = B + max(N, 0)

main figure = D
subtitle amount = L
```

- `L` is the existing API `long_term_net_cents`; reuse it as the authoritative rent-minus-expenses result.
- `B` is a display baseline, not a new expense or ledger transaction.
- `D` is a derived card value, not a replacement definition for Recognized net or the API's raw long-term net.
- The baseline cannot be positive. When rent and expenses are equal, it is zero.
- The main figure has a lower bound of `B` and no upper cap at zero.
- Apply the absolute value to `L`, not to Recognized net or the final main figure.
- Normalize a computed zero before formatting so the UI never displays negative zero.
- Colour is based only on the sign of `D`, with exact cent equality and no tolerance threshold.
- Do not use the API's `short_term_difference_cents` or `outcome` to calculate or colour this card. They represent PMS 29's mathematical comparison and can disagree with the new display, particularly when `N < 0` or `L < 0`.

### Existing inputs

Continue to use the selected property's applicable effective-month rent and the full monthly amount, without proration. Eligible outgoing remains the sum of stored outgoing transactions in the selected transaction month, excluding:

1. `source_type = 'booking_payout'`;
2. `source_type = 'cleaning_salary'`;
3. transactions whose category code is `cleaning_salary`.

Uncategorized outgoing remains eligible. Recognized net remains the existing recognized booking net plus other incoming minus other outgoing, including the existing treatment of owner contributions and cleaning costs. Clamping applies only to the contribution used in this card; it does not change the Recognized net card or stored/reporting amounts.

## 5. Presentation and states

### Configured, available data

- Keep `label="Long-term net"`; existing CSS produces the uppercase title.
- Main value: format `D` with existing `formatEuros` behaviour.
- Tone: `danger` when `D < 0`, `default` when `D = 0`, `success` when `D > 0`.
- Smaller in-card hint: **Long term rent: {formatted L}**.
- No explicit `+` for positive values and no trailing equation or operand labels.
- Remove `outcomeText` and the old ahead/behind/equal wording from this card.
- Remove the separate numerical span currently showing `{rent} rent − {expenses} eligible expenses.` below the cards.
- Retain the existing rent-management action, its permission checks, explanatory help about expense exclusions, and month/provisional labels. Keep the explanation's layout readable after removing the numerical span.
- Retain the existing card order, sizing, and responsive layout. Existing `UiKpiCard` capabilities are sufficient.

### Availability and lifecycle

| State | Required presentation |
|---|---|
| No applicable rent rate (`not_configured`) | Existing **Long-term rent not configured** value and configuration hint; neutral tone. No numerical subtitle. |
| Missing comparison, unavailable recognition, or missing/invalid required numerical input | **Unavailable**, existing unavailable hint, and neutral tone. Do not substitute zero or display the baseline as though recognition were known. |
| Configured with valid zero Recognized net | Show the baseline and raw subtitle normally; zero is valid data. |
| Current month | Numeric display plus existing **Month in progress** label. |
| Future month | Numeric display plus existing future-month qualification. |
| Unsynced generated entries | Numeric display plus existing provisional information. |
| Property/month change or reload | Use the existing loading/availability and stale-response protections; recompute from the new selection's inputs. |

For numerical rendering, require configured comparison status, available Recognized net, and valid integer-cent values for `L` and `N`. A null input must not pass through the existing `eur` wrapper's null-to-zero fallback. Time and sync labels remain independent of the sign of the figure.

## 6. Implementation plan for the coding agent

Follow `spec/PMS_13_Coding_Conventions.md` and recheck the current working tree before editing.

### Step 1 — Add the derived presentation in Overview

In `frontend/src/views/finance/FinanceOverviewTab.vue`:

1. Add a small typed computed presentation based on comparison status, `long_term_net_cents`, `recognizedNetCents`, and `recognizedNetAvailable`.
2. Guard unavailable inputs before arithmetic or formatting.
3. Compute `D = -Math.abs(L) + Math.max(N, 0)` in cents, normalizing zero.
4. Bind the card's value and tone to `D`, and the hint to formatted `L`.
5. Remove the old outcome-based computed text and duplicate numerical explanation.
6. Keep existing supporting controls and state labels functional.

Use local computed values rather than adding a new persisted result or a new endpoint. This is a presentation derivation from already-returned authoritative inputs.

### Step 2 — Verify existing data flow

Review `frontend/src/views/FinanceView.vue` to confirm the current availability and reload paths continue to feed the card after month/property selection, rent edits, transaction edits/imports, generated-entry sync, and reset. No additional request is needed.

The Go handler and store, database schema, OpenAPI schema, and generated API types require no changes for this design. Preserve the existing meanings of `long_term_net_cents`, `short_term_difference_cents`, and `outcome`; do not repurpose these API fields for the new display amount.

### Step 3 — Add focused behavioural coverage

Add `frontend/src/views/finance/FinanceOverviewTab.spec.ts` using the existing Vitest/Vue Test Utils conventions. Test rendered main values, hints, and tone classes for the acceptance cases below. Use the existing currency formatter or a controlled locale in assertions rather than assuming every environment places the euro symbol identically.

Include tests where the old API `outcome` conflicts with the new main figure's tone, proving that it no longer drives this presentation. Exercise prop updates so a new selection/result recalculates the card rather than retaining its prior figure. Keep existing Finance integration coverage passing.

### Step 4 — Verification

From `frontend/`, run:

```sh
npm test -- src/views/finance/FinanceOverviewTab.spec.ts src/views/FinanceView.spec.ts
npm run type-check
npx eslint src/views/finance/FinanceOverviewTab.vue src/views/finance/FinanceOverviewTab.spec.ts
```

Manually inspect the card at desktop and mobile widths, checking the smaller subtitle, negative/zero/positive colours, rent-management action, and supporting labels. Report any pre-existing check failures separately from regressions introduced by implementation.

## 7. Acceptance examples

Amounts below are EUR for readability; implementation uses integer cents. Positive amounts use ordinary formatting without a plus sign.

| ID | Rent | Eligible expenses | Recognized net | Main figure | Tone | Subtitle amount |
|---|---:|---:|---:|---:|---|---:|
| AC01 | 900.00 | 572.72 | −301.35 | −327.28 | Red | 327.28 |
| AC02 | 900.00 | 572.72 | 0.00 | −327.28 | Red | 327.28 |
| AC03 | 900.00 | 572.72 | 100.00 | −227.28 | Red | 327.28 |
| AC04 | 900.00 | 572.72 | 327.27 | −0.01 | Red | 327.28 |
| AC05 | 900.00 | 572.72 | 327.28 | 0.00 | Normal dark text | 327.28 |
| AC06 | 900.00 | 572.72 | 327.29 | 0.01 | Green | 327.28 |
| AC07 | 900.00 | 572.72 | 400.00 | 72.72 | Green | 327.28 |
| AC08 | 900.00 | 1,200.00 | −100.00 | −300.00 | Red | −300.00 |
| AC09 | 900.00 | 1,200.00 | 0.00 | −300.00 | Red | −300.00 |
| AC10 | 900.00 | 1,200.00 | 100.00 | −200.00 | Red | −300.00 |
| AC11 | 900.00 | 1,200.00 | 300.00 | 0.00 | Normal dark text | −300.00 |
| AC12 | 900.00 | 1,200.00 | 400.00 | 100.00 | Green | −300.00 |
| AC13 | 900.00 | 900.00 | −100.00 | 0.00 | Normal dark text | 0.00 |
| AC14 | 900.00 | 900.00 | 0.00 | 0.00 | Normal dark text | 0.00 |
| AC15 | 900.00 | 900.00 | 100.00 | 100.00 | Green | 0.00 |
| AC16 | 900.00 | 0.00 | 0.00 | −900.00 | Red | 900.00 |

Additional acceptance checks:

- Every configured numeric hint is exactly `Long term rent: {formatted subtitle amount}`. No equation or ahead/behind/equal sentence remains in the card, and no duplicate numerical explanation remains underneath.
- AC08's original API outcome is `ahead` (Recognized net −€100 versus raw long-term net −€300), but the new main figure is still −€300 and red.
- Unconfigured rent uses its existing state rather than €0.00; unavailable recognition or null raw long-term net does not render a numerical result.
- Zero never displays a minus sign; positive figures never gain a forced plus sign.
- Updating only Recognized net changes the main figure and tone as appropriate, while the subtitle stays fixed. Updating rent/eligible expenses changes both derived values as specified.
- Current/future/provisional context and authorized rent management remain visible and usable.

## 8. Completion criteria

The implementation is complete when the displayed amount follows the confirmed absolute-value baseline and nonnegative Recognized net contribution, the subtitle shows the signed raw rent-minus-expenses result, colours follow the displayed amount, and the acceptance checks pass. The implementation handoff must identify changed files and verification results.
