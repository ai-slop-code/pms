package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FinanceImport is one row in the finance_imports audit table — written
// once per upload (preview-then-commit happens server-side; only commits
// produce rows).
type FinanceImport struct {
	ID                        int64
	PropertyID                int64
	SourceType                string
	SourceChannel             string
	HotelID                   sql.NullString
	InvoiceNumber             sql.NullString
	PeriodStart               sql.NullString
	PeriodEnd                 sql.NullString
	UploadedByUserID          sql.NullInt64
	UploadedAt                time.Time
	FileSHA256                sql.NullString
	RowCountTotal             int
	RowCountInserted          int
	RowCountUpdated           int
	RowCountUnchanged         int
	RowCountSkippedOtherHotel int
	RowCountRejected          int
}

// FinanceBookingMerge is one row of the merge audit log — describes
// exactly which fields a single import altered on a single booking row.
type FinanceBookingMerge struct {
	ID                int64
	BookingID         int64
	ImportID          int64
	SourceType        string
	ChangedFieldsJSON sql.NullString
	OccurredAt        time.Time
}

// CreateFinanceImport inserts the audit row and returns its ID.
func (s *Store) CreateFinanceImport(ctx context.Context, imp *FinanceImport) (int64, error) {
	if imp.UploadedAt.IsZero() {
		imp.UploadedAt = time.Now().UTC()
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO finance_imports (
			property_id, source_type, source_channel, hotel_id, invoice_number, period_start, period_end,
			uploaded_by_user_id, uploaded_at, file_sha256,
			row_count_total, row_count_inserted, row_count_updated, row_count_unchanged,
			row_count_skipped_other_hotel, row_count_rejected
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		imp.PropertyID, imp.SourceType, imp.SourceChannel,
		nullStringValue(imp.HotelID), nullStringValue(imp.InvoiceNumber),
		nullStringValue(imp.PeriodStart), nullStringValue(imp.PeriodEnd),
		nullInt64Value(imp.UploadedByUserID), imp.UploadedAt.UTC().Format(time.RFC3339),
		nullStringValue(imp.FileSHA256),
		imp.RowCountTotal, imp.RowCountInserted, imp.RowCountUpdated, imp.RowCountUnchanged,
		imp.RowCountSkippedOtherHotel, imp.RowCountRejected,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListFinanceImports returns recent uploads for a property, newest first.
func (s *Store) ListFinanceImports(ctx context.Context, propertyID int64, limit int) ([]FinanceImport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, source_type, source_channel, hotel_id, invoice_number,
			period_start, period_end, uploaded_by_user_id, uploaded_at, file_sha256,
			row_count_total, row_count_inserted, row_count_updated, row_count_unchanged,
			row_count_skipped_other_hotel, row_count_rejected
		FROM finance_imports
		WHERE property_id = ?
		ORDER BY uploaded_at DESC, id DESC
		LIMIT ?`, propertyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FinanceImport, 0)
	for rows.Next() {
		var imp FinanceImport
		var uploadedAt string
		if err := rows.Scan(
			&imp.ID, &imp.PropertyID, &imp.SourceType, &imp.SourceChannel,
			&imp.HotelID, &imp.InvoiceNumber, &imp.PeriodStart, &imp.PeriodEnd,
			&imp.UploadedByUserID, &uploadedAt, &imp.FileSHA256,
			&imp.RowCountTotal, &imp.RowCountInserted, &imp.RowCountUpdated,
			&imp.RowCountUnchanged, &imp.RowCountSkippedOtherHotel, &imp.RowCountRejected,
		); err != nil {
			return nil, err
		}
		imp.UploadedAt, _ = time.Parse(time.RFC3339, uploadedAt)
		out = append(out, imp)
	}
	return out, rows.Err()
}

// LastFinanceImportBySHA returns the most recent import whose file_sha256
// matches; used as a UI hint when re-uploading the same file. Returns nil
// when no such import exists.
func (s *Store) LastFinanceImportBySHA(ctx context.Context, propertyID int64, sha string) (*FinanceImport, error) {
	if sha == "" {
		return nil, nil
	}
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, source_type, source_channel, hotel_id, invoice_number,
			period_start, period_end, uploaded_by_user_id, uploaded_at, file_sha256,
			row_count_total, row_count_inserted, row_count_updated, row_count_unchanged,
			row_count_skipped_other_hotel, row_count_rejected
		FROM finance_imports
		WHERE property_id = ? AND file_sha256 = ?
		ORDER BY uploaded_at DESC, id DESC
		LIMIT 1`, propertyID, sha)
	var imp FinanceImport
	var uploadedAt string
	err := row.Scan(
		&imp.ID, &imp.PropertyID, &imp.SourceType, &imp.SourceChannel,
		&imp.HotelID, &imp.InvoiceNumber, &imp.PeriodStart, &imp.PeriodEnd,
		&imp.UploadedByUserID, &uploadedAt, &imp.FileSHA256,
		&imp.RowCountTotal, &imp.RowCountInserted, &imp.RowCountUpdated,
		&imp.RowCountUnchanged, &imp.RowCountSkippedOtherHotel, &imp.RowCountRejected,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	imp.UploadedAt, _ = time.Parse(time.RFC3339, uploadedAt)
	return &imp, nil
}

// CreateFinanceBookingMerge appends a merge-audit row.
func (s *Store) CreateFinanceBookingMerge(ctx context.Context, m *FinanceBookingMerge) error {
	if m.OccurredAt.IsZero() {
		m.OccurredAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO finance_booking_merges (
			booking_id, import_id, source_type, changed_fields_json, occurred_at
		) VALUES (?, ?, ?, ?, ?)`,
		m.BookingID, m.ImportID, m.SourceType,
		nullStringValue(m.ChangedFieldsJSON),
		m.OccurredAt.UTC().Format(time.RFC3339),
	)
	return err
}

// ListFinanceBookingMergesByImport returns the merge log for one import,
// keyed by booking_id.
func (s *Store) ListFinanceBookingMergesByImport(ctx context.Context, importID int64) ([]FinanceBookingMerge, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, booking_id, import_id, source_type, changed_fields_json, occurred_at
		FROM finance_booking_merges
		WHERE import_id = ?
		ORDER BY id ASC`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FinanceBookingMerge, 0)
	for rows.Next() {
		var m FinanceBookingMerge
		var occurred string
		if err := rows.Scan(&m.ID, &m.BookingID, &m.ImportID, &m.SourceType, &m.ChangedFieldsJSON, &occurred); err != nil {
			return nil, err
		}
		m.OccurredAt, _ = time.Parse(time.RFC3339, occurred)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetPropertyBookingHotelIDIfEmpty captures the observed Hotel id from the
// first statement upload. Subsequent uploads with a different value are
// reported as multi-hotel skips by the caller (we don't overwrite).
func (s *Store) SetPropertyBookingHotelIDIfEmpty(ctx context.Context, propertyID int64, hotelID string) (string, error) {
	hotelID = strings.TrimSpace(hotelID)
	if hotelID == "" {
		return "", nil
	}
	var current sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT booking_hotel_id FROM properties WHERE id = ?`, propertyID).Scan(&current); err != nil {
		return "", err
	}
	if current.Valid && strings.TrimSpace(current.String) != "" {
		return current.String, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx, `UPDATE properties SET booking_hotel_id = ?, updated_at = ? WHERE id = ?`, hotelID, now, propertyID); err != nil {
		return "", err
	}
	return hotelID, nil
}

// GetPropertyBookingHotelID reads the captured Booking.com hotel id.
func (s *Store) GetPropertyBookingHotelID(ctx context.Context, propertyID int64) (string, error) {
	var v sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT booking_hotel_id FROM properties WHERE id = ?`, propertyID).Scan(&v); err != nil {
		return "", err
	}
	if !v.Valid {
		return "", nil
	}
	return strings.TrimSpace(v.String), nil
}

// CountFinanceBookingsByReferences returns the count of finance_bookings
// rows matching any of the given references for a property. Used by the
// preview path to seed insert-vs-update counts.
func (s *Store) CountFinanceBookingsByReferences(ctx context.Context, propertyID int64, refs []string) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	ph := make([]string, len(refs))
	args := make([]interface{}, 0, len(refs)+1)
	args = append(args, propertyID)
	for i, r := range refs {
		ph[i] = "?"
		args = append(args, r)
	}
	q := fmt.Sprintf(`SELECT COUNT(*) FROM finance_bookings WHERE property_id = ? AND reference_number IN (%s)`, strings.Join(ph, ","))
	var n int
	if err := s.DB.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
