# PMS 34 — Nuki API correctness and durable managed-code cleanup

## 1. Scope and status

**Deliverable: specification only. Implementation is not authorized.**

The owner asks to correct Nuki API usage and investigate PMS-created codes
becoming “External” and remaining undeleted. The concrete example is
**Booking-Amarildo**, marked External in PMS. This document is the consolidated
specification for authorization lifecycle, ownership, cleanup, and log-fetch
corrections, including their evidence and regression requirements.

Investigation: 2026-10-09, source commit
`216b2b802ea1f3bedbcd7c27f5e2969537b10392`, read-only `data/pms.db`, read-only live
Nuki GET requests, and Nuki Swagger version 4.20.0. No remote mutation, local
database update, application-code edit, or test execution was performed.

**Confirmed failure:** local expiry cleanup completed and discarded ownership
while the corresponding remote authorization survives. Source explains why
PMS labels it External and never selects it for another cleanup attempt.
The precise historical DELETE outcome was not retained; do not invent it.

When implementation is requested, read `PMS_13_Coding_Conventions.md` and retain
PMS 23's explicit single-stay creation policy: create only from Nuki Access;
automatic maintenance can update/revoke existing credentials but never create
replacements. Inspect working-tree changes before editing.

## 2. Booking-Amarildo: evidence and timeline

Property `1` / Airport Lounge, timezone `Europe/Bratislava`, smartlock
`18233733061`.

| Evidence | Observed value |
| --- | --- |
| Named stay | `293`, September 22–23, 2026 |
| Local access-code row | `238`, label `Booking-Amarildo` |
| Local creation | `2026-09-22T17:43:09Z` |
| Generation audit | `6287`, `nuki_generate`, success, `2026-09-22T17:43:15Z` |
| PIN-reveal audit | `6289`, access-code `238`, success, same second |
| Cached remote authorization | `6ab2be33a7fc3019addfa5d1`, cache row `4576` |
| Remote creation date | `2026-09-22T17:43:15.000Z`, matching the generation audit |
| Local and remote validity | September 22 `12:00Z` → September 23 `07:00Z` |
| Cleanup evidence | event log `50`, code `238`, `cleanup_expired`, `2026-09-25T17:50:28Z` |
| Local state after cleanup | `revoked`, external ID NULL, error NULL, revoked-at equals cleanup time |
| Last cached sighting | `2026-10-08T16:35:12Z` |

There is exactly one local code with this label and one cached authorization
with this label for property 1. No current access-code row retains that remote
ID. The owner's attribution, exact generation/remote-creation time, exact
validity, unique name, and PIN-reveal audit strongly correlate the remote object
to the recorded PMS generation. The original create response and pre-cleanup
external-ID field were not recovered; distinguish this correlation from a
retained direct-ID creation record.

### 2.1 Live confirmation

At `2026-10-09T14:20:36.578Z`, both requests returned HTTP 200:

- `GET /smartlock/18233733061/auth` — 17 authorizations, including this code.
- `GET /smartlock/18233733061/auth/6ab2be33a7fc3019addfa5d1` — this exact object.

Selected fields (PIN deliberately excluded):

```json
{
  "id": "6ab2be33a7fc3019addfa5d1",
  "smartlockId": 18233733061,
  "authId": 8200,
  "type": 13,
  "name": "Booking-Amarildo",
  "enabled": true,
  "allowedFromDate": "2026-09-22T12:00:00.000Z",
  "allowedUntilDate": "2026-09-23T07:00:00.000Z",
  "creationDate": "2026-09-22T17:43:15.000Z",
  "updateDate": "2026-10-08T09:42:23.770Z",
  "lockCount": 2,
  "lastActiveDate": "2026-09-22T20:13:12.000Z"
}
```

The code remains a remote object, but its validity has expired. `enabled=true`
alone is not evidence that it currently permits entry. No unlock was attempted.
The remote update timestamp does not identify who or what updated the object.

### 2.2 Why “External” is deterministic

1. `nuki/service.go`, `CleanupExpiredCodes` (492–527):
   - only calls DELETE if an external ID is present;
   - ignores the DELETE error;
   - unconditionally sets `status=revoked`, clears external ID and PIN fields,
     clears errors, sets revoked-at, and ignores persistence/logging errors;
   - logs “expired code moved to historical” and returns nil.
2. `store/nuki.go`, `ListNukiCodesForCleanup` (397–404) selects only
   `status='generated' AND valid_until < now`. Row 238 is no longer eligible.
3. `ListNukiKeypadCodes` (584–630) computes `pms_linked` only from local
   **generated** rows. A revoked row cannot establish linkage, even if it had
   retained an exact ID.
4. `nuki/service.go`, `bindAccessCodesToKeypadRows` (954–986), skips revoked
   rows, so a later sync does not restore the lost link.
5. `frontend/src/views/nuki/NukiCodeTable.vue:46–65` renders false linkage as
   **External**, hides Delete, and displays Read-only.

This proves the ownership-loss/no-retry mechanism. The evidence does not tell
whether September 25's deletion was skipped for missing ID, returned an error,
or was accepted asynchronously without ultimately removing the authorization.
All three are mishandled by the current implementation.

The current cleanup test `TestCleanupExpiredCodes_MovesToRevoked` checks only
the local revoked result; it does not prove remote removal or retry behavior.

### 2.3 Related source defects, not proven historical triggers

- `ListKeypadCodes` probes four endpoints/variants and merges any successful
  subset. It can return partial data as success, or fail a legitimate empty
  result because an optional probe failed. `SyncProperty` subsequently deletes
  missing cache rows and marks missing local codes revoked, clearing IDs.
- The SQL `nukiLabelWindowLinkPredicate` permits equal `Booking-*` labels
  regardless of validity windows. Go matching uses overlapping windows or a
  unique same-name fallback with missing windows. Neither is ownership proof.
- `isExternalLinkUsable` treats a changed label/window as an unusable ID;
  generation may discard that ID and create another remote code.
- Cached name comparisons reveal other possible leftovers, but also repeated
  guest labels across stays. They are investigation candidates, not a deletion
  list. In particular, a current `Booking-Jan` cache row matches the labels of
  two different historical local rows. A blanket name-based recovery is unsafe.
- Manual delete clears identity via `MarkNukiAccessCodesDeletedByExternalID`;
  cancellation and manual revoke also treat accepted DELETE as completion.
  Correcting only the scheduled-cleanup method leaves inconsistent paths.

## 3. Verified Nuki API contract

Source: <https://api.nuki.io/static/swagger/swagger.json>, retrieved during the
investigation. This is the Web API, not the Bridge/Bluetooth API.

| Operation | Documented contract | Current problem |
| --- | --- | --- |
| Log list | RFC3339 `fromDate`/`toDate`, maximum 50, `id` cursor | Unix dates, 500 default, offset paging, hidden partial failures |
| Authorization list | `GET /smartlock/{smartlockId}/auth`, array; optional `types`, `includeEmail` | Speculative enabled variants and undocumented `/auth/advanced` fallback union |
| Create authorization | `PUT /smartlock/{smartlockId}/auth`, asynchronous, **204 without body** | Code expects a response object but treats an empty ID as generated success |
| Update authorization | **POST** `/smartlock/{smartlockId}/auth/{id}`, asynchronous, 204 | `UpdateAccess` uses PUT; toggle probes PUT/POST/PATCH |
| Delete authorization | DELETE same identity URL, **asynchronous**, 204 | Accepted request treated as confirmed deletion |
| Create/update payload | `code` and `accountUserId` are integers; update requires `name` | Code and discovered account-user ID are serialized as strings; toggle can omit required name |

Create's empty-ID path is deterministic with the documented 204 response:
`client.do` skips JSON decoding; `CreateAccess` returns an empty ID;
`generateCodesInternal` persists `generated` and the locally chosen PIN anyway.
Subsequent name/window matching attempts to repair identity opportunistically.
This is a proven contract/state-handling defect, not proof of Amarildo's exact
historical response.

## 4. Required design invariants

1. **Ownership is durable provenance**, independent of validity, enabled state,
   provider visibility, current stay status, and deletion state.
2. A remote identity is scoped by **property + configured smartlock + remote
   authorization ID**. Numeric device `authId` is not interchangeable with the
   Web API object's string `id` used in authorization URLs.
3. A remote ID is never discarded because an operation failed, a list omitted
   it, a label/window changed, or a credential expired. Historical identities
   survive later explicit creation of another credential for the same stay.
4. Automatic deletion applies only to verified PMS-owned identities. Names,
   `Booking-` prefixes, accountUserId, enabled status, and overlapping dates do
   not independently establish ownership. External codes remain read-only.
5. Provider acceptance, confirmed remote state, and local persistence success
   are separate outcomes. Pending/failed operations remain visible and retryable.
6. Read/list refresh never creates credentials or adopts unrelated remote codes.
7. Preserve current salary rules, PIN values on updates, property timezone
   validity rules, single-stay creation, permissions, and unaffected stays.

## 5. Persistence and ownership correction

Introduce durable managed-credential lifecycle storage; do not try to represent
history solely by the existing one-row-per-stay `nuki_access_codes` projection.
Use a new migration, not edits to applied migrations. A suitable model is one
`nuki_managed_credentials` row per credential creation intent/remote identity,
with the following contract (physical naming may follow repository conventions):

- local ID, property ID, named-stay/access-code association, bound smartlock ID;
- nullable remote string ID until resolved, unique per property/smartlock/ID;
- provenance: direct provider identity, correlated creation intent, validated
  legacy link, or evidence-backed incident recovery; retain supporting reference;
- desired label/window/enabled state, requested-operation revision, observed
  remote state and observation time;
- operation state: `create_pending`, `active`, `update_pending`, `delete_pending`,
  `deleted`, or `needs_review`; latest error/attempt time and next retry time;
- an encrypted pending creation PIN/correlation value while needed; never a
  plaintext PIN in logs, evidence JSON, audit payloads or ordinary list DTOs;
- timestamps distinguishing request acceptance from observed completion.

Persist ownership/intent before remote work, commit short local transactions,
and use persisted operation claims/revisions to serialize overlapping mutation
work for the same credential across requests and scheduler instances. Do not
hold a SQLite write transaction open across HTTP. A process-local mutex alone
does not prevent multi-instance duplicate creates or stale cleanup decisions.
Preserve property-scoped foreign keys and prevent conflicting ownership claims.

`nuki_access_codes` can remain the existing stay-facing projection with its
three-state CHECK constraint. Do not overload `revoked` to mean “request sent.”
Add operation-state projections so an unresolved creation is not offered as a
fresh Generate and pending deletion is not offered as usable guest access.
Update all readers atomically/consistently with the lifecycle row, including
messages, PIN reveal, upcoming stays and named-stay generation badges.

Migration rules:

- Preserve exact existing IDs and history. Validate legacy links before enabling
  automatic destructive operations because the old linker could guess by name.
  Use retained full PIN in memory plus type/window/device where available, or
  retained direct identity evidence. Conflicting/unverifiable links become
  `needs_review`, retaining their PMS association rather than becoming External.
- Do not populate missing IDs by generic label joins. Missing-ID historical
  records remain unresolved unless supported by specific evidence (section 9).
- A new explicit creation must not overwrite the sole record of an earlier
  credential. Unresolved prior creation/deletion blocks duplicate creation until
  resolved; confirmed deletion permits a new explicit Nuki Access creation.
- Retaining an ID is not permission to display/reuse its PIN after deletion.
  Purge obsolete secret material after confirmed deletion but retain provenance.
- Guest-entry attribution already intends to include generated and revoked
  credentials (`store/nuki_guest_logs.go`). Adapt it to retained identities
  without reviving guest access or deleting previously collected analytics.
- Changing property smartlock configuration must not send operations for an
  old identity to the new smartlock; suspend mismatched pending work for review.

## 6. HTTP, inventory and asynchronous operations

### 6.1 Correct API calls and snapshots

- Implement typed status-aware HTTP errors; callers must distinguish accepted
  204, missing object, authorization errors, rate limiting, provider failure and
  invalid/oversized JSON. Do not identify status by matching error strings.
- Use POST for updates/toggles and the documented create/delete methods. Send
  numeric PIN/account-user fields where required. For keypad creation the account
  user is optional; stop selecting an arbitrary account user from another cached
  authorization. Preserve a known association during updates when appropriate.
- Build operation-specific payloads. Preserve PIN on maintenance; do not resend
  a different/generated PIN, unknown fields, or unrelated cached authorization
  data. Toggles must obtain the required current name before POST; if unavailable,
  report an error rather than sending an incomplete body. Validate the documented
  32-character authorization-name limit without silently renaming the stay.
- Replace the four-probe inventory union with the canonical documented GET
  array endpoint. The retrieved contract does not document auth-list pagination;
  do not transplant log pagination onto this endpoint. Preserve the inventory
  needed by current consumers and apply keypad display filtering deliberately.
- Distinguish a successfully decoded empty array from a failed/partial fetch.
  Validate every required identity and array completion before using a snapshot
  for absence decisions. The current 1 MiB body limit must report excess/truncation
  explicitly, never supply a partial successful inventory.
- Apply successful cache refreshes transactionally. On fetch/validation/storage
  failure, do not delete cache rows, clear IDs or declare remote absence. Return
  the failure; `SyncProperty` and its API must not hide a partial result as success.
- Even a complete snapshot only describes observed inventory. Missing entries
  do not erase provenance or pending intents; newly accepted creates may not
  yet be visible. Reject stale snapshot application using operation revisions
  or equivalent persisted coordination with mutations.

### 6.2 Creation and update confirmation

1. On an explicit Generate, persist a unique creation intent and encrypted
   chosen PIN before sending PUT. Prevent concurrent/repeated requests from
   sending another create for the same unresolved intent.
2. Treat 204 as accepted/pending. Resolve the real remote ID with bounded
   read-after-write polling or subsequent reconciliation. Correlation must use
   the persisted intent's full PIN, keypad type, device, and exact requested
   name/window, with exactly one matching remote object and no competing owner.
   Never correlate on a masked PIN, name alone, or merely overlapping windows.
3. If visibility is delayed, preserve pending state. A timeout after dispatch is
   an unknown remote outcome, not proof of no creation; reconcile first instead
   of automatically repeating a non-idempotent create. Zero/ambiguous matches
   remain pending/review without adopting or deleting anything.
4. Persist the identity and confirmation before exposing generated success or
   usable PIN in messages/reveal. Crash recovery resumes the persisted intent.
   A failed local commit after remote success must not cause a second create.
5. Update a verified identity using POST and preserve its PIN/remote ID. Confirm
   the requested mutable fields using GET; acceptance alone leaves it pending.
   A name/date discrepancy does not invalidate ownership. A missing identity
   never triggers automatic replacement. Respect newer stay/operation revisions.

Provide a bounded scheduled reconciler for pending lifecycle operations: they
must progress after the browser closes or the server restarts. It may resolve
an already requested creation but must never start a new creation on its own.
Backoff on transient failures/429, honor Retry-After when provided, retain errors,
and use existing scheduler cancellation/lease conventions. An operator should
see actionable `needs_review` rather than endless silent retries for ambiguity
or persistent invalid credentials.

### 6.3 Expiry and revocation confirmation

Route scheduled expiry, cancellation/no-show, manual revoke and managed keypad
delete through the same durable deletion transition:

1. Recheck current eligibility, verified ownership, smartlock binding and operation
   revision before dispatch. Do not delete based on a stale expiry snapshot after
   a stay extension. Once deletion is dispatched, later changes must not pretend
   to undo it or automatically recreate access.
2. Persist `delete_pending` and reason, retaining remote identity. Missing ID is
   unresolved work, not successful deletion. It requires intent resolution or
   evidence-backed recovery, never an arbitrary name-based DELETE.
3. Submit DELETE for the exact owned identity. On errors retain pending state,
   identity and error; schedule retry. Do not write a successful cleanup event.
4. A 204 means accepted. Confirm absence using a fresh complete authoritative
   inventory or a supported, unambiguous exact-object not-found response. Nuki's
   inspected Swagger does not explicitly document 404 here: validate its meaning
   before relying on it. Do not treat 401/403, malformed responses, wrong-device
   errors, or generic gateway 404s as proof of removal.
5. While remote GET still returns the object, it remains PMS-owned and pending.
   Refresh cannot reclassify it as External or change it back to active. Check
   again before retrying DELETE; use bounded backoff rather than rapid resubmits.
6. Only after confirmed absence commit terminal state, clear usable PIN material,
   update projections/cache, and record completion. Keep the historical ID and
   provenance. If local commit fails, subsequent reconciliation must converge
   without creating a code or losing the retry target.
7. Cleanup selection includes pending deletions/retries, not only locally
   generated expired rows. Continue unrelated credentials on an individual
   failure while returning/persisting a truthful aggregate failure/partial result.
   Scheduler metrics must not report an all-success outcome after hidden errors.

After a previously confirmed-deleted owned ID reappears, retain provenance,
surface the discrepancy, and re-evaluate policy. Never silently reclassify it
as External or blindly reactivate it.

## 7. PMS API, UI and ownership enforcement

- `pms_linked` becomes an exact durable-ownership projection independent of the
  old `generated` filter. Expose a separate operation state/error. Retain **PMS**
  source for pending/failed cleanup; display “Deletion pending” or its failure.
- External codes remain read-only. Enforce ownership in mutation services/API,
  not just the Vue Delete button. Current delete/toggle handlers check property
  permission but not code ownership. Reject unowned identities before remote
  mutation; apply the same rule to every alternative revoke/update route.
- Treat migrated ambiguous links as PMS-associated/review-required, not verified
  deletion targets. Do not enable destructive actions merely by setting a badge.
- For mutations still pending after bounded confirmation, return HTTP 202 with
  an operation ID/state; reserve current terminal success responses for confirmed
  completion. Add operation state to existing list projections for polling.
  Update `spec/openapi.yaml`, response DTOs, frontend types and all callers
  together. Display accepted/pending separately from completed; do not immediately
  reveal, claim deletion, or offer another Generate on a pending response.
- Automatic stay updates should retain the saved stay if remote maintenance is
  pending/failed, surfacing lifecycle state. Preserve PMS 23 behavior for missing
  access; unrelated workflows cannot compensate by creating a credential.
- Audit requested/accepted/confirmed/failed transitions accurately. Persist
  property, credential/operation identity, reason, attempt time and sanitized
  error. A success audit cannot depend solely on 204 or a cache update. Never
  store plaintext PINs/tokens in operation events or error messages.

## 8. Log retrieval: evidence, correction and regression requirements

### 8.1 Proven contract defects

The documented `GET /smartlock/{smartlockId}/log` endpoint requires RFC3339
date bounds, supports a maximum page size of 50, and uses `id` to request older
events. The current implementation in `backend/internal/nuki/client.go:187–303`
uses Unix seconds, defaults to 500, and sends `offset=page*limit`. It also probes
`dateSince` and date-unbounded variants, and can return success after later-page
errors or page-budget exhaustion.

Read-only live observations on 2026-10-09:

- `fromDate=1788213600` returns HTTP 400:
  `The supplied value '1788213600' for parameter 'fromDate' is not valid`.
- Queries for the configured cleaner authorization with
  `dateSince=1788213600&limit=500` and with `limit=500` alone each return 50
  events spanning October 9 through August 6. The former does not enforce the
  requested September lower bound.
- Adding `offset=500` returns the same 50-row payload as the initial page.
  Both normalized JSON payload SHA-256 hashes were
  `2210ad94d5c9fde2752c3dd9769b93dbd2bb3b2f81f480a705beb0fcdd211a0f`.
- With documented bounds `fromDate=2026-08-31T22:00:00Z`,
  `toDate=2026-09-30T22:00:00Z`, `limit=50`, and no auth filter, cursor paging
  using the last raw event's `id` returned pages of **50, 50, 34, 0**:
  **134 distinct events**. Exhaustion was confirmed at
  `2026-10-09T14:14:12.501Z`.

With the default 500, the current `len(out) < limit` test mistakes a full
server page of 50 for exhaustion. Increasing the page budget alone cannot fix
this. The current `client_test.go` covers keypad-list merging, not log paging.

### 8.2 Callers and behavior to preserve

- `ReconcileCleanerDailyLogs` uses a 45-day lookback;
  `ReconcileCleanerDailyLogsSince` selects the earliest matching entry per
  property-local day (`backend/internal/nuki/service.go:530–600`).
- `ReconcileGuestDailyEntriesSince` shares `ListSmartlockEvents`, with an empty
  auth filter (`service.go:636–723`). Both callers return on fetch errors before
  writing daily entries; preserve that boundary.
- The cleaning month-reconcile handler passes local month start as UTC
  (`backend/internal/api/cleaning_handlers.go:342–384`). The existing interface
  is “since”, not a closed-month interval, so it can include later dates.
- Preserve property isolation, timezone bucketing, daily uniqueness, earliest
  entry selection, fee history and salary adjustments. Do not change event
  eligibility or auth/name matching as part of fetching corrections. The current
  classifier accepts some non-entry actions and ignores completion state; that
  separate business-rule issue must not be silently redesigned here.

### 8.3 Required implementation

1. Send `fromDate` as UTC RFC3339 with sufficient precision to avoid dropping
   boundary events. Retain local lower-bound filtering as a defense. Preserve
   optional `authId` behavior, including empty auth for guest reconciliation.
2. Set default/effective page size to 50. Clamp larger positive configured
   values to 50 and honor smaller positive values. Align `NewClient`, direct
   client defaults, `backend/internal/config/config.go`, configuration examples
   and docs. Existing deployments configured with 500 must work correctly.
3. Use `id=<last raw row ID>` for subsequent pages. Derive advancement from raw
   responses, not only parsed, retained, or auth-matched events. Irrelevant
   events must not stop an otherwise progressing traversal.
4. Deduplicate by stable event identity. Handle overlapping boundary rows and
   equal timestamps without losing distinct events. Do not use timestamps as
   the pagination cursor.
5. Freeze an upper bound per traversal using documented `toDate`. Keep it
   internal to the existing “since” interface; do not silently make the month
   endpoint exclude later months.
6. Continue nonempty pages until an empty response proves exhaustion. An
   unexpectedly short page is not sufficient proof. Honor page budget, timeout
   and cancellation. Repeated/non-progressing cursors, missing required cursor
   IDs, or budget exhaustion before completion must return an incomplete-fetch
   error. Deduplication must not turn repeated pages into successful completion.
7. Propagate errors from every page: HTTP 400/401/403/429/5xx, transport errors,
   malformed JSON and unreadable required timestamps cannot become apparently
   complete data. Use the shared typed HTTP errors, bounded/context-aware retry
   policy, and secret-safe diagnostics described in section 6.
8. Remove speculative Unix/dateSince/no-parameter success fallbacks for the
   documented Web API. Do not hide contract/auth/rate-limit errors by weakening
   filters. Any nonstandard-server compatibility needs demonstrated contract
   evidence and dedicated tests rather than assumptions from existing comments.
9. Return no usable partial result on incomplete fetching. Cleaner and guest
   services must not write daily rows after a fetch error. Preserve the cleaning
   reconcile endpoint's existing error envelope; it must not report `ok=true`
   or write a success audit for failed fetching. Verify frontend error display.
10. Never delete previously collected attendance merely because a later query
    lacks events. Upstream retention must not erase collected history. Apply
    log pagination only to log endpoints, not authorization inventory.

### 8.4 Required log-fetch tests

Use offline HTTP fixtures and isolated databases:

1. Fake Nuki rejects Unix dates, caps pages at 50, ignores offset and honors
   ID cursors. Retrieve more than 50 records completely; assert RFC3339 bounds,
   clamping, cursor advancement and both populated/empty auth filters.
2. Empty history, multiple full pages followed by empty, smaller configured
   limits, and a short nonempty page followed by more data terminate correctly.
3. Equal timestamps across boundaries and an overlapping row retain every
   distinct event once. Pages without relevant cleaner events still advance.
   Repeated pages/cursors fail without looping or claiming complete coverage.
4. Page-one/later-page failures, malformed JSON, unreadable timestamps, missing
   IDs, cancellation and inadequate budget return errors. Seed existing daily
   entries and verify no writes after incomplete fetch for both service callers.
5. Put a successful synthetic cleaner event on a later page with unrelated
   guest events and repeated cleaner entries. Assert one row per local day,
   earliest timestamp, and idempotence after a second reconcile. Cover
   Europe/Bratislava month and DST boundaries. Mark synthetic fixtures as such.
6. No matching events must not manufacture attendance. Verify that successful
   repeated reconciliation preserves adjustments and other-property records.
7. Cover the cleaning reconcile API error envelope and absence of success audit
   on fetch failure. Verify that existing records survive upstream retention.

Run these alongside the authorization lifecycle checks in section 10; both
features share the HTTP client, so neither may regress the other.

## 9. Evidence-backed recovery of Booking-Amarildo

Fixing future cleanup does not restore IDs already erased. Specify an idempotent
repair operation with a read-only preview and an explicit, narrow manifest for
this incident. It must not be a migration that deletes remote authorizations.

Allowed candidate tuple from section 2:

- property `1`, smartlock `18233733061`, stay `293`, local code `238`;
- remote ID `6ab2be33a7fc3019addfa5d1`, type `13`, label `Booking-Amarildo`;
- exact remote creation timestamp `2026-09-22T17:43:15.000Z`;
- exact validity `2026-09-22T12:00:00Z` → `2026-09-23T07:00:00Z`;
- local generation/reveal audits `6287`/`6289`, cleanup log `50`;
- owner identified this name as PMS-created, supported by the unique correlation.

Re-read authoritative remote and local data before applying. Require this
tuple, uniqueness and absence of a conflicting owner; stop on mismatch or
changed applicability. Record provenance as **evidence-backed incident recovery**,
not as an original create-response ID. Restore durable ownership and enqueue
normal expiry deletion. Do not generate, extend validity, or enable access.
If already absent, record that observation and recover provenance idempotently
without issuing an unrelated DELETE. Preserve the original cleanup log and
append corrective evidence rather than rewriting the historical record.

Other leftover-looking codes need their own identity evidence. Do not use
`WHERE name LIKE 'Booking-%'`, the list of same-name candidates, or the fact
that a row is expired as an ownership/deletion rule. A name/window match can
suggest review, but cannot authorize bulk adoption or deletion.

The current task performs none of these repair/deletion operations. Their
execution belongs to a subsequent implementation/operational request.

## 10. Implementation sequence and regression requirements

### 10.1 Sequence

1. Add failing offline reproductions for cleanup errors, async delayed deletion,
   ID retention and the External badge. Add Web API contract fixtures.
2. Add lifecycle/provenance migration and store operations, including migration
   tests. Implement exact ownership projections before enabling repair/deletion.
3. Correct client contracts, authoritative inventory, async intent resolution and
   pending-operation reconciliation. Migrate all mutation/maintenance callers.
4. Update API/UI pending/error handling and backend ownership enforcement.
5. Implement and test section 8's log-fetch correction with shared HTTP behavior.
6. Implement the scoped recovery preview/operation; rehearse with an isolated
   database copy and mocked provider. Run repository checks and review the full
   change before any live repair.

### 10.2 Required tests

| Area | Required regression |
| --- | --- |
| Cleanup failure | DELETE timeout/423/429/500 retains owned ID, pending/error state, retry selection; no success audit |
| Async deletion | 204 then several GETs still present: stays PMS/pending; later absence completes; rerun is idempotent |
| Missing ID | No remote DELETE, no false revoked completion; pending/review survives restart |
| Persistence failure | Remote mutation accepted but DB confirmation fails: retry converges with same identity, no new create |
| Create 204 | Empty response, delayed listing, strong full-PIN correlation resolves one intent to one ID |
| Unknown create result | Transport timeout/concurrent Generate/restart does not issue duplicate create; ambiguous matches remain unresolved |
| Maintenance | Uses POST, typed payload, same PIN and ID; requires name for toggle; delayed confirmation and errors surfaced |
| Ownership | Expiry/revoke/name change/stay change does not relabel as External; same-name external codes are not adopted |
| Historical identity | Revoke then explicit new creation preserves old ID and old guest attribution without exposing revoked PIN |
| Authorization | Direct delete/toggle/update requests for external or other-property IDs make zero provider mutation calls |
| Inventory | Successful empty array distinct from failure; failed/oversized/malformed snapshot leaves cache and ownership intact |
| Concurrency | Cleanup vs stay extension, stale inventory vs pending create, multiple workers vs one intent; old revision cannot overwrite newer state |
| Lock changes | A changed property smartlock cannot redirect old pending operations to a different device |
| Incident recovery | Exact Amarildo manifest restores ownership; wrong ID/time/window/type/property or conflict stops; second run is harmless |
| UI/API | Pending creation disables duplicate Generate/reveal; pending deletion remains PMS; external stays read-only; errors not shown as success |
| Logs | Section 8 HTTP pagination, incomplete-fetch and cleaner/guest integration regressions |

Fixtures must reproduce real documented 204/no-body behavior instead of always
returning a convenient synchronous ID. Tests must not call live Nuki mutations.
Masking/secrets regressions must cover normal list DTOs and error/audit payloads.
Retain existing single-stay generation and no-automatic-recreation tests.

From `backend`, start with:

```sh
go test ./internal/nuki ./internal/store ./internal/api ./internal/config ./internal/migrate
```

Run relevant frontend component/API tests and the required backend/frontend
checks from repository conventions, inspecting actual package scripts before
choosing commands. Update obsolete tests that encode unsafe name-based adoption;
do not preserve those expectations merely to keep old tests green.

### 10.3 Acceptance

- Booking-Amarildo is recognized as PMS-owned after the scoped recovery;
  deletion remains visible/retryable until authoritative remote absence is
  confirmed. The remote object is removed through the common lifecycle, not
  hidden by deleting a local cache row.
- All future confirmed owned credentials retain their provenance after expiry
  and failed/pending deletion. Genuine external credentials are untouched.
- No duplicate creations, unintended PIN/window changes, cross-property/lock
  mutations, or lost historical identities occur in the regression scenarios.
- Inventory and log fetches use documented contracts and report incomplete work.
- PMS 23's creation policy and existing attendance/salary business rules remain
  intact.

## 11. Read-only evidence queries

```sql
SELECT id, property_id, named_stay_id, code_label, external_nuki_id,
       valid_from, valid_until, status, error_message, created_at,
       updated_at, revoked_at
FROM nuki_access_codes WHERE property_id = 1 AND id = 238;

SELECT id, property_id, external_nuki_id, name, valid_from, valid_until,
       enabled, last_seen_at, created_at
FROM nuki_keypad_codes
WHERE property_id = 1 AND external_nuki_id = '6ab2be33a7fc3019addfa5d1';

SELECT id, event_type, message, created_at
FROM nuki_event_logs WHERE property_id = 1 AND nuki_access_code_id = 238;

SELECT id, action, entity_type, entity_id, outcome, created_at
FROM api_audit_logs WHERE id IN (6287, 6288, 6289);
```

Execute against SQLite with `-readonly`. Use section 2.1's authenticated GETs
for remote verification, selecting nonsecret fields only. Read results may
change after future operations; record retrieval time. The historical DELETE
HTTP response remains unavailable, and this spec makes no claim to know it.
