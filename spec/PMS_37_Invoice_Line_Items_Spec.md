# PMS 37 — Multiple items on pro formas and final invoices

## 1. Status and confirmed request

**Status: implemented and verified locally.**

Created on 2026-10-22 after inspecting the existing invoice editor, persistence,
handler amount handling, PDF renderer, and PMS 36 specification. The initial
draft and confirmed technical contract were written before application changes.

The owner requests multiple items on both pro formas and final invoices. The
concrete use case is a Booking.com accommodation charge plus a separate cleaning
item charged on top. The specification must precede implementation; unclear
requirements must be resolved with the owner rather than assumed.

## 2. Baseline implementation inspected before this change

- Both document types use the existing invoice aggregate and editor.
- `backend/internal/store/invoices.go` stores one `AmountTotalCents` value per
  document; there is no persisted item collection.
- `frontend/src/views/InvoicesView.vue` and
  `frontend/src/views/invoices/InvoiceEditorForm.vue` expose one EUR amount.
  Selecting a payout replaces that amount with its billable amount.
- `backend/internal/api/invoice_handlers.go` accepts and returns a document
  total. It also contains payout-based total refresh behavior during invoice
  regeneration. Itemization must account for this behavior so an additional
  cleaning charge cannot disappear during regeneration.
- `backend/internal/invoicepdf/invoicepdf.go` renders an accommodation service
  summary and document total, rather than an editable item breakdown.
- PMS 36 defines pro formas as informational documents for a non-VAT supplier,
  with Booking.com handling payment. Their fixed PDF text explicitly instructs
  the customer not to make a separate payment to the accommodation provider.
- Final invoices currently default to paid, with the note
  `Already paid via Booking.com.` This needs an explicit decision if cleaning
  is collected separately from the Booking.com accommodation payment.

## 3. Owner decisions (2026-10-22)

### 3.1 Cleaning payment

The guest pays cleaning directly to the owner. The owner explicitly confirmed
that the invoice does not need to distinguish this payment route and accepted
the current invoice behavior. Implement itemization without introducing
per-item payment methods, payment allocation, or a separate payment lifecycle.
Cleaning is an additional manually entered item on top of accommodation.

### 3.2 Item fields

Confirmed: editable description, quantity, and unit price, with calculated line
and document totals.

Confirmed: quantities allow fractions. The technical contract uses positive
decimal strings with up to six fractional digits, avoiding binary floating-point
money calculations. Monetary prices remain integer cents.

### 3.3 Item entry and payout updates

Confirmed: start with one editable accommodation row and an **Add item** action
for cleaning or other charges. Use the Booking.com amount to prefill
accommodation when explicitly selected, allow manual editing, and preserve all
saved item amounts on regeneration.

The initial accommodation row has quantity 1 and unit price equal to the full
selected accommodation amount. This avoids inventing a nightly price or
rounding the total by dividing it over nights. The operator can change both
quantity and price explicitly. Additional rows are not overwritten by payout
selection. The first row is the accommodation-prefill target, identified as
such in the editor; no matching by editable description is used.

### 3.4 Preparing the final invoice

Confirmed: provide an explicit **Create final invoice** action on a saved
pro forma. Copy its items and customer details into a new final-invoice draft
for review. Also retain the stay, language, booking reference, guest, stay dates,
and price details. Use current new-invoice defaults for issue, supply, and due
dates and payment note; do not reuse the pro forma's null dates/payment status.

Copying is a frontend draft action, not an immediate write. It uses the existing
POST endpoint only when the operator saves. The final document gets its own ID
and invoice number. Its items and customer object are independent copies, so
editing the draft cannot mutate the source document. Existing same-stay/type
uniqueness remains authoritative; if a final invoice already exists, show that
document rather than offering another final draft.

## 4. Implementation contract

The following incorporates the confirmed workflow and fractional quantities.

1. Add an ordered item collection to both document types using the existing
   invoice routes, editor, and PDF renderer.
2. Support adding, editing, and removing rows, with at least one valid item.
   Each item contains description, quantity, and unit price in cents. Trim and
   require descriptions; preserve order and user-entered descriptions across
   language changes. Localize the initial accommodation description to SK/EN.
3. Calculate the document total from the items on the server and display the
   calculated total in the editor. Store and exchange money in integer cents,
   preserving the existing decimal EUR input contract.
4. Persist the complete item collection and total atomically. Saved descriptions,
   order, and amounts survive reopening and explicit PDF regeneration.
5. Render all items and their amounts in both Slovak and English PDF layouts,
   with wrapping and pagination as needed. Render localized Description,
   Quantity, Unit price, and Total columns. Repeat column headings on item-table
   continuation pages and keep the total and following notes clear of the footer.
6. Introduce a new sequential migration pair. Preserve historical document
   totals, numbers, associations, and PDF versions. Define how legacy single-total
   documents are represented as one item and how older API payloads are handled
   in the final technical specification.
7. Update OpenAPI, handwritten frontend invoice types, and generated types.
8. Retain the payment behavior accepted in section 3.1. This feature adds no
   payment-method field or separate cleaning-payment tracking.

### 4.1 Persistence and compatibility

Add nullable `line_items_json` to `invoices` in migration
`000046_invoice_line_items`. It stores ordered document snapshots in the existing
aggregate. NULL denotes a legacy single-total document. Reads expose such a
document as one localized accommodation item, quantity `"1"`, and unit price
equal to its saved total. Existing PDF bytes and invoice identity are untouched.
The migration down refuses while explicit item snapshots exist rather than
silently discarding their descriptions or quantities.

POST accepts `items` or the legacy `amount_total_cents` (including existing
payout prefill). PATCH with omitted/null items preserves saved items. An explicit
empty array is invalid. Legacy total-only updates remain supported for legacy
documents; on itemized documents, a total-only change is rejected unless it
equals the saved item sum. Sending items replaces the full ordered collection.
If a total is also supplied, it must equal the calculated sum. All writes persist
items and total together in the existing invoice transaction.

Both list and detail responses include `items`, with server-calculated
`line_total_cents`. PDF regeneration preserves saved totals for all documents;
the old automatic payout refresh is removed. Payout imports never edit invoice
items. Explicit payout selection in the editor resets only the first item's
quantity to 1 and price to the selected amount, with visible explanatory help.

### 4.2 Exact arithmetic and validation

An item contains `description`, `quantity` (decimal string), and
`unit_price_cents` (integer). The response also contains `line_total_cents`.
Support 1–100 items, descriptions of 1–500 Unicode characters, quantities
greater than zero up to 1,000,000 with at most six decimal places, and nonnegative
unit prices. Reject exponent notation and nonfinite/negative values. Limit both
unit prices and the aggregate total to 1,000,000,000,000 cents, safely within
JavaScript's exact integer range. These are implementation bounds, not business
defaults.

Calculate `quantity × unit_price_cents` using exact integer/decimal arithmetic,
round each line half-up to whole cents, then sum rounded line totals. The API is
authoritative; the frontend uses the identical rule for live totals. For example,
`1.5 × 101 cents = 152 cents`. A final invoice may total zero under the existing
contract; a pro forma must still have a positive total. The server ignores any
client-supplied line total and derives it from quantity and price.

### 4.3 Copy-to-final draft details

Copy from the saved pro forma, not unsaved edits. Clear selection and PDF history
in the new draft, refresh the final-invoice number preview, leave payout unlinked,
and allow the operator to review before saving. Preserve the supplier's explicit
non-VAT declaration when creating the final draft. The existing property-scoped
routes and uniqueness constraints protect the eventual save.

## 5. Acceptance scenarios

- Create a pro forma containing accommodation and cleaning as distinct items;
  the PDF shows both and a correct total.
- Create the corresponding final invoice with distinct items, using the workflow
  confirmed in section 3.4; changes to the final draft do not modify the pro forma.
- Decimal item amounts remain exact through save, reload, edit, and regeneration.
- Selecting or linking a payout and regenerating a document cannot silently
  remove cleaning or double-count a fee included in Booking.com's price.
- No per-item payment distinction is added, as confirmed by the owner.
- Existing single-total documents remain readable and editable with their
  original amounts and previously generated PDFs intact.
- Invalid item inputs and inconsistent totals fail without partial persistence.
- Long descriptions and multiple items remain legible in SK/EN PDFs, including
  documents requiring multiple pages.

## 6. Implementation and verification plan

Implement persistence/API, editor, and PDF rendering using existing project
patterns. Add focused regression coverage for monetary totals, saved items,
payout interactions, legacy compatibility, and both document types. Run relevant
Go and frontend tests, OpenAPI generation, type checking, and builds. Record
actual results here after execution.

## 7. Implementation and verification results

Implemented on 2026-10-22:

- Migration `000046_invoice_line_items` adds the nullable ordered item snapshot.
  Legacy totals and existing PDF versions remain intact. Its down migration
  refuses to discard explicit item snapshots.
- The shared backend `invoiceitems` package validates fractional quantities and
  computes line totals with exact arithmetic. API create/update/list/detail and
  PDF generation carry the item snapshots. Regeneration preserves saved amounts.
- The existing editor supports add/edit/remove rows and live totals. Payout
  selection changes only the first row's quantity and price.
- A saved pro forma offers **Create final invoice**, copying saved data into an
  independent reviewable draft. If a final invoice already exists for the stay,
  the action becomes **Open final invoice**. Copied dates use the property's
  timezone, and the supplier's explicit non-VAT declaration survives creation
  and subsequent regeneration.
- SK/EN PDFs show descriptions, fractional quantities, unit prices, line totals,
  and the summed total. Tables repeat headings across pages; unusually long
  descriptions can continue across pages. The footer uses its reserved bottom
  space rather than creating an otherwise empty page for ordinary two-item
  documents. Price details remain within their service-summary card.
- OpenAPI and handwritten/generated frontend types are updated. Quantity is a
  decimal string on the API, while money remains integer cents.

Checks completed successfully:

- Backend focused tests for invoice items, API, store, PDF, migrations, and
  PMS-21 cleanup; `go test ./...`; `go build ./...`; `go vet ./...`.
- Frontend OpenAPI generation, type-check, all 55 test files / 331 tests,
  production build, and lint.
- After the final PDF layout fix, reran PDF/API tests, backend build and PDF vet.
  After frontend/schema finishing changes, reran OpenAPI generation, type-check,
  the 29 focused invoice tests, and lint.
- `git diff --check` passed.
- Visually inspected synthetic English and Slovak pro formas and an English
  final invoice. The representative two-item SK/EN documents fit on one page.
  Automated PDF assertions verify all 100 item descriptions/amounts, repeated
  headings, long-description continuation, totals, and the no-orphan-footer case.

Regression coverage includes half-cent rounding, six-decimal quantities,
per-line rounding before summation, invalid/missing fields, overflow, atomic
rejection of inconsistent totals, edit/reload/regeneration of both document
types, legacy compatibility, payout-linked total preservation, independent
copy-to-final drafts, version history, and guarded migration rollback.

Real-browser end-to-end interaction was not run. Verification used temporary
databases/files and synthetic document data; no production document was issued.
