package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type CleanerFeeHistoryRow struct {
	ID                     int64
	PropertyID             int64
	CleaningFeeAmountCents int
	WashingFeeAmountCents  int
	EffectiveFrom          time.Time
	CreatedBy              sql.NullInt64
	CreatedAt              time.Time
}

type CleaningDailyLog struct {
	ID                 int64
	PropertyID         int64
	DayDate            string
	FirstEntryAt       sql.NullTime
	NukiEventReference sql.NullString
	CountedForSalary   bool
	CreatedAt          time.Time
}

type CleaningSalaryAdjustment struct {
	ID                    int64
	PropertyID            int64
	Year                  int
	Month                 int
	AdjustmentAmountCents int
	Reason                string
	CreatedBy             sql.NullInt64
	CreatedAt             time.Time
}

type CleaningMonthlySummary struct {
	PropertyID            int64
	Year                  int
	Month                 int
	CountedDays           int
	BaseSalaryCents       int
	AdjustmentsTotalCents int
	FinalSalaryCents      int
	ComputedAt            time.Time
}

type CleaningYearMonthCount struct {
	Month int
	Count int
}

func (s *Store) ListCleanerFeeHistory(ctx context.Context, propertyID int64) ([]CleanerFeeHistoryRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, cleaning_fee_amount_cents, washing_fee_amount_cents, effective_from, created_by, created_at
		FROM cleaner_fee_history
		WHERE property_id = ?
		ORDER BY effective_from DESC, id DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleanerFeeHistoryRow, 0)
	for rows.Next() {
		var r CleanerFeeHistoryRow
		var eff, created string
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.CleaningFeeAmountCents, &r.WashingFeeAmountCents, &eff, &r.CreatedBy, &created); err != nil {
			return nil, err
		}
		r.EffectiveFrom, _ = time.Parse(time.RFC3339, eff)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateCleanerFeeHistoryRow(ctx context.Context, propertyID int64, cleaningFeeCents, washingFeeCents int, effectiveFrom time.Time, createdBy *int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var by interface{}
	if createdBy != nil {
		by = *createdBy
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaner_fee_history (property_id, cleaning_fee_amount_cents, washing_fee_amount_cents, effective_from, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		propertyID, cleaningFeeCents, washingFeeCents, effectiveFrom.UTC().Format(time.RFC3339), by, now)
	return err
}

func (s *Store) UpsertCleaningDailyLog(ctx context.Context, row *CleaningDailyLog) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var first interface{}
	if row.FirstEntryAt.Valid {
		first = row.FirstEntryAt.Time.UTC().Format(time.RFC3339)
	}
	var ref interface{}
	if row.NukiEventReference.Valid {
		ref = row.NukiEventReference.String
	}
	counted := 0
	if row.CountedForSalary {
		counted = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_daily_logs (property_id, day_date, first_entry_at, nuki_event_reference, counted_for_salary, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, day_date) DO UPDATE SET
			first_entry_at = excluded.first_entry_at,
			nuki_event_reference = excluded.nuki_event_reference,
			counted_for_salary = excluded.counted_for_salary`,
		row.PropertyID, row.DayDate, first, ref, counted, now)
	return err
}

func (s *Store) ListCleaningDailyLogsForMonth(ctx context.Context, propertyID int64, month string) ([]CleaningDailyLog, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, day_date, first_entry_at, nuki_event_reference, counted_for_salary, created_at
		FROM cleaning_daily_logs
		WHERE property_id = ? AND substr(day_date, 1, 7) = ?
		ORDER BY day_date ASC`, propertyID, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleaningDailyLog, 0)
	for rows.Next() {
		var r CleaningDailyLog
		var first, created sql.NullString
		var counted int
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.DayDate, &first, &r.NukiEventReference, &counted, &created); err != nil {
			return nil, err
		}
		r.CountedForSalary = counted == 1
		if first.Valid && first.String != "" {
			t, _ := time.Parse(time.RFC3339, first.String)
			r.FirstEntryAt = sql.NullTime{Time: t, Valid: true}
		}
		if created.Valid && created.String != "" {
			r.CreatedAt, _ = time.Parse(time.RFC3339, created.String)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListCleaningAdjustmentsForMonth(ctx context.Context, propertyID int64, year, month int) ([]CleaningSalaryAdjustment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, year, month, adjustment_amount_cents, reason, created_by, created_at
		FROM cleaning_salary_adjustments
		WHERE property_id = ? AND year = ? AND month = ?
		ORDER BY created_at ASC, id ASC`, propertyID, year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleaningSalaryAdjustment, 0)
	for rows.Next() {
		var r CleaningSalaryAdjustment
		var created string
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.Year, &r.Month, &r.AdjustmentAmountCents, &r.Reason, &r.CreatedBy, &created); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateCleaningAdjustment(ctx context.Context, propertyID int64, year, month, amountCents int, reason string, createdBy *int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var by interface{}
	if createdBy != nil {
		by = *createdBy
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_salary_adjustments (property_id, year, month, adjustment_amount_cents, reason, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		propertyID, year, month, amountCents, reason, by, now)
	return err
}

func (s *Store) UpsertCleaningMonthlySummary(ctx context.Context, summary *CleaningMonthlySummary) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_monthly_summaries (property_id, year, month, counted_days, base_salary_cents, adjustments_total_cents, final_salary_cents, computed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, year, month) DO UPDATE SET
			counted_days = excluded.counted_days,
			base_salary_cents = excluded.base_salary_cents,
			adjustments_total_cents = excluded.adjustments_total_cents,
			final_salary_cents = excluded.final_salary_cents,
			computed_at = excluded.computed_at`,
		summary.PropertyID, summary.Year, summary.Month, summary.CountedDays,
		summary.BaseSalaryCents, summary.AdjustmentsTotalCents, summary.FinalSalaryCents, now)
	return err
}

func (s *Store) ComputeCleaningMonthlySummary(ctx context.Context, propertyID int64, year, month int, loc *time.Location) (*CleaningMonthlySummary, error) {
	monthKey := fmt.Sprintf("%04d-%02d", year, month)
	logs, err := s.ListCleaningDailyLogsForMonth(ctx, propertyID, monthKey)
	if err != nil {
		return nil, err
	}
	fees, err := s.ListCleanerFeeHistory(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	adj, err := s.ListCleaningAdjustmentsForMonth(ctx, propertyID, year, month)
	if err != nil {
		return nil, err
	}
	base := 0
	counted := 0
	for _, log := range logs {
		if !log.CountedForSalary || !log.FirstEntryAt.Valid {
			continue
		}
		counted++
		fee := feeForMoment(fees, log.FirstEntryAt.Time)
		base += fee.CleaningFeeAmountCents + fee.WashingFeeAmountCents
	}
	adjustments := 0
	for _, a := range adj {
		adjustments += a.AdjustmentAmountCents
	}
	final := base + adjustments
	summary := &CleaningMonthlySummary{
		PropertyID:            propertyID,
		Year:                  year,
		Month:                 month,
		CountedDays:           counted,
		BaseSalaryCents:       base,
		AdjustmentsTotalCents: adjustments,
		FinalSalaryCents:      final,
		ComputedAt:            time.Now().In(loc),
	}
	_ = s.UpsertCleaningMonthlySummary(ctx, summary)
	return summary, nil
}

func feeForMoment(fees []CleanerFeeHistoryRow, at time.Time) CleanerFeeHistoryRow {
	for _, f := range fees {
		if !f.EffectiveFrom.After(at.UTC()) {
			return f
		}
	}
	return CleanerFeeHistoryRow{}
}

func (s *Store) ListPropertyIDsWithCleanerAuthID(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id
		FROM properties p
		INNER JOIN property_profiles pp ON pp.property_id = p.id
		INNER JOIN property_secrets ps ON ps.property_id = p.id
		WHERE p.active = 1
		  AND pp.cleaner_nuki_auth_id IS NOT NULL AND TRIM(pp.cleaner_nuki_auth_id) != ''
		  AND ps.nuki_api_token IS NOT NULL AND TRIM(ps.nuki_api_token) != ''
		  AND ps.nuki_smartlock_id IS NOT NULL AND TRIM(ps.nuki_smartlock_id) != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ListCleaningYearMonthCounts(ctx context.Context, propertyID int64, year int) ([]CleaningYearMonthCount, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT CAST(substr(day_date, 6, 2) AS INTEGER) AS m, COUNT(1) AS c
		FROM cleaning_daily_logs
		WHERE property_id = ?
		  AND counted_for_salary = 1
		  AND substr(day_date, 1, 4) = ?
		GROUP BY m
		ORDER BY m ASC`, propertyID, fmt.Sprintf("%04d", year))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleaningYearMonthCount, 0, 12)
	for rows.Next() {
		var r CleaningYearMonthCount
		if err := rows.Scan(&r.Month, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
