# PMS 36 — Booking.com pro forma documents for a non-VAT landlord

## 1. Scope and approval status

**Status: approved and implemented.** Automated verification is recorded in the
implementation notes below; browser/PDF visual checks remain environment-dependent.

Investigated on 2026-10-21 against source commit
`dba012cfa5ff545f7f8b90f80b6196919edb1651` and the owner's answers. Source and
public legal/platform guidance were inspected. No production reservation,
Booking.com payment status, local business database, or accountant's records
were inspected. Application changes use synthetic/test data only.

### 1.1 Confirmed business facts

- The supplier is an individual renting a property, with a Slovak **DIČ only**;
  the owner describes the supplier as not VAT-registered.
- The apartment is in Bratislava. Booking.com handles the reservation and guest
  payment; the owner receives the Booking.com payout after the stay.
- The guest requested a preliminary invoice addressed to a Bulgarian company
  for reservation `6429185079`, covering approximately one month.
- The owner is **not requesting an additional payment** from the company.
- Required workflow: issue a pro forma before the stay, then independently
  issue the usual final invoice after the stay. Download and manually send
  SK/EN PDFs through the existing workflow.
- There is no established pro forma numbering series or external template.

The guest wrote that the company needs a document so it can pay before arrival.
The owner's suggestion that this is for employee reimbursement is an
**unconfirmed explanation**, not a fact to encode in the PDF. The document
supports company approval/administration; it must not promise that the company
will accept it as evidence of an incurred or paid expense.

### 1.2 Proposed product decisions for approval

The following are design decisions proposed by this specification, rather
than statements that the owner has already selected their technical details:

1. Add **Pro forma** as a document type in the existing Invoices module.
2. Keep a separate pro forma number series, using `PF/` before the existing
   property/year/sequence format.
3. Allow one pro forma and one final invoice for the same Named stay.
4. Keep both documents and their PDF histories; never turn an issued pro forma
   into a final invoice by changing its type or reusing its number.
5. This release supports the confirmed **non-VAT supplier / Booking.com
   collects payment** pro forma scenario. It is not a general advance-payment
   accounting module.

## 2. Business and legal interpretation

### 2.1 What the customer is asking for

It is reasonable for an accommodation provider to supply a pro forma or cost
confirmation for a business traveller's employer before arrival. This is an
optional administrative document in this scenario, not evidence that
Booking.com requires every host to issue pro formas.

Booking.com's public FAQ says that the property provides the invoice for a
completed stay. Booking.com's customer terms distinguish the accommodation
provider from the platform and state that, when Booking.com organizes payment,
the customer's payment settles the due and payable price. Consequently:

- The host can provide a pro forma even before a payout exists.
- Booking.com's collection from the customer and its later payout to the host
  are separate events. Lack of a host payout does not establish that the guest
  has not paid.
- A pro forma must not assert either `paid` or `unpaid` without evidence. For
  this release it makes **no payment-status assertion at all**.
- The company must follow the reservation's Booking.com payment arrangements.
  A host-issued pro forma does not create a new bank-transfer route or change
  the card/payment schedule.
- The final invoice covers the accommodation supplied, not the net amount
  remaining after Booking.com's commission. Do not subtract platform fees
  from the customer-facing service price.

### 2.2 Slovak treatment for the confirmed supplier

| Topic | Conclusion for this feature | Implementation consequence |
| --- | --- | --- |
| DIČ versus VAT registration | A DIČ is not an IČ DPH and is not evidence of full VAT-payer status. The owner explicitly confirmed the DIČ-only scenario. | Show the actual DIČ; do not manufacture an IČ DPH or an IČO. Do not infer every property's tax status from an empty VAT ID. |
| Pro forma status | The document describes a proposed/reserved service and price. It does not itself prove receipt of money or completed supply. | Clearly label it pro forma, not a tax invoice or receipt. Issuing it must not post revenue, payment, a receivable, or a VAT event in PMS. |
| VAT amounts | The confirmed non-VAT supplier does not charge output VAT on this document. VAT Act §69(5) separately makes a person who states VAT on a sales document liable for that VAT. | Do not add 5%, 23%, a VAT amount, a VAT breakdown, or `0% VAT`. Non-registration is not a zero VAT rate. |
| Bulgarian company | Under VAT Act §16(1), services relating to Slovak immovable property, including accommodation, have their place of supply in Slovakia. | A Bulgarian address or BG VAT number must not trigger a general cross-border B2B reverse-charge template. |
| Advance VAT invoice | VAT Act §19(4) concerns tax becoming chargeable on a received advance; §§72–74 address relevant VAT invoicing obligations. Those VAT-payer rules are not the workflow requested for this confirmed non-VAT supplier. | Do not generate a VAT advance invoice, payment-received date, or 15-day VAT-invoice reminder from creation of this pro forma. |
| Final document | The owner's ordinary completed-stay invoice remains a separate document. A pro forma is not an advance payment to deduct from it. | Issue the final document for the actual service price, without subtracting the pro forma amount as if money had been collected twice. |
| Accounting evidence | Accounting Act §10 addresses documentation of an accounting event, and §11(3) prohibits recording an event that has not occurred. Applicability to an individual depends on the accounting/tax-record regime, not possession of a DIČ alone. | Do not claim this landlord is necessarily an accounting entity or that the pro forma is the final accounting document. Keep it as supporting correspondence. |

The stay's precise dates, detailed price, the company's legal details, and the
supplier's accounting/tax-record regime have not been supplied. They are not
needed to specify the narrowly scoped document feature. Enter the actual
booking data when issuing it. This specification does not determine annual
income-tax recognition dates, tax-registration obligations, or whether the
underlying activity is legally residential letting versus accommodation.
Neither a one-month duration nor an employer's involvement determines that
classification on its own.

### 2.3 Local tax and price boundaries

The Bratislava location and a one-month stay are insufficient to calculate a
local accommodation-tax charge or exemption. Do not hard-code a local rate,
night cap, work-travel exemption, or an assumption that Booking.com collects
every local charge.

The operator enters the customer-facing amount from the actual reservation,
including only the charges represented by that amount. Provide an optional
price-details note for explaining inclusions/exclusions, such as a local tax
collected separately, when supported by the booking. Do not label an unknown
amount as “all taxes included.” No new local-tax calculation is required.

### 2.4 Sources and research boundary

Sources accessed on 2026-10-21:

1. [Slovak VAT Act 222/2004 Z. z., version effective 2026-01-01 to 2026-12-31](https://static.slov-lex.sk/static/SK/ZZ/2004/222/20260101.html):
   §16(1), §19(4), §69(5), §§72–74. These support the place-of-supply,
   VAT/advance distinction and invoicing analysis; they do not prescribe a
   special `PF/` numbering format.
2. [Slovak Accounting Act 431/2002 Z. z., version effective 2026-06-01 to 2026-12-31](https://static.slov-lex.sk/static/SK/ZZ/2002/431/20260601.html):
   §1, §10 and §11(3). Its accounting-entity scope must not be automatically
   attributed to the owner or the Bulgarian company.
3. [Booking.com FAQ](https://www.booking.com/faq.html), Booking Details,
   “How can I get an invoice?”: “Only the property can provide an invoice for
   your completed stay.”
4. [Booking.com customer terms](https://www.booking.com/content/terms.html),
   A4.3 and A8.1: supplier/platform roles and settlement where Booking.com
   organizes payment. The accessed page identifies its update as September
   15, 2025. These are general platform terms, not this reservation's payment
   schedule or the host's individual partner agreement.

The Slov-Lex HTML pages identify themselves as informative versions and link
to legally binding PDFs. The conclusions above are based on the accessed HTML
text. Recheck applicable effective law if the feature's tax scope changes or a
later legal period introduces relevant changes. No Bulgarian reimbursement
eligibility or company-specific expense policy has been established.

## 3. Current implementation evidence

Read `PMS_13_Coding_Conventions.md`, `AI_Agent_Coding_Style.md`,
`PMS_07_Design_Language.md`, and the current Named stay authority in
`PMS_21_Legacy_Occupancy_Removal_Spec.md` before implementation. Preserve the
decimal-money contract documented in PMS 26. Apply this feature through the
existing Go handlers/store, Vue editor, SQLite migrations and Go PDF renderer.

| Area | Current evidence | Necessary change |
| --- | --- | --- |
| Invoice identity | `backend/internal/migrate/000039_legacy_occupancy_removal.up.sql`: unique `(property_id, named_stay_id)` and `(property_id, sequence_year, sequence_value)` | Distinguish document types in stay and sequence uniqueness. Merely changing the PDF heading cannot permit both documents. |
| Number allocation | `backend/internal/store/invoices.go`: `FormatInvoiceNumber`, `PreviewNextInvoiceNumber`, `CreateInvoice`, `currentInvoiceSequenceTx`; `invoice_sequences` was created in migration 11 | Reuse transactional allocation, scoped additionally by document type. |
| Payment defaults | `invoice_handlers.go`, `buildInvoiceRowFromBody`, and store `CreateInvoice` default to `paid` / `Already paid via Booking.com.` | Pro formas need their own server-side invariant and must never enter these defaults. |
| Date defaults | Handler defaults taxable-supply and due dates to issue date; schema makes both required | Pro forma has neither a tax point nor a new host-imposed payment due date. Represent their absence honestly. |
| PDF | `backend/internal/invoicepdf/invoicepdf.go`: English subtitle `Tax invoice`, paid badge, `Amount due`, invoice number labelled as variable symbol, and paid fallback note | Make document-specific rendering explicit; a changed title alone is insufficient. |
| Supplier | `defaultInvoiceSupplier` uses billing/legal-owner name, but puts the property marketing name in `CompanyName`; `partyLines` prints company name first | In the new individual-landlord pro forma, identify the legal individual as supplier and the apartment separately. |
| Company identifiers | API/PDF party structures already have `ICO`, `DIC`, `VATID`, but `normalizeInvoiceCustomer` drops ICO/DIČ and the editor exposes only VAT ID | Preserve optional identifiers through normalization and editor round trips; use a suitable company-registration label for foreign customers. |
| Existing editor | `frontend/src/views/InvoicesView.vue`, `invoices/InvoiceEditorForm.vue`, `InvoiceList.vue`, `InvoiceFilesTable.vue` | Extend the existing form/list/version workflow with a type selector and conditional fields. |
| Pre-arrival stay selection | `ListNamedStayFinanceCandidates` includes active `booking_com`/`external` stays without requiring finance data | Future Booking.com Named stays can already be candidates. Do not require a payout or create a synthetic finance row. |
| Dashboard | `backend/internal/api/server.go` uses `ListInvoices` for recent invoices | Exclude pro formas before applying the three-invoice limit. |
| Finance linkage | `store/finance_booking_payouts.go` derives `linked_invoice_id` from invoices | Only final invoices may occupy that linkage. |
| Finance reset | `store/finance_reset.go` deletes invoices/files also by matching Named stay, even without a payout link | Exclude pro formas consistently from reset deletion, file cleanup and deletion counts. |
| Historical cleanup tooling | `backend/internal/pms21cleanup/cleanup.go` encodes one-invoice-per-stay and an exact invoice column inventory | Account for both schema generations when current-schema validation runs; do not falsify migration-39 historical tests. |

The working tree already contains an untracked PMS 35 specification. Preserve
it and inspect the then-current working tree before implementation.

## 4. Required operator workflow

### 4.1 Create a pro forma

1. Select the property and choose **Pro forma** in the existing invoice editor.
   The form explains: “For a non-VAT supplier. Booking.com handles payment;
   this document does not request a separate payment.”
2. Select an active `booking_com` Named stay. No finance/payout record is
   required. Prefill the planned stay dates from the selected stay. A display
   name may suggest a guest name, but must remain editable; an internal stay
   label is not necessarily the guest's legal name.
3. Enter/confirm the Booking.com reservation reference, actual guest name,
   company billing details, issue date, planned stay period and full
   customer-facing reservation amount in EUR.
4. Show the supplier's legal name, address and DIČ using the current property
   billing profile. Missing legal identity/address/DIČ must produce a clear
   field/profile validation message rather than a PDF with the apartment name
   impersonating the legal supplier. An IČO is not required for this landlord.
5. Optionally enter price details. The standard payment explanation is fixed
   by the application, not an editable “already paid” note.
6. Create the record, allocate a pro forma number and generate its first PDF
   version using the existing save/render/attach flow.
7. Download and send it manually. The PDF does not request a bank transfer,
   change Booking.com's payment terms, or state that any money was received.

The reservation number in this specification is an acceptance example, not
configuration or a hard-coded value. Keep reservation references as strings;
do not parse them as numbers or confuse them with a payout ID or ICS UID.

### 4.2 Edit and regenerate

- Reuse the existing edit-and-create-next-PDF-version behavior. The record ID,
  document type and allocated number remain stable.
- After creation the pro forma's Named stay association is immutable; correct
  its document details without reassigning it to a different reservation.
- Customer, guest, reference, dates, amount and price details are stored
  document values. Changes in a stay, company or payout must not silently
  rewrite them.
- Explicit regeneration may refresh supplier profile details, following the
  current convention. Preserve the stored non-VAT declaration and the correct
  individual-supplier mapping during that refresh. Do not refresh a pro forma
  amount from a payout.
- Keep previously attached PDF versions under the existing file-history rules.
  A failed PDF render must not falsely report success. The saved record remains
  discoverable and regeneration retries use its existing number; do not tell
  the operator to create a duplicate record to recover.
- An existing pro forma remains readable/downloadable if the stay is later
  cancelled. No refund, credit note, automatic final invoice, or new
  cancellation-document lifecycle is introduced.

### 4.3 Issue the final invoice later

Use the existing **New invoice** workflow for the same Named stay after the
service ends. Enter the actual service price and the correct company details;
optionally copy the guest/reference/price details manually. No conversion
endpoint or automatic finalization is required.

The final invoice gets its own ID and normal invoice number. The pro forma
continues to exist. It neither consumes the final invoice slot nor blocks
selection of the actual payout. Creating a final invoice must not modify,
renumber, or delete the pro forma or subtract its amount.

Payment evidence for the final invoice remains the existing completed-stay
process. This feature must not use the mere existence of a pro forma as proof
of payment or completion. The current final-invoice paid default is a known
baseline, not a general payment-verification capability added by this spec.

## 5. Document content and PDF contract

### 5.1 Required content

- Prominent title: **Pro forma invoice** / **Proforma faktúra**.
- Pro forma number, issue date and Booking.com reservation reference.
- Supplier: legal individual's name, billing address/country and DIČ.
  Apartment name is separate accommodation identification, not a company name.
- Customer: company legal name and billing address/country; optional company
  registration number, tax ID and VAT ID as supplied. A BG VAT ID is optional
  and does not change supplier VAT treatment.
- Guest name shown separately from the customer company/contact.
- Apartment/property identification and **planned** stay dates.
- **Reservation amount (EUR)** / **Cena rezervácie (EUR)**, using exact cents
  and existing SK/EN monetary formatting. Do not say `Amount due`.
- Optional price-details text, wrapped without overlapping the fixed text.
- Non-VAT supplier statement and mandatory document/payment explanation below.

### 5.2 Mandatory localized text

English:

> The supplier is not registered for VAT.
>
> This pro forma is issued for information and company approval. It is not a
> tax invoice or proof of payment. Payment for this reservation is handled by
> Booking.com under the reservation's payment terms. Do not make a separate
> payment to the accommodation provider based on this document. A final
> invoice will be issued after the stay.

Slovak:

> Dodávateľ nie je platiteľom DPH.
>
> Táto proforma faktúra slúži na informačné účely a na schválenie nákladov
> spoločnosťou. Nie je daňovým dokladom ani potvrdením o úhrade. Platbu za túto
> rezerváciu spracúva Booking.com podľa platobných podmienok rezervácie. Na
> základe tohto dokladu neuhrádzajte samostatnú platbu poskytovateľovi
> ubytovania. Konečná faktúra bude vystavená po skončení pobytu.

Render these application-owned texts according to document language on every
creation/edit/regeneration. They cannot be replaced by `payment_note` or the
existing renderer's paid fallback. This is a required part of this document
type, not an optional user disclaimer field.

### 5.3 Elements that must not appear on a pro forma

- `Tax invoice` subtitle or a VAT-tax-document claim.
- `PAID` / `ZAPLATENÁ`, `UNPAID`, an amount received, a payment-received date,
  or a claim that Booking.com has already collected the money.
- Taxable-supply date or a host-set payment due date, including fabricated
  defaults equal to the issue date.
- “Amount due,” bank details, QR payment instructions, or variable-symbol
  labelling on the pro forma number.
- A tax rate, VAT calculation, reverse-charge phrase, or invoice to Booking.com
  instead of the actual customer.

Use the existing `invoicepdf` renderer, font assets, layout, wrapping and
language helpers. Extend them with explicit document-type conditions; do not
fork a second PDF implementation or introduce a template engine.

For normal invoices, replace the unconditional English `Tax invoice` subtitle
with a neutral `Accommodation invoice`. Existing historical PDF files are not
rewritten. This prevents the confirmed non-VAT owner's newly generated final
invoice from automatically claiming to be a VAT invoice; it is not a general
VAT calculation feature.

## 6. Persistence, numbering and compatibility

### 6.1 Extend the existing invoice aggregate

Use `invoices`, `invoice_sequences` and `invoice_files`. Keep current IDs,
property permissions, file storage and API resource ownership. Do not introduce
a parallel pro forma service/repository framework.

Required persisted additions/changes:

| Field | Contract |
| --- | --- |
| `document_type` | Required enum `invoice` / `proforma`; migrated rows and omitted type on legacy POSTs default to `invoice`. Immutable after creation. |
| `booking_reference` | Nullable text; trimmed non-empty string required for a pro forma; optional for a normal invoice. Maximum 100 characters. |
| `guest_name` | Nullable text; required for this pro forma; optional for existing normal invoices. Maximum 200 characters. |
| `price_details` | Nullable text; optional operator-entered price inclusions/exclusions, maximum 1000 characters. Not a replacement for fixed payment text. |
| `supplier.vat_status` in supplier snapshot JSON | Optional enum `non_vat_payer`. Required for pro formas; absent on migrated invoices, meaning unspecified rather than automatically non-VAT. Supplied explicitly by the non-VAT document form and preserved through regeneration. |
| `taxable_supply_date`, `due_date` | SQL NULL and API null for pro formas. Retain normal-invoice date behavior and validation. Use existing nullable-time/pointer conventions. |
| `payment_status` | Server-owned `not_applicable` for pro formas; existing normal-invoice values remain unchanged. It means this document does not attest payment, not that the stay is free. |
| `payment_note` | Empty stored value for pro formas; render fixed language-specific text instead. Preserve existing normal-invoice behavior. |
| `finance_booking_payout_id` | Always NULL for pro formas. A reference string is documentary information, not a finance association. |

The pro forma form presents supplier VAT status as **Non-VAT payer**, with its
supported scope clearly visible, and sends `supplier_vat_status:
"non_vat_payer"`. This is an explicit document declaration based on the
confirmed scenario, not an inference from the property's DIČ/VAT fields. Do not
backfill it onto old documents or add a general tax-status administration UI.
If the supplier's circumstances change, the non-VAT pro forma mode is no longer
the appropriate mode; this spec does not define a VAT-payer variant.

Preserve exact-cent storage and EUR for this workflow. Pro forma validation
requires a positive amount and a planned end date after the start date. These
are document-product requirements, not changes to normal invoices' existing
non-negative amount policy.

### 6.2 Uniqueness and numbering

- Replace stay uniqueness with `(property_id, named_stay_id, document_type)`.
  This permits one pro forma and one final invoice, while duplicate same-type
  creation remains a conflict. Revisions use the existing record and versions.
- Add `document_type` to the uniqueness of `(property_id, sequence_year,
  sequence_value)` and to the sequence counter's `(property_id, sequence_year)`
  key. Migrate all existing counters as `invoice` with unchanged values.
- Preserve `(property_id, invoice_number)` uniqueness. Keep existing final
  invoice format and allocation unchanged.
- Pro forma format: `PF/<existing formatted property/year/sequence number>`.
  With code `APT`, example `PF/APT/2026/0001`; without code, example
  `PF/P001/2026/0001`.
- Each property's pro forma counter begins independently at 1 for its year.
  Creating/editing/regenerating a pro forma never advances the final invoice
  counter; previews never reserve numbers.
- For the new pro forma series, derive the year from the issue date in the
  property timezone. Keep the existing final-series year behavior; do not
  renumber historical invoices. Freeze the allocated sequence year/number and
  reject a pro forma edit that moves its issue date outside that year.
- Allocate the number, persist the document and update the counter in one
  transaction using existing store patterns. Concurrent creation must not
  allocate duplicate numbers or leave two pro formas for one stay.
- Retain the unique payout linkage for final invoices. Add a database check
  enforcing NULL payout, NULL supply/due dates and `not_applicable` status for
  pro formas, with corresponding handler/store enforcement.

### 6.3 Migration requirements

Create a new sequential `up` and `down` migration pair. The highest migration
observed was 44; determine the next available number at implementation time.
Do not edit migration 11 or 39.

The inline unique constraints and date nullability require a SQLite table
rebuild. Use the repository's transactional rebuild conventions, explicitly
preserving invoice IDs, file rows, autoincrement high-water marks, supplier and
customer snapshots, file paths, version numbers, dates, amounts and counters.
The `invoice_files` foreign key uses `ON DELETE RESTRICT`: do not assume dropping
the parent table is safe without handling its children/deferred constraints.
Preserve the composite property/Named stay/finance foreign keys and verify
`foreign_key_check` and `integrity_check` on the upgraded database.

New invoice rows and counters get default type `invoice`; existing PDFs are
not regenerated by migration. The rollback must not silently delete pro formas
or force two documents into the old one-per-stay schema. It must refuse with a
clear failure while pro forma records/counters or other unrepresentable new
document data exist. Exercise an up/down round trip on representable legacy
data and a rejected, non-destructive down on populated new data.

## 7. API and frontend contract

### 7.1 Reuse existing routes

Extend the existing `/api/properties/{id}/invoices` create/list/detail/update,
regenerate and download routes. Continue using `permissions.Invoices`,
`requirePropertyModuleAccess`, `ReadJSON`, `WriteJSON`, `WriteError` and audit
conventions. Both types are returned in the Invoices module list.

- POST accepts `document_type`, `booking_reference`, `guest_name`,
  `price_details`, and the explicit `supplier_vat_status` declaration described
  above, alongside existing fields. No client-supplied supplier identity
  replaces the property billing profile.
- Existing clients omitting `document_type` create normal invoices. PATCH
  omitting it retains the saved type; supplying a different type is HTTP 400.
- Pro forma POST/PATCH rejects non-empty taxable-supply/due dates, non-empty
  `payment_note`, and any nonzero `booking_payout_id` or non-empty
  `booking_payout_reference`. Omitted/null dates are accepted and stored as
  NULL. Do not silently resolve a payout and then discard it.
- Pro forma creation requires an active Booking.com Named stay in the current
  property, real guest/reference, company name and complete company billing
  address (street, city, postal code, country), valid supplier profile, declared
  `non_vat_payer`, valid dates and positive integer cents. Optional customer
  identifiers must survive normalization. Do not impose Slovak-only identifier
  formats on a Bulgarian customer.
- Pro forma edits retain same-stay ownership. Historical documents remain
  readable after cancellation; do not reapply active-stay creation eligibility
  to download or ordinary regeneration.
- On PATCH, omitted/null new metadata/declaration fields retain their saved
  values, using the existing request-pointer pattern. An explicit blank string
  clears an optional text field; clearing required pro forma reference/guest
  data or its non-VAT declaration is HTTP 400. Validate
  the resulting document, not only the fields present in the patch. For normal
  invoices an omitted supplier VAT declaration remains unspecified; do not
  derive one from customer identifiers or overwrite an existing declaration.
- `payment_status` remains server-controlled. Return the saved type and new
  fields, nullable dates, and `supplier.vat_status` in the existing response
  envelopes. Reject unsupported document types/status declarations as 400.
- Duplicate same-type/stay is HTTP 409 with a message distinguishing pro forma
  from final invoice. Cross-property IDs follow existing denial conventions.
- Extend `/invoice-sequence/next-preview` with optional
  `document_type=invoice|proforma`, defaulting to `invoice`. Return the type with
  the preview so the UI cannot mislabel a stale response. Preview and save
  must agree on the selected series/year; the allocated number remains the
  authoritative result under concurrent requests.

Update `spec/openapi.yaml`, handwritten `frontend/src/api/types/invoice.ts`, and
the generated OpenAPI types through the existing generation command. Inspect
invoice consumers before changing nullable date types; avoid `.slice()` calls
on null values.

### 7.2 Existing view/editor changes

- Add a `UiSelect` document-type selector in create mode. In edit mode show
  the saved type as read-only. Defaults remain normal invoice.
- Use the current `ref` form, `defineModel`, `api()` client, buttons, badges,
  banners and version table. No new page, store architecture or generic
  document builder.
- In pro forma mode show the supported non-VAT/Booking.com explanation,
  reservation reference, guest name, company details, issue/planned dates,
  reservation amount and optional price details.
- Provide a required, editable **Booking.com reservation number** text input,
  bound to `booking_reference`, with example placeholder `6429185079`. The
  operator must be able to enter it manually even when no payout or imported
  reservation statement exists. Selecting a Named stay does not replace this
  input or establish that its display name contains the reservation number.
  Save and reload the entered value and print it on the PDF under the same
  label (Slovak: **Číslo rezervácie Booking.com**). Keep it distinct from the
  PMS pro forma number and any Booking.com payout reference.
- Hide payout selection, taxable-supply date, due date and editable payment
  note. Switching into pro forma mode clears their draft values and prevents
  sending them. Switching back restores normal-invoice defaults rather than
  retaining `not_applicable` or missing required invoice dates.
- Prefill planned dates on selecting the stay for a new pro forma; do not
  overwrite saved snapshots just because a document is reloaded.
- Keep optional company registration/tax/VAT identifiers round-trip safe in
  both modes. The customer/contact name must not replace the company name;
  the separate guest field must not be put in the supplier/customer identity.
- Add an explicit **Pro forma** / **Invoice** badge in the list. Label the
  mixed list/count as documents. Type, dates and amount must remain clear.
- Refresh number preview when draft type/year changes. Discard stale async
  preview responses for a previously selected property/type/year.
- Keep `UiInput`'s `step="0.01"` and cent conversion; do not round prices to
  whole euros. Required fields must be visibly identified.
- Display intended calendar dates in the property timezone when reopening or
  rendering a pro forma. Do not use a UTC string's first ten characters when
  that changes a Bratislava midnight date. Preserve the project's UTC wire/
  storage convention and test local date/sequence-year boundaries.

## 8. Integration boundaries and implementation order

### 8.1 Required downstream handling

1. **Finance/analytics:** pro forma create/edit/regenerate must not create or
   change `finance_bookings`, transactions, payouts, manual stay revenue,
   recognized revenue, occupancy, cleaning or Nuki data. A separate document
   record does not represent an additional sale.
2. **Dashboard:** existing “Recent invoices” continues to show final invoices
   only. Filter before taking three entries so newer pro formas do not hide
   eligible final invoices.
3. **Payout linkage:** `linked_invoice_id` continues to mean the final invoice.
   Filter explicitly by type in the query even though pro formas cannot have
   a payout foreign key.
4. **Finance reset:** preserve pro forma rows, PDFs and counters, including
   pro formas whose Named stay has acquired a finance row since issuance.
   Apply identical type predicates to deletion counts, row/file deletion and
   physical file-path cleanup. Preserve existing final-invoice reset rules.
5. **Backup:** keep both types under the existing `invoices/` data directory
   and existing backup coverage. The `PF/` number prefix must survive filename
   sanitization without colliding with a final invoice's PDF.
6. **PMS 21 tooling:** preserve the historical migration-39 schema contract on
   migration-39 fixtures. Where readiness/verification runs against the new
   schema, use type-aware uniqueness and column expectations selected by the
   installed schema. Do not weaken property/stay/finance ownership checks or
   rewrite historical migrations to make tests pass.

### 8.2 Implementation sequence after approval

1. Reinspect working tree and current schema. Add migration and store support
   for types, nullable dates, metadata, independent counters and uniqueness.
2. Extend handler validation, snapshots, DTOs and sequence preview; handle
   pro forma branches before payout resolution and paid/date defaults.
3. Extend the existing PDF renderer with the content in section 5 and version
   recovery behavior. Preserve historical file bytes.
4. Extend the existing Vue editor/list and typed contracts; complete the
   downstream query/reset/tooling changes in section 8.1.
5. Update OpenAPI/generated types, add focused regressions and perform the
   verification below. Update documentation with actual implementation status
   only after checks have run.

## 9. Acceptance criteria and verification

### 9.1 Main acceptance scenario

Use synthetic personal/company data. `6429185079` is the owner's supplied
reservation reference; no actual price or dates are known. A test amount such
as **EUR 1,234.56** is deliberately illustrative.

1. An active future Booking.com Named stay exists with no payout/finance row.
2. The property billing profile contains an individual's legal name, address
   and DIČ, with no invented IČO or VAT ID.
3. The operator creates an English pro forma for a Bulgarian company, entering
   reservation reference, guest name, planned dates and `123456` cents.
4. It receives a pro forma number; the normal-invoice preview/counter is
   unchanged. The PDF has all mandatory content and no paid badge, tax-invoice
   claim, due date or instruction to pay the host directly.
5. Reopen, edit and regenerate. Cents and calendar dates remain exact; the
   record and number remain stable; a new PDF version is attached and existing
   versions remain intact.
6. Later create a normal final invoice for the same Named stay, optionally
   linking its actual payout. Both records coexist; normal duplicate-invoice
   protection still operates; no amount is deducted merely because the pro
   forma exists.
7. Finance totals do not increase when the pro forma is created or revised.

### 9.2 Focused automated regressions

**Store and migration**

- Upgrade a populated legacy fixture: all final documents, IDs, amounts,
  snapshots, counters, file paths and version history are unchanged.
- One final plus one pro forma for a stay succeeds; duplicate same-type/stay
  and cross-property references fail. Exercise concurrent creation through
  the real store and prove uniqueness/counter integrity.
- Independent numbering, preview without writes, create rollback and
  regeneration without new allocation; pro forma year-boundary behavior.
- SQL NULL dates and payout, fixed `not_applicable` status, and constraints
  rejecting invalid pro forma rows even if HTTP validation is bypassed.
- Upgrade integrity checks; safe downgrade behavior from section 6.3.
- Finance reset keeps pro formas/files/counters when the same Named stay has a
  finance row, while still applying established final-invoice reset behavior.

**API and PDF**

- Full create/detail/edit/regenerate/download flow using `httptest` and a real
  test store; no payout required. Normal invoice requests omitting type retain
  their previous behavior.
- Invalid type/type mutation, missing company/guest/reference/supplier data,
  wrong stay type/property, zero/negative amount, invalid period, forbidden
  payout/date/payment-note inputs, and duplicate same-type requests.
- Customer registration/tax/VAT identifiers survive round trips, including a
  Bulgarian company and Unicode names. A BG VAT ID never triggers VAT or
  reverse-charge rendering.
- Mandatory SK/EN pro forma text, titles, reservation amount and non-VAT
  statement; no paid badge, tax-invoice subtitle, fabricated date or variable
  symbol. An empty payment note cannot reach the old paid fallback.
- Non-VAT individual is the supplier; the apartment and guest are separately
  identifiable. Long Bulgarian company/address strings and long price details
  wrap correctly. If a page break is needed, mandatory text remains visible.
- Regeneration preserves document snapshots as specified; later payout
  imports do not change the pro forma amount or payment meaning.
- Failed initial rendering leaves an identifiable record recoverable through
  regeneration with the same number. Do not assert success or allocate a
  replacement number.
- Dashboard excludes pro formas before its limit; payout candidates link
  only to final invoices; permission boundaries apply to both types.
- PDF tests must check meaningful document content/branches, not only that the
  output exceeds a byte threshold. Visually inspect representative SK/EN PDFs
  in addition to automated assertions.

**Frontend and real-browser acceptance**

- Type changes show/hide the correct controls, clear forbidden values, refresh
  the correct preview, and produce the intended typed create/update payload.
- Test switching back to normal invoice, loading null dates, opening saved
  pro formas and selecting a future stay without a payout.
- Enter a decimal through the real amount input and submit normally; `1234.56`
  becomes `123456` cents and renders correctly. Retain PMS 26 precision checks.
- Guest and company stay separate; optional identifiers/price details survive
  edits. Both document types are distinguishable in the list and downloadable.
- Enter `6429185079` through the Booking.com reservation number input on a
  pro forma with no payout. Verify `booking_reference` in the request, saved
  response, reopened input and PDF. A blank/whitespace-only value prevents
  creation in both the UI and API; an edited value survives save and reload.
- Delayed old property/type/year responses cannot replace the current preview.
- Test Bratislava midnight, DST and January 1 date round trips. No unintended
  previous-day date or previous-year pro forma number appears.
- Cancel the stay after issuance and verify the stored pro forma remains
  readable/downloadable without creating a cancellation charge or final invoice.

### 9.3 Implementation checks

Run from `backend/` after implementation:

- `go test ./internal/store ./internal/api ./internal/invoicepdf ./internal/migrate ./internal/pms21cleanup`
- `go test ./...`
- `go build ./...`

Run from `frontend/` after implementation:

- `npm run types:openapi`
- `npm test -- src/views/InvoicesView.spec.ts src/views/DashboardView.spec.ts src/views/BookingPayoutsView.spec.ts`
- `npm run type-check`
- `npm test`
- `npm run build`

Include any new co-located tests in the appropriate focused run. Record actual
results and pre-existing failures. Use synthetic data and temporary storage,
not a production invoice or payment, for tests and browser/PDF verification.

## 10. Implementation Notes

Implemented in the existing invoices aggregate with migration `000045_invoice_proforma`.
Backend focused and full Go tests, Go build, OpenAPI generation, frontend focused
and full tests, frontend type-check, and frontend build passed on 2026-10-21.
No production data, invoice, payment, or external customer workflow was used.
Real-browser submission and visual SK/EN PDF inspection were not run in this
environment.

## 11. Completion and owner communication

The feature is complete when the approved non-VAT Booking.com pro forma can
be issued before arrival, accurately communicates its purpose, preserves the
existing document patterns, coexists with the later final invoice, and passes
the verification above without posting a second sale or requesting payment
outside Booking.com.

Suggested reply to the guest, independent of implementation:

> Hello, thank you for your message. I can provide a pro forma showing your
> company details and the accommodation cost for reservation 6429185079.
> Payment for this reservation is handled by Booking.com under the booking's
> payment terms, so the pro forma will not request a separate payment to me.
> The final accommodation invoice will be issued after the stay.
>
> Please send the company's legal name, billing address, registration number
> and VAT number if applicable, together with the staying guest's full name.
> Could you also confirm whether the company needs the document for internal
> approval/reimbursement or intends to pay Booking.com directly?

That customer clarification is needed to meet their administrative purpose,
not to authorize a new PMS payment flow. Actual company details, dates and
price are issuance-time inputs. The feature is available through the existing
Invoices module after migration 000045 is applied.
