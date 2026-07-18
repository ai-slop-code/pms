package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PMS_19 §6.1.1 per-night coverage. occupancy_nights is the night-level source
// of truth for calendar counts, analytics, and duplicate resolution. A partial
// unique index (property_id, local_night_date) WHERE active = 1 is the hard
// capacity-one guarantee.

// OccupancyNight is one property-local night owned by an occupancy row.
type OccupancyNight struct {
	ID                 int64
	PropertyID         int64
	OccupancyID        int64
	LocalNightDate     string
	UpstreamSourceType sql.NullString
	UpstreamEventUID   sql.NullString
	Active             bool
}

// ErrOccupancyNightConflict is returned when activating a night that another
// active representation already owns for a capacity-one property.
var ErrOccupancyNightConflict = fmt.Errorf("occupancy night already covered by another active representation")

// nightsUTC returns the property-local night labels covered by [start, end).
// Booking.com all-day events are stored as UTC midnights whose date parts are
// the feed's civil labels, so we compare on UTC date parts (see parse.go).
func nightsUTC(start, end time.Time) []string {
	s := toUTCMidnight(start)
	e := toUTCMidnight(end)
	var out []string
	for d := s; d.Before(e); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// deactivateOccupancyNightsTx flips every coverage row for an occupancy to
// inactive so its nights free up for other representations.
func deactivateOccupancyNightsTx(ctx context.Context, tx *sql.Tx, occupancyID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE occupancy_nights SET active = 0 WHERE occupancy_id = ?`, occupancyID)
	return err
}

// clearOccupancyNightsTx removes coverage rows entirely (used before a rebuild).
func clearOccupancyNightsTx(ctx context.Context, tx *sql.Tx, occupancyID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM occupancy_nights WHERE occupancy_id = ?`, occupancyID)
	return err
}

// insertOccupancyNightTx inserts one night. When active, the partial unique
// index rejects a second active row for the same night; the caller receives
// ErrOccupancyNightConflict.
func insertOccupancyNightTx(ctx context.Context, tx *sql.Tx, propertyID, occupancyID int64, night, upstreamSourceType, upstreamUID string, active bool) error {
	a := 0
	if active {
		a = 1
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO occupancy_nights (property_id, occupancy_id, local_night_date, upstream_source_type, upstream_event_uid, active)
		VALUES (?, ?, ?, ?, ?, ?)`,
		propertyID, occupancyID, night, nullableString(upstreamSourceType), nullableString(upstreamUID), a)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrOccupancyNightConflict
		}
		return err
	}
	return nil
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

// setOccupancyNightsTx replaces the coverage for a row with the given nights.
// It clears existing rows first, so it is safe to call repeatedly.
func setOccupancyNightsTx(ctx context.Context, tx *sql.Tx, propertyID, occupancyID int64, nights []string, upstreamSourceType, upstreamUID string, active bool) error {
	if err := clearOccupancyNightsTx(ctx, tx, occupancyID); err != nil {
		return err
	}
	for _, n := range nights {
		if err := insertOccupancyNightTx(ctx, tx, propertyID, occupancyID, n, upstreamSourceType, upstreamUID, active); err != nil {
			return err
		}
	}
	return nil
}

// ActiveOccupancyNightCount returns how many active representations cover a
// property-local night. For a capacity-one property this must never exceed 1;
// the calendar/analytics use it as a data-quality guard.
func (s *Store) ActiveOccupancyNightCount(ctx context.Context, propertyID int64, night string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM occupancy_nights WHERE property_id = ? AND local_night_date = ? AND active = 1`,
		propertyID, night).Scan(&n)
	return n, err
}

// ListActiveOccupancyNightDates returns the set of active covered nights for a
// property inside [fromDate, toDate) (YYYY-MM-DD, half-open).
func (s *Store) ListActiveOccupancyNightDates(ctx context.Context, propertyID int64, fromDate, toDate string) (map[string]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT local_night_date, occupancy_id FROM occupancy_nights
		WHERE property_id = ? AND active = 1 AND local_night_date >= ? AND local_night_date < ?`,
		propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var d string
		var occID int64
		if err := rows.Scan(&d, &occID); err != nil {
			return nil, err
		}
		out[d] = occID
	}
	return out, rows.Err()
}

// GetOccupancyCoveredNights returns the active covered nights for one occupancy.
func (s *Store) GetOccupancyCoveredNights(ctx context.Context, occupancyID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT local_night_date FROM occupancy_nights WHERE occupancy_id = ? AND active = 1 ORDER BY local_night_date ASC`, occupancyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CoveredNightsForOccupancies batch-loads active covered nights for a set of
// occupancy ids so the calendar can render night-level truth in one query.
func (s *Store) CoveredNightsForOccupancies(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT occupancy_id, local_night_date FROM occupancy_nights
		WHERE active = 1 AND occupancy_id IN (%s) ORDER BY local_night_date ASC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var occID int64
		var d string
		if err := rows.Scan(&occID, &d); err != nil {
			return nil, err
		}
		out[occID] = append(out[occID], d)
	}
	return out, rows.Err()
}

// OccupancyMetricNights returns PMS 21 night-level counts for [fromDate,
// toDate) (YYYY-MM-DD, half-open): availability-blocking named/availability
// nights and sold guest named-stay nights. Raw booking blocks are intentionally
// excluded.
func (s *Store) OccupancyMetricNights(ctx context.Context, propertyID int64, fromDate, toDate string) (availability, guest int, err error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyOccupancyMetricNights(ctx, propertyID, fromDate, toDate)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.stay_type,
		       COALESCE(ns.review_status, 'confirmed'),
		       CASE WHEN ns.manual_revenue_cents IS NOT NULL OR EXISTS (
		         SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id
		       ) THEN 1 ELSE 0 END,
		       COALESCE(ns.stay_outcome, '')
		FROM named_stay_nights n
		JOIN named_stays ns ON ns.id = n.named_stay_id
		WHERE n.property_id = ? AND n.active = 1 AND n.local_night_date >= ? AND n.local_night_date < ?
		  AND ns.status = 'active'`,
		propertyID, fromDate, toDate)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var stayType, reviewStatus, stayOutcome string
		var hasRevenue int
		if err := rows.Scan(&stayType, &reviewStatus, &hasRevenue, &stayOutcome); err != nil {
			return 0, 0, err
		}
		availability++
		if reviewStatus == "confirmed" && stayOutcome == "" && (stayType == "booking_com" || (stayType == "external" && hasRevenue == 1)) {
			guest++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	blockRows, err := s.DB.QueryContext(ctx, `
		SELECT start_date, end_date
		FROM property_availability_blocks
		WHERE property_id = ? AND status = 'active' AND start_date < ? AND end_date > ?`, propertyID, toDate, fromDate)
	if err != nil {
		return 0, 0, err
	}
	defer blockRows.Close()
	from, _ := time.Parse("2006-01-02", fromDate)
	to, _ := time.Parse("2006-01-02", toDate)
	for blockRows.Next() {
		var start, end string
		if err := blockRows.Scan(&start, &end); err != nil {
			return 0, 0, err
		}
		bs, err1 := time.Parse("2006-01-02", start)
		be, err2 := time.Parse("2006-01-02", end)
		if err1 != nil || err2 != nil {
			continue
		}
		if bs.Before(from) {
			bs = from
		}
		if be.After(to) {
			be = to
		}
		if be.After(bs) {
			availability += int(be.Sub(bs).Hours()/24 + 0.5)
		}
	}
	return availability, guest, blockRows.Err()
}

func (s *Store) legacyOccupancyMetricNights(ctx context.Context, propertyID int64, fromDate, toDate string) (availability, guest int, err error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT COALESCE(o.closure_state, ''), COALESCE(o.representation_kind, '')
		FROM occupancy_nights n
		JOIN occupancies o ON o.id = n.occupancy_id
		WHERE n.property_id = ? AND n.active = 1 AND n.local_night_date >= ? AND n.local_night_date < ?`, propertyID, fromDate, toDate)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var closure, kind string
		if err := rows.Scan(&closure, &kind); err != nil {
			return 0, 0, err
		}
		if closure == ClosureStateClosed {
			continue
		}
		availability++
		if kind == RepresentationNamedStay || closure == ClosureStateExternalSale {
			guest++
		}
	}
	return availability, guest, rows.Err()
}

// PropertyHasOccupancyNights reports whether any night-coverage rows exist for a
// property. Used to decide whether occupancy_nights is authoritative (sync /
// backfill ran) or whether the raw start/end span must be used as a fallback
// (rows created outside sync, e.g. tests / legacy).
func (s *Store) PropertyHasOccupancyNights(ctx context.Context, propertyID int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM occupancy_nights WHERE property_id = ?)`, propertyID).Scan(&n)
	return n == 1, err
}

// CleaningCheckoutDatesForOccupancy returns the checkout dates + cleaning kind
// for an occupancy (PMS_19 §5.4):
//   - named_stay / external_sale: one checkout at the stay end.
//   - unnamed_block / legacy: one provisional checkout per blocked night
//     (night + 1). When propertyHasCoverage is true, occupancy_nights is
//     authoritative (an empty set means no checkouts); otherwise the raw
//     start/end span is used.
func (s *Store) CleaningCheckoutDatesForOccupancy(ctx context.Context, o *Occupancy, propertyHasCoverage bool) (dates []string, kind string) {
	named := o.RepresentationKind.String == RepresentationNamedStay ||
		o.ClosureState.String == ClosureStateExternalSale
	if named {
		return []string{toUTCMidnight(o.EndAt).Format("2006-01-02")}, CleaningKindNamedStay
	}
	var nights []string
	if propertyHasCoverage {
		nights, _ = s.GetOccupancyCoveredNights(ctx, o.ID)
	} else {
		nights = nightsUTC(o.StartAt, o.EndAt)
	}
	out := make([]string, 0, len(nights))
	for _, n := range nights {
		d, err := time.Parse("2006-01-02", n)
		if err != nil {
			continue
		}
		out = append(out, d.AddDate(0, 0, 1).Format("2006-01-02"))
	}
	return out, CleaningKindProvisionalBlock
}
