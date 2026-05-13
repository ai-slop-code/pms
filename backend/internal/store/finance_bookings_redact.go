package store

import (
	"context"
	"time"
)

// RedactFinanceBookingPII clears personally-identifying columns on a
// finance_bookings row and overwrites the raw CSV-row JSON blobs with a
// minimal redacted shape. Used by the GDPR/erase-request path; the row
// itself is preserved so historical analytics aggregations remain
// queryable.
//
// Redacted columns: guest_name, booker_name, guest_request, country.
// The raw JSON columns are replaced with a sentinel JSON object so
// callers can still tell whether the row originated from a payout or
// statement upload.
func (s *Store) RedactFinanceBookingPII(ctx context.Context, bookingID int64) error {
	if bookingID <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE finance_bookings SET
			guest_name = NULL,
			booker_name = NULL,
			guest_request = NULL,
			country = NULL,
			raw_payout_row_json = CASE
				WHEN raw_payout_row_json IS NULL THEN NULL
				ELSE '{"redacted":true}' END,
			raw_statement_row_json = CASE
				WHEN raw_statement_row_json IS NULL THEN NULL
				ELSE '{"redacted":true}' END,
			updated_at = ?
		WHERE id = ?`,
		now, bookingID)
	return err
}
