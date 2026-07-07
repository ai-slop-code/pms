# PMS_15 - Google Calendar Cleaning Events Spec

> Audience: product / property manager + implementing engineer.
> Scope: native PMS integration that creates cleaning events in a configured Google Calendar after guest check-out.
> Status: future feature; this supersedes the earlier v1-only recommendation to use n8n when native Google Calendar sync is intentionally picked up.

## 1. Problem Framing

The cleaning lady needs a reliable calendar view of upcoming turnover work. Today PMS knows the authoritative occupancy schedule after ICS sync, but the cleaner still depends on manual communication or an external automation. PMS should create Google Calendar events automatically so the cleaner sees each cleaning task without the owner manually entering it.

The core business rule is checkout-driven: as soon as PMS sees a new reservation with a future guest check-out, PMS creates a cleaning event in the configured Google Calendar for the check-out date. If another guest checks in on the same property-local date, the event title must make that same-day turnover visible. If that same-day-arrival status changes later because a new booking appears, moves, or is cancelled, PMS updates the existing cleaning event title instead of creating a duplicate.

## 2. Definitions

- **Cleaning calendar**: the Google Calendar selected by the property owner for PMS-generated cleaning events.
- **Cleaning event**: a Google Calendar event created and managed by PMS for one checkout.
- **Checkout occupancy**: an occupancy that ends on the cleaning date and is eligible to trigger cleaning.
- **Same-day arrival**: another eligible occupancy for the same property whose check-in date, in the property timezone, equals the checkout occupancy's check-out date.
- **Turnover cleaning**: a cleaning where same-day arrival is true and the apartment must be ready before the next guest's check-in time.
- **No-guest cleaning**: a cleaning where no eligible occupancy checks in on the same property-local date.

## 3. Functional Requirements

- Each property can enable or disable Google Calendar cleaning sync.
- Each enabled property stores exactly one target Google Calendar ID for cleaning events.
- Each enabled property stores configurable event-title parts: title prefix, same-day-arrival label, and no-guest label.
- Default title prefix is `Upratovanie:`.
- Default same-day-arrival label is `Pride Host`.
- Default no-guest label is `Bez Hosta`.
- PMS automatically creates one Google Calendar cleaning event for each eligible guest checkout as soon as the reservation is imported or otherwise appears in PMS.
- PMS updates the existing Google event if the source occupancy date, property name, or check-out time changes.
- PMS updates only the existing Google event title when same-day-arrival status changes.
- PMS cancels or deletes the Google event when the source occupancy stops being eligible for cleaning.
- PMS must be idempotent: re-running occupancy sync or the calendar reconciliation job must not create duplicate Google Calendar events.
- PMS must show sync status and the last error for every managed cleaning calendar event.
- PMS must provide a manual retry action for failed Google Calendar syncs.

## 4. Eligibility Rules

A checkout creates a cleaning event when all of the following are true:

- The property has Google Calendar cleaning sync enabled.
- The property has a configured cleaning calendar ID.
- The checkout occupancy status is active for downstream automation, for example `active` or `updated`.
- The checkout occupancy is not labelled `closure_state = 'closed'`.
- The checkout date is known after converting the occupancy end into the property timezone.

Externally-sold stays represent real guest stays and must create cleaning events.

Cancelled occupancies, deleted-source occupancies, and closed maintenance blocks must not create cleaning events. If a Google event already exists for one of these rows, PMS reconciles it away.

## 5. Event Timing Rules

All date and time calculations use the property's configured timezone.

- Cleaning date = checkout occupancy's check-out date in property timezone.
- Event start = cleaning date at the property's configured check-out time, for example `09:00`.
- If there is a same-day arrival at the time the cleaning event is created, event end = one hour before the arriving occupancy's configured check-in time.
- If there is no same-day arrival, event end = start + the property's configured default cleaning duration.
- Default cleaning duration is property-configurable, with a recommended default of 3 hours. This duration is used only when there is no same-day arrival.
- If same-day-arrival status changes later because another booking is imported, moved, cancelled, deleted, or closed, PMS updates only the event title and does not reschedule the existing Google Calendar event.
- If the calculated same-day end time is equal to or earlier than the event start, PMS still creates the event at checkout time using a minimal 30-minute duration and records a schedule-conflict warning.

Future enhancement: allow per-property custom cleaning start offset, per-cleaner duration, or manual drag/drop rescheduling. Those are out of scope for this spec.

## 6. Event Title Rules

The event title must be deterministic and built from configurable property settings so updates are predictable.

Default titles:

- Same-day turnover cleaning: `Upratovanie: Pride Host`
- No-guest cleaning: `Upratovanie: Bez Hosta`

Configurable title parts:

- `cleaning_event_title_prefix`, default `Upratovanie:`.
- `cleaning_event_same_day_label`, default `Pride Host`.
- `cleaning_event_no_guest_label`, default `Bez Hosta`.

Title rendering rules:

- If same-day arrival is true, title = `{prefix} {same_day_label}`.
- If same-day arrival is false, title = `{prefix} {no_guest_label}`.
- PMS trims duplicate whitespace when rendering titles.

The same-day turnover title applies when at least one eligible arrival starts on the same property-local date as the checkout. The title must revert to the no-guest title if the same-day arrival is cancelled, deleted, closed, or moved to another date. If a new same-day arrival appears after PMS already created a no-guest cleaning event, PMS updates that existing event title to the same-day turnover title.

The title should not include guest names by default. ICS feeds may not reliably contain guest identity, and cleaner calendar entries should avoid unnecessary personal data.

## 7. Event Description Rules

The Google Calendar event description should be empty by default. If PMS later adds an optional description setting, it should include operational information only:

- Property name.
- Property address, if configured.
- Cleaning date.
- Check-out time.
- Same-day check-in time, only when same-day arrival is true.
- A clear note when this is a turnover cleaning.
- PMS occupancy ID or internal reference for troubleshooting.

Do not include Nuki codes, Google OAuth tokens, raw ICS payloads, payout details, or guest personal data.

## 8. Reconciliation Rules

PMS is the source of truth for PMS-managed cleaning calendar events.

- Store the Google Calendar event ID after successful creation.
- Use the occupancy ID as the local idempotency key.
- Optionally set a private Google extended property such as `pms_cleaning_event_id` or `pms_occupancy_id` to support recovery if local state is lost.
- If an occupancy date or check-out time changes, update the existing Google event instead of creating a new one.
- If a same-day arrival is newly imported, cancelled, deleted, closed, or moved, update only the existing cleaning event title to match the current same-day-arrival status. Do not update event start, event end, or description for this reason alone.
- If a checkout becomes ineligible, delete or cancel the Google event and mark the local sync row as removed.
- If a Google event was manually edited in Google Calendar, PMS may overwrite title, description, start, and end on the next reconciliation run.
- If a Google event was manually deleted in Google Calendar while the occupancy remains eligible, PMS recreates it and records that recovery in the sync log.

The cleaner is expected to have read-only access to the target Google Calendar. Manual edits by the cleaner are therefore not part of the supported workflow. Manual edits by an owner are not preserved in v1 of this feature unless they are fields PMS does not manage, such as Google Calendar colour or reminders.

## 9. Google Integration Requirements

Native Google Calendar sync requires a real Google API integration rather than the v1 n8n workaround.

- Support connecting a Google account through OAuth, or document and implement a service-account setup if the deployment will only target calendars owned by the same Google Workspace/domain.
- Store refresh tokens or service-account credentials as secrets, never in API responses or logs.
- Request the minimum Calendar API scope required to manage events in the selected calendar.
- Provide a calendar picker or validated text input for the target calendar ID.
- Refresh expired access tokens automatically.
- Treat Google API rate limits and transient 5xx responses as retryable failures.
- Treat missing calendar, permission denied, and invalid credentials as configuration errors visible in the UI.

## 10. Suggested Backend Model

Add property-level settings:

- `google_cleaning_sync_enabled` boolean.
- `google_cleaning_calendar_id` text.
- `google_cleaning_default_duration_minutes` integer, default `180`.
- `google_cleaning_title_prefix` text, default `Upratovanie:`.
- `google_cleaning_same_day_label` text, default `Pride Host`.
- `google_cleaning_no_guest_label` text, default `Bez Hosta`.
- `google_cleaning_connected_account_id` or equivalent secret reference.

Add a managed event table, for example `cleaning_calendar_events`:

- `id`.
- `property_id`.
- `occupancy_id` unique.
- `google_calendar_id`.
- `google_event_id` nullable until created.
- `cleaning_date`.
- `starts_at`.
- `ends_at`.
- `same_day_arrival` boolean.
- `next_occupancy_id` nullable.
- `title`.
- `status`: `pending`, `synced`, `error`, `removed`.
- `warning_message` nullable.
- `error_message` nullable.
- `last_synced_at` nullable.
- `created_at`.
- `updated_at`.

Add sync log rows if the existing audit/integration log pattern is not enough:

- `cleaning_calendar_sync_runs`.
- `cleaning_calendar_event_logs`.

## 11. Suggested API Endpoints

Settings:

- `GET /api/properties/{id}/cleaning-calendar/settings`
- `PATCH /api/properties/{id}/cleaning-calendar/settings`
- `GET /api/properties/{id}/cleaning-calendar/google/calendars`
- `POST /api/properties/{id}/cleaning-calendar/google/connect`
- `POST /api/properties/{id}/cleaning-calendar/google/disconnect`

Events and reconciliation:

- `GET /api/properties/{id}/cleaning-calendar/events?month=YYYY-MM`
- `POST /api/properties/{id}/cleaning-calendar/reconcile`
- `POST /api/properties/{id}/cleaning-calendar/events/{eventId}/retry`

All endpoints require property-scoped cleaning module write/admin permission for settings and retry actions. Listing events may use cleaning read permission.

## 12. Background Job

The feature needs a scheduled reconciliation job in addition to manual retry.

- Run after every successful occupancy sync for the property.
- Run periodically, for example hourly, to recover from transient Google API failures.
- Process upcoming and recently changed occupancies. A practical default window is from 30 days in the past to 365 days in the future.
- Keep the job safe to rerun.
- Use bounded concurrency and backoff so Google API failures do not block other PMS jobs.

## 13. Frontend Requirements

Add a cleaning calendar settings panel under the existing Cleaning or Property Settings area.

The UI should show:

- Whether Google Calendar cleaning sync is enabled.
- Connected Google account status.
- Selected target calendar.
- Default cleaning duration.
- Configurable title prefix, same-day-arrival label, and no-guest label.
- Last successful reconciliation time.
- Latest sync error, if any.
- A manual reconcile/retry button.

Add a calendar-event status table or section showing generated cleaning events for the selected month:

- Cleaning date.
- Event title.
- Start and end time.
- Same-day arrival indicator.
- Google sync status.
- Warning or error message.
- Link to the Google Calendar event when available.

## 14. Edge Cases

- Same-day checkout and check-in: title uses `{prefix} {same_day_label}`, default `Upratovanie: Pride Host`.
- Multiple arrivals on the same day: any eligible arrival makes `same_day_arrival = true`; link the earliest next occupancy as `next_occupancy_id`.
- Multiple checkouts on the same day: out of scope for the current product model because there can only be one checkout per property. If multi-unit properties are introduced later, revisit whether to create one event per unit or one merged event.
- Back-to-back bookings where the previous stay changes date: update the existing event title, timing, and next-arrival reference.
- Occupancy disappears from ICS: remove the event unless PMS marks the row as a historical active stay that still requires cleaning.
- Closed occupancy: no event, and any existing event is removed.
- Externally-sold occupancy checkout: event is created because a real guest used the property.
- Google permission revoked: mark affected events as `error` and surface a property-level configuration error.
- Calendar ID changed: future reconciliation creates events in the new calendar and removes PMS-managed future events from the old calendar when permissions allow it.

## 15. Test Focus

Backend tests:

- One active checkout creates exactly one local cleaning calendar event and one Google upsert request.
- Re-running reconciliation does not create duplicates.
- Same-day arrival changes the title to `Upratovanie: Pride Host` using the default settings.
- Cancelling or closing the same-day arrival reverts the title to `Upratovanie: Bez Hosta` using the default settings.
- Custom title prefix, same-day-arrival label, and no-guest label render into the expected Google Calendar event title.
- Cancelling, deleting, or closing the checkout occupancy removes the Google event.
- Externally-sold checkout still creates a cleaning event.
- Event start is copied from the property's configured check-out time.
- Same-day event end is one hour before the arriving guest's check-in time.
- A same-day arrival imported after the cleaning event already exists updates only the title and keeps the original start/end times.
- Property timezone boundaries compute the correct cleaning date around midnight and DST changes.
- Short same-day windows produce a warning instead of failing the sync.
- Google 429/5xx responses leave the event retryable.
- Invalid calendar ID or revoked credentials produce visible configuration errors.

Frontend tests:

- Settings save and reload correctly.
- Event table renders no-guest and same-day-arrival titles.
- Error and warning states are visible.
- Retry action calls the expected endpoint.
- Users without cleaning write/admin permission cannot change settings.

## 16. Acceptance Criteria

- Owner can connect/select a Google Calendar for a property and enable cleaning sync.
- After occupancy sync or any new reservation import, every eligible checkout has exactly one PMS-managed Google Calendar cleaning event.
- A checkout with a same-day arrival creates an event titled `Upratovanie: Pride Host` by default.
- A checkout without a same-day arrival creates an event titled `Upratovanie: Bez Hosta` by default.
- Title prefix, same-day-arrival label, and no-guest label are configurable in the UI.
- Moving, cancelling, or newly importing the next same-day arrival updates only the existing event title correctly.
- Moving, cancelling, or newly importing the next same-day arrival does not change the event time window.
- Moving or cancelling the checkout updates or removes the existing Google Calendar event without duplicates.
- Sync errors are visible in PMS and retryable.
- No guest personal data, Nuki codes, OAuth tokens, or raw ICS payloads are written into calendar titles/descriptions or logs.
