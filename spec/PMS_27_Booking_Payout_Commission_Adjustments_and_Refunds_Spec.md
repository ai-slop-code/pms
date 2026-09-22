# PMS 27 — Booking.com payout commission adjustments and refunds

## 1. Status and objective

**Business specification; implementation design partially unresolved.** Prepared on 2026-09-14 from `payout_2609_02.csv`, the current working tree, supporting `payout_2608_03.csv`, and a read-only inspection of the local `data/pms.db` snapshot. The working tree already contains Finance/import changes associated with PMS 24 and PMS 25; this specification describes the inspected code, not an assumed deployed version.

Enable the existing Finance CSV preview/commit process to update the financial values of existing Booking.com stays when a payout export contains commission adjustments or a guest refund. Correct commission, guest-paid amount where applicable, net payout and the linked Finance transaction together.

This work produced documentation only. No CSV import was executed and no application code, schema or database data was changed. Database observations are local snapshot evidence, not verification of production balances. SQLite was opened read-only with `immutable=1`; no WAL/SHM sidecars were present in the inspected data directory.

## 2. Confirmed business decisions

The user confirmed the following during analysis:

1. **Commission adjustment is an additional charge.** For `Type = Commission adjustment`, the negative CSV `Amount` increases the existing positive commission cost by its absolute value. For example, €12.96 becomes €16.14 after a −€3.18 adjustment. Do not replace commission with €3.18 or zero.
2. **Guest-paid amount is preserved for commission adjustments.** These are additional Booking.com charges, not guest refunds or guest invoice reductions.
3. **Update net and Finance as well.** Deduct each adjustment from existing net payout and revise the linked Finance transaction.
4. **Revise the original payout period.** Retain the original payout date rather than posting an additional September cash transaction. This deliberately revises historical Finance totals. It does not reconcile September Finance directly to the September settlement amount.
5. **Booking `5126238619` is a guest refund.** Subtract €28.48 from its €64.05 guest-paid total, giving €35.57, and from its €51.58 net payout, giving €23.10.
6. **Require an original payout baseline.** Reject adjustments for `6800596898` and `6388468113` until their original payouts are imported. Do not partially apply their commission changes while leaving net/Finance unresolved.
7. **An explicitly imported zero payout is a valid baseline.** For `6270622388`, increase commission from €12.31 to €27.45, change net from €0.00 to −€15.14, and create an outgoing €15.14 transaction on the original August 6 payout date.
8. **Refund-aware invoice amounts.** Future invoice creation and existing manual regeneration use the corrected €35.57 guest-paid total for the refund booking. Import itself does not rewrite issued invoices. Commission-only changes do not change guest invoice amounts.
9. **Include VAT in commission for positive reservation rows.** Add the new `VAT for online platform services` column to commission cost. The supplied Net already includes VAT; do not deduct VAT again from Net.
10. **Storage design remains unresolved.** Reliable repeat-import protection is required, but the user chose to leave the design of durable adjustment evidence open. This document does not approve a new table, migration, or particular storage representation.
11. **Changed later exports remain unresolved.** Do not assume whether a later, different statement commission total already contains these adjustments. Re-importing identical historical files must preserve applied corrections.

## 3. Source file analysis

### 3.1 Settlement control totals

All 28 data rows are EUR, use payout ID `P5VcvNDIAHVOsVcu`, and have payout date **2026-09-10**. All show reservation status `ok` and payment status `by_booking`; a negative amount is not evidence of a cancelled stay.

| Row group | CSV lines | Count | Signed Net total |
|---|---|---:|---:|
| Positive reservations | 2–5 | 4 | €287.44 |
| Commission adjustments | 6–28 | 23 | −€141.57 |
| Guest refund, labelled Reservation | 29 | 1 | −€28.48 |
| **Whole file** | **2–29** | **28** | **€117.39** |

Total negative movements are **€170.05**. These are source-file controls, not the final sum of the affected bookings' cumulative balances.

Every commission-adjustment row has equal negative `Amount` and `Net`; its `Commission`, VAT and payment-fee columns are zero. Thus the adjustment amount must come from `Amount`, despite being a commission update. The user's VAT explanation is business context; those rows do not themselves report a nonzero VAT amount or tax rate.

### 3.2 New positive reservations: VAT-inclusive commission

| Booking | Stay dates, 2026 | Guest-paid Amount | Commission cost | VAT cost | Revised commission cost | Payment fee | Net |
|---|---|---:|---:|---:|---:|---:|---:|
| 6184079217 | Sep 7–8 | €85.38 | €15.59 | €3.82 | €19.41 | €1.02 | €64.95 |
| 5672486715 | Sep 6–7 | €77.29 | €14.11 | €3.46 | €17.57 | €0.93 | €58.79 |
| 6965402732 | Sep 4–6 | €159.16 | €29.06 | €7.12 | €36.18 | €1.91 | €121.07 |
| 5562791321 | Sep 3–4 | €56.05 | €10.24 | €2.51 | €12.75 | €0.67 | €42.63 |
| **Total** | | **€377.88** | **€69.00** | **€16.91** | **€85.91** | **€4.53** | **€287.44** |

For these rows, `Amount − VAT-inclusive commission − payment fee = Net`. Older payout files without the VAT column retain their existing commission interpretation. No tax-rate calculation or separate VAT-accounting module is required by the confirmed scope.

### 3.3 Adjustment examples and expected local-snapshot results

Values below are EUR. A dash denotes an unapplied row, not zero. Commission baselines were read from the current canonical booking; net baselines were checked against payout evidence. These values are acceptance fixtures, not constants to hard-code.

| Line | Booking | Commission before | Additional charge | Commission after | Net before | Net after |
|---:|---|---:|---:|---:|---:|---:|
| 6 | 6800596898 | 43.26 | 10.60 | — | No payout baseline | — |
| 7 | 6388468113 | 30.27 | 7.42 | — | No payout baseline | — |
| 8 | 6940764496 | 28.80 | 7.05 | 35.85 | 127.02 | 119.97 |
| 9 | 6104510626 | 17.62 | 4.32 | 21.94 | 77.70 | 73.38 |
| 10 | 5424955375 | 26.73 | 6.55 | 33.28 | 117.89 | 111.34 |
| 11 | 5079011734 | 71.93 | 17.63 | 89.56 | 317.23 | 299.60 |
| 12 | 5441941191 | 12.96 | 3.18 | 16.14 | 57.16 | 53.98 |
| 13 | 6986274535 | 16.53 | 4.05 | 20.58 | 72.92 | 68.87 |
| 14 | 6222340864 | 16.49 | 4.04 | 20.53 | 72.73 | 68.69 |
| 15 | 6874089083 | 15.86 | 3.89 | 19.75 | 69.93 | 66.04 |
| 16 | 6255831315 | 19.46 | 4.77 | 24.23 | 85.85 | 81.08 |
| 17 | 5916848183 | 13.08 | 3.21 | 16.29 | 57.68 | 54.47 |
| 18 | 6237317051 | 16.14 | 3.95 | 20.09 | 71.16 | 67.21 |
| 19 | 5832656436 | 14.97 | 3.67 | 18.64 | 66.02 | 62.35 |
| 20 | 6773131904 | 50.74 | 12.44 | 63.18 | 223.79 | 211.35 |
| 21 | 5265338781 | 16.53 | 4.05 | 20.58 | 72.92 | 68.87 |
| 22 | 5586026055 | 14.43 | 3.54 | 17.97 | 63.66 | 60.12 |
| 23 | 6593934589 | 16.40 | 4.02 | 20.42 | 72.34 | 68.32 |
| 24 | 5789105994 | 16.18 | 3.96 | 20.14 | 71.39 | 67.43 |
| 25 | 5746777721 | 31.56 | 7.74 | 39.30 | 139.19 | 131.45 |
| 26 | 6270640410 | 11.99 | 2.94 | 14.93 | 52.87 | 49.93 |
| 27 | 6270622388 | 12.31 | 15.14 | 27.45 | 0.00 | −15.14 |
| 28 | 6042579020 | 13.90 | 3.41 | 17.31 | 61.31 | 57.90 |

The seven original payouts in `payout_2608_03.csv` independently establish the original August 20 payout amounts for lines 12–18 above. In particular, booking `5441941191` must end at **€53.98**, not −€3.18.

### 3.4 Snapshot-specific prerequisites

- All 24 negative-row references already have a same-property canonical finance booking and named-stay association in property 1.
- Twenty-two have `has_payout_data = 1`; two have statement-only data. Their stored zero net and non-payout timestamps must not be treated as original payout evidence.
- `6270622388` has raw payout evidence explicitly recording Amount and Net of €0.00 on August 6, and no linked transaction. Its canonical Amount is €61.56; preserve that amount under the confirmed commission-only rule. Do not manufacture a missing original receipt or force a gross-minus-fees reconciliation for this unusual baseline.
- The refund booking has an incoming €51.58 transaction dated **2026-07-16**, commission €11.70 and fee €0.77. There is no invoice linked by its finance-booking ID in this snapshot.
- The four positive references do not yet have finance bookings. Each has one active, confirmed Booking.com named stay with the exact supplied dates, IDs 281, 280, 279 and 278 respectively. Existing matching rules remain applicable.

## 4. Verified current behavior and failure mechanism

| Area / source | Current behavior and consequence |
|---|---|
| `backend/internal/finance/statements/parser.go`, `parsePayout` | Recognizes Booking number and Sept dates; preserves signed Amount/Net; normalizes commission and fee to positive costs. VAT is retained in Raw only, with no canonical VAT mapping. Negative parsing is already supported. |
| `backend/internal/finance/statements/merge.go`, `mergePayout` | Replaces canonical Net, payout ID/date and row type with the incoming row. Replaces nonzero Amount, including negative Amount. Zero Commission does not update existing commission. No adjustment-specific arithmetic exists. |
| Same file, `mergeStatement` | Nonzero statement commission wins over canonical commission. Re-importing an old statement can therefore erase a corrected commission unless adjustment-aware behavior is added. |
| `backend/internal/api/finance_imports_handlers.go`, preview | Looks up the canonical booking by property/channel/reference, merges the row, then resolves the existing stay association. It does not distinguish charge movements from replacement payout totals. |
| Same file, `commitPayoutBookingSideEffects` | Passes the incoming row's Net and payout date to transaction upsert, rather than a corrected cumulative total on the original date. |
| `backend/internal/store/finance_bookings_merge.go`, `UpsertBookingFinanceTransaction` | Finds the existing booking_payout transaction by property/reference; overwrites its amount, direction and date. Negative net becomes outgoing with a positive stored amount. Zero net is currently ignored. |
| `backend/internal/store/finance_booking_payouts.go` | Finance bookings retain a named-stay association and one linked transaction. Payout lists filter by canonical payout date. |
| `backend/internal/store/finance_imports.go` | Import audit stores file SHA and row counts; merge audit stores changed-field names. A matching file SHA is a preview hint, not durable per-adjustment deduplication. |
| `backend/internal/api/invoice_handlers.go`, `payoutInvoiceBillableCents` | Invoice amount first consults raw payout Amount, then positive canonical Amount, then Net. Preserving original raw data alone would still yield the pre-refund amount; replacing it with an adjustment row is also wrong. |
| `backend/internal/store/analytics.go` | Existing financial analytics read canonical Amount, commission, fee and Net independently. Updating commission alone cannot reduce stored Net. Monthly revenue cohorts use named-stay arrival dates. |
| `backend/internal/store/analytics_statement.go` | Existing commission charts use canonical commission and Amount with their existing cohort/eligibility rules. |
| `backend/internal/store/finance.go`, `ComputeFinanceSummary` | Net is incoming minus outgoing. Property Income separately counts qualifying incoming transactions only. An outgoing negative-net correction affects Net/outgoing but is not subtracted by the existing Property Income formula. |

The inspected schema has one `finance_bookings` row per property/reference, one raw payout snapshot and one linked transaction. It does not model successive payout movements. Current import can turn the original €57.16 receipt for `5441941191` into a September outgoing €3.18 entry, while leaving commission €12.96 and overwriting Amount with −€3.18. This is replacement, not the requested deduction.

## 5. Required business behavior

### 5.1 Classification, identity and dates

Use the existing property-scoped Booking.com import and canonical finance booking linked to its named stay. “Update the stay's commission” means updating that booking's `commission_cents`, which existing stay analytics consume; named stays do not acquire a second commission field.

- Positive Reservation rows follow the existing reservation import with VAT-inclusive commission as specified below.
- The supplied `Commission adjustment` rows follow the additive commission rule.
- The supplied negative Reservation for `5126238619` follows the confirmed refund rule. Do not hard-code this booking as application logic or assume every possible negative Reservation has the same cause; general refund classification is an open question in section 8.
- Existing property/reference identity and persisted stay association take precedence. Do not match corrections to the four new September stays using the settlement date.
- Preserve the original stay dates and use existing reporting cohorts. In particular, `6042579020` arrives July 31: its commission/net correction belongs to July in arrival-month analytics, although its original receipt is in August Finance.
- Preserve original payout date and identity as the corrected receipt's context; retain the September adjustment source identity as evidence rather than substituting it for the original receipt.
- No occupancy, stay outcome or cancellation change follows merely from a financial deduction. Corrections must not create a replacement stay.

### 5.2 Commission adjustments

For one previously unapplied charge, in integer EUR cents:

`charge = -CSV.Amount` for the observed negative adjustment rows.

`corrected commission = existing effective commission + charge`

`corrected net = existing effective net + CSV.Net`

Preserve guest-paid Amount and payment-service fee. For this file, Amount and Net movements are identical and negative. Do not use the zero CSV Commission as a replacement; do not also deduct the increased commission from corrected Net, which would count the charge twice. Do not infer a commission percentage or VAT rate from the charge.

Require original payout evidence and a usable original payout date. If absent, reject the entire adjustment row through the existing rejection mechanism. The existing statement-only canonical record is not sufficient. An explicitly imported zero payout is sufficient, as confirmed for `6270622388`.

### 5.3 Guest refund

For the confirmed €28.48 refund:

`corrected guest-paid Amount = 6405 - 2848 = 3557 cents`

`corrected Net = 5158 - 2848 = 2310 cents`

Keep commission €11.70 and payment fee €0.77: the supplied refund returns neither. Keep the July 13–14 stay and July 16 original payout date. The linked incoming Finance transaction becomes €23.10. The refund amount must not become the complete guest-paid amount or a commission charge.

Invoice creation/prefill and manual regeneration must resolve the effective guest-paid total of €35.57 despite the original raw payout showing €64.05. Preserve source truth; do not rewrite an original CSV snapshot to falsely claim it contained the corrected amount. Import does not automatically regenerate or amend an issued invoice.

### 5.4 VAT on positive reservations

For the observed negative cost columns on positive reservation rows:

`commission cost = abs(CSV.Commission) + abs(CSV.VAT for online platform services)`

Keep Amount and Net as supplied and payment fee under existing normalization. Retain the separate raw source columns. This is a mapping into the existing commission-cost measure, not a new tax field or tax report. Missing VAT in older files contributes zero VAT. Apply the four exact expected results in section 3.2, without a second deduction from Net.

### 5.5 Linked Finance transaction and reporting

Update the existing booking_payout transaction using the **corrected cumulative Net**, the **original payout date**, and the existing booking_income categorization. Preserve its booking association; create the missing transaction for the confirmed zero-baseline case using the existing transaction representation.

- Positive corrected Net: incoming, amount equal to Net.
- Negative corrected Net: outgoing, amount equal to the absolute Net.
- A corrected zero balance must not leave an old nonzero receipt active. The current upsert's zero early-return needs consideration; exact retain-versus-remove behavior remains an open design decision.

Finance Net, direction totals and category breakdowns must reflect the corrected transactions. Existing Property Income is an incoming-only measure; this spec does not silently redefine it to subtract outgoing entries. Accordingly, `6270622388` contributes an outgoing €15.14 and reduces Finance Net, while the current Property Income formula excludes it. Existing cleaner-margin calculations inherit the current Property Income definition.

Stay financial analytics consume the corrected canonical commission, guest-paid Amount where refunded, and Net under their existing date allocation and eligibility rules. No new reporting period, chart or revenue formula is introduced. Commission rates calculated from commission/Amount will change naturally; source `commission_pct` is not a newly inferred tax-inclusive rate.

### 5.6 Preview, commit and retries

Reuse `FinanceView.vue`'s existing Booking.com CSV preview, insert/update/unchanged counts, rejected rows, stay selection where applicable, and commit process.

- Classify already linked corrections as updates to existing bookings, not new bookings/stays.
- Include the actual changed canonical fields: commission and Net for commission corrections; Amount and Net for the refund.
- Explain missing-original-payout rejections with CSV line/reference and the prerequisite to import the original payout.
- Retain existing normal positive-reservation stay matching and approved PMS 24 behavior.
- Commit the booking correction, linked Finance change and any required applied-adjustment evidence atomically per row, following the existing per-row transaction pattern. A rejected/failed row must leave none of those effects applied.
- Do not apply a stale additive preview against a newer financial balance. Financial state and duplicate status must be verified at commit, not just stay identity.
- Repeating a successful adjustment must change no financial values. This must survive a process restart, row reordering, retry after partial success, and uploading identical source rows within a different file.
- Re-importing the original payout or the identical pre-adjustment statement must not undo corrections, restore the pre-refund invoice amount, or cause a subsequent retry to charge twice.
- Rejected rows remain retryable: after original payouts for the two missing baselines are imported, retrying this file applies only the previously unapplied corrections.

These duplicate/replay requirements are required outcomes, **not a claim that the current audit schema supports them**. Finalizing how they are achieved is blocked by the unresolved design decision below.

## 6. Reconciliation expectations for the inspected snapshot

Subject to the same matching state and successful row commits:

- First corrected import: **4 inserts, 22 updates, 2 rejections**. Updates are 21 commission adjustments plus 1 refund.
- Unapplied charges: €10.60 + €7.42 = **€18.02**.
- Applied historical commission increases: **€123.55**. Applied historical Net reduction including refund: **€152.03**.
- Net change across all Finance periods from this first import: €287.44 − €152.03 = **+€135.41**.
- August Finance Net decreases €123.55; July Finance Net decreases €28.48; September receives the four normal receipts totalling €287.44.
- The source file still totals €117.39. The €18.02 difference from the first-import all-period change is the two explicitly rejected charges.
- Once both original payouts are imported and their corrections retried, all 24 negative movements total €170.05. The movements attributable to this September file then net to €117.39 across the affected original periods and September. Importing the missing original payouts is a separate financial effect and must not be included in this file-only control.

These are expected outcomes after the specified functionality is implemented, not results of a live import performed during analysis.

## 7. Acceptance scenarios for eventual implementation

1. **Commission example:** import the original August 20 row for `5441941191`, then the September adjustment. Commission is €16.14, Amount stays €70.97, Net and incoming receipt become €53.98, and payout date stays August 20.
2. **Full eligible charge set:** each eligible row in section 3.3 reaches the listed commission/Net values without changing guest-paid Amount or duplicating the stay.
3. **Refund:** `5126238619` reaches Amount €35.57, Net/incoming receipt €23.10 on July 16, commission €11.70 and fee €0.77. Existing invoice creation/regeneration uses €35.57; import does not rewrite issued invoices.
4. **Zero baseline:** `6270622388` reaches commission €27.45, Net −€15.14 and one outgoing €15.14 transaction dated August 6. Guest-paid Amount remains the prior canonical value.
5. **Missing baseline:** lines 6 and 7 are rejected with no commission, Net, transaction or applied-evidence mutation. Statement-only data must not become a fabricated payout baseline.
6. **VAT positive rows:** all four results in section 3.2 hold. Total commission is €85.91 and Net €287.44. A legacy row without VAT retains its established behavior.
7. **Dates/cohorts:** corrections do not move August/July stays into September analytics. July 31 arrival remains in the existing July arrival cohort; original Finance receipt dates remain intact.
8. **Duplicate adjustment:** uploading the same adjustment again, including after restart and in a reordered file, has zero additional monetary effect and no additional transaction.
9. **Historical replay:** re-uploading identical original payout/statement evidence after the adjustment preserves the corrected commission, Amount, Net and invoice amount. Retrying the adjustment afterward still has zero additional effect.
10. **Partial retry:** first-import outcomes reconcile as in section 6. After supplying the two original payouts, retry applies only their outstanding charges.
11. **Atomic failure/concurrency:** a row-level persistence failure leaves all its financial values/evidence unapplied; a concurrent/stale preview cannot double-apply or overwrite another correction.
12. **Scope regression:** normal stay matching, property isolation, existing issued-invoice behavior and statement cancellation cohorts remain consistent with the existing processes. A deduction alone does not change stay status or nights.

Verification should exercise parser/merge, preview/commit with store transactions, Finance summaries, existing commission/revenue analytics, and invoice amount resolution. No implementation tests were added or executed as part of this documentation task.

## 8. Explicitly unresolved decisions

### 8.1 Durable evidence and duplicate identity — user deferred

What existing-storage extension, if any, is permitted to retain original financial baselines and the identity/value of each applied correction? How should two corrections for the same booking in one settlement be distinguished?

Current file SHA, one replaceable raw payout row and a list of changed field names do not establish a durable correction history. Whole-file duplicate blocking alone cannot support the confirmed partial retry or identical rows in different files. No new table, endpoint, module, or exact deduplication key is approved here. The implementation design is not ready until this is resolved.

### 8.2 Different subsequent exports — user deferred

When a later payout or statement changes commission/Amount, does its total already include earlier corrections? How should VAT-inclusive payout commission interact with a statement's commission total? Identical old evidence must be replay-safe, but changed evidence has no agreed reconciliation rule. Do not infer inclusion from import order or blindly add adjustments to an already-adjusted total.

### 8.3 Remaining boundary questions

The following are outside the exact confirmed sample and require answers before a general implementation selects a policy:

- Should **every** negative Reservation with an existing original payout be treated as a guest refund, or how does the operator identify another kind of deduction within the existing process?
- If corrected Net becomes exactly zero, should the existing Finance transaction remain with zero amount or be removed through an existing supported mechanism?
- If booking reference matches but CSV stay dates differ from the linked stay, should the correction be rejected or applied to the persisted association? Do not silently change stay dates.
- How should positive commission adjustments, VAT credits, blank/malformed adjustment Amount, unequal Amount/Net movements, or a refund exceeding the recorded guest-paid amount be treated? This file does not establish those rules.

No default answers to these questions are implied by the acceptance cases. New standalone workflows, tax accounting, credit-note generation and schema designs are not specified.

## 9. Implementation impact map, for planning only

- `backend/internal/finance/statements/parser.go`: expose/interpret the observed VAT cost and distinguish valid movement data from replacement totals without losing raw evidence.
- `backend/internal/finance/statements/merge.go`: additive commission/refund behavior, effective amounts, replay protection and source precedence once the open design is resolved.
- `backend/internal/api/finance_imports_handlers.go`: classification, original-payout eligibility, correct preview outcomes, financial commit validation and atomic side effects.
- `backend/internal/store/finance_bookings_merge.go`: update the existing linked transaction from corrected Net and the original payout context; handle the confirmed negative/zero-baseline case.
- `backend/internal/store/finance_imports.go`: assess audit/evidence feasibility; exact changes pending approval.
- `backend/internal/api/invoice_handlers.go` and `booking_payout_display.go`: ensure invoice amount selection uses the effective guest-paid amount without corrupting original raw evidence.
- `backend/internal/store/analytics.go`, `analytics_statement.go`, and `finance.go`: verify consumers of corrected values and existing period/formula semantics.
- `frontend/src/views/FinanceView.vue`: reuse the existing preview/commit and rejection presentation. API/type changes, if required by the agreed design, should follow existing contracts rather than introduce a separate import workflow.
- Existing PMS 24 and PMS 25 specifications: retain their approved stay-selection and statement-evidence/cancellation processes while reconciling the explicitly identified commission-precedence conflict.

No application changes are authorized by this document-writing task.
