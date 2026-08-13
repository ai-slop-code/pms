# PMS_15 - Google Calendar Cleaning Events Spec

> Audience: product / property manager + implementing engineer.
> Scope: native PMS integration that reconciles checkout-driven cleaning events
> in a configured Google Calendar.
> Status: active behavioral specification, aligned to the PMS 21 final model.
> It supersedes the earlier v1-only n8n recommendation. Named stays and raw
> booking blocks own cleaning candidates; legacy occupancy IDs are not part of
> the final contract.

## 1. Problem Framing

The cleaning lady needs a reliable calendar view of upcoming turnover work.
PMS derives desired work from named stays and provisional raw blocks, but the
cleaner should not depend on manual communication. PMS creates Google Calendar
events automatically so each cleaning task is visible without duplicate data
entry.

The core business rule is checkout-driven. Uncovered raw-block nights create
provisional checkout placeholders; active named stays with cleaning enabled
create final checkout events. If another named stay checks in on the same
property-local date, the final event title and end time make that turnover
visible. Later changes reconcile the existing event rather than creating a
duplicate.

## 2. Definitions

- **Cleaning calendar**: the Google Calendar selected by the property owner for PMS-generated cleaning events.
- **Cleaning event**: a Google Calendar event created and managed by PMS for one checkout.
- **Named-stay cleaning**: a cleaning candidate owned by one eligible named
  stay and scheduled for that stay's checkout.
- **Provisional raw-block cleaning**: one property/date candidate coalesced from
  active raw Booking.com nights not covered by a named stay. It remains an
  operational placeholder until promotion establishes final stay truth.
- **Same-day arrival**: an eligible named stay for the same property whose
  check-in date equals the cleaning candidate's checkout date in property time.
- **Turnover cleaning**: a cleaning where same-day arrival is true and the apartment must be ready before the next guest's check-in time.
- **No-guest cleaning**: a cleaning where no eligible named stay checks in on
  the same property-local date.

## 3. Functional Requirements

- Each property can enable or disable Google Calendar cleaning sync.
- Each enabled property stores exactly one target Google Calendar ID for cleaning events.
- Each enabled property stores configurable event-title parts: title prefix, same-day-arrival label, and no-guest label.
- Default title prefix is `Upratovanie:`.
- Default same-day-arrival label is `Pride Host`.
- Default no-guest label is `Bez Hosta`.
- PMS automatically reconciles one Google Calendar cleaning event for each
  eligible desired cleaning identity.
- PMS updates the existing Google event if its owning named stay/raw block,
  property name, or configured check-out time changes.
- PMS updates the existing Google event title and end time when
  same-day-arrival status changes.
- PMS cancels or deletes the Google event when its new-model owner stops being
  eligible for cleaning.
- PMS must be idempotent: re-running source sync or cleaning reconciliation
  must not create duplicate Google Calendar events.
- PMS must show sync status and the last error for every managed cleaning calendar event.
- PMS must provide a manual retry action for failed Google Calendar syncs.

## 4. Eligibility Rules

A desired cleaning candidate creates an event when all of the following are true:

- The property has Google Calendar cleaning sync enabled.
- The property has a configured cleaning calendar ID.
- It is owned by exactly one same-property `named_stay_id` or
  `raw_booking_block_id`.
- A named-stay owner is active, cleaning-required, and eligible under its stay
  type/review/outcome state; a raw-block owner is an eligible provisional
  cleaning candidate.
- The checkout date is known in the property timezone.

Eligible external named stays represent real guest stays and create cleaning
events. Maintenance, personal use, availability blocks, cancelled/archived
stays, and explicitly cleaning-excluded stays do not.

Raw-block disappearance removes future provisional events. It does not cancel,
resize, rename, or otherwise mutate a named stay; named-stay lifecycle and
cleaning-required state independently determine its event.

## 5. Event Timing Rules

All date and time calculations use the property's configured timezone.

- Cleaning date = the desired cleaning identity's checkout date in property timezone.
- Event start = cleaning date at the property's configured check-out time, for example `09:00`.
- For a final named-stay event with a same-day arrival, event end = one hour
  before the arriving named stay's configured check-in time.
- If there is no same-day arrival, event end = start + the property's configured default cleaning duration.
- Provisional raw-block events always use the configured default duration.
- Default cleaning duration is property-configurable, with a recommended default of 3 hours.
- If same-day-arrival status changes later because a named stay is created,
  moved, cancelled, archived, or reactivated, PMS reconciles the existing final
  event's title and end time.
- If the calculated same-day end time is equal to or earlier than the event start, PMS still creates the event at checkout time using a minimal 30-minute duration and records a schedule-conflict warning.

Future enhancement: allow per-property custom cleaning start offset, per-cleaner duration, or manual drag/drop rescheduling. Those are out of scope for this spec.

## 6. Event Title Rules

The event title must be deterministic and built from configurable property settings so updates are predictable.

Default titles:

- Provisional raw-block cleaning: `Upratovanie`
- Same-day turnover cleaning: `Upratovanie: Pride Host`
- No-guest cleaning: `Upratovanie: Bez Hosta`

Configurable title parts:

- `cleaning_event_title_prefix`, default `Upratovanie:`.
- `cleaning_event_same_day_label`, default `Pride Host`.
- `cleaning_event_no_guest_label`, default `Bez Hosta`.

Title rendering rules:

- A provisional raw-block event uses exactly `Upratovanie`.
- If same-day arrival is true, title = `{prefix} {same_day_label}`.
- If same-day arrival is false, title = `{prefix} {no_guest_label}`.
- PMS trims duplicate whitespace when rendering titles.

The same-day turnover title applies when at least one eligible named stay starts
on the same property-local date as the checkout. The title and end time revert
to no-guest/default-duration state if that arrival is cancelled, archived, or
moved. If a new same-day arrival appears later, PMS updates that same managed
event to the turnover title and shortened end time.

The title should not include guest names by default. ICS feeds may not reliably contain guest identity, and cleaner calendar entries should avoid unnecessary personal data.

## 7. Event Description Rules

The Google Calendar event description should be empty by default. If PMS later adds an optional description setting, it should include operational information only:

- Property name.
- Property address, if configured.
- Cleaning date.
- Check-out time.
- Same-day check-in time, only when same-day arrival is true.
- A clear note when this is a turnover cleaning.
- Deterministic cleaning identity and new-model owner reference for troubleshooting.

Do not include Nuki codes, Google OAuth tokens, raw ICS payloads, payout details, or guest personal data.

## 8. Reconciliation Rules

PMS is the source of truth for PMS-managed cleaning calendar events.

- Store the Google Calendar event ID after successful creation.
- Use the deterministic `cleaning_identity` as the local idempotency key.
- Private Google metadata may contain the cleaning identity and new-model owner
  identity, but must not write `pms_occupancy_id`.
- If an owning named-stay/raw-block checkout or configured check-out time
  changes, update the existing Google event instead of creating a new one.
- If a same-day named stay is created, cancelled, archived, made ineligible, or
  moved, update the existing cleaning event title and end time to match current
  same-day-arrival status. Do not change its start or description for this
  reason alone.
- If a candidate becomes ineligible, delete or cancel the Google event and mark the local sync row as removed.
- If a Google event was manually edited in Google Calendar, PMS may overwrite title, description, start, and end on the next reconciliation run.
- If a managed Google event was manually deleted while its desired cleaning
  identity remains eligible, PMS recreates it and records the recovery.

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

The managed `cleaning_calendar_events` contract includes:

- `id`.
- `property_id`.
- `named_stay_id` nullable.
- `raw_booking_block_id` nullable.
- a constraint requiring exactly one same-property owner.
- `cleaning_identity` unique within the property.
- `google_calendar_id`.
- `google_event_id` nullable until created.
- `cleaning_date`.
- `starts_at`.
- `ends_at`.
- `same_day_arrival` boolean.
- Same-day-arrival state derived from named-stay nights; no legacy
  `next_occupancy_id` is retained.
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

- Run after every successful raw-block sync and every relevant named-stay change.
- Run periodically, for example hourly, to recover from transient Google API failures.
- Process upcoming and recently changed named-stay/raw-block candidates. A
  practical default window is from 30 days in the past to 365 days in the future.
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
- Multiple arrivals on the same day: any eligible named-stay arrival makes
  `same_day_arrival = true`; derive the earliest next stay without persisting a
  legacy next-occupancy identity.
- Multiple checkouts on the same day: out of scope for the current product model because there can only be one checkout per property. If multi-unit properties are introduced later, revisit whether to create one event per unit or one merged event.
- Back-to-back named stays where the previous stay changes date: update the
  existing event title, timing, and derived next-arrival state.
- Raw block disappears from ICS: remove its future provisional event; do not
  mutate independently owned named stays.
- Maintenance/personal-use or unavailable-only state: no final stay cleaning.
- Eligible external named-stay checkout: event is created because a real guest used the property.
- Google permission revoked: mark affected events as `error` and surface a property-level configuration error.
- Calendar ID changed: future reconciliation creates events in the new calendar and removes PMS-managed future events from the old calendar when permissions allow it.

## 15. Test Focus

Backend tests:

- One eligible desired cleaning identity creates exactly one local event and one Google upsert request.
- Re-running reconciliation does not create duplicates.
- Same-day arrival changes the title to `Upratovanie: Pride Host` using the default settings.
- Cancelling or closing the same-day arrival reverts the title to `Upratovanie: Bez Hosta` using the default settings.
- Custom title prefix, same-day-arrival label, and no-guest label render into the expected Google Calendar event title.
- Cancelling/archiving a named stay, disabling its cleaning, or removing an
  eligible provisional raw block removes the corresponding Google event.
- Externally-sold checkout still creates a cleaning event.
- Event start is copied from the property's configured check-out time.
- Same-day event end is one hour before the arriving guest's check-in time.
- A same-day named stay added after the cleaning event already exists updates
  the same event's title and end time without changing its identity.
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
- After raw-block sync or any named-stay lifecycle change, every eligible
  cleaning identity has exactly one PMS-managed Google Calendar event.
- A checkout with a same-day arrival creates an event titled `Upratovanie: Pride Host` by default.
- A checkout without a same-day arrival creates an event titled `Upratovanie: Bez Hosta` by default.
- Title prefix, same-day-arrival label, and no-guest label are configurable in the UI.
- Moving, cancelling, or adding the next same-day named stay updates the
  existing event's title and end time correctly.
- Moving or cancelling the checkout updates or removes the existing Google Calendar event without duplicates.
- Sync errors are visible in PMS and retryable.
- No guest personal data, Nuki codes, OAuth tokens, or raw ICS payloads are written into calendar titles/descriptions or logs.
