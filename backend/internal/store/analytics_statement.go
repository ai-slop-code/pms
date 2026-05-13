package store

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// FEAT-05 / PMS_12 §4 Phase-2 analytics primitives.
//
// All metrics here read from `finance_bookings` (the merged
// payout+statement table introduced in FEAT-04) and only consider rows
// where `has_statement_data = 1`. Buckets / cohorts that contain zero
// statement-aware rows are returned as the empty slice — the API layer
// then renders the "no statement data" empty state.

// ---------- Cancellation rate ----------

// CancellationCohortRow is one (cohort_month, rate) data point. The
// numerator is rows with status='CANCELLED'; rows whose status is
// MODIFIED / NO_SHOW / REFUSED_BY_HOTEL fall into the `other` bucket
// and are excluded from both numerator and denominator (PMS_12 N7).
type CancellationCohortRow struct {
	Month         string  // "2026-04"
	Cancelled     int
	Active        int
	Other         int
	Rate          float64
}

// ListCancellationByBookingCohort groups cancellations by the month
// of `booked_on`. Used for the "marketing/lead-quality" view.
func (s *Store) ListCancellationByBookingCohort(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]CancellationCohortRow, error) {
	return s.cancellationCohort(ctx, propertyID, fromUTC, toUTC, loc, "booked_on")
}

// ListCancellationByArrivalCohort groups cancellations by the month
// of `check_in_date`. Used for the operational "next 30 days exposure"
// view.
func (s *Store) ListCancellationByArrivalCohort(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]CancellationCohortRow, error) {
	return s.cancellationCohort(ctx, propertyID, fromUTC, toUTC, loc, "check_in_date")
}

func (s *Store) cancellationCohort(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location, dateCol string) ([]CancellationCohortRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+dateCol+` AS d, UPPER(COALESCE(status, '')) AS st
		  FROM finance_bookings
		 WHERE property_id = ?
		   AND has_statement_data = 1
		   AND `+dateCol+` IS NOT NULL`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bucket := map[string]*CancellationCohortRow{}
	for rows.Next() {
		var d, status string
		if err := rows.Scan(&d, &status); err != nil {
			return nil, err
		}
		t, parseErr := parseFlexibleDate(d, loc)
		if parseErr != nil {
			continue
		}
		if t.Before(fromUTC) || !t.Before(toUTC) {
			continue
		}
		key := t.In(loc).Format("2006-01")
		row := bucket[key]
		if row == nil {
			row = &CancellationCohortRow{Month: key}
			bucket[key] = row
		}
		switch status {
		case "CANCELLED":
			row.Cancelled++
		case "OK", "":
			row.Active++
		default:
			row.Other++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CancellationCohortRow, 0, len(bucket))
	for _, row := range bucket {
		den := row.Cancelled + row.Active
		if den > 0 {
			row.Rate = float64(row.Cancelled) / float64(den)
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out, nil
}

func parseFlexibleDate(v string, loc *time.Location) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, sql.ErrNoRows
}

// ---------- Lead time (statement-precise) ----------

// LeadTimeStatementBucket counts statement-active bookings whose
// `arrival - booked_on` falls in a fixed bucket. Active rows only:
// CANCELLED and `other` statuses are excluded so the histogram
// reflects materialised demand.
type LeadTimeStatementBucket struct {
	Bucket string
	Count  int
}

func (s *Store) ListLeadTimeStatementBuckets(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]LeadTimeStatementBucket, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT booked_on, check_in_date
		  FROM finance_bookings
		 WHERE property_id = ?
		   AND has_statement_data = 1
		   AND UPPER(COALESCE(status, '')) IN ('OK', '')
		   AND booked_on IS NOT NULL
		   AND check_in_date IS NOT NULL`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	order := []string{"0-3", "4-14", "15-45", "46+"}
	counts := map[string]int{"0-3": 0, "4-14": 0, "15-45": 0, "46+": 0}
	for rows.Next() {
		var bookedOn, checkIn string
		if err := rows.Scan(&bookedOn, &checkIn); err != nil {
			return nil, err
		}
		bo, err := parseFlexibleDate(bookedOn, loc)
		if err != nil {
			continue
		}
		ci, err := parseFlexibleDate(checkIn, loc)
		if err != nil {
			continue
		}
		if ci.Before(fromUTC) || !ci.Before(toUTC) {
			continue
		}
		days := int(ci.Sub(bo).Hours() / 24)
		if days < 0 {
			days = 0
		}
		switch {
		case days <= 3:
			counts["0-3"]++
		case days <= 14:
			counts["4-14"]++
		case days <= 45:
			counts["15-45"]++
		default:
			counts["46+"]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]LeadTimeStatementBucket, 0, len(order))
	for _, k := range order {
		out = append(out, LeadTimeStatementBucket{Bucket: k, Count: counts[k]})
	}
	return out, nil
}

// ---------- Persons distribution + ADR by persons ----------

// PersonsBucket counts active stays per `persons` value and reports
// the weighted ADR for that bucket. Rows with NULL or 0 `persons` are
// excluded (not enough information).
type PersonsBucket struct {
	Persons       int
	Stays         int
	GrossCents    int64
	RoomNights    int
	ADRCents      int64
}

func (s *Store) ListPersonsDistribution(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]PersonsBucket, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT COALESCE(persons, 0) AS p,
		       COALESCE(amount_cents, 0) AS gross,
		       COALESCE(room_nights, 0) AS nights,
		       check_in_date
		  FROM finance_bookings
		 WHERE property_id = ?
		   AND has_statement_data = 1
		   AND UPPER(COALESCE(status, '')) IN ('OK', '')
		   AND check_in_date IS NOT NULL
		   AND COALESCE(persons, 0) > 0`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bucket := map[int]*PersonsBucket{}
	for rows.Next() {
		var persons int
		var gross int64
		var nights int
		var checkIn string
		if err := rows.Scan(&persons, &gross, &nights, &checkIn); err != nil {
			return nil, err
		}
		ci, err := parseFlexibleDate(checkIn, loc)
		if err != nil {
			continue
		}
		if ci.Before(fromUTC) || !ci.Before(toUTC) {
			continue
		}
		row := bucket[persons]
		if row == nil {
			row = &PersonsBucket{Persons: persons}
			bucket[persons] = row
		}
		row.Stays++
		row.GrossCents += gross
		row.RoomNights += nights
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]PersonsBucket, 0, len(bucket))
	for _, row := range bucket {
		if row.RoomNights > 0 {
			row.ADRCents = row.GrossCents / int64(row.RoomNights)
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Persons < out[j].Persons })
	return out, nil
}

// ---------- Commission rate trend + per-stay ----------

// CommissionTrendRow reports the weighted commission rate per booked-on
// month: Σ commission / Σ amount over active stays.
type CommissionTrendRow struct {
	Month            string
	CommissionCents  int64
	GrossCents       int64
	Rate             float64
	Stays            int
}

func (s *Store) ListCommissionRateTrend(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]CommissionTrendRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT booked_on,
		       COALESCE(commission_cents, 0),
		       COALESCE(amount_cents, 0)
		  FROM finance_bookings
		 WHERE property_id = ?
		   AND has_statement_data = 1
		   AND UPPER(COALESCE(status, '')) IN ('OK', '')
		   AND booked_on IS NOT NULL`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bucket := map[string]*CommissionTrendRow{}
	for rows.Next() {
		var bookedOn string
		var commission, gross int64
		if err := rows.Scan(&bookedOn, &commission, &gross); err != nil {
			return nil, err
		}
		bo, err := parseFlexibleDate(bookedOn, loc)
		if err != nil {
			continue
		}
		if bo.Before(fromUTC) || !bo.Before(toUTC) {
			continue
		}
		key := bo.In(loc).Format("2006-01")
		row := bucket[key]
		if row == nil {
			row = &CommissionTrendRow{Month: key}
			bucket[key] = row
		}
		row.CommissionCents += commission
		row.GrossCents += gross
		row.Stays++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CommissionTrendRow, 0, len(bucket))
	for _, row := range bucket {
		if row.GrossCents > 0 {
			row.Rate = float64(row.CommissionCents) / float64(row.GrossCents)
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out, nil
}

// CommissionPerStayRow is one row of the commission-per-stay bar chart.
// Sorted by check-in date desc by the caller.
type CommissionPerStayRow struct {
	BookingID       int64
	ReferenceNumber string
	GuestName       string
	CheckInDate     string
	CheckOutDate    string
	GrossCents      int64
	CommissionCents int64
	Rate            float64
}

func (s *Store) ListCommissionPerStay(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time, loc *time.Location) ([]CommissionPerStayRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, reference_number,
		       COALESCE(guest_name, ''),
		       COALESCE(check_in_date, ''),
		       COALESCE(check_out_date, ''),
		       COALESCE(amount_cents, 0),
		       COALESCE(commission_cents, 0)
		  FROM finance_bookings
		 WHERE property_id = ?
		   AND has_statement_data = 1
		   AND UPPER(COALESCE(status, '')) IN ('OK', '')
		   AND check_in_date IS NOT NULL
		 ORDER BY check_in_date DESC, id DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommissionPerStayRow{}
	for rows.Next() {
		var r CommissionPerStayRow
		if err := rows.Scan(&r.BookingID, &r.ReferenceNumber, &r.GuestName, &r.CheckInDate, &r.CheckOutDate, &r.GrossCents, &r.CommissionCents); err != nil {
			return nil, err
		}
		ci, err := parseFlexibleDate(r.CheckInDate, loc)
		if err != nil {
			continue
		}
		if ci.Before(fromUTC) || !ci.Before(toUTC) {
			continue
		}
		if r.GrossCents > 0 {
			r.Rate = float64(r.CommissionCents) / float64(r.GrossCents)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- Last statement freshness ----------

// LastStatementBookedOn returns the most recent `booked_on` timestamp
// among rows with statement data, used by the freshness disclaimer.
func (s *Store) LastStatementBookedOn(ctx context.Context, propertyID int64) (*time.Time, error) {
	var v sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT MAX(booked_on) FROM finance_bookings
		 WHERE property_id = ? AND has_statement_data = 1`, propertyID).Scan(&v)
	if err != nil {
		return nil, err
	}
	if !v.Valid || v.String == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, perr := time.Parse(layout, v.String); perr == nil {
			return &t, nil
		}
	}
	return nil, nil
}

// HasAnyStatementData reports whether the property has at least one
// statement-aware finance_bookings row. Used by the API layer to
// decide whether to surface "no statement data" empty states.
func (s *Store) HasAnyStatementData(ctx context.Context, propertyID int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM finance_bookings
		 WHERE property_id = ? AND has_statement_data = 1
		 LIMIT 1`, propertyID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
