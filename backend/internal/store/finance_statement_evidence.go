package store

import (
	"context"
	"database/sql"
	"time"

	"pms/backend/internal/finance/statements"
)

type FinanceStatementEvidence struct {
	ID                  int64
	PropertyID          int64
	SourceChannel       string
	ReferenceNumber     string
	HotelID             sql.NullString
	Status              string
	BookedOn            string
	CheckInDate         string
	CheckOutDate        string
	RawStatementRowJSON string
	LastImportID        int64
	SourceLine          int
}

func (s *Store) UpsertFinanceStatementEvidence(ctx context.Context, propertyID, importID int64, row statements.Row) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := dbForContext(ctx, s.DB).ExecContext(ctx, `
		INSERT INTO finance_statement_evidence
		(property_id, source_channel, reference_number, hotel_id, status, booked_on, check_in_date, check_out_date,
		 raw_statement_row_json, last_import_id, source_line, created_at, updated_at)
		VALUES (?, 'booking_com', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_channel, reference_number) DO UPDATE SET
		 hotel_id=excluded.hotel_id, status=excluded.status, booked_on=excluded.booked_on,
		 check_in_date=excluded.check_in_date, check_out_date=excluded.check_out_date,
		 raw_statement_row_json=excluded.raw_statement_row_json, last_import_id=excluded.last_import_id,
		 source_line=excluded.source_line, updated_at=excluded.updated_at`,
		propertyID, row.ReferenceNumber, nullableEvidenceString(row.HotelID), row.Status,
		row.BookedOn.UTC().Format(time.RFC3339), row.CheckInDate, row.CheckOutDate,
		statements.CanonicalRawJSON(row.Raw), importID, row.Line, now, now)
	return err
}

func nullableEvidenceString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}
