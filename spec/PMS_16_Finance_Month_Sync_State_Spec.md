# PMS_16 — Finance Month Generated-Entry Sync State Spec

> Audience: product owner + implementing engineer.
> Scope: make the Finance month action understandable by renaming it and exposing when generated finance entries were last synchronized for the selected month.
> Status: draft. Do not implement until the open questions in section 8 are answered.

---

## 1. Revised Product Framing

The issue is not only that `Open month` can be clicked repeatedly. The larger issue is that the label `Open month` does not explain what the action does.

Current behavior is closer to:

- initialize the month if it has never been initialized;
- generate missing recurring-rule transactions;
- update existing generated recurring-rule transactions;
- purge generated recurring-rule transactions whose rule no longer applies;
- create or update the generated cleaning salary transaction.

That is a **generated-entry synchronization** operation, not a pure open/close state transition.

Recommended product direction:

- Do **not** force the operation to become strictly binary yet.
- Rename the user-facing action away from `Open month`.
- Show a clear selected-month sync status: never synced, synced, sync in progress, or sync failed.
- Preserve repeatability because re-syncing generated entries can be legitimate.
- Make repeatability explicit so users understand why they might click it again.

---

## 2. Current Behavior

### 2.1 What the user observes

In `frontend/src/views/FinanceView.vue`, the Finance toolbar always renders an `Open month` primary button for the selected month. After clicking it, the UI shows a toast and reloads the Finance data, but the toolbar still looks exactly the same.

The user can click `Open month` repeatedly for the same month. There is no visible indication that generated entries were synchronized or when synchronization last happened.

### 2.2 What the backend currently does

The route already exists:

- `POST /api/properties/{id}/finance/months/{YYYY-MM}/open`

The store already has `finance_month_states`:

- `property_id`
- `month`
- `opened_at`
- `opened_by`
- unique `(property_id, month)`

`Store.OpenFinanceMonth` inserts the month-state row with `ON CONFLICT(property_id, month) DO NOTHING`, so the state insert itself is idempotent.

However, the method does more than mark the month open. It also:

- creates or updates auto-generated transactions from active recurring rules;
- purges orphaned recurring-rule transactions for that month;
- creates or updates the auto-generated cleaning salary transaction;
- is reused internally after recurring rule updates/deletes to re-sync all already-open months.

### 2.3 Current gap

The backend can tell whether a month has ever been initialized, but it does not expose a reliable selected-month **last sync** signal to the Finance UI.

The current `opened_at` value is not enough because repeated syncs do not update it. A month may have been opened in January and re-synced in March, but the UI has no way to show that March sync.

---

## 3. Challenge To The Original Binary Request

The original request proposed making the operation binary and visually showing whether a month is open or closed. That would solve one symptom but may make the underlying workflow worse.

Reasons to avoid a strict binary-only change now:

- Re-syncing generated entries is a valid operation when recurring rules or cleaning salary inputs change.
- The current backend already relies on the same operation to reconcile generated rows after recurring-rule changes.
- `Closed` is ambiguous in Finance. It may mean "not initialized" or "locked accounting period".
- Disabling the action after first click would hide the only visible reconciliation affordance unless a replacement is added.

Better framing:

- Use `Month setup` or `Generated entries` language for the state.
- Treat the action as repeatable synchronization, not binary opening.
- If true accounting close/lock is needed later, specify it separately as a different lifecycle.

---

## 4. Recommended UX

### 4.1 Rename the action

Recommended button label:

- `Sync generated entries`

Acceptable shorter alternatives:

- `Sync month`
- `Update generated entries`
- `Prepare month`

Avoid:

- `Open month`, because it sounds like a one-way state transition.
- `Close month`, unless the product introduces accounting-period locking.

Recommended tooltip/help text:

> Creates or updates this month's recurring expenses and cleaning salary entry. Manual transactions and imported payouts are not changed.

### 4.2 Show selected-month sync status

The Finance toolbar should show a visible status near the month picker.

Possible states:

| State | Badge | Helper copy |
|---|---|---|
| Never synced | `Not synced` | `Generated recurring and cleaning salary entries have not been synced for this month.` |
| Synced | `Synced` | `Generated entries last synced <date/time>.` |
| Syncing | `Syncing...` | `Updating generated recurring and cleaning salary entries.` |
| Failed | `Sync failed` | `The last generated-entry sync failed. Try again.` |

The MVP can skip persisted failure state if errors are already surfaced through toasts and inline banners. It still needs `Not synced` and `Synced`.

### 4.3 After a successful sync

After the user clicks `Sync generated entries`:

- show a success toast with counts where possible;
- reload Finance data for the selected month;
- update the toolbar status to `Synced`;
- display the latest sync timestamp.

Recommended toast copy:

> Generated entries synced for 2026-04: 3 recurring, 1 cleaning salary updated.

If counts are not yet detailed:

> Generated entries synced for 2026-04.

### 4.4 Repeat clicks

Repeat clicks should remain allowed for users with Finance write access, but the UI must explain that this is a re-sync.

For an already-synced month, the button should still be available and labelled `Sync generated entries`, not `Open month`.

---

## 5. Functional Requirements

### 5.1 Sync semantics

The sync operation affects only system-generated Finance rows for the selected month:

- `source_type = 'recurring_rule'`
- `source_type = 'cleaning_salary'`

The sync operation must not alter:

- manual transactions;
- imported Booking.com payout transactions;
- imported statement-derived data;
- attachments;
- transaction notes on user-created rows;
- category definitions;
- recurring-rule definitions.

### 5.2 Month initialization

The first sync for a month should continue creating a `finance_month_states` row so the system knows the month has entered the generated-entry workflow.

The product-facing term should not be `open` unless the UI explicitly explains it.

Recommended internal interpretation:

- `opened_at`: first generated-entry sync timestamp, kept for backward compatibility.
- `last_synced_at`: latest generated-entry sync timestamp.
- `last_synced_by`: latest user who manually triggered sync, if applicable.

### 5.3 Recurring-rule mutations

When recurring rules are created, updated, deactivated, or deleted, the existing behavior should remain: all initialized months are re-synced so generated transactions stay consistent.

Automatic/internal re-syncs after recurring-rule mutations must update the visible `last_synced_at`. The timestamp answers the user question "when were generated entries last brought up to date?", regardless of whether the sync was manually triggered from the toolbar or automatically triggered by a rule change.

Required behavior:

- update `last_synced_at` for any successful sync, manual or automatic;
- set `last_synced_reason` to distinguish `manual`, `recurring_rule_create`, `recurring_rule_update`, and `recurring_rule_delete` if cheap enough;
- set `last_synced_by` to the acting user for user-triggered automatic syncs caused by recurring-rule mutations;
- leave `last_synced_by` empty only for system/background syncs without a user actor.

### 5.4 Cleaning salary changes

If cleaning inputs change after a month was synced, users need a clear way to refresh the generated cleaning salary transaction.

Recommended MVP:

- users can click `Sync generated entries` again;
- sync updates the cleaning salary generated transaction for the selected month;
- the toolbar timestamp confirms the refresh happened.

---

## 6. API And Data Shape

### 6.1 Preferred API naming

The existing endpoint can remain initially for compatibility:

- `POST /api/properties/{id}/finance/months/{YYYY-MM}/open`

But the frontend should call a clearer endpoint once implemented:

- `POST /api/properties/{id}/finance/months/{YYYY-MM}/sync-generated`

If keeping only the old endpoint for now, the UI can still rename the button. The implementation should treat the old route as a legacy alias.

### 6.2 Month sync state response

Extend `GET /api/properties/{id}/finance/summary?month=YYYY-MM` with sync metadata because `FinanceView.vue` already loads summary for the selected month.

Recommended shape:

```json
{
  "month": "2026-04",
  "generated_entry_sync": {
    "status": "synced",
    "first_synced_at": "2026-04-02T10:20:30Z",
    "first_synced_by": 123,
    "last_synced_at": "2026-04-07T09:15:00Z",
    "last_synced_by": 123,
    "last_synced_reason": "manual"
  }
}
```

For a month never synced:

```json
{
  "month": "2026-04",
  "generated_entry_sync": {
    "status": "not_synced"
  }
}
```

Allowed `status` values:

- `not_synced`
- `synced`

Optional future values:

- `syncing`
- `failed`
- `stale`

### 6.3 Sync response

The sync endpoint should return:

```json
{
  "ok": true,
  "generated_entry_sync": {
    "status": "synced",
    "first_synced_at": "2026-04-02T10:20:30Z",
    "last_synced_at": "2026-04-07T09:15:00Z",
    "last_synced_reason": "manual"
  },
  "changes": {
    "recurring_inserted": 1,
    "recurring_updated": 2,
    "recurring_deleted": 0,
    "cleaning_salary_inserted": 0,
    "cleaning_salary_updated": 1
  }
}
```

If detailed counts are too invasive for MVP, return the existing `generated_recurring_count` plus sync metadata. The UI can use generic success copy.

---

## 7. Data Model Notes

The current table is:

```sql
finance_month_states (
  id,
  property_id,
  month,
  opened_at,
  opened_by
)
```

Recommended migration:

```sql
ALTER TABLE finance_month_states ADD COLUMN last_synced_at TEXT;
ALTER TABLE finance_month_states ADD COLUMN last_synced_by INTEGER REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE finance_month_states ADD COLUMN last_synced_reason TEXT;
```

Backfill:

- `last_synced_at = opened_at`
- `last_synced_by = opened_by`
- `last_synced_reason = 'initial_open_legacy'`

Do not add `closed_at`, `locked_at`, or `status` in this feature unless accounting-period lifecycle is explicitly introduced.

---

## 8. Open Questions

1. Should the primary button label be `Sync generated entries`, `Sync month`, or another phrase?
2. Do you want detailed sync counts in the success toast, or is a generic success message enough for MVP?
3. Should the UI warn when generated entries may be stale, for example after cleaning data changes?
4. Is it acceptable that the first sync still creates `finance_month_states.opened_at` internally, even if the user-facing language avoids `open`?
5. Should read-only Finance users see sync status but no sync button?
6. Should Dashboard show a warning when the current Finance month is `Not synced`?

---

## 9. Acceptance Criteria

- The Finance toolbar no longer exposes `Open month` as the primary user-facing label.
- The selected month visibly shows whether generated entries have never been synced or were synced previously.
- The selected month shows `last_synced_at` when available.
- Users with Finance write access can manually re-sync generated entries for the selected month.
- Manual re-sync updates generated recurring-rule and cleaning salary rows only.
- Manual re-sync does not modify manual transactions, imported payout rows, imported statement rows, or attachments.
- Recurring-rule mutations continue to re-sync initialized months.
- The API exposes generated-entry sync metadata to the frontend.
- Backend tests cover first sync, repeated sync, sync metadata updates, and protection of non-generated rows.
- Frontend tests cover `Not synced`, `Synced`, loading, success, and error states.

---

## 10. Recommended Implementation Order

1. Add sync metadata columns to `finance_month_states` and backfill from `opened_at`.
2. Split generated-entry sync logic into an explicitly named store method.
3. Add or alias a clearer API endpoint such as `/sync-generated`.
4. Extend Finance summary with `generated_entry_sync` metadata.
5. Update frontend API types.
6. Rename the toolbar action to `Sync generated entries`.
7. Add the selected-month sync status badge and timestamp.
8. Add backend and frontend tests for the visible sync workflow.
