# PMS 35 — Nuki pending-creation fix and complete PMS 34 implementation audit

## 1. Assignment and evidence boundary

**This deliverable is analysis and a fix specification only. Application code has not been changed.**

Investigated on 2026-10-10 at local HEAD `73ca2be` (following `cf9114e`, `FIX-34: correct Nuki API and credential lifecycle`). Sources: current backend/frontend code, PMS 34, and selected nonsecret fields from `data/pms.db` opened with SQLite `-readonly`. No live Nuki requests, database writes, PIN decryption, or tests were performed in this investigation.

The owner confirmed:

- The message appeared on the first Generate click.
- The affected attempt is **Booking-Samuel**, stay **309**.
- The supplied local database includes the attempt.
- They do not see the code in Nuki currently.

The database nevertheless contains a cached remote authorization matching the label, exact validity window, smartlock, keypad type, and creation second. These are historical cache observations, not a live check or full-PIN identity verification. Do not assume current remote presence, lock synchronization, or successful physical keypad access. The historical HTTP status/body was not retained; the local accepted timestamp is not evidence of the exact HTTP status.

Read `PMS_13_Coding_Conventions.md`, PMS 23's explicit single-stay generation policy, and PMS 34 sections 4–7 and 10 before implementation. This specification completes the missing creation-reconciliation path required by PMS 34. Preserve its durable ownership and no-name-only-adoption requirements.

**Scope expansion requested by the owner:** sections 7–11 audit all PMS 34 requirement groups against the current source and tests and prescribe the remaining work. Sections 1–6 retain the concrete pending-creation investigation. The complete developer assignment is now **sections 4–5 plus sections 8–11**, with PMS 34's detailed contract and invariants retained. Fixing creation alone does not complete the expanded assignment.

## 2. Confirmed incident evidence

| Item | Observed value |
| --- | --- |
| Property / smartlock | `1` / `18233733061` |
| Named stay | `309` |
| Label | `Booking-Samuel` |
| Requested validity | `2026-10-09T12:00:00Z` → `2026-10-10T07:00:00Z` |
| Managed intent | `nuki_managed_credentials.id=1` |
| Intent creation | `2026-10-09T19:59:57.301068604Z` |
| Request accepted timestamp | `2026-10-09T19:59:57.35068526Z` |
| Intent state / revision | `create_pending` / `2` |
| Intent remote ID | NULL |
| Pending PIN | Present, encrypted; value not read or printed |
| Intent last attempt / next retry / completion / latest error | NULL |
| Stay-facing access-code row | `254`, `not_generated`, remote ID NULL |
| Access-code error | `nuki_creation_pending_reconciliation` |
| Generated PIN projection | Absent |
| Generation run | `562`, `generate_one`, `partial`, 1 processed / 1 failed / 0 created |
| Manual sync runs | `563` at `20:00:10Z`, `564` at `20:00:43Z`; each success, 17 processed / 17 updated |
| Cached authorization row | `5392` |
| Cached remote ID | `6ac947bde1965130a070b83c` |
| Cached remote metadata | Same label and exact dates; smartlock `18233733061`, type `13`, enabled |
| Cached remote creation | `2026-10-09T19:59:57.000Z` |
| Cached last seen | `2026-10-09T20:00:43Z` |
| Cached `code` field | Integer present; value not printed or compared |

The Generate time is October 9, **21:59:57 Europe/Bratislava**. The validity ends October 10 at **09:00 Europe/Bratislava**. Recovery must check current time and stay eligibility, rather than assuming this credential is still usable when the fix runs.

## 3. Root cause: a persisted pending state with no resolver

### 3.1 Deterministic real-client path

1. `backend/internal/nuki/client.go:345–369`, `httpClient.CreateAccess`, sends PUT and passes `out=nil` to the HTTP helper. Every successful call returns only the chosen PIN, **never an external ID**. This applies even if a successful response hypothetically contains an ID. PMS 34 records Nuki's documented normal create response as asynchronous 204/no body.
2. `backend/internal/nuki/service.go:297–364` persists an intent with encrypted PIN before dispatch. On successful creation without an ID it retains `create_pending`, records acceptance, writes an access-code projection as `not_generated`, clears revealable PIN fields, marks named-stay generation as error, and returns the reported string.
3. `backend/internal/api/nuki_handlers.go:224–232` converts that return into HTTP 200 `{ok:false,error:...}` and returns **before** the after-generation sync and success audit.
4. Subsequent Generate attempts hit the pending guard (`service.go:269–277` or `407–415`). The guard prevents a duplicate request but performs no reconciliation.
5. `SyncProperty` fetches and stores inventory only. Its `bindAccessCodesToKeypadRows` (`service.go:1097–1110`) has no linking implementation. `findMatchingKeypadEntry` (`1196–1200`) always returns empty, intentionally preventing unsafe metadata-based adoption. There is no replacement intent-based resolver.
6. `backend/internal/store/nuki_managed_credentials.go` can create and update intents but has no pending-work loader/decryption/claim/resolution implementation. Searches for pending PIN and managed-table references found no consumer that resolves accepted creation intents.
7. `backend/cmd/server/main.go:220–293` schedules expiry cleanup and cleaner/guest event reconciliation, not pending authorization creation reconciliation. Cleaner/guest reconciliation is unrelated to this error.

**Conclusion:** for a new stay-facing code using the real HTTP client, accepted creation necessarily takes the unresolved branch. Waiting, restarting, or manual inventory sync cannot advance that intent in this implementation. The database shows this exact non-progress after two manual syncs.

### 3.2 UI makes the missing lifecycle visible as a dead end

- `frontend/src/views/nuki/NukiUpcomingStays.vue:75–99` exposes Reveal only for generated codes and hides Generate for pending/review operation states. Those restrictions are appropriate while identity is unresolved; removing them would not fix the lifecycle.
- `frontend/src/views/NukiView.vue:239–271` treats `ok:false` as failure. Its post-generation refresh/reveal branch executes only for `ok:true`.
- The user sees a technical error rather than an accepted operation with progress, confirmation, or an actionable review state.

### 3.3 Adjacent defects the creation fix must cover

- The existing-code/recreation branch (`service.go:401–455`) can call CreateAccess without creating a durable intent and can label an empty-ID result `generated`. All explicit create branches need one lifecycle implementation.
- Managed-operation writes at `service.go:321,331` discard errors. Confirmation and projection writes are separate, so a persistence failure can leave contradictory records.
- `UpdateNukiManagedOperation` uses `operation_revision <= requested_revision`, not an exact revision claim, and does not check affected-row count. This is insufficient worker coordination.
- The unique pending-stay index also includes `active` rows and excludes `needs_review`; understand this before changing retry/regeneration behavior. Review-required unknown creations must not permit fresh creation merely because the database index allows it.
- Create errors of every kind currently retain pending state. The fix needs explicit classification of definitely-not-dispatched validation failures, explicit provider rejection, accepted requests, and unknown post-dispatch outcomes. An unknown outcome must not be automatically resubmitted.
- Create sends `code` as a JSON string (`client.go:363–365`), despite PMS 34's integer contract. Correct this with an HTTP contract test; it is not established as the cause of this incident.
- `client_test.go` has inventory/log tests but no real HTTP creation lifecycle test. The mock client supplies an immediate ID, hiding the production behavior. `TestGenerateCodes_StatusTransition_NotGeneratedToGeneratedToRevoked` now only checks that retry is blocked; it does not test eventual completion.

## 4. Required implementation

### 4.1 One durable explicit-create workflow

1. Route initial Generate, retry of a previously failed projection, and explicit creation after confirmed deletion through the same creation-intent workflow. Automatic maintenance must never create replacement credentials.
2. Persist one intent and its encrypted chosen PIN before sending PUT; associate the stay-facing projection consistently. Do not replace that PIN or intent on refresh/retry.
3. Use short transactions and persisted claims/revisions to serialize overlapping requests/workers. No SQLite transaction across HTTP. Check affected rows and propagate storage failures.
4. An existing unresolved intent returns that operation's current state and may request a **read-only reconciliation**, never another create. Retained older credentials and review-required unknown outcomes also need explicit eligibility checks.
5. Represent provider acceptance separately from confirmation and actual failure. Return a typed service result containing operation ID/state; reserve an error for actual failure. Do not use a magic error string as the pending transport contract.
6. Treat dispatch timeout, cancellation after possible dispatch, and remote success followed by DB failure as unknown/pending. Reconcile before deciding anything else. Classify provably undispatched/rejected attempts using actual client/provider evidence, retain diagnostics, and support an explicit retry only when a previous remote creation is ruled out.

### 4.2 Implement strong intent-to-authorization reconciliation

Implement a bounded service operation that loads due persisted intents, decrypts their pending PIN in memory, fetches a fresh authoritative inventory, and resolves identity using PMS 34's full correlation contract:

- Bound property and original smartlock, type `13`, **full PIN**, exact canonical requested label, exact requested validity instants (normalize equivalent RFC3339 representations), and relevant requested enabled state.
- Exactly one matching candidate and no conflicting owner. A label, masked PIN suffix, or overlapping dates is insufficient. Validate remote string `id`; device numeric `authId` is not a URL identity substitute.
- Use the intent's original request values, not a subsequently edited stay name/window, for identifying the original operation. Recheck the current stay/revision before making it usable.
- Missing required correlation fields, mismatched device, competing ownership, or multiple matches must not produce a guessed link. Preserve evidence and report a specific pending/review reason.
- Zero matches after a complete successful inventory is **not proof of create failure**, particularly after asynchronous acceptance. Preserve the intent and back off; after a documented bounded policy, expose `needs_review` with useful diagnostics. No automatic replacement PUT.
- Transport failure, rate limit, malformed/oversized inventory, or persistence failure must never be interpreted as zero candidates or successful completion.
- Validate that inventory really is a complete array: the current generic HTTP helper can accept an empty body without decoding, and JSON null can decode as a nil slice. Those cases must not masquerade as an authoritative empty array for lifecycle decisions.

On unique verified completion, atomically update the managed remote identity/observation/completion and the associated access-code projection, clear pending errors, and update named-stay generation state. Make a current eligible confirmed code revealable only after this commit. Preserve encryption conventions and do not expose pending PIN in list DTOs, raw evidence, logs, or audits. Retire the encrypted pending copy only after its durable destination and necessary recovery state are committed.

If the stay is now expired, cancelled, no-show, or otherwise ineligible, resolving ownership must not advertise usable access. Route the verified identity into the appropriate existing revocation/expiry lifecycle, retaining identity and historical association. A failed/deferred deletion must remain observable and retryable; do not claim successful removal on acceptance alone.

### 4.3 Run reconciliation without the browser

- Add bounded read-after-write confirmation when practical, with context-aware budgets. Delayed confirmation returns pending rather than failure.
- Add a scheduled pending-creation worker using existing job-lease/shutdown patterns. Persist attempt time, next retry, attempt/error information and operation revision so restart resumes old rows, including accepted rows with NULL `next_retry_at`.
- Use bounded batches, backoff and Retry-After; persistent ambiguity/authentication/configuration problems become actionable review states. Document retry/budget defaults in code/configuration and tests.
- Manual Nuki sync must invoke or enqueue the same resolver, and report reconciliation progress separately from inventory refresh success. Do not revive the old name-based linker.
- A property smartlock change suspends incompatible pending work for review; do not redirect an old intent to the newly configured device.
- Reconciliation may finish a previously explicit creation but never initiate a new creation. Repeat/restart/concurrent reconciliation must be idempotent.

### 4.4 API and frontend

- Pending generation returns **HTTP 202** with a typed operation ID/state. Confirmed generation returns terminal success; genuine errors remain errors. Define accepted semantics consistently rather than leaving callers to interpret `ok:false`.
- Update handler DTOs, `spec/openapi.yaml`, generated frontend types, `frontend/src/api/types/nuki.ts`, and `NukiView.vue` together.
- Show “Creation pending — waiting for Nuki confirmation” with last attempt/retry or review diagnostics where appropriate. Stop displaying the raw internal error as the only explanation.
- Reload/poll state after accepted generation and on reopening the page; use bounded polling and component cancellation. Backend progress must continue if the page closes.
- Keep duplicate Generate and Reveal unavailable while unresolved. Provide a “Check status” action using reconciliation, not creation, and an actionable review message when automatic correlation cannot finish.
- Only show generated success or auto-reveal after the **confirmed current eligible** state; an access-code row ID alone is not sufficient evidence of completion.
- Audit requested/accepted/confirmed/failed transitions with operation and stay IDs, without PINs/tokens. A successful inventory refresh must not imply a successful generation.

### 4.5 Recover already-stuck records, including this incident

The fix must process existing `create_pending` rows, not only rows it creates after deployment. Prefer the normal reconciliation path; do not ship a name-only SQL patch or a second PUT for Booking-Samuel.

For incident intent `1` / property `1` / stay `309` / access-code `254`:

1. Re-read current intent, stay, lock binding and authoritative remote inventory.
2. Treat cached remote ID `6ac947bde1965130a070b83c` as a candidate. Verify full PIN against decrypted intent, metadata, uniqueness and absence of conflicting ownership in memory. Cached timestamps and owner confirmation do not substitute for that comparison.
3. If the evidence matches, persist the real identity and provenance through the common transaction. Re-evaluate expiry/ineligibility before any usable projection or reveal.
4. If the remote object is absent now, PIN cannot be decrypted, or correlation fails, retain an explicit reviewable unresolved outcome. Record the historical candidate separately from confirmed current remote state. Do not fabricate success, delete the intent, or generate a replacement.
5. Recovery is idempotent and preserves original acceptance time/audits. A worker restart or second manual sync cannot produce an extra remote credential.

Use a new migration if claims/retry/history fields or indexes need changes; do not edit migration 43 in place. Do not hard-code Samuel's identity into the general resolver. Any incident-only tooling must use the same evidence checks.

## 5. Implementation sequence and regression tests

1. Add a failing offline integration reproduction using the **real HTTP client** pointed at `httptest`, an isolated database, and a fake Nuki server: PUT returns 204; list later exposes the chosen PIN and exact requested metadata. Assert one PUT and eventual confirmed identity/projection/reveal eligibility.
2. Implement transactional store operations, exact revision/claim handling and pending-intent loading/decryption; add migrations only if necessary.
3. Unify explicit create paths, implement correlation, and wire immediate/manual/scheduled reconciliation and failure classification.
4. Update API/OpenAPI/UI pending behavior, then test pre-fix database recovery and ineligible-stay handling.

Required cases:

| Scenario | Required result |
| --- | --- |
| 204 then immediate unique match | Confirmed ID and projection, one PUT |
| 204 then several empty lists then match | Pending 202, eventual confirmation, no duplicate PUT |
| Browser close / server restart | Stored intent resumes and finishes |
| Existing incident-shaped row, NULL next retry, cached matching object | Fresh verification resolves it; inventory sync alone cannot claim completion |
| Accepted request but response lost | Reconcile original PIN/intent, never automatically resend |
| Local persistence failure after dispatch or during confirmation | Retained recovery evidence; subsequent pass converges atomically |
| Concurrent Generate/manual sync/two workers | One dispatch/owner; stale revision cannot overwrite newer state |
| Same label/window with different full PIN | No adoption |
| Multiple candidates / competing owner / missing PIN | Actionable unresolved/review state, no guessing |
| Inventory 401/403/429/5xx, malformed/null/empty-body/oversized response | Truthful failure/backoff; no false resolution |
| Code string vs integer JSON payload | PUT sends documented integer code and correct method/path |
| Existing failed/revoked code projection | Same durable intent rules as first creation; no empty-ID generated state |
| Stay edited during pending create | Correlate original request; recheck latest desired state before usable access |
| Expired/cancelled stay, including delayed Samuel recovery | Retain owned identity, no usable PIN/success banner, appropriate lifecycle work |
| Property smartlock changed / cross-property candidate | Review/no cross-lock or cross-property mutation |
| Definite local validation failure vs ambiguous timeout | Different retryability, no permanent unexplained block and no duplicate creation |
| Pending API/UI | 202 operation details, refresh/check status, Generate/Reveal gated; completion updates visibly |
| Secrets | No pending PIN/token in normal API, logs, audits, fixtures derived from production, or diagnostic output |

Extend rather than merely preserve the test that currently asserts retry stays blocked. Keep existing PMS 23 no-automatic-creation tests and ownership/isolation checks. Use synthetic credentials/PINs and mocked provider responses; tests must not mutate live Nuki or the supplied incident database.

Start verification from `backend` with:

```sh
go test ./internal/nuki ./internal/store ./internal/api ./internal/migrate
```

Run relevant frontend tests and repository-required checks after inspecting actual scripts/conventions. Include config/scheduler checks when changing those packages. Report exact checks/results and unresolved evidence gaps; passing synchronous mock tests is not proof the 204 path works.

## 6. Creation-fix completion criteria and developer handoff

The creation fix is complete only when a realistic asynchronous creation progresses from accepted to confirmed (or an actionable unresolved state), survives restart, and never requires a duplicate Generate to advance. Both new intents and the already-persisted incident shape must be covered. Manual sync must help reconcile rather than reporting inventory success while leaving pending work permanently untouched. The expanded PMS 34 completion gate is in section 11.

In the implementation summary provide:

- Root-cause explanation, changed files, state transition/API contract, retry/review policy, and concurrency strategy.
- Tests proving 204/delayed listing/restart and old-record recovery, including counts of provider mutation calls.
- Evidence that no code becomes usable before confirmation and ineligible recovered access is handled correctly.
- A read-only recovery preview for the incident before any separately requested operational application; report match/no-match/ambiguity without disclosing PINs.

Unresolved incident facts: full-PIN equality, present live remote visibility, physical lock synchronization, and original HTTP response status. Ask the owner for additional evidence if needed; do not invent these facts or treat the cached object as proof of current usable access.

## 7. Full PMS 34 source-and-test audit

### 7.1 Method, status definitions, and overall verdict

Audit date: **2026-10-10**, source **`73ca2be`**, unchanged from the incident investigation. Reviewed PMS 34 in full, the Nuki client/service/recovery code, managed storage and migration 43, lifecycle readers, server scheduling, API/OpenAPI, frontend callers, and relevant backend/frontend test source. This is a **static implementation and test-coverage audit**, not a claim that tests have passed or that production recovery has occurred. No application code or database was changed, no live request was made, and no tests were executed for this audit.

Statuses:

- **Implemented:** the named behavior is present in inspected source. Runtime verification is still required.
- **Partial:** some requirements exist, but a named path or condition violates or omits the contract.
- **Missing:** no implementation was found for the required behavior in the inspected call graph/storage/scheduler.
- **Unverified:** historical, operational, or runtime fact cannot be established from this audit.

**Verdict: PMS 34 is not fully implemented.** There are useful contract corrections, but the durable lifecycle is incomplete end to end. Schema columns, pending labels, or tests that assert an intermediate state are not proof of completion. The most consequential gaps are creation resolution, common deletion confirmation, update confirmation, verified ownership/history, concurrency, and truthful API/UI state.

Source abbreviations below (all paths relative to repository root):

| Abbreviation | Path |
| --- | --- |
| CLIENT | `backend/internal/nuki/client.go` |
| SERVICE | `backend/internal/nuki/service.go` |
| MANAGED | `backend/internal/store/nuki_managed_credentials.go` |
| STORE | `backend/internal/store/nuki.go` |
| MIG43 | `backend/internal/migrate/000043_nuki_managed_credentials.up.sql` |
| RECOVERY | `backend/internal/nuki/recovery.go` |
| API | `backend/internal/api/nuki_handlers.go` |
| SERVER | `backend/cmd/server/main.go` |
| VIEW | `frontend/src/views/NukiView.vue` |

Line numbers identify the audited revision; use the function names if implementation moves them. Work-package identifiers **W1–W8** refer to section 9.

### 7.2 PMS 34 sections 1–3: scope, historical evidence, and API contract

PMS 34 sections 1–2 are historical incident evidence, not a checklist of current implementation. Their October 9 live observations and historical DELETE uncertainty were not revalidated. The Samuel cache evidence in section 2 of this document is a separate incident and must not be substituted for Amarildo evidence.

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A01 / §1, §4.6–8 — explicit single-stay creation, preserved maintenance policy | Partial | `GenerateCodeForNamedStay` requires stay/name; SERVICE `MaintainNamedStay` does not create; existing lifecycle tests assert no automatic recreation. However, inconsistent empty-ID recreation and missing confirmations violate the rest of the usable-access contract. Preserve the creation boundary while completing W2–W4. |
| A02 / §3 — create PUT, update POST, delete DELETE | Implemented | CLIENT `CreateAccess`, `UpdateAccess`, `SetAccessEnabled`, `RevokeAccess` use the documented methods. No corresponding mutation HTTP contract fixtures exist. Keep methods and add W7 tests. |
| A03 / §3 — numeric payloads and required name | Partial | `numericField` serializes account-user IDs numerically; Create/Update still send `code` as a string. Toggle rejects absent cached name, but raw-payload handling can replace a validated name with a blank value. W4/W5. |
| A04 / §3 — asynchronous acceptance is not completion | Partial | New-code creation records pending, but cannot resolve it; update/toggle and some deletion paths still complete locally after acceptance. W2–W4. |
| A05 / §3 — canonical authorization inventory | Implemented | CLIENT `ListKeypadCodes:163–178` uses one GET `/smartlock/{id}/auth`; four-probe union is removed. Complete-array/identity validation and cache application remain partial (A24–A27). |
| A06 / §3 — documented log query and paging | Partial | RFC3339, page-size cap, frozen upper bound and ID cursor implemented. Error shape, retry and required regression coverage remain incomplete; see A45–A54. |

### 7.3 PMS 34 sections 4–5: invariants, persistence, ownership, migration

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A07 / §4.1, §5 — durable identity/provenance record | Partial | MIG43 creates the model and MANAGED inserts it. New initial creation and some deletes use it; maintenance, ownership readers, recreation and history do not consistently use it. W1. |
| A08 / §4.2, §5 — property + original smartlock + string remote ID | Partial | Managed unique index contains all three keys. Cache, `IsNukiExternalIDOwned`, linkage and cleanup exclusion use property/ID only; mutation credentials come from current property secrets. CLIENT `rowToKeypadAccessCode:260–267` substitutes `authId`/`smartlockAuthId` for missing `id`. W1/W5 must enforce the correct scope and identity type. |
| A09 / §4.3 — identity survives absence/metadata changes | Partial | `SyncProperty` no longer clears access-code identities on list absence; matching stubs prevent guessed relinking, and a regression checks retained ID. But one-row-per-stay replacement still overwrites the projection and history consumers ignore managed history. W1. |
| A10 / §4.4, §5 migration — only verified owners may be mutated | Partial | Exact-ID checks replace name joins, but exact IDs on old projections are accepted as ownership without validating old heuristic links. Delete/cleanup can create `direct_provider_identity` provenance from these unvalidated rows. No legacy validation/backfill exists. W1/W3. |
| A11 / §5 — complete desired/observed/error/attempt state | Partial | Fields exist. No general observation or retry consumer; last-attempt/next-retry/acceptance/completion fields are not consistently populated. `update_pending`/`needs_review` exist as schema possibilities without a lifecycle driver. W1–W4. |
| A12 / §5 — encrypted pending PIN and secret-safe evidence | Partial | MANAGED encrypts pending PIN on insert and STORE encrypts generated PIN. Pending PIN is not consumed or retired. Raw provider authorization JSON, including `code`, is serialized and persisted directly by CLIENT/STORE. HTTP errors contain unredacted response text. W1/W5 must separate secret correlation from sanitized observations and diagnostics. |
| A13 / §5 — intent before dispatch, short transactions, persisted claims | Partial | Initial creation inserts intent before PUT and its unique index limits concurrent insertion. Other create path bypasses intent. `UpdateNukiManagedOperation` accepts `revision <= requested`; no exact claim/row-count validation or shared transaction for lifecycle + projection. `StartNukiManagedDeletion` uses separate `SELECT changes()` rather than its Exec result. W1/W2. |
| A14 / §5 — property-scoped associations and ownership conflicts | Partial | MIG43 has independent FKs to property/stay/code, not a constraint proving those records share a property; create helper does not validate the association. Remote uniqueness exists in managed storage but projection ownership can conflict independently. W1. |
| A15 / §5 — lifecycle-aware projections/readers | Partial | Upcoming stays query exposes latest operation state/error, but joins it through `nac`: an intent without a successfully persisted access-code row is invisible there. Keypad DTO has no operation fields. Messages/reveal ignore managed state; see A41. W1/W6. |
| A16 / §5 migration — preserve/validate legacy links; unresolved review state | Missing | MIG43 is schema/index creation only. No data backfill, exact-ID evidence validation, ambiguity quarantine, or missing-ID legacy recovery pipeline was found. W1, with migration tests. |
| A17 / §5 — retain old identity through new explicit creation | Partial | Storage can represent multiple historical rows, but recreation bypasses it and ownership/analytics readers still consult only the latest access-code projection. Pending guard ignores `needs_review`; the index also excludes it. W1/W2. |
| A18 / §5 — purge obsolete PIN after confirmed deletion | Partial | `MarkNukiAccessCodesDeletedByExternalID` and expiry cleanup clear projection secrets. `revokeCode` and `revokeNamedStayCode` retain PIN fields. Pending encrypted PIN and raw cached provider PIN are not purged through a common terminal transition. W1/W3. |
| A19 / §5 — historical guest attribution | Partial | `store/nuki_guest_logs.go:99–118` includes any projection row with an ID regardless of status, preserving that current row after revoke. It does not read managed history; replacement loses earlier lookup keys. Event `authId` and Web API `id` are not explicitly separated/mapped for guests. W1; validate provider identity mapping without redesigning event eligibility. |
| A20 / §5 — changed lock suspends old work | Missing | Lifecycle operations use the current configured lock; no comparison to the managed row's original lock precedes mutation. W1/W3/W4. |

### 7.4 PMS 34 section 6.1: HTTP and authoritative snapshots

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A21 / §6.1.1 — typed HTTP outcomes | Partial | `HTTPError` holds status and numeric Retry-After for non-2xx; malformed/oversized/transport errors are generic. Success discards actual status; `HTTPError.Accepted()` cannot describe normal 204 because 204 does not return an HTTPError. Callers do not implement lifecycle-aware classification/retries. W5. |
| A22 / §6.1.2–3 — correct payloads, no arbitrary account-user inference | Partial | Arbitrary account-user discovery is removed. Name length is checked for initial generation and `MaintainNamedStay`. Update payload hard-codes enabled/type/other fields; toggle copies old cached fields and can omit a known association when cache is missing. Correct numeric PIN and field-preservation semantics need HTTP tests. W4/W5. |
| A23 / §6.1.4 — inventory for consumers with deliberate PIN filtering | Partial | Canonical inventory is retained. API `pins_only` admits a row if masked PIN, `Booking-` name, or type 13 exists; name is still a display heuristic, though not a current ownership join. Clarify type-based keypad filtering while preserving cleaner-ID choices. W5. |
| A24 / §6.1.5 — valid complete array and required identities | Partial | Missing parsed identity triggers an error and body >1 MiB is rejected. Empty body/204 and JSON `null` can yield a successful nil slice. Fallback numeric auth IDs satisfy identity validation even when Web API `id` is absent; duplicate/wrong-device identities are not rejected. W5. |
| A25 / §6.1.6 — transactional cache update and truthful failure | Missing | SERVICE `SyncProperty:73–118` upserts one row at a time, continues on storage failures, ignores normalization error, and can still delete missing rows. Returns only `finishRun` error, so a stored `partial` run can be reported to the API as successful. W5. |
| A26 / §6.1.7 — snapshot absence preserves provenance | Partial | Active `SyncProperty` no longer calls the old destructive projection reconciler. `STORE.ReconcileNukiAccessCodesWithKeypad:658–709` still clears IDs if called, but source search found no caller; classify it as a dormant hazardous helper, not the live sync failure. Replace/remove this obsolete helper as part of W1/W5. |
| A27 / §6.1.7 — reject stale snapshots against mutation revisions | Missing | No persisted snapshot/operation coordination around cache application or absence decisions. W1/W5. |

### 7.5 PMS 34 section 6.2: creation/update completion and scheduling

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A28 / §6.2.1 — one persisted explicit creation | Partial | Initial branch persists intent; existing-code creation branch does not. Duplicate pending guard alone cannot recover work. Sections 3–5 and W2. |
| A29 / §6.2.2–3 — 204, full-PIN correlation, delayed/ambiguous results | Missing | No pending-intent correlation implementation. Disabled heuristic linker is not a substitute. Sections 3–5 and W2. |
| A30 / §6.2.4 — confirmation before PIN; crash-safe commit | Partial | Initial empty-ID case hides PIN correctly, but recreation can store generated/usable PIN without ID. No atomic completion/recovery. W1/W2/W6. |
| A31 / §6.2.5 — confirmed POST updates preserving PIN/identity | Partial | POST, retained ID and no new PIN request in ordinary maintenance exist. `MaintainNamedStay` marks local requested values complete on acceptance; no persisted update intent, observed-state GET or revision handling. Generate's update branch can replace PIN from a client response. W4. |
| A32 / §6.2 final paragraphs — scheduled bounded pending worker | Missing | SERVER schedules cleanup and entry-log jobs only. No create/update pending worker, persisted bounded retry/review handling, or Retry-After consumer. W1–W4. |

### 7.6 PMS 34 section 6.3: expiry, cancellation, revoke, deletion

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A33 / §6.3 introduction, step 1 — one lifecycle and fresh eligibility | Missing | Four divergent paths: `CleanupExpiredCodes`, `revokeNamedStayCode`, `revokeCode`, `DeleteKeypadCode`. No common revision/eligibility claim protects cleanup against a concurrent stay extension. W3. |
| A34 / §6.3.2 — persist deletion intent; unresolved IDs remain unresolved | Partial | Cleanup/manual revoke/keypad delete persist an intent if they have an ID. Cancellation does not; `revokeNamedStayCode:193–210` skips remote work when no ID and still marks revoked. Cleanup selection excludes missing IDs entirely. W3. |
| A35 / §6.3.3 — failed DELETE retains retry target/error | Partial | Several paths return errors and retain IDs. Not all persist operation error/next retry; cancellation has no managed intent. Cleanup has an error-handling fall-through detailed in §8.2. W3. |
| A36 / §6.3.4–5 — accepted deletion stays pending until authoritative absence | Partial | Manual revoke and keypad delete make one inventory GET; expiry does too but still writes `revoked` and clears PIN even if the object remains. Cancellation immediately revokes on DELETE acceptance. Keypad delete returns nil while still present, so API falsely reports 204/deleted. W3/W6. |
| A37 / §6.3.6 — atomic terminal commit, secret clearing, retained provenance | Partial | Identity is retained in active revoke/delete/cleanup paths, but managed terminal state and projection commits are separate. Manual revoke retains PIN. No transaction or terminal repair mechanism. W1/W3. |
| A38 / §6.3.7 — pending retry selection and truthful aggregate results | Partial | Cleanup now selects expired `generated` and `revoked` rows with IDs, excluding a managed deleted ID. It does not select all managed pending work, missing-ID work, or future-valid pending cancellation/revoke; does not honor backoff or check absence before resending. SERVER logs per-property failures then records `ran`. W3. |
| A39 / §6.3 final paragraph — confirmed-deleted ID reappears | Missing | Linkage can stay PMS while current projection retains the ID, but there is no discrepancy state/re-evaluation. Cleanup exclusion can suppress future handling; replacement can instead lose the PMS label. W1/W3/W6. |

### 7.7 PMS 34 section 7: API, UI, authorization, audit

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A40 / §7 bullets 1–3 — durable source badge and enforced verified ownership | Partial | `STORE.ListNukiKeypadCodes` recognizes exact projection IDs irrespective of status. Delete/toggle reject IDs absent from projections. Neither uses managed provenance, original lock, or review status; keypad list lacks operation state. W1/W6. |
| A41 / §5 projections, §7.4 — usable access gates on all readers | Missing | API `revealNukiCodePIN:404–412` checks only presence of saved PIN after permission checks, not revoked/pending state. `store/messages.go:483–487` checks generated + PIN only; pending manual delete can leave that state. Upcoming Reveal checks generated only. W6 must centralize eligibility/confirmation gating. |
| A42 / §7.4 — HTTP 202 + operation ID, OpenAPI/callers consistent | Missing | Generate returns HTTP 200 `ok:false` for pending; delete/revoke define only terminal success or failure; toggle returns desired value immediately. OpenAPI has no 202 contracts for these operations. W6. |
| A43 / §7.5 — save stay independently; surface lifecycle outcome | Partial | `occupancy_named_stay_handlers.go:356–373` retains stay and marks maintenance errors. Saved Nuki name endpoint returns success after maintenance error; operation state is not returned there. No pending lifecycle integration. Preserve saving behavior and expose operation outcome in W4/W6. |
| A44 / §7.6 — truthful sanitized transition audit | Partial | Terminal API success audits remain. Generation's pending return produces no requested/accepted event; deletion/toggle can audit success before completion. `HTTPError.Error` propagates provider body text to stored errors/API; there is no demonstrated secret scrubber. W5/W6. |

### 7.8 PMS 34 section 8: log-fetch implementation and preserved behavior

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A45 / §8.3.1 — UTC RFC3339, precision, auth filter | Implemented | CLIENT `ListSmartlockEvents:180–228` uses `RFC3339Nano`, optional `authId`, and local lower-bound filtering. Existing HTTP test only weakly asserts date format and uses empty auth. W7 adds exact assertions and populated auth. |
| A46 / §8.3.2 — limit 50/defaults/clamping/documentation | Partial | Client and config default to 50; request limit caps at 50 and permits smaller positive values. No `NUKI_LOG_FETCH_LIMIT`/`NUKI_LOG_MAX_PAGES` configuration documentation/examples were found outside config source. Add docs and boundary/default tests in W5/W7. |
| A47 / §8.3.3–4 — raw-row ID cursor, dedup, timestamp ties | Implemented | Cursor is last raw row ID; dedup map keyed by event ID; retained filtering does not select the cursor; deterministic equal-time sort. Only one 51-record happy-path HTTP test exists. W7 adds overlap/irrelevant-row/tie cases. |
| A48 / §8.3.5 — frozen upper bound and month semantics | Implemented | One `toDate` per traversal. Cleaning API continues to pass property-local month start as UTC and keeps the since-interface. Preserve this behavior; do not silently impose month-end filtering. |
| A49 / §8.3.6 — empty-page exhaustion, short pages, budgets/cycles | Partial | Nonempty short pages continue; empty page completes; missing IDs, immediate repeated cursor, and page exhaustion return errors. Multi-step cursor cycles are only caught by budget; generic empty-body/null response can falsely count as exhaustion. Add strict array validation, visited-cursor detection and tests. W5/W7. |
| A50 / §8.3.7 — every-page failures, timeout/cancellation, retries | Partial | HTTP/JSON/timestamp errors return nil/error, with request context and per-request timeout. No context-aware retry/backoff or Retry-After use; HTTP-date Retry-After is not parsed. W5. |
| A51 / §8.3.8 — no speculative date/endpoint fallbacks | Implemented | No Unix/dateSince/offset/no-bounds fallback remains in active log fetch. Preserve this correction. |
| A52 / §8.3.9 — no partial writes after incomplete fetch | Implemented | Both cleaner/guest services fetch completely and return on fetch error before daily upserts. Cleaning API returns `ok:false` and skips success audit on error; `CleaningView.vue:284–300` displays that error. Required integration/error tests are absent. W7. |
| A53 / §8.2, §8.3.10 — retain attendance, salary rules, event classification | Partial | No fetch-driven deletion of prior daily rows; 45-day lookback, local-day grouping, cleaner aliases and fallback classifier preserved. Store upserts replace earliest timestamp unconditionally, so a later retained-history subset can move a previously recorded first entry later; retention/earliest invariants need tests and correction where required. Do not change eligibility or salary policy. W5/W7. |
| A54 / §8.4 — required complete log regression suite | Missing | Only CLIENT test `TestListSmartlockEvents_UsesRFC3339AndIDCursor` exercises real paging (51 rows, default/cap). Service fake-event tests cover basic earliest entry and partitioning, not incomplete multi-page errors, rate limits, page budget, retention, or DST. Detailed gaps in §10. |

### 7.9 PMS 34 section 9: Amarildo recovery

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A55 / §9 — narrow manifest and read-only preview | Partial | RECOVERY defines the exact tuple and preview/helper methods, checks local stay/code/label/dates and remote ID/label/dates/type/creation time. No CLI/API/other production caller of these methods was found. No recovery tests were found. W8. |
| A56 / §9 — fresh lock/identity/conflict/evidence validation | Partial | Preview loads current credentials but never asserts configured lock equals manifest lock or verifies returned smartlock field. `IsNukiExternalIDOwned` returns true for any projection owner and is interpreted as `AlreadyRecovered`, without verifying that owner is this manifest or a managed provenance record exists. Manifest does not validate referenced local generation/reveal/cleanup evidence. W8. |
| A57 / §9 — atomic, idempotent repair and already-absent case | Partial | RECOVERY writes projection, then managed row, then audit independently. If managed insert fails, the changed projection causes a subsequent preview to return already recovered. It always creates `delete_pending`, including the remote-absent path that promises no DELETE. W8 must make completion/repair atomic and record confirmed absence without queuing unnecessary remote deletion. |
| A58 / §9 — actual operational recovery/deletion | Unverified | Helper existence is not execution evidence. No live recovery was performed during this audit. Do not claim the historical Amarildo incident is resolved. The future operator workflow must separately report preview/application/remote confirmation. |

### 7.10 PMS 34 sections 10–11: execution, regression, acceptance, evidence

| ID / PMS 34 reference | Status | Current evidence and remaining requirement |
| --- | --- | --- |
| A59 / §10.1 — implementation sequence completed | Partial | Migration, client corrections, UI labels and recovery helper exist, but connected lifecycle and verification phases are incomplete. Commit title is not proof of all steps. Follow §9 dependencies and §11 gates. |
| A60 / §10.2 — full authorization/log regression matrix | Partial | Some basic synchronous service/API tests remain, including ID retention and secret omission. Most async/concurrency/history/recovery requirements have no matching tests. §10 maps every required row. |
| A61 / §10.3 — Amarildo resolution, durable lifecycle and no regressions | Not satisfied / partly unverified | Static defects prevent declaring the behavioral requirements complete. Production repair remains unverified. Require code/test evidence and separate operational outcome, §11. |
| A62 / §11 — read-only evidence discipline | Implemented for this investigation | Incident queries were read-only and selected nonsecret fields. PMS 34 historical live observations were not rerun. Future evidence must record collection time and preserve uncertainty; no inferred HTTP/DELETE outcome. |

## 8. Additional concrete failure paths the implementation must address

These are source-derived defects, not claims that they occurred in the owner's production data.

### 8.1 Cancellation can retain a revealable PIN after reporting revocation

`revokeNamedStayCode` sends DELETE if an ID exists, then immediately persists `revoked` without managed intent, remote confirmation, or PIN clearing. `MaintainNamedStay` returns early for revoked rows thereafter. `revealNukiCodePIN` checks PIN presence but not lifecycle state, so a direct permitted reveal request can still return that saved PIN. An unexpired cancelled credential has no scheduled pending-deletion selection until expiry. Route cancellation into W3 and enforce W6 reader gates.

### 8.2 Expiry has unsafe partial-failure and terminal-state ordering

In SERVICE `CleanupExpiredCodes:541–549`, the condition is `err != nil && firstErr == nil`. Once an earlier row has set `firstErr`, a later failed `StartNukiManagedDeletion` falls through and may still send DELETE without its required durable state. Always stop remote work for that row on persistence failure, while preserving the first/aggregate error.

After successful DELETE + GET, cleanup sets the projection `revoked` even if the object remains. When absence is observed, managed `deleted` is written before the projection update. If projection persistence then fails, the cleanup query's `NOT EXISTS (... operation_state='deleted')` can exclude that still-inconsistent projection from future repair. Solve with a common atomic completion and a reconciliation path for already-observed terminal outcomes.

### 8.3 Manual deletion can report success while access remains exposed

`DeleteKeypadCode` returns nil if GET still contains the object. API then returns 204 and writes a successful delete audit. Its projection can remain `generated` with PIN, making messages/reveal treat it as available. `revokeCode` instead returns a pending string as an error, while its confirmed path leaves PIN material. These routes need the same operation result and state machine, not route-specific interpretations.

### 8.4 Managed history is not the source of ownership yet

`ListNukiKeypadCodes` and `IsNukiExternalIDOwned` query only `nuki_access_codes`. A historical identity existing solely in managed storage is neither displayed nor authorized consistently after the stay-facing projection changes. Conversely, any retained legacy guessed ID is trusted. The guest log identity map has the same projection-only limitation. Make managed verified provenance authoritative and keep a separate PMS-associated/review-required display state.

### 8.5 Recovery can become falsely “already recovered” after a partial write

`RecoverAmarildo` updates the projection first. If the managed insert or later audit fails, preview uses projection existence to shortcut as complete, preventing normal retry from repairing missing provenance/audit. Another stay owning the same remote ID also triggers this shortcut. A transaction and tuple-specific idempotency check are required; “some row has this ID” is not recovery completion.

### 8.6 Metadata and HTTP diagnostics still carry secret values

CLIENT marshals the complete authorization response to `PayloadJSON`; STORE persists it unchanged as `raw_json`. Samuel's cache has a numeric `code` field. `HTTPError` includes up to 900 bytes of raw response body that service/API paths can persist or display. This is a demonstrated data path, not evidence that a particular error leaked a PIN. Sanitize observations/errors while preserving any required full-PIN correlation only in memory or designated encrypted storage. Account for existing cached rows when adopting the sanitized format.

## 9. Complete remaining implementation work packages

All **Partial/Missing** requirements in §7 are in scope. Keep the implemented corrections. The developer must not close this assignment after resolving only Samuel's creation state.

### W1 — Authoritative lifecycle storage, ownership, migration, coordination

Applies to A07–A20, A26–A27 and supports all mutation packages.

1. Make managed credentials the authoritative identity/history store. Define verified ownership separately from associated-but-unverified legacy data. Include property, bound smartlock, remote Web API ID, provenance, stay/code association, desired revision and sanitized observation in all decisions.
2. Add a **new** migration/store migration workflow validating existing links before they authorize automatic mutation. Do not backfill trusted ownership solely from old projection IDs or label matches. Use retained direct evidence or full-PIN/type/device/window checks; ambiguous/unverifiable rows become review-required. Preserve histories and old IDs rather than deleting them. Provider-dependent validation belongs in a resumable service workflow, not network calls inside a SQL migration.
3. Ensure cross-property stay/code associations cannot be stored; validate or constrain them transactionally. Define uniqueness and history rules so confirmed-deleted history coexists with a new explicit credential, but unresolved/review-required prior work cannot silently permit duplicates.
4. Add loaders, encrypted-secret access, exact revision claims, claim expiry/recovery, and atomic transition/projection/event helpers. Use `RowsAffected` on the actual write; reject stale transitions explicitly. A scheduler-wide lease alone does not serialize API requests against workers.
5. Update ownership checks, keypad linkage, cleanup selection, upcoming-state projection and guest attribution to use lifecycle history. Upcoming operation lookup must work when `nuki_access_codes` creation failed. Retain device `authId` aliases separately when validated as necessary for log attribution; never substitute them into mutation URLs.
6. Gate changed smartlock configuration and legacy unknown lock binding through review. Retain historical observation scope so old cache data cannot authorize mutation on a new lock.
7. Remove or make identity-preserving the unused destructive projection reconciler. Implement terminal secret-retention policy for pending encrypted PIN, usable projection PIN and cached raw metadata, retaining only nonsecret provenance/history after deletion.

### W2 — Complete creation and existing pending recovery

Implement sections 4.1–4.3 and 4.5 in full. Use W1 storage/claims, W5 authoritative reads and W6 result semantics. Required behaviors include strong correlation, all explicit create branches, accepted/unknown/rejected distinctions, scheduled progress, restart, and correct ineligible-stay resolution. No generic metadata adoption and no automatic non-idempotent create retry.

### W3 — Common deletion/revocation/expiry state machine

Applies to A18, A20, A33–A39, A41–A44.

1. Replace divergent mutation logic with one service flow used by cancellation/no-show, scheduled expiry, manual revoke and keypad delete. Re-read eligibility/desired revision and verified ownership under a claim before dispatch. A concurrent stay extension must invalidate an undispatched stale expiry claim.
2. Persist `delete_pending`, target identity, reason and attempt before HTTP. Missing identity is unresolved/review work, not `revoked` success. A persistence failure means no DELETE for that credential, regardless of errors on other rows.
3. After dispatch, keep ownership, identity, pending/error/retry status and block usable-access presentation. A later stay change cannot undo a sent deletion or cause automatic recreation.
4. Confirm via a fresh complete authoritative list or independently validated exact-object absence semantics. Do not infer absence from 401/403/gateway 404, wrong lock, empty body or malformed list. The current code does not use exact-object 404; do not introduce it without contract evidence.
5. On retry, observe state before resending DELETE. Persist bounded next retry and honor provider backoff. Select all due managed deletion work even when a stay is unexpired, removed from upcoming listings, or the projection is missing/revoked.
6. Commit confirmed `deleted`, secret removal, projection/cache updates and completion event atomically while retaining historical identity. Recover a failed local commit without resending a create or losing target identity. Handle an already-deleted object reappearing as a discrepancy/reviewable policy decision.
7. Report per-credential errors and aggregate scheduler failure/partial metrics accurately while processing unaffected records. Correct the fall-through and managed-before-projection defects described in §8.2.

### W4 — Confirmed updates/toggles and maintenance

Applies to A03, A22, A31–A32, A43.

1. Persist desired fields and `update_pending` against the same verified credential and operation revision before POST. Serialize with deletion/creation; a stale maintenance operation cannot revive a pending/deleted credential.
2. Use operation-specific payloads containing required current name and intended changes. Preserve PIN, remote identity and unrelated fields. Do not accidentally enable a previously disabled credential through hard-coded update fields or copy stale full cache payloads over newer remote settings. Where existing product behavior is ambiguous, ask the owner rather than redefining toggle/maintenance semantics.
3. Preserve a known account-user association as a numeric field where needed; do not discover an arbitrary one. Validate required nonblank name after payload construction and the documented name length.
4. Confirm requested mutable fields from GET; acceptance remains pending. Persist observations/attempts/errors and let the common worker resume after restart. Missing remote identity never causes automatic replacement.
5. Save local stay changes even when Nuki is pending/failed, returning separate lifecycle state/error and keeping the same credential/PIN. Update all maintenance callers, including saved PIN-name edits and Generate's reuse/update path.

### W5 — Strict HTTP/snapshots, errors, secrets and log completion

Applies to A02–A06, A12, A21–A27, A44–A54.

1. Keep corrected HTTP methods/canonical endpoint and numeric account-user handling. Send PIN as documented JSON integer with validation; add operation-specific response/status handling rather than forcing 204 through an error type.
2. Strictly require array responses for authorization/log lists. Distinguish valid `[]` from null, empty body, unexpected status, truncated/oversized JSON and missing/invalid IDs. Require the Web API string `id` for authorization identity; validate bound device/duplicate IDs where needed for authoritative snapshot decisions. Do not transplant log pagination to inventory.
3. Apply cache refreshes in a short transaction only after a complete valid fetch. Roll back on any row/storage/normalization failure. Coordinate snapshot application with mutation revisions and propagate partial/failure instead of returning `finishRun` success.
4. Add bounded context-aware retry policy for safe reads and operation-specific retries. Parse Retry-After delta-seconds and HTTP-date; honor cancellation/budgets. Do not use a generic retry wrapper to repeat unknown-outcome creates. Retain typed status and sanitized diagnostics, not raw provider body snippets containing secrets.
5. Separate full-PIN correlation data from sanitized JSON observations/cache. Do not copy provider `code` into ordinary evidence or error fields; migrate/sanitize old cache values consistently with future correlation needs.
6. Keep RFC3339Nano bounds, fixed upper traversal bound, raw-row ID cursor, ≤50 limit, full dedup and until-empty traversal. Detect previously visited/nonprogressing cursors, enforce strict response shapes, propagate every-page failures, and return no usable partial result.
7. Preserve cleaner/guest classifiers, aliases, property-local grouping, since-month semantics, salary rates/adjustments and stored attendance. Ensure repeat reconciliation or provider retention cannot replace an earlier already-recorded daily first entry with a later one. Change storage/test expectations only for this retention invariant, not unrelated attendance business policy.
8. Document effective log limit/max pages, backoff and lifecycle-worker defaults; existing deployments with limit 500 must continue to request at most 50.

### W6 — Consistent API, UI, readers, permission and audit contracts

Applies to A15, A30, A36–A44 and extends §4.4 beyond creation.

1. Define a common operation result for create/update/delete: operation identity, state, accepted/confirmed outcome, sanitized error/retry information. Return 202 while pending, terminal success only when confirmed; update OpenAPI and all callers/types together.
2. Expose managed source/provenance assurance and operation status in keypad and stay-facing views, including history and rows whose projection was not written. Pending deletion remains PMS-owned; review-associated is distinguishable from verified ownership and from genuinely External.
3. Apply one usable-credential predicate server-side to PIN reveal, message values/status, stay badges and UI actions. Pending deletion, unconfirmed creation, deleted/revoked and ineligible access must not be offered as usable. Keep existing permissions and audited reveal behavior; do not rely on hidden UI buttons to enforce state.
4. Show accepted/pending/review/error states, bounded polling and check-status actions for all mutation types. Do not show deletion success or generated-success banner merely because HTTP returned normally. Reopen/restart must expose persisted work. Actions for genuine External objects remain read-only and service mutation checks must enforce verified ownership on every route.
5. Write requested/accepted/confirmed/failed/discrepancy events accurately, with property/credential/operation/reason/revision/time and sanitized errors. Keep local stay-save success distinct from integration completion. Ensure scheduler metrics and sync responses reflect partial failures.

### W7 — Requirement-linked tests and final verification

Implement section 5 plus the full section 10 matrix. Use the real HTTP client with offline fake Nuki for asynchronous contract cases, not exclusively the synchronous `fakeClient`. Add controllable delayed visibility, definite rejection, lost response, list errors, conflicts, DB failures, concurrency and restart fixtures.

Every completed audit row must cite implementation locations and regression tests (or an explicit reason it is a documentation/operational requirement). Run package suites and required frontend/OpenAPI checks; report commands and results. Do not edit a test to merely expect permanent pending or premature revoked state. Existing test names can be misleading; inspect actual assertions.

### W8 — Complete evidence-backed Amarildo recovery workflow

Applies to A55–A58. This is separate from Samuel's pending-intent recovery.

1. Provide a documented callable operator entry point for read-only preview and explicit application, using the existing narrow manifest and common lifecycle service. Do not run it automatically in a schema migration.
2. Validate property, original configured/returned lock, exact code/stay association, ID/type/name/window/creation time, referenced local evidence, uniqueness, conflicting ownership and applicability. If evidence is absent or changed, report that and stop rather than claiming original direct-ID provenance.
3. `AlreadyRecovered` requires the exact intended durable association/provenance and consistent terminal/pending projection, not merely any projection containing the ID. Conflicting owners must stop. A partially applied older helper must be repairable idempotently after revalidation.
4. Apply managed provenance, projection/history and corrective audit atomically. Append evidence; do not rewrite old audits/cleanup events. No generated PIN, enable, validity extension, or new creation.
5. If currently present and expired, enqueue common confirmed-deletion workflow. If authoritative absence is confirmed, record that terminal observation/provenance without enqueuing another DELETE. Preserve historical ID in both cases.
6. Add exact tuple/mismatch/conflict/changed-lock/absent/persistence-failure/repeated-run tests and rehearse on synthetic or isolated copied data with provider mutations mocked. Live application remains a separate operational action; report it separately from implementation completion.

Dependency order: **W1 + W5 foundations → W2/W3/W4 → W6 → W8**, with **W7 tests developed alongside every package**. Keep the immediate creation regression failing first, but do not defer shared ownership/transaction fixes that its recovery depends on.

## 10. PMS 34 required-test coverage audit and completion matrix

The following maps every row of PMS 34 §10.2 and every group of §8.4. “Existing” means test source was inspected, not executed in this audit.

### 10.1 Authorization lifecycle (§10.2)

| PMS 34 test row | Existing coverage | Required completion |
| --- | --- | --- |
| Cleanup failure | `fakeClient` can fail revoke, but no cleanup-failure test uses it; cleanup test asserts only revoked projection | Test timeout/423/429/500, retained ID/ownership, error/retry selection and no completion audit; test second-row persistence failure after first-row failure |
| Async deletion | Delete test only checks cached row remains; fake list does not model remote mutation | Real HTTP 204 + repeated presence + eventual absence, truthful 202, restart, idempotence, every deletion caller |
| Missing ID | No complete regression | Zero DELETE, pending/review not revoked, restart and cancellation/expiry coverage |
| Persistence failure | No lifecycle dispatch/confirmation commit-failure test | Remote acceptance then DB failure; atomic repair; no duplicate PUT and no stranded projection after managed deleted |
| Create 204 | No creation HTTP test; fake always returns ID | Real HTTP client, no-body response, strong correlation and delayed visibility |
| Unknown create result | Retry-block test only asserts one fake create and permanent pending | Lost response, restart, simultaneous Generate, unique/zero/ambiguous correlation and eventual progression |
| Maintenance | Synchronous service tests preserve ID/PIN on failed update and do not recreate after reactivation | POST payload contract, no PIN change, pending confirmation, toggle/name checks, same revision, old/new desired states |
| Ownership | Tests keep an unrelated listed ID external and retain original projection ID | Ownership after expiry/failed deletion, managed-only history, unvalidated legacy links, same-name collision and every mutation route |
| Historical identity | No revoke → explicit recreation → old-history regression | Preserve old managed ID/guest attribution; no old PIN reveal; coexistence constraints |
| Authorization | Reveal read-only test exists; no direct external delete/toggle/update service/API contract tests found | Zero provider calls for external/other-property/other-lock/review-required targets on all routes |
| Inventory | One canonical endpoint happy-path test | Valid empty vs null/empty-body/errors, malformed/oversized/invalid-ID/duplicate/device mismatch; transactional storage failure and stale snapshot |
| Concurrency | No managed lifecycle races/claims test | Cleanup vs extension, API vs worker, two workers, stale snapshot/revision, claim expiration/restart |
| Lock changes | No matching test | Bound old identity is never mutated on newly configured device |
| Incident recovery | No RECOVERY tests/callers found | Exact manifest, every mismatch/conflict, partial old application, absent identity, repeat run, no unrelated mutation |
| UI/API | Reveal permission/audit/PIN-omission tests; `NukiView.spec.ts` only empty/load/load-error | 202 operations, all pending transitions, ownership badges, reveal/messages gating, review/check-status, errors versus success |
| Logs | One pagination happy path and synthetic service-event tests | §10.2 below |

### 10.2 Log fetching (§8.4 groups 1–7)

| PMS 34 group | Existing coverage | Required completion |
| --- | --- | --- |
| 1 — contract, >50, cap, filters | `TestListSmartlockEvents_UsesRFC3339AndIDCursor`: 51 rows, request cap 50, no offset, empty auth | Assert actual RFC3339 parse/exact bounds/precision, frozen `toDate`, nonempty auth, defaults/smaller limits; fake rejects Unix and models ignored offset |
| 2 — empty/full/short pages | Same test has 50 + 1 + empty | Separate empty history, multiple full pages, short page followed by more data, configured smaller limit |
| 3 — ties/overlap/irrelevant/nonprogress | No corresponding HTTP regression | Equal timestamps/overlap dedup, raw irrelevant rows advance, immediate and multi-step cursor cycles fail |
| 4 — every failure/no partial writes | No matching HTTP-to-service failure suite | First/later-page statuses, rate limit/retry, malformed/null/empty-body/oversize/unreadable timestamp/missing ID, cancellation/budget; seed existing cleaner+guest rows and assert zero writes on incomplete fetch |
| 5 — later-page cleaner, idempotence, DST | Fake-event tests verify earliest event/basic partition only | Synthetic later-page cleaner amid guests, repeat reconcile, Europe/Bratislava month/DST boundaries |
| 6 — no matches, preserved other data | Guest no-code test is a no-op; store filters tested | No matching events creates no attendance; adjustments/other properties untouched, retention preserves existing earliest events |
| 7 — cleaning API/UI failure and retention | Code returns/displays `ok:false`; no reconcile-error test found in CleaningView suite | End-to-end error envelope/no success audit; UI error instead of success; no history loss under retention |

Additional migration/storage checks required by PMS 34 §5/§10.1: migrate representative pre-43 data through the new migration, validate/review legacy associations, preserve exact IDs/history and encrypted secrets, enforce property association constraints, and recover interrupted validation/claims. Existing migration test `nuki_named_stay_primary_test.go` covers the earlier named-stay cutover, not the managed-credential migration contract.

## 11. Expanded completion gate: prevent another partially implemented spec

The developer must deliver a final checklist for **A01–A62**, with:

1. Implemented source/function references and linked tests for each behavior; keep already-implemented corrections covered.
2. Exact verification commands/results. Start with the backend packages listed in §5 plus `./internal/config`; include scheduler/config and frontend/OpenAPI required checks as affected. No claim that a test passed based only on its presence.
3. All Partial/Missing rows completed, or explicitly listed as unresolved with reason and owner decision. Do not silently omit a row or mark the whole task complete while lifecycle work is deferred.
4. Demonstrated creation, update and deletion progression under documented asynchronous behavior, restarts and persistence failures, with zero unintended creation/identity/PIN changes.
5. Demonstrated authoritative ownership/history and lock isolation across migration, cleanup, replacement, review states and historical analytics.
6. Demonstrated no false terminal success, no usable pending/deleted access in reveal/messages, no unsafe metadata adoption, and no incomplete-fetch attendance writes.
7. Separate reports for **implementation completion**, **Samuel recovery preview/outcome**, and **Amarildo recovery preview/outcome**. Live recovery or deletion cannot be claimed from code/test completion or stale cache alone.

All PMS 34 §10.3 acceptance bullets remain binding: evidence-backed Amarildo recovery/confirmed deletion when operationally applied; durable ownership after failures/expiry; no duplicate/cross-scope mutations or lost history; documented inventory/log contracts and truthful failure; preserved explicit creation and attendance/salary policy. Unknown historical HTTP outcomes remain unknown.

**Current audit disposition:** requirements and remaining work are now documented across the full PMS 34 scope. The source is still incomplete; this specification update implements none of the application fixes.
