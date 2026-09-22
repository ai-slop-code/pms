# PMS 26 — Invoice decimal amount input fix

## 1. Purpose and confirmed scenario

Investigated on 2026-09-14 against the current working tree and the user-supplied screenshot. This is a specification for future implementation; no application code was changed or runtime invoice created during this investigation.

**User-confirmed scenario:** a Named stay exists/is selected, but no Booking.com payout is linked. The operator manually enters the full guest price in **Amount billed (EUR)**. The field must accept a monetary value such as `123.99` and allow an otherwise valid invoice to be saved with that exact cent amount.

The original phrase “we don't have a mapped stay yet” was clarified with the user as **“No payout linked.”** A Named stay and an optional payout link are different associations. This specification retains the existing required Named stay workflow.

## 2. Incident evidence and root cause

### 2.1 Observed symptom

The screenshot shows `4234.23` in the amount field and the native browser message:

> Please enter a valid value. The two nearest valid values are 4234 and 4235.

It also shows **Mapped Booking.com payout (optional)** set to **— None —**. The screenshot demonstrates that decimal text can be entered but fails validation; it does not demonstrate that decimal keystrokes themselves are blocked.

### 2.2 Confirmed code path

| Layer | Evidence in the current working tree | Finding |
|---|---|---|
| Invoice editor | `frontend/src/views/invoices/InvoiceEditorForm.vue:84–128` | A native form contains `UiInput` with `v-model.number`, `type="number"`, `min="0"` and **already-present** `step="0.01"`. Create and edit use this same form. |
| Shared input | `frontend/src/components/ui/UiInput.vue:4–38, 62–88` | Props include `min` and `max`, but no `step`. The native input binds `min`/`max`, but no `step`. The component's single root is a wrapping `div`. |
| EUR → cents | `frontend/src/views/InvoicesView.vue:214–238` | The payload uses `amount_total_cents: Math.round((form.value.amount_eur || 0) * 100)`. |
| Create/edit requests | `frontend/src/views/InvoicesView.vue:241–269` | Saving requires a Named stay and uses POST for creation or PATCH for editing. No payout is required. |
| Cents → editor | `frontend/src/views/InvoicesView.vue:84–105` | Saved amounts are loaded using `invoice.amount_total_cents / 100`. |
| API | `backend/internal/api/invoice_handlers.go:86–99, 455–481, 551–555, 616–621` | Amount is an integer **in cents**. Creation requires an amount after optional payout prefill. Negative cents are rejected; a Named stay is required. |
| Persistence | `backend/internal/store/invoices.go`, `CreateInvoice`, `UpdateInvoice`, and row scanning | `AmountTotalCents` is persisted and read directly; there is no conversion to whole euros. |
| Schema | `backend/internal/migrate/000039_legacy_occupancy_removal.up.sql:672–701` | `amount_total_cents` is a non-negative integer; `named_stay_id` is required and the payout association is nullable. |
| PDF | `backend/internal/api/invoice_handlers.go:654–668`; `backend/internal/invoicepdf/invoicepdf.go:819–824` | The renderer receives cents and formats two decimal places; Slovak PDFs use a comma and English PDFs a dot. |

**Root cause:** the invoice's `step="0.01"` is an undeclared Vue component attribute. With the current default attribute inheritance it falls through to the wrapper `div`, rather than the native `input`. The number input therefore has the HTML default step of `1`. Its `min="0"` establishes an integer step base, so fractional euro amounts produce a native `stepMismatch`. Interactive browser submission is blocked before the form's submit handler can save the invoice.

This explains the supplied browser warning. The conclusion is based on source inspection and the screenshot; the deployed build, browser version, and live DOM were not independently inspected.

### 2.3 Impact and existing test gap

- The defect is in a shared component and is not conditional on payout selection. Manual creation exposes it directly; editing a saved fractional amount and payout-prefilled fractional amounts use the same affected input.
- The only other explicit `step` callers found under `frontend/src` are Cleaning fee and Washing fee in `frontend/src/views/cleaning/CleaningFeeHistory.vue:37–50`. Both use `UiInput` with `step="0.01"` and are subject to the same attribute-forwarding defect by source inspection. These are regression-check targets for the shared fix, not new cleaning requirements.
- `frontend/src/components/ui/UiInput.spec.ts` covers label, model updates, min/max, help, error and type, but not step forwarding or numeric validity.
- `frontend/src/views/InvoicesView.spec.ts` covers loading, required stay selection and PDF links, but not decimal entry and saving. Calling the save function or dispatching a synthetic submit alone does not establish that real browser constraint validation permits submission.

## 3. Required behavior

1. In an invoice with a selected Named stay and no payout link, accept non-negative amounts representable in whole euro cents. Examples: `0`, `0.01`, `123`, `123.9`, `123.99`, and the screenshot's `4234.23`.
2. Fractional amounts on the cent grid must not produce a whole-number step warning. An otherwise valid invoice must be submittable using the normal Create invoice or Save invoice button.
3. Preserve cents throughout saving and reloading: `123.99` → `12399` cents → `123.99` EUR, and `4234.23` → `423423` cents → `4234.23` EUR.
4. Honor the field's existing `min="0"` and intended `step="0.01"` constraints. Negative values and off-cent values such as `123.999` must fail native amount validation. Whole-euro and one-decimal inputs remain valid; forcing two typed digits is not required.
5. Create and edit use the same corrected behavior. Selecting a payout that prefills a fractional amount must not introduce a step validation failure.
6. The shared input must honor a caller-supplied step on the native input. A caller that omits step must continue to use native default behavior; do not impose a global monetary step on integer or non-money fields.

These requirements restore the existing invoice configuration and cents-based contract. Zero is already allowed by both the field minimum and API/schema; this fix does not introduce a new zero-invoice policy.

## 4. Proposed implementation boundary

The targeted implementation is to add optional step support to `UiInput`, using the same string/number prop and explicit native-attribute binding pattern already used for `min` and `max`. Omit the native attribute when the caller supplies no step. Preserve native step values, including explicit integer steps and `any`, without setting `any` on the invoice field.

The invoice already specifies the correct step; adding a second step declaration there cannot repair the component boundary. Keep native form validation enabled. Do not use `novalidate`, whole-euro rounding, or unrestricted fractional precision as a workaround.

| File/area | Expected implementation work |
|---|---|
| `frontend/src/components/ui/UiInput.vue` | Forward optional step to the actual input following existing component conventions. |
| `frontend/src/components/ui/UiInput.spec.ts` | Focused step/validity regression coverage. |
| `frontend/src/views/InvoicesView.spec.ts` | Exercise actual amount input and verify create/edit payload cents with realistic mocked invoice responses. |
| Invoice editor and cleaning fee form | Validate callers against the shared fix. |

No API, generated type, database schema, monetary storage, payout calculation, PDF formatting, or stay ownership change is indicated by this defect. Existing payout prefill and linked-payout regeneration behavior remain the baseline: regeneration can refresh a linked invoice amount from its payout (`refreshInvoiceBillableFromLinkedPayout`); this bug report does not request a change to that rule.

The working tree contains pre-existing changes in finance, occupancy, Nuki, tests and specifications. Implementation must inspect the then-current diff and integrate with that state.

## 5. Acceptance criteria and verification

### 5.1 Focused automated regressions

1. Mount the real `UiInput` with `type="number"`, `min="0"`, `step="0.01"`. Verify the step is on the native input, then verify numeric validity for fractional and invalid examples. The wrapper alone carrying the attribute must not satisfy this test.
2. Verify an explicit integer step still rejects an off-step fraction and that omitted step leaves the native attribute absent. Retain existing min/max and model-update coverage.
3. With a selected Named stay and no payout, enter `123.99` through the actual invoice amount input and verify the creation request contains `amount_total_cents: 12399` and no payout link. Repeat the amount assertion for `4234.23` → `423423`.
4. Load a saved invoice with fractional cents, edit the amount through the input, and verify the PATCH request and returned/reloaded form preserve the exact cent amount.
5. Verify a fractional payout prefill is valid and produces the expected cents using existing payout amount selection rules.

### 5.2 Real-browser acceptance

Use test data and an otherwise valid invoice: selected property and eligible Named stay, required customer details, valid dates, and no payout link. Use separate eligible stays or edit the invoice as appropriate for existing invoice uniqueness rules.

| Amount entered | Expected amount validity | Expected stored cents after successful save |
|---|---|---:|
| `0` | Valid under existing rules | 0 |
| `0.01` | Valid | 1 |
| `123` | Valid | 12300 |
| `123.9` | Valid | 12390 |
| `123.99` | Valid | 12399 |
| `4234.23` | Valid | 423423 |
| `-0.01` | Invalid: below minimum | No save |
| `123.999` | Invalid: off cent step | No save |

For the incident values, type/paste the value, click the actual submit button, confirm successful creation, reopen the invoice and inspect its PDF. The UI/API must preserve cents and the PDF must show the corresponding two-decimal amount using its existing language format. Repeat saving through the edit form. Confirm the native input reports no `stepMismatch` for cent-aligned values.

Smoke-check decimal cleaning/washing fee entry because those forms explicitly use the same shared step contract. Confirm an integer-step field retains its existing behavior. Record the browser used; invoice language selection is not evidence of browser input locale.

### 5.3 Implementation checks

Run from `frontend/` after implementation:

- `npm test -- src/components/ui/UiInput.spec.ts src/views/InvoicesView.spec.ts`
- `npm run type-check`
- `npm test` (the shared component serves multiple views)
- `npm run build`

Real-browser submission is required in addition to component tests because synthetic form submission can bypass native validation. Record check results and distinguish any pre-existing failures. No tests were executed during this specification-only investigation.

## 6. Clarifications and completion

- **Resolved:** “No mapped stay” means no payout linked; the Named stay exists/is selected.
- **No remaining business question blocks this narrowly defined fix.** The requested dot-decimal examples and the existing cent step provide its input requirement. Browser-independent comma parsing, grouping separators, blank-field policy changes and alternative rounding rules have not been requested or specified.
- The fix is complete when normal browser submission accepts the incident's cent amounts, creation/editing preserve them through API/storage/PDF, and shared-input regressions pass.
- This document records the specification, not an implemented or deployed fix.
