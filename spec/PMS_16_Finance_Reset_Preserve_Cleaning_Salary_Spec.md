# PMS_16 - Finance Reset While Preserving Cleaning Salary Spec

> Audience: product / property manager + implementing engineer.
> Scope: destructive finance-data reset for one property, while preserving the cleaning lady salary generated from flat-entry cleaning logs.
> Status: implementation-ready specification with confirmed business decisions.

## 1. Business Analyst Challenge

The request says "delete all finance records", but that wording is too broad to implement safely without product decisions. In this PMS, finance records are not just manually entered transactions. They include imported Booking.com payout/statement rows, generated recurring transactions, generated cleaning salary rows, import audit rows, month sync state, categories, rules, attachments, and links used by invoices and analytics.

The cleaning lady salary is also partly finance data and partly cleaning data:

- Source of truth: `cleaning_daily_logs`, `cleaning_salary_adjustments`, `cleaning_monthly_summaries`, and `cleaner_fee_history`.
- Finance projection: `finance_transactions` rows with `source_type = 'cleaning_salary'` and `source_reference_id = YYYY-MM`.

The safest interpretation is not "wipe every table whose name starts with finance". The feature should be a property-scoped finance reset that deletes user/import/generated finance data but explicitly preserves cleaning salary source data and the generated finance salary rows derived from it.

## 2. Confirmed Business Decisions

The product owner confirmed the following decisions after BA review:

- Reset applies only to the selected property.
- Reset covers all time for that selected property.
- Imported Booking.com payout and statement rows are deleted.
- Invoices linked to finance booking rows are deleted as part of the reset.
- Recurring finance rules are deleted.
- Finance categories are kept.
- Uploaded finance attachment files are physically deleted after database commit.
- Cleaning salary finance rows are recomputed from cleaning source data.
- Finance-created synthetic occupancies are kept.
- A dedicated `finance_reset_runs` table records reset counts for audit/reporting.
- Only owner/admin users can execute the reset.
- The UI uses a confirmation dialog; no typed phrase is required.

Business analyst challenge: deleting invoices is materially riskier than blocking the reset because invoices can be legal/accounting records. The implementation must therefore make invoice deletion explicit in the preview and confirmation copy, must audit invoice counts, must delete related invoice files, and must not rewind invoice sequences or reuse invoice numbers.

## 3. Product Decisions For MVP

- The reset is property-scoped.
- The reset deletes finance activity/import data for the selected property.
- The reset preserves cleaning source-of-truth tables.
- The reset preserves or regenerates finance salary transactions with `source_type = 'cleaning_salary'`.
- The reset does not delete global finance categories or property-specific finance categories.
- The reset deletes finance recurring rules and all non-cleaning-salary finance transactions.
- The reset deletes finance import and booking data for the selected property.
- The reset deletes invoices linked to deleted finance booking rows, including invoice file metadata and physical invoice files.
- The reset preserves invoice number sequences; deleted invoice numbers must not be reused.
- The reset leaves audit logs intact, writes a normal audit event, and writes detailed counts to `finance_reset_runs`.
- The UI must provide a preview and confirmation dialog before the destructive action.

## 4. Definitions

- **Finance reset**: a destructive property-scoped operation that removes finance activity/import data while keeping required configuration and protected cleaning salary data.
- **Flat-entry cleaning data**: cleaning daily logs produced from the cleaner's property entry events, plus related fee and adjustment data used to compute salary.
- **Cleaning salary source of truth**: `cleaning_daily_logs`, `cleaning_salary_adjustments`, `cleaning_monthly_summaries`, and `cleaner_fee_history`.
- **Cleaning salary finance projection**: rows in `finance_transactions` where `source_type = 'cleaning_salary'` and `source_reference_id` is the salary month in `YYYY-MM` format.
- **Reset preview**: a dry-run count of rows and files that would be affected by the reset.

## 5. Existing Technical Context

Relevant backend files:

- `backend/internal/store/finance.go`
- `backend/internal/api/finance_handlers.go`
- `backend/internal/api/server.go`
- `backend/internal/store/cleaning.go`
- `backend/internal/migrate/000009_finance.up.sql`
- `backend/internal/migrate/000021_finance_bookings_ingestion.up.sql`
- `backend/internal/migrate/000024_finance_month_sync_state.up.sql`

Relevant frontend files:

- `frontend/src/views/FinanceView.vue`
- `frontend/src/views/FinanceView.spec.ts`
- `frontend/src/api/types/finance.ts`

Current finance transaction source types include:

- `manual`
- `booking_payout`
- `recurring_rule`
- `cleaning_salary`

`DeleteFinanceTransaction` currently deletes only non-auto-generated single transactions. This new feature requires a separate bulk reset path; do not widen the existing single-row delete behavior.

## 6. Functional Requirements

- A user with sufficient permission can preview a finance reset for the selected property.
- A user with sufficient permission can execute the reset only after explicit confirmation.
- The reset must never delete cleaning source-of-truth tables.
- The reset must not delete `finance_transactions` rows with `source_type = 'cleaning_salary'` unless the implementation immediately regenerates them from cleaning source data in the same operation.
- The reset must delete manual finance transactions for the property.
- The reset must delete Booking.com payout/statement finance transactions for the property.
- The reset must delete recurring-rule finance transactions for the property.
- The reset must delete finance recurring rules for the property.
- The reset must delete finance import rows and merge rows for the property.
- The reset must delete finance booking rows for the property.
- The reset must delete invoices linked to deleted finance booking rows, including invoice file metadata and physical invoice files.
- The reset must preserve invoice sequences so deleted invoice numbers are not reused.
- The reset must preserve finance-created synthetic occupancies with `source_type = 'booking_payout'` or `source_type = 'booking_statement'`.
- The reset must preserve finance categories.
- The reset must preserve audit logs.
- The reset must persist detailed reset counts in `finance_reset_runs`.
- The reset must remove physical attachment files for deleted finance transactions after the database transaction commits.
- The reset must return counts of deleted and preserved records.
- After reset, Finance UI summaries must still include cleaning salary expense where cleaning logs produce salary.
- After reset, manually syncing generated entries must not recreate deleted recurring transactions unless the user creates new recurring rules.

## 7. Non-Goals

- Do not implement a global multi-property wipe.
- Do not delete cleaning logs, salary adjustments, cleaning summaries, cleaner fee history, Nuki events, occupancy records, invoice sequences, or audit logs.
- Do not add any database migration beyond the required `finance_reset_runs` migration unless implementation discovers another real schema need.
- Do not change existing single-transaction delete semantics.
- Do not add automatic scheduled reset behavior.

## 8. Data Deletion Matrix

| Data | Table / storage | MVP behavior | Reason |
|---|---|---|---|
| Manual finance transactions | `finance_transactions` where `source_type = 'manual'` | Delete | User asked to remove finance records. |
| Booking payout/statement finance transactions | `finance_transactions` where `source_type = 'booking_payout'` | Delete | Imported finance activity. |
| Recurring generated transactions | `finance_transactions` where `source_type = 'recurring_rule'` | Delete | Generated finance activity. |
| Cleaning salary finance rows | `finance_transactions` where `source_type = 'cleaning_salary'` | Preserve or regenerate | Explicit exception in user request. |
| Recurring rules | `finance_recurring_rules` | Delete | Otherwise reset is undone by next generated sync. |
| Month sync state | `finance_month_states` | Preserve only for months with cleaning salary after regeneration; otherwise delete | Avoid stale synced state for deleted recurring data. |
| Finance categories | `finance_categories` | Preserve | Configuration, including global seed categories. |
| Finance bookings | `finance_bookings` | Delete | Imported finance records. |
| Finance imports | `finance_imports` | Delete | Import audit for deleted finance import data. |
| Booking merge logs | `finance_booking_merges` | Delete through booking/import deletion | Internal import history for deleted rows. |
| Finance attachments | filesystem under transaction attachment paths | Delete for deleted transactions after DB commit | Prevent orphaned files. |
| Cleaning daily logs | `cleaning_daily_logs` | Preserve | Salary source of truth. |
| Cleaning summaries | `cleaning_monthly_summaries` | Preserve/recompute as needed | Salary source of truth/cache. |
| Cleaning adjustments | `cleaning_salary_adjustments` | Preserve | Salary source of truth. |
| Cleaner fee history | `cleaner_fee_history` | Preserve | Salary computation source. |
| Linked invoices | `invoices` where linked to reset finance bookings | Delete | Confirmed product decision; high-risk accounting data. |
| Invoice file metadata | `invoice_files` for deleted invoices | Delete through invoice cascade | Keeps DB consistent with deleted invoices. |
| Physical invoice files | filesystem under invoice file paths | Delete after DB commit | Prevent orphaned generated invoice files. |
| Invoice sequences | `invoice_sequences` | Preserve | Deleted invoice numbers must not be reused. |
| Synthetic occupancies from finance imports | `occupancies` where `source_type IN ('booking_payout', 'booking_statement')` | Preserve | Confirmed product decision; occupancy history remains visible after finance reset. |
| Audit logs | existing audit table | Preserve and append reset event | Security and traceability. |
| Reset run details | `finance_reset_runs` | Insert one row per executed reset | Stores detailed counts that do not fit `api_audit_logs`. |

## 9. Backend Specification

### 9.1 API Endpoints

Add two property-scoped endpoints:

- `POST /api/properties/{id}/finance/reset/preview`
- `POST /api/properties/{id}/finance/reset`

Request body for execute:

```json
{
  "confirmed": true,
  "preserve_cleaning_salary": true
}
```

`preserve_cleaning_salary` must default to `true` if omitted. For MVP, reject `false` with `400 Bad Request` unless product explicitly approves deleting cleaning salary finance rows too.

Preview response:

```json
{
  "property_id": 123,
  "would_delete": {
    "finance_transactions": 42,
    "finance_recurring_rules": 3,
    "finance_bookings": 18,
    "finance_imports": 2,
    "finance_booking_merges": 18,
    "finance_month_states": 5,
    "finance_attachment_files": 7,
    "invoices": 4,
    "invoice_files": 4
  },
  "would_preserve": {
    "cleaning_salary_transactions": 4,
    "cleaning_daily_logs": 31,
    "cleaning_salary_adjustments": 1,
    "cleaner_fee_history": 2,
    "finance_categories": 10,
    "invoice_sequences": 1,
    "audit_logs": 0
  }
}
```

Execute response:

```json
{
  "ok": true,
  "deleted": {
    "finance_transactions": 42,
    "finance_recurring_rules": 3,
    "finance_bookings": 18,
    "finance_imports": 2,
    "finance_booking_merges": 18,
    "finance_month_states": 5,
    "finance_attachment_files": 7,
    "invoices": 4,
    "invoice_files": 4
  },
  "preserved": {
    "cleaning_salary_transactions": 4
  },
  "regenerated": {
    "cleaning_salary_inserted": 0,
    "cleaning_salary_updated": 4
  },
  "reset_run_id": 12
}
```

### 9.2 Database Migration

Add a dedicated reset-run table because `api_audit_logs` does not have metadata/count columns:

```sql
CREATE TABLE finance_reset_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    deleted_counts_json TEXT NOT NULL,
    preserved_counts_json TEXT NOT NULL,
    regenerated_counts_json TEXT NOT NULL,
    attachment_delete_errors_json TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_finance_reset_runs_property_completed
ON finance_reset_runs (property_id, completed_at DESC);
```

The JSON columns should use the same key names returned by the execute response. `attachment_delete_errors_json` should be `NULL` or an empty JSON array when every physical file delete succeeds.

### 9.3 Permission Rules

- Require owner/admin-level property access.
- If the permission model checks module permissions separately, also require property-scoped finance write access.
- Return `403` for insufficient permission.
- Audit both preview and execute attempts only if existing audit conventions support preview auditing; always audit execute success/failure.

### 9.4 Store Layer

Add store methods in `backend/internal/store/finance.go` or a new focused file such as `finance_reset.go`:

- `PreviewFinanceReset(ctx context.Context, propertyID int64) (*FinanceResetPreview, error)`
- `ResetFinanceRecords(ctx context.Context, propertyID int64, actorID *int64, loc *time.Location) (*FinanceResetResult, []string, error)`
- `CreateFinanceResetRun(ctx context.Context, result *FinanceResetResult, actorID *int64) (int64, error)`
- `UpdateFinanceResetRunFileDeleteErrors(ctx context.Context, resetRunID int64, attachmentDeleteErrors []string) error`

The second return value is a list of finance attachment and invoice file paths to delete after commit.

Recommended reset sequence:

Run steps 1-16 inside one database transaction:

1. Capture invoice IDs and invoice file paths for invoices linked to finance bookings for the property.
2. Capture attachment paths from transactions that will be deleted.
3. Capture months that have cleaning salary source data or existing `cleaning_salary` finance rows.
4. Delete linked invoices. `invoice_files` metadata should cascade through the existing FK; verify this in tests.
5. Delete non-cleaning salary finance transactions for the property.
6. Delete finance recurring rules for the property.
7. Delete finance booking merge rows connected to property bookings/imports.
8. Delete finance imports for the property.
9. Delete finance bookings for the property.
10. Delete finance month states for the property.
11. Preserve synthetic occupancies created from finance imports; do not delete or mark `occupancies.source_type IN ('booking_payout', 'booking_statement')` as deleted.
12. Recompute cleaning summaries and call existing generated-entry sync logic for only the captured cleaning salary months, or directly upsert `cleaning_salary` finance transactions from `ComputeCleaningMonthlySummary`.
13. For any captured cleaning salary month whose recomputed `FinalSalaryCents <= 0`, delete the stale `cleaning_salary` finance transaction for that property/month.
14. Reinsert/update `finance_month_states` for months where cleaning salary rows remain, using `last_synced_reason = 'finance_reset_preserve_cleaning_salary'`.
15. Insert `finance_reset_runs` with deleted, preserved, and regenerated counts, and `attachment_delete_errors_json = NULL`.
16. Commit.

After the database transaction commits:

17. Delete captured finance attachment files and invoice files from disk; failures must not roll back the already-committed database reset.
18. Update `finance_reset_runs.attachment_delete_errors_json` for the inserted run with any file-delete errors. If this update fails, log the error server-side but do not fail the already-completed reset response.

The implementation must be idempotent. Running reset twice should not fail and should report zero deletions on the second run, while preserving cleaning salary rows.

### 9.5 Invoice Deletion Safety

Before deleting finance bookings, identify invoices that reference property finance bookings:

```sql
SELECT i.id
FROM invoices i
JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id
WHERE fb.property_id = ?
```

Preview must include the count of invoices and invoice files that will be deleted. The confirmation dialog must explicitly say that linked invoices will be deleted.

Do not silently detach invoices in MVP. Do not delete or rewind `invoice_sequences`; future invoices must continue from the highest sequence already issued for that property/year.

### 9.6 Cleaning Salary Preservation

The reset must keep salary derived from flat entries visible in finance after completion.

Implementation options:

- Preferred: delete all non-cleaning finance data, preserve existing `cleaning_salary` transaction rows, then recompute/update them from `ComputeCleaningMonthlySummary` for affected months.
- Acceptable: delete all finance transactions, then regenerate only `cleaning_salary` rows from cleaning source data before commit.

Do not call the general `SyncFinanceGeneratedEntriesForMonth` after recurring rules are deleted unless verified that it cannot recreate unwanted rows. If using that function, recurring rules must already be deleted and tests must prove only cleaning salary rows come back.

Months to preserve/regenerate should be the union of:

- Months present in existing `finance_transactions` where `source_type = 'cleaning_salary'`.
- Months present in `cleaning_daily_logs` for the property with `counted_for_salary = 1`.
- Months present in `cleaning_salary_adjustments` for the property.

If a month computes to `FinalSalaryCents <= 0`, no `cleaning_salary` finance transaction is required. If an old `cleaning_salary` finance transaction already exists for that property/month, reset must delete it so the finance summary does not keep a stale cleaner expense.

### 9.7 Synthetic Occupancy Preservation

Finance imports can create synthetic occupancy rows when no matching occupancy exists. These rows use `source_type = 'booking_payout'` or `source_type = 'booking_statement'`.

Confirmed decision: reset keeps these synthetic occupancies. The reset removes finance/import data but does not delete or mark these occupancy rows as `deleted_from_source`. This means occupancy history may still show stays originally discovered from finance imports after the finance ledger has been reset.

## 10. Frontend Specification

Add a destructive-action area in the Finance view, preferably behind a clearly labelled dialog rather than a primary toolbar button.

Required UX:

- Button label: `Reset finance records`.
- Dialog title: `Reset finance records?`.
- Explain that manual transactions, imports, payout/statement rows, recurring rules, recurring generated rows, and finance attachments will be deleted.
- Explain that cleaning salary from flat entries will remain.
- Show preview counts before the final confirmation button is enabled.
- Require a confirmation dialog that explicitly mentions linked invoice deletion when invoice count is greater than zero.
- Disable submit while preview is loading or reset is running.
- On success, reload finance categories, transactions, summary, and recurring rules.

Do not hide the preserved cleaning salary rows after reset. The transaction list for months with salary should still show rows with category `Cleaning Salary` and `source_type = cleaning_salary`.

## 11. Testing Requirements

Backend store tests:

- Reset deletes manual transactions but preserves `cleaning_salary` transactions.
- Reset deletes `booking_payout` transactions and related finance booking/import rows.
- Reset deletes invoices linked to deleted finance bookings.
- Reset deletes invoice file metadata and physical invoice files for deleted invoices after DB commit.
- Reset preserves invoice sequences and does not reuse invoice numbers.
- Reset preserves synthetic occupancies with `source_type = 'booking_payout'` and `source_type = 'booking_statement'`.
- Reset deletes recurring rules and `recurring_rule` transactions.
- Reset preserves categories.
- Reset preserves cleaning logs, fee history, adjustments, and summaries.
- Reset recomputes cleaning salary from cleaning logs and adjustments.
- Reset deletes stale `cleaning_salary` finance rows for months whose recomputed final salary is zero.
- Reset writes a `finance_reset_runs` row with deleted, preserved, regenerated, and file-delete-error counts.
- Reset is idempotent.
- Reset remains idempotent when there are no remaining linked invoices after the first reset.

Backend API tests:

- Preview returns expected delete/preserve counts.
- Execute requires explicit confirmation from the confirmation dialog.
- Execute requires sufficient permission.
- Execute deletes linked invoices and reports their counts.
- Execute writes an audit event.
- Execute writes a `finance_reset_runs` row.

Frontend tests:

- Finance view opens reset dialog and loads preview.
- Submit remains disabled until preview is loaded and the confirmation dialog is accepted.
- Success calls reset endpoint and reloads finance data.
- Linked invoice deletion is shown in preview and confirmation copy.
- Copy explicitly states cleaning salary remains.

Suggested commands:

```sh
go test ./backend/internal/store ./backend/internal/api
cd frontend && npm test -- FinanceView
```

## 12. Acceptance Criteria

- A property owner can preview and execute a finance reset for one property.
- After reset, non-cleaning finance transactions are gone.
- After reset, recurring rules are gone and do not regenerate transactions.
- After reset, cleaning salary from flat-entry cleaning logs remains visible in finance.
- Cleaning module data is unchanged.
- Linked invoices and their files are deleted, and invoice sequences are preserved.
- Finance-created synthetic occupancies remain unchanged.
- Stale cleaning salary finance rows are removed when recomputed salary is zero.
- A `finance_reset_runs` row stores detailed reset counts.
- The operation is audited with deleted/preserved counts.
- Running the reset repeatedly is safe.

## 13. Implementation Notes For AI Agent

- Start by adding store-level preview/result structs and tests before wiring the API.
- Keep the reset logic separate from `DeleteFinanceTransaction`; that function intentionally protects auto-generated rows.
- Use a DB transaction for all database changes.
- Never delete files until the database transaction has committed.
- Do not introduce backward-compatibility branches unless a failing test exposes persisted legacy data that needs handling.
- Keep the UI minimal and consistent with the existing Finance view patterns and `useConfirm`/dialog components.
- Update `frontend/src/api/types/finance.ts` only if shared response types are needed.
