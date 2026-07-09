# PMS_18 - Manual Cleaning Calendar Exclusion Spec

> Audience: product / property manager + implementing engineer.
> Scope: occupancy-level control that suppresses PMS-managed Google Calendar cleaning events for selected real guest stays.
> Status: future feature specification only; no implementation in this change.

## 1. Problem Framing

PMS currently creates Google Calendar cleaning events automatically for eligible guest checkouts. This is the correct default because most stays need to appear in the cleaning lady's calendar.

There are operational exceptions where a real guest stay still happens, but the cleaning lady should not receive that cleaning event. For example, the cleaning lady is unavailable and the owner arranges cleaning another way. The stay must remain a normal occupied guest stay for occupancy, finance, analytics, guest messaging, and access-code purposes, but the PMS-managed Google Calendar cleaning event for that checkout must be removed.

The feature should add a simple UI action for selected stays: by default every eligible stay is cleaned by the cleaning lady; when the owner marks a stay as not cleaned by the cleaning lady, the associated PMS-managed Google Calendar cleaning event is deleted or not created.

## 2. Definitions

- **Cleaning calendar exclusion**: a manual occupancy-level flag saying this stay's checkout should not create or keep a PMS-managed Google Calendar cleaning event.
- **Cleaned by cleaning lady**: the default state. The occupancy follows normal PMS_15 cleaning-calendar eligibility rules.
- **Not cleaned by cleaning lady**: the override state. The occupancy remains a real guest stay, but PMS suppresses the cleaning calendar event for its checkout.
- **Excluded checkout**: a checkout occupancy with the manual cleaning calendar exclusion applied.

## 3. Product Decisions

- Default behavior remains unchanged: all eligible guest stays create cleaning calendar events.
- The exclusion is manual and occupancy-specific.
- The exclusion affects only PMS-managed Google Calendar cleaning events.
- The exclusion must not change occupancy status, closure state, stay outcome, finance values, cancellation metrics, Nuki access-code behavior, or guest messaging behavior.
- Re-running occupancy sync must preserve the exclusion.
- The UI must allow the owner to restore the default behavior at any time.
- PMS should keep an audit trail for who changed the exclusion and when.
- Applying or clearing the exclusion must trigger cleaning-calendar reconciliation immediately, using the same pattern as PMS_17 stay outcome actions. Waiting only for the scheduled reconciliation job is not acceptable UX.
- The MVP should not add a new `cleaning_calendar_events.removal_reason` column. Use the existing removed-event `error_message` field and `cleaning_calendar_event_logs.message` for the human-readable removal reason.

## 4. Business Analyst Challenge Questions And Decisions

These are the questions a business analyst should ask before implementation. The answers below are part of the implementation contract so an AI coding agent does not need to pause for clarification.

1. Is this a property-wide switch, a date-range exception, or a per-stay exception?

Answer: per-stay exception only. A property-wide Google Calendar sync toggle already exists in PMS_15. Date-range bulk exclusions are out of scope for this feature.

2. Does the stay still count as occupied and financially normal?

Answer: yes. This is a real guest stay. Occupancy, finance, cancellation-rate, Nuki, and guest messaging behavior must remain unchanged.

3. Who is allowed to mark this?

Answer: use `permissions.Occupancy` with `permissions.LevelWrite`, matching the existing occupancy closure and stay-outcome actions. Do not introduce a new permission for this feature.

4. Should externally sold stays support the exclusion?

Answer: yes. PMS_15 treats externally sold stays as real guest stays that create cleaning events. Therefore external-sale rows must be eligible for manual cleaning-calendar exclusion unless they are otherwise ineligible.

5. Should closed maintenance blocks support the exclusion?

Answer: no. Closed blocks do not represent guest checkout cleaning and are already ineligible for cleaning events.

6. Should a stay marked `cancelled_non_refundable` or `no_show` also be allowed to receive this exclusion?

Answer: no new exclusion should be applied while `stay_outcome` is set because cleaning is already suppressed by PMS_17. If an exclusion was applied first and a stay outcome is applied later, both flags may coexist; clearing only the PMS_18 exclusion must not recreate the event while the PMS_17 outcome remains set.

7. Should a manually excluded arriving stay still make the previous checkout a same-day turnover?

Answer: yes. The arriving guest is real. Manual cleaning exclusion affects only the marked stay's own checkout event.

8. Should the action delete the Google Calendar event immediately?

Answer: yes, best effort. The API action must save the flag and then call cleaning-calendar reconciliation immediately. If Google deletion fails, the API should return `200` with `ok: false` and an explanatory `error`, following the existing PMS_17 outcome handler pattern.

9. What should happen if Google Calendar sync is disabled or not configured?

Answer: the exclusion flag must still be saved. Reconciliation may be a no-op when sync is disabled. The UI should still show the exclusion because it will matter if sync is enabled later.

10. Should a reason be mandatory?

Answer: no. The reason is optional, trimmed, and limited to 500 characters, matching existing occupancy manual override reason limits.

11. Should clearing the exclusion recreate past cleaning events?

Answer: only if the checkout remains inside the existing cleaning-calendar reconciliation window. The current service reconciles from 30 days in the past to 365 days in the future.

12. Is this intended to track who actually cleaned the apartment?

Answer: no. The feature only controls whether PMS sends the checkout task to the cleaning lady's Google Calendar. It does not create an owner-cleaned log, payroll adjustment, or cleaning completion workflow.

13. If multiple suppression reasons apply, which removal reason should PMS show?

Answer: show the strongest current business reason. `stay_outcome` takes precedence because it means the guest did not physically use the apartment. Manual cleaning-calendar exclusion is shown only when there is no PMS_17 stay outcome.

14. Should UI labels be English or Slovak?

Answer: English is confirmed for this feature. Use labels such as `Cleaning lady: Yes`, `Cleaning lady: No`, `Do not send cleaning event`, and `Mark as cleaned by cleaning lady`.

15. Should the exclusion be visible directly on the occupancy calendar?

Answer: yes. Show a small, non-intrusive badge on affected calendar stay chips or day details so the owner can see excluded stays without opening the full list. The badge copy can be short, for example `No cleaning event`.

## 5. Relationship To Existing Specs

This feature extends PMS_15 cleaning-calendar eligibility with one more manual exclusion condition.

It is different from PMS_17 stay outcome overrides:

- PMS_17 covers cases where the guest does not physically use the apartment, such as no-show or non-refundable cancellation.
- PMS_18 covers normal guest stays where cleaning is handled outside the cleaning lady's calendar.

Do not reuse `stay_outcome`, `closure_state`, or `occupancies.status` for this feature. Those fields carry different business meanings and would distort analytics.

## 6. Functional Requirements

- The Occupancy UI must expose a control for eligible stays to mark whether the checkout should be cleaned by the cleaning lady.
- The default state for all existing and newly imported occupancies is cleaned by cleaning lady.
- Marking a stay as not cleaned by cleaning lady suppresses the PMS-managed Google Calendar event for that stay's checkout.
- Marking a stay must trigger cleaning-calendar reconciliation immediately after the database update.
- Clearing the exclusion restores normal PMS_15 eligibility and must trigger cleaning-calendar reconciliation immediately after the database update.
- The action should be available for real guest stays in active downstream status, for example `active` or `updated`.
- The action should be disabled for closed maintenance blocks because they do not represent guest checkout cleaning.
- The action must be allowed for externally sold stays because they represent real guest stays and currently create cleaning events.
- The action should be disabled for rows with `stay_outcome` set because PMS_17 already suppresses cleaning.
- The operator may enter an optional reason, for example `Cleaner unavailable; owner will clean`.
- PMS must audit when the exclusion is applied, cleared, or changed.
- Re-running occupancy sync must not clear the exclusion.

## 7. Backend Model

Add cleaning-exclusion fields to `occupancies`:

- `cleaning_calendar_excluded` integer/boolean not null default `0` / `false`.
- `cleaning_calendar_exclusion_reason` text nullable.
- `cleaning_calendar_excluded_by_user_id` integer nullable, FK to `users.id`.
- `cleaning_calendar_excluded_at` text nullable.

Add migration files after the current latest migration:

- `backend/internal/migrate/000028_cleaning_calendar_exclusion.up.sql`.
- `backend/internal/migrate/000028_cleaning_calendar_exclusion.down.sql`.

Recommended up migration:

```sql
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_exclusion_reason TEXT;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded_at TEXT;

CREATE INDEX idx_occupancies_property_cleaning_calendar_excluded
    ON occupancies (property_id, cleaning_calendar_excluded);
```

Recommended down migration:

```sql
DROP INDEX IF EXISTS idx_occupancies_property_cleaning_calendar_excluded;
ALTER TABLE occupancies DROP COLUMN cleaning_calendar_excluded_at;
ALTER TABLE occupancies DROP COLUMN cleaning_calendar_excluded_by_user_id;
ALTER TABLE occupancies DROP COLUMN cleaning_calendar_exclusion_reason;
ALTER TABLE occupancies DROP COLUMN cleaning_calendar_excluded;
```

When the exclusion is cleared:

- Set `cleaning_calendar_excluded = false`.
- Clear `cleaning_calendar_exclusion_reason` unless audit history stores the previous reason separately.
- Clear `cleaning_calendar_excluded_by_user_id`.
- Clear `cleaning_calendar_excluded_at`.

Recommended audit event names:

- `occupancy_cleaning_calendar_exclude`.
- `occupancy_cleaning_calendar_include`.

The implementation must not mutate these fields during ICS occupancy upsert unless the source occupancy is intentionally hard-deleted by retention cleanup.

## 8. Implementation Contract

This section maps the spec to the current codebase. An AI coding agent should implement these exact changes unless the code has changed since this spec was written.

Backend store changes:

- Update `backend/internal/store/occupancy.go` `Occupancy` with `CleaningCalendarExcluded bool`, `CleaningCalendarExclusionReason sql.NullString`, `CleaningCalendarExcludedByUserID sql.NullInt64`, and `CleaningCalendarExcludedAt sql.NullTime`.
- Update `occupancySelectColumns` to select the four new fields after the existing stay-outcome fields.
- Update `scanOccupancies` to scan the new values. Parse `cleaning_calendar_excluded_at` the same way `stay_outcome_marked_at` is parsed.
- Add `MarkOccupancyCleaningCalendarExcluded(ctx, propertyID, occupancyID, userID int64, reason string) error`.
- Add `ClearOccupancyCleaningCalendarExcluded(ctx, propertyID, occupancyID int64) error`.
- Add `ErrOccupancyCleaningCalendarExclusionIneligible` or reuse an equivalent conflict error so ineligible rows return `409 Conflict` rather than `500`.

Store validation for `MarkOccupancyCleaningCalendarExcluded`:

```sql
UPDATE occupancies
SET cleaning_calendar_excluded = 1,
    cleaning_calendar_exclusion_reason = ?,
    cleaning_calendar_excluded_by_user_id = ?,
    cleaning_calendar_excluded_at = ?,
    last_synced_at = ?
WHERE property_id = ?
  AND id = ?
  AND status IN ('active', 'updated')
  AND (closure_state IS NULL OR closure_state <> 'closed')
  AND stay_outcome IS NULL
```

Notes:

- Allow `closure_state = 'external_sale'`.
- Applying the exclusion to an already-excluded row should update the reason and marker metadata and return success.
- If the row does not exist, return `sql.ErrNoRows`.
- If the row exists but is closed, cancelled/deleted, or has `stay_outcome`, return a conflict/ineligible error.

Store behavior for `ClearOccupancyCleaningCalendarExcluded`:

```sql
UPDATE occupancies
SET cleaning_calendar_excluded = 0,
    cleaning_calendar_exclusion_reason = NULL,
    cleaning_calendar_excluded_by_user_id = NULL,
    cleaning_calendar_excluded_at = NULL,
    last_synced_at = ?
WHERE property_id = ?
  AND id = ?
  AND cleaning_calendar_excluded = 1
```

Notes:

- Clearing an already-included existing row should be a no-op success.
- Clearing should be allowed even when the row is no longer active, closed, or has `stay_outcome`, so the operator can remove stale metadata.

Cleaning-calendar changes:

- Update `backend/internal/store/cleaning_calendar.go` `ListCleaningCalendarCheckoutCandidates` to add `AND cleaning_calendar_excluded = 0`.
- Do not add `cleaning_calendar_excluded = 0` to `FindCleaningCalendarSameDayArrival`. Excluded arrivals are still real guest arrivals and must still drive the previous checkout's same-day turnover title.
- Update `backend/internal/cleaningcalendar/service.go` `removalReason` so precedence is deterministic: return `stay outcome: {value}` when `StayOutcome.Valid`; otherwise return `manual cleaning calendar exclusion` when `CleaningCalendarExcluded = true`; otherwise return `nil`.
- Keep using `MarkCleaningCalendarEventRemoved(ctx, propertyID, eventID, errMsg)` so the removal reason is visible through the existing `error_message` field and event logs.

API changes:

- Add routes in `backend/internal/api/server.go` under the authenticated property routes:
- `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/exclude`.
- `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/include`.
- Implement handlers near the existing occupancy manual-label handlers, either in `occupancy_closure_handlers.go` or a new `occupancy_cleaning_calendar_handlers.go`.
- Require `permissions.Occupancy` with `permissions.LevelWrite`.
- Use `closureReasonMaxLen` (`500`) for reason length.
- Audit `attempt` before the write and `success` after reconciliation succeeds.
- After a successful exclude, call `s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "cleaning_calendar_exclusion")` when `s.CleaningCalendar != nil`.
- After a successful include, call `s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "cleaning_calendar_inclusion")` when `s.CleaningCalendar != nil`.
- If reconciliation fails after the DB update, return HTTP `200` with `{"ok": false, "error": "cleaning calendar exclusion saved, cleaning calendar failed: ..."}` or `{"ok": false, "error": "cleaning calendar inclusion saved, cleaning calendar failed: ..."}`, matching the PMS_17 outcome action pattern.

API response field changes:

- Update `backend/internal/api/occupancy_handlers.go` `occupancyRow` to include `cleaning_calendar_excluded`, `cleaning_calendar_exclusion_reason`, `cleaning_calendar_excluded_at`, and `cleaning_calendar_excluded_by_user_id`.
- `cleaning_calendar_excluded` should always be present as a boolean.
- The reason, timestamp, and user ID fields may be omitted when null.
- Update `occupancyRows` to populate these fields.

Frontend changes:

- Update `frontend/src/api/types/occupancy.ts` with the four new fields.
- Add helper functions to `frontend/src/views/occupancy/closure.ts` or a renamed shared helper file: `hasCleaningCalendarExclusion`, `canExcludeCleaningCalendar`, and `cleaningCalendarStatusLabel`.
- Update `frontend/src/views/OccupancyView.vue` to add an exclude/include dialog with optional reason and max-length validation.
- Add day-dialog actions alongside existing closure/outcome actions.
- Update `frontend/src/views/occupancy/OccupancyStayList.vue` to show the cleaning-calendar status and actions in list view.
- Update `frontend/src/views/occupancy/OccupancyCalendar.vue` to show a small badge for excluded stays, for example `No cleaning event`.
- Ensure the day-dialog stay rows in `OccupancyView.vue` also show the exclusion state.
- Update or add frontend tests in `OccupancyView.spec.ts` and `occupancy/OccupancyStayList.spec.ts`.

## 9. API Requirements

Suggested endpoints:

- `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/exclude`
- `POST /api/properties/{id}/occupancies/{occupancyId}/cleaning-calendar/include`

Request body for exclude:

```json
{
  "reason": "Cleaner unavailable; owner will clean"
}
```

Response body:

```json
{
  "ok": true
}
```

Validation rules:

- Require occupancy write permission.
- Require the occupancy to belong to the requested property.
- Reject non-active occupancy statuses for the exclude action.
- Reject `closure_state = 'closed'` with `409 Conflict`.
- Allow `closure_state = 'external_sale'`.
- Reject `stay_outcome IS NOT NULL` for the exclude action because PMS_17 already suppresses cleaning.
- Limit reason length to the same shared note limit used by other occupancy manual overrides.
- Applying the same exclusion twice should be idempotent or update only the reason and audit that change.
- Including an already-included stay should be a no-op success.

## 10. Cleaning Calendar Rules

Cleaning eligibility must exclude manually excluded occupancies:

```sql
status IN ('active', 'updated')
AND (closure_state IS NULL OR closure_state <> 'closed')
AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
AND cleaning_calendar_excluded = 0
```

When a stay is marked not cleaned by cleaning lady:

- The next cleaning-calendar reconciliation must delete or cancel the PMS-managed Google Calendar event for that stay's checkout.
- The local `cleaning_calendar_events` row should move to `status = 'removed'` using the existing removal lifecycle.
- The removal reason should be stored or displayed as `manual cleaning calendar exclusion`.
- The deletion must be idempotent.
- If Google deletion fails, PMS must keep the local event in `error` and expose retry exactly like other cleaning-calendar failures.

When the exclusion is cleared:

- The immediate reconciliation triggered by the include API action must recreate the event if all normal PMS_15 eligibility rules still pass.
- The recreated event must use the normal PMS_15 title, timing, same-day-arrival, and idempotency rules.
- PMS must not create duplicates if a previous Google event already exists and was not successfully deleted.

## 11. Same-Day Arrival Rules

The exclusion applies to the checkout event for the marked occupancy only.

- A manually excluded stay should still count as a same-day arrival for the previous stay if guests really arrive that day.
- Marking an arriving stay as not cleaned by cleaning lady must not change the previous checkout's same-day turnover title.
- Marking a checkout stay as not cleaned by cleaning lady removes only that checkout's cleaning event.

This differs from PMS_17 no-show and non-refundable cancellation outcomes, where the arriving stay should not count as a real same-day guest arrival.

## 12. Frontend Requirements

Occupancy UI:

- Show a clear per-stay cleaning-calendar status, for example `Cleaning lady: Yes` or `Cleaning lady: No`.
- Default all stays to `Cleaning lady: Yes` unless the exclusion is set.
- Add an action named `Do not send cleaning event` or `Mark as not cleaned by cleaning lady`.
- Add a restore action named `Send cleaning event` or `Mark as cleaned by cleaning lady`.
- Show a small badge on excluded stays in the occupancy calendar, for example `No cleaning event`.
- Use a confirmation dialog before every exclude action. The Occupancy UI does not need to know whether a Google Calendar event already exists.
- The confirmation copy must explain that only the cleaning lady's calendar event is removed and the stay remains a normal guest stay.
- Show the optional reason in the stay details or action menu when present.
- After a successful action, reload the current occupancy/calendar state and show a success message.
- If the API returns `ok: false` because calendar reconciliation failed after saving the flag, reload the occupancy state and show the error as a warning/error so the operator knows retry is needed.

Suggested confirmation copy:

- Exclude: `This removes the PMS-created Google Calendar cleaning event for this checkout. The stay will remain a normal occupied guest stay.`
- Include: `This restores the default behavior. PMS will create the cleaning calendar event again if the stay is still eligible.`

Cleaning calendar UI:

- Removed cleaning events should show the same `removed` state used by other ineligible occupancies.
- If the event history exposes reasons, show `Manual exclusion: not cleaned by cleaning lady`.
- If the user clears the exclusion, the event table should show the recreated or pending event after reconciliation.

## 13. Analytics, Finance, Nuki, And Messaging Rules

The cleaning calendar exclusion is operational only.

- Occupancy rate must remain unchanged.
- Revenue, ADR, RevPAR, commission, payout, and finance transaction calculations must remain unchanged.
- Cancellation-rate logic must remain unchanged.
- Nuki code generation and revocation must remain unchanged.
- Guest messaging must remain unchanged.
- The stay must not be displayed as cancelled, closed, no-show, or non-refundable cancellation.

## 14. Edge Cases

- Excluding a future stay before the event was created: reconciliation must skip event creation.
- Excluding a stay after the event was created: reconciliation must remove the existing Google event.
- Excluding a stay after checkout has already passed: PMS should mark the local event removed for history; Google deletion can follow the existing retention/deletion policy for past managed events.
- Clearing an exclusion after checkout has passed: PMS should follow the existing reconciliation window. If past events are outside the sync window, show that no event will be recreated automatically.
- Google Calendar permission is revoked when the exclusion is applied: PMS should record an error and expose retry.
- Calendar ID changes while a stay is excluded: PMS must not create that excluded stay's event in the new calendar.
- The same occupancy is later marked no-show or non-refundable cancellation: PMS_17 stay outcome still suppresses cleaning; clearing the PMS_18 exclusion alone must not recreate the event until the stay outcome is also cleared.
- Excluding a stay when Google Calendar sync is disabled: save the flag and do not return an error just because reconciliation is a no-op.
- Excluding an already-excluded stay with a different reason: update the reason, marker user, marker timestamp, and audit the action.
- Clearing an already-included stay: return success without changing unrelated occupancy data.

## 15. Test Focus

Backend tests:

- Newly imported eligible occupancies default to `cleaning_calendar_excluded = false`.
- Occupancy sync preserves `cleaning_calendar_excluded = true` and the reason fields.
- Marking an eligible stay excluded persists the flag and audit event.
- Marking an excluded stay included clears the flag and audit event.
- Excluding an already-excluded stay updates the reason and remains successful.
- Including an already-included stay returns success.
- Exclude action rejects `closure_state = 'closed'`.
- Exclude action allows `closure_state = 'external_sale'`.
- Exclude action rejects rows with `stay_outcome` set.
- Include action clears stale exclusion metadata even if a row later became inactive.
- Cleaning reconciliation removes an existing Google event for an excluded checkout.
- Cleaning reconciliation skips creation for an excluded checkout with no existing event.
- Clearing the exclusion recreates one event when the occupancy is otherwise eligible.
- Re-running reconciliation after clearing the exclusion does not create duplicates.
- Excluded arriving stays still count for previous checkout same-day-arrival title logic.
- `removalReason` returns `manual cleaning calendar exclusion` for excluded stays.
- API action returns `ok: false` with a useful error when reconciliation fails after saving the flag.

Frontend tests:

- Occupancy calendar/list shows the default cleaned-by-cleaning-lady state.
- Excluded stays show a distinct not-cleaned-by-cleaning-lady state.
- Exclude action calls the expected endpoint with the reason.
- Include action calls the expected endpoint.
- Confirmation copy states that occupancy, finance, Nuki, and guest messaging are not changed.
- Cleaning event table shows removed state and manual exclusion reason.
- UI disables the exclude action for stay outcomes and closed rows.
- UI allows the exclude action for external-sale rows.
- UI reloads occupancies after a partial success where the flag saved but calendar reconciliation failed.

## 16. Acceptance Criteria

- Owner can mark a selected stay as not cleaned by the cleaning lady from the UI.
- Owner can restore the selected stay to the default cleaned-by-cleaning-lady behavior.
- All existing and newly imported stays default to cleaned by cleaning lady.
- A marked stay does not create a PMS-managed Google Calendar cleaning event.
- If a PMS-managed Google Calendar event already exists for the marked stay, PMS removes it on reconciliation.
- Clearing the mark recreates exactly one cleaning event when the stay is otherwise eligible.
- The marked stay remains a normal occupied guest stay in occupancy and finance reporting.
- Nuki access codes and guest messaging are not changed by this action.
- Same-day-arrival detection still treats the marked stay as a real arriving guest stay for the previous checkout.
- Sync failures are visible and retryable through the existing cleaning calendar failure flow.
- The API action attempts cleaning-calendar reconciliation immediately and reports partial failure without rolling back the saved exclusion.

## 17. Suggested Delivery Order

1. Add occupancy persistence fields and migrations.
2. Add API actions with validation and audit events.
3. Update cleaning-calendar eligibility and reconciliation removal reasons.
4. Update occupancy API DTOs and frontend API types.
5. Add Occupancy UI status, actions, reason entry, and confirmation copy.
6. Add Cleaning calendar UI visibility for removed manual-exclusion events.
7. Add backend and frontend tests.

The feature is complete only when the UI action, persisted override, Google Calendar event removal, event recreation after clearing, and same-day-arrival behavior all match this spec.
