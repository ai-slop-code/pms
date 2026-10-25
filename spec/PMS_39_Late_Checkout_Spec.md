# PMS 39 — Late checkout

## 1. Status and objective

- **Date:** 2026-10-25.
- **Status:** Specification and implementation plan only; application implementation has not been requested.
- **Audience:** AI developer agent, reviewer, and QA.
- **Authority:** The product requirements below record the user's answers to two clarification rounds. The implementation sections describe how to deliver those requirements using the inspected repository.

Add a late-checkout checkbox to named-stay creation, promotion, and editing. Saving an eligible stay with late checkout selected creates a **€30 incoming transaction** and, when cleaning is required, a **€10 outgoing cleaner bonus**. Both belong to the checkout date. Include the charge in recognized incoming and the bonus in cleaner salary, and use a configurable property-level late-checkout time for Nuki expiry.

This document contains the feature specification, implementation sequence, and acceptance criteria. Follow `PMS_13_Coding_Conventions.md` during implementation.

## 2. Confirmed product decisions

| ID | Confirmed decision |
|---|---|
| D01 | Add the checkbox beside the existing cleaning checkbox in **Create named stay**, **Promote raw block to named stay**, and **Edit named stay**. |
| D02 | The default is unchecked. Financial entries are created on successful stay save, not when the unsaved checkbox is clicked. |
| D03 | Late checkout is available only for **Booking.com** and **External** stays. |
| D04 | Each late-checkout stay creates one **€30 incoming** transaction, categorized as **Late checkout**. |
| D05 | If **Cleaning required** is checked, also create one **€10 outgoing** transaction in the existing **Cleaning Salary** category. Otherwise create only the €30 incoming entry. |
| D06 | Both transactions use the stay's **checkout date**, including when the stay crosses a month boundary. |
| D07 | Unchecking late checkout removes its generated transactions. Changing checkout date moves the applicable transactions to the new date. |
| D08 | Cancellation or no-show removes the late-checkout transactions and clears the checkbox. Restoring the stay does not restore the selection or transactions; the user must select it again. |
| D09 | Include the €10 bonus in **Cleaning log & salary** and the cleaner-expense reporting discussed in clarification. Count it once in expenses. |
| D10 | Recognize the entire €30 in the **checkout month**, as well as including it in Monthly CashFlow. Do not allocate it across stay nights. |
| D11 | Nuki expiry for late-checkout stays uses a **per-property late-checkout time**, default **13:00**, interpreted in the property's timezone. |
| D12 | Update an existing Nuki code immediately when late checkout is enabled/disabled or the relevant checkout date changes. Disabling restores normal checkout expiry. Changing the property setting must immediately update affected existing codes for upcoming late-checkout stays. |
| D13 | The cleaning-calendar event needs no late-checkout-specific change. |
| D14 | Finance reset preserves the late-checkout charges and cleaner bonuses. |

Amounts are the specified €30 and €10. The configurable value requested for this feature is the Nuki late-checkout time.

### Relationship to existing specifications

- Use the named-stay ownership model from PMS 21, not legacy occupancies or raw booking blocks as the financial owner.
- Extend PMS 38's recognized-incoming formula with this explicitly confirmed checkout-month charge. Other incoming transactions do not become recognized revenue merely because they are incoming.
- Extend PMS 38's Cleaner expense calculation to include these separate cleaner bonuses once.
- Extend PMS 16's Finance reset preservation rules to include both late-checkout transaction types.
- Use PMS 23/34/35's existing Nuki creation, maintenance, and pending-operation behavior: stay saves maintain existing credentials; initial code creation remains the explicit Nuki Access workflow.
- Continue using the existing cleaning-calendar contract. The new late-checkout time is a separate setting from the ordinary checkout time.

## 3. Verified repository integration points

These are implementation entry points, not a claim that the new behavior exists already. The working tree contains ongoing Finance changes; inspect the current versions before implementation.

| Area | Files and current behavior |
|---|---|
| Stay dialogs | `frontend/src/views/OccupancyView.vue` contains separate create, promote, and edit forms and their request bodies. Each already has `cleaning_required`. |
| Stay API | `backend/internal/api/occupancy_named_stay_handlers.go` contains create/promote/update/status/outcome handlers, request DTOs, and the Nuki change detector. |
| Stay persistence | `backend/internal/store/named_stays.go` owns named stays and their lifecycle mutations. Create/update/status writes already use database transactions; outcome updates currently need integration with transactional financial changes. |
| Stay read models | `backend/internal/store/stay_calendar.go` and `frontend/src/api/types/occupancy.ts` carry the stay data used by the dialogs. |
| Ledger | `backend/internal/store/finance.go` provides transaction storage, read-only generated entries, generated-month sync, and Finance summary. Existing generated uniqueness does not cover the proposed late-checkout sources. |
| Revenue | `backend/internal/store/finance_revenue_recognition.go` allocates payout-backed booking revenue over nights. `backend/internal/api/finance_handlers.go` and `server.go` expose Finance and Dashboard figures. |
| Cleaning salary | `backend/internal/store/cleaning.go` computes salary from counted cleaning logs plus manual adjustments. Monthly Finance sync currently posts `FinalSalaryCents` as one `cleaning_salary` transaction. |
| Finance reset | `backend/internal/store/finance_reset.go` currently deletes transactions other than `cleaning_salary`. It has its own transactional salary calculation and salary-regeneration path. |
| Property settings | `backend/internal/store/store.go` owns `PropertyProfile`; `backend/internal/api/server.go` and `server_response_types.go` expose settings. `frontend/src/views/PropertyDetailView.vue` edits ordinary checkout time. |
| Nuki validity | `backend/internal/nuki/service.go` uses property defaults in generation and `MaintainNamedStay`, with `namedStayWindow` constructing property-local times and converting to UTC. `backend/internal/store/nuki.go` supplies Nuki stay projections. |
| Cleaning UI | `frontend/src/views/CleaningView.vue`, its `cleaning/` components, `backend/internal/api/cleaning_handlers.go`, and `frontend/src/api/types/cleaning.ts` expose salary totals and supporting data. |
| Finance UI/contracts | `frontend/src/views/FinanceView.vue`, `frontend/src/views/finance/`, Dashboard widgets, `frontend/src/api/types/finance.ts`, `dashboard.ts`, and `spec/openapi.yaml`. |

## 4. Stay and ledger behavior

### 4.1 Form behavior

Use the label **Late checkout** and the existing checkbox presentation beside **Cleaning required** in all three dialogs.

- New create/promote forms initialize it to false each time they open.
- Edit loads the persisted selection, including for an existing stay created before this feature.
- Only Booking.com and External selections can enable it. The client must not submit an enabled selection for Personal use or Maintenance; the backend must enforce the same eligibility.
- Saving is the trigger. Closing/cancelling an unsaved dialog or failing stay validation creates no financial records.
- Reflect the selected cleaning state in the result; late checkout does not force cleaning to be checked.

### 4.2 Persisted state and amounts

Persist a named-stay boolean, proposed name `late_checkout`, default false for existing and new records. Use integer cents: **3000** incoming and **1000** outgoing.

| Saved eligible stay state | Incoming late-checkout entry | Outgoing late-checkout bonus |
|---|---:|---:|
| Late checkout off | None | None |
| Late checkout on; cleaning off | €30 | None |
| Late checkout on; cleaning on | €30 | €10 |

Each entry belongs to the same property and named stay. The bonus is triggered by the saved flags; it does not wait for a cleaner's Nuki entry or a later Finance month-sync action.

### 4.3 Subsequent changes

- Late checkout off → on: create the applicable entries once.
- Late checkout on → off: delete its incoming entry and any bonus entry.
- Cleaning on → off while late checkout remains on: delete only the bonus.
- Cleaning off → on while late checkout remains on: add the bonus once.
- Checkout date changes: move both applicable entries, and their reporting/salary contribution, to the new checkout date/month. Refresh the old and new affected monthly salary results.
- Cancellation, `cancelled_non_refundable`, or `no_show`: set `late_checkout = false` and delete both entries in the same database transaction as the lifecycle change.
- Reactivation or clearing the outcome: leave late checkout false until explicitly selected again.
- Saving a change to Personal use or Maintenance cannot retain an enabled late-checkout selection or its generated financial entries, consistent with D03.
- Saving an unchanged eligible selection, retrying downstream synchronization, or repeating reconciliation must not duplicate entries.

Apply these rules in the canonical persistence paths, not only in the browser. Inspect other callers that actually mutate named-stay cancellation/outcome/date/type and route them through the same reconciliation. Do not infer cancellation from the disappearance of raw booking evidence.

### 4.4 Dates

The accounting date is exactly the named stay's checkout calendar date. It is neither the date the checkbox is selected nor the date the guest pays.

For a stay **30 October–2 November**, both entries are dated **2 November**, and all late-checkout revenue/bonus contribution belongs to November. Moving checkout to 31 October moves that contribution to October.

Use the ledger's existing timestamp representation while preserving that calendar-date identity. Current Finance queries frequently select by the stored `transaction_date` date/month prefix. Avoid a property-local-midnight-to-UTC conversion that accidentally assigns a first-of-month checkout to the preceding month. This accounting-date concern is separate from the actual Nuki expiry instant.

### 4.5 Financial consistency

Save the stay flag, generated ledger entries, and any persisted salary result changed by the operation atomically. If a required ledger write fails, the stay mutation fails too; do not report a successful save with missing financial effects.

External Nuki/Google calls run after the database commit. Their failure must not undo or duplicate the successfully saved financial records. Use the existing saved-versus-integration-failed response patterns, with accurate user-visible status.

Generated entries follow existing read-only ledger behavior. The stay checkbox, checkout date, and cleaning flag are their controls. Use stable source identifiers, not notes, category titles, guest names, amounts, or dates, to identify the entries for updates/deletion.

## 5. Cleaning salary and Finance reporting

### 5.1 Separate bonus entry without double counting

The user requested a separate outgoing €10 ledger entry and inclusion in Cleaning log & salary. The monthly cleaning-salary transaction must not include that same €10 again.

Implement the following decomposition:

```text
Base salary                 = existing counted-cleaning-day calculation
Manual salary adjustments   = existing manual adjustments
Late-checkout bonuses       = sum of the month's generated late-checkout bonus entries
Final cleaner salary        = Base salary + Manual salary adjustments + Late-checkout bonuses

Ordinary monthly salary row = existing payable Base salary + Manual salary adjustments
Separate bonus rows         = one €10 row per qualifying late-checkout stay
```

Preserve the existing treatment of nonpositive ordinary salary amounts. Adding a bonus must not turn the whole `FinalSalaryCents` value into the ordinary monthly salary row.

Engineering approach: expose a separate `late_checkout_bonus_cents` summary value and supporting stay/date/amount rows, sourced from the generated bonus ledger entries. Keep existing manual adjustments distinguishable from generated bonuses. Show the bonus contribution and the resulting final salary in **Cleaning log & salary**, using its existing UI components and currency formatting.

Do not create synthetic cleaner-entry logs, increment counted cleaning days, or add a second manual salary adjustment for the same bonus. A bonus-only month must still show €10 salary with zero counted cleaning days.

Update both the normal summary/sync path and `computeCleaningMonthlySummaryTx` / `financeResetUpsertCleaningSalary`. After any number of month syncs or Finance resets, a €100 ordinary salary plus a €10 bonus must produce €110 total expense, not €120.

### 5.2 Monthly CashFlow and Cleaner expense

Use ordinary transaction-direction accounting for these non-payout ledger entries:

- Monthly incoming: +€30 per selected eligible stay in its checkout month.
- Monthly outgoing: +€10 when cleaning is required.
- Monthly net: +€20 with cleaning, +€30 without cleaning.
- Cleaner expense: existing generated monthly salary expense **plus the separate late-checkout bonus entries** for the month, once.
- Cleaner margin: use the updated Cleaner expense numerator and the existing Property income denominator and zero-income behavior.
- Lifetime totals and category breakdown reflect the same entries once.

The PMS 38 **Property income** metric remains payout-derived. Late-checkout charges are additional incoming transactions; their inclusion in recognized incoming is the explicit exception described below. Keep existing Long-term net expense eligibility, including the exclusion of the Cleaning Salary category, and let its recognized-net input use the updated result.

### 5.3 Recognized incoming

Let:

```text
A(M) = existing recognized payout-backed booking Net for month M
L(M) = generated late-checkout incoming amounts dated in month M
O(M) = existing eligible monthly outgoing, including separate late-checkout bonuses once

Recognized incoming(M) = A(M) + L(M)
Recognized net(M)      = A(M) + L(M) - O(M)
```

- Recognize all €30 on the checkout date, even when no stay night falls in the checkout month (for example, checkout on 1 November).
- Include the late-checkout charge for both supported stay types even when no payout-backed `finance_bookings` row exists.
- Recognize the dedicated generated source, not all incoming transactions or everything with a matching category name.
- Keep `recognized_booking_net_cents` booking-only. Expose a separate late-checkout component and combined recognized-incoming value rather than putting non-booking charges into a booking-named field.
- Do not add the €30 to imported payout Net or the nightly allocation. Do not then add it a second time through other incoming.
- Apply the same total to Finance Overview, Revenue, and Dashboard. Show enough component detail in Revenue to explain the late-checkout addition even without a payout-backed booking row.
- Update affected calculation help/footnotes so they describe the checkout-month addition and bonus expense accurately.

## 6. Nuki expiry and property configuration

### 6.1 Property setting

Add **Late checkout time** to the existing property Profile settings beside ordinary checkout time. Proposed API/storage name: `late_check_out_time`.

- Default: **13:00** for existing and new properties.
- Value: a valid 24-hour `HH:MM` time, using the existing time-input UI.
- Interpret it in the property's timezone on the actual checkout date, then convert the resulting Nuki instant to UTC.
- An omitted field in an unrelated settings update preserves the current value. Reject malformed explicit values with a validation error.
- Use the existing property-settings read/write permissions.

### 6.2 Effective validity

```text
late_checkout = false: expiry = checkout date + ordinary property checkout time
late_checkout = true:  expiry = checkout date + property late-checkout time
```

Use this same calculation in initial explicit Nuki generation and maintenance of existing codes. The cleaning flag does not affect late-checkout expiry.

After a stay save that changes late checkout or checkout date, immediately request maintenance of its existing code. After saving a changed property late-checkout time, immediately maintain the affected existing codes for current/upcoming late-checkout stays in that property, including stays checking out today. Reuse existing Nuki eligibility and expired/revoked-credential handling.

“Immediate” means dispatch the provider update as part of the save workflow after persistence, rather than waiting for a scheduled sync. Provider confirmation may be pending under the existing managed-operation lifecycle. Report pending/error state accurately and preserve the existing retry/reconciliation mechanism.

Maintain credential identity and PIN. If no code exists, saving the flag does not create one; subsequent explicit generation uses the configured late-checkout time. On cancellation/no-show, existing code revocation takes precedence over restoring normal expiry. Changing the late-checkout setting must not update unrelated properties or ordinary-checkout stays.

Include the late-checkout flag in the named-stay Nuki change detector and in every stay projection used to calculate validity. Ensure pending operations cannot later restore a superseded expiry.

The new time does not reschedule cleaning-calendar events. Existing date/cleaning/status changes continue through the current calendar reconciliation.

## 7. Finance reset

Preserve both generated late-checkout charges and generated cleaner bonuses, together with their named-stay linkage. The saved late-checkout selection remains intact.

Update the reset deletion predicate, preview/result counts, attachment cleanup selection where applicable, UI explanation, and OpenAPI/types together. The preview must accurately describe what the reset will preserve.

The reset's salary recomputation must include bonuses in final cleaner salary while excluding them from the regenerated ordinary monthly salary transaction. Include bonus-only months in the relevant salary recomputation. Preserved charges remain recognizable by checkout month after payout/import data is deleted.

Preservation does not mark an unsynced Finance month as fully synced. Keep month-sync state semantics accurate. A repeated reset or subsequent generated-entry sync must not duplicate preserved entries. Later unchecking/cancelling the stay must still remove its preserved late-checkout entries normally.

## 8. Engineering design and API integration

The following are internal implementation choices for the confirmed behavior; they do not add product options.

### 8.1 Storage and reconciliation

Prefer the existing ledger with two dedicated generated source types:

| Proposed `source_type` | Direction | Amount | Category | Stable reference |
|---|---|---:|---|---|
| `late_checkout` | incoming | 3000 cents | new `late_checkout` / Late checkout | named-stay ID |
| `late_checkout_cleaner_bonus` | outgoing | 1000 cents | existing `cleaning_salary` | named-stay ID |

Use `is_auto_generated = true`. Category is presentation/accounting classification; source type identifies lifecycle ownership. In particular, the bonus must not use monthly `cleaning_salary` source identity, whose reference is a month.

- Add database-enforced uniqueness on property + generated source type + named-stay reference for these sources. Do not include transaction date in that identity: changing the date must move the row, not permit a duplicate.
- Reuse property-aware category resolution and seed the new incoming category through a new migration.
- Implement one transaction-aware reconciliation helper that creates, updates, or deletes the owned entries from the final saved stay state.
- Read old and new checkout months when updating dependent salary results.
- Preserve unrelated manual transactions, including historical manually entered late-checkout payments. Existing stays default false; this feature does not infer previous selections from old ledger notes.
- Choose the next available migration number when implementing and add up/down migrations; do not edit earlier migrations.

### 8.2 Existing endpoints

Extend existing endpoints rather than introducing a separate booking or payment workflow:

- Create/promote bodies accept `late_checkout`, default false when omitted.
- Stay PATCH accepts an optional `late_checkout`; omission preserves it except for explicit lifecycle/eligibility transitions described in section 4.
- Stay/calendar read DTOs expose the persisted boolean so Edit can round-trip it.
- Property settings expose and update `late_check_out_time` with the default and validation above.
- Cleaning summary/detail responses expose the generated bonus contribution.
- Finance recognition exposes booking-only recognition, late-checkout recognition, and their combined incoming total. Dashboard uses that same combined calculation.
- Finance reset responses expose accurate preservation counts.

Keep existing permissions for stay mutations, property settings, Finance reads/reset, and Cleaning reads. Generate financial effects server-side as part of the authorized stay mutation. Preserve property scoping and existing audit patterns, including lifecycle removals and property-setting changes.

Update `spec/openapi.yaml`, handwritten frontend contracts, backend DTOs, and generated frontend types together. Use consistent names across layers. Retain the saved-stay outcome when reporting an external integration failure so a UI retry does not create another stay.

## 9. Implementation plan for the AI developer agent

### Phase 1 — Contracts and migration

1. Inspect the current working tree and the integration points in section 3, particularly the ongoing Finance changes and the existing Nuki operation lifecycle.
2. Add named-stay `late_checkout` and property-profile `late_check_out_time` with the specified defaults.
3. Add the Late checkout category and generated-source uniqueness/linkage needed for idempotent entries.
4. Extend stay/profile read models and request contracts, including all explicit SQL scans and the calendar/Nuki projections.
5. Verify migration defaults on existing data and creation defaults on new properties/stays.

### Phase 2 — Transactional stay-to-ledger lifecycle

1. Implement the shared transaction-aware entry reconciler using 3000/1000 cents and stable stay identity.
2. Call it from create, promote, edit, cancellation, and outcome mutations. Make outcome changes transactional with flag clearing and entry deletion.
3. Handle cleaning toggles, date moves across months, and supported stay-type enforcement.
4. Inspect canonical mutation callers so alternate write paths cannot bypass the confirmed behavior.
5. Verify atomic rollback, property isolation, repeated saves, and removal/reselection behavior.

### Phase 3 — Cleaning, Finance, and reset

1. Add the separate bonus component to cleaning salary computation and expose supporting bonus rows.
2. Keep the monthly salary ledger writer limited to its ordinary salary component in both normal sync and reset regeneration.
3. Extend Cleaner expense and existing summary/category/lifetime reporting consistently.
4. Add checkout-month late-checkout recognition and the combined recognized-incoming result; wire Finance and Dashboard to the shared calculation.
5. Preserve both generated sources during Finance reset and update preview/counts/month handling.
6. Verify bonus-only months, cross-month moves, repeated sync/reset, and recognition with no imported payout.

### Phase 4 — Nuki configuration and immediate maintenance

1. Add property-profile validation and the persisted late-checkout setting.
2. Share the effective-expiry calculation between explicit generation and maintenance.
3. Trigger immediate existing-code maintenance from late-checkout changes and property-setting changes, using existing pending-operation handling.
4. Verify enabling, disabling, changed dates/settings, timezone/DST conversion, missing-code behavior, and failure/pending-state reporting with the existing fake Nuki client.

### Phase 5 — Frontend and API documentation

1. Add the checkbox and reset/load/save handling in all three dialogs, with supported-type eligibility.
2. Add the property time input and handle the settings-save result when Nuki updates are pending or fail.
3. Display bonuses in Cleaning log & salary and update affected Finance/Revenue/Dashboard values and calculation descriptions.
4. Update reset explanation/counts and regenerate frontend OpenAPI types.
5. Add focused frontend coverage for saved-state round-tripping, type/cleaning changes, the time setting, and updated reporting.

### Phase 6 — Verification and handoff

Run focused tests while implementing, then the repository checks appropriate to these cross-module changes:

| Working directory | Command |
|---|---|
| `backend/` | `go test ./internal/store ./internal/api ./internal/nuki ./internal/migrate` |
| `backend/` | `go test ./...` |
| `backend/` | `go build ./...` |
| `frontend/` | `npm run types:openapi` |
| `frontend/` | `npm test` |
| `frontend/` | `npm run type-check` |
| `frontend/` | `npm run lint` |
| `frontend/` | `npm run build` |

Review the documentation and generated contract diff. Manually exercise the three dialogs, property setting, ledger, Cleaning log & salary, Revenue, and reset preview. Record actual checks and any remaining verification gaps in the implementation handoff.

## 10. Acceptance criteria

| ID | Scenario | Required result |
|---|---|---|
| AC01 | Open create/promote | Late checkout is unchecked; no entries exist from opening or clicking the unsaved checkbox. |
| AC02 | Save either supported stay type with late checkout and cleaning | Exactly one €30 incoming and one €10 outgoing entry, both dated on checkout and linked to the stay. |
| AC03 | Save late checkout without cleaning | Exactly one €30 incoming entry; no bonus; late Nuki expiry still applies. |
| AC04 | Cancel a dialog or fail validation | No stay/ledger side effects from the attempted selection. |
| AC05 | Reopen Edit | Persisted late-checkout selection is loaded correctly. |
| AC06 | Uncheck and save | Both generated entries disappear and salary/revenue contributions are removed; existing Nuki expiry returns to ordinary checkout time. |
| AC07 | Toggle cleaning while late checkout stays on | Only the bonus is added/removed; the €30 and late Nuki expiry remain. |
| AC08 | Move checkout 2 November → 31 October | Entries move to 31 October; November loses the feature's amounts and October gains them; Nuki uses the new date. |
| AC09 | Checkout on the first day of a month | Entire €30 recognized in that checkout month even when all stay nights are in the previous month; no UTC date-prefix leakage. |
| AC10 | Cancel or mark no-show/non-refundable cancellation | Flag becomes false, both entries are deleted, salary/recognition are corrected, and existing Nuki revocation behavior applies. |
| AC11 | Restore the stay or clear its outcome | Flag remains false and entries remain absent until the user explicitly selects late checkout again. |
| AC12 | Personal use/Maintenance | Cannot save an enabled late-checkout selection or retain its charges after changing to an unsupported type. |
| AC13 | Repeat save or reconciliation | No duplicate entries or bonus salary; entries for another stay/property are unaffected. |
| AC14 | Fail a required ledger write | The associated stay mutation rolls back; no partial financial result is reported as a successful save. |
| AC15 | Month has €100 ordinary salary and one €10 bonus | Cleaning final salary and fully synced cleaner ledger expense are €110; sync/reset never increases them to €120. |
| AC16 | Bonus with no cleaner-entry logs | Cleaning salary includes €10, counted days remain zero, and the ledger contains only one bonus expense. |
| AC17 | No imported payout for an eligible late-checkout stay | Monthly incoming and recognized incoming each include €30; outgoing includes €10 only when cleaning is required. |
| AC18 | Supported stay spans two months | Ordinary payout recognition retains its existing allocation; the €30 is recognized once, entirely in checkout month. |
| AC19 | Other manual incoming transaction | It retains current recognition behavior; this feature does not recognize all incoming transactions. |
| AC20 | New/default property and explicit Nuki generation | Late-checkout code expires at 13:00 property-local time on checkout; ordinary code uses ordinary checkout time. |
| AC21 | Enable/disable late checkout on a stay with a code | Immediate provider update using the same credential/PIN and correct effective expiry. |
| AC22 | Change property late-checkout time | Existing affected current/upcoming codes immediately receive the new expiry; other properties/ordinary-checkout stays are unaffected. |
| AC23 | Property timezone/DST boundary | Nuki receives the UTC instant corresponding to configured local time on the actual checkout date. |
| AC24 | No existing code or pending/failed provider update | Save does not create a code; persisted finances remain correct; existing pending/error reporting and reconciliation are used. |
| AC25 | Change late-checkout selection/time only | Cleaning-calendar event timing is unchanged. |
| AC26 | Finance reset, then repeat reset/month sync | Both generated sources and their linkage survive; previews/counts agree; bonuses are counted once and charges remain recognized. |
| AC27 | Uncheck/cancel after Finance reset | Preserved late-checkout entries are still correctly removed by the normal stay lifecycle. |
| AC28 | Existing data after migration | Stay selections default false, property late-checkout time defaults to 13:00, and existing manual ledger entries are preserved. |

## 11. Completion criteria

Implementation is complete when the confirmed behavior and acceptance criteria above are satisfied, contracts and generated types agree, and verification results are recorded. Any newly discovered product decision that cannot be resolved from this document or existing authoritative behavior must be raised with the user rather than silently invented.
