package store

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
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
	OccupancyID             sql.NullInt64
	NamedStayID             sql.NullInt64
	OutcomeOverride         sql.NullString
	OutcomeOverrideMarkedAt sql.NullTime
	RawRowJSON              sql.NullString
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type FinanceBookingPayoutListRow struct {
	FinanceBookingPayout
	LinkedInvoiceID         sql.NullInt64
	OccupancySourceEventUID sql.NullString
	OccupancyStartAt        sql.NullTime
	OccupancyEndAt          sql.NullTime
	OccupancySummary        sql.NullString
	NamedStayDisplayName    sql.NullString
	NamedStayType           sql.NullString
	NamedStayCheckInDate    sql.NullString
	NamedStayCheckOutDate   sql.NullString
	HasPayoutData           bool
	HasStatementData        bool
}

func (s *Store) GetBookingPayoutByID(ctx context.Context, propertyID, payoutID int64) (*FinanceBookingPayout, error) {
	var r FinanceBookingPayout
	var payoutDate, created, updated string
	var outcomeMarkedAt sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name,
			reservation_status, currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents,
			net_cents, payout_date, transaction_id, occupancy_id, named_stay_id, outcome_override, outcome_override_marked_at,
			raw_payout_row_json, created_at, updated_at
		FROM finance_bookings
		WHERE property_id = ? AND id = ?`, propertyID, payoutID).
		Scan(&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate, &r.GuestName,
			&r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents, &r.PaymentServiceFeeCents,
			&r.NetCents, &payoutDate, &r.TransactionID, &r.OccupancyID, &r.NamedStayID, &r.OutcomeOverride, &outcomeMarkedAt,
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
			net_cents, payout_date, transaction_id, occupancy_id, named_stay_id, outcome_override, outcome_override_marked_at,
			raw_payout_row_json, created_at, updated_at
		FROM finance_bookings
		WHERE property_id = ? AND reference_number = ?`, propertyID, referenceNumber).
		Scan(&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate, &r.GuestName,
			&r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents, &r.PaymentServiceFeeCents,
			&r.NetCents, &payoutDate, &r.TransactionID, &r.OccupancyID, &r.NamedStayID, &r.OutcomeOverride, &outcomeMarkedAt,
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
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO finance_bookings (
			property_id, reference_number, payout_id, row_type, check_in_date, check_out_date, guest_name, reservation_status,
			currency, payment_status, amount_cents, commission_cents, payment_service_fee_cents, net_cents, payout_date,
			transaction_id, occupancy_id, named_stay_id, raw_payout_row_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.PropertyID, row.ReferenceNumber, nullStringValue(row.PayoutID), nullStringValue(row.RowType),
		nullStringValue(row.CheckInDate), nullStringValue(row.CheckOutDate), nullStringValue(row.GuestName), nullStringValue(row.ReservationStatus),
		nullStringValue(row.Currency), nullStringValue(row.PaymentStatus), nullInt64Value(row.AmountCents),
		nullInt64Value(row.CommissionCents), nullInt64Value(row.PaymentServiceFeeCents), row.NetCents,
		row.PayoutDate.UTC().Format(time.RFC3339), nullInt64Value(row.TransactionID), nullInt64Value(row.OccupancyID), nullInt64Value(row.NamedStayID),
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
			transaction_id, occupancy_id, named_stay_id, raw_payout_row_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		payout.PropertyID, payout.ReferenceNumber, nullStringValue(payout.PayoutID), nullStringValue(payout.RowType),
		nullStringValue(payout.CheckInDate), nullStringValue(payout.CheckOutDate), nullStringValue(payout.GuestName), nullStringValue(payout.ReservationStatus),
		nullStringValue(payout.Currency), nullStringValue(payout.PaymentStatus), nullInt64Value(payout.AmountCents),
		nullInt64Value(payout.CommissionCents), nullInt64Value(payout.PaymentServiceFeeCents), payout.NetCents,
		payout.PayoutDate.UTC().Format(time.RFC3339), nullInt64Value(payout.TransactionID), nullInt64Value(payout.OccupancyID), nullInt64Value(payout.NamedStayID),
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

func (s *Store) UpdateBookingPayoutMapping(ctx context.Context, propertyID int64, referenceNumber string, occupancyID *int64) error {
	var occ interface{}
	if occupancyID != nil && *occupancyID > 0 {
		occ = *occupancyID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE finance_bookings
		SET occupancy_id = ?, updated_at = ?
		WHERE property_id = ? AND reference_number = ?`, occ, now, propertyID, referenceNumber)
	return err
}

func (s *Store) UpdateBookingPayoutNamedStayMapping(ctx context.Context, propertyID int64, referenceNumber string, namedStayID *int64) error {
	var stay interface{}
	var occ interface{}
	if namedStayID != nil && *namedStayID > 0 {
		stay = *namedStayID
		var legacy sql.NullInt64
		_ = s.DB.QueryRowContext(ctx, `
			SELECT old_occupancy_id
			FROM occupancy_stay_migration_map
			WHERE property_id = ? AND named_stay_id = ? AND migration_kind = 'named_stay'
			ORDER BY old_occupancy_id DESC LIMIT 1`, propertyID, *namedStayID).Scan(&legacy)
		if legacy.Valid {
			occ = legacy.Int64
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE finance_bookings
		SET named_stay_id = ?, occupancy_id = ?, updated_at = ?
		WHERE property_id = ? AND reference_number = ?`, stay, occ, now, propertyID, referenceNumber)
	return err
}

func (s *Store) LinkBookingToNamedStay(ctx context.Context, propertyID, bookingID, namedStayID int64) error {
	if bookingID <= 0 || namedStayID <= 0 {
		return nil
	}
	var legacy sql.NullInt64
	_ = s.DB.QueryRowContext(ctx, `
		SELECT old_occupancy_id
		FROM occupancy_stay_migration_map
		WHERE property_id = ? AND named_stay_id = ? AND migration_kind = 'named_stay'
		ORDER BY old_occupancy_id DESC LIMIT 1`, propertyID, namedStayID).Scan(&legacy)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE finance_bookings
		SET named_stay_id = ?, occupancy_id = COALESCE(?, occupancy_id), updated_at = ?
		WHERE property_id = ? AND id = ?`, namedStayID, nullInt64Value(legacy), now, propertyID, bookingID)
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

func (s *Store) MarkNamedStayFinanceReviewForBooking(ctx context.Context, propertyID, bookingID int64, reason string) error {
	if bookingID <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE named_stays
		SET review_status = 'needs_review', review_reason = ?, updated_at = ?
		WHERE property_id = ?
		  AND id = (SELECT named_stay_id FROM finance_bookings WHERE property_id = ? AND id = ? AND named_stay_id IS NOT NULL)
		  AND status = 'active'`, strings.TrimSpace(reason), now, propertyID, propertyID, bookingID)
	return err
}

func (s *Store) OccupancyIDsWithPayoutData(ctx context.Context, propertyID int64, occupancyIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(occupancyIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(occupancyIDs))
	args := make([]interface{}, 0, len(occupancyIDs)+1)
	args = append(args, propertyID)
	for i, id := range occupancyIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT occupancy_id
		FROM finance_bookings
		WHERE property_id = ? AND occupancy_id IS NOT NULL AND occupancy_id IN (%s)`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return out, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Store) FindOccupancyForStayDates(ctx context.Context, propertyID int64, checkInDate, checkOutDate string, loc *time.Location) (*Occupancy, error) {
	if checkInDate == "" || checkOutDate == "" {
		return nil, nil
	}
	inDate, err := time.ParseInLocation("2006-01-02", checkInDate, loc)
	if err != nil {
		return nil, nil
	}
	outDate, err := time.ParseInLocation("2006-01-02", checkOutDate, loc)
	if err != nil {
		return nil, nil
	}
	windowStart := inDate.AddDate(0, 0, -3).UTC()
	windowEnd := outDate.AddDate(0, 0, 3).UTC()
	candidates, err := s.ListOccupanciesBetween(ctx, propertyID, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		o := &candidates[i]
		startLocal := o.StartAt.In(loc).Format("2006-01-02")
		endLocal := o.EndAt.In(loc).Format("2006-01-02")
		if startLocal == checkInDate && endLocal == checkOutDate {
			return o, nil
		}
	}
	return nil, nil
}

// legacyFindOrCreateOccupancyForPayoutStayDates exists only for rollback tests
// of the pre-PMS 21 compatibility path. Production import/rematch code must use
// FindNamedStayForFinanceStayDates and must never call this synthetic writer.
func (s *Store) legacyFindOrCreateOccupancyForPayoutStayDates(
	ctx context.Context,
	propertyID int64,
	referenceNumber, checkInDate, checkOutDate, guestName string,
	loc *time.Location,
) (*Occupancy, error) {
	return s.findOrCreateOccupancyForFinanceStayDates(ctx, propertyID, "booking_payout", referenceNumber, checkInDate, checkOutDate, guestName, loc)
}

// legacyFindOrCreateOccupancyForStatementStayDates is the statement equivalent
// of the rollback-only payout helper above.
func (s *Store) legacyFindOrCreateOccupancyForStatementStayDates(
	ctx context.Context,
	propertyID int64,
	referenceNumber, checkInDate, checkOutDate, guestName string,
	loc *time.Location,
) (*Occupancy, error) {
	return s.findOrCreateOccupancyForFinanceStayDates(ctx, propertyID, "booking_statement", referenceNumber, checkInDate, checkOutDate, guestName, loc)
}

func (s *Store) SupersedeGenericICSBlocksForFinanceStayDates(ctx context.Context, propertyID int64, checkInDate, checkOutDate string, loc *time.Location, keepOccupancyID int64) error {
	if s.OccupancyLegacyWriteDisabled {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}
	checkInDate = strings.TrimSpace(checkInDate)
	checkOutDate = strings.TrimSpace(checkOutDate)
	if checkInDate == "" || checkOutDate == "" {
		return nil
	}
	inDate, err := time.ParseInLocation("2006-01-02", checkInDate, loc)
	if err != nil {
		return nil
	}
	outDate, err := time.ParseInLocation("2006-01-02", checkOutDate, loc)
	if err != nil || !outDate.After(inDate) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET status = 'deleted_from_source', last_synced_at = ?
		WHERE property_id = ?
		  AND id != ?
		  AND source_type = 'booking_ics'
		  AND status IN ('active', 'updated')
		  AND (guest_display_name IS NULL OR TRIM(guest_display_name) = '')
		  AND LOWER(COALESCE(raw_summary, '')) LIKE '%closed%'
		  AND LOWER(COALESCE(raw_summary, '')) LIKE '%not available%'
		  AND start_at >= ?
		  AND end_at <= ?
		  AND NOT EXISTS (
		      SELECT 1
		      FROM nuki_access_codes nac
		      WHERE nac.property_id = occupancies.property_id
		        AND nac.occupancy_id = occupancies.id
		        AND nac.status = 'generated'
		  )`,
		now,
		propertyID,
		keepOccupancyID,
		inDate.UTC().Format(time.RFC3339),
		outDate.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) findOrCreateOccupancyForFinanceStayDates(
	ctx context.Context,
	propertyID int64,
	sourceType, referenceNumber, checkInDate, checkOutDate, guestName string,
	loc *time.Location,
) (*Occupancy, error) {
	if loc == nil {
		loc = time.UTC
	}
	checkInDate = strings.TrimSpace(checkInDate)
	checkOutDate = strings.TrimSpace(checkOutDate)
	if checkInDate == "" || checkOutDate == "" {
		return nil, nil
	}
	if occ, err := s.FindOccupancyForStayDates(ctx, propertyID, checkInDate, checkOutDate, loc); err != nil || occ != nil {
		return occ, err
	}
	if s.OccupancyLegacyWriteDisabled {
		return nil, nil
	}

	inDate, err := time.ParseInLocation("2006-01-02", checkInDate, loc)
	if err != nil {
		return nil, nil
	}
	outDate, err := time.ParseInLocation("2006-01-02", checkOutDate, loc)
	if err != nil {
		return nil, nil
	}
	if !outDate.After(inDate) {
		return nil, nil
	}

	ref := strings.TrimSpace(referenceNumber)
	sourceUID := financeOccupancyUID(sourceType, ref, checkInDate, checkOutDate)
	guest := strings.TrimSpace(guestName)
	summary := guest
	if summary == "" {
		if ref != "" {
			summary = financeOccupancySummary(sourceType, ref)
		} else {
			summary = financeOccupancySummary(sourceType, "") + " stay " + checkInDate + " - " + checkOutDate
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	contentHash := payoutOccupancyHash(sourceType+":"+ref, checkInDate, checkOutDate, guest)
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO occupancies (property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, NULL)
		ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			status = excluded.status,
			raw_summary = excluded.raw_summary,
			guest_display_name = COALESCE(excluded.guest_display_name, occupancies.guest_display_name),
			content_hash = excluded.content_hash,
			last_synced_at = excluded.last_synced_at`,
		propertyID,
		sourceType,
		sourceUID,
		inDate.UTC().Format(time.RFC3339),
		outDate.UTC().Format(time.RFC3339),
		summary,
		nullStringValue(sql.NullString{String: guest, Valid: guest != ""}),
		contentHash,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetOccupancyBySourceEventUID(ctx, propertyID, sourceUID)
}

func payoutOccupancyUID(referenceNumber, checkInDate, checkOutDate string) string {
	return financeOccupancyUID("booking_payout", referenceNumber, checkInDate, checkOutDate)
}

func financeOccupancyUID(sourceType, referenceNumber, checkInDate, checkOutDate string) string {
	prefix := "booking_payout"
	if sourceType == "booking_statement" {
		prefix = "booking_statement"
	}
	if referenceNumber != "" {
		return prefix + ":" + referenceNumber
	}
	return prefix + ":" + checkInDate + ":" + checkOutDate
}

func financeOccupancySummary(sourceType, referenceNumber string) string {
	label := "Booking.com payout"
	if sourceType == "booking_statement" {
		label = "Booking.com statement"
	}
	if strings.TrimSpace(referenceNumber) == "" {
		return label
	}
	return label + " " + strings.TrimSpace(referenceNumber)
}

func payoutOccupancyHash(referenceNumber, checkInDate, checkOutDate, guestName string) string {
	sum := sha1.Sum([]byte(referenceNumber + "|" + checkInDate + "|" + checkOutDate + "|" + guestName))
	return hex.EncodeToString(sum[:])
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
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ft.id, ft.property_id, ft.transaction_date, ft.direction, ft.amount_cents, ft.category_id,
			ft.note, ft.source_type, ft.source_reference_id, ft.is_auto_generated, ft.attachment_path, ft.created_at, ft.updated_at,
			fc.code, fc.title, COALESCE(fc.counts_toward_property_income, 0),
			COALESCE(CASE WHEN COALESCE(fbp.named_stay_id, fbp.occupancy_id) IS NOT NULL THEN 1 ELSE 0 END, 0)
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
			fbp.payment_service_fee_cents, fbp.net_cents, fbp.payout_date, fbp.transaction_id, fbp.occupancy_id, fbp.named_stay_id, fbp.raw_payout_row_json,
			fbp.outcome_override, fbp.outcome_override_marked_at, fbp.created_at, fbp.updated_at,
			(
				SELECT i.id FROM invoices i
				WHERE i.property_id = fbp.property_id AND i.finance_booking_payout_id = fbp.id
				LIMIT 1
			) AS linked_invoice_id,
			occ.source_event_uid, occ.start_at, occ.end_at, COALESCE(ns.display_name, occ.guest_display_name, occ.raw_summary),
			ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date,
			fbp.has_payout_data, fbp.has_statement_data
		FROM finance_bookings fbp
		LEFT JOIN occupancies occ
		  ON occ.id = fbp.occupancy_id
		 AND occ.property_id = fbp.property_id
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
			query += ` AND COALESCE(fbp.named_stay_id, fbp.occupancy_id) IS NOT NULL`
		} else {
			query += ` AND fbp.named_stay_id IS NULL AND fbp.occupancy_id IS NULL`
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
		var occStart, occEnd sql.NullString
		var hasPayout, hasStatement int
		if err := rows.Scan(
			&r.ID, &r.PropertyID, &r.ReferenceNumber, &r.PayoutID, &r.RowType, &r.CheckInDate, &r.CheckOutDate,
			&r.GuestName, &r.ReservationStatus, &r.Currency, &r.PaymentStatus, &r.AmountCents, &r.CommissionCents,
			&r.PaymentServiceFeeCents, &r.NetCents, &payoutDate, &r.TransactionID, &r.OccupancyID, &r.NamedStayID, &r.RawRowJSON,
			&r.OutcomeOverride, &outcomeMarkedAt, &created, &updated, &r.LinkedInvoiceID, &r.OccupancySourceEventUID, &occStart, &occEnd, &r.OccupancySummary,
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
		if occStart.Valid && occStart.String != "" {
			t, _ := time.Parse(time.RFC3339, occStart.String)
			r.OccupancyStartAt = sql.NullTime{Time: t, Valid: true}
		}
		if occEnd.Valid && occEnd.String != "" {
			t, _ := time.Parse(time.RFC3339, occEnd.String)
			r.OccupancyEndAt = sql.NullTime{Time: t, Valid: true}
		}
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
			net_cents, payout_date, transaction_id, occupancy_id, named_stay_id, raw_payout_row_json, created_at, updated_at
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
			&r.NetCents, &payoutDate, &r.TransactionID, &r.OccupancyID, &r.NamedStayID, &r.RawRowJSON, &created, &updated); err != nil {
			return nil, err
		}
		r.PayoutDate, _ = time.Parse(time.RFC3339, payoutDate)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}
