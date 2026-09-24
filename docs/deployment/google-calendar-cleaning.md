# Google Calendar Cleaning Sync Guide

This guide explains how to make PMS create Google Calendar events for cleaning after each guest checkout.

It is written for a normal operator, not a developer.

## What This Feature Does

PMS imports Booking.com ICS entries as raw unavailable blocks first; an ICS
block is not assumed to be a guest reservation. Raw blocks never create cleaning
events. After an operator creates or promotes a named stay, that persisted stay
becomes the source for its guest-checkout cleaning event.

Default event titles:

| Situation | Google Calendar event title |
| --- | --- |
| A guest checks out and another guest checks in the same day | `Upratovanie: Pride Host` |
| A guest checks out and nobody checks in the same day | `Upratovanie: Bez Hosta` |

The title parts are configurable in the PMS Cleaning page:

| Setting | Default value |
| --- | --- |
| Title prefix | `Upratovanie:` |
| Same-day guest label | `Pride Host` |
| No-guest label | `Bez Hosta` |

Example: if checkout is at `09:00`, PMS creates the cleaning event at `09:00` on the checkout date.

If another guest checks in the same day at `14:00`, PMS creates the event from `09:00` until `13:00`, because the rule is to end one hour before check-in.

## Important Concepts

### PMS Is The Source Of Truth

Do not manually edit PMS-created cleaning event titles in Google Calendar. PMS may overwrite them on the next sync.

The cleaner should have read-only access to the cleaning calendar.

### Raw Blocks And Named Stays

Booking.com ICS is an availability feed. PMS stores each event as a raw booking
 block and stores its covered nights separately. Raw nights are availability
 evidence only and never produce cleaning work.

Named stays are separate, operator-owned business records. An active named stay
 with cleaning enabled produces one event on its checkout date. ICS sync can
 update or remove raw evidence, but it does not cancel, resize, rename, or
 otherwise change a named stay.

The final model does not expose occupancy IDs or legacy occupancy repair APIs.
Use the Availability calendar's raw-block, named-stay, and availability-block
workflows instead.

### Google Calendar ID

A Google Calendar ID tells PMS which calendar to write into.

For your main Google calendar, the ID is usually your email address, for example:

```text
my.email@gmail.com
```

For a separate calendar, the ID usually looks like this:

```text
abcd1234efgh5678@group.calendar.google.com
```

You can find it in Google Calendar settings. Steps are below.

### Service Account

PMS does not log in as you with a browser. Instead, PMS uses a Google service account.

Think of a service account as a robot Google user. You create it in Google Cloud, download a JSON key, and share your cleaning calendar with that robot user.

The service account email looks like this:

```text
pms-calendar-sync@your-project.iam.gserviceaccount.com
```

## Setup Overview

There are two parts:

1. Google setup: create credentials and share a calendar with PMS.
2. PMS setup: add the credentials to the server and enable the feature in the Cleaning page.

Do the Google setup first.

## Part 1: Google Setup

### Step 1: Create Or Pick A Google Calendar

Recommended: create a dedicated calendar only for cleaning.

1. Open Google Calendar.
2. In the left sidebar, find `Other calendars`.
3. Click the `+` button.
4. Choose `Create new calendar`.
5. Name it something clear, for example `PMS Cleaning`.
6. Click `Create calendar`.

You can also use an existing calendar, but a dedicated cleaning calendar is safer and easier to share with the cleaner.

### Step 2: Find The Calendar ID

1. Open Google Calendar.
2. In the left sidebar, hover over the cleaning calendar.
3. Click the three-dot menu.
4. Click `Settings and sharing`.
5. Scroll to `Integrate calendar`.
6. Copy `Calendar ID`.

Keep this value. You will paste it into PMS later.

Example calendar ID:

```text
abcd1234efgh5678@group.calendar.google.com
```

### Step 3: Create A Google Cloud Project

1. Open https://console.cloud.google.com/.
2. Sign in with the Google account that owns or manages the calendar.
3. Open the project selector at the top of the page.
4. Click `New Project`.
5. Name it something like `PMS Calendar Sync`.
6. Click `Create`.

### Step 4: Enable The Google Calendar API

1. In Google Cloud Console, make sure the `PMS Calendar Sync` project is selected.
2. Go to `APIs & Services`.
3. Go to `Library`.
4. Search for `Google Calendar API`.
5. Open it.
6. Click `Enable`.

### Step 5: Create A Service Account

1. In Google Cloud Console, go to `IAM & Admin`.
2. Open `Service Accounts`.
3. Click `Create service account`.
4. Enter a name, for example `pms-calendar-sync`.
5. Click `Create and continue`.
6. You do not need to grant project roles for this PMS use case.
7. Click `Done`.

After creation, copy the service account email. It looks like this:

```text
pms-calendar-sync@your-project.iam.gserviceaccount.com
```

### Step 6: Create And Download A JSON Key

1. In `Service Accounts`, click the service account you just created.
2. Open the `Keys` tab.
3. Click `Add key`.
4. Choose `Create new key`.
5. Choose `JSON`.
6. Click `Create`.
7. Google downloads a `.json` file.

Treat this JSON file like a password. Anyone with this file can act as the service account.

Do not commit it to git. Do not send it in chat. Do not paste it into logs.

### Step 7: Share The Cleaning Calendar With The Service Account

This is the most commonly missed step.

1. Open Google Calendar.
2. Open `Settings and sharing` for the cleaning calendar.
3. Scroll to `Share with specific people or groups`.
4. Click `Add people and groups`.
5. Paste the service account email.
6. Set permission to `Make changes to events`.
7. Click `Send` or `Share`.

The service account must be able to create and update events. Read-only access is not enough for PMS.

The cleaner can have read-only access. The service account needs write access.

## Part 2: PMS Server Setup

PMS needs the service account JSON key on the backend server.

There are two supported ways to provide it:

| Method | Recommended when |
| --- | --- |
| `PMS_GOOGLE_SERVICE_ACCOUNT_FILE` | You can place the JSON file on the server. Recommended. |
| `PMS_GOOGLE_SERVICE_ACCOUNT_JSON` | You prefer storing the whole JSON in an environment variable. |

Use only one of them.

### Option A: Use A JSON File

This is usually easiest and safest.

1. Put the downloaded JSON file on the server.
2. Make sure only the PMS backend user/container can read it.
3. Set this environment variable:

```dotenv
PMS_GOOGLE_SERVICE_ACCOUNT_FILE=/path/to/google-service-account.json
```

For Docker Compose, this means:

1. Put the JSON file somewhere near your deployment files, for example `deploy/secrets/google-service-account.json`.
2. Mount that file into the backend container.
3. Set `PMS_GOOGLE_SERVICE_ACCOUNT_FILE` to the path inside the container.

Example Compose idea:

```yaml
services:
  pms-backend:
    volumes:
      - ./secrets/google-service-account.json:/run/secrets/google-service-account.json:ro
    environment:
      PMS_GOOGLE_SERVICE_ACCOUNT_FILE: /run/secrets/google-service-account.json
```

If your Compose file uses `env_file`, put this in `deploy/.env`:

```dotenv
PMS_GOOGLE_SERVICE_ACCOUNT_FILE=/run/secrets/google-service-account.json
```

You still need the `volumes:` entry so the file exists inside the container.

### Option B: Use The JSON Directly In An Env Var

Use this only if your deployment system handles multiline secrets cleanly.

```dotenv
PMS_GOOGLE_SERVICE_ACCOUNT_JSON={"type":"service_account", ...}
```

This is easy to break because JSON contains quotes and newlines. The file method is less error-prone.

### Restart PMS

After setting the environment variable, restart the backend.

Docker Compose:

```bash
cd deploy
docker compose up -d --build
```

Systemd:

```bash
sudo systemctl restart pms-server
```

After restart, PMS should report that the Google client is configured in the Cleaning page.

## Part 3: PMS UI Setup

### Step 1: Open The Cleaning Page

1. Log in to PMS.
2. Select the correct property.
3. Open `Cleaning`.
4. Find the section `Google cleaning calendar`.

### Step 2: Enable Calendar Sync

Fill in:

| Field | What to enter |
| --- | --- |
| Calendar sync | `Enabled` |
| Google Calendar ID | The calendar ID copied from Google Calendar settings |
| Default duration | Usually `180` minutes |
| Title prefix | `Upratovanie:` |
| Same-day guest label | `Pride Host` |
| No-guest label | `Bez Hosta` |

Click `Save calendar settings`.

### Step 3: Run Manual Reconciliation

Click `Reconcile now` or `Sync cleaning calendar`.

PMS will scan upcoming and recently changed raw booking blocks and named stays,
then create, update, or remove the corresponding cleaning events.

### Step 4: Check The Event Table

The Cleaning page shows PMS-managed cleaning calendar events for the selected month.

Columns:

| Column | Meaning |
| --- | --- |
| Date | Cleaning date, based on checkout date |
| Title | Google Calendar event title |
| Time | Event start/end time |
| Same-day guest | Whether another guest checks in on that date |
| Status | Sync status |
| Message | Error, warning, or last sync timestamp |

The table can contain provisional raw-block rows and final named-stay rows.
Provisional rows use the plain `Upratovanie` title; same-day guest title logic
applies only to final named-stay checkout events.

Statuses:

| Status | Meaning |
| --- | --- |
| `pending` | PMS has a local event row that still needs to sync |
| `synced` | Event was created or updated in Google Calendar |
| `error` | PMS tried to sync but failed |
| `removed` | PMS removed or cancelled the event because it is no longer eligible |

If a row is `error`, use the `Retry` button after fixing the cause.

## How PMS Decides Event Title

An uncovered raw block night creates a provisional event titled exactly
`Upratovanie`. Configurable title labels do not apply until a named stay owns
the checkout.

For a named stay, PMS checks the checkout date in the property's timezone.

If another active named stay starts on that same date, the title uses the same-day label:

```text
Upratovanie: Pride Host
```

If no active named stay starts on that same date, the title uses the no-guest label:

```text
Upratovanie: Bez Hosta
```

If a new named stay appears later and changes the same-day status, PMS updates
the title and reconciles the event end to one hour before that stay's check-in.

Example:

1. PMS has a named stay that checks out on Friday.
2. No Friday arrival exists yet.
3. PMS creates `Upratovanie: Bez Hosta`.
4. Later an active named stay is added with a Friday check-in.
5. PMS updates the existing event title to `Upratovanie: Pride Host`.
6. PMS does not create a duplicate event.

## How PMS Decides Event Time

Event start comes from the property's configured checkout time.

For provisional raw-block events, PMS uses the configured default duration.
For final named-stay events, a same-day named-stay arrival can shorten the event
to end one hour before check-in.

Example:

```text
Property checkout time: 09:00
Cleaning event starts: 09:00
```

If there is a same-day named-stay check-in when the final event is first created, PMS ends the event one hour before check-in.

Example:

```text
Checkout time: 09:00
Same-day check-in time: 14:00
Cleaning event: 09:00 - 13:00
```

If there is no same-day named-stay check-in when the event is first created, PMS uses the default duration.

Example:

```text
Checkout time: 09:00
Default duration: 180 minutes
Cleaning event: 09:00 - 12:00
```

If a same-day named stay appears later, reconciliation updates both the title
and the event end. If that arrival is removed, reconciliation restores the
configured default-duration end.

## What Creates A Cleaning Event

PMS creates a provisional raw-block event when Google cleaning sync and the
calendar ID are configured, the raw block/night is active, and no active named
stay covers that night. Overlapping raw blocks are coalesced into one
provisional event for a property checkout date.

PMS creates a final named-stay event when all of these are true:

| Rule | Required value |
| --- | --- |
| Google cleaning sync | Enabled |
| Google Calendar ID | Filled in |
| Named-stay status | Active |
| Cleaning required | Enabled on the named stay |

PMS does not create final events for cancelled or archived named stays, named
stays with cleaning disabled, or property availability blocks. If raw ICS
evidence disappears, its provisional event is removed; this does not cancel a
separate named stay.

Externally-sold named stays create cleaning events when they are active and
cleaning is enabled because they represent real guest stays.

## Day-To-Day Usage

Normal workflow:

1. PMS imports raw unavailable blocks from the ICS feed.
2. PMS creates provisional placeholders for uncovered raw nights.
3. The operator creates or promotes the real guest ranges as named stays.
4. PMS replaces affected provisional placeholders with final named-stay checkout events.
5. The cleaner opens the shared Google Calendar and sees the cleaning schedule.
6. If a raw block or named stay changes, PMS reconciles the managed event on the next sync.

You usually only need to open the PMS Cleaning page if you want to check sync status or fix an error.

## PMS-22 Transition

PMS-22 removes raw-owned cleaning events. The application must be stopped before
the one-time cleanup and migration. Build the backend, frontend, and
`cleaning-calendar-cleanup` command as one release.

Run the command against the configured database and current configured calendar:

```text
/app/cleaning-calendar-cleanup prepare
/app/cleaning-calendar-cleanup status
/app/cleaning-calendar-cleanup run
/app/cleaning-calendar-cleanup status
/app/pms-migrate cleaning-calendar
```

Repeat `run` after correcting access or configuration errors. It uses the
original property-local cutoff and only the current calendar. Do not start an
old backend after remote cleanup. Once `run` reports all properties complete,
apply the forward migration with `/app/pms-migrate cleaning-calendar` and start only the new release. The migration
refuses incomplete cleanup and removes provisional local rows and logs without
archiving them.

Both commands are packaged in the main backend image alongside `/app/pms-server`.
The migration command applies only migration 40 and requires the PMS-21
transition (migration 39) already installed. For the standalone Podman deployment,
use the [production repair runbook](pms-30-podman-calendar-repair.md).

## Troubleshooting

### The Cleaning Page Says Google Client Is Not Configured

Meaning:

PMS backend does not have service account credentials loaded.

Fix:

1. Set `PMS_GOOGLE_SERVICE_ACCOUNT_FILE` or `PMS_GOOGLE_SERVICE_ACCOUNT_JSON`.
2. Restart the PMS backend.
3. Reload the Cleaning page.

### Event Status Is `error`: Google Calendar API 403

Most likely cause:

The service account does not have permission to write to the calendar.

Fix:

1. Copy the service account email from Google Cloud.
2. Open Google Calendar settings for the cleaning calendar.
3. Share the calendar with that email.
4. Give it `Make changes to events` permission.
5. Click `Retry` in PMS.

### Event Status Is `error`: Google Calendar API 404

Most likely causes:

| Cause | Fix |
| --- | --- |
| Calendar ID is wrong | Copy the Calendar ID again from Google Calendar settings |
| Calendar is not shared with service account | Share it with `Make changes to events` |
| Event was manually deleted | Click `Retry`; PMS may recreate it if the raw block or named stay is still eligible |

### No Events Are Created

Check these in order:

1. Is Google cleaning sync enabled in PMS?
2. Is the Calendar ID filled in?
3. Is the Google client configured on the server?
4. Did ICS sync import active raw booking blocks, or is there an active named stay?
5. For a final event, is cleaning enabled on the named stay?
6. Is the checkout date inside the reconciliation window?
7. Does the event table show errors?

### Duplicate Events Appear In Google Calendar

PMS is designed to avoid duplicates by storing the Google event ID.

Possible causes:

| Cause | Explanation |
| --- | --- |
| Events were created manually before PMS sync | PMS cannot know they are the same task |
| PMS database was restored from an old backup | PMS may not remember newer Google event IDs |
| Calendar events were copied manually | Google created separate events |

Fix:

1. Keep only PMS-managed events in the cleaning calendar.
2. Delete manually-created duplicates in Google Calendar.
3. Run `Reconcile now` in PMS.

### The Title Is Wrong

If the title says `Bez Hosta` but there is a same-day guest:

1. Confirm the same-day arrival exists as an active named stay in PMS Availability.
2. Confirm the named stay is active, not cancelled or archived.
3. Reconcile the relevant named-stay data; ICS sync alone does not create or edit named stays.
4. Run cleaning calendar reconcile.

If the same-day named stay was added after the cleaning event already existed,
run reconciliation so PMS updates its title and end time.

### The Time Is Wrong

Check property settings:

1. Open Property Settings.
2. Check default checkout time.
3. Check default check-in time.

Event start follows checkout time.

Same-day event end is one hour before the active named stay's check-in.

If a same-day named stay appears or disappears later, reconciliation updates the
managed event's title and time window.

## Security Notes

Treat the service account JSON key as a secret.

Do:

- Store it outside git.
- Restrict file permissions.
- Share only the cleaning calendar with the service account.
- Give the cleaner read-only calendar access.

Do not:

- Commit the JSON key.
- Paste the JSON key into tickets or chat.
- Give the service account access to unrelated calendars.
- Give the cleaner edit access unless you intentionally want manual changes.

## Quick Checklist

Use this checklist when setting up a new property.

1. Create a dedicated Google Calendar, for example `PMS Cleaning`.
2. Copy its Calendar ID.
3. Create a Google Cloud project.
4. Enable Google Calendar API.
5. Create a service account.
6. Download the JSON key.
7. Put the JSON key on the PMS backend server.
8. Set `PMS_GOOGLE_SERVICE_ACCOUNT_FILE` or `PMS_GOOGLE_SERVICE_ACCOUNT_JSON`.
9. Restart PMS backend.
10. Share the cleaning calendar with the service account using `Make changes to events`.
11. Open PMS Cleaning page.
12. Enable Google cleaning calendar sync.
13. Paste the Calendar ID.
14. Confirm title settings.
15. Save settings.
16. Click `Reconcile now`.
17. Check Google Calendar for provisional `Upratovanie` and final `Upratovanie: ...` events.
