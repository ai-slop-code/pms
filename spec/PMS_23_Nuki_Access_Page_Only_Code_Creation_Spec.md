# PMS_23 - Nuki Access Page Only Code Creation

> Audience: product / property manager and implementing Go / Vue engineer.
> Status: all product decisions confirmed; ready for implementation planning.
> Implementation is not authorized.
> Analysis date: 2026-09-14.
> Evidence: current repository source and selected existing tests; no live Nuki
> inventory or production database inspection was performed. Tests were not run
> for this documentation-only analysis.
> Confirmed product requirements and engineering recommendations are explicitly
> separated. Engineering recommendations remain implementation design choices.

## 1. Objective and Confirmed Requirements

1. Creating a named stay must no longer generate a Nuki access code.
2. Nuki code creation must be initiated exclusively through the Nuki Access page.
   Other PMS workflows must not create codes indirectly.
3. No one-time cleanup of existing generated codes is required for this change.
   Scheduled expiry cleanup remains enabled.
4. This deliverable is analysis and a specification only. Application changes
   require a subsequent instruction.

The business workflow becomes: save the named stay, then explicitly create its
guest PIN in Nuki Access when the operator wants to provision access. Saving a
stay and provisioning guest access become separate business actions.

5. Stay date/name changes automatically update an existing PIN's validity/label;
   cancellation and no-show automatically revoke existing access.
6. Generation is strictly one stay at a time. Remove bulk generation; the user
   confirms no script uses the bulk API.
7. Retain Occupancy's visual Nuki markers.
8. Retain current eligibility, time boundaries, worklist limits, canonical-name
   editing semantics, and missing-code message/copy behavior.
9. Generating for stay A must not revoke or modify stay B's code.

Automatic maintenance must preserve the existing PIN value and credential
identity. It must never fall back to creating a replacement. The additional
confirmed lifecycle details are recorded in section 3.8.

## 2. Verified Current Behavior

### 2.1 Entry-point inventory

| Entry point | Current behavior | Required implication |
|---|---|---|
| `postStay` | Persists the stay, then calls `triggerNamedStayNukiGeneration` synchronously | Remove automatic creation |
| `postBookingBlockPromote` | Promotes raw evidence into a named stay, then calls the same creation helper | Remove automatic creation here too |
| `patchStay` | Name/date/type changes invoke `reconcileNamedStayNuki` | Must not create a missing or replacement code |
| `patchStayStatus` | Status changes reconcile Nuki, including reactivation | Reactivation must not silently recreate access |
| `patchStayOutcome` | Outcome changes reconcile Nuki | Clearing no-show/non-refundable cancellation must not create access |
| `patchStayReview` | Confirmation/rejection reconciles Nuki | Becoming eligible must not create access |
| Nuki `POST /api/properties/{id}/nuki/codes/generate` | Single-stay generation with `stay_id` and nonblank `pin_name`; absent stay ID invokes bulk generation | Require a selected stay; remove bulk dispatch |
| Nuki page name-field blur | Saves `pin_name` by updating the named stay's `DisplayName`; does not directly generate | Saving a name must remain distinct from Generate |
| Nuki `sync/run`, page refresh, post-generation refresh | `SyncProperty` lists provider credentials and reconciles local cache/linkage | No provider credential creation is present in this path |
| Scheduled `nuki_cleanup` | Revokes expired generated credentials and clears local PIN/link fields | Retain scheduled cleanup; no one-time cleanup |
| Finance evidence confirmation | Can set stay generation metadata to `pending`; does not call provider creation | Remove misleading automatic-pending semantics |

Primary evidence:

- `backend/internal/api/occupancy_named_stay_handlers.go`: create/promote hooks,
  patch/status/outcome/review hooks, generation status DTO and helper functions.
- `backend/internal/api/nuki_handlers.go`: generation command, permissions,
  provider-cache refresh, name saving, reveal, revoke and keypad operations.
- `backend/internal/nuki/service.go`: generation, reconciliation, cache sync,
  cleanup, and provider client calls.
- `backend/cmd/server/main.go`: scheduled expiry cleanup and entry-log jobs.
- `backend/internal/store/finance_booking_payouts.go`:
  `ConfirmNamedStayWithFinanceEvidence` writes `pending` metadata.

The inspected production Go call sites of `CreateAccess` are both inside
`generateCodesInternal`: initial creation and the create branch of an upsert.
Production callers of the generation service are the Nuki generation handler
and named-stay lifecycle code. No scheduled generation caller was found.

### 2.2 Creation and maintenance are currently coupled

`ReconcileNamedStay` delegates eligible stays to `GenerateCodeForNamedStay`.
That operation can update an existing credential, create a new one, recreate a
revoked one, or create after a missing/stale external link is discarded.
Removing only the create/promote hooks therefore does not satisfy the request.

`generateCodesInternal` also performs a property-wide revocation pass **before**
filtering creation/update work to the selected stay. Clicking Generate for stay
A can currently revoke stay B's code. Revocation increments the same processed
counter used to decide whether the selected stay was found, so unrelated work
can mask an invalid/ineligible selected stay and produce apparent success.

### 2.3 Eligibility and validity

`ListNamedStaysForNukiSync` selects:

- active named stays;
- confirmed review state, treating NULL as confirmed;
- `booking_com` or `external` stay types;
- neither `no_show` nor `cancelled_non_refundable` outcome;
- checkout date on or after the current **UTC date**.

There is no upper creation horizon in this query. Ongoing stays and stays whose
checkout time has already passed today may qualify by date. Actual PIN validity
uses property timezone and profile check-in/out times (`namedStayWindow`).

`ListUpcomingStaysForNuki` uses similar selection but **omits the outcome
exclusion**. The page can consequently offer generation for a stay the generator
will not process. The page asks for 120 rows, with no stay-list pagination in the
current view. The user chose to retain current eligibility and worklist behavior.
Pagination, search, exact-checkout expiry validation and listing-filter changes
are therefore outside this change. The command must still reject an ineligible
selected stay truthfully rather than return a no-op success.

Raw booking blocks and availability blocks are not Nuki generation owners.
Code identity is `(property_id, named_stay_id)`, not guest name or raw event ID.

### 2.4 State and UI coupling

- `CreateNamedStayRecord` initializes eligible stays to `pending`.
- `named_stays` carries `nuki_generation_status`, error, and updated-at metadata.
  The schema allows `not_applicable`, `pending`, `generated`, `error`.
- `nuki_access_codes` has a separate lifecycle, including `not_generated`,
  `generated`, and `revoked`. The Nuki upcoming-stays projection derives
  `not_generated` when no code exists, so it already lists unprovisioned stays.
- The handler's reconciliation helper marks eligible stays `generated` after a
  nil service error rather than independently verifying a usable code exists.
  This would be incorrect if reconciliation became an update-only no-op.
- `OccupancyView.vue` has create/promote success messages saying Nuki generation
  needs attention. It and `OccupancyCalendar.vue` render generation error state.
- `NukiView.vue` exposes per-stay Generate, followed by cache refresh and PIN
  reveal. Page opening only loads data. `nuki/status.ts` permits Generate for
  missing/not-generated/revoked state, and hides it for generated state.
- Nuki generation requires property-scoped `NukiAccess` write permission. Stay
  handlers require `Occupancy` write permission and currently trigger generation
  without separately requiring Nuki write permission.

### 2.5 Downstream consumers

Messages consume an existing named-stay code and expose `nuki_available`; missing
codes already have a placeholder path. Dashboard Nuki widgets display existing
generated access. Guest entry analytics use external credential-to-stay links.
Cleaner logs use the configured cleaner authorization separately.

Making creation manual increases the normal period during which an eligible
stay has no PIN. Message generation, dashboard loading, finance confirmation,
and entry-log reconciliation must not compensate by generating one.

## 3. Product Decision Record

The following answers were supplied by the user after the initial analysis.
They resolve Q1-Q7's main scope decisions; implementation remains unauthorized.

### 3.1 Q1 - Automatic existing-code maintenance retained

Stay date/name changes automatically update an existing PIN. Cancellation and
no-show automatically revoke it. Separate update/revoke-only maintenance from
explicit creation. Preserve PIN value and provider identity during updates;
missing/stale/revoked credentials cannot trigger automatic replacement creation.
Reactivation and clearing an outcome must not regenerate revoked access.

The chosen policy permits automatic access-window extensions when stay dates
are extended. Maintenance errors must remain visible after the stay is saved.
The additional revocation cases and alternate naming route are confirmed
under section 3.8.

### 3.2 Q2 - Scheduled cleanup retained

“No cleanup” means no one-time cleanup during this change. Scheduled expiry
cleanup stays enabled. Existing codes need no migration-specific deletion,
replacement, or relinking; normal future maintenance/expiry still applies.

### 3.3 Q3 - Strict single-stay generation

Require a positive `stay_id` and nonblank `pin_name`. Reject empty/missing/null
stay selection and remove the bulk service/dispatch path. The user confirms no
script uses the bulk API; no compatibility period for bulk generation is needed.

Technical boundary: a browser page calls an HTTP API, and an authorized caller
can reproduce that request. Enforce a dedicated authenticated Nuki creation
command exposed only by Nuki Access within PMS, with no indirect business/job
callers. `Referer`, trigger strings or `from_page` are not authorization proof.
This application boundary cannot prevent independent use of Nuki's own app/API.

### 3.4 Q4 - Retain visual markers

Keep Occupancy's visual Nuki marks and truthful error state. Intentionally
ungenerated access must not appear as queued work or a failed operation.
No Generate/creation Retry action may be added outside Nuki Access.
New navigation links, reminders, filters and dashboard worklists are not part
of this confirmed scope; retaining markers does not authorize those additions.

### 3.5 Q5 - Eligibility and worklist stay as-is

Preserve current Booking.com/external generation eligibility, review/outcome
rules, UTC-date checkout selection, ongoing-stay behavior, no upper horizon,
property-timezone validity calculation, and the current 120-row page request.
Do not add pagination/search or exact-checkout-instant rejection in this change.

The listing/generator outcome mismatch remains a documented existing issue;
do not silently broaden “stay as-is” into listing-filter remediation. The
single-stay command must still return a truthful ineligible/not-found result.
This retains the limitation that some shown stays cannot generate and stays
beyond the current worklist limit are not exposed there.

### 3.6 Q6 - Naming and messages stay as-is

“PIN name” continues editing the canonical named-stay display name; no independent
label model is introduced. Preserve existing missing-code message placeholders
and copy behavior; no new requirement to generate before copying a message.
Automatic date/name maintenance removes the need for a new manual Update action.
The Nuki name-save route also gains immediate update-only provider-label
maintenance, as confirmed in section 3.8.

### 3.7 Q7 - Isolate the selected stay

Generating for A must not revoke or modify B's code. Remove the property-wide
revocation pass from selected-stay provisioning and validate/process only A.
Ordinary cache refresh may still observe other provider rows; it must not
perform remote mutations for other stays. An unrelated processed counter must
never make an invalid or ineligible selected stay appear successful.

### 3.8 Confirmed lifecycle clarifications

The user explicitly confirmed both follow-up decisions:

1. **Additional revocation transitions:** Retain automatic revocation when a
   stay is archived, its review is rejected, or its type becomes ineligible.
   Retain these existing revocation rules, alongside confirmed
   cancellation/no-show revocation, without any automatic recreation.
2. **Name changes from Nuki Access:** The current blur/save-name endpoint updates
    the canonical stay name without immediately updating the provider label.
   Invoke the same automatic update-only maintenance after this save. It must
   never generate a missing PIN. This extends that route's maintenance behavior
    while retaining Q6's canonical-name ownership semantics.

The state-storage design in section 6 remains an engineering choice to document
before implementation, rather than an unanswered business-policy question.

## 4. Target Business Contract

The following are mandatory consequences of the confirmed request:

1. Direct creation and raw-block promotion persist a stay without attempting
   provider creation, generating a random PIN, or creating a Nuki generation run.
2. Subsequent stay edits/review/status/outcome changes cannot create a code.
3. Becoming eligible means “available for manual generation,” not “queued.”
4. Missing credentials, a Nuki outage, or an absent Nuki service cannot cause a
   new stay to report a failure of a generation operation that was not requested.
5. No background, sync, message, dashboard, finance, save-name, or automatic retry
   path may create or recreate provider credentials.
6. New/replacement creation requires an explicit command initiated from Nuki
   Access. Page entry, refresh, sync, and name saving must not imply consent.
7. Codes generated before deployment remain intact; this feature performs no
   one-time deletion, regeneration, PIN rewrite, or code ownership migration.
8. Named stays remain the sole business owners of managed guest codes.

Confirmed target lifecycle behavior:

| Situation | Creation rule | Other lifecycle behavior |
|---|---|---|
| Eligible stay saved with no code | No creation | Passive not-generated state |
| Stay reviewed/returned to active; no usable code | No creation | Present manual action if eligible |
| Dates/name changed; usable code exists | No replacement creation | Automatically update label/window, preserving PIN and provider identity |
| Existing external credential is missing/stale/revoked | No automatic replacement | Show manual recovery; no upsert fallback |
| Cancellation/no-show | No creation | Automatically revoke existing access |
| Archive/rejected review/ineligible type | No creation | Automatically revoke existing access |
| Name saved from Nuki Access; usable code exists | No replacement creation | Automatically update provider label, preserving PIN and provider identity |
| Code expires | No recreation | Retain scheduled expiry cleanup |
| User chooses Generate in Nuki Access | Validate then create/link eligible selected stay | Process only the selected stay; no bulk or unrelated revocation |
| User repeats Generate | Avoid duplicate remote credentials | Existing-code/retry outcome must be truthful |

## 5. Go Implementation Impact

### 5.1 Stay handlers

In `backend/internal/api/occupancy_named_stay_handlers.go`:

- Remove creation calls from `postStay` and `postBookingBlockPromote` and remove
  `triggerNamedStayNukiGeneration` when unused.
- Change every lifecycle hook, not just initial creation, to use an explicitly
  update/revoke-only operation. Apply automatic date/name maintenance and
  cancellation/no-show/archive/rejected-review/ineligible-type revocation.
- Remove the “eligible plus nil error means generated” assumption from
  `reconcileNamedStayNuki`. Base operational state on actual credential evidence.
- Keep stay persistence and any approved external maintenance failure distinct:
  the stay has already committed. A maintenance error is not failed stay save.
- Adjust `namedStayResponse` according to the chosen passive-state/API policy.
- Preserve stay nights, overlaps, raw source links, audit behavior, finance
  effects, and cleaning reconciliation. PMS 22 is a separate draft touching
  these same handlers; reconcile the shared response design during implementation.

### 5.2 Nuki service boundary

In `backend/internal/nuki/service.go`:

- Split explicit provisioning from automatic existing-code maintenance. Prefer
  distinct methods/private helpers over a general upsert with an easily missed
  `allowCreate` flag. Trigger strings identify runs; they do not authorize work.
- Every provider `CreateAccess` branch must be reachable only from the explicit
  Nuki generation command. Audit stale-link, revoked, missing-ID and failed
  update branches; update-only must fail or report manual action, never create.
- Remove `GenerateCodes` and the optional-stay bulk dispatch.
- Replace global target/revocation scanning for a single command
  with selected-stay validation and processing. Validate property ownership,
  eligibility and time window before any remote mutation.
- Return meaningful selected-stay results. Unrelated processed counts cannot
  prove that generation succeeded. A partial/failed operation must not produce
  a successful generation banner or success-only audit record.
- Reuse an existing usable code on repeated requests. Define recovery for
  provider success followed by timeout/local persistence failure: reconcile
  provider evidence before another explicit create. A local unique stay key
  does not by itself prevent duplicate remote creation under concurrent calls.
  Use operation serialization/ownership suitable for multiple server instances
  where needed; avoid holding a SQL transaction across provider network calls.

`SyncProperty` currently creates no provider access. Preserve that boundary,
including its automatic post-generation usage. Local cache/link reconciliation
must not be accidentally converted into a generate-missing operation.

### 5.3 API and permissions

In `backend/internal/api/nuki_handlers.go`:

- After the Nuki name-save route persists the canonical stay name, invoke the
  same update-only maintenance for an existing credential. A missing PIN is a
  no-create case; a real maintenance failure remains visible after the name save.
- Keep property-scoped `NukiAccess` write authorization for explicit creation.
  Occupancy write alone must have no way to cause creation.
- Reject missing/empty/null/nonpositive stay ID
  and missing/blank label rather than interpreting them as bulk generation.
- Distinguish invalid input, not-found/property mismatch, ineligible stay,
  provider failure, and successful creation/reuse in the contract. Choose HTTP
  and action-response conventions consistently with the repository.
- Cache-refresh failure after successful provisioning is a refresh failure,
  not justification to retry creation automatically.
- Keep PIN masking, write-gated audited reveal, and property isolation intact.
- Audit actor, property, selected stay, operation and result without plaintext
  PINs. Retain historical trigger labels in run display.

## 6. State, Schema and API Contract Strategy

The creation restriction itself requires no access-code schema migration.
The important state change is that eligible/unprovisioned is no longer pending.

**Recommended engineering design (visual markers must remain):** derive passive access state from eligibility
and `nuki_access_codes`, using `not_generated` for eligible stays with no usable
credential. Deprecate redundant stay-owned generation metadata as operational
truth; preserve existing values as history if retained. Expose eligibility
separately from whether a credential exists or a real operation failed.

Implementation must choose and document one coherent strategy:

- Retain columns but stop auto-pending writes; adapt projections and consumers
  so legacy values do not display as live queued work. This avoids rewriting
  existing code records and may avoid a schema migration.
- Alternatively, retain stay-owned operational state with an explicit
  `not_generated` value. This requires a new forward migration because the
  existing CHECK constraint does not permit it, plus consistent writers.
- Removing metadata columns is optional broader cleanup, not required by this
  feature. Do not change historical migrations to achieve either strategy.

Affected places:

- `backend/internal/store/named_stays.go`: initialization, state validation,
  scans, and `MarkNamedStayNukiGeneration` if retained.
- `backend/internal/store/finance_booking_payouts.go`: stop treating finance
  confirmation as pending generation; preserve finance review confirmation.
- `backend/internal/store/stay_calendar.go`: calendar state projection.
- `backend/internal/store/nuki.go`: preserve current listing/command eligibility
  and named-stay linkage; use existing generator rules to validate the selected
  stay without treating unrelated work as success.
- `spec/openapi.yaml`: Nuki request contract, stay mutation response and
  calendar DTO state semantics.
- `frontend/src/api/types/generated.ts`: regenerate from OpenAPI after approval;
  also update handwritten `occupancy.ts` and `nuki.ts` where contracts change.

Do not bulk reset existing `nuki_access_codes`, keypad rows, PINs, external IDs,
validity windows, statuses, run/event history, or guest-entry links as rollout
work. Any proposed metadata migration must explicitly distinguish historical
generation metadata from operational credentials and be reviewed separately.

## 7. Frontend and Operational Workflow Impact

### Occupancy

- `frontend/src/views/OccupancyView.vue`: remove automatic-generation failure
  wording from create/promote completion. State that the stay was saved.
- Retain passive marks; do not introduce new navigation/worklist features or
  Generate/creation Retry actions outside Nuki Access.
- `frontend/src/views/occupancy/OccupancyCalendar.vue`: align badge/tooltip state
  with Q4. Intentionally missing access is not a failed generation attempt.

### Nuki Access

- `NukiView.vue`, `nuki/NukiUpcomingStays.vue`, `nuki/status.ts`: keep explicit
  per-stay Generate as the creation interaction; distinguish label save, refresh,
  sync, generation and reveal failures.
- Preserve existing eligibility/list rendering. A rejected generation command
  must show its real error instead of a no-op success banner.
- Keep automatic maintenance; no new manual Update action is required.
- Reflect Nuki read/write permissions in action visibility/disabled state,
  in addition to backend enforcement.
- Preserve the current worklist limit and name-editing model. Recover from failed
  provisioning through the existing explicit per-stay generation workflow.

### Other modules

- Messages retain current missing-code and copy behavior until explicit
  generation; no create-on-demand fallback.
- Dashboard passive display remains. It must not become a
  second creation surface.
- Preserve cleaner salary/log behavior and existing guest-entry attribution.
  New stays without provisioned/linked access will naturally lack that linkage.

## 8. Verification and Acceptance Criteria for Later Implementation

Use a counting/failing fake Nuki client to assert actual provider calls, rather
than proving only that a button disappeared or a helper was renamed.

### Mandatory boundary tests

1. Direct named-stay creation and raw-block promotion succeed with configured,
   failing and absent Nuki services, with zero `CreateAccess` calls and no
   generation attempt/run created by those actions.
2. Editing name, dates or type; confirming review; reactivating; and clearing
   outcomes all result in zero creation calls when a code is absent.
3. Repeat lifecycle tests with revoked, missing external ID, stale external link,
   and failed existing-code update. No fallback creates a replacement.
4. Occupancy-write/Nuki-no-write users can save stays and cannot generate codes.
   Nuki read-only users cannot generate; property-mismatched stay IDs cannot
   cause any provider mutation.
5. Explicit Generate creates a valid code for an eligible selected named stay.
   Refresh, sync, label save, messages, dashboard and background jobs create none.
6. New stays and finance-confirmed stays are not shown as automatically pending.
   No Nuki outage warning is invented for an unrequested generation operation.
7. Existing generated records survive deployment logic without PIN/external-ID,
   window, linkage or status rewrite. No one-time provider cleanup is invoked.
8. Repeated/concurrent manual requests and retry after ambiguous provider success
   do not blindly create duplicates; failure/recovery behavior is explicit.
9. Real generation failures are visible in Nuki Access and retried only there.
   A failed cache refresh cannot masquerade as failed provider creation.

### Confirmed lifecycle and scope tests

- Date/name edits update the existing provider credential and preserve its PIN
  and external ID. Cancellation/no-show revoke access. Failed updates must not
  fall back to creation; revoked access must not be regenerated automatically.
- Verify scheduled expiry cleanup remains enabled and behaves as before.
- Reject empty body, `{}`, null/missing/invalid `stay_id` with zero provider
  calls. The bulk generation dispatch and service path no longer exist.
- Preserve current outcomes, stay types, ongoing stays, UTC checkout-date filter,
  timezone validity calculation, no upper horizon and current worklist limit.
- Generating A does not revoke or otherwise mutate B; invalid/ineligible A
  cannot return success merely because B was processed.
- Retain visual markers, canonical naming semantics and existing missing-code
  message/copy behavior. No new navigation/filter/pagination is required.
- Verify archive, rejected-review and ineligible-type transitions revoke existing
  access without recreation. Verify Nuki name-save updates an existing provider
  label while preserving PIN/identity, makes zero creation calls when no usable
  credential exists, and surfaces real maintenance failures after name save.

Existing test files to adapt/extend:

- `backend/internal/api/occupancy_named_stay_handlers_test.go`
- `backend/internal/nuki/service_test.go` (currently expects reconciliation to
  generate and recreate on reactivation)
- `backend/internal/store/named_stays_test.go` (currently expects `pending`)
- `backend/internal/store/stay_calendar_test.go`
- `backend/internal/api/nuki_pin_reveal_test.go`
- `backend/internal/api/message_handlers_test.go`
- `frontend/src/views/NukiView.spec.ts`
- `frontend/src/views/OccupancyView.spec.ts`
- `frontend/src/views/occupancy/OccupancyCalendar.spec.ts`

Run targeted Go API/store/Nuki tests, relevant frontend suites and type checking,
and the repository's required contract-generation/route-coverage checks during
implementation. Build/test results must be recorded then, not inferred from this
analysis.

## 9. Delivery Sequence and Documentation Authority

1. Document the state-storage choice and obtain a separate instruction
   authorizing implementation. All product decisions, including section 3.8,
   are confirmed.
2. Restrict provider creation entry points and remove stay creation hooks.
3. Implement the selected existing-code maintenance policy and selected-stay
   command isolation; close all replacement-creation escape paths.
4. Align metadata, finance confirmation, API contracts and frontend status.
5. Preserve existing worklist/naming/message behavior, align visual state, and
   verify the acceptance matrix.
6. Deploy without existing-code cleanup. Verify a new stay has no code until the
   explicit page action, and an existing code retains its stored credential.

Retire old application instances that still contain stay-triggered generation;
otherwise requests served by them can violate the new rule. Do not run a
generate-all/backfill job during rollout. Rolling back to old binaries restores
automatic generation behavior and must not be described as policy-neutral.

On approval, this spec supersedes automatic named-stay-triggered Nuki creation
statements in PMS 20 and historical PMS 21 migration/remediation documents.
PMS 21's final named-stay ownership model remains authoritative. Update current
Nuki module documentation in `PMS_02_Module_Specifications.md`, relevant checks
in `PMS_03_Implementation_Checklists.md`, and the spec index to avoid reintroducing
automation from historical instructions. PMS 22 remains the separate cleaning
calendar decision record.

No spec-index or existing-spec edits were included in this analysis deliverable;
the workspace already contains in-progress PMS 22/index changes. This file is
the standalone draft and does not claim implementation or deployment.
