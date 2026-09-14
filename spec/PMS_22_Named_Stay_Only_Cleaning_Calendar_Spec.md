# PMS_22 - Named-Stay-Only Cleaning Calendar Spec

> Audience: product / property manager + implementing engineer.
> Scope: remove raw Booking.com provisional Google cleaning events and make
> persisted named stays the only operational cleaning-event owners.
> Status: final implementation contract; ready for the coding agent to implement.
> Decision record updated: 2026-09-14.
> Product decisions are recorded below. Engineering decisions in sections 7-9
> resolve the former draft's technical blockers. No further draft-approval gate
> applies to implementation requested by the user. This document does not claim
> that code, migrations, or production cleanup have already been executed.

## 1. Problem Framing

Booking.com ICS is availability evidence, not a confirmed description of guest
stays. PMS currently sends provisional cleaning placeholders for uncovered raw
nights to Google Calendar. These placeholders must stop completely.

The intended workflow is: import raw availability, let the user create or
promote a named stay, then create that stay's checkout cleaning event when it is
eligible. Remaining raw nights never imply cleaning work.

## 2. Definitions

- **Named stay**: a persisted `named_stays` business record, created directly,
  promoted from raw evidence, or retained from an earlier migration. A raw ICS
  event with a non-empty summary is not a named stay.
- **Provisional event**: the retired raw-owned cleaning flow, currently using
  `provisional_block`, `raw_booking_block_id`, and
  `raw-provisional:{propertyID}:{checkoutDate}`.
- **Operational event**: a cleaning record that participates in normal desired
  state, reconciliation, retry, or current calendar ownership.
- **Cleanup cutoff**: the start of the deployment date in each property's
  timezone, fixed and persisted for cleanup. Provisional events on that entire
  date or later are in scope, including events earlier than the deployment time.
- **Creation horizon**: the common date window applied by every path capable of
  creating a Google cleaning event: property-local today through today plus
  365 calendar days, inclusive.

## 3. Verified Current Implementation

This analysis is based on source and existing tests, not a live database or
Google Calendar inventory.

1. `store.ListCleaningRawProvisionalTargets` selects every active raw night not
   covered by an active named stay and derives the following date as checkout.
   Overlapping raw blocks coalesce by property/date. July 9-12 raw coverage
   therefore produces July 10, 11, and 12 provisional events.
2. `cleaningcalendar.Service.buildDesiredEvents` combines raw provisional and
   named-stay candidates. Provisional titles are exactly `Upratovanie`.
3. `ListCleaningNamedStayTargets` currently requires `status = 'active'`,
   `cleaning_required = 1`, and checkout inside the requested range. It does not
   filter review status or stay outcome.
4. Named-stay creation and promotion commit the stay before immediately
   attempting date-scoped cleaning reconciliation. The handler currently
   discards cleaning errors and returns the saved stay without a cleaning error.
5. Manual and scheduled ICS sync invoke cleaning reconciliation afterward.
   A separate scheduled cleaning job already exists.
6. Broad reconciliation uses 30 days back and 365 days forward. Stay actions
   currently reconcile their affected range without that common limit.
7. `RetryEvent` directly upserts stored non-removed events. Failed deletion sets
   status to `error`, allowing a later Retry to upsert instead of delete.
8. Cleaning rows have exactly one raw or named owner. Same-property foreign keys
   and event-log references use restrictive deletion behavior in migration 39.
9. Google listing currently reads one response page; complete remote discovery
   requires pagination.
10. Same-day detection uses active named-stay nights, not raw nights. The helper
    `preserveExistingWindowForSameDayOnlyChange` preserves the stored end time on
    a same-day-status transition, whereas PMS 15/the deployment guide describe
    updating that end time.

Primary references:

- `backend/internal/store/cleaning_calendar.go`
- `backend/internal/cleaningcalendar/service.go`
- `backend/internal/cleaningcalendar/google_client.go`
- `backend/internal/api/occupancy_named_stay_handlers.go`
- `backend/internal/api/occupancy_handlers.go`
- `backend/cmd/server/main.go`
- `backend/internal/migrate/000039_legacy_occupancy_removal.up.sql`

## 4. Confirmed Product Decisions

### 4.1 Named-Stay-Only Ownership

Every newly created PMS-managed Google cleaning event must belong to an eligible
persisted named stay. Raw blocks and nights must never independently produce an
event, including through retry, restart, manual reconcile, or scheduled jobs.

### 4.2 Existing Provisional Events

Delete provisional Google events on or after the property-local deployment
date, including events earlier that day. Preserve events dated before that day.
Persist this per-property cutoff so later cleanup retries use the original
boundary, not the date of the retry.

### 4.3 Complete Removal And History

Remove provisional behavior from runtime code, current API/UI contracts, and
the final operational schema. Preserve applied migrations and mark conflicting
historical specifications as superseded.

Product explicitly requests discarding local provisional-event history: delete
provisional event rows and their associated event logs, with no historical
archive or permanent per-event cleanup-history requirement. This supersedes the
earlier proposal to retain those records. Named-stay event history is unaffected.

Keep only the temporary identifiers/progress needed to complete and retry an
unfinished in-scope remote deletion. Purge that work data once resolved. Past
Google events remain untouched even though their local provisional history is
discarded; they must have no operational update/recreation path.

### 4.3.1 Current Calendar Only

Product uses a single cleaning calendar. Remote cleanup applies only to the
currently configured calendar ID. Do not inspect or delete events in previously
configured calendars, even if local provisional rows reference them.

Local provisional rows/logs referencing previous calendars are discarded without
remote deletion. Events left in those calendars are outside cleanup scope and
are not unresolved cleanup failures.

Before a cleanup attempt or retry, verify the target is still the configured
calendar. A stored old calendar ID must not authorize deletion there after a
settings change. If configuration changes during the transition, stop issuing
requests to the previous target and re-evaluate pending work against the current
configuration. Keep the original deployment-date cutoff.

### 4.4 Stay Types

Any active named stay with cleaning enabled qualifies by type, including
maintenance and personal-use stays when cleaning is explicitly enabled.
Booking.com and external stays retain their existing default of enabled;
maintenance and personal-use retain their default of disabled.

### 4.5 Existing Named Stays

Existing eligible named stays, including migrated records, remain candidates.
Do not require a creation timestamp after deployment or a newly populated
manual-creator field.

### 4.6 User-Visible Failures

A cleaning failure after stay persistence must produce an explicit user-visible
error. The UI must distinguish a successfully saved stay from a failed calendar
operation and must not encourage the user to create the stay again.

### 4.7 Common Creation Horizon

Every event-creation path must use one common horizon. A stay action must not
bypass it through a date-scoped call. The confirmed horizon is property-local
today through today plus 365 calendar days, inclusive. Never create or recreate
a Google cleaning event for an earlier date. Today is date-based; it does not
exclude a checkout earlier on the current day.

### 4.8 Same-Day Raw Evidence

Raw arrivals remain excluded from named-stay same-day-turnover detection.

When a same-day named arrival is added, removed, or changes eligibility later,
the cleaning event's end time remains as originally scheduled. Same-day status
and title may update, but this transition must not recalculate the end time.
This rule must hold across repeated reconciliation, retry, and restart, not
merely the first reconciliation that observes the transition.

Initial scheduling continues to use the applicable same-day/default-duration
rule. This decision concerns later same-day-arrival changes; it does not define
a new policy for moving checkout dates or explicitly editing property times.

### 4.9 API Consumers

The bundled frontend is the only API consumer. Backend contracts, OpenAPI, and
frontend types/UI can change together without an external compatibility layer.
Product confirms all old backend instances can be stopped before cleanup.
Deployment must do so to prevent the retired code from recreating blockers.

### 4.10 Outcomes And Review Eligibility

Product approved the Q4 recommendation in full:

- `no_show` and `cancelled_non_refundable` suppress own-checkout cleaning even
  when the stay remains active and cleaning is enabled.
- `needs_review` waits for confirmation; `rejected` suppresses cleaning.
- Require confirmed effective review status for cleaning eligibility, using the
  current named-stay review-resolution model.
- These suppressed/unconfirmed records do not count as real same-day arrivals.
- An otherwise eligible real arriving stay still counts for turnover when its
  own checkout cleaning is disabled.
- Suppression overrides the cleaning checkbox without overwriting its stored
  value. Clearing the outcome or confirming the stay restores eligibility only
  if all other conditions, including the creation horizon, pass.

Reconciliation removes an existing event when the stay becomes ineligible,
subject to the historical named-event removal policy in section 7.1.

## 5. Resolved Business Analyst Clarifications

### 5.1 Q4: Resolved

Product approved suppression for no-show, non-refundable cancellation, pending
review, and rejected review, including the related arrival and preference rules.
See section 4.10 for the contract.

### 5.2 Cleanup Coverage And Local History: Resolved

Product selected current-calendar-only remote cleanup and discarding local
provisional history. Previous calendar IDs are not cleanup targets. No archive
will be added. See sections 4.3 and 4.3.1 for deletion scope and temporary
unfinished-work handling.

### 5.3 Common Horizon Bounds: Resolved

Today through 365 days ahead, using property-local dates. See section 4.7.

### 5.4 Same-Day End Time: Resolved

Preserve the originally scheduled end time when same-day arrivals change.
See section 4.8. Update conflicting operational documentation during implementation.

### 5.5 Deployment Coordination: Resolved

Product confirms old instances can be stopped before cleanup; this is the
required cutover procedure. See section 4.9.

## 6. Functional Requirements

| Action/state | Required behavior |
| --- | --- |
| Raw import, overlap, resize, cancellation, disappearance, reappearance | No provisional cleaning creation |
| Direct eligible named-stay creation | Attempt one checkout event inside the common horizon |
| Full or partial raw promotion | Attempt one event for the eligible named stay; none for leftover raw nights |
| Multiple distinct named stays within raw coverage | One checkout event per eligible named stay |
| Cleaning disabled | Suppress creation and reconcile existing event removal |
| Named stay cancelled or archived | Reconcile its managed event removal |
| Stay restored or cleaning enabled | Re-evaluate eligibility and horizon without duplicates |
| Dates changed | Reconcile old/new checkout and affected arrival dates |
| Linked raw evidence changed or lost | Named-stay business state continues to govern cleaning |
| Review/outcome changed | Apply section 4.10 suppression/restoration and arrival eligibility |
| Historical provisional event before cutoff | Preserve remotely; discard local provisional row/logs; never recreate/update |
| Provisional event in a previous calendar | No remote cleanup; discard local provisional row/logs |

The raw availability, source-link, and named-stay business workflows remain
independent of Google cleaning ownership. A cleaning failure must not mutate a
stay's business status or cleaning preference.

## 7. Trigger, Horizon, And Retry Contract

- Remove cleaning calls after both manual and scheduled ICS sync.
- Retain immediate reconciliation following relevant named-stay mutations,
  manual cleaning reconcile, and the independent scheduled cleaning job.
- Compute the common horizon using property-local dates and a consistent clock.
  Intersect candidate creation ranges with this horizon on every entry point,
  including retry and recreation of missing Google events.
- Use inclusive local dates: today and today plus 365 calendar days. Creation
  must be excluded yesterday and at today plus 366 days on every entry point.
- A cleanup deletion range is independent of the creation horizon. Do not leave
  a future provisional event behind merely because it is more than 365 days out.
- Named-stay moves must not lose the old event's deletion requirement when the
  new date is outside the creation horizon. Apply section 7.1; deletion and
  creation eligibility are evaluated separately.
- Every upsert must validate current ownership, eligibility, settings, and
  horizon instead of replaying a stale stored payload.
- A failed deletion remains a deletion on retry; generic `error` status must
  not silently change the requested operation into an upsert.
- Repeated and concurrent reconcile/retry calls must converge without duplicate
  Google events. Job-specific scheduler leases alone do not serialize all
  manual, scheduled, and stay-triggered operations for a property.

### 7.1 Historical Named Events And Removal Policy

The following is the engineering resolution of the historical-removal question.
It distinguishes historical preservation from unfinished synchronization; it
does not extend the approved creation horizon.

| Stored named event | Reconciliation behavior |
| --- | --- |
| Date before property-local today, with no previously pending deletion | Preserve the Google event and local history. No new remote create, patch, or delete for that historical event |
| Date today or later, current owner still eligible and dates/identity unchanged | Keep it; refresh within the creation horizon. Merely being outside the horizon is not a reason to delete an already scheduled valid event |
| Date today or later, owner becomes ineligible or checkout/identity changes | Persist deletion intent and delete the obsolete event, including dates beyond +365 days |
| Previously pending deletion whose date has since passed | Complete the already requested deletion; do not abandon failed work at midnight |
| Missing remote event dated before today | Do not recreate it, including through Retry |

For a date edit, evaluate the old event by its own stored date. A preserved past
event can coexist as history with a new future checkout identity. Create the
replacement only if its new checkout falls inside the common creation horizon.
Inspect all existing future named events for obsolete identities/owners rather
than deriving deletion candidates solely from the clipped creation range.

Persist `pending_action` on operational named cleaning rows with values `none`,
`upsert`, or `delete` (`TEXT NOT NULL`, checked enum, default `none`). Set it
before the corresponding remote write. Failed deletion keeps `delete`; success
sets status `removed` and action `none`. Upsert failure keeps `upsert`; success
sets status `synced` and action `none`. Retry recomputes current state before an
upsert and never converts a pending delete into an upsert. Complete the delete
first, then re-evaluate possible recreation if eligibility was restored meanwhile.
Past pending upserts are cleared without remote creation; preserve the local row
with an explanatory message. Disabled normal sync retains unfinished intent for
later reconciliation, without writing to Google.

During migration, seed pending actions on retained named rows: `pending` maps
to `upsert`, `synced`/`removed` to `none`; for `error`, use the latest applicable
event log (`delete_error` => `delete`, `upsert_error` => `upsert`). If an error
row has no such evidence, seed `none` and let current desired-state evaluation
decide. Never treat a generic error alone as permission to replay an upsert.

### 7.2 Persistent Timing Contract

For an existing checkout identity whose start and scheduling settings have not
changed, preserve the previously scheduled end and conflict warning even after
the same-day status has already been updated on an earlier reconciliation.
Compute the desired hash after applying this preservation.

Store a scheduling-input fingerprint on named cleaning rows (`schedule_hash`,
nullable TEXT for migration), covering property timezone, checkout date,
configured checkout/check-in times, and default cleaning duration; exclude
arrival presence, labels, and title. On first reconcile of a migrated row, adopt
current scheduling inputs while retaining its stored window. A later change to
these explicit scheduling inputs allows recalculation. A new checkout identity
gets a fresh initial schedule. Title or arriving-stay changes alone preserve the
end. Missing-remote recovery for the same local identity uses its stored schedule.

## 8. API And Frontend Requirements

- Stay persistence and calendar synchronization outcomes must be distinguishable.
- On calendar failure after a successful stay save, show an error such as:
  `Stay saved, but cleaning calendar sync failed. Retry calendar synchronization.`
- Return the saved named-stay ID and an explicit structured cleaning error/status.
  Preserve relevant existing Nuki response fields. Use the concrete response
  contract in section 8.1; do not present a persisted stay as a failed creation.
- Reload the saved stay even when calendar synchronization fails.
- Offer calendar retry without replaying the stay-create request.
- A disabled integration or out-of-horizon checkout is an intentional skip, not
  a Google failure. Surface that state accurately when needed.
- Remove provisional counts, provisional-specific success copy, raw-owned
  cleaning DTO fields, and raw-block cleaning attachments/error indicators.
- Update OpenAPI and regenerate types alongside backend and frontend changes.
- Unfinished in-scope cleanup failures need visible, retryable temporary work
  state. They must not be exposed as provisional event-creation candidates or
  retained as a historical archive after resolution.

### 8.1 Named-Stay Mutation Response And HTTP Status

Apply this contract to direct create, promotion, and named-stay edit, status,
outcome, and review mutations that trigger cleaning reconciliation.

- Validation/access/not-found/conflict and database failures before persistence
  retain their current non-2xx error contracts. Do not return `stay_saved: true`.
- After the stay transaction commits, return HTTP **200**, including when Google
  reconciliation fails. This is a completed stay mutation with a separately
  failed side effect, not a request that should be retried as stay creation.
- Add required `stay_saved: true` and `cleaning_calendar` to the existing
  `namedStayV2Response`. Retain `named_stay_id`, `nuki_generation_status`, and
  optional `nuki_generation_error` with their existing meanings.
- `cleaning_calendar.status` is `synced`, `skipped`, or `error`.
  `synced` means the requested reconciliation succeeded, including no changes
  being needed; it does not assert that the edited stay owns an event.
- `cleaning_calendar.reason` is present only for `skipped`, with values
  `disabled`, `calendar_not_set`, or `no_work`. `no_work` applies only when there
  is no eligible creation/update, required deletion, or affected arrival work.
- `cleaning_calendar.error` is a non-empty, user-safe string only for `error`.
  An enabled/configured integration with unavailable service/credentials, failed
  Google calls, lease acquisition failure, or failed local sync persistence is
  an error, not a skip. Do not echo credential material or raw upstream bodies.
- Set top-level `ok: false` and `error` on a cleaning failure; otherwise retain
  the existing top-level success behavior. Existing Nuki outcome reporting is
  unchanged. Evaluate cleaning outcome for the whole requested reconcile range,
  including an affected previous stay, not only the edited stay's own event.

Example after the stay is saved but calendar synchronization fails:

```json
{
  "ok": false,
  "stay_saved": true,
  "named_stay_id": 123,
  "nuki_generation_status": "generated",
  "cleaning_calendar": {
    "status": "error",
    "error": "Google Calendar access denied. Check calendar permissions."
  },
  "error": "Stay saved, but cleaning calendar sync failed. Retry calendar synchronization."
}
```

The frontend must handle this response before any generic `ok: false` handling:
close/complete the saved form, reload the stay, show a persistent error with a
retry action, and never resend the create/promote request automatically.

### 8.2 Retry And Manual Reconciliation Responses

Use the existing property cleaning reconcile endpoint for a retry when there is
no local event ID (for example, Google listing failed before a row was saved).
Use the existing event retry endpoint when an event ID exists. Neither calls a
stay mutation. Property reconciliation inspects pending named deletion work
outside the creation horizon as required by section 7.1.

Keep current permission checks. For an authorized, valid reconcile/retry request,
return HTTP 200 with `ok: true` on completion, or `ok: false` and a non-empty
user-safe `error` on integration failure. Reconcile retains its general event
counts in `stats`, without provisional counters. Missing property/event returns
404; invalid input 400; permission errors retain existing 401/403 semantics.
The UI must show `ok: false` as an error even though the HTTP status is 200.

Provisional migration cleanup uses the offline command in section 9, not these
runtime retry endpoints. Its temporary failures are shown in command output and
exit status, avoiding a new UI/archive or permanent provisional runtime path.

## 9. Transition And Persistence Requirements

1. Establish and persist a fixed deployment cutoff. A retried cleanup must not
   advance the cutoff and accidentally preserve events it previously failed to
   delete.
2. Prevent all old/new application paths from generating provisional events.
3. Inventory local provisional rows and linked logs. Classify remote work using
   the current configured calendar ID and deployment cutoff. Do not list or
   delete events in previous calendars.
4. Preserve only identifiers and progress needed for unfinished in-scope remote
   deletions durably before removing operational provisional ownership.
5. Delete only identified PMS-owned provisional events in scope. Match recorded
   IDs and ownership metadata, never the title alone. Do not delete a named-stay
   event that shares an old reference without resolving actual ownership.
6. Preserve past provisional Google events and all events in previous calendars.
   Delete local provisional rows and associated logs with foreign-key-safe
   ordering. Do not archive them. Purge temporary deletion work after resolution.
7. Handle interrupted cleanup, remote deletion already completed, permissions
   failure, missing IDs, and unavailable current-calendar configuration explicitly.
   Never fall back to an old stored calendar ID when no current target is set.
8. If discovering remote orphans/duplicates, paginate fully and scope by PMS
   property/ownership metadata in the current calendar only. A missing local ID
   is not proof of remote absence.
9. Rebuild the final operational cleaning schema with required same-property
   named-stay ownership. Preserve named-event row IDs, identities, Google IDs,
   and required event history. Recreate relevant constraints/indexes deliberately.
10. Verify no operational raw-owned events or provisional history archive remain,
    no upsert can recreate one, and all in-scope remote deletion work is complete
    or explicitly pending. Resolved per-event work records need not be retained.

SQL migration cannot atomically delete Google events. Use a resumable transition
with durable deletion work rather than discarding identifiers and assuming the
next scheduled reconcile will remove remote events.

Applied migrations remain historical evidence. Add a forward migration; do not
rewrite applied migrations to pretend provisional ownership never existed.
The cleanup mechanism must be deletion-only and must not retain a hidden
provisional feature flag or a reactivation path.

### 9.1 Offline Cleanup Command And Temporary Schema

Implement `backend/cmd/cleaning-calendar-cleanup` as a one-time offline deployment
tool. It uses the existing database/configuration and service-account facilities,
but must not start the HTTP server, schedulers, normal cleaning upserts, or
automatic migrations. It supports `prepare`, `run`, and `status` subcommands.
All invocations require the application stopped and exclusive operational access
to the database; use the repository job-lease mechanism to prevent two cleanup
commands working concurrently. No new credential or calendar-ID CLI inputs:
target only the calendar configured in PMS.

`prepare` creates these ordinary durable SQLite work tables with `IF NOT EXISTS`
(not connection-local TEMP tables). The final forward migration drops both:

**`cleaning_calendar_cleanup_state`**

| Column | SQLite contract |
| --- | --- |
| `property_id` | INTEGER PRIMARY KEY, FK properties(id) ON DELETE CASCADE |
| `cutoff_date` | TEXT NOT NULL, validated YYYY-MM-DD in the property's deployment timezone |
| `timezone` | TEXT NOT NULL, timezone captured when cutoff is initialized |
| `calendar_id` | TEXT NULL, trimmed current target captured for this pass |
| `phase` | TEXT NOT NULL CHECK IN ('prepared', 'discovered', 'complete') |
| `last_error` | TEXT NULL, latest actionable error, no credentials |
| `created_at`, `updated_at` | TEXT NOT NULL, UTC RFC3339 |

**`cleaning_calendar_cleanup_items`**

| Column | SQLite contract |
| --- | --- |
| `id` | INTEGER PRIMARY KEY |
| `property_id` | INTEGER NOT NULL, FK cleanup state(property_id) ON DELETE CASCADE |
| `calendar_id` | TEXT NOT NULL |
| `google_event_id` | TEXT NOT NULL, non-empty |
| `attempts` | INTEGER NOT NULL DEFAULT 0 CHECK attempts >= 0 |
| `last_error` | TEXT NULL |
| `created_at`, `updated_at` | TEXT NOT NULL, UTC RFC3339 |

Require UNIQUE (`property_id`, `calendar_id`, `google_event_id`). Every item means
pending deletion; do not store completed items. No guest payloads, raw ICS,
provisional row snapshots, or historical logs are copied into these tables.
State is deployment control data, not retained event history.

### 9.2 Command State Machine And Discovery

1. `prepare` initializes state for existing properties with a configured cleaning
   calendar or local provisional records, irrespective of the normal sync toggle.
   It fixes the cutoff to the property's local date at deployment preparation.
   Invalid timezone is an error, not a silent UTC fallback. Repeated preparation
   preserves an existing cutoff. Do not discard source rows/logs in this step.
2. `run` reloads the current calendar settings. A changed target discards queued
   items for the former target without remote requests and resets discovery for
   the new target, retaining the cutoff. An absent target has no permitted remote
   work: clear old queued items and complete this property's pass with an explicit
   `no current calendar configured` message. Never fall back to historical IDs.
3. For a configured target, first finish a complete paginated listing of that
   calendar, starting at cutoff local midnight, **without a +365-day upper bound**.
   Use a cleanup-specific listing method if necessary. Identify provisional PMS
   events using property metadata and raw provisional identity/raw ownership, or
   an exact local `(calendar_id, google_event_id)` provisional association. Require
   no conflicting named ownership. Use actual remote start date to protect
   pre-cutoff events even if the local date is stale. Do not infer ownership from
   titles. An ambiguous raw/named association fails the pass with an actionable
   error; leave that event untouched rather than silently deleting or completing.
4. Queue each identified in-scope event idempotently. Match local records without
   Google IDs by the existing supported PMS identities/private metadata during
   that full scan. A failed/incomplete scan must not mark discovery complete.
   Repeating the full scan is permitted; do not need a persisted page cursor.
5. After complete discovery, set phase `discovered`. Process queued deletes,
   checking the target calendar still matches settings before each request.
   Confirm current remote ownership/date before deleting if work is resumed or
   the event could have changed; preserve an event moved before cutoff, and stop
   on conflicting named ownership. Remove items for already absent events.
6. Delete success or verified event absence removes the work item immediately.
   Errors retain it with incremented attempts and a user-safe message. Verify
   calendar access before interpreting an event 404 as absence; an inaccessible
   calendar must not be declared successfully cleaned. Google 410 for a deleted
   event is idempotent completion. A crash after remote success but before local
   removal is repaired by the next run.
7. When all work is resolved, perform a final complete discovery pass. Mark the
   property `complete` only when no in-scope provisional events remain. State
   errors clear on successful recovery. Normal disabled sync does not disable
   this explicitly invoked deployment cleanup, nor does cleanup enable sync.
8. `status` is read-only and prints each property's phase, pending count, cutoff,
   target and latest error. `run` exits zero only when all state is complete;
   otherwise nonzero. `prepare`/`status` exit nonzero on command errors. Running
   again is the retry mechanism; no manual database editing is required.

### 9.3 Final Forward Migration

Choose the next unused migration number at implementation time; do not renumber
or modify applied migrations. The normal server must not perform remote cleanup
inside a migration.

For an existing installation, the migration checks that every property requiring
preparation has a `complete` state with the current configured calendar matching
the completed pass and no pending work items. Otherwise stop with an actionable
message to run the offline command. A freshly initialized database with no
properties or cleaning records needs no cleanup and migrates normally.
Create empty work tables with the identical `IF NOT EXISTS` definitions at the
start of the migration transaction so these checks also work when preparation
was never run; empty tables must not bypass checks for existing properties.

In one database transaction:

- Delete logs referencing provisional rows, then delete those rows, including
  past and previous-calendar rows, with no archive.
- Rebuild operational cleaning ownership as named-only; retain all named rows,
  IDs, Google references, and their event logs. Apply sections 7.1/7.2 columns.
- Recreate foreign keys and relevant indexes; verify foreign-key integrity.
- Drop the two temporary cleanup-work tables, including the completed state.

On transaction failure, source rows/work state remain available for retry. The
recorded migration version is the completion marker; no permanent provisional
cleanup registry is required. After the migration, the offline command detects
the installed version and reports already completed without recreating work
tables or rediscovering old events. Keep this deletion-only historical upgrade
tool separate from runtime packages; it is not a provisional-generation flow.

### 9.4 Deployment And Recovery Procedure

1. Build/test the new backend, matching frontend, and cleanup command before
   touching production. Deploy them as one release artifact.
2. Stop every old backend instance and scheduler. Stop application writes for
   the entire cleanup/migration pass. Take a database backup using the existing
   deployment backup method. Record the prepared deployment date via `prepare`.
3. Run `status`, then `run` using the same database and current configured
   calendar/service-account settings. Do not change the calendar during the pass.
   For errors, correct access/configuration and rerun; the cutoff stays fixed.
4. Do not start the new server until cleanup completes. Run the normal forward
   migration, then verify named-row/reference preservation and foreign-key
   checks. The migration itself refuses an incomplete pass.
5. Start only the new backend and matching frontend. Run named-only cleaning
   reconciliation and verify raw imports produce no events and eligible named
   stays reconcile within today/+365 days. Verify a calendar error is surfaced
   without losing the saved stay or duplicating it on retry.
6. On a cleanup failure, remain stopped and resume cleanup; on migration failure,
   fix and rerun the transactional migration. Do not start an old binary after
   remote cleanup: it can regenerate blockers. If a backup is restored, repeat
   cleanup before serving traffic. Google deletion is not rolled back by a
   database restore. Prefer correcting the new release over schema rollback.

No staged mixed-version deployment or temporary generator flag is required.
Production execution follows the operator's deployment request; code generation
can proceed from this contract without waiting for another specification phase.

## 10. Implementation Map

| Area | Planned change |
| --- | --- |
| `backend/internal/cleaningcalendar/service.go` | Remove raw builders/matching/counters; enforce current eligibility, horizon and deletion retry |
| `backend/internal/cleaningcalendar/google_client.go` | Remove raw metadata from new writes; support complete discovery where required |
| `backend/internal/store/cleaning_calendar.go` | Remove raw target type/query/identity; enforce named ownership |
| `backend/internal/store/stay_calendar.go` | Remove raw cleaning attachments; preserve named attachments |
| `backend/internal/api/occupancy_handlers.go` | Decouple manual ICS sync from cleaning |
| `backend/cmd/server/main.go` | Decouple scheduled ICS sync; retain independent cleaning job |
| `backend/internal/api/occupancy_named_stay_handlers.go` | Surface saved-stay/calendar partial failures; common horizon; preserve affected arrival ranges |
| `backend/internal/api/cleaning_calendar_handlers.go` | Update DTO/retry contracts |
| `backend/internal/migrate/` | Forward schema transition and migration tests |
| `backend/cmd/cleaning-calendar-cleanup/` | Offline prepare/run/status command and restartable deletion-only upgrade tests |
| `backend/internal/pms21cleanup/` | Review historical tooling assumptions against schema-version boundaries |
| `frontend/src/views/CleaningView.vue` | Remove provisional copy/counters; retry/error behavior |
| `frontend/src/views/occupancy/` and named-stay UI | Remove raw cleaning indicators; show saved-stay/calendar failures |
| `frontend/src/api/types/{cleaning,occupancy,generated}.ts` | Align types with current contracts |
| `spec/openapi.yaml` | Update schemas and regenerate frontend types during implementation |
| `docs/deployment/google-calendar-cleaning.md` | Document named-only lifecycle, horizon, errors, and transition |

This map defines the implementation scope. Inspect shared
uses before removing fields: raw-block IDs remain valid for availability and
promotion even though they no longer own cleaning.

## 11. Relationship To Existing Specs

PMS 22 supersedes raw/provisional cleaning behavior and ownership
in PMS 15, PMS 19, PMS 21, and ADR-003. PMS 21 continues to govern availability
and named-stay identity outside this change.

Preserve historical decisions/migrations, with clear supersession pointers.
Update current operational documentation during implementation. An authority pointer
must not claim the runtime change or production cleanup has already happened.

PMS 17/PMS 18 express older outcome/exclusion rules. Implement section 4.10 using
the current named-stay model instead of reintroducing legacy occupancy fields.

## 12. Test Focus And Acceptance Criteria

- Raw-only ICS imports followed by every reconciliation path produce zero local
  operational cleaning rows and zero Google upserts.
- Manual/scheduled ICS sync no longer calls cleaning reconciliation.
- Direct creation, full/partial promotion, and multiple named stays produce
  exactly their eligible checkout events; uncovered raw nights produce none.
- Existing/migrated named stays remain eligible without a post-deployment gate.
- All four stay types follow active/cleaning-enabled rules; outcome/review tests
  enforce section 4.10, including restoration without resetting the preference
  and exclusion from same-day arrival detection.
- Failed deletion followed by Retry never upserts; stale raw references cannot
  resurrect an event. Named retry uses current state and the common horizon.
- Common horizon boundaries, property timezone, DST, missing Google events,
  dates moved across the horizon, and old/new arrival effects are covered.
- Same-day arrival addition/removal updates status/title while preserving the
  original end time across repeated reconciles, retry, and restart. Hashes must
  describe the actual preserved payload, not an unused recalculated end time.
- Stay saved plus Google failure is visible as an error in the frontend, preserves
  the saved ID, and retries cleaning without duplicate stays.
- Repeated/concurrent reconciliation and interrupted remote/local writes do not
  create duplicate named events or lose pending deletion work.
- Cleanup preserves pre-cutoff Google events and deletes approved post-cutoff
  provisional events, including earlier on the deployment day and beyond the
  creation horizon; evaluate the cutoff in each property's timezone.
- Cleanup retry uses the original cutoff; discovery covers pagination and
  distinguishes unrelated/named events from provisional events.
- Cleanup lists/deletes only in the currently configured calendar. Previous
  calendar records never cause remote requests there; settings changes before
  retry cannot redirect deletion to a retired calendar.
- Local provisional rows/logs are discarded, including past and previous-calendar
  records. No archive is created. Unfinished in-scope deletion identifiers survive
  failure/restart and are purged after resolution.
- Migration preserves required named-event values/history and passes foreign-key
  checks; historical migration paths remain testable.
- Saved-stay partial failure uses the exact HTTP 200/`stay_saved`/cleaning outcome
  contract; UI error handling does not replay creation. Skip/error states and
  property/event retry without a local event ID are tested.
- Historical named events are preserved, obsolete future events beyond the
  creation horizon are removed, and pending deletes survive midnight/restart.
- Cleanup/migration tests cover interrupted discovery, crash after remote delete,
  inaccessible calendar versus absent event, settings change, fresh installation,
  migration refused before cleanup, and no work tables after successful migration.
- API, generated types, UI and current docs no longer require provisional behavior.

## 13. Implementation Delivery Order

1. Use this final contract and PMS coding conventions; inspect current callers
   and migrations before editing. Sections 7-9 resolve the draft blockers.
2. Implement named-only desired state, safe retry, common horizon, and trigger
   decoupling with regression tests.
3. Implement the offline cleanup command and forward schema transition.
4. Update frontend/API contracts and user-visible partial-failure behavior.
5. Update current docs and supersession pointers, execute relevant checks, and
   review the deployment inventory/results.

The coding agent may implement this specification when instructed by the user.
There are no remaining draft-status or technical-contract approval blockers.
Implementation completion and production deployment must be reported from actual
execution and verification, not inferred from this specification's final status.
