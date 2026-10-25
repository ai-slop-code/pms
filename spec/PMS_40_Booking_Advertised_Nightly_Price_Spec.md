# PMS 40 — Booking.com advertised nightly price

## 1. Status and objective

- **Date:** 2026-10-25.
- **Status:** Ready for implementation; this document does not implement the feature.
- **Audience:** AI developer agent, reviewer, and QA.
- **Authority:** Product decisions below record the user's request and clarification answers. Technical contracts specify how to implement those decisions in the inspected repository.

Allow an operator to record an optional **advertised nightly price before Booking.com promotions/discounts**, excluding extras, on a Booking.com named stay. This is a reference price, not income. It will support a future comparison of the price set versus the amount actually received.

Implement capture, persistence, editing, clearing, and display now. Analytics comparisons are deferred. Follow `spec/PMS_13_Coding_Conventions.md`.

## 2. Confirmed product decisions

| ID | Decision |
|---|---|
| D01 | The value is the operator's advertised **per-night** price before Booking.com promotions/discounts. It excludes additional fees, taxes, and other extras. |
| D02 | Currency is **EUR only**. Input supports up to two decimal places. An entered amount must be greater than zero; blank means not recorded. |
| D03 | The field is optional in **Create named stay**, **Promote raw block to named stay**, and **Edit named stay**, and available only for `booking_com` stays. |
| D04 | Existing stays may receive a price later. Stays may remain without one indefinitely. |
| D05 | Users can replace a saved price or clear it. Keep only the latest saved value, not price history. |
| D06 | Show the saved price in Edit named stay and in the calendar's night-detail dialog. |
| D07 | Changing check-in/check-out preserves the saved price until the user changes it. |
| D08 | Changing the stay type away from Booking.com clears the price. |
| D09 | Cancellation, no-show, and archival preserve the price. |
| D10 | Payout/reservation imports must never populate or overwrite this value. Finance reset preserves it. |
| D11 | Actual Booking.com financial amounts continue to come from payout/reservation files. This field is tracked separately and does not affect current financial or Analytics calculations. |
| D12 | Implement the future Analytics comparison later; retain an unambiguous stay-linked nightly amount now. |

Example: for three nights advertised at €100 per night, the user enters **100.00**, stored as **10,000 cents**, not 30,000 cents. There is one current nightly-price value per stay. This feature does not introduce a nightly rate schedule or compute an average from individual dates.

## 3. Verified current implementation and gap

The following observations are based on application code, rather than historical specification status labels. In particular, late-checkout code is already present in the inspected implementation.

| Area | Current implementation | Required integration |
|---|---|---|
| Stay forms and detail | `frontend/src/views/OccupancyView.vue` owns separate create, promote, and edit state/submissions. Its `dayNamedStays` section renders the night-detail dialog. | Add the optional price in these three forms and the named-stay detail section. |
| Canonical persistence | `backend/internal/store/named_stays.go`: `NamedStay`, create/update inputs, `CreateNamedStayRecord`, `PromoteRawBookingBlockToNamedStay`, `UpdateNamedStayRecord`, `namedStaySelectSQL`, `scanNamedStays`, and lifecycle methods. | Add dedicated nullable price fields and enforce type/clear/preserve rules transactionally. Promotion delegates to the same create path. |
| Mutation handlers | `backend/internal/api/occupancy_named_stay_handlers.go`: `stayCreateBody`, `stayPatchBody`, create/promote/patch handlers, and `writeNamedStayError`. | Decode and validate the new field, including omitted-versus-null PATCH behavior. |
| Read model | `backend/internal/store/stay_calendar.go`: `CalendarNamedStay` and `ListCalendarNamedStays` use their own explicit SELECT and scan. | Carry the saved value through this projection; updating only `NamedStay` is insufficient. |
| Read handlers | `backend/internal/api/occupancy_calendar_v2_handlers.go` exposes the combined calendar and the month-filtered stay list using the calendar read model. | Both must expose the same nullable price. |
| Frontend contracts | `frontend/src/api/types/occupancy.ts`, `spec/openapi.yaml`, and generated `frontend/src/api/types/generated.ts`. | Extend the handwritten calendar type and OpenAPI create/patch/read schemas, then regenerate types. |
| Existing manual revenue | `named_stays.manual_revenue_cents`, currency, and note exist; `backend/internal/store/analytics.go` consumes manual revenue for external stays. | Use separate fields. Reusing manual revenue would give this reference price the wrong financial semantics. |
| Imported finance | `backend/internal/store/finance_booking_payouts.go`, `finance_bookings_merge.go`, and associated import paths operate on linked financial evidence. | Keep imported amounts independent; exercise preservation through real import flows. |
| Finance reset | `backend/internal/store/finance_reset.go` deletes finance-owned data while retaining named stays. | A price stored directly on the stay survives naturally; verify this with a regression test. |
| Money UI | `frontend/src/components/ui/UiInput.vue` supports string input, decimal input mode, help, and errors. `frontend/src/utils/format.ts` supplies `formatEuros`. `frontend/src/views/invoices/items.ts` demonstrates decimal-string-to-cents conversion. | Reuse UI/formatting conventions and exact decimal conversion, without coupling stay input to invoice-specific rules. |

The current mutation response contains save/integration status and the named-stay ID, rather than the full stay. The frontend reloads the calendar after saving. Preserve that response shape and make the reloaded read model authoritative for the displayed price.

## 4. Domain and persistence design

### 4.1 Ownership and schema

Persist on `named_stays`, owned by the existing named-stay ID and property. Use these names consistently:

| Column | Type | Meaning |
|---|---|---|
| `booking_advertised_nightly_price_cents` | Nullable `INTEGER` | Latest manually entered advertised nightly amount in EUR cents. |
| `booking_advertised_nightly_price_currency` | Nullable `TEXT` | `EUR` when a price exists; null when absent. |

Currency is stored separately in accordance with repository money conventions, but is fixed by the backend. There is no currency selector or currency write parameter.

Required invariants:

- Both columns are null when no price exists, including for all pre-feature stays.
- A present amount is a positive integer, its currency is exactly `EUR`, and its stay type is `booking_com`.
- No zero default or sentinel. Missing price and zero are different; zero is invalid.
- Protect exact JSON/TypeScript integer round-tripping: maximum accepted cents is `9,007,199,254,740,991` (`Number.MAX_SAFE_INTEGER`). This is a representation limit, not a business pricing cap. Check the limit before converting a large decimal string to a JavaScript number.
- Add database CHECK constraints for amount/type/currency consistency and integer/range validity. Be explicit about null handling: SQLite CHECK expressions evaluating to NULL do not reject a row.
- Add a new up/down migration using the next available migration number at implementation time. Do not modify historical migrations or backfill from payouts, reservations, manual revenue, invoice amounts, or raw blocks.
- A down migration removes only this feature's fields/constraints and preserves other stay data.

Use the repository's nullable SQL types and explicit select/scan mapping. No new price-history table, finance row, per-night row, or independent entity is needed.

### 4.2 Save and lifecycle rules

| Operation | Price result |
|---|---|
| Create/promote Booking.com stay without price | Null. |
| Create/promote Booking.com stay with valid price | Save amount and `EUR` on the new stay. |
| Add or replace price on Booking.com stay | Save latest supplied amount and `EUR`. |
| Clear price | Set amount and currency to null. |
| Edit unrelated fields, omitting price | Preserve it, unless the final stay type is no longer Booking.com. |
| Change check-in/check-out | Preserve the nightly price; no multiplication, division, or recalculation. |
| Change Booking.com to External, Personal use, or Maintenance | Clear both fields in the same transaction as the type change. |
| Later change back to Booking.com | Remain null unless a new price is explicitly supplied. |
| Cancel, archive, mark no-show/non-refundable cancellation, clear outcome, or reactivate | Preserve the value. |
| Payout/reservation import, reimport, matching, or Finance reset | Preserve it; do not populate an absent value. |
| Raw Booking.com ICS synchronization/source-link changes | Preserve it; raw source data does not own the price. |

Apply validation and final-state rules in canonical store methods, not only in the browser. Retain normal property scoping, stay validation, audit actions, and transaction boundaries. Validation failures must leave the entire stay mutation unapplied.

Keep the new field independent of late checkout. Current status/outcome paths clear late-checkout selections; they must not clear this advertised price.

## 5. API contract

### 5.1 Existing endpoints

Extend existing endpoints; add no price-only endpoint:

- `POST /api/properties/{id}/stays`
- `POST /api/properties/{id}/booking-blocks/{blockId}/promote`
- `PATCH /api/properties/{id}/stays/{stayId}`
- Existing month-filtered stay-list and combined occupancy-calendar GET responses.

Reuse Occupancy module write permission for mutations and read permission for display. This is stay metadata, not a new Finance permission or role.

### 5.2 Request semantics

The sole new writable field is `booking_advertised_nightly_price_cents`: optional nullable integer, minimum 1, maximum as specified in §4.1.

| Request value | Create/promote | PATCH |
|---|---|---|
| Key omitted | No price | Preserve existing price, subject to final stay type. |
| `null` | No price | Explicitly clear price. |
| Positive integer in range | Set price if final type is Booking.com | Set/replace price if final type is Booking.com. |
| Zero, negative, fractional cents, string, boolean, object, array, or out-of-range number | HTTP 400 | HTTP 400. |

An explicit non-null price for a final non-Booking.com stay type is HTTP 400. A type change away from Booking.com with the price omitted or null succeeds and clears the old stored value. This distinguishes an ordinary type change from an inconsistent request explicitly assigning a Booking.com price to another stay type.

Promotion's existing default stay type remains Booking.com when the handler accepts an omitted type. Evaluate price eligibility against the resolved final type.

For PATCH, a plain `*int64` JSON field cannot distinguish missing from explicit null. Use presence-aware decoding, such as `json.RawMessage` with length checked for omission and explicit decoding for null/integer, or an equivalent repository-compatible optional nullable type. Carry that distinction into the store input using a presence flag plus nullable value. Do not implement clearing by treating omitted input as null.

Use specific validation messages and map the new domain validation error to HTTP 400 in `writeNamedStayError`, rather than returning the generic invalid-date-range error. Preserve existing HTTP 403/404/conflict behavior.

Examples for a Booking.com stay:

- Set €99.95/night: `{"booking_advertised_nightly_price_cents":9995}`.
- Clear: `{"booking_advertised_nightly_price_cents":null}`.
- Rename without changing the price: `{"display_name":"Updated guest name"}`.
- Change type and automatically clear: `{"stay_type":"external"}`.

### 5.3 Read contract

Add both price and currency to `CalendarNamedStay` and the OpenAPI `NamedStay` schema. Return both keys consistently:

- Present: `booking_advertised_nightly_price_cents: 9995`, `booking_advertised_nightly_price_currency: "EUR"`.
- Absent: both null.

In TypeScript, use `number | null` and `'EUR' | null`. Mark these read keys required and nullable in OpenAPI; keep the amount optional and nullable in create/patch requests. Explain nightly units, pre-discount meaning, fixed currency, and null/PATCH semantics in schema descriptions.

Update the canonical store read path and the separate calendar SQL projection. Existing archived-stay visibility filtering remains the current calendar behavior; preservation does not require an archive browser in this feature.

## 6. Frontend behavior

### 6.1 Three stay forms

Use the same label, help, validation, and conversion for create, promote, and edit:

- **Label:** `Booking.com advertised nightly price (EUR)`.
- **Help:** `Optional. Your nightly price before Booking.com promotions or discounts, excluding extras. This is not income.`
- Place it consistently after the stay dates and before the existing cleaning/late-checkout controls.
- Render it only when the selected stay type is `booking_com`.
- Start blank on each new create/promote dialog opening. Do not carry an earlier stay's value into another form.
- Edit initializes from saved cents, rendered as a two-decimal EUR value; null initializes as an empty string.
- Keep unsaved input as a string. Trim surrounding whitespace; empty/whitespace-only means null.
- Use `UiInput` with decimal input mode and exact decimal-string parsing. Support plain whole amounts and one/two decimal digits using `.` as the decimal separator, consistent with current monetary entry parsing. Reject invalid syntax or excess precision rather than silently rounding.
- Convert by separating whole euros and cents, not by truncating floating-point multiplication. Use a shared helper for the three workflows; do not duplicate parsing or reuse invoice-specific quantity/limit logic.
- Zero and negative values show a clear error. For example: `Enter a price greater than zero with up to 2 decimal places, or leave it blank.`
- Submit an integer number of cents or null. Clearing an existing input must send null, not omit the key.
- When a non-Booking.com type is selected, hide/reset the price draft and submit no non-null price. The server performs the persisted clearing on a successful save.
- Cancelling the dialog or failing validation does not clear or otherwise modify persisted data.
- After a successful save, use the existing calendar reload so editing and details show the persisted result. Preserve existing saved-versus-integration-error response handling.

Frontend validation must run in the submit functions: the dialog footer buttons already call those functions directly, so HTML input validity alone is insufficient.

### 6.2 Night-detail display

In the existing `Named stays` section of `Calendar details for …`, show for each Booking.com stay:

- **Booking.com advertised nightly price:** formatted EUR amount followed by `/ night`.
- For an absent price, show the existing empty-value marker `—`, never €0.00.

Use `formatEuros` and the existing note/detail layout. Show no price row for other stay types. The amount belongs to the entire named stay as its saved nightly reference, so opening another night of the same stay displays the same value. Do not add the price to raw blocks or introduce calendar-cell badges for this feature.

## 7. Financial boundaries and future Analytics readiness

The persisted value is operator-owned reference data. It must not:

- Populate or modify `manual_revenue_cents` or imported booking amount/commission/fee/net fields.
- Create transactions, affect cashflow, recognized income/net, payout reconciliation, invoices, or cleaning salary.
- Act as finance evidence, mark an unmatched stay matched, or change sold-night eligibility.
- Change current Analytics revenue, ADR, RevPAR, cancellation reporting, or Dashboard financial figures.
- Be automatically set from imported amounts, divided from an imported stay total, or inferred from ICS evidence.

Future Analytics can join the stay-linked reference price to existing financial evidence using named-stay identity. The explicit nightly unit, EUR currency, and nullability supply the required foundation. Missing values remain missing; no zero fallback is introduced.

No comparison formulas, charts, filters, expected totals, discount calculations, per-night distribution, or historical snapshots are implemented now. In particular, preserving a price across date changes does not capture past date ranges; the user requested only the latest price on the existing stay.

Do not add the price to Nuki change detection or cleaning-calendar payloads. Existing save-side reconciliation behavior may continue; this field introduces no new integration action. Finance reset needs no new preservation category if its current stay-preserving implementation already satisfies the requirement.

## 8. Implementation sequence for the AI developer agent

1. **Recheck integration points.** Inspect current files listed in §3 and the latest migrations. Preserve intervening work, especially stay lifecycle and late-checkout behavior. Treat this document's confirmed decisions as authoritative for this price field.
2. **Define storage and wire contracts.** Add the next migration pair and update the OpenAPI read/create/patch schemas, with explicit null semantics and fixed EUR metadata.
3. **Implement canonical persistence.** Extend model/input types, creation and promotion, update logic, SQL selects/scans, validation, and transactional type-change clearing. Leave existing rows null.
4. **Implement API/read propagation.** Add presence-aware request decoding, store mapping, HTTP 400 handling, and calendar projection fields. Keep existing permission and mutation-response contracts.
5. **Implement UI.** Update handwritten types, regenerate OpenAPI types, add shared decimal conversion, the three form inputs, and night-detail display. Ensure blank create, later add, edit, and explicit clear all round-trip.
6. **Verify ownership boundaries.** Exercise lifecycle, real import/reimport, ICS, reset, and representative financial/Analytics calculations. Fix only changes needed to satisfy this feature; do not substitute the reference price into existing calculations.
7. **Run checks and report.** Record actual commands/results and any blockers. Do not claim implementation or verification from this specification alone.

## 9. Acceptance criteria and verification

Use existing store/API/frontend test infrastructure and temporary databases. Focus tests on persisted semantics and cross-module boundaries, not snapshots mirroring field declarations.

### 9.1 Persistence, API, and lifecycle

1. Migration of a pre-feature database preserves existing stays and initializes both fields to null. Up/down behavior preserves unrelated data.
2. Both create and promote persist a supplied €99.95/night as 9995 cents with EUR, and succeed with no amount supplied.
3. An existing Booking.com stay can receive a price later, replace it, and clear it. Reloading proves each result.
4. PATCH omission preserves a value; explicit null clears it. Include a rename-only patch as a regression case.
5. Zero, negative, fractional cents, wrong JSON types, and values beyond the representation limit fail with HTTP 400 without partial writes. Database constraints reject inconsistent direct writes.
6. Non-Booking.com creation/update cannot store a price. Changing a priced stay to each other type with omitted/null price succeeds and clears both columns; returning to Booking.com does not restore the old value.
7. Date edits preserve the nightly amount. A three-night stay with a 10000-cent price remains 10000 cents when changed to four nights.
8. Cancellation, non-refundable cancellation, no-show, archival, reactivation, and outcome clearing preserve the value. Test archival at the store level because the calendar currently filters archived stays.
9. Combined calendar and stay-list responses return the saved price/currency or explicit nulls. Different-property access cannot read or update another property's price; existing read/write permission checks remain enforced.
10. A failed stay save, including overlap/range validation failure, does not change the saved price or leave a partial create/promotion.

### 9.2 Ownership and reporting regressions

11. Import/reimport payout and reservation evidence for a priced stay without changing the reference price. Import for an unpriced stay leaves it null. Use actual repository ingestion paths, not merely a direct insert into `finance_bookings`.
12. Finance reset removes its normal finance-owned records and leaves the stay's amount/currency intact; later editing/clearing still works.
13. Raw ICS synchronization/source reconciliation leaves a manually entered price intact.
14. Adding/changing/clearing only this reference price leaves representative Finance, Dashboard, and Analytics financial results and `has_finance_evidence` unchanged. A Booking.com stay with only this price has no new imported income or generated financial entry because of it.
15. On a stay with existing late-checkout charges, a price-only update does not alter or duplicate those charges. A lifecycle change retains the advertised price while existing late-checkout lifecycle rules continue normally.

### 9.3 Frontend

16. Each of the three workflows shows the input only for Booking.com and submits the correct cents/null payload. New dialogs start blank; editing initializes from the persisted value.
17. Blank/whitespace input clears or omits the value as specified. Values such as `100`, `100.5`, `99.95`, and `0.01` convert exactly; `0`, `0.00`, negative, malformed, excess-precision, and oversized input block submission.
18. Type switching and cancelled dialogs cannot accidentally persist a clearing. Saving a non-Booking.com type clears through the normal API contract.
19. Night details show a EUR per-night value for priced Booking.com stays, `—` for unpriced Booking.com stays, and no price row for other types.
20. Reload/reopen after save reflects additions, corrections, and clearing; no stale draft leaks across stays.

### 9.4 Relevant checks

Update or extend `backend/internal/store/named_stays_test.go`, `stay_calendar_test.go`, `backend/internal/api/occupancy_named_stay_handlers_test.go`, and `frontend/src/views/OccupancyView.spec.ts`. Add focused migration, decimal-parser, and import/reset regression coverage using the repository's existing test helpers.

Run commands from the stated directories:

| Directory | Command | Purpose |
|---|---|---|
| `frontend/` | `npm run types:openapi` | Regenerate API types after contract changes. |
| `backend/` | `go test ./...` | Backend, migration, integration, and regression checks. |
| `frontend/` | `npm test` | Frontend behavior and existing regressions. |
| `frontend/` | `npm run type-check` | Verify handwritten/generated contracts and all consumers. |
| `frontend/` | `npm run lint` | Existing frontend lint checks. |
| `frontend/` | `npm run build` | Production bundle check. |

Format touched files using existing tooling and inspect the final diff. Completion means the confirmed behavior and boundary checks above pass; the future Analytics comparison remains a separate feature.
