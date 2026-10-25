package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	lateCheckoutSource      = "late_checkout"
	lateCheckoutBonusSource = "late_checkout_cleaner_bonus"
	lateCheckoutChargeCents = 3000
	lateCheckoutBonusCents  = 1000
)

func (s *Store) reconcileLateCheckoutTx(ctx context.Context, tx *sql.Tx, stay *NamedStay) error {
	eligible := stay.Status == NamedStayStatusActive &&
		(stay.StayType == StayTypeBookingCom || stay.StayType == StayTypeExternal) && stay.LateCheckout &&
		(!stay.StayOutcome.Valid || (stay.StayOutcome.String != StayOutcomeNoShow && stay.StayOutcome.String != StayOutcomeCancelledNonRefundable))
	ref := fmt.Sprintf("%d", stay.ID)
	if !eligible {
		_, err := tx.ExecContext(ctx, `DELETE FROM finance_transactions WHERE property_id = ? AND source_type IN (?, ?) AND source_reference_id = ?`, stay.PropertyID, lateCheckoutSource, lateCheckoutBonusSource, ref)
		return err
	}
	chargeCategory, err := findFinanceCategoryByCodeTx(ctx, tx, stay.PropertyID, "late_checkout")
	if err != nil {
		return err
	}
	bonusCategory, err := findFinanceCategoryByCodeTx(ctx, tx, stay.PropertyID, "cleaning_salary")
	if err != nil {
		return err
	}
	if chargeCategory == 0 || bonusCategory == 0 {
		return fmt.Errorf("late checkout finance category not found")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	date := stay.CheckOutDate + "T12:00:00Z"
	if err := upsertLateCheckoutEntryTx(ctx, tx, stay.PropertyID, date, "incoming", lateCheckoutChargeCents, chargeCategory, lateCheckoutSource, ref, "Late checkout", now); err != nil {
		return err
	}
	if stay.CleaningRequired {
		return upsertLateCheckoutEntryTx(ctx, tx, stay.PropertyID, date, "outgoing", lateCheckoutBonusCents, bonusCategory, lateCheckoutBonusSource, ref, "Late checkout cleaner bonus", now)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM finance_transactions WHERE property_id = ? AND source_type = ? AND source_reference_id = ?`, stay.PropertyID, lateCheckoutBonusSource, ref)
	return err
}

func upsertLateCheckoutEntryTx(ctx context.Context, tx *sql.Tx, propertyID int64, date, direction string, amount, categoryID int64, source, ref, note, now string) error {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM finance_transactions WHERE property_id = ? AND source_type = ? AND source_reference_id = ?`, propertyID, source, ref).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE finance_transactions SET transaction_date = ?, direction = ?, amount_cents = ?, category_id = ?, note = ?, is_auto_generated = 1, updated_at = ? WHERE id = ?`, date, direction, amount, categoryID, note, now, id)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO finance_transactions (property_id, transaction_date, direction, amount_cents, category_id, note, source_type, source_reference_id, is_auto_generated, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`, propertyID, date, direction, amount, categoryID, note, source, ref, now, now)
	return err
}
