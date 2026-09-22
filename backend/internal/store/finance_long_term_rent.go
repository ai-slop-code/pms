package store

import (
	"context"
	"database/sql"
	"time"
)

type FinanceLongTermRentRate struct {
	ID                 int64
	PropertyID         int64
	EffectiveFromMonth string
	MonthlyRentCents   int64
	Currency           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (s *Store) ListFinanceLongTermRentRates(ctx context.Context, propertyID int64) ([]FinanceLongTermRentRate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, effective_from_month, monthly_rent_cents, currency, created_at, updated_at
		FROM finance_long_term_rent_rates
		WHERE property_id = ?
		ORDER BY effective_from_month ASC, id ASC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rates := make([]FinanceLongTermRentRate, 0)
	for rows.Next() {
		var rate FinanceLongTermRentRate
		var created, updated string
		if err := rows.Scan(&rate.ID, &rate.PropertyID, &rate.EffectiveFromMonth, &rate.MonthlyRentCents, &rate.Currency, &created, &updated); err != nil {
			return nil, err
		}
		rate.CreatedAt, _ = time.Parse(time.RFC3339, created)
		rate.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

func (s *Store) GetFinanceLongTermRentRate(ctx context.Context, propertyID, rateID int64) (*FinanceLongTermRentRate, error) {
	var rate FinanceLongTermRentRate
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, effective_from_month, monthly_rent_cents, currency, created_at, updated_at
		FROM finance_long_term_rent_rates WHERE property_id = ? AND id = ?`, propertyID, rateID).
		Scan(&rate.ID, &rate.PropertyID, &rate.EffectiveFromMonth, &rate.MonthlyRentCents, &rate.Currency, &created, &updated)
	if err != nil {
		return nil, err
	}
	rate.CreatedAt, _ = time.Parse(time.RFC3339, created)
	rate.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &rate, nil
}

func (s *Store) ApplicableFinanceLongTermRentRate(ctx context.Context, propertyID int64, month string) (*FinanceLongTermRentRate, error) {
	var rate FinanceLongTermRentRate
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, effective_from_month, monthly_rent_cents, currency, created_at, updated_at
		FROM finance_long_term_rent_rates
		WHERE property_id = ? AND effective_from_month <= ?
		ORDER BY effective_from_month DESC, id DESC LIMIT 1`, propertyID, month).
		Scan(&rate.ID, &rate.PropertyID, &rate.EffectiveFromMonth, &rate.MonthlyRentCents, &rate.Currency, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rate.CreatedAt, _ = time.Parse(time.RFC3339, created)
	rate.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &rate, nil
}

func (s *Store) CreateFinanceLongTermRentRate(ctx context.Context, propertyID int64, month string, rentCents int64) (*FinanceLongTermRentRate, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx, `INSERT INTO finance_long_term_rent_rates
		(property_id, effective_from_month, monthly_rent_cents, currency, created_at, updated_at)
		VALUES (?, ?, ?, 'EUR', ?, ?)`, propertyID, month, rentCents, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetFinanceLongTermRentRate(ctx, propertyID, id)
}

func (s *Store) UpdateFinanceLongTermRentRate(ctx context.Context, propertyID, rateID int64, month string, rentCents int64) (*FinanceLongTermRentRate, error) {
	result, err := s.DB.ExecContext(ctx, `UPDATE finance_long_term_rent_rates
		SET effective_from_month = ?, monthly_rent_cents = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, month, rentCents, time.Now().UTC().Format(time.RFC3339), propertyID, rateID)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, sql.ErrNoRows
	}
	return s.GetFinanceLongTermRentRate(ctx, propertyID, rateID)
}

func (s *Store) DeleteFinanceLongTermRentRate(ctx context.Context, propertyID, rateID int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM finance_long_term_rent_rates WHERE property_id = ? AND id = ?`, propertyID, rateID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ComputeFinanceEligibleOutgoing applies the benchmark's source/category exclusions.
func (s *Store) ComputeFinanceEligibleOutgoing(ctx context.Context, propertyID int64, month string) (int64, error) {
	var outgoing int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(ft.amount_cents), 0)
		FROM finance_transactions ft
		LEFT JOIN finance_categories fc ON fc.id = ft.category_id
		WHERE ft.property_id = ?
		  AND ft.direction = 'outgoing'
		  AND substr(ft.transaction_date, 1, 7) = ?
		  AND ft.source_type <> 'booking_payout'
		  AND ft.source_type <> 'cleaning_salary'
		  AND (fc.code IS NULL OR fc.code <> 'cleaning_salary')`, propertyID, month).Scan(&outgoing)
	return outgoing, err
}
