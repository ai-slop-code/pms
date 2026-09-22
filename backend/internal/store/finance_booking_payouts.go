package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type FinanceBookingPayout struct {
	ID                      int64
	PropertyID              int64
	ReferenceNumber         string
	PayoutID                sql.NullString
	RowType                 sql.NullString
	CheckInDate             sql.NullString
	CheckOutDate            sql.NullString
	GuestName               sql.NullString
	ReservationStatus       sql.NullString
	Currency                sql.NullString
	PaymentStatus           sql.NullString
	AmountCents             sql.NullInt64
	CommissionCents         sql.NullInt64
	PaymentServiceFeeCents  sql.NullInt64
	NetCents                int
	PayoutDate              time.Time
	TransactionID           sql.NullInt64
	NamedStayID             sql.NullInt64
	OutcomeOverride         sql.NullString
	OutcomeOverrideMarkedAt sql.NullTime
	RawRowJSON              sql.NullString
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type FinanceBookingPayoutListRow struct {
	FinanceBookingPayout
	LinkedInvoiceID       sql.NullInt64
	NamedStayDisplayName  sql.NullString
	NamedStayType         sql.NullString
	NamedStayCheckInDate  sql.NullString
	NamedStayCheckOutDate sql.NullString
	HasPayoutData         bool
	HasStatementData      bool
}

func (s *Store) GetBookingPayoutByID(ctx context.Context, propertyID, payoutID int64) (*FinanceBookingPayout, error) {
	var r FinanceBookingPayout
	var payoutDate, created, updated string
	var outcomeMarkedAt sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name,
			reservation_status, currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents,
			net_cents, payout_date, transaction_id, named_stay_id, outcome_override, outcome_override_marked_at,
			raw_payout_row_json, created_at, updated_at
		FROM finance_bookings
		WHERE property_id = ? AND id = ?`, propertyID, payoutID).
		Scan(&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate, &r.GuestName,
			&r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents, &r.PaymentServiceFeeCents,
			&r.NetCents, &payoutDate, &r.TransactionID, &r.NamedStayID, &r.OutcomeOverride, &outcomeMarkedAt,
			&r.RawRowJSON, &created, &updated)
	if err != nil {
		return nil, err
	}
	r.PayoutDate, _ = time.Parse(time.RFC3339, payoutDate)
	if outcomeMarkedAt.Valid && outcomeMarkedAt.String != "" {
		t, _ := time.Parse(time.RFC3339, outcomeMarkedAt.String)
		r.OutcomeOverrideMarkedAt = sql.NullTime{Time: t, Valid: true}
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &r, nil
}

func (s *Store) GetBookingPayoutByReference(ctx context.Context, propertyID int64, referenceNumber string) (*FinanceBookingPayout, error) {
	var r FinanceBookingPayout
	var payoutDate, created, updated string
	var outcomeMarkedAt sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name,
			reservation_status, currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents,
			net_cents, payout_date, transaction_id, named_stay_id, outcome_override, outcome_override_marked_at,
			raw_payout_row_json, created_at, updated_at
		FROM finance_bookings
		WHERE property_id = ? AND reference_number = ?`, propertyID, referenceNumber).
		Scan(&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate, &r.GuestName,
			&r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents, &r.PaymentServiceFeeCents,
			&r.NetCents, &payoutDate, &r.TransactionID, &r.NamedStayID, &r.OutcomeOverride, &outcomeMarkedAt,
			&r.RawRowJSON, &created, &updated)
	if err != nil {
		return nil, err
	}
	r.PayoutDate, _ = time.Parse(time.RFC3339, payoutDate)
	if outcomeMarkedAt.Valid && outcomeMarkedAt.String != "" {
		t, _ := time.Parse(time.RFC3339, outcomeMarkedAt.String)
		r.OutcomeOverrideMarkedAt = sql.NullTime{Time: t, Valid: true}
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &r, nil
}

func (s *Store) CreateBookingPayout(ctx context.Context, row *FinanceBookingPayout) error {
	if row == nil {
		return fmt.Errorf("finance booking is required")
	}
	if err := s.validateFinanceBookingStay(ctx, row.PropertyID, row.NamedStayID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		INSERT INTO finance_bookings (
			property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name, reservation_status,
			currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents, net_cents, payout_date,
			transaction_id, named_stay_id, raw_payout_row_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.PropertyID, row.ReferenceNumber, nullStringValue(row.PayoutID), nullStringValue(row.RowType),
		nullStringValue(row.CheckInDate), nullStringValue(row.CheckOutDate), nullStringValue(row.GuestName), nullStringValue(row.ReservationStatus),
		nullStringValue(row.Currency), nullStringValue(row.PaymentStatus), nullInt64Value(row.AmountCents),
		nullInt64Value(row.CommissionCents), nullInt64Value(row.PaymentServiceFeeCents), row.NetCents,
		row.PayoutDate.UTC().Format(time.RFC3339), nullInt64Value(row.TransactionID), row.NamedStayID.Int64,
		nullStringValue(row.RawRowJSON), now, now)
	return err
}

// ImportBookingPayoutRow atomically persists a booking payout row and, when needed,
// the linked finance transaction in a single DB transaction. If existingTxID > 0
// the caller has already created (or located) the finance transaction and only
// the payout row needs inserting; otherwise txInput is inserted first and the
// resulting ID is written into the payout row. This ensures a failure can never
// leave a finance row without its payout mapping (or vice versa).
func (s *Store) ImportBookingPayoutRow(ctx context.Context, txInput *FinanceTransaction, payout *FinanceBookingPayout, existingTxID int64) (int64, error) {
	if payout == nil {
		return 0, fmt.Errorf("finance booking is required")
	}
	if err := s.validateFinanceBookingStay(ctx, payout.PropertyID, payout.NamedStayID); err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339)
	txID := existingTxID
	if txID <= 0 {
		if txInput == nil {
			return 0, fmt.Errorf("finance transaction input required")
		}
		auto := 0
		if txInput.IsAutoGenerated {
			auto = 1
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO finance_transactions (
				property_id, transaction_date, direction, amount_cents, category_id, note,
				source_type, source_reference_id, is_auto_generated, attachment_path, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			txInput.PropertyID, txInput.TransactionDate.UTC().Format(time.RFC3339), txInput.Direction, txInput.AmountCents,
			nullInt64Value(txInput.CategoryID), nullStringValue(txInput.Note), txInput.SourceType,
			nullStringValue(txInput.SourceReference), auto, nullStringValue(txInput.AttachmentPath), now, now)
		if err != nil {
			return 0, err
		}
		txID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	}
	payout.TransactionID = sql.NullInt64{Int64: txID, Valid: txID > 0}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO finance_bookings (
			property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name, reservation_status,
			currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents, net_cents, payout_date,
			transaction_id, named_stay_id, raw_payout_row_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		payout.PropertyID, payout.ReferenceNumber, nullStringValue(payout.PayoutID), nullStringValue(payout.RowType),
		nullStringValue(payout.CheckInDate), nullStringValue(payout.CheckOutDate), nullStringValue(payout.GuestName), nullStringValue(payout.ReservationStatus),
		nullStringValue(payout.Currency), nullStringValue(payout.PaymentStatus), nullInt64Value(payout.AmountCents),
		nullInt64Value(payout.CommissionCents), nullInt64Value(payout.PaymentServiceFeeCents), payout.NetCents,
		payout.PayoutDate.UTC().Format(time.RFC3339), nullInt64Value(payout.TransactionID), payout.NamedStayID.Int64,
		nullStringValue(payout.RawRowJSON), now, now); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	return txID, nil
}

// BackfillBookingPayoutTransaction creates a finance_transactions row for an
// existing finance_bookings row whose transaction_id is NULL, and links them
// atomically. If the booking already has a transaction_id, the existing id is
// returned and no insert happens. This repairs orphan payout rows whose
// matching transaction was lost (e.g. deleted manually or never created due to
// a prior failed import).
func (s *Store) BackfillBookingPayoutTransaction(ctx context.Context, payoutID int64, txInput *FinanceTransaction) (int64, error) {
	if payoutID <= 0 {
		return 0, fmt.Errorf("payout id required")
	}
	if txInput == nil {
		return 0, fmt.Errorf("finance transaction input required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	var existingTxID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT transaction_id FROM finance_bookings WHERE id = ?`, payoutID).Scan(&existingTxID); err != nil {
		return 0, err
	}
	if existingTxID.Valid && existingTxID.Int64 > 0 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		tx = nil
		return existingTxID.Int64, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	auto := 0
	if txInput.IsAutoGenerated {
		auto = 1
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO finance_transactions (
			property_id, transaction_date, direction, amount_cents, category_id, note,
			source_type, source_reference_id, is_auto_generated, attachment_path, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		txInput.PropertyID, txInput.TransactionDate.UTC().Format(time.RFC3339), txInput.Direction, txInput.AmountCents,
		nullInt64Value(txInput.CategoryID), nullStringValue(txInput.Note), txInput.SourceType,
		nullStringValue(txInput.SourceReference), auto, nullStringValue(txInput.AttachmentPath), now, now)
	if err != nil {
		return 0, err
	}
	txID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE finance_bookings
		SET transaction_id = ?, updated_at = ?
		WHERE id = ? AND transaction_id IS NULL`, txID, now, payoutID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	return txID, nil
}

func (s *Store) UpdateBookingPayoutNamedStayMapping(ctx context.Context, propertyID int64, referenceNumber string, namedStayID *int64) error {
	if namedStayID == nil || *namedStayID <= 0 {
		return fmt.Errorf("named_stay_id is required")
	}
	if err := s.validateFinanceBookingStay(ctx, propertyID, sql.NullInt64{Int64: *namedStayID, Valid: true}); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		UPDATE finance_bookings
		SET named_stay_id = ?, updated_at = ?
		WHERE property_id = ? AND reference_number = ?`, *namedStayID, now, propertyID, referenceNumber)
	if err != nil {
		return err
	}
	_, err = s.ConfirmNamedStayWithFinanceEvidence(ctx, propertyID, *namedStayID)
	return err
}

func (s *Store) LinkBookingToNamedStay(ctx context.Context, propertyID, bookingID, namedStayID int64) error {
	if bookingID <= 0 || namedStayID <= 0 {
		return fmt.Errorf("named_stay_id is required")
	}
	if err := s.validateFinanceBookingStay(ctx, propertyID, sql.NullInt64{Int64: namedStayID, Valid: true}); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		UPDATE finance_bookings
		SET named_stay_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, namedStayID, now, propertyID, bookingID)
	if err != nil {
		return err
	}
	_, err = s.ConfirmNamedStayWithFinanceEvidence(ctx, propertyID, namedStayID)
	return err
}

func (s *Store) validateFinanceBookingStay(ctx context.Context, propertyID int64, namedStayID sql.NullInt64) error {
	if !namedStayID.Valid || namedStayID.Int64 <= 0 {
		return fmt.Errorf("named_stay_id is required")
	}
	var exists int
	if err := dbForContext(ctx, s.DB).QueryRowContext(ctx, `SELECT 1 FROM named_stays WHERE property_id = ? AND id = ?`, propertyID, namedStayID.Int64).Scan(&exists); err != nil {
		return fmt.Errorf("invalid named_stay_id: %w", err)
	}
	return nil
}

// ConfirmNamedStayWithFinanceEvidence upgrades only migration-created review
// state. Operational review reasons such as a later cancellation remain intact.
func (s *Store) ConfirmNamedStayWithFinanceEvidence(ctx context.Context, propertyID, namedStayID int64) (bool, error) {
	if propertyID <= 0 || namedStayID <= 0 {
		return false, nil
	}
	if err := s.lowerNamedStayFirstKnownFromFinanceEvidence(ctx, propertyID, namedStayID); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	res, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		UPDATE named_stays
		SET review_status = 'confirmed',
		    review_resolution = 'confirmed',
		    review_reason = NULL,
		    updated_at = ?
		WHERE property_id = ?
		  AND id = ?
		  AND review_status = 'needs_review'
		  AND review_resolution IS NULL
		  AND review_reason = 'legacy_non_reservation_stay'
		  AND EXISTS (
		      SELECT 1
		      FROM finance_bookings fb
		      WHERE fb.property_id = named_stays.property_id
		        AND fb.named_stay_id = named_stays.id
		        AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
		        AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
		        AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
		            ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
		  )`, now.Format(time.RFC3339), propertyID, namedStayID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}

func (s *Store) lowerNamedStayFirstKnownFromFinanceEvidence(ctx context.Context, propertyID, namedStayID int64) error {
	_, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		WITH earliest AS (
			SELECT booked_on
			FROM finance_bookings
			WHERE property_id = ? AND named_stay_id = ?
			  AND booked_on IS NOT NULL AND trim(booked_on) <> ''
			ORDER BY julianday(booked_on) IS NULL, julianday(booked_on), booked_on
			LIMIT 1
		)
		UPDATE named_stays
		SET first_known_at = (SELECT booked_on FROM earliest)
		WHERE property_id = ? AND id = ?
		  AND EXISTS (SELECT 1 FROM earliest)
		  AND (first_known_at IS NULL OR trim(first_known_at) = ''
		       OR julianday((SELECT booked_on FROM earliest)) < julianday(first_known_at))`,
		propertyID, namedStayID, propertyID, namedStayID)
	return err
}

func (s *Store) FindNamedStayForFinanceStayDates(ctx context.Context, propertyID int64, referenceNumber, checkInDate, checkOutDate, guestName string) (*NamedStay, error) {
	checkInDate = strings.TrimSpace(checkInDate)
	checkOutDate = strings.TrimSpace(checkOutDate)
	if checkInDate == "" || checkOutDate == "" {
		return nil, nil
	}
	ref := strings.TrimSpace(referenceNumber)
	if ref != "" {
		rows, err := s.DB.QueryContext(ctx, namedStaySelectSQL+`
			WHERE ns.property_id = ? AND ns.source_reference = ? AND ns.check_in_date = ? AND ns.check_out_date = ?
			  AND ns.status = 'active' AND ns.stay_type IN ('booking_com', 'external')`, propertyID, ref, checkInDate, checkOutDate)
		if err != nil {
			return nil, err
		}
		stays, err := scanNamedStays(rows)
		if err != nil {
			return nil, err
		}
		if len(stays) == 1 {
			return &stays[0], nil
		}
		if len(stays) > 1 {
			return nil, nil
		}
	}
	query := namedStaySelectSQL + `
		WHERE ns.property_id = ? AND ns.check_in_date = ? AND ns.check_out_date = ?
		  AND ns.status = 'active' AND ns.stay_type IN ('booking_com', 'external')`
	args := []interface{}{propertyID, checkInDate, checkOutDate}
	guest := strings.TrimSpace(guestName)
	if guest != "" {
		query += ` AND LOWER(TRIM(ns.display_name)) = LOWER(TRIM(?))`
		args = append(args, guest)
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	stays, err := scanNamedStays(rows)
	if err != nil {
		return nil, err
	}
	if len(stays) == 1 {
		return &stays[0], nil
	}
	return nil, nil
}

func (s *Store) ListNamedStaysForFinanceReferenceDates(ctx context.Context, propertyID int64, referenceNumber, checkInDate, checkOutDate string) ([]NamedStay, error) {
	rows, err := s.DB.QueryContext(ctx, namedStaySelectSQL+`
		WHERE ns.property_id = ? AND ns.source_reference = ? AND ns.check_in_date = ? AND ns.check_out_date = ?
		  AND ns.status = 'active' AND ns.stay_type = 'booking_com'`, propertyID, strings.TrimSpace(referenceNumber), strings.TrimSpace(checkInDate), strings.TrimSpace(checkOutDate))
	if err != nil {
		return nil, err
	}
	return scanNamedStays(rows)
}

// ListNamedStaysForFinanceExactDates returns all eligible Booking.com stays
// for the supplied named-stay dates. It intentionally does not inspect raw
// blocks or source links.
func (s *Store) ListNamedStaysForFinanceExactDates(ctx context.Context, propertyID int64, checkInDate, checkOutDate string) ([]NamedStayFinanceCandidate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
		       COALESCE(ns.review_resolution, ns.review_status), ns.manual_revenue_cents,
		       CASE WHEN EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id) THEN 1 ELSE 0 END
		FROM named_stays ns
		WHERE ns.property_id = ? AND ns.check_in_date = ? AND ns.check_out_date = ?
		  AND ns.status = 'active' AND ns.stay_type = 'booking_com'
		ORDER BY ns.id`, propertyID, strings.TrimSpace(checkInDate), strings.TrimSpace(checkOutDate))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NamedStayFinanceCandidate, 0)
	for rows.Next() {
		var row NamedStayFinanceCandidate
		var has int
		if err := rows.Scan(&row.ID, &row.DisplayName, &row.StayType, &row.CheckInDate, &row.CheckOutDate, &row.Status, &row.ReviewStatus, &row.ManualRevenueCents, &has); err != nil {
			return nil, err
		}
		row.HasFinanceData = has != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) FinanceBookingNamedStay(ctx context.Context, propertyID, bookingID int64) (*NamedStay, error) {
	var stayID sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT named_stay_id FROM finance_bookings WHERE property_id = ? AND id = ?`, propertyID, bookingID).Scan(&stayID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if !stayID.Valid || stayID.Int64 <= 0 {
		return nil, nil
	}
	return s.GetNamedStay(ctx, propertyID, stayID.Int64)
}

// UpdateFinanceMatchedStayName changes only the operator-facing name. It is
// deliberately narrower than the general stay editor: payout imports must not
// alter dates, source links, nights, lifecycle, or access-code state.
func (s *Store) UpdateFinanceMatchedStayName(ctx context.Context, propertyID, stayID int64, name string, userID int64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("display name is required")
	}
	db := dbForContext(ctx, s.DB)
	var previous string
	if err := db.QueryRowContext(ctx, `SELECT display_name FROM named_stays WHERE property_id = ? AND id = ? AND status = 'active' AND stay_type = 'booking_com'`, propertyID, stayID).Scan(&previous); err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE named_stays
		SET display_name = ?, updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ? AND status = 'active' AND stay_type = 'booking_com' AND display_name = ?`,
		name, nullableInt64(userID), time.Now().UTC().Format(time.RFC3339), propertyID, stayID, previous)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("named stay changed during payout import")
	}
	return nil
}

func (s *Store) MarkNamedStayFinanceReviewForBooking(ctx context.Context, propertyID, bookingID int64, reason string) error {
	if bookingID <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE named_stays
		SET review_status = 'needs_review', review_resolution = NULL, review_reason = ?,
		    review_actor_user_id = NULL, reviewed_at = NULL, updated_at = ?
		WHERE property_id = ?
		  AND id = (SELECT named_stay_id FROM finance_bookings WHERE property_id = ? AND id = ? AND named_stay_id IS NOT NULL)
		  AND status = 'active'`, strings.TrimSpace(reason), now, propertyID, propertyID, bookingID)
	return err
}

func (s *Store) FinanceCategoryIDByCode(ctx context.Context, propertyID int64, code string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT id
		FROM finance_categories
		WHERE active = 1
		  AND code = ?
		  AND (property_id IS NULL OR property_id = ?)
		ORDER BY CASE WHEN property_id IS NULL THEN 1 ELSE 0 END
		LIMIT 1`, code, propertyID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (s *Store) FinanceTransactionBySourceReference(ctx context.Context, propertyID int64, sourceType, sourceReference string) (*FinanceTransaction, error) {
	return s.financeTransactionBySourceReference(ctx, dbForContext(ctx, s.DB), propertyID, sourceType, sourceReference)
}

func (s *Store) financeTransactionBySourceReference(ctx context.Context, db sqlContextDB, propertyID int64, sourceType, sourceReference string) (*FinanceTransaction, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ft.id, ft.property_id, ft.transaction_date, ft.direction, ft.amount_cents, ft.category_id,
			ft.note, ft.source_type, ft.source_reference_id, ft.is_auto_generated, ft.attachment_path, ft.created_at, ft.updated_at,
			fc.code, fc.title, COALESCE(fc.counts_toward_property_income, 0),
			CASE WHEN fbp.named_stay_id IS NOT NULL THEN 1 ELSE 0 END
		FROM finance_transactions ft
		LEFT JOIN finance_categories fc ON fc.id = ft.category_id
		LEFT JOIN finance_bookings fbp
		  ON fbp.property_id = ft.property_id
		 AND fbp.reference_number = ft.source_reference_id
		WHERE ft.property_id = ? AND ft.source_type = ? AND ft.source_reference_id = ?
		ORDER BY ft.id DESC
		LIMIT 1`, propertyID, sourceType, sourceReference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanFinanceTransactionsRows(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

func (s *Store) ListBookingPayouts(ctx context.Context, propertyID int64, month string, mappedOnly *bool) ([]FinanceBookingPayoutListRow, error) {
	query := `
		SELECT
			fbp.id, fbp.property_id, fbp.reference_number, fbp.payout_id, fbp.row_type, fbp.check_in_date, fbp.check_out_date,
			fbp.guest_name, fbp.reservation_status, fbp.currency, fbp.payment_status, fbp.amount_cents, fbp.commission_cents,
			fbp.payment_service_fee_cents, fbp.net_cents, fbp.payout_date, fbp.transaction_id, fbp.named_stay_id, fbp.raw_payout_row_json,
			fbp.outcome_override, fbp.outcome_override_marked_at, fbp.created_at, fbp.updated_at,
			(
				SELECT i.id FROM invoices i
				WHERE i.property_id = fbp.property_id AND i.finance_booking_payout_id = fbp.id
				LIMIT 1
			) AS linked_invoice_id,
			ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date,
			fbp.has_payout_data, fbp.has_statement_data
		FROM finance_bookings fbp
		LEFT JOIN named_stays ns
		  ON ns.id = fbp.named_stay_id
		 AND ns.property_id = fbp.property_id
		WHERE fbp.property_id = ?`
	args := []interface{}{propertyID}
	if month != "" {
		query += ` AND substr(fbp.payout_date, 1, 7) = ?`
		args = append(args, month)
	}
	if mappedOnly != nil {
		if *mappedOnly {
			query += ` AND fbp.named_stay_id IS NOT NULL`
		} else {
			query += ` AND fbp.named_stay_id IS NULL`
		}
	}
	query += ` ORDER BY fbp.payout_date DESC, fbp.id DESC`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FinanceBookingPayoutListRow, 0)
	for rows.Next() {
		var r FinanceBookingPayoutListRow
		var payoutDate, created, updated string
		var outcomeMarkedAt sql.NullString
		var hasPayout, hasStatement int
		if err := rows.Scan(
			&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate,
			&r.GuestName, &r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents,
			&r.PaymentServiceFeeCents, &r.NetCents, &payoutDate, &r.TransactionID, &r.NamedStayID, &r.RawRowJSON,
			&r.OutcomeOverride, &outcomeMarkedAt, &created, &updated, &r.LinkedInvoiceID,
			&r.NamedStayDisplayName, &r.NamedStayType, &r.NamedStayCheckInDate, &r.NamedStayCheckOutDate,
			&hasPayout, &hasStatement,
		); err != nil {
			return nil, err
		}
		r.HasPayoutData = hasPayout != 0
		r.HasStatementData = hasStatement != 0
		r.PayoutDate, _ = time.Parse(time.RFC3339, payoutDate)
		if outcomeMarkedAt.Valid && outcomeMarkedAt.String != "" {
			t, _ := time.Parse(time.RFC3339, outcomeMarkedAt.String)
			r.OutcomeOverrideMarkedAt = sql.NullTime{Time: t, Valid: true}
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListOrphanBookingPayouts returns finance_bookings rows whose transaction_id is
// NULL, across all properties. These rows represent payout bookings that did
// not get a matching finance_transactions row (typically due to a partial or
// failed import). Used by the finance-repair CLI to backfill them.
func (s *Store) ListOrphanBookingPayouts(ctx context.Context) ([]FinanceBookingPayout, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name,
			reservation_status, currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents,
			net_cents, payout_date, transaction_id, named_stay_id, raw_payout_row_json, created_at, updated_at
		FROM finance_bookings
		WHERE transaction_id IS NULL
		ORDER BY property_id, payout_date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FinanceBookingPayout, 0)
	for rows.Next() {
		var r FinanceBookingPayout
		var payoutDate, created, updated string
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate, &r.GuestName,
			&r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents, &r.PaymentServiceFeeCents,
			&r.NetCents, &payoutDate, &r.TransactionID, &r.NamedStayID, &r.RawRowJSON, &created, &updated); err != nil {
			return nil, err
		}
		r.PayoutDate, _ = time.Parse(time.RFC3339, payoutDate)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}
