# PMS 25 — Reservation statement import and cancellation cohorts

## 1. Objective and confirmed decisions

Fix importing `reservation_statements_overview_2026-08.csv` through the existing Finance CSV preview/commit workflow while preserving the implemented PMS 24 payout behavior.

This is an implementation specification, based on inspection of the current working tree and local `data/pms.db` on 2026-09-14. No application code, schema or database data was changed during this investigation.

The user confirmed:

1. **Statements: link, preserve stay name.** For an `OK` statement row, automatically link the unique active Booking.com stay with exactly matching arrival and departure dates. Keep the named stay's display name. The full statement guest name remains finance/source data.
2. **Unmatched cancellations: skip canonical booking creation, retain analytics evidence.** Do not present these as matching errors or discard the reservation evidence.
3. **Analytics scope: statement cohorts only.** Include these cancellations in the existing booking-month and arrival-month statement cancellation charts. Do not add them to the overall named-stay cancellation rate or cancellation lead-time histogram.
4. **Storage addition approved:** add a small SQLite statement-evidence table using existing migration/store/import patterns. It must survive restarts and deduplicate repeat uploads without requiring a named stay.
5. **Payouts remain functional:** preserve automatic exact-date payout linking, payout-authoritative stay renaming, cash transaction creation and idempotence. Statement linking must not invoke payout renaming or cash side effects.

## 2. Verified incident and root causes

### 2.1 Current environment and counts

- Property `1`: Airport Lounge, timezone `Europe/Bratislava`, configured Booking.com hotel ID `13452548`, equal to all 27 statement rows.
- File has **24 OK reservations and 3 CANCELLED reservations**, not 27 completed stays.
- Twelve references already have finance bookings, all with `has_payout_data = 1`, `has_statement_data = 0` in the inspected snapshot. They account for the reported 12 updates.
- Eleven other `OK` rows fail strict stay matching; the final `OK` row fails datetime parsing before matching.
- None of the 15 rejected references has a finance booking in the inspected database. No named stay has one of the three cancelled reservation numbers as its source reference.
- The latest local import, ID `17`, is the supplied seven-row payout: 7 inserted, 0 rejected. Those stays now carry the full payout guest names. This supports the user's report that payout importing works.

The database was inspected with SQLite URI `mode=ro&immutable=1`; the data-directory listing had no WAL/SHM sidecars. These findings describe the supplied snapshot and working tree, not an independently verified running deployment. No live import was performed.

### 2.2 New OK bookings that should resolve

Each row below has exactly one active `booking_com` named stay in property 1 with matching dates. IDs are evidence, not implementation constants.

| CSV line | Reservation | Arrival → departure | Stay ID | Existing display name | Statement Guest name |
|---|---|---|---:|---|---|
| 8 | 6593934589 | 2026-08-05 → 2026-08-06 | 242 | Iurii | Iurii Furman |
| 9 | 5586026055 | 2026-08-06 → 2026-08-07 | 247 | Richard | Richard Paulinyi |
| 10 | 5265338781 | 2026-08-07 → 2026-08-08 | 248 | Andrea | Andrea Popovičová |
| 12 | 6773131904 | 2026-08-08 → 2026-08-10 | 243 | Michaela | Michaela Chlebovcova |
| 13 | 5832656436 | 2026-08-11 → 2026-08-12 | 249 | Rand | Rand Matan |
| 21 | 5079011734 | 2026-08-19 → 2026-08-23 | 270 | Jaroslav | Jaroslav Kučírek |
| 23 | 5424955375 | 2026-08-23 → 2026-08-24 | 272 | Erik | Erik Forro |
| 24 | 6104510626 | 2026-08-24 → 2026-08-25 | 273 | Andrea | Campaniello Andrea Hans |
| 25 | 6940764496 | 2026-08-25 → 2026-08-26 | 245 | Javorka | Javorka, s.r.o. Javorka, s.r.o. |
| 26 | 6388468113 | 2026-08-26 → 2026-08-28 | 274 | Amelia | Amelia Percy |
| 27 | 6800596898 | 2026-08-28 → 2026-08-30 | 246 | Michal | Michal Doležal |
| 28 | 5622366371 | 2026-08-30 → 2026-08-31 | 275 | Melinda | Melinda Pavel |

Stay `269` is also named Jaroslav Kučírek with dates August 19–23, but is **cancelled**. The `OK` statement row must select active stay `270`, not cancelled stay `269` just because its full name matches.

The 12 existing finance references are `6042579020`, `6270622388`, `6270640410`, `5746777721`, `5789105994`, `6237317051`, `5916848183`, `6255831315`, `6874089083`, `6222340864`, `6986274535`, `5441941191` (CSV lines 3–7 and 14–20). Their current `named_stay_id` associations take precedence over new matching.

### 2.3 Matching is explicitly payout-only today

In `backend/internal/api/finance_imports_handlers.go`, preview:

- Resolves existing bookings through `FinanceBookingNamedStay`.
- Otherwise uses `FindNamedStayForFinanceStayDates`, which requires an exact source reference/date match or exact trimmed case-insensitive full-name/date match.
- Enters `ListNamedStaysForFinanceExactDates` fallback only under `parsed.Source == statements.SourcePayout`.
- Rejects statement inserts when strict matching returns no stay.

The new statement failures are therefore an unimplemented statement fallback, not a CSV header problem or a regression in payout parsing. The shared store candidate query already returns active Booking.com named stays by exact dates and ignores raw-block/source-link lifecycle; reuse it with source/status-aware policy.

### 2.4 Line 28 uses a valid unsupported datetime precision

`parseStatementDateTime` accepts `2006-01-02T15:04:05`, `2006-01-02 15:04:05` and date-only input. It does not accept `2026-08-30T10:25`.

Add the observed minute-precision layout `2006-01-02T15:04`. Interpret it in the property timezone, just like existing offset-free statement timestamps; omitted seconds are zero. For Bratislava this value becomes **2026-08-30T08:25:00Z**. Keep strict validation and existing formats. Do not substitute upload time or mark a genuinely invalid datetime valid.

Once line 28 parses, the existing arrival-based `derivePeriod` produces **2026-07-31 – 2026-08-30**. The period ends at the maximum arrival, not departure; August 31 is not the expected period end under the existing contract.

### 2.5 Cancellation evidence is a different identity

| Line | Reservation | Guest | Booked on (source local time) | Arrival → departure | Why date matching is wrong |
|---|---|---|---|---|---|
| 2 | 5790874952 | Cayuela Antonia | 2026-07-05T23:41:48 | 2026-07-31 → 2026-08-01 | Exact dates belong to active Joey, stay 237, reference 6042579020 |
| 11 | 5616543386 | Matej Francuz | 2025-12-25T14:14:28 | 2026-08-07 → 2026-08-10 | No exact active stay; overlaps Andrea and Michaela |
| 22 | 5198639750 | Hana Hellebrandová | 2026-08-14T11:45:20 | 2026-08-20 → 2026-08-21 | No exact stay; falls inside Jaroslav's active stay |

All three have final amount 0, commission 0, room nights 0 and blank payment fee. No cancellation-effective timestamp is supplied. Booked-on, invoice number and upload time must not be treated as cancellation time.

Current `finance_bookings.named_stay_id` is mandatory with a same-property FK. The existing cancellation-cohort query in `backend/internal/store/analytics_statement.go` joins statement-aware finance bookings to named stays. A skipped row has no canonical booking and currently disappears from those cohorts. `finance_imports` contains aggregate counters, not sufficient per-reservation evidence.

## 3. Source-specific import policy

### 3.1 OK statements

Apply hotel filtering and parser validation first. For normalized `Status = OK`:

1. Preserve an existing same-property finance association by booking reference.
2. Otherwise preserve deterministic strict matching, with active-stay eligibility.
3. If strict matching fails, query active same-property `booking_com` stays with **both dates exactly equal**. One candidate resolves automatically; multiple candidates enter the implemented manual-selection preview flow; no candidates remain a genuine actionable rejection.
4. Never use overlap, shortened dates, raw-block dates, fuzzy names, or archived/cancelled stays as the fallback.
5. Store the statement's `Guest name` in canonical finance data under existing precedence. Preserve `Booker name` independently: line 13 has booker `R Matt` and guest `Rand Matan`.
6. **Never rename a named stay from a statement**, including manual selections and existing finance associations. A statement preview must have no proposed stay-name changes.
7. Consume the resolved identity at commit. `commitStatementBookingSideEffects` currently performs another strict lookup and can relink; replace that rematching behavior for this flow with the validated plan identity. It must not discard the new date-only decision or override a persisted association.

Do not broaden this fallback to other statement statuses without an explicit rule. Existing non-OK/non-CANCELLED behavior and PMS 17 outcome handling remain in force.

### 3.2 CANCELLED statements

Route cancellations before any date/name fallback that could choose a replacement guest.

- An existing finance booking with the same property/channel/reservation reference is reliable identity. Merge its statement fields using existing precedence and retain its stay link. Preserve the existing cancellation-review workflow instead of automatically cancelling the stay.
- A new canonical association may use a unique same-property Booking.com reservation-reference match with matching dates; the numeric reservation reference must genuinely match existing source-reference data, not an iCal UID or a date inference. Ambiguous reference matches do not resolve automatically.
- Without reliable identity, classify the row as **Skipped booking creation — cancellation retained for statement analytics**. Persist evidence at commit; create no finance booking, transaction, stay, stay nights, review flag on another guest, invoice, cleaning task or Nuki action.
- Do not offer date/name-only manual candidates for these cancellations. The skip is deliberate, not an unresolved error requiring the operator to manufacture a stay.
- Preserve operator-confirmed `cancelled_non_refundable` and `no_show` exclusions when a reliable existing finance/stay association supplies them. Do not infer such outcomes from `CANCELLED` alone.

### 3.3 Payout isolation

Keep payout-specific decisions explicit:

| Behavior | Payout | Statement |
|---|---|---|
| Unique active Booking.com exact-date fallback | Existing PMS 24 behavior | Add for OK rows |
| Stay display-name update | Existing payout-authoritative behavior | Never |
| Multiple date candidates | Existing selection | Reuse for OK rows |
| Cash transaction upsert | Existing payout net/date/category | Never from statement import |
| Unmatched CANCELLED source evidence | Existing payout policy | Persist analytics evidence, skip booking creation |

Do not simply remove every `SourcePayout` condition. In particular, pending-selection commit currently assigns `StayNameChanged` from the name difference without a source check, while automatic name changes are payout-gated. Adding statement pending rows requires explicitly preventing that rename flag and its UI presentation for statements.

## 4. Persistent statement evidence (approved schema addition)

### 4.1 Purpose and shape

Add `finance_statement_evidence` using the next available migration number (the repository already contains migration 000040; do not reuse it). This is source evidence, not a second cash ledger or synthetic stay table.

Proposed minimum fields:

- `id`, `property_id` (property FK), `source_channel` (`booking_com`), `reference_number`.
- `hotel_id`, normalized `status`, `booked_on` (UTC RFC3339), `check_in_date`, `check_out_date` (property-local dates).
- `raw_statement_row_json`, preserving the original supplied values including zero/blank distinctions and guest/booker names.
- `last_import_id` referencing `finance_imports`, `source_line`, `created_at`, `updated_at`.
- Unique key `(property_id, source_channel, reference_number)`; indices supporting property/cohort queries as needed.

No required `named_stay_id`, payout amount, payout date or cash transaction link belongs here. Resolve any canonical association by the same property/channel/reference key; the evidence row need not carry a redundant mutable association.

### 4.2 Writes and lifecycle

- Persist evidence on **commit**, never on preview. Capture valid same-hotel statement records, including retained cancellations. Keeping normalized evidence for valid statement records permits later status corrections without leaving a stale cancellation indefinitely.
- Rejected syntax/invalid required dates and other-hotel rows do not enter this table. A valid row rejected only for missing an OK stay still retains source evidence on commit, so an explicit correction can replace old cancellation evidence; it remains rejected for canonical booking creation and does not become an active cohort contributor merely because evidence exists. Make this distinction clear in its rejection explanation.
- Re-upload upserts the same reservation evidence. File SHA/import ID/CSV line are provenance, not the reservation identity. Reordered/repeated/overlapping files must not multiply the cancellation count.
- Follow existing statement update ordering: the latest explicitly committed valid statement value replaces the current evidence for that reservation. The file contains no version timestamp from which to infer a newer business state. An identical re-upload is not a new cancellation.
- An explicitly uploaded later `OK` or other status replaces prior `CANCELLED` evidence. Missing references in another file do not imply cancellation, reactivation or deletion; exports are not proven complete snapshots.
- Reject conflicting duplicate versions of the same reference within one upload with line-specific diagnostics, rather than letting record order create multiple contradictory plans. Identical duplicates must not create multiple canonical/evidence identities or metric contributions; document their row-count disposition.
- For a canonical statement row, write booking, evidence and merge/review side effects in one existing SQLite row transaction. For an evidence-only cancellation, commit the evidence successfully before incrementing the retained-cancellation counter. Propagate failures and maintain accurate partial-import counts.
- Retain per-import aggregate counts using `finance_imports`. Evidence upsert must not be mistaken for another new booking. Existing application audit logging records the import actor and result; use those conventions.
- Include evidence in the existing finance-reset preview/deletion counts and delete it before its import parents during an explicitly requested finance reset. This matches the existing reset's removal of statement/import data. Do not execute a reset for this fix.
- Migration adds storage only; existing canonical statement bookings remain readable through the fallback analytics branch below. Do not invent historical evidence for previously rejected rows from aggregate import counts. Re-upload is how those rows become known.

## 5. Cancellation-cohort analytics

### 5.1 Extend only the existing statement charts

Update `ListCancellationByBookingCohort` and `ListCancellationByArrivalCohort`, keeping their DTOs and formula:

`rate = cancelled / (cancelled + active)`; `other` remains excluded from numerator and denominator.

Construct one effective statement record per property/channel/reference:

1. Use committed statement evidence where available, with the corresponding canonical finance/stay association if one exists.
2. For historical statement-aware finance bookings without evidence, retain the current canonical query behavior.
3. Deduplicate **before bucketing**; do not add a cancellation evidence record to a second copy of the same linked finance booking. Joins must include property/channel/reference, never dates or names.
4. An evidence-only `CANCELLED` reservation counts once as cancelled. This is the approved exception to named-stay-only cohort membership.
5. For `OK` and other statuses, retain the current canonical statement-booking eligibility. Evidence-only unmatched OK rows do not become active stays or inflate the denominator. A corrected non-cancelled status removes its previous evidence-only cancellation contribution.
6. Apply existing effective outcome precedence (`finance_bookings.outcome_override`, then the associated `named_stays.stay_outcome`) when a real association exists. `cancelled_non_refundable` and `no_show` belong in `other`, not cancelled/active. A later payout association must not add a second cohort record.

For associated historical/canonical records preserve current cohort dates: booking cohort uses `named_stays.first_known_at`, arrival cohort uses `named_stays.check_in_date`. For evidence without a stay, use statement `booked_on` and `check_in_date`. Apply the property timezone and existing half-open window boundaries. Do not silently redefine all historical booking cohorts as source `booked_on` cohorts.

Update `HasAnyStatementData` and statement freshness helpers to recognize committed evidence, including a property with only retained cancellations. Keep the current meaning of last statement booked-on (not upload time). Cohort charts must not be hidden behind “No statement data” after a cancellation-only import.

### 5.2 Metrics explicitly outside the addition

Per the user's answer, do not add evidence-only cancellations to `ListCancellationsInArrivalWindow`, `CountActiveArrivalsInWindow`, `performance.cancellation`, cancellation lead-time buckets, sold nights, revenue, occupancy, pace, commission trends or guest-stay counts. No cancellation timestamp is known from this CSV. Update the statement-chart help text to explain inclusion of reservations cancelled before a PMS stay existed; the overall cancellation panel retains named-stay semantics.

### 5.3 Exact expected contributions

The three retained cancellations contribute to:

- Booking cohorts: **2025-12 +1**, **2026-07 +1**, **2026-08 +1**, subject to the requested query window.
- Arrival cohorts: **2026-07 +1**, **2026-08 +2**.

Do not place all three in August merely because the filename or commission invoice is August-based.

In an isolated fixture containing exactly this statement's reservations and corresponding stays, with linked first-known dates aligned to their booked-on values and no outcome overrides:

- July arrival cohort: 1 cancelled / (1 cancelled + 1 OK) = **50%**.
- August arrival cohort: 2 cancelled / (2 cancelled + 23 OK) = **8%**.
- Combined July–August arrival cohort population: 3 / 27 = **11.111…%**.

These are controlled-fixture expectations, not asserted totals for the real property, which has other bookings and canonical first-known timestamps.

## 6. Preview, API and commit contracts

Reuse `/finance/imports/preview`, `/finance/imports/commit`, the current preview cache, `needs_stay_selection`, and line-keyed `stay_selections`. Add source-aware policy rather than a separate upload endpoint.

Proposed additive contract:

- Preview `skipped_cancellations`: items with `line`, `reference`, `guest_name`, `check_in_date`, `check_out_date`, `booked_on`, `reason`. Label them as evidence to be retained **when committed**, not already saved.
- Commit/import history `row_count_skipped_cancellations`: number of rows successfully retained through the cancellation-only route. Add a default-zero column to `finance_imports` and extend store/list DTOs accordingly.
- Keep `skipped_other_hotel` and `row_count_skipped_other_hotel` for hotel mismatches only. Do not overload them with cancellations.
- Every input row has one primary disposition. Total = inserted + updated + unchanged + skipped-other-hotel + skipped-cancellations + rejected. Evidence persistence accompanying a canonical booking is not an extra row in these totals.
- Payout responses use empty/zero values for these additions. Preserve existing payout request fields and `stay_name_changes`/`payout_guest_name` compatibility; statements simply emit no rename proposals.

The inspected statement should preview and commit as:

| Category | Expected |
|---|---:|
| Source rows | 27 |
| New finance bookings | 12 |
| Updated finance bookings | 12 |
| Unchanged finance bookings | 0 |
| Skipped cancellations retained for cohorts | 3 |
| Other-hotel skips | 0 |
| Rejected | 0 |
| Stay-name changes | 0 |
| Cash transactions created by statement | 0 |

This expectation applies to the inspected pre-statement snapshot. Re-upload after successful commit should produce 24 unchanged canonical bookings plus 3 retained/skipped cancellations, with no additional evidence identities or metric contributions.

Show the retained-cancellation list and count separately from errors; the commit success message must mention it. Surface manual selections for ambiguous OK rows using current labels. Do not display a statement net of zero as a promised payout; use the statement final amount where a financial preview value is needed. Show all selectable rows, not only the first 50.

Revalidate statement plan identities/eligibility/uniqueness before and within writes as appropriate. Existing persisted associations must not be silently remapped on re-upload. Use a request-local resolved plan rather than appending selections into the cached plan before all validations succeed: the current commit handler mutates `preview.Plan` during validation, which would make retry behavior especially fragile when statement selections are added. Test invalid-selection retry without duplicate entries.

`ContextWithTransaction`/`dbForContext` already exist in the payout implementation. Reuse them for statement atomicity; do not call store methods that escape the transaction for related writes. In particular, review `MarkNamedStayFinanceReviewForBooking`, which currently executes directly on `s.DB`, and propagate statement side-effect errors rather than discarding them. Preserve payout behavior while making shared paths transaction-aware.

## 7. Financial precedence and payout protection

Do not change monetary precedence to make the matching fix pass:

- Existing payout `Amount` remains authoritative for populated invoice gross amounts; statement `Final amount` fills missing amounts under current merge rules.
- Statement fields such as booked-on, persons, rooms, guest/booker names, status, commission and fee follow the existing merger. Source raw JSON and flags remain intact.
- Statement import does not overwrite payout net, payout ID/date, payment status, reservation-status provenance, transaction IDs or transaction amounts/directions/categories.
- New statement-only bookings use the current `has_statement_data = 1`, `has_payout_data = 0`, zero-net/no-transaction behavior and current required payout-date placeholder handling. A placeholder is not cash evidence.
- Preserve the special observed existing booking `6270622388`: it has `has_payout_data = 1`, null amount, zero net and no transaction. Statement may fill its missing amount with 6,156 cents but must not manufacture a cash receipt or “repair” it as part of this import.
- Example regression: reference `5441941191` has payout gross **7,097 cents**, statement final **6,480 cents**, payout net **5,716 cents**. After statement, gross must remain 7,097 and net/transaction remain 5,716.
- The seven PMS 24 payouts retain gross total **60,521 cents** and net total **48,743 cents**, linked transaction identities and payout metadata after the statement import.

One cross-source implementation hazard is already visible: payout preview decides whether a name changed using the incoming payout guest name, but its side-effect writer uses merged `entry.Result.GuestName`, for which statement data has precedence. A statement and payout with different guest names can therefore make the payout rename use the wrong source. Regression coverage must require the existing approved payout rule: use the cached **raw payout guest name** for payout stay renaming, while canonical finance guest name retains statement precedence. Make only the minimal correction needed if the test exposes this; do not rename stays from statements to hide the conflict.

The merger currently skips some numeric zero updates. This was not the cause of the supplied matching/parser failures. Preserve raw zero/blank evidence and base cancellation counting on status; do not widen this task into an unapproved financial-zero precedence redesign.

## 8. Implementation locations and verification

| Area | Existing code / intended extension |
|---|---|
| Timestamp parser | `backend/internal/finance/statements/parser.go`, `parser_test.go` |
| Source precedence | `backend/internal/finance/statements/merge.go`, `merge_test.go` |
| Preview/commit, skip counts, selections | `backend/internal/api/finance_imports_handlers.go` |
| Stay matching and payout name helper | `backend/internal/store/finance_booking_payouts.go` |
| Transaction-aware canonical persistence | `backend/internal/store/finance_bookings_merge.go`, `store.go` |
| Import persistence/history | `backend/internal/store/finance_imports.go` |
| New evidence store/schema | New focused store file and next available up/down migration under `backend/internal/migrate/` |
| Statement cohorts/freshness | `backend/internal/store/analytics_statement.go`, `analytics.go`, associated tests |
| Analytics response integration | `backend/internal/api/analytics_handlers.go` |
| Reset integration | `backend/internal/store/finance_reset.go`, existing reset API/types/tests |
| Import UI | `frontend/src/views/FinanceView.vue` and focused Vitest coverage |
| Cohort help/empty states | `frontend/src/views/analytics/AnalyticsPerformanceTab.vue` |
| Contract/types | `spec/openapi.yaml`, generated frontend types |

Inspect the existing working-tree diff first. PMS 24 implementation and unrelated Nuki/named-stay work are uncommitted; integrate with them rather than reverting or replacing them from HEAD. Update relevant analytics data-inventory documentation to record the explicit evidence-only cohort exception to the previous named-stay-only model.

Required tests on isolated databases/fixtures:

1. Parse all 27 supplied records; minute precision becomes `08:25:00Z`, lines remain 2–28, period ends August 30. Existing second-precision/date-only parsing and payout formats remain valid; malformed timestamps remain rejected.
2. Reproduce all 12 new mappings in §2.2 and the existing 12 associations. Jaroslav links to active 270, not cancelled 269. Statement leaves every display name unchanged, including `Rand`, `Jaroslav`, and `Javorka`.
3. Mixed file commits with exact counts in §6. Cancelled Cayuela never touches Joey; Matej/Hana never affect overlapping stays. Commit creates 12 canonical bookings, updates 12, retains three cancellations, and creates no cash transactions or synthetic stays.
4. Unique/ambiguous/no-date matches, source-deleted raw links, multi-hotel filtering, inactive/other-property candidates, manual OK selection and retry all retain explicit source/status rules. Statement manual selection does not set a rename flag.
5. Evidence persists across process/cache restart. Same file, reordered file and overlapping imports deduplicate references; corrected statuses/dates move or remove contributions correctly. Invalid rows and other-hotel records cannot pollute cohorts. Later canonical linkage does not double-count an evidence-only cancellation.
6. Test cancellation-only property: statement-data flag is true; both cohort charts render. Test precise cohort contributions in §5.3, including the December 2025 booking outside an August query window. Preserve PMS 17 `other` exclusions and property isolation.
7. Overall cancellation rate, lead-time buckets, cashflow and sold-night metrics are unchanged by evidence-only cancellations. Existing canonical data without new evidence remains visible in cohorts after migration.
8. Run **payout → statement → same payout**, **statement → payout → same statement**, and payout-only repeat tests. Assert canonical guest-name precedence independently of payout stay-name authority, unchanged payout totals/transaction IDs and no duplicated income. Include different guest names across the two source files.
9. Inject statement evidence/booking/review/merge failure: no successful half-written canonical row or inflated success count. Invalid selection followed by a corrected retry must not duplicate cached plans. Payout transaction/name rollback tests continue to pass.
10. Migration/finance reset: existing payout/stay/invoice values and constraints survive migration; reset counts and deletion ordering cover evidence; evidence resets only through the existing explicit finance-reset operation. Re-upload reconstructs deduplicated evidence afterward.
11. UI/contract tests cover separate cancellation skip counts, zero statement rename proposals, ambiguous selection, history totals, cancellation-only empty states, and unchanged payout preview/commit payload compatibility.

Run existing checks using existing tooling:

- In `backend/`: `go test ./internal/finance/statements ./internal/store ./internal/api ./internal/migrate`, then final `go test ./...`.
- In `frontend/`: `npm run types:openapi`, `npm run type-check`, `npm test`, `npm run build`.
- Review contract changes and final diff. Report pre-existing failures separately; do not describe payouts as protected based only on parser tests.

Do not run implementation tests against `data/pms.db`. Use self-contained durable fixtures rather than depending on untracked CSV files or hard-coded production IDs. Completion requires all statement, cancellation-cohort and cross-source payout acceptance cases above; merely removing the 15 rejection messages is insufficient.
