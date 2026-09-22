# PMS 24 — Payout import: resolve named stays before commit

## 1. Purpose and approved business decision

Investigated on 2026-09-14 using the current working tree, `payout_2608_03.csv`, and the local `data/pms.db` snapshot. This document specifies future implementation; application code and database contents were not changed during analysis.

**Revised user decision:** when exactly one active Booking.com named stay in the selected property has both check-in and checkout dates exactly equal to the payout, **automatically link it and replace its display name with the payout guest name at commit** if the names differ. Multiple candidates still require manual selection. The user explicitly confirmed “Automatically link and rename”; this supersedes the earlier manual-selection requirement for unique exact-date candidates.

Goal: an operator can upload the supplied file, review seven automatic matches and proposed name changes, and commit seven linked finance bookings, their payout transactions and the stay-name updates using PMS's existing model.

## 2. Verified incident evidence

### 2.1 Database and CSV

- Database has property `1`, Airport Lounge, timezone `Europe/Bratislava`.
- Applied migration list ends at `000039_legacy_occupancy_removal`.
- All seven CSV rows are `Reservation`, EUR, reservation status `ok`, payment status `by_booking`, payout date `20 Aug 2026`, payout ID `e9bS4LeTAGXWtgIq`.
- There are no `finance_bookings` rows for these seven reservation references, no finance bookings linked to the seven candidate stays, and no `booking_payout` transactions with these references in this snapshot.
- Each CSV date pair identifies exactly one active `booking_com` named stay in property 1. All seven have `review_status = confirmed` and `source_channel = booking_ics`.

| CSV line | Booking reference | CSV guest name | Check-in → checkout | Named stay ID | PMS display name | Net EUR |
|---|---|---|---|---:|---|---:|
| 2 | 5441941191 | Martin Schneider | 2026-08-18 → 2026-08-19 | 271 | Martin | 57.16 |
| 3 | 6986274535 | Krisztián Križan | 2026-08-17 → 2026-08-18 | 268 | Krisztian | 72.92 |
| 4 | 6222340864 | Adelina Martina | 2026-08-16 → 2026-08-17 | 267 | Adelina | 72.73 |
| 5 | 6874089083 | Ihor Petrivskiy | 2026-08-15 → 2026-08-16 | 264 | Ihor | 69.93 |
| 6 | 6255831315 | Veronika Sovanyova | 2026-08-14 → 2026-08-15 | 244 | Veronika | 85.85 |
| 7 | 5916848183 | Zuzana Feledikova | 2026-08-13 → 2026-08-14 | 265 | Zuzana | 57.68 |
| 8 | 6237317051 | Richard Krall | 2026-08-12 → 2026-08-13 | 250 | Richard | 71.16 |

Expected totals: gross **60,521 cents (€605.21)**; net **48,743 cents (€487.43)**. IDs above are evidence, never implementation constants.

The stays' `source_reference` values are iCal UIDs ending in `@booking.com`, not numeric booking references. Several stays share a UID because their raw source block covers a longer date range. The seven `stay_source_links` rows have `link_status = source_deleted`; corresponding raw blocks have `status = deleted_from_source`. Their named stays are nevertheless active. Raw source disappearance is not evidence that these business stays are missing or cancelled.

Read-only inspection used SQLite URI `mode=ro&immutable=1` after ordinary read-only opening failed. The inspected data directory had no WAL/SHM sidecars. Findings describe this local snapshot; the running server's deployed revision and database were not independently verified. No upload/commit was run against the supplied database.

### 2.2 Rejection path

1. `frontend/src/views/FinanceView.vue`, `importBookingPayoutCSV`, posts the file to `/api/properties/{id}/finance/imports/preview`.
2. `backend/internal/finance/statements/parser.go` already supports the `Booking number` header alias, the file's date format, signed costs and EUR. The reported `Source: payout` and seven reservation references agree with that path.
3. `postFinanceImportPreview` in `backend/internal/api/finance_imports_handlers.go` finds no existing finance booking for each reference. `statements.Merge` therefore produces an insert.
4. `FindNamedStayForFinanceStayDates` in `backend/internal/store/finance_booking_payouts.go` first requires matching property, reference, dates, active status and eligible stay type. Numeric booking references do not equal these iCal UIDs.
5. Its fallback still requires identical dates and `LOWER(TRIM(display_name)) = LOWER(TRIM(guest_name))` when a guest name is present. Every row fails because a short PMS name is not the full payout name; Krisztian also differs in accents.
6. For an insert with no match, preview appends `no matching named stay for <reference>` and discards the row from its executable plan.

**Root cause:** valid existing stays cannot be resolved through the current strict matcher. The revised fix adds unique exact-date matching and payout-authoritative name updates, with manual resolution for ambiguous candidates.

### 2.3 Why existing manual mapping cannot repair this upload

`BookingPayoutsView.vue` already offers stay suggestions and calls `PATCH /finance/booking-payouts/{referenceNumber}/map`. Its `suggestionsForPayout` prefers exact dates, and its labels include dates, display name, stay type and ID. However, `mapFinanceBookingPayout` first requires the finance booking to exist. Rejected preview rows have no such record. The existing rematch endpoint also scans committed finance bookings only.

### 2.4 Separate line-number defect

`statements.Rejection` has `Line`, but successfully parsed `statements.Row` has no source-line field. The matching rejection is constructed with only `Reason`, leaving `Line = 0`. The UI renders that zero verbatim and keys rejection rows by it. Fix source provenance through parsing and matching, not just the displayed label.

### 2.5 Constraints already in use

- `finance_bookings.named_stay_id` is `NOT NULL` with a same-property foreign key to `named_stays`.
- `spec/PMS_21_Legacy_Occupancy_Removal_Spec.md`, particularly its finance contract, requires unmatched evidence to remain staged/rejected rather than becoming ownerless canonical bookings. It supersedes ADR-006's temporary ownerless-booking behavior.
- Named stays are operator-owned truth under the existing specifications. This revised requirement authorizes a narrow exception: update `named_stays.display_name` from a non-empty payout guest name for an eligible exact-date matched stay on commit. This supersedes the previous no-finance-name-mutation rule only for that field and condition. Synthetic stay creation remains prohibited.
- The current preview cache is process-local with a 15-minute TTL; commit consumes a preview token.
- Existing merge precedence, source flags, cash transaction category `booking_income`, transaction source `booking_payout`, and reference-based upsert remain the accounting mechanisms.

## 3. Required workflow

### 3.1 Matching and candidate discovery

1. For an existing canonical finance booking, retain its same-property `named_stay_id`. Re-upload must not require selecting the same stay again or silently remap a previously confirmed association.
2. Preserve existing deterministic reference/date and name/date matching for new rows. Add the exact-date fallback below for payouts only; no first-name, fuzzy or accent-folding algorithm is needed.
3. For a payout insert that otherwise fails matching, query same-property, active `booking_com` named stays with **both** dates exactly equal to the payout dates, regardless of display-name differences. If exactly one candidate exists, automatically resolve the row to that stay. This does not expand automatic matching of external stays.
4. If multiple candidates exist, show them all and require one explicit choice. Never use ordering or `LIMIT 1` as disambiguation. After manual resolution, apply the same exact-date name-update rule to the selected eligible Booking.com stay.
5. Use named-stay dates, not raw-block dates, for candidate discovery. Do not require an active raw block or source link.
6. If no candidate exists, keep the row rejected with an actionable explanation. Existing stay management is the way to create/correct a stay, followed by a fresh upload. Arbitrary date overrides, inactive-stay selection and new stay creation inside this dialog are outside this fix.
7. Existing strict matching supports `external` stays; preserve that behavior. Statement ingestion uses the shared matcher: preserve its existing behavior and add regression coverage rather than implicitly widening statement matching.
8. For a resolved active `booking_com` stay with both dates exactly equal to the payout, the non-empty payout `Guest name` is authoritative for `named_stays.display_name`, including when resolution used an existing finance link or deterministic reference. Trim surrounding whitespace and preserve the CSV's spelling, accents and capitalization. Never replace a name with an empty/whitespace-only value. This new rule does not rename external stays or a linked stay whose dates differ.
9. Plan a name write only when the trimmed non-empty payout name differs from the current display name. A finance booking classified as unchanged may still need a stay-name update; preview must disclose it and commit must not skip it. Keep booking counts separate from name-change counts.

### 3.2 Preview UI

Extend the current Booking.com CSV preview dialog in `FinanceView.vue` using existing UI components and the date/name/type/ID stay-label pattern from `BookingPayoutsView.vue`.

For an automatically resolved payout row, show the matched stay's ID and dates, the matching basis, and any proposed name change as **current PMS name → payout guest name**. Include these rows in planned inserts/updates/unchanged according to the finance merge result, and show a separate count of proposed stay-name changes. Unique exact-date matches require no per-row selection. Preview itself must not rename any stay.

For an ambiguous payout row show:

- Original CSV line, booking reference, full CSV guest name, dates and net amount.
- Explanation such as “No exact name/reference match. Select the existing stay for these dates.”
- An initially empty selector containing server-provided candidates, each visibly identified by dates, PMS display name, type and ID.
- Explicit selected state and an option to clear it.

Present these as **Needs stay selection**, separate from malformed/no-candidate rejections. Selected rows contribute to the displayed planned insert count; unselected rows do not. Never double-count one row between categories. All selectable rows must be reachable, including uploads exceeding the existing 50-item display truncation.

Keep existing partial-import semantics: committing leaves unselected ambiguous rows rejected and writes only automatic/resolved rows. Before commit, clearly show how many rows will be omitted. The supplied file requires no selections and plans seven inserts and seven name changes. For manual candidates, show the proposed rename when a selection is made. Cancel discards the preview without finance/stay writes. Clear selections when the file, preview token or property changes. Keep the dialog and selections on a recoverable commit error; explain that an expired/stale preview must be uploaded again.

### 3.3 API/cache extension — proposed implementation contract

The fields below are additions to existing endpoints, not descriptions of current API capabilities. Update `spec/openapi.yaml` and regenerate frontend types as part of implementation. No new service, persistent staging table or dependency is required.

Extend preview response with an additive `stay_name_changes` array containing `line`, `reference`, `named_stay_id`, `previous_display_name` and `payout_guest_name` for resolved rows with a proposed rename. Include matched-stay identity and matching basis on resolved preview entries; unchanged finance rows needing a rename must still be visible through `stay_name_changes`. Cache the original stay identity/name/dates and proposed name so commit can detect stale decisions. These are preview metadata, not canonical finance merge fields.

Extend preview response with an additive `needs_stay_selection` array for ambiguous rows only. Each item contains:

- `line`: positive source line, also identifying this record within this preview.
- `reference`, `guest_name`, `check_in_date`, `check_out_date`, `net_cents`, `reason`.
- `candidates`: array of `{ named_stay_id, display_name, stay_type, check_in_date, check_out_date }`.

Keep a corresponding server-side pending plan entry, including original parsed/canonical data and candidate IDs. Current rejection-only storage is insufficient: these rows must remain available for commit after selection. Do not count them simultaneously in the preview `rejected` array.

Extend `POST /finance/imports/commit` request for ambiguous rows. The example illustrates the payload shape only: the actual incident's lines 2 and 3 resolve automatically and must not be submitted as manual selections.

```json
{
  "preview_token": "<existing token>",
  "stay_selections": [
    { "line": 2, "named_stay_id": 271 },
    { "line": 3, "named_stay_id": 268 }
  ]
}
```

`stay_selections` is optional. Absence keeps unresolved rows rejected, preserving clients that send only `preview_token`. The server obtains the booking reference, dates and financial values from the cached row; the client never supplies authoritative booking/financial data. Source line is preferable to reference as the selection key because repeated references must not make two CSV records indistinguishable.

Return `400` for malformed selections, duplicate line selections, unknown preview lines, selections for non-pending rows, or IDs not offered for that row. Validate the complete selection list before business writes. Re-read each selected stay scoped to the property and validate current eligibility and dates. Return an actionable `409` requiring a fresh preview if a previously offered candidate or finance association has changed incompatibly. Keep existing `410` for missing/expired/wrong-property tokens. Require current Finance write access throughout.

Apply equivalent revalidation to automatic matches and renames: dates, eligibility, uniqueness for the date-only fallback, current association and previous display name must still agree with the preview. If an operator edits the name or a second eligible date candidate appears before commit, require a fresh preview rather than silently overwriting that edit or resolving new ambiguity. The replacement name comes from cached CSV data, never a client-provided name. Validate within the write transaction or use conditional writes to avoid a check/write race.

Resolve valid choices into the same insert/merge pipeline with the chosen `named_stay_id`; do not call the post-import mapping endpoint as a workaround. Preserve existing commit response count fields. Unselected pending rows count as rejected at commit, exactly once. Selected rows count according to their persisted merge outcome.

### 3.4 Commit correctness and accounting

- The automatically resolved or explicitly selected stay identity must survive through the canonical insert and payout transaction link. It must not be replaced by a second strict name lookup.
- On re-upload, use the persisted finance association. Apply the exact-date name-authority rule from §3.1; if the stay already has that name, do not write it again. Keep canonical finance guest names from the source according to existing merge precedence.
- Re-read affected finance references at commit, or detect stale state and require a fresh preview. Cached insert/update classifications must not overwrite a newer booking or remap it.
- Concurrent consumption of one preview must not execute its plan twice. Synchronize preview claim/consumption with the existing in-process cache mechanism; database uniqueness and transaction upserts remain additional safeguards.
- For each successful payout row, persist booking, resolved link, applicable cash transaction and any stay-name update as one atomic unit, using existing SQLite transaction patterns. Increment successful counts only after that unit succeeds. Failure must roll back the rename as well as the financial writes. Keep existing partial-file processing semantics for independent rows.
- `commitPayoutBookingSideEffects` currently discards link/transaction errors, and success counts are incremented before it runs. This is an observed adjacent defect, not the cause of the seven preview rejections. The new resolved-row path must propagate failures and must not claim a successful payout without its required transaction. Reuse/refactor existing atomic payout store patterns rather than adding a second accounting model.
- Retain merge/import audit writes and report failures; do not silently report a complete successful import when required persistence fails.
- Preserve net sign handling, positive cost normalization, EUR handling, payout ID/date storage, and statement/payout precedence. This file requires seven incoming transactions totaling 48,743 cents, categorized as `booking_income`, linked by `transaction_id` with source references matching the booking numbers.
- Update only the authorized display name and the existing stay-edit metadata/audit attribution appropriate to a name change. Reuse existing named-stay update/audit conventions; do not add a name-history system. Preserve named-stay identity, dates, source references, source links, nights, lifecycle, cleaning and Nuki state. Retain the existing narrowly defined finance-evidence confirmation behavior. A rename must not recreate stays, generate/rotate access codes, or rewrite invoice snapshots or historical source evidence.

### 3.5 Line provenance

Add source-line metadata to successfully parsed rows and carry it into cached plans and matching failures. Prefer `encoding/csv.Reader.FieldPos(0)` for the start of a successful record so quoted multiline records and blank lines are represented correctly; handle parse errors using available CSV error positions. Apply the same provenance convention to both parsers.

For this file, report lines **2–8**, never 0. Line metadata is transport/import provenance, not a canonical finance field: do not put it into raw source maps or merge equality in a way that makes otherwise identical re-uploads look financially changed. Use stable unique UI keys for rejected and pending rows.

## 4. Implementation map

| Area | Existing files / extension |
|---|---|
| Parser/provenance | `backend/internal/finance/statements/parser.go`, `parser_test.go` |
| Preview/cache/commit | `backend/internal/api/finance_imports_handlers.go` |
| Stay candidate query and link validation | `backend/internal/store/finance_booking_payouts.go` |
| Existing stay-name update and audit conventions | `backend/internal/store/named_stays.go`, `backend/internal/api/occupancy_named_stay_handlers.go` |
| Canonical and transaction persistence | `backend/internal/store/finance_bookings_merge.go` |
| Existing API integration test | `backend/internal/api/finance_imports_runtime_test.go` |
| Payout/import regressions | Existing finance API/store test suites |
| Upload preview and selections | `frontend/src/views/FinanceView.vue` and focused Vitest tests |
| Existing UI pattern | `frontend/src/views/BookingPayoutsView.vue`, `frontend/src/api/types/bookingPayouts.ts` |
| API contract/types | `spec/openapi.yaml`, `frontend/src/api/types/generated.ts` |

The working tree already contains user changes, including `finance_booking_payouts.go`, named-stay/Nuki code, tests and generated/OpenAPI types. Inspect the current diff before implementing and integrate with those changes. Do not replace files wholesale with the version assumed by this specification.

## 5. Acceptance criteria and regression tests

Use an isolated test database with synthetic/minimal stays reflecting the incident. Do not depend on production IDs or mutate `data/pms.db`. The supplied CSV is an investigation fixture currently untracked; keep the durable regression self-contained rather than relying on an untracked root file.

1. **Incident preview:** seven records parsed, seven automatically planned inserts, seven proposed stay-name changes, correct lines 2–8 and associations from §2.1, no manual-resolution rows and no rejections. Preview causes no finance/stay writes.
2. **Incident commit:** without manual selections, response is total 7 / inserted 7 / updated 0 / unchanged 0 / rejected 0. Exactly seven finance bookings and linked incoming transactions persist, net total 48,743 cents, with correct per-row amounts and payout metadata. The seven stay display names become the full CSV guest names from §2.1, including `Krisztián Križan`. Other business/source/operational fields remain unchanged apart from normal update/audit metadata.
3. **Ambiguity/partial selection:** use a separate fixture with multiple eligible candidates. Ambiguous rows are never automatically linked or renamed; unselected ambiguous rows remain rejected. Automatic and explicitly resolved rows commit with accurate counts and visible omissions.
4. **Repeat upload:** the same file after successful commit requires no selections, reports seven unchanged bookings under existing merge semantics and no proposed name changes, and creates no duplicate bookings/transactions or extra income. No redundant stay-name update occurs. Duplicate-file warning remains informational.
5. **Source deletion/split blocks:** active named stays remain candidates even when raw source links are deleted or a raw block covers several named stays. Candidate identity comes from exact named-stay dates.
6. **Candidate boundaries:** other-property, cancelled/archived, maintenance/personal-use and date-mismatched stays are not offered by this new fallback. Multiple eligible candidates are all displayed, with no automatic first choice. A unique candidate resolves even with an unrelated name: exact dates, not name compatibility, authorize this fallback.
7. **Existing behavior:** exact reference/date and exact name/date matches still work; existing external-stay matching and statement merge behavior remain covered. Persisted explicit associations are not silently rematched by re-upload.
8. **Validation:** reject forged candidate IDs, another property's IDs, unknown/duplicate source-line selections and expired tokens. Candidate eligibility/date/name changes, newly introduced date ambiguity or conflicting booking writes between preview and commit are detected before affected business writes, for automatic and manual resolutions alike.
9. **Failure/retry:** inject payout-transaction or stay-name-update failure and verify no half-written successful row, persisted rename from a failed row, or inflated success count. Repeated/concurrent commit attempts cannot double-count income. Invalid selection payloads cause no partial business writes.
10. **Line reporting:** malformed rows, unmatched rows, blank lines and quoted multiline fields have meaningful source positions. Re-uploading an unchanged row from another physical line must not create a financial update merely because its line moved.
11. **UI:** selection/clear, cancellation, partial commit counts, commit failure, token expiry, property/file changes and more than 50 pending rows are covered with existing Vitest/Vue Test Utils tooling.
12. **Name authority:** cover short/full names, accent/case differences, surrounding whitespace, empty payout names and existing-finance-link uploads. Empty names never erase a stay name. A finance-unchanged row with an eligible differing display name shows and persists the rename. Statements, external stays and date-mismatched linked stays do not acquire this new renaming behavior. Cancelling preview causes no rename.

Required implementation checks (run in the indicated directories):

- `backend/`: `go test ./internal/finance/statements ./internal/store ./internal/api`, followed by `go test ./...` for final backend verification.
- `frontend/`: `npm run types:openapi`, `npm run type-check`, `npm test`, `npm run build`.
- Inspect the final diff and generated contract consistency; document any pre-existing failures separately from new failures.

## 6. Scope and completion

This fix extends existing upload preview/commit and stay selection. It requires no schema relaxation, finance reset, database repair script, synthetic stay creation, replacement reservation-ID system, fuzzy-name library or external integration. Do not rewrite iCal UIDs into booking numbers. No historical payout or invoice deletion is needed for this incident because the seven finance bookings do not exist in the inspected snapshot.

The reported payout period is not the rejection cause. Preserve current payout-date parsing/period behavior for this fix; timezone/date-display changes need their own demonstrated regression case.

Automatic linking and payout-authoritative renaming for a unique exact-date Booking.com stay is the revised business decision; manual selection remains for ambiguity. The deliverable is complete when the supplied scenario commits seven automatic links and seven full-name updates, retry is idempotent, unresolved evidence still cannot create ownerless finance bookings, line diagnostics are accurate, and the checks above pass.
