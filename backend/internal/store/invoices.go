package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Invoice struct {
	ID                   int64
	PropertyID           int64
	OccupancyID          sql.NullInt64
	InvoiceNumber        string
	SequenceYear         int
	SequenceValue        int
	Language             string
	IssueDate            time.Time
	TaxableSupplyDate    time.Time
	DueDate              time.Time
	StayStartDate        time.Time
	StayEndDate          time.Time
	SupplierSnapshotJSON string
	CustomerSnapshotJSON string
	AmountTotalCents     int
	Currency             string
	PaymentStatus        string
	PaymentNote          string
	FinanceBookingPayoutID sql.NullInt64
	Version              int
	CreatedBy            sql.NullInt64
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LatestFilePath       sql.NullString
	LatestFileSizeBytes  sql.NullInt64
	LatestFileCreatedAt  sql.NullTime
}

type InvoiceFile struct {
	ID            int64
	InvoiceID     int64
	Version       int
	FilePath      string
	FileSizeBytes int64
	CreatedAt     time.Time
}

func SanitizeInvoiceCode(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func sanitizeInvoiceCodeFromNull(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return SanitizeInvoiceCode(ns.String)
}

func FormatInvoiceNumber(code string, propertyID int64, year, sequence int) string {
	seqStr := fmt.Sprintf("%04d", sequence)
	if c := strings.TrimSpace(code); c != "" {
		return fmt.Sprintf("%s/%d/%s", c, year, seqStr)
	}
	return fmt.Sprintf("P%03d/%d/%s", propertyID, year, seqStr)
}

func (s *Store) PreviewNextInvoiceNumber(ctx context.Context, propertyID int64, year int) (string, int, error) {
	prop, err := s.GetProperty(ctx, propertyID)
	if err != nil {
		return "", 0, err
	}
	code := sanitizeInvoiceCodeFromNull(prop.InvoiceCode)
	var current int
	err = s.DB.QueryRowContext(ctx,
		`SELECT current_value FROM invoice_sequences WHERE property_id = ? AND sequence_year = ?`,
		propertyID, year,
	).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return "", 0, err
	}
	next := current + 1
	return FormatInvoiceNumber(code, propertyID, year, next), next, nil
}

func (s *Store) ListInvoices(ctx context.Context, propertyID int64) ([]Invoice, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT i.id, i.property_id, i.occupancy_id, i.finance_booking_payout_id, i.invoice_number, i.sequence_year, i.sequence_value,
			i.language, i.issue_date, i.taxable_supply_date, i.due_date, i.stay_start_date, i.stay_end_date,
			i.supplier_snapshot_json, i.customer_snapshot_json, i.amount_total_cents, i.currency, i.payment_status,
			i.payment_note, i.version, i.created_by, i.created_at, i.updated_at,
			(
				SELECT file_path
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_path,
			(
				SELECT file_size_bytes
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_size_bytes,
			(
				SELECT created_at
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_created_at
		FROM invoices i
		WHERE i.property_id = ?
		ORDER BY i.issue_date DESC, i.id DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvoicesRows(rows)
}

func (s *Store) GetInvoiceByID(ctx context.Context, propertyID, invoiceID int64) (*Invoice, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT i.id, i.property_id, i.occupancy_id, i.finance_booking_payout_id, i.invoice_number, i.sequence_year, i.sequence_value,
			i.language, i.issue_date, i.taxable_supply_date, i.due_date, i.stay_start_date, i.stay_end_date,
			i.supplier_snapshot_json, i.customer_snapshot_json, i.amount_total_cents, i.currency, i.payment_status,
			i.payment_note, i.version, i.created_by, i.created_at, i.updated_at,
			(
				SELECT file_path
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_path,
			(
				SELECT file_size_bytes
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_size_bytes,
			(
				SELECT created_at
				FROM invoice_files f
				WHERE f.invoice_id = i.id
				ORDER BY f.version DESC
				LIMIT 1
			) AS latest_file_created_at
		FROM invoices i
		WHERE i.property_id = ? AND i.id = ?`, propertyID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanInvoicesRows(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

func (s *Store) CreateInvoice(ctx context.Context, row *Invoice) (*Invoice, error) {
	if row == nil {
		return nil, fmt.Errorf("invoice is required")
	}
	year := row.IssueDate.UTC().Year()
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	current, err := currentInvoiceSequenceTx(ctx, tx, row.PropertyID, year)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	next := current + 1
	row.SequenceYear = year
	row.SequenceValue = next
	var invCode sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT invoice_code FROM properties WHERE id = ?`, row.PropertyID).Scan(&invCode); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	row.InvoiceNumber = FormatInvoiceNumber(sanitizeInvoiceCodeFromNull(invCode), row.PropertyID, year, next)
	if row.Currency == "" {
		row.Currency = "EUR"
	}
	if row.PaymentStatus == "" {
		row.PaymentStatus = "paid"
	}
	if row.PaymentNote == "" {
		row.PaymentNote = "Already paid via Booking.com."
	}
	row.Version = 1
	res, err := tx.ExecContext(ctx, `
		INSERT INTO invoices (
			property_id, occupancy_id, finance_booking_payout_id, invoice_number, sequence_year, sequence_value, language,
			issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date,
			supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency,
			payment_status, payment_note, version, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.PropertyID, nullInt64Value(row.OccupancyID), nullInt64Value(row.FinanceBookingPayoutID), row.InvoiceNumber, row.SequenceYear, row.SequenceValue, row.Language,
		row.IssueDate.UTC().Format(time.RFC3339), row.TaxableSupplyDate.UTC().Format(time.RFC3339), row.DueDate.UTC().Format(time.RFC3339),
		row.StayStartDate.UTC().Format(time.RFC3339), row.StayEndDate.UTC().Format(time.RFC3339),
		row.SupplierSnapshotJSON, row.CustomerSnapshotJSON, row.AmountTotalCents, row.Currency,
		row.PaymentStatus, row.PaymentNote, row.Version, nullInt64Value(row.CreatedBy), now, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO invoice_sequences (property_id, sequence_year, current_value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(property_id, sequence_year)
		DO UPDATE SET current_value = excluded.current_value, updated_at = excluded.updated_at`,
		row.PropertyID, row.SequenceYear, row.SequenceValue, now, now); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetInvoiceByID(ctx, row.PropertyID, id)
}

func (s *Store) UpdateInvoice(ctx context.Context, row *Invoice) (*Invoice, error) {
	if row == nil {
		return nil, fmt.Errorf("invoice is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE invoices
		SET occupancy_id = ?, finance_booking_payout_id = ?, language = ?, issue_date = ?, taxable_supply_date = ?, due_date = ?,
			stay_start_date = ?, stay_end_date = ?, supplier_snapshot_json = ?, customer_snapshot_json = ?,
			amount_total_cents = ?, currency = ?, payment_status = ?, payment_note = ?, updated_at = ?
		WHERE id = ? AND property_id = ?`,
		nullInt64Value(row.OccupancyID), nullInt64Value(row.FinanceBookingPayoutID), row.Language, row.IssueDate.UTC().Format(time.RFC3339),
		row.TaxableSupplyDate.UTC().Format(time.RFC3339), row.DueDate.UTC().Format(time.RFC3339),
		row.StayStartDate.UTC().Format(time.RFC3339), row.StayEndDate.UTC().Format(time.RFC3339),
		row.SupplierSnapshotJSON, row.CustomerSnapshotJSON, row.AmountTotalCents, row.Currency,
		row.PaymentStatus, row.PaymentNote, now, row.ID, row.PropertyID)
	if err != nil {
		return nil, err
	}
	return s.GetInvoiceByID(ctx, row.PropertyID, row.ID)
}

func (s *Store) AttachInvoiceFile(ctx context.Context, propertyID, invoiceID int64, version int, filePath string, fileSizeBytes int64) (*InvoiceFile, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var existingPropertyID int64
	if err := tx.QueryRowContext(ctx, `SELECT property_id FROM invoices WHERE id = ?`, invoiceID).Scan(&existingPropertyID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if existingPropertyID != propertyID {
		_ = tx.Rollback()
		return nil, sql.ErrNoRows
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO invoice_files (invoice_id, version, file_path, file_size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		invoiceID, version, filePath, fileSizeBytes, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE invoices
		SET version = ?, updated_at = ?
		WHERE id = ? AND property_id = ?`,
		version, now, invoiceID, propertyID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetInvoiceFileByID(ctx, invoiceID, id)
}

func (s *Store) ListInvoiceFiles(ctx context.Context, propertyID, invoiceID int64) ([]InvoiceFile, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id, f.invoice_id, f.version, f.file_path, f.file_size_bytes, f.created_at
		FROM invoice_files f
		INNER JOIN invoices i ON i.id = f.invoice_id
		WHERE i.property_id = ? AND f.invoice_id = ?
		ORDER BY f.version DESC, f.id DESC`, propertyID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvoiceFilesRows(rows)
}

func (s *Store) GetLatestInvoiceFile(ctx context.Context, propertyID, invoiceID int64) (*InvoiceFile, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id, f.invoice_id, f.version, f.file_path, f.file_size_bytes, f.created_at
		FROM invoice_files f
		INNER JOIN invoices i ON i.id = f.invoice_id
		WHERE i.property_id = ? AND f.invoice_id = ?
		ORDER BY f.version DESC, f.id DESC
		LIMIT 1`, propertyID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files, err := scanInvoiceFilesRows(rows)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, sql.ErrNoRows
	}
	return &files[0], nil
}

func (s *Store) GetInvoiceFileByID(ctx context.Context, invoiceID, fileID int64) (*InvoiceFile, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, invoice_id, version, file_path, file_size_bytes, created_at
		FROM invoice_files
		WHERE invoice_id = ? AND id = ?`, invoiceID, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files, err := scanInvoiceFilesRows(rows)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, sql.ErrNoRows
	}
	return &files[0], nil
}

func currentInvoiceSequenceTx(ctx context.Context, tx *sql.Tx, propertyID int64, year int) (int, error) {
	var current int
	err := tx.QueryRowContext(ctx,
		`SELECT current_value FROM invoice_sequences WHERE property_id = ? AND sequence_year = ?`,
		propertyID, year,
	).Scan(&current)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return current, err
}

func scanInvoicesRows(rows *sql.Rows) ([]Invoice, error) {
	out := make([]Invoice, 0)
	for rows.Next() {
		var row Invoice
		var issueDate, taxableSupplyDate, dueDate, stayStartDate, stayEndDate string
		var createdAt, updatedAt string
		var latestFileCreatedAt sql.NullString
		if err := rows.Scan(
			&row.ID, &row.PropertyID, &row.OccupancyID, &row.FinanceBookingPayoutID, &row.InvoiceNumber, &row.SequenceYear, &row.SequenceValue,
			&row.Language, &issueDate, &taxableSupplyDate, &dueDate, &stayStartDate, &stayEndDate,
			&row.SupplierSnapshotJSON, &row.CustomerSnapshotJSON, &row.AmountTotalCents, &row.Currency, &row.PaymentStatus,
			&row.PaymentNote, &row.Version, &row.CreatedBy, &createdAt, &updatedAt,
			&row.LatestFilePath, &row.LatestFileSizeBytes, &latestFileCreatedAt,
		); err != nil {
			return nil, err
		}
		row.IssueDate, _ = time.Parse(time.RFC3339, issueDate)
		row.TaxableSupplyDate, _ = time.Parse(time.RFC3339, taxableSupplyDate)
		row.DueDate, _ = time.Parse(time.RFC3339, dueDate)
		row.StayStartDate, _ = time.Parse(time.RFC3339, stayStartDate)
		row.StayEndDate, _ = time.Parse(time.RFC3339, stayEndDate)
		row.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		row.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if latestFileCreatedAt.Valid {
			if parsed, err := time.Parse(time.RFC3339, latestFileCreatedAt.String); err == nil {
				row.LatestFileCreatedAt = sql.NullTime{Time: parsed, Valid: true}
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanInvoiceFilesRows(rows *sql.Rows) ([]InvoiceFile, error) {
	out := make([]InvoiceFile, 0)
	for rows.Next() {
		var row InvoiceFile
		var createdAt string
		if err := rows.Scan(&row.ID, &row.InvoiceID, &row.Version, &row.FilePath, &row.FileSizeBytes, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}
