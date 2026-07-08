package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type FinanceResetDeleteCounts struct {
	FinanceTransactions    int `json:"finance_transactions"`
	FinanceRecurringRules  int `json:"finance_recurring_rules"`
	FinanceBookings        int `json:"finance_bookings"`
	FinanceImports         int `json:"finance_imports"`
	FinanceBookingMerges   int `json:"finance_booking_merges"`
	FinanceMonthStates     int `json:"finance_month_states"`
	FinanceAttachmentFiles int `json:"finance_attachment_files"`
	Invoices               int `json:"invoices"`
	InvoiceFiles           int `json:"invoice_files"`
}

type FinanceResetPreserveCounts struct {
	CleaningSalaryTransactions int `json:"cleaning_salary_transactions"`
	CleaningDailyLogs          int `json:"cleaning_daily_logs"`
	CleaningSalaryAdjustments  int `json:"cleaning_salary_adjustments"`
	CleanerFeeHistory          int `json:"cleaner_fee_history"`
	FinanceCategories          int `json:"finance_categories"`
	InvoiceSequences           int `json:"invoice_sequences"`
	AuditLogs                  int `json:"audit_logs"`
}

type FinanceResetRegeneratedCounts struct {
	CleaningSalaryInserted int `json:"cleaning_salary_inserted"`
	CleaningSalaryUpdated  int `json:"cleaning_salary_updated"`
}

type FinanceResetPreview struct {
	PropertyID    int64                      `json:"property_id"`
	WouldDelete   FinanceResetDeleteCounts   `json:"would_delete"`
	WouldPreserve FinanceResetPreserveCounts `json:"would_preserve"`
}

type FinanceResetResult struct {
	PropertyID  int64                         `json:"-"`
	OK          bool                          `json:"ok"`
	Deleted     FinanceResetDeleteCounts      `json:"deleted"`
	Preserved   FinanceResetPreserveCounts    `json:"preserved"`
	Regenerated FinanceResetRegeneratedCounts `json:"regenerated"`
	ResetRunID  int64                         `json:"reset_run_id"`
}

func (s *Store) PreviewFinanceReset(ctx context.Context, propertyID int64) (*FinanceResetPreview, error) {
	preview := &FinanceResetPreview{PropertyID: propertyID}
	if err := s.fillFinanceResetDeleteCounts(ctx, s.DB, propertyID, &preview.WouldDelete); err != nil {
		return nil, err
	}
	if err := s.fillFinanceResetPreserveCounts(ctx, s.DB, propertyID, &preview.WouldPreserve); err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *Store) ResetFinanceRecords(ctx context.Context, propertyID int64, actorID *int64, loc *time.Location) (*FinanceResetResult, []string, error) {
	if loc == nil {
		loc = time.UTC
	}
	started := time.Now().UTC()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	result := &FinanceResetResult{PropertyID: propertyID, OK: true}
	if err := s.fillFinanceResetDeleteCounts(ctx, tx, propertyID, &result.Deleted); err != nil {
		return nil, nil, err
	}
	if err := s.fillFinanceResetPreserveCounts(ctx, tx, propertyID, &result.Preserved); err != nil {
		return nil, nil, err
	}
	filePaths, err := financeResetFilePaths(ctx, tx, propertyID)
	if err != nil {
		return nil, nil, err
	}
	months, err := financeResetCleaningMonths(ctx, tx, propertyID)
	if err != nil {
		return nil, nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM invoice_files
		WHERE invoice_id IN (
			SELECT i.id
			FROM invoices i
			JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id
			WHERE fb.property_id = ?
		)`, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM invoices
		WHERE id IN (
			SELECT i.id
			FROM invoices i
			JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id
			WHERE fb.property_id = ?
		)`, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM finance_transactions WHERE property_id = ? AND source_type <> 'cleaning_salary'`, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM finance_recurring_rules WHERE property_id = ?`, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM finance_booking_merges
		WHERE booking_id IN (SELECT id FROM finance_bookings WHERE property_id = ?)
		   OR import_id IN (SELECT id FROM finance_imports WHERE property_id = ?)`, propertyID, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM finance_imports WHERE property_id = ?`, propertyID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM finance_bookings WHERE property_id = ?`, propertyID); err != nil {
		return nil, nil, err
	}
	for _, month := range months {
		inserted, updated, err := financeResetUpsertCleaningSalary(ctx, tx, propertyID, month, actorID, loc)
		if err != nil {
			return nil, nil, err
		}
		result.Regenerated.CleaningSalaryInserted += inserted
		result.Regenerated.CleaningSalaryUpdated += updated
	}
	res, err := tx.ExecContext(ctx, `
		DELETE FROM finance_month_states
		WHERE property_id = ?
		  AND month NOT IN (
			SELECT source_reference_id
			FROM finance_transactions
			WHERE property_id = ?
			  AND source_type = 'cleaning_salary'
			  AND source_reference_id IS NOT NULL
			  AND source_reference_id <> ''
		  )`, propertyID, propertyID)
	if err != nil {
		return nil, nil, err
	}
	if aff, err := res.RowsAffected(); err == nil {
		result.Deleted.FinanceMonthStates = int(aff)
	}

	resetRunID, err := createFinanceResetRunTx(ctx, tx, propertyID, actorID, started, time.Now().UTC(), result)
	if err != nil {
		return nil, nil, err
	}
	result.ResetRunID = resetRunID

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return result, filePaths, nil
}

func (s *Store) CreateFinanceResetRun(ctx context.Context, result *FinanceResetResult, actorID *int64) (int64, error) {
	if result == nil {
		return 0, fmt.Errorf("finance reset result is required")
	}
	started := time.Now().UTC()
	return createFinanceResetRunDB(ctx, s.DB, result.PropertyID, actorID, started, started, result)
}

func (s *Store) UpdateFinanceResetRunFileDeleteErrors(ctx context.Context, resetRunID int64, attachmentDeleteErrors []string) error {
	var encoded interface{}
	if len(attachmentDeleteErrors) > 0 {
		b, err := json.Marshal(attachmentDeleteErrors)
		if err != nil {
			return err
		}
		encoded = string(b)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE finance_reset_runs SET attachment_delete_errors_json = ? WHERE id = ?`, encoded, resetRunID)
	return err
}

type financeResetQuerier interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func (s *Store) fillFinanceResetDeleteCounts(ctx context.Context, q financeResetQuerier, propertyID int64, counts *FinanceResetDeleteCounts) error {
	queries := []struct {
		dest *int
		sql  string
		args []interface{}
	}{
		{&counts.FinanceTransactions, `SELECT COUNT(*) FROM finance_transactions WHERE property_id = ? AND source_type <> 'cleaning_salary'`, []interface{}{propertyID}},
		{&counts.FinanceRecurringRules, `SELECT COUNT(*) FROM finance_recurring_rules WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.FinanceBookings, `SELECT COUNT(*) FROM finance_bookings WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.FinanceImports, `SELECT COUNT(*) FROM finance_imports WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.FinanceBookingMerges, `SELECT COUNT(*) FROM finance_booking_merges WHERE booking_id IN (SELECT id FROM finance_bookings WHERE property_id = ?) OR import_id IN (SELECT id FROM finance_imports WHERE property_id = ?)`, []interface{}{propertyID, propertyID}},
		{&counts.FinanceMonthStates, `
			SELECT COUNT(*)
			FROM finance_month_states
			WHERE property_id = ?
			  AND month NOT IN (
				SELECT source_reference_id
				FROM finance_transactions
				WHERE property_id = ?
				  AND source_type = 'cleaning_salary'
				  AND source_reference_id IS NOT NULL
				  AND source_reference_id <> ''
				UNION
				SELECT DISTINCT substr(day_date, 1, 7)
				FROM cleaning_daily_logs
				WHERE property_id = ? AND counted_for_salary = 1
				UNION
				SELECT DISTINCT printf('%04d-%02d', year, month)
				FROM cleaning_salary_adjustments
				WHERE property_id = ?
			  )`, []interface{}{propertyID, propertyID, propertyID, propertyID}},
		{&counts.FinanceAttachmentFiles, `SELECT COUNT(DISTINCT attachment_path) FROM finance_transactions WHERE property_id = ? AND source_type <> 'cleaning_salary' AND attachment_path IS NOT NULL AND attachment_path <> ''`, []interface{}{propertyID}},
		{&counts.Invoices, `SELECT COUNT(*) FROM invoices i JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id WHERE fb.property_id = ?`, []interface{}{propertyID}},
		{&counts.InvoiceFiles, `SELECT COUNT(*) FROM invoice_files f JOIN invoices i ON i.id = f.invoice_id JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id WHERE fb.property_id = ?`, []interface{}{propertyID}},
	}
	for _, query := range queries {
		if err := q.QueryRowContext(ctx, query.sql, query.args...).Scan(query.dest); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) fillFinanceResetPreserveCounts(ctx context.Context, q financeResetQuerier, propertyID int64, counts *FinanceResetPreserveCounts) error {
	queries := []struct {
		dest *int
		sql  string
		args []interface{}
	}{
		{&counts.CleaningSalaryTransactions, `SELECT COUNT(*) FROM finance_transactions WHERE property_id = ? AND source_type = 'cleaning_salary'`, []interface{}{propertyID}},
		{&counts.CleaningDailyLogs, `SELECT COUNT(*) FROM cleaning_daily_logs WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.CleaningSalaryAdjustments, `SELECT COUNT(*) FROM cleaning_salary_adjustments WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.CleanerFeeHistory, `SELECT COUNT(*) FROM cleaner_fee_history WHERE property_id = ?`, []interface{}{propertyID}},
		{&counts.FinanceCategories, `SELECT COUNT(*) FROM finance_categories WHERE active = 1 AND (property_id IS NULL OR property_id = ?)`, []interface{}{propertyID}},
		{&counts.InvoiceSequences, `SELECT COUNT(*) FROM invoice_sequences WHERE property_id = ?`, []interface{}{propertyID}},
	}
	for _, query := range queries {
		if err := q.QueryRowContext(ctx, query.sql, query.args...).Scan(query.dest); err != nil {
			return err
		}
	}
	counts.AuditLogs = 0
	return nil
}

func financeResetFilePaths(ctx context.Context, tx *sql.Tx, propertyID int64) ([]string, error) {
	paths := map[string]struct{}{}
	queries := []string{
		`SELECT attachment_path FROM finance_transactions WHERE property_id = ? AND source_type <> 'cleaning_salary' AND attachment_path IS NOT NULL AND attachment_path <> ''`,
		`SELECT f.file_path FROM invoice_files f JOIN invoices i ON i.id = f.invoice_id JOIN finance_bookings fb ON fb.id = i.finance_booking_payout_id WHERE fb.property_id = ?`,
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, query, propertyID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				rows.Close()
				return nil, err
			}
			p = strings.TrimSpace(p)
			if p != "" {
				paths[p] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	return out, nil
}

func financeResetCleaningMonths(ctx context.Context, tx *sql.Tx, propertyID int64) ([]string, error) {
	months := map[string]struct{}{}
	queries := []struct {
		sql  string
		args []interface{}
	}{
		{`SELECT source_reference_id FROM finance_transactions WHERE property_id = ? AND source_type = 'cleaning_salary' AND source_reference_id IS NOT NULL AND source_reference_id <> ''`, []interface{}{propertyID}},
		{`SELECT DISTINCT substr(day_date, 1, 7) FROM cleaning_daily_logs WHERE property_id = ? AND counted_for_salary = 1`, []interface{}{propertyID}},
		{`SELECT DISTINCT printf('%04d-%02d', year, month) FROM cleaning_salary_adjustments WHERE property_id = ?`, []interface{}{propertyID}},
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, query.sql, query.args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var month string
			if err := rows.Scan(&month); err != nil {
				rows.Close()
				return nil, err
			}
			if _, _, err := parseMonthYYYYMM(month); err == nil {
				months[month] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	out := make([]string, 0, len(months))
	for month := range months {
		out = append(out, month)
	}
	return out, nil
}

func financeResetUpsertCleaningSalary(ctx context.Context, tx *sql.Tx, propertyID int64, month string, actorID *int64, loc *time.Location) (int, int, error) {
	year, m, err := parseMonthYYYYMM(month)
	if err != nil {
		return 0, 0, err
	}
	summary, err := computeCleaningMonthlySummaryTx(ctx, tx, propertyID, year, m, loc)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cleaning_monthly_summaries (property_id, year, month, counted_days, base_salary_cents, adjustments_total_cents, final_salary_cents, computed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, year, month) DO UPDATE SET
			counted_days = excluded.counted_days,
			base_salary_cents = excluded.base_salary_cents,
			adjustments_total_cents = excluded.adjustments_total_cents,
			final_salary_cents = excluded.final_salary_cents,
			computed_at = excluded.computed_at`,
		propertyID, year, m, summary.CountedDays, summary.BaseSalaryCents, summary.AdjustmentsTotalCents, summary.FinalSalaryCents, now); err != nil {
		return 0, 0, err
	}
	monthAnchorUTC := time.Date(year, time.Month(m), 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if summary.FinalSalaryCents <= 0 {
		_, err := tx.ExecContext(ctx, `DELETE FROM finance_transactions WHERE property_id = ? AND source_type = 'cleaning_salary' AND source_reference_id = ?`, propertyID, month)
		return 0, 0, err
	}
	cleaningCategoryID, err := findFinanceCategoryByCodeTx(ctx, tx, propertyID, "cleaning_salary")
	if err != nil {
		return 0, 0, err
	}
	if cleaningCategoryID <= 0 {
		return 0, 0, fmt.Errorf("cleaning_salary finance category not found")
	}
	var existingID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM finance_transactions
		WHERE property_id = ? AND source_type = 'cleaning_salary' AND source_reference_id = ?
		LIMIT 1`, propertyID, month).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, err
	}
	if err == sql.ErrNoRows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO finance_transactions (
				property_id, transaction_date, direction, amount_cents, category_id, note,
				source_type, source_reference_id, is_auto_generated, created_at, updated_at
			) VALUES (?, ?, 'outgoing', ?, ?, ?, 'cleaning_salary', ?, 1, ?, ?)`,
			propertyID, monthAnchorUTC, summary.FinalSalaryCents, cleaningCategoryID, fmt.Sprintf("Cleaner salary for %s", month), month, now, now)
		if err != nil {
			return 0, 0, err
		}
		if err := upsertFinanceMonthStateAfterReset(ctx, tx, propertyID, month, actorID, now); err != nil {
			return 0, 0, err
		}
		return 1, 0, nil
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE finance_transactions
		SET transaction_date = ?, direction = 'outgoing', amount_cents = ?, category_id = ?, note = ?, is_auto_generated = 1, updated_at = ?
		WHERE id = ?`, monthAnchorUTC, summary.FinalSalaryCents, cleaningCategoryID, fmt.Sprintf("Cleaner salary for %s", month), now, existingID)
	if err != nil {
		return 0, 0, err
	}
	if err := upsertFinanceMonthStateAfterReset(ctx, tx, propertyID, month, actorID, now); err != nil {
		return 0, 0, err
	}
	return 0, 1, nil
}

func computeCleaningMonthlySummaryTx(ctx context.Context, tx *sql.Tx, propertyID int64, year, month int, loc *time.Location) (*CleaningMonthlySummary, error) {
	monthKey := fmt.Sprintf("%04d-%02d", year, month)
	fees, err := listCleanerFeesTx(ctx, tx, propertyID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT first_entry_at, counted_for_salary
		FROM cleaning_daily_logs
		WHERE property_id = ? AND substr(day_date, 1, 7) = ?`, propertyID, monthKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	base := 0
	counted := 0
	for rows.Next() {
		var first sql.NullString
		var countedForSalary int
		if err := rows.Scan(&first, &countedForSalary); err != nil {
			return nil, err
		}
		if countedForSalary != 1 || !first.Valid || strings.TrimSpace(first.String) == "" {
			continue
		}
		firstAt, err := time.Parse(time.RFC3339, first.String)
		if err != nil {
			continue
		}
		counted++
		fee := feeForMoment(fees, firstAt)
		base += fee.CleaningFeeAmountCents + fee.WashingFeeAmountCents
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	adjustments := 0
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(adjustment_amount_cents), 0)
		FROM cleaning_salary_adjustments
		WHERE property_id = ? AND year = ? AND month = ?`, propertyID, year, month).Scan(&adjustments); err != nil {
		return nil, err
	}
	return &CleaningMonthlySummary{
		PropertyID:            propertyID,
		Year:                  year,
		Month:                 month,
		CountedDays:           counted,
		BaseSalaryCents:       base,
		AdjustmentsTotalCents: adjustments,
		FinalSalaryCents:      base + adjustments,
		ComputedAt:            time.Now().In(loc),
	}, nil
}

func listCleanerFeesTx(ctx context.Context, tx *sql.Tx, propertyID int64) ([]CleanerFeeHistoryRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT cleaning_fee_amount_cents, washing_fee_amount_cents, effective_from
		FROM cleaner_fee_history
		WHERE property_id = ?
		ORDER BY effective_from DESC, id DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fees := make([]CleanerFeeHistoryRow, 0)
	for rows.Next() {
		var fee CleanerFeeHistoryRow
		var effective string
		if err := rows.Scan(&fee.CleaningFeeAmountCents, &fee.WashingFeeAmountCents, &effective); err != nil {
			return nil, err
		}
		fee.EffectiveFrom, _ = time.Parse(time.RFC3339, effective)
		fees = append(fees, fee)
	}
	return fees, rows.Err()
}

func upsertFinanceMonthStateAfterReset(ctx context.Context, tx *sql.Tx, propertyID int64, month string, actorID *int64, now string) error {
	var by interface{}
	if actorID != nil {
		by = *actorID
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO finance_month_states (property_id, month, opened_at, opened_by, last_synced_at, last_synced_by, last_synced_reason)
		VALUES (?, ?, ?, ?, ?, ?, 'finance_reset_preserve_cleaning_salary')
		ON CONFLICT(property_id, month) DO UPDATE SET
			last_synced_at = excluded.last_synced_at,
			last_synced_by = excluded.last_synced_by,
			last_synced_reason = excluded.last_synced_reason`, propertyID, month, now, by, now, by)
	return err
}

func createFinanceResetRunTx(ctx context.Context, tx *sql.Tx, propertyID int64, actorID *int64, startedAt, completedAt time.Time, result *FinanceResetResult) (int64, error) {
	return createFinanceResetRunDB(ctx, tx, propertyID, actorID, startedAt, completedAt, result)
}

func createFinanceResetRunDB(ctx context.Context, q financeResetQuerier, propertyID int64, actorID *int64, startedAt, completedAt time.Time, result *FinanceResetResult) (int64, error) {
	deletedJSON, err := json.Marshal(result.Deleted)
	if err != nil {
		return 0, err
	}
	preservedJSON, err := json.Marshal(result.Preserved)
	if err != nil {
		return 0, err
	}
	regeneratedJSON, err := json.Marshal(result.Regenerated)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := q.ExecContext(ctx, `
		INSERT INTO finance_reset_runs (
			property_id, actor_user_id, started_at, completed_at,
			deleted_counts_json, preserved_counts_json, regenerated_counts_json,
			attachment_delete_errors_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		propertyID, nullActorID(actorID), startedAt.UTC().Format(time.RFC3339), completedAt.UTC().Format(time.RFC3339),
		string(deletedJSON), string(preservedJSON), string(regeneratedJSON), now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullActorID(actorID *int64) interface{} {
	if actorID == nil {
		return nil
	}
	return *actorID
}
