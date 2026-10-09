# PMS 32 — Prevent unrelated cleaning-event deletion

## 1. Status and implementation objective

Investigated on 2026-10-09 using source at commit
`fb6dc6606ed9160268155c8524442b24034d0c19` and read-only SQLite queries of the
user-supplied production snapshot, `data/pms.db`.

**Confirmed:** the snapshot records creation of the October 9 cleaning and
deletion of the October 11 cleaning in the same named-stay promotion run. The
source contains a deterministic reconciliation-scope defect explaining this.

**Deliverable:** implementation instructions only. No application code, database
state, or Google Calendar state was changed during this investigation. No new
test was written or executed. The user subsequently confirmed that production
uses the same build as the current local build, both emails arrived at 18:34
CEST on October 8, and the October 11 event is currently missing from Google
Calendar. Subsequently, with user authorization, read-only Google API requests
confirmed cancelled remote state and no active replacement for that identity
(section 6). No independent deployed-binary inspection was collected.

Implement the deletion fix and regression coverage below. Follow
`PMS_13_Coding_Conventions.md`. PMS 22, especially sections 4.7, 4.8 and 7.1,
remains the business authority; this is a correction to its implementation.
Inspect existing working-tree changes before editing.

## 2. Incident evidence

Property `1` uses `Europe/Bratislava`. On October 8, UTC timestamps below are
two hours behind CEST. Guest names and credentials are unnecessary evidence.

### 2.1 Relevant persisted records

| Named stay | Check-in → checkout | Cleaning row | Identity |
| --- | --- | --- | --- |
| 306 | 2026-10-07 → 2026-10-08 | 42686 | `stay:1:306:2026-10-08` |
| 307 | 2026-10-10 → 2026-10-11 | 42687 | `stay:1:307:2026-10-11` |
| 308 | 2026-10-08 → 2026-10-09 | 42712 | `stay:1:308:2026-10-09` |

All three stays are currently active, cleaning-required, confirmed in both
review fields, and have no suppressing outcome. Stay 307 was created at
`2026-10-08T10:43:16Z`, last updated at `10:43:28Z`, before the incident.
Stay 308 was created at `2026-10-08T16:34:54Z`.
Rows 42687 and 42712 have stored windows 07:00–10:00 UTC on their respective
cleaning dates: **09:00–12:00 CEST**, matching the reported notifications.

### 2.2 Recorded incident timeline

| UTC timestamp | Evidence | Result |
| --- | --- | --- |
| Oct 8 16:34:54 | Audit 6446 | Promotion of raw block 37710 attempted |
| Oct 8 16:34:54 | Run 13991, `named_stay_promote` | Reconciliation begins |
| Oct 8 16:34:54 | Event log 44982 | Row 42686 upserted |
| Oct 8 16:34:55 | Event log 44983 | October 9 row 42712 upserted, `synced` |
| Oct 8 16:34:55 | Event log 44984 | October 11 row 42687 deleted, `removed` |
| Oct 8 16:34:55 | Audit 6447 | Named stay 308 promotion succeeds |
| Oct 8 16:35:28 | Scheduled run 13992, log 44986 | Row 42687 upserted again, `synced` |

Run 13991 finishes successfully with `events_seen=2`, `events_upserted=2`,
`events_removed=1`. The deletion is not merely inferred from an email title.
The service records a successful delete after `DeleteEvent` returns success;
that client also treats remote 404/410 as successful absence. The database does
not retain the actual Google HTTP response.

An earlier occurrence corroborates the pattern: run 13967 at Oct 8 10:43:17
creates October 11 row 42687 (log 44957) and deletes October 8 row 42686
(log 44958). Scheduled run 13968 upserts row 42686 at 10:50:28.

### 2.3 Confirmed recovery failure

In the snapshot all three cleaning rows are `synced` with `pending_action=none`.
The user confirms the October 11 event is **currently missing in Google
Calendar**, despite this local state. Row 42687 has 82 `upsert`
logs and one `delete` log, with the latest upsert at Oct 9 12:35:28 UTC.
Repeated scheduled upserts have therefore not established visible recovery.
The subsequent read-only Google inspection confirms `cancelled` state. Source
inspection and Google's documented PATCH semantics identify the recovery defect:
upserts omit status and accept an ID without checking returned status. See
section 6 for evidence, limits and required correction. The confirmed email time
and matching production/local build complete operator correlation with run 13991.

Migration `000040_named_stay_only_cleaning_calendar` is recorded, and queries
using `pending_action` work. This is distinct from the older PMS-30
missing-migration incident. Do not rerun provisional cleanup to fix this bug.

## 3. Confirmed source defect

References below use investigation-time line numbers; symbols are authoritative.

1. `backend/internal/api/occupancy_named_stay_handlers.go:105,299–344`:
   promotion calls `reconcileCleaningStayRangesBestEffort`, which calculates
   the affected stay window and calls `ReconcilePropertyDateRange`.
   For stay 308 the inferred range is **October 8–9 inclusive**. The run table
   does not persist these bounds; this range follows from the source and dates.
2. `backend/internal/cleaningcalendar/service.go:157–187` builds `desired`
   from only the requested range intersected with today/+365 days. Thus the
   October 11 identity is legitimately absent from this partial desired set.
3. `service.go:217–252` fetches **all** active property cleaning rows using
   `ListActiveCleaningCalendarEvents`. Any identity absent from `desired`
   proceeds toward deletion unless protected by the past-date/reused-ID branches.
4. October 11 is future, outside the requested range, and has a different
   identity/Google reference. It is deleted without checking whether its owner
   remains eligible at the same checkout date.
5. `service.go:543–567` persists delete intent, calls Google, marks the row
   removed and logs the deletion. A success response is therefore possible even
   when the service made the wrong business decision.

**Root cause:** absence from a range-limited creation/update set is incorrectly
used as property-wide proof that an existing event is obsolete.

The same defect can delete a still-valid stored event beyond +365 days during
a broad reconcile: horizon exclusion is also not evidence of invalidity.
Matching is by IDs/ownership metadata; identical `Bez Hosta` titles are not the
identified cause (`googleEventIndex.match` does not use summary matching).

### Existing test gap

`TestReconcileRemovalIsDateScoped` in `service_test.go:117–147` archives **both**
owners, then expects both events removed despite reconciling one date. It does
not cover an eligible owner outside the range. Preserve its legitimate obsolete
owner behavior, give it an accurate name, and add the missing eligible-owner case.
The current fake does not update its remote inventory when writes occur and
ignores listing bounds; it cannot by itself prove recovery or no-op convergence.

## 4. Required implementation

### 4.1 Separate creation scope from deletion validity

Retain range-scoped creation/updates and property-wide inspection of existing
future rows. Introduce an explicit current-owner validity decision for deletion;
do not infer validity from membership in the partial `desired` map.

For every existing row, use property-scoped persisted ownership and the same
eligibility semantics as `ListCleaningNamedStayTargets`:

- owner belongs to the property and is an active named stay;
- cleaning is required;
- effective review is `COALESCE(review_resolution, review_status, 'confirmed')`
  and equals `confirmed`;
- outcome is neither `no_show` nor `cancelled_non_refundable`;
- stored checkout/cleaning date and cleaning identity correspond to the owner's
  current checkout and `NamedStayCleaningIdentity`.

Share eligibility logic/query construction where appropriate so desired-state
selection and deletion validation cannot silently diverge. An owner read/query
failure must return an error, not be interpreted as an absent/ineligible owner.
Validate the deletion plan before issuing deletes; avoid partial destructive
work caused by an incomplete owner inventory. Preserve existing property leases
and synchronization. Do not hold a database transaction across Google requests.

Apply this decision table:

| Existing event state | Required action |
| --- | --- |
| Future/today, owner eligible, identity unchanged, inside affected creation range and horizon | Normal desired-state upsert/no-op |
| Future/today, owner eligible, identity unchanged, outside affected range | Keep local row and remote event; no deletion intent or removed count |
| Future/today, owner eligible, identity unchanged, beyond +365 | Keep already scheduled event; no creation/update solely to expand the horizon |
| Future/today, owner ineligible or checkout identity obsolete | Persist deletion intent and remove obsolete event, even outside requested range or beyond +365 |
| Past with no previously pending delete | Preserve history and remote event; no create/patch/delete; clear expired pending upsert as PMS 22 requires |
| Previously pending delete, including after midnight | Complete deletion intent; never convert it directly into an upsert |

Creation still uses the intersection of affected dates and property-local
today/+365 inclusive. An empty intersection does **not** authorize deleting all
active rows. Use a consistent clock and calendar-day arithmetic.

Do not fix this by only switching to
`ListActiveCleaningCalendarEventsForDateRange`: that would abandon required
property-wide obsolete-future cleanup and pending deletions. Do not simply
disable deletions, broaden creation to arbitrary dates, or reconcile only the
new stay's checkout; arrival-side effects and old checkout cleanup must remain.

### 4.2 Preserve lifecycle and identity behavior

- Keep existing local row IDs and logs. Do not clear calendar tables, bulk reset
  statuses, or generate new identities for unchanged stays.
- Preserve same-day title updates and persisted end-time behavior from PMS 22.
- Keep old/new ranges from date edits. Remove obsolete future checkout identity;
  create its replacement only when allowed by the creation horizon. Preserve a
  past old identity according to the table above.
- Preserve deletion errors as `pending_action=delete` through retry/restart.
  Review the desired-loop ordering and `RetryEvent`: current code can encounter
  a pending deletion before owner validation. Complete outstanding deletion
  before considering a fresh eligible creation, as PMS 22 specifies.
- Retry must not replay stale upsert payloads for ineligible/out-of-horizon
  owners. Reuse current-state evaluation rather than creating a second policy.
- Keep remote ownership matching based on property, identities and IDs, never
  titles. An event reused/retained by a desired identity must not subsequently
  be remotely deleted through another local row. Include unchanged/skipped
  desired events in retained-reference protection; scope references by calendar.
- Preserve the saved-stay/calendar-error API contract. A calendar failure must
  not roll back the stay or tell the client to promote/create it again.

### 4.3 Focused diagnostics

Make future deletion decisions explainable: log property, run/trigger, requested
range, creation horizon, local event/stay identity, and reason such as
`owner_ineligible`, `checkout_changed`, or `pending_delete`. Reuse existing
logging/event-log facilities; no guest names or secrets are needed. Do not use
`not_in_desired` alone as a deletion reason. A schema migration is not expected
for this correction.

## 5. Regression tests required before completion

Use isolated migrated test databases, fixed clocks, and fake Google transport.
Add a stateful/range-aware fake where remote lifecycle assertions require it.
Never point automated tests at `data/pms.db` or a real calendar.

1. **Exact incident:** clock Oct 8, 2026, Europe/Bratislava; existing eligible
   Oct 11 event, then eligible stay Oct 8–9. Reconcile Oct 8–9. Assert Oct 9
   creation and zero Oct 11 DELETE calls; Oct 11 local ID, Google ID, status,
   pending action, window and deletion-log count remain unchanged. Include the
   Oct 8 departure fixture so expected arrival-side refresh is covered.
   Demonstrate this regression fails on the original implementation.
2. **Both sides of a narrow range:** eligible existing future events before and
   after it remain intact. Identical titles must not cause reference reuse.
3. **Legitimate global removal:** outside-range owner archived/cancelled,
   cleaning disabled, review pending/rejected, or suppressing outcome; each
   obsolete future event is still deleted. Verify effective review precedence
   and normal eligibility restoration without overwriting cleaning preference.
4. **Horizon:** keep valid existing event at +366 during broad and narrow runs;
   delete it if owner becomes ineligible or checkout changes. Create only at
   today through +365 inclusive, never yesterday/+366. Test an entirely
   out-of-horizon requested range and a range spanning a DST transition.
5. **Moves:** old/new future checkout inside horizon, move from inside to beyond
   horizon, move from beyond to inside, and past-to-future. Check which old
   events are removed/preserved and which replacements may be created.
6. **Deletion retry:** remote delete failure persists intent; reconcile and
   event retry continue delete after restart/midnight, including restored owner
   eligibility. Fresh creation is separately re-evaluated afterward.
7. **Read failure:** failed owner lookup/inventory must not trigger deletion.
   Remote listing failure must not be treated as an empty authoritative list.
8. **Idempotence:** repeat narrow reconcile, reconstruct Service with the same
   store, then broad scheduled reconcile. Unchanged remotely present events
   yield no further writes/deletes; verify local state and remote inventory,
   not just reported counters. Retained-reference protection must also cover
   a desired event whose upsert is skipped.
9. **Property isolation and entry points:** another property's event is
   untouched. Exercise direct creation and promotion through API tests with an
   existing outside-range event; verify edit/status/outcome/review callers
   retain old/new affected ranges and saved-stay error reporting.

Expected primary files: `backend/internal/cleaningcalendar/service.go`,
`backend/internal/store/cleaning_calendar.go`, their focused tests, and API
regression tests. Change Google client behavior only when supported by a
separate failing remote-response test/evidence (section 6).
Recovery is a required part of this incident fix, not an optional follow-up;
establish the cause before choosing that change.

Run from `backend/` and record results:

```sh
go test ./internal/cleaningcalendar ./internal/store ./internal/api
go test ./...
```

## 6. Confirmed cancelled-event recovery defect and required fix

Preventing future accidental deletes and verifying recovery are separate
acceptance items. The snapshot's `synced` flag cannot establish the latter.

The user confirms production matches the current local build. Record the exact
release identifier at deployment; that source-version question need not be
asked again.

### 6.1 Read-only Google evidence, collected 2026-10-09

Used the configured service account with an OAuth token scoped only to
`calendar.events.readonly`, and the calendar ID from property 1's settings.
All Calendar requests were GETs. Authentication used the OAuth token endpoint;
no Calendar POST/PATCH/PUT/DELETE was issued. Credentials were not printed.

Direct event GETs returned HTTP 200:

| Local row / stay | Google event ID | Remote status | Remote updated timestamp (UTC) |
| --- | --- | --- | --- |
| 42686 / 306 | `srveo6fnp9vsmarp1udnbnfesg` | `cancelled` | 2026-10-08T16:34:54.847Z |
| 42687 / 307 | `nkst3pm3qtfgo3ticlb5vo0te0` | `cancelled` | 2026-10-08T16:34:55.438Z |
| 42712 / 308 | `bfd13h1diheteh59e79af2m3k4` | `confirmed` | 2026-10-08T16:34:55.133Z |

All three retain the correct property, local event, named stay and cleaning
identity private properties. They retain their expected 09:00–12:00 +02:00
windows on October 8, 11 and 9 respectively. Google reports `Europe/Prague`
on the time fields; offsets/windows match the property-local schedule, and this
inspection supplies no evidence that the timezone label caused the incident.

Complete listings for October 8 00:00 through October 12 00:00 +02:00:

- `showDeleted=false`: two total events, only one matching inspected PMS
  ownership: the confirmed October 9 event.
- `showDeleted=true`: four total events, including all three PMS events above.
- An all-dates listing filtered by private property
  `pms_cleaning_identity=stay:1:307:2026-10-11`, with `showDeleted=true`, returned
  exactly the cancelled October 11 event and no active replacement.

Each listing completed in one page with no next-page token. Unrelated event
content was not retained. These results establish the inspected identity's
state, not an inventory of every affected event in the entire calendar.

### 6.2 Recovery mechanism

`google_client.go` PATCHes a stored ID and inserts only on PATCH 404. Its
`writeEvent` payload includes schedule/metadata but **omits `status`**; its
response decoder reads **only `id`**. A successful response containing an ID
therefore becomes local `synced` even if the event remains cancelled.

Google's documented PATCH semantics leave omitted fields unchanged. Event
`status` distinguishes `confirmed` from `cancelled`; GET may return deleted
events, while normal listing excludes them. Together with the observed cancelled
resource and successful application logs, this explains the repeated attempts:
the hidden event is missing from normal listing, is patched by its persisted ID
without restoring status, and is marked synced without validating that status.

Sources inspected:

- https://developers.google.com/workspace/calendar/api/v3/reference/events/patch
- https://developers.google.com/workspace/calendar/api/v3/reference/events

Evidence limit: historical PATCH response bodies were not retained and no live
diagnostic PATCH was issued. Do not describe a captured historical PATCH body
or a successfully tested production restoration; neither occurred.

### 6.3 Required recovery implementation and tests

1. For an authorized, currently eligible, in-horizon desired event, explicitly
   send `status: confirmed` in the Google upsert payload, including PATCH. This
   supplies the previously omitted desired state and permits restoration using
   the same Google ID. Keep lifecycle validation in the service: this must not
   revive ineligible stays, past events or outstanding deletion work.
2. Decode and validate returned event status as well as ID. A cancelled response
   is never a successful upsert. Require the expected confirmed state; missing
   or unexpected status must produce an actionable integration error rather
   than silently recording `synced`. Preserve pending upsert intent on failure.
3. Keep the existing verified-absence fallback for PATCH 404 with coverage.
   Do not insert a duplicate merely because normal listing omitted a cancelled
   event. Do not classify all 4xx, access errors, or ambiguous transport failures
   as absence. If another status such as 410 requires new handling, establish
   its specific Google error meaning before adding a fallback.
4. Update HTTP transport test fixtures to return realistic status fields.
   Add a regression with a stored cancelled event: normal listing omits it;
   PATCH without status preserves cancellation under documented semantics;
   the corrected PATCH requests `confirmed`, and its confirmed response yields
   one active event with the original ID and correct window/metadata. This is
   a modeled transport test, not a claim that production PATCH was exercised.
5. Add explicit HTTP-200-with-`status: cancelled`, missing-status, and malformed
   response tests: none may set local status to synced or trigger blind insert.
   Test confirmed responses, verified 404 replacement and response validation
   on inserts too. Preserve retryability if local persistence fails after a
   successful remote restoration.
6. Run a second reconciliation against the restored remote inventory: zero
   redundant upserts/deletes, same local row/identity/Google ID. Verify direct
   retry uses current ownership, settings and horizon before restoring.

The core recovery correction can now be implemented without another production
write experiment. Successful real restoration remains a deployment check.

Deploy the corrected service before reconciling eligible current/future events.
Use current stay state and existing reconciliation, not bulk SQL resurrection.
Recheck affected identities for exactly one active correctly timed Google event,
then verify a second reconciliation is a no-op. Past deleted events must not be
automatically recreated in conflict with PMS 22's historical preservation rule.

Operator confirmations received after the initial investigation:

- Production is the same as the current local build.
- The October 11 cleaning is currently missing from Google Calendar.
- Both emails arrived at exactly 18:34 CEST on October 8.

The read-only evidence gap is resolved. Remaining execution work is regression
testing, implementation, deployment and verification of actual restoration.

## 7. Completion criteria

- Incident regression fails before the fix and passes afterward.
- Eligible unchanged events survive regardless of requested range or creation
  horizon; obsolete future events and durable pending deletes still converge.
- Existing ownership, history, scheduling, API and retry contracts are preserved.
- Focused and full backend tests pass, with actual commands/results recorded.
- The confirmed recovery failure is explained with remote-response evidence and
  covered by a failing-before/passing-after regression. The eligible October 11
  event is verified active remotely during operational recovery, subject to
  the current-date horizon policy if recovery occurs after its date.
- Deployment revision and current remote state are recorded before declaring
  production recovery complete; unresolved repeated-upsert behavior is reported
  explicitly rather than concealed by a local `synced` status.
