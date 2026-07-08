package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pms/backend/internal/finance/statements"
)

// FinanceBookingByReference returns the canonical merged-booking shape
// (the parser's CanonicalBooking) for a given (property, channel, ref).
// Returns (nil, nil) when no row exists.
//
// The struct is the parser-layer type because the merger consumes/produces
// it; this keeps the store→merger boundary thin (no extra DTO).
func (s *Store) FinanceBookingByReference(ctx context.Context, propertyID int64, channel, reference string) (*statements.CanonicalBooking, int64, error) {
	if channel == "" {
		channel = "booking_com"
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, source_channel, reference_number,
			has_payout_data, has_statement_data,
			booked_on, check_in_date, check_out_date,
			guest_name, booker_name, guest_request,
			reservation_status, payment_status, currency,
			amount_cents, original_amount_cents, commission_cents, commission_pct,
			payment_service_fee_cents, net_cents,
			persons, rooms, room_nights,
			payout_date, payout_id, row_type,
			invoice_number, hotel_id, property_label, country,
			raw_payout_row_json, raw_statement_row_json,
			status
		FROM finance_bookings
		WHERE property_id = ? AND source_channel = ? AND reference_number = ?`,
		propertyID, channel, reference)
	var (
		id                  int64
		sourceChannel       string
		ref                 string
		hasPayout           int
		hasStatement        int
		bookedOn            sql.NullString
		checkIn             sql.NullString
		checkOut            sql.NullString
		guestName           sql.NullString
		bookerName          sql.NullString
		guestRequest        sql.NullString
		reservationStatus   sql.NullString
		paymentStatus       sql.NullString
		currency            sql.NullString
		amountCents         sql.NullInt64
		originalAmountCents sql.NullInt64
		commissionCents     sql.NullInt64
		commissionPct       sql.NullFloat64
		paymentFeeCents     sql.NullInt64
		netCents            sql.NullInt64
		persons             sql.NullInt64
		rooms               sql.NullInt64
		roomNights          sql.NullInt64
		payoutDate          sql.NullString
		payoutID            sql.NullString
		rowType             sql.NullString
		invoiceNumber       sql.NullString
		hotelID             sql.NullString
		propertyLabel       sql.NullString
		country             sql.NullString
		rawPayout           sql.NullString
		rawStatement        sql.NullString
		statusValue         sql.NullString
	)
	err := row.Scan(
		&id, &sourceChannel, &ref,
		&hasPayout, &hasStatement,
		&bookedOn, &checkIn, &checkOut,
		&guestName, &bookerName, &guestRequest,
		&reservationStatus, &paymentStatus, &currency,
		&amountCents, &originalAmountCents, &commissionCents, &commissionPct,
		&paymentFeeCents, &netCents,
		&persons, &rooms, &roomNights,
		&payoutDate, &payoutID, &rowType,
		&invoiceNumber, &hotelID, &propertyLabel, &country,
		&rawPayout, &rawStatement,
		&statusValue,
	)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	cb := &statements.CanonicalBooking{
		ReferenceNumber:     ref,
		SourceChannel:       sourceChannel,
		HasPayoutData:       hasPayout != 0,
		HasStatementData:    hasStatement != 0,
		BookedOn:            ptrFromNullString(bookedOn),
		CheckInDate:         ptrFromNullString(checkIn),
		CheckOutDate:        ptrFromNullString(checkOut),
		GuestName:           ptrFromNullString(guestName),
		BookerName:          ptrFromNullString(bookerName),
		GuestRequest:        ptrFromNullString(guestRequest),
		Status:              ptrFromNullString(statusValue),
		ReservationStatus:   ptrFromNullString(reservationStatus),
		PaymentStatus:       ptrFromNullString(paymentStatus),
		Currency:            ptrFromNullString(currency),
		AmountCents:         ptrFromNullInt(amountCents),
		OriginalAmountCents: ptrFromNullInt(originalAmountCents),
		CommissionCents:     ptrFromNullInt(commissionCents),
		PaymentFeeCents:     ptrFromNullInt(paymentFeeCents),
		NetCents:            ptrFromNullInt(netCents),
		Persons:             ptrFromNullInt(persons),
		Rooms:               ptrFromNullInt(rooms),
		RoomNights:          ptrFromNullInt(roomNights),
		PayoutDate:          ptrFromNullString(payoutDate),
		PayoutID:            ptrFromNullString(payoutID),
		RowType:             ptrFromNullString(rowType),
		InvoiceNumber:       ptrFromNullString(invoiceNumber),
		HotelID:             ptrFromNullString(hotelID),
		PropertyLabel:       ptrFromNullString(propertyLabel),
		Country:             ptrFromNullString(country),
		RawPayoutRowJSON:    ptrFromNullString(rawPayout),
		RawStatementRowJSON: ptrFromNullString(rawStatement),
	}
	if commissionPct.Valid {
		v := commissionPct.Float64
		cb.CommissionPct = &v
	}
	// Status (separate column) — fall back to reservation_status uppercased
	// when canonical status not stored. Reservation_status fills the same
	// canonical role for payout-only rows.
	if cb.Status == nil && cb.ReservationStatus != nil {
		v := strings.ToUpper(*cb.ReservationStatus)
		cb.Status = &v
	}
	return cb, id, nil
}

// UpsertFinanceBookingFromCanonical inserts a new finance_bookings row
// (or updates the existing one identified by id when id > 0) using the
// merger's CanonicalBooking shape. Returns the row id.
func (s *Store) UpsertFinanceBookingFromCanonical(ctx context.Context, propertyID int64, existingID int64, b statements.CanonicalBooking) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if existingID > 0 {
		_, err := s.DB.ExecContext(ctx, `
			UPDATE finance_bookings SET
				has_payout_data = ?,
				has_statement_data = ?,
				booked_on = ?,
				check_in_date = ?,
				check_out_date = ?,
				guest_name = ?,
				booker_name = ?,
				guest_request = ?,
				reservation_status = ?,
				payment_status = ?,
				currency = ?,
				amount_cents = ?,
				original_amount_cents = ?,
				commission_cents = ?,
				commission_pct = ?,
				payment_service_fee_cents = ?,
				net_cents = COALESCE(?, net_cents),
				persons = ?,
				rooms = ?,
				room_nights = ?,
				payout_date = COALESCE(?, payout_date),
				payout_id = ?,
				row_type = ?,
				invoice_number = ?,
				hotel_id = ?,
				property_label = ?,
				country = ?,
				raw_payout_row_json = COALESCE(?, raw_payout_row_json),
				raw_statement_row_json = COALESCE(?, raw_statement_row_json),
				status = ?,
				updated_at = ?
			WHERE id = ?`,
			boolToInt(b.HasPayoutData), boolToInt(b.HasStatementData),
			ptrToNullString(b.BookedOn),
			ptrToNullString(b.CheckInDate), ptrToNullString(b.CheckOutDate),
			ptrToNullString(b.GuestName), ptrToNullString(b.BookerName), ptrToNullString(b.GuestRequest),
			ptrToNullString(b.ReservationStatus), ptrToNullString(b.PaymentStatus), ptrToNullString(b.Currency),
			ptrToNullInt(b.AmountCents), ptrToNullInt(b.OriginalAmountCents),
			ptrToNullInt(b.CommissionCents), ptrToNullFloat(b.CommissionPct),
			ptrToNullInt(b.PaymentFeeCents), ptrToNullInt(b.NetCents),
			ptrToNullInt(b.Persons), ptrToNullInt(b.Rooms), ptrToNullInt(b.RoomNights),
			ptrToNullString(b.PayoutDate), ptrToNullString(b.PayoutID), ptrToNullString(b.RowType),
			ptrToNullString(b.InvoiceNumber), ptrToNullString(b.HotelID),
			ptrToNullString(b.PropertyLabel), ptrToNullString(b.Country),
			ptrToNullString(b.RawPayoutRowJSON), ptrToNullString(b.RawStatementRowJSON),
			ptrToNullString(canonicalStatusForDB(b)),
			now, existingID,
		)
		return existingID, err
	}
	netCents := 0
	if b.NetCents != nil {
		netCents = *b.NetCents
	}
	payoutDate := ""
	if b.PayoutDate != nil {
		payoutDate = *b.PayoutDate
	} else {
		// payout_date is NOT NULL on the table (legacy invariant). For
		// statement-only rows that have not yet been paid out, use the
		// booked_on timestamp as a placeholder so the cash-basis sort
		// order remains stable; analytics filter by has_payout_data.
		if b.BookedOn != nil {
			payoutDate = *b.BookedOn
		} else {
			payoutDate = now
		}
	}
	channel := b.SourceChannel
	if channel == "" {
		channel = "booking_com"
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO finance_bookings (
			property_id, reference_number, source_channel,
			has_payout_data, has_statement_data,
			booked_on, check_in_date, check_out_date,
			guest_name, booker_name, guest_request,
			reservation_status, payment_status, currency,
			amount_cents, original_amount_cents, commission_cents, commission_pct,
			payment_service_fee_cents, net_cents,
			persons, rooms, room_nights,
			payout_date, payout_id, row_type,
			invoice_number, hotel_id, property_label, country,
			raw_payout_row_json, raw_statement_row_json,
			status,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		propertyID, b.ReferenceNumber, channel,
		boolToInt(b.HasPayoutData), boolToInt(b.HasStatementData),
		ptrToNullString(b.BookedOn),
		ptrToNullString(b.CheckInDate), ptrToNullString(b.CheckOutDate),
		ptrToNullString(b.GuestName), ptrToNullString(b.BookerName), ptrToNullString(b.GuestRequest),
		ptrToNullString(b.ReservationStatus), ptrToNullString(b.PaymentStatus), ptrToNullString(b.Currency),
		ptrToNullInt(b.AmountCents), ptrToNullInt(b.OriginalAmountCents),
		ptrToNullInt(b.CommissionCents), ptrToNullFloat(b.CommissionPct),
		ptrToNullInt(b.PaymentFeeCents), netCents,
		ptrToNullInt(b.Persons), ptrToNullInt(b.Rooms), ptrToNullInt(b.RoomNights),
		payoutDate, ptrToNullString(b.PayoutID), ptrToNullString(b.RowType),
		ptrToNullString(b.InvoiceNumber), ptrToNullString(b.HotelID),
		ptrToNullString(b.PropertyLabel), ptrToNullString(b.Country),
		ptrToNullString(b.RawPayoutRowJSON), ptrToNullString(b.RawStatementRowJSON),
		ptrToNullString(canonicalStatusForDB(b)),
		now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CancelOccupancyForBooking marks the occupancy linked to a finance
// booking as cancelled (status='cancelled'). It is a no-op when no
// occupancy is linked. Cancelled rows are kept (never deleted) so
// historical analytics remain queryable.
func (s *Store) CancelOccupancyForBooking(ctx context.Context, bookingID int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		   SET status = 'cancelled',
		       last_synced_at = ?
		 WHERE finance_booking_id = ? AND status != 'cancelled'`,
		now, bookingID,
	)
	if err != nil {
		return 0, err
	}
	if res == nil {
		return 0, nil
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// LinkBookingToOccupancy wires a finance_bookings row to an occupancy
// in both directions: finance_bookings.occupancy_id and
// occupancies.finance_booking_id. No-op when ids are zero.
func (s *Store) LinkBookingToOccupancy(ctx context.Context, propertyID int64, referenceNumber string, occupancyID, bookingID int64) error {
	if bookingID <= 0 || occupancyID <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE finance_bookings
		   SET occupancy_id = ?, updated_at = ?
		 WHERE id = ? AND (occupancy_id IS NULL OR occupancy_id != ?)`,
		occupancyID, now, bookingID, occupancyID); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		   SET finance_booking_id = ?, last_synced_at = ?
		 WHERE id = ? AND (finance_booking_id IS NULL OR finance_booking_id != ?)`,
		bookingID, now, occupancyID, bookingID); err != nil {
		return err
	}
	return nil
}

// UpsertBookingFinanceTransaction creates or updates the cash-basis
// finance_transactions row for a booking payout (source_type =
// "booking_payout", source_reference_id = reference). It also writes
// the resulting transaction_id back onto finance_bookings.id.
func (s *Store) UpsertBookingFinanceTransaction(ctx context.Context, propertyID, bookingID int64, reference string, netCents int, payoutDate time.Time, categoryID int64, payoutID string) error {
	if bookingID <= 0 || netCents == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	dir := "incoming"
	amount := netCents
	if amount < 0 {
		dir = "outgoing"
		amount = -amount
	}
	note := "Booking.com payout"
	if payoutID != "" {
		note = note + " (" + payoutID + ")"
	}
	existing, err := s.FinanceTransactionBySourceReference(ctx, propertyID, "booking_payout", reference)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var txID int64
	if existing != nil {
		txID = existing.ID
		if _, err := s.DB.ExecContext(ctx, `
			UPDATE finance_transactions
			   SET transaction_date = ?, direction = ?, amount_cents = ?, category_id = ?,
			       note = ?, is_auto_generated = 1, updated_at = ?
			 WHERE id = ?`,
			payoutDate.UTC().Format(time.RFC3339), dir, amount, categoryID, note, now, txID); err != nil {
			return err
		}
	} else {
		res, err := s.DB.ExecContext(ctx, `
			INSERT INTO finance_transactions (
				property_id, transaction_date, direction, amount_cents, category_id, note,
				source_type, source_reference_id, is_auto_generated, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			propertyID, payoutDate.UTC().Format(time.RFC3339), dir, amount, categoryID, note,
			"booking_payout", reference, now, now)
		if err != nil {
			return err
		}
		txID, err = res.LastInsertId()
		if err != nil {
			return err
		}
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE finance_bookings SET transaction_id = ?, updated_at = ? WHERE id = ?`,
		txID, now, bookingID)
	return err
}

// UpdateFinanceImportCounts overwrites the row-count columns on an
// existing finance_imports row.
func (s *Store) UpdateFinanceImportCounts(ctx context.Context, importID int64, imp *FinanceImport) error {
	if importID <= 0 || imp == nil {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE finance_imports SET
			row_count_total = ?,
			row_count_inserted = ?,
			row_count_updated = ?,
			row_count_unchanged = ?,
			row_count_skipped_other_hotel = ?,
			row_count_rejected = ?
		 WHERE id = ?`,
		imp.RowCountTotal, imp.RowCountInserted, imp.RowCountUpdated,
		imp.RowCountUnchanged, imp.RowCountSkippedOtherHotel, imp.RowCountRejected,
		importID)
	return err
}

// LinkOccupancyToBooking sets occupancies.finance_booking_id when the
// caller has resolved a payout row to an occupancy.
func (s *Store) LinkOccupancyToBooking(ctx context.Context, occupancyID, bookingID int64) error {
	if occupancyID <= 0 || bookingID <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		   SET finance_booking_id = ?, last_synced_at = ?
		 WHERE id = ? AND (finance_booking_id IS NULL OR finance_booking_id != ?)`,
		bookingID, now, occupancyID, bookingID)
	return err
}

// ---------------- helpers ----------------

func ptrFromNullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func ptrFromNullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	x := int(v.Int64)
	return &x
}

func ptrToNullString(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func ptrToNullInt(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func ptrToNullFloat(p *float64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// canonicalStatusForDB picks the canonical status to persist in the
// finance_bookings.status column. Statement rows fill b.Status with the
// raw "Status" CSV value ("OK", "Cancelled", ...). Payout-only rows
// fall back to the lower-cased reservation_status. Returns nil when
// neither is populated; otherwise an upper-cased trimmed value.
func canonicalStatusForDB(b statements.CanonicalBooking) *string {
	pick := func(p *string) (string, bool) {
		if p == nil {
			return "", false
		}
		v := strings.TrimSpace(*p)
		if v == "" {
			return "", false
		}
		return strings.ToUpper(v), true
	}
	if v, ok := pick(b.Status); ok {
		return &v
	}
	if v, ok := pick(b.ReservationStatus); ok {
		return &v
	}
	return nil
}
