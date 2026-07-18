package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ---------------- Shared helpers ----------------

// analyticsActiveStatus is the legacy predicate kept for compatibility-only
// queries that still inspect historical occupancy cancellation rows.
//
// Closure-aware semantics (PMS_14 §4):
//
//   - status ∈ {active, updated} — inherited ICS-driven gating.
//   - closure_state IS NULL OR != 'closed' — closed rows drop out of
//     all active-aggregates entirely. Externally-sold rows stay in
//     because the night is still occupied (just not by a Booking guest).
//     Stay outcomes stay active: the Booking.com calendar remained blocked.
const analyticsActiveStatus = `status IN ('active', 'updated') AND (closure_state IS NULL OR closure_state != 'closed')`
const analyticsCancelledStatus = `status IN ('cancelled', 'deleted_from_source')`

// diacriticFold is a compact lookup table for the common Latin
// diacritic characters that appear in Booking.com guest names. It
// implements the NFD-then-strip-combining-marks behaviour required
// by the spec without pulling in golang.org/x/text.
var diacriticFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a', 'ā': 'a', 'ą': 'a',
	'Á': 'a', 'À': 'a', 'Â': 'a', 'Ä': 'a', 'Ã': 'a', 'Å': 'a', 'Ā': 'a', 'Ą': 'a',
	'č': 'c', 'ć': 'c', 'ç': 'c', 'ĉ': 'c',
	'Č': 'c', 'Ć': 'c', 'Ç': 'c', 'Ĉ': 'c',
	'ď': 'd', 'đ': 'd',
	'Ď': 'd', 'Đ': 'd',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ě': 'e', 'ę': 'e',
	'É': 'e', 'È': 'e', 'Ê': 'e', 'Ë': 'e', 'Ē': 'e', 'Ě': 'e', 'Ę': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i',
	'Í': 'i', 'Ì': 'i', 'Î': 'i', 'Ï': 'i', 'Ī': 'i',
	'ĺ': 'l', 'ľ': 'l', 'ł': 'l',
	'Ĺ': 'l', 'Ľ': 'l', 'Ł': 'l',
	'ñ': 'n', 'ń': 'n', 'ň': 'n',
	'Ñ': 'n', 'Ń': 'n', 'Ň': 'n',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ø': 'o', 'ō': 'o', 'ő': 'o',
	'Ó': 'o', 'Ò': 'o', 'Ô': 'o', 'Ö': 'o', 'Õ': 'o', 'Ø': 'o', 'Ō': 'o', 'Ő': 'o',
	'ŕ': 'r', 'ř': 'r',
	'Ŕ': 'r', 'Ř': 'r',
	'š': 's', 'ś': 's', 'ş': 's', 'ß': 's',
	'Š': 's', 'Ś': 's', 'Ş': 's',
	'ť': 't', 'ţ': 't',
	'Ť': 't', 'Ţ': 't',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ū': 'u', 'ů': 'u', 'ű': 'u',
	'Ú': 'u', 'Ù': 'u', 'Û': 'u', 'Ü': 'u', 'Ū': 'u', 'Ů': 'u', 'Ű': 'u',
	'ý': 'y', 'ÿ': 'y',
	'Ý': 'y', 'Ÿ': 'y',
	'ž': 'z', 'ź': 'z', 'ż': 'z',
	'Ž': 'z', 'Ź': 'z', 'Ż': 'z',
}

// NormalizeGuestName applies the normalization rule defined in
// PMS_05 Analytics Module Spec: lowercase, strip common Latin
// diacritics, drop any remaining combining marks, trim, collapse
// internal whitespace. Returns "" for inputs that would normalize to
// empty.
func NormalizeGuestName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if mapped, ok := diacriticFold[r]; ok {
			b.WriteRune(mapped)
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// ---------------- A1: Freshness ----------------

type AnalyticsFreshness struct {
	LastICSSyncAt         *time.Time
	LastPayoutDate        *time.Time
	LastStatementBookedOn *time.Time
	UnmatchedPayoutsCount int
	HasStatementData      bool
}

func (s *Store) GetAnalyticsFreshness(ctx context.Context, propertyID int64) (*AnalyticsFreshness, error) {
	out := &AnalyticsFreshness{}

	var lastSync sql.NullString
	_ = s.DB.QueryRowContext(ctx, `
		SELECT MAX(finished_at) FROM occupancy_sync_runs
		WHERE property_id = ? AND status = 'success'`, propertyID).Scan(&lastSync)
	if lastSync.Valid && lastSync.String != "" {
		if t, err := time.Parse(time.RFC3339, lastSync.String); err == nil {
			out.LastICSSyncAt = &t
		}
	}

	var lastPayout sql.NullString
	_ = s.DB.QueryRowContext(ctx, `
		SELECT MAX(payout_date) FROM finance_bookings
		WHERE property_id = ?`, propertyID).Scan(&lastPayout)
	if lastPayout.Valid && lastPayout.String != "" {
		if t, err := time.Parse(time.RFC3339, lastPayout.String); err == nil {
			out.LastPayoutDate = &t
		} else if t2, err := time.Parse("2006-01-02", lastPayout.String); err == nil {
			out.LastPayoutDate = &t2
		}
	}

	if t, err := s.LastStatementBookedOn(ctx, propertyID); err == nil {
		out.LastStatementBookedOn = t
		out.HasStatementData = t != nil
	}

	if s.propertyHasNamedStays(ctx, propertyID) {
		_ = s.DB.QueryRowContext(ctx, `
			SELECT COUNT(1) FROM finance_bookings
			WHERE property_id = ? AND named_stay_id IS NULL`, propertyID).Scan(&out.UnmatchedPayoutsCount)
	} else {
		_ = s.DB.QueryRowContext(ctx, `
			SELECT COUNT(1) FROM finance_bookings
			WHERE property_id = ? AND occupancy_id IS NULL`, propertyID).Scan(&out.UnmatchedPayoutsCount)
	}

	return out, nil
}

// ---------------- A2: Outlook primitives ----------------

// OccupancyLite is a minimal projection used by analytics
// computations that do not need the full Occupancy row.
type OccupancyLite struct {
	ID                     int64
	StartAt                time.Time
	EndAt                  time.Time
	Status                 string
	ImportedAt             time.Time
	GuestName              string
	ClosureState           string // "", "closed", or "external_sale"
	ExternalNetAmountCents int64  // populated when ClosureState == "external_sale"
}

func (s *Store) propertyLocation(ctx context.Context, propertyID int64) *time.Location {
	var tz string
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(timezone, 'UTC') FROM properties WHERE id = ?`, propertyID).Scan(&tz)
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

func parsePropertyDate(date string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func dateRangeParams(fromUTC, toUTC time.Time, loc *time.Location) (string, string) {
	return fromUTC.In(loc).Format("2006-01-02"), toUTC.In(loc).Format("2006-01-02")
}

func (s *Store) propertyHasNamedStays(ctx context.Context, propertyID int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM named_stays WHERE property_id = ?)`, propertyID).Scan(&n)
	return n == 1
}

// ListActiveOccupanciesInDateRange returns sold/revenue-counting named stays
// overlapping [fromUTC, toUTC). Raw booking blocks and non-sold named stays are
// intentionally excluded for PMS 21 Stage 9 analytics semantics.
func (s *Store) ListActiveOccupanciesInDateRange(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]OccupancyLite, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyListActiveOccupanciesInDateRange(ctx, propertyID, fromUTC, toUTC)
	}
	loc := s.propertyLocation(ctx, propertyID)
	fromDate, toDate := dateRangeParams(fromUTC, toUTC, loc)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.check_in_date, ns.check_out_date, ns.status, ns.created_at,
		       ns.display_name, ns.stay_type, COALESCE(ns.manual_revenue_cents, 0)
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND COALESCE(ns.stay_outcome, '') = ''
		  AND ns.check_in_date < ?
		  AND ns.check_out_date > ?
		  AND (
		    ns.stay_type = 'booking_com'
		    OR (
		      ns.stay_type = 'external'
		      AND (
		        ns.manual_revenue_cents IS NOT NULL
		        OR EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)
		      )
		    )
		  )
		ORDER BY ns.check_in_date ASC, ns.id ASC`,
		propertyID, toDate, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyLite
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported, stayType string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName, &o.ClosureState, &o.ExternalNetAmountCents); err != nil {
			return nil, err
		}
		stayType = o.ClosureState
		o.ClosureState = ""
		if stayType == "external" {
			o.ClosureState = "external_sale"
		}
		o.StartAt = parsePropertyDate(start, loc)
		o.EndAt = parsePropertyDate(end, loc)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) legacyListActiveOccupanciesInDateRange(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]OccupancyLite, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, start_at, end_at, status, imported_at,
		       COALESCE(guest_display_name, ''),
		       COALESCE(closure_state, ''),
		       COALESCE(external_net_amount_cents, 0)
		FROM occupancies
		WHERE property_id = ?
		  AND `+analyticsActiveStatus+`
		  AND start_at < ?
		  AND end_at > ?
		ORDER BY start_at ASC`, propertyID, toUTC.Format(time.RFC3339), fromUTC.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyLite
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName, &o.ClosureState, &o.ExternalNetAmountCents); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		out = append(out, o)
	}
	return out, rows.Err()
}

// NightsSoldInRange counts occupied nights (date `d` with
// start_at_date ≤ d < end_at_date) that fall inside [fromDate, toDate)
// in the property timezone. The caller supplies the stay list already
// filtered to active status.
func NightsSoldInRange(stays []OccupancyLite, fromDate, toDate time.Time) int {
	if !toDate.After(fromDate) {
		return 0
	}
	count := 0
	for _, st := range stays {
		sd := toDateStart(st.StartAt, fromDate.Location())
		ed := toDateStart(st.EndAt, fromDate.Location())
		// Intersection with [fromDate, toDate)
		lo := sd
		if lo.Before(fromDate) {
			lo = fromDate
		}
		hi := ed
		if hi.After(toDate) {
			hi = toDate
		}
		if hi.After(lo) {
			count += int(hi.Sub(lo).Hours()/24 + 0.5)
		}
	}
	return count
}

func toDateStart(t time.Time, loc *time.Location) time.Time {
	tl := t.In(loc)
	return time.Date(tl.Year(), tl.Month(), tl.Day(), 0, 0, 0, 0, loc)
}

// AvailableNightsInRange returns the raw number of calendar nights in
// [fromDate, toDate) — one per calendar day. Use BookableNightsInRange
// when computing the occupancy% denominator: PMS_14 §4 requires that
// closed nights be subtracted from BOTH numerator and denominator so
// occupancy% stays stable when a Booking.com block is labelled
// `closed`.
func AvailableNightsInRange(fromDate, toDate time.Time) int {
	if !toDate.After(fromDate) {
		return 0
	}
	return int(toDate.Sub(fromDate).Hours()/24 + 0.5)
}

// BookableNightsInRange = AvailableNightsInRange − ClosedNightsInRange,
// floored at zero. This is the correct denominator for occupancy%
// calculations per PMS_14 §4.
func BookableNightsInRange(closedStays []OccupancyLite, fromDate, toDate time.Time) int {
	n := AvailableNightsInRange(fromDate, toDate) - ClosedNightsInRange(closedStays, fromDate, toDate)
	if n < 0 {
		return 0
	}
	return n
}

// ClosedNightsInRange counts the nights inside [fromDate, toDate) that
// fall on a stay flagged closure_state='closed'. Because
// ListActiveOccupanciesInDateRange filters closed rows out, callers
// that want this count must use ListOccupanciesIncludingClosedInRange
// instead.
func ClosedNightsInRange(closedStays []OccupancyLite, fromDate, toDate time.Time) int {
	if !toDate.After(fromDate) {
		return 0
	}
	count := 0
	for _, st := range closedStays {
		if st.ClosureState != "closed" {
			continue
		}
		sd := toDateStart(st.StartAt, fromDate.Location())
		ed := toDateStart(st.EndAt, fromDate.Location())
		lo := sd
		if lo.Before(fromDate) {
			lo = fromDate
		}
		hi := ed
		if hi.After(toDate) {
			hi = toDate
		}
		if hi.After(lo) {
			count += int(hi.Sub(lo).Hours()/24 + 0.5)
		}
	}
	return count
}

// ExternalSaleRevenueCentsInRange returns the operator-entered net
// amounts contributed by externally-sold rows in [fromDate, toDate),
// prorated by the number of overlapping nights / total stay nights.
// Stays whose ClosureState is not 'external_sale' are skipped.
func ExternalSaleRevenueCentsInRange(stays []OccupancyLite, fromDate, toDate time.Time) int64 {
	if !toDate.After(fromDate) {
		return 0
	}
	var total int64
	for _, st := range stays {
		if st.ClosureState != "external_sale" || st.ExternalNetAmountCents == 0 {
			continue
		}
		sd := toDateStart(st.StartAt, fromDate.Location())
		ed := toDateStart(st.EndAt, fromDate.Location())
		stayNights := int(ed.Sub(sd).Hours()/24 + 0.5)
		if stayNights <= 0 {
			continue
		}
		lo := sd
		if lo.Before(fromDate) {
			lo = fromDate
		}
		hi := ed
		if hi.After(toDate) {
			hi = toDate
		}
		if !hi.After(lo) {
			continue
		}
		overlap := int(hi.Sub(lo).Hours()/24 + 0.5)
		total += st.ExternalNetAmountCents * int64(overlap) / int64(stayNights)
	}
	return total
}

// ListClosedOccupanciesInDateRange returns active non-sold nights that reduce
// bookable availability under PMS 21: maintenance/personal-use stays, external
// stays without revenue, review-required stays, availability blocks, and legacy
// closed rows retained for compatibility.
func (s *Store) ListClosedOccupanciesInDateRange(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]OccupancyLite, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyListClosedOccupanciesInDateRange(ctx, propertyID, fromUTC, toUTC)
	}
	loc := s.propertyLocation(ctx, propertyID)
	fromDate, toDate := dateRangeParams(fromUTC, toUTC, loc)
	var out []OccupancyLite

	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.check_in_date, ns.check_out_date, ns.status, ns.created_at,
		       ns.display_name
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND ns.check_in_date < ?
		  AND ns.check_out_date > ?
		  AND (
		    COALESCE(ns.review_status, 'confirmed') != 'confirmed'
		    OR ns.stay_type IN ('maintenance', 'personal_use')
		    OR (
		      ns.stay_type = 'external'
		      AND ns.manual_revenue_cents IS NULL
		      AND NOT EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)
		    )
		  )
		ORDER BY ns.check_in_date ASC, ns.id ASC`, propertyID, toDate, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName); err != nil {
			return nil, err
		}
		o.StartAt = parsePropertyDate(start, loc)
		o.EndAt = parsePropertyDate(end, loc)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		o.ClosureState = "closed"
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	blockRows, err := s.DB.QueryContext(ctx, `
		SELECT id, start_date, end_date, created_at, COALESCE(reason, '')
		FROM property_availability_blocks
		WHERE property_id = ? AND status = 'active' AND start_date < ? AND end_date > ?
		ORDER BY start_date ASC, id ASC`, propertyID, toDate, fromDate)
	if err != nil {
		return nil, err
	}
	defer blockRows.Close()
	for blockRows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := blockRows.Scan(&o.ID, &start, &end, &imported, &o.GuestName); err != nil {
			return nil, err
		}
		o.StartAt = parsePropertyDate(start, loc)
		o.EndAt = parsePropertyDate(end, loc)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		o.Status = "active"
		o.ClosureState = "closed"
		out = append(out, o)
	}
	if err := blockRows.Err(); err != nil {
		return nil, err
	}

	legacyRows, err := s.DB.QueryContext(ctx, `
		SELECT id, start_at, end_at, status, imported_at, COALESCE(guest_display_name, '')
		FROM occupancies
		WHERE property_id = ?
		  AND status IN ('active', 'updated')
		  AND closure_state = 'closed'
		  AND start_at < ?
		  AND end_at > ?
		ORDER BY start_at ASC`, propertyID, toUTC.Format(time.RFC3339), fromUTC.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer legacyRows.Close()
	for legacyRows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := legacyRows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		o.ClosureState = "closed"
		out = append(out, o)
	}
	return out, legacyRows.Err()
}

func (s *Store) legacyListClosedOccupanciesInDateRange(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]OccupancyLite, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, start_at, end_at, status, imported_at,
		       COALESCE(guest_display_name, ''), COALESCE(closure_state, ''), 0
		FROM occupancies
		WHERE property_id = ?
		  AND status IN ('active', 'updated')
		  AND closure_state = 'closed'
		  AND start_at < ?
		  AND end_at > ?
		ORDER BY start_at ASC`, propertyID, toUTC.Format(time.RFC3339), fromUTC.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyLite
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName, &o.ClosureState, &o.ExternalNetAmountCents); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		out = append(out, o)
	}
	return out, rows.Err()
}

// PayoutForStayRow is the subset of payout fields analytics needs.
type PayoutForStayRow struct {
	OccupancyID            int64
	CheckInDate            string
	GrossCents             int64
	CommissionCents        int64
	PaymentServiceFeeCents int64
	NetCents               int64
	GuestName              string
}

// SumPayoutGrossNetForStays returns finance/manual-revenue totals plus the set
// of named stay IDs that have materialized revenue with arrival inside
// [fromDate, toDate).
func (s *Store) SumPayoutGrossNetForStays(ctx context.Context, propertyID int64, fromDate, toDate time.Time) (grossCents, netCents, commissionCents, feesCents int64, matchedIDs []int64, err error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacySumPayoutGrossNetForStays(ctx, propertyID, fromDate, toDate)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT named_stay_id,
			COALESCE(amount_cents, 0), COALESCE(commission_cents, 0),
			COALESCE(payment_service_fee_cents, 0), COALESCE(net_cents, 0)
		FROM finance_bookings
		WHERE property_id = ?
		  AND named_stay_id IS NOT NULL
		  AND check_in_date IS NOT NULL
		  AND check_in_date >= ?
		  AND check_in_date < ?
		  AND (reservation_status IS NULL OR reservation_status NOT IN ('cancelled_by_guest','cancelled_by_partner'))`,
		propertyID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	defer rows.Close()
	seen := map[int64]struct{}{}
	for rows.Next() {
		var stayID sql.NullInt64
		var g, c, f, n int64
		if err := rows.Scan(&stayID, &g, &c, &f, &n); err != nil {
			return 0, 0, 0, 0, nil, err
		}
		grossCents += g
		commissionCents += c
		feesCents += f
		netCents += n
		if stayID.Valid {
			if _, ok := seen[stayID.Int64]; !ok {
				seen[stayID.Int64] = struct{}{}
				matchedIDs = append(matchedIDs, stayID.Int64)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, 0, nil, err
	}

	manualRows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, COALESCE(ns.manual_revenue_cents, 0)
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.stay_type = 'external'
		  AND ns.manual_revenue_cents IS NOT NULL
		  AND ns.check_in_date >= ?
		  AND ns.check_in_date < ?
		  AND NOT EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)`,
		propertyID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	defer manualRows.Close()
	for manualRows.Next() {
		var stayID, amount int64
		if err := manualRows.Scan(&stayID, &amount); err != nil {
			return 0, 0, 0, 0, nil, err
		}
		grossCents += amount
		netCents += amount
		if _, ok := seen[stayID]; !ok {
			seen[stayID] = struct{}{}
			matchedIDs = append(matchedIDs, stayID)
		}
	}
	return grossCents, netCents, commissionCents, feesCents, matchedIDs, manualRows.Err()
}

func (s *Store) legacySumPayoutGrossNetForStays(ctx context.Context, propertyID int64, fromDate, toDate time.Time) (grossCents, netCents, commissionCents, feesCents int64, matchedIDs []int64, err error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT occupancy_id,
			COALESCE(amount_cents, 0), COALESCE(commission_cents, 0),
			COALESCE(payment_service_fee_cents, 0), COALESCE(net_cents, 0)
		FROM finance_bookings
		WHERE property_id = ?
		  AND occupancy_id IS NOT NULL
		  AND check_in_date IS NOT NULL
		  AND check_in_date >= ?
		  AND check_in_date < ?
		  AND (reservation_status IS NULL OR reservation_status NOT IN ('cancelled_by_guest','cancelled_by_partner'))`,
		propertyID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	defer rows.Close()
	seen := map[int64]struct{}{}
	for rows.Next() {
		var occID sql.NullInt64
		var g, c, f, n int64
		if err := rows.Scan(&occID, &g, &c, &f, &n); err != nil {
			return 0, 0, 0, 0, nil, err
		}
		grossCents += g
		commissionCents += c
		feesCents += f
		netCents += n
		if occID.Valid {
			if _, ok := seen[occID.Int64]; !ok {
				seen[occID.Int64] = struct{}{}
				matchedIDs = append(matchedIDs, occID.Int64)
			}
		}
	}
	return grossCents, netCents, commissionCents, feesCents, matchedIDs, rows.Err()
}

// TrailingADR computes property-wide trailing 12-months ADR
// (cents per night). Returns 0 if fewer than 30 matched nights exist.
func (s *Store) TrailingADR(ctx context.Context, propertyID int64, asOf time.Time) (int64, error) {
	from := asOf.AddDate(-1, 0, 0)
	gross, _, _, _, matchedIDs, err := s.SumPayoutGrossNetForStays(ctx, propertyID, from, asOf)
	if err != nil {
		return 0, err
	}
	if len(matchedIDs) == 0 {
		return 0, nil
	}
	// Compute matched nights from the stays themselves.
	stays, err := s.listOccupanciesByIDs(ctx, propertyID, matchedIDs)
	if err != nil {
		return 0, err
	}
	totalNights := 0
	for _, st := range stays {
		nights := int(toDateStart(st.EndAt, time.UTC).Sub(toDateStart(st.StartAt, time.UTC)).Hours()/24 + 0.5)
		if nights > 0 {
			totalNights += nights
		}
	}
	if totalNights < 30 {
		return 0, nil
	}
	return gross / int64(totalNights), nil
}

func (s *Store) listOccupanciesByIDs(ctx context.Context, propertyID int64, ids []int64) ([]OccupancyLite, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyListOccupanciesByIDs(ctx, propertyID, ids)
	}
	loc := s.propertyLocation(ctx, propertyID)
	placeholders := make([]string, len(ids))
	args := []interface{}{propertyID}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`SELECT id, check_in_date, check_out_date, status, created_at, display_name
		FROM named_stays
		WHERE property_id = ? AND id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyLite
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName); err != nil {
			return nil, err
		}
		o.StartAt = parsePropertyDate(start, loc)
		o.EndAt = parsePropertyDate(end, loc)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) legacyListOccupanciesByIDs(ctx context.Context, propertyID int64, ids []int64) ([]OccupancyLite, error) {
	placeholders := make([]string, len(ids))
	args := []interface{}{propertyID}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`SELECT id, start_at, end_at, status, imported_at, COALESCE(guest_display_name, '')
		FROM occupancies WHERE property_id = ? AND id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyLite
	for rows.Next() {
		var o OccupancyLite
		var start, end, imported string
		if err := rows.Scan(&o.ID, &start, &end, &o.Status, &imported, &o.GuestName); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imported)
		out = append(out, o)
	}
	return out, rows.Err()
}

// UnsoldNightRow describes a single unbooked calendar night plus the
// label of the neighbouring stays.
type UnsoldNightRow struct {
	Date       string
	PrevStayID *int64
	PrevGuest  string
	NextStayID *int64
	NextGuest  string
}

func (s *Store) ListUnsoldNightsWithContext(ctx context.Context, propertyID int64, fromDate, toDate time.Time) ([]UnsoldNightRow, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate.AddDate(0, 0, -30), toDate.AddDate(0, 0, 30))
	if err != nil {
		return nil, err
	}
	loc := fromDate.Location()
	booked := map[string]int64{}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			booked[d.Format("2006-01-02")] = st.ID
		}
	}
	var out []UnsoldNightRow
	for d := fromDate; d.Before(toDate); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if _, occupied := booked[key]; occupied {
			continue
		}
		row := UnsoldNightRow{Date: key}
		// Find prev stay ending on d (checkout == d) or earlier.
		for i := len(stays) - 1; i >= 0; i-- {
			if !stays[i].EndAt.After(d.Add(24 * time.Hour)) {
				id := stays[i].ID
				row.PrevStayID = &id
				row.PrevGuest = stays[i].GuestName
				break
			}
		}
		// Find next stay starting on d+1 or later.
		for _, st := range stays {
			if !st.StartAt.Before(d.Add(24 * time.Hour)) {
				id := st.ID
				row.NextStayID = &id
				row.NextGuest = st.GuestName
				break
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// NewBookingsByDayRow — count of occupancies by imported_at date.
type NewBookingsByDayRow struct {
	Date  string
	Count int
}

func (s *Store) NewBookingsByDay(ctx context.Context, propertyID int64, sinceUTC time.Time) ([]NewBookingsByDayRow, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		rows, err := s.DB.QueryContext(ctx, `
			SELECT substr(imported_at, 1, 10) AS d, COUNT(1)
			FROM occupancies
			WHERE property_id = ?
			  AND imported_at >= ?
			  AND `+analyticsActiveStatus+`
			GROUP BY d
			ORDER BY d ASC`, propertyID, sinceUTC.Format(time.RFC3339))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []NewBookingsByDayRow
		for rows.Next() {
			var r NewBookingsByDayRow
			if err := rows.Scan(&r.Date, &r.Count); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, rows.Err()
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT substr(created_at, 1, 10) AS d, COUNT(1)
		FROM named_stays
		WHERE property_id = ?
		  AND created_at >= ?
		  AND status = 'active'
		  AND COALESCE(review_status, 'confirmed') = 'confirmed'
		  AND stay_type IN ('booking_com', 'external')
		GROUP BY d
		ORDER BY d ASC`, propertyID, sinceUTC.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NewBookingsByDayRow
	for rows.Next() {
		var r NewBookingsByDayRow
		if err := rows.Scan(&r.Date, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PacePointSeriesT returns cumulative nights-sold for each day in
// [fromDate, toDate) computed as "nights booked and already known at
// day d". Used for the Outlook pacing chart.
func (s *Store) PaceSeriesCumulative(ctx context.Context, propertyID int64, fromDate, toDate time.Time) ([]NewBookingsByDayRow, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	loc := fromDate.Location()
	byDay := map[string]int{}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(fromDate) || !d.Before(toDate) {
				continue
			}
			byDay[d.Format("2006-01-02")]++
		}
	}
	var out []NewBookingsByDayRow
	running := 0
	for d := fromDate; d.Before(toDate); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		running += byDay[key]
		out = append(out, NewBookingsByDayRow{Date: key, Count: running})
	}
	return out, nil
}

// ---------------- A3: Performance primitives ----------------

type MonthlyOccAdrRow struct {
	Month            string
	NightsSold       int
	AvailableNights  int
	GrossCents       int64
	NetCents         int64
	CommissionCents  int64
	PaymentFeesCents int64
	MatchedNights    int
}

// ListMonthlyOccupancyAndADR returns monthly rollups for an inclusive
// range of YYYY-MM strings. Months without any activity are returned
// as zero rows.
func (s *Store) ListMonthlyOccupancyAndADR(ctx context.Context, propertyID int64, fromMonth, toMonth string, loc *time.Location) ([]MonthlyOccAdrRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	start, err := time.ParseInLocation("2006-01", fromMonth, loc)
	if err != nil {
		return nil, err
	}
	end, err := time.ParseInLocation("2006-01", toMonth, loc)
	if err != nil {
		return nil, err
	}
	end = end.AddDate(0, 1, 0)
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, start, end)
	if err != nil {
		return nil, err
	}
	closedStays, err := s.ListClosedOccupanciesInDateRange(ctx, propertyID, start, end)
	if err != nil {
		return nil, err
	}

	// Per-month active stay -> nights split (ignoring revenue).
	type monthAgg struct {
		nights, matchedNights      int
		grossC, netC, commC, feesC int64
		availableNights            int
	}
	agg := map[string]*monthAgg{}
	ensure := func(key string) *monthAgg {
		if a, ok := agg[key]; ok {
			return a
		}
		a := &monthAgg{}
		agg[key] = a
		return a
	}

	// Bookable nights per month (= raw available − closed nights).
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 1, 0) {
		monthEnd := cursor.AddDate(0, 1, 0)
		key := cursor.Format("2006-01")
		a := ensure(key)
		a.availableNights = BookableNightsInRange(closedStays, cursor, monthEnd)
	}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(start) || !d.Before(end) {
				continue
			}
			key := d.Format("2006-01")
			a := ensure(key)
			a.nights++
		}
	}

	// Revenue — cohort by arrival date (check_in_date on payout/stay).
	payoutRows, err := s.DB.QueryContext(ctx, `
		SELECT named_stay_id, substr(check_in_date, 1, 7) AS m,
			COALESCE(amount_cents, 0), COALESCE(commission_cents, 0),
			COALESCE(payment_service_fee_cents, 0), COALESCE(net_cents, 0)
		FROM finance_bookings
		WHERE property_id = ?
		  AND named_stay_id IS NOT NULL
		  AND check_in_date IS NOT NULL
		  AND check_in_date >= ?
		  AND check_in_date < ?`,
		propertyID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer payoutRows.Close()
	matchedByStay := map[int64]string{}
	for payoutRows.Next() {
		var stayID sql.NullInt64
		var m string
		var g, c, f, n int64
		if err := payoutRows.Scan(&stayID, &m, &g, &c, &f, &n); err != nil {
			return nil, err
		}
		a := ensure(m)
		a.grossC += g
		a.commC += c
		a.feesC += f
		a.netC += n
		if stayID.Valid {
			matchedByStay[stayID.Int64] = m
		}
	}
	manualRows, err := s.DB.QueryContext(ctx, `
		SELECT id, substr(check_in_date, 1, 7), COALESCE(manual_revenue_cents, 0)
		FROM named_stays ns
		WHERE property_id = ?
		  AND status = 'active'
		  AND COALESCE(review_status, 'confirmed') = 'confirmed'
		  AND stay_type = 'external'
		  AND manual_revenue_cents IS NOT NULL
		  AND check_in_date >= ? AND check_in_date < ?
		  AND NOT EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)`,
		propertyID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer manualRows.Close()
	for manualRows.Next() {
		var stayID, amount int64
		var m string
		if err := manualRows.Scan(&stayID, &m, &amount); err != nil {
			return nil, err
		}
		a := ensure(m)
		a.grossC += amount
		a.netC += amount
		matchedByStay[stayID] = m
	}
	// Matched nights = nights from stays whose ID appears in matchedByStay.
	for _, st := range stays {
		m, ok := matchedByStay[st.ID]
		if !ok {
			continue
		}
		_ = m
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(start) || !d.Before(end) {
				continue
			}
			key := d.Format("2006-01")
			a := ensure(key)
			a.matchedNights++
		}
	}

	var out []MonthlyOccAdrRow
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 1, 0) {
		key := cursor.Format("2006-01")
		a := ensure(key)
		out = append(out, MonthlyOccAdrRow{
			Month: key, NightsSold: a.nights, AvailableNights: a.availableNights,
			GrossCents: a.grossC, NetCents: a.netC,
			CommissionCents: a.commC, PaymentFeesCents: a.feesC,
			MatchedNights: a.matchedNights,
		})
	}
	return out, nil
}

type WeeklyCell struct {
	Year       int
	Week       int
	NightsSold int
	AvailableN int
}

func (s *Store) ListWeeklyOccupancy(ctx context.Context, propertyID int64, fromYear, toYear int, loc *time.Location) ([]WeeklyCell, error) {
	if loc == nil {
		loc = time.UTC
	}
	start := time.Date(fromYear, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(toYear+1, 1, 1, 0, 0, 0, 0, loc)
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, start, end)
	if err != nil {
		return nil, err
	}
	closedStays, err := s.ListClosedOccupanciesInDateRange(ctx, propertyID, start, end)
	if err != nil {
		return nil, err
	}
	// Per-day flag: true when the day falls inside a closed stay.
	closedDay := map[string]bool{}
	for _, st := range closedStays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(start) || !d.Before(end) {
				continue
			}
			closedDay[d.Format("2006-01-02")] = true
		}
	}
	cellKey := func(y, w int) string { return fmt.Sprintf("%04d-%02d", y, w) }
	sold := map[string]int{}
	avail := map[string]int{}
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		if closedDay[d.Format("2006-01-02")] {
			continue
		}
		y, w := d.ISOWeek()
		avail[cellKey(y, w)]++
	}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(start) || !d.Before(end) {
				continue
			}
			y, w := d.ISOWeek()
			sold[cellKey(y, w)]++
		}
	}
	keys := make([]string, 0, len(avail))
	for k := range avail {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]WeeklyCell, 0, len(keys))
	for _, k := range keys {
		var y, w int
		fmt.Sscanf(k, "%d-%d", &y, &w)
		out = append(out, WeeklyCell{Year: y, Week: w, NightsSold: sold[k], AvailableN: avail[k]})
	}
	return out, nil
}

type DowRow struct {
	DOW             int // 0 = Sunday … 6 = Saturday
	NightsSold      int
	AvailableNights int
}

func (s *Store) ListDOWOccupancy(ctx context.Context, propertyID int64, fromDate, toDate time.Time) ([]DowRow, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	closedStays, err := s.ListClosedOccupanciesInDateRange(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	loc := fromDate.Location()
	// Per-day flag: true when the day falls inside a closed stay.
	closedDay := map[string]bool{}
	for _, st := range closedStays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(fromDate) || !d.Before(toDate) {
				continue
			}
			closedDay[d.Format("2006-01-02")] = true
		}
	}
	sold := [7]int{}
	avail := [7]int{}
	for d := fromDate; d.Before(toDate); d = d.AddDate(0, 0, 1) {
		if closedDay[d.Format("2006-01-02")] {
			continue
		}
		avail[int(d.Weekday())]++
	}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			if d.Before(fromDate) || !d.Before(toDate) {
				continue
			}
			sold[int(d.Weekday())]++
		}
	}
	out := make([]DowRow, 7)
	for i := 0; i < 7; i++ {
		out[i] = DowRow{DOW: i, NightsSold: sold[i], AvailableNights: avail[i]}
	}
	return out, nil
}

type CancellationRow struct {
	StayID      int64
	StartAt     time.Time
	CancelledAt time.Time
	LeadDays    int
}

// ListCancellationsInArrivalWindow returns cancelled stays whose
// arrival falls in [fromUTC, toUTC).
func (s *Store) ListCancellationsInArrivalWindow(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]CancellationRow, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		rows, err := s.DB.QueryContext(ctx, `
			SELECT id, start_at, last_synced_at
			FROM occupancies
			WHERE property_id = ?
			  AND `+analyticsCancelledStatus+`
			  AND start_at >= ? AND start_at < ?
			ORDER BY start_at ASC`, propertyID, fromUTC.Format(time.RFC3339), toUTC.Format(time.RFC3339))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []CancellationRow
		for rows.Next() {
			var r CancellationRow
			var start, cancelled string
			if err := rows.Scan(&r.StayID, &start, &cancelled); err != nil {
				return nil, err
			}
			r.StartAt, _ = time.Parse(time.RFC3339, start)
			r.CancelledAt, _ = time.Parse(time.RFC3339, cancelled)
			days := int(r.StartAt.Sub(r.CancelledAt).Hours() / 24)
			if days < 0 {
				days = 0
			}
			r.LeadDays = days
			out = append(out, r)
		}
		return out, rows.Err()
	}
	loc := s.propertyLocation(ctx, propertyID)
	fromDate, toDate := dateRangeParams(fromUTC, toUTC, loc)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, check_in_date, updated_at
		FROM named_stays
		WHERE property_id = ?
		  AND status = 'cancelled'
		  AND check_in_date >= ? AND check_in_date < ?
		ORDER BY check_in_date ASC`, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CancellationRow
	for rows.Next() {
		var r CancellationRow
		var start, cancelled string
		if err := rows.Scan(&r.StayID, &start, &cancelled); err != nil {
			return nil, err
		}
		r.StartAt = parsePropertyDate(start, loc)
		r.CancelledAt, _ = time.Parse(time.RFC3339, cancelled)
		days := int(r.StartAt.Sub(r.CancelledAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		r.LeadDays = days
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountActiveArrivalsInWindow — used for cancellation-rate denominator.
func (s *Store) CountActiveArrivalsInWindow(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) (int, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromUTC, toUTC)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, st := range stays {
		if !st.StartAt.Before(fromUTC) && st.StartAt.Before(toUTC) {
			n++
		}
	}
	return n, nil
}

type NetPerStayRow struct {
	StayID                 int64
	StartAt                time.Time
	EndAt                  time.Time
	GuestName              string
	GrossCents             int64
	CommissionCents        int64
	PaymentFeeCents        int64
	CleaningAllocatedCents int64
	NetCents               int64
}

func (s *Store) ListNetPerStay(ctx context.Context, propertyID int64, fromDate, toDate time.Time, loc *time.Location) ([]NetPerStayRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	// Load payouts keyed by named_stay_id.
	type pag struct {
		gross, comm, fees, net int64
	}
	payouts := map[int64]pag{}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT named_stay_id,
			COALESCE(SUM(amount_cents), 0),
			COALESCE(SUM(commission_cents), 0),
			COALESCE(SUM(payment_service_fee_cents), 0),
			COALESCE(SUM(net_cents), 0)
		FROM finance_bookings
		WHERE property_id = ? AND named_stay_id IS NOT NULL
		GROUP BY named_stay_id`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p pag
		if err := rows.Scan(&id, &p.gross, &p.comm, &p.fees, &p.net); err != nil {
			return nil, err
		}
		payouts[id] = p
	}
	manualRows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, COALESCE(ns.manual_revenue_cents, 0)
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.stay_type = 'external'
		  AND ns.manual_revenue_cents IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)`, propertyID)
	if err != nil {
		return nil, err
	}
	defer manualRows.Close()
	for manualRows.Next() {
		var id, amount int64
		if err := manualRows.Scan(&id, &amount); err != nil {
			return nil, err
		}
		payouts[id] = pag{gross: amount, net: amount}
	}
	// Cleaning allocation per checkout day reuses cleaner_fee_history
	// (the fee effective on the checkout date). We compute once per stay.
	out := make([]NetPerStayRow, 0, len(stays))
	for _, st := range stays {
		p := payouts[st.ID]
		// Skip stays with no finance data at all — reporting a negative
		// "net per stay" driven solely by the cleaner fee is misleading
		// (finance import may simply lag the ICS import).
		if p.gross == 0 && p.comm == 0 && p.fees == 0 && p.net == 0 {
			continue
		}
		cleaning, _ := s.cleanerFeeOnDate(ctx, propertyID, st.EndAt.In(loc))
		row := NetPerStayRow{
			StayID: st.ID, StartAt: st.StartAt, EndAt: st.EndAt, GuestName: st.GuestName,
			GrossCents: p.gross, CommissionCents: p.comm, PaymentFeeCents: p.fees,
			CleaningAllocatedCents: cleaning,
			NetCents:               p.net - cleaning,
		}
		out = append(out, row)
	}
	return out, nil
}

// cleanerFeeOnDate returns the cleaning fee effective on the supplied
// date. Uses cleaner_fee_history.effective_from <= date, most recent
// wins. Returns 0 if nothing is configured.
func (s *Store) cleanerFeeOnDate(ctx context.Context, propertyID int64, date time.Time) (int64, error) {
	var cleaning int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(cleaning_fee_amount_cents, 0) + COALESCE(washing_fee_amount_cents, 0)
		FROM cleaner_fee_history
		WHERE property_id = ? AND effective_from <= ?
		ORDER BY effective_from DESC LIMIT 1`,
		propertyID, date.Format(time.RFC3339)).Scan(&cleaning)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return cleaning, nil
}

// YearlyFinanceRollup mirrors the deprecated /finance/summary
// yearly_* fields for use inside analytics endpoints.
type YearlyFinanceRollupRow struct {
	IncomingCents int64
	OutgoingCents int64
	NetCents      int64
}

func (s *Store) YearlyFinanceRollup(ctx context.Context, propertyID int64, year int) (*YearlyFinanceRollupRow, error) {
	yearPrefix := fmt.Sprintf("%04d", year)
	var out YearlyFinanceRollupRow
	err := s.DB.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'incoming' THEN amount_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'outgoing' THEN amount_cents ELSE 0 END), 0)
		FROM finance_transactions
		WHERE property_id = ? AND substr(transaction_date, 1, 4) = ?`,
		propertyID, yearPrefix).Scan(&out.IncomingCents, &out.OutgoingCents)
	if err != nil {
		return nil, err
	}
	out.NetCents = out.IncomingCents - out.OutgoingCents
	return &out, nil
}

// ---------------- A4: Demand primitives ----------------

type LeadBucket struct {
	Bucket string
	Count  int
}

func leadBucketFor(days int) string {
	switch {
	case days <= 3:
		return "0-3"
	case days <= 14:
		return "4-14"
	case days <= 45:
		return "15-45"
	case days <= 90:
		return "46-90"
	default:
		return "91+"
	}
}

var leadBucketOrder = []string{"0-3", "4-14", "15-45", "46-90", "91+"}

func (s *Store) ListLeadTimeBuckets(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]LeadBucket, error) {
	m := map[string]int{}
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromUTC, toUTC)
	if err != nil {
		return nil, err
	}
	for _, st := range stays {
		sa := st.StartAt
		ia := st.ImportedAt
		days := int(sa.Sub(ia).Hours() / 24)
		if days < 0 {
			days = 0
		}
		m[leadBucketFor(days)]++
	}
	out := make([]LeadBucket, 0, len(leadBucketOrder))
	for _, b := range leadBucketOrder {
		out = append(out, LeadBucket{Bucket: b, Count: m[b]})
	}
	return out, nil
}

type LOSBucket struct {
	Bucket string
	Count  int
}

var losBucketOrder = []string{"1", "2", "3", "4-5", "6-7", "8-14", "15+"}

func losBucketFor(nights int) string {
	switch {
	case nights <= 1:
		return "1"
	case nights == 2:
		return "2"
	case nights == 3:
		return "3"
	case nights <= 5:
		return "4-5"
	case nights <= 7:
		return "6-7"
	case nights <= 14:
		return "8-14"
	default:
		return "15+"
	}
}

func (s *Store) ListLengthOfStayBuckets(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]LOSBucket, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromUTC, toUTC)
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	for _, st := range stays {
		nights := int(toDateStart(st.EndAt, time.UTC).Sub(toDateStart(st.StartAt, time.UTC)).Hours()/24 + 0.5)
		if nights < 1 {
			continue
		}
		m[losBucketFor(nights)]++
	}
	out := make([]LOSBucket, 0, len(losBucketOrder))
	for _, b := range losBucketOrder {
		out = append(out, LOSBucket{Bucket: b, Count: m[b]})
	}
	return out, nil
}

type ADRDimensionRow struct {
	Bucket        string
	GrossCents    int64
	MatchedNights int
}

// ADRByDimension — dim ∈ {"month","dow","lead_bucket"}.
func (s *Store) ADRByDimension(ctx context.Context, propertyID int64, fromDate, toDate time.Time, dim string, loc *time.Location) ([]ADRDimensionRow, error) {
	if loc == nil {
		loc = time.UTC
	}
	// Load confirmed payouts with linked stay IDs.
	rows, err := s.DB.QueryContext(ctx, `
		SELECT named_stay_id, check_in_date, COALESCE(amount_cents, 0)
		FROM finance_bookings
		WHERE property_id = ? AND named_stay_id IS NOT NULL
		  AND check_in_date IS NOT NULL
		  AND check_in_date >= ? AND check_in_date < ?`,
		propertyID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matchedGross := map[int64]int64{}
	checkin := map[int64]string{}
	for rows.Next() {
		var stayID sql.NullInt64
		var d string
		var g int64
		if err := rows.Scan(&stayID, &d, &g); err != nil {
			return nil, err
		}
		if !stayID.Valid {
			continue
		}
		matchedGross[stayID.Int64] += g
		checkin[stayID.Int64] = d
	}
	manualRows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.check_in_date, COALESCE(ns.manual_revenue_cents, 0)
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.stay_type = 'external'
		  AND ns.manual_revenue_cents IS NOT NULL
		  AND ns.check_in_date >= ? AND ns.check_in_date < ?
		  AND NOT EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)`,
		propertyID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer manualRows.Close()
	for manualRows.Next() {
		var id, g int64
		var d string
		if err := manualRows.Scan(&id, &d, &g); err != nil {
			return nil, err
		}
		matchedGross[id] += g
		checkin[id] = d
	}
	// Load those stays.
	ids := make([]int64, 0, len(matchedGross))
	for id := range matchedGross {
		ids = append(ids, id)
	}
	stays, err := s.listOccupanciesByIDs(ctx, propertyID, ids)
	if err != nil {
		return nil, err
	}
	agg := map[string]*ADRDimensionRow{}
	get := func(k string) *ADRDimensionRow {
		if v, ok := agg[k]; ok {
			return v
		}
		v := &ADRDimensionRow{Bucket: k}
		agg[k] = v
		return v
	}
	for _, st := range stays {
		nights := int(toDateStart(st.EndAt, loc).Sub(toDateStart(st.StartAt, loc)).Hours()/24 + 0.5)
		if nights < 1 {
			continue
		}
		gross := matchedGross[st.ID]
		switch dim {
		case "month":
			k := st.StartAt.In(loc).Format("2006-01")
			r := get(k)
			r.GrossCents += gross
			r.MatchedNights += nights
		case "dow":
			// Spread the gross proportionally across weekday nights.
			for d := toDateStart(st.StartAt, loc); d.Before(toDateStart(st.EndAt, loc)); d = d.AddDate(0, 0, 1) {
				k := fmt.Sprintf("%d", int(d.Weekday()))
				r := get(k)
				r.GrossCents += gross / int64(nights)
				r.MatchedNights++
			}
		case "lead_bucket":
			days := int(st.StartAt.Sub(st.ImportedAt).Hours() / 24)
			if days < 0 {
				days = 0
			}
			k := leadBucketFor(days)
			r := get(k)
			r.GrossCents += gross
			r.MatchedNights += nights
		default:
			return nil, fmt.Errorf("unsupported dim %q", dim)
		}
	}
	out := make([]ADRDimensionRow, 0, len(agg))
	for _, v := range agg {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bucket < out[j].Bucket })
	return out, nil
}

type GapNightRow struct {
	Date             string
	PrevStayID       int64
	NextStayID       int64
	PrevCheckoutDate string
	NextCheckinDate  string
}

// ListGapNights — single available nights sandwiched between two
// active stays. Same-day checkout/check-in yields no gap.
func (s *Store) ListGapNights(ctx context.Context, propertyID int64, fromDate, toDate time.Time) ([]GapNightRow, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate.AddDate(0, 0, -30), toDate.AddDate(0, 0, 30))
	if err != nil {
		return nil, err
	}
	loc := fromDate.Location()
	sort.Slice(stays, func(i, j int) bool { return stays[i].StartAt.Before(stays[j].StartAt) })
	var out []GapNightRow
	for i := 0; i < len(stays)-1; i++ {
		a := stays[i]
		b := stays[i+1]
		aEnd := toDateStart(a.EndAt, loc)
		bStart := toDateStart(b.StartAt, loc)
		diff := int(bStart.Sub(aEnd).Hours()/24 + 0.5)
		if diff != 1 {
			continue // zero-gap or multi-night gap — not a single gap night
		}
		d := aEnd
		if d.Before(fromDate) || !d.Before(toDate) {
			continue
		}
		out = append(out, GapNightRow{
			Date:             d.Format("2006-01-02"),
			PrevStayID:       a.ID,
			NextStayID:       b.ID,
			PrevCheckoutDate: aEnd.Format("2006-01-02"),
			NextCheckinDate:  bStart.Format("2006-01-02"),
		})
	}
	return out, nil
}

// ListOrphanMidweek — 1-2 consecutive Mon-Thu unsold nights wrapped
// by a booked weekend on both sides.
func (s *Store) ListOrphanMidweek(ctx context.Context, propertyID int64, fromDate, toDate time.Time) ([]GapNightRow, error) {
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, fromDate.AddDate(0, 0, -7), toDate.AddDate(0, 0, 7))
	if err != nil {
		return nil, err
	}
	loc := fromDate.Location()
	occupied := map[string]int64{}
	for _, st := range stays {
		sd := toDateStart(st.StartAt, loc)
		ed := toDateStart(st.EndAt, loc)
		for d := sd; d.Before(ed); d = d.AddDate(0, 0, 1) {
			occupied[d.Format("2006-01-02")] = st.ID
		}
	}
	isOccupied := func(d time.Time) (int64, bool) {
		id, ok := occupied[d.Format("2006-01-02")]
		return id, ok
	}
	var out []GapNightRow
	d := fromDate
	for d.Before(toDate) {
		wd := d.Weekday()
		if _, booked := isOccupied(d); booked || wd == time.Saturday || wd == time.Sunday || wd == time.Friday {
			d = d.AddDate(0, 0, 1)
			continue
		}
		// found a free Mon-Thu day. Walk forward while free and still Mon-Thu.
		streakStart := d
		streakEnd := d
		for streakEnd.Before(toDate) {
			w := streakEnd.Weekday()
			if _, booked := isOccupied(streakEnd); booked || w == time.Friday || w == time.Saturday || w == time.Sunday {
				break
			}
			streakEnd = streakEnd.AddDate(0, 0, 1)
		}
		length := int(streakEnd.Sub(streakStart).Hours() / 24)
		if length >= 1 && length <= 2 {
			// Check weekend-before booked and weekend-after booked.
			prev := streakStart.AddDate(0, 0, -1)
			next := streakEnd
			_, prevBooked := isOccupied(prev)
			_, nextBooked := isOccupied(next)
			if prevBooked && nextBooked {
				for x := streakStart; x.Before(streakEnd); x = x.AddDate(0, 0, 1) {
					row := GapNightRow{
						Date:             x.Format("2006-01-02"),
						PrevCheckoutDate: streakStart.Format("2006-01-02"),
						NextCheckinDate:  streakEnd.Format("2006-01-02"),
					}
					if id, ok := isOccupied(prev); ok {
						row.PrevStayID = id
					}
					if id, ok := isOccupied(next); ok {
						row.NextStayID = id
					}
					out = append(out, row)
				}
			}
		}
		d = streakEnd.AddDate(0, 0, 1)
	}
	return out, nil
}

// PaceCurveRow — for each T in [0,180], number of bookings with
// arrival in the target window that were already known at T days
// before windowStart.
type PaceCurveRow struct {
	DaysBefore int
	Count      int
}

func (s *Store) PaceCurveForWindow(ctx context.Context, propertyID int64, windowStart, windowEnd time.Time) ([]PaceCurveRow, []PaceCurveRow, error) {
	thisYear, err := s.paceCurve(ctx, propertyID, windowStart, windowEnd)
	if err != nil {
		return nil, nil, err
	}
	lyStart := windowStart.AddDate(-1, 0, 0)
	lyEnd := windowEnd.AddDate(-1, 0, 0)
	lastYear, err := s.paceCurve(ctx, propertyID, lyStart, lyEnd)
	if err != nil {
		return nil, nil, err
	}
	return thisYear, lastYear, nil
}

func (s *Store) paceCurve(ctx context.Context, propertyID int64, windowStart, windowEnd time.Time) ([]PaceCurveRow, error) {
	imported := []time.Time{}
	stays, err := s.ListActiveOccupanciesInDateRange(ctx, propertyID, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	for _, st := range stays {
		imported = append(imported, st.ImportedAt)
	}
	out := make([]PaceCurveRow, 0, 181)
	for T := 180; T >= 0; T-- {
		cutoff := windowStart.AddDate(0, 0, -T)
		count := 0
		for _, t := range imported {
			if !t.After(cutoff) {
				count++
			}
		}
		out = append(out, PaceCurveRow{DaysBefore: T, Count: count})
	}
	return out, nil
}

type ReturningGuestRow struct {
	NormalizedName string
	DisplayName    string
	StayCount      int
	FirstStay      time.Time
	LastStay       time.Time
}

// ListReturningGuests groups matched active stays in [from,to) by
// normalized guest name (≥6 chars) and returns one row per guest
// with 2+ stays seen in history at the property.
func (s *Store) ListReturningGuests(ctx context.Context, propertyID int64, fromDate, toDate time.Time, limit, offset int) ([]ReturningGuestRow, int, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyListReturningGuests(ctx, propertyID, fromDate, toDate, limit, offset)
	}
	loc := s.propertyLocation(ctx, propertyID)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.check_in_date, ns.display_name
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.display_name <> ''
		  AND (
		    ns.stay_type = 'booking_com'
		    OR (
		      ns.stay_type = 'external'
		      AND (
		        ns.manual_revenue_cents IS NOT NULL
		        OR EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)
		      )
		    )
		  )
		ORDER BY ns.check_in_date ASC, ns.id ASC`, propertyID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type stayInfo struct {
		id      int64
		start   time.Time
		display string
	}
	byNorm := map[string][]stayInfo{}
	displayByNorm := map[string]string{}
	for rows.Next() {
		var id int64
		var start, name string
		if err := rows.Scan(&id, &start, &name); err != nil {
			return nil, 0, err
		}
		n := NormalizeGuestName(name)
		if len([]rune(n)) < 6 {
			continue
		}
		t := parsePropertyDate(start, loc)
		byNorm[n] = append(byNorm[n], stayInfo{id: id, start: t, display: name})
		if _, ok := displayByNorm[n]; !ok {
			displayByNorm[n] = name
		}
	}
	all := make([]ReturningGuestRow, 0, len(byNorm))
	for n, infos := range byNorm {
		if len(infos) < 2 {
			continue
		}
		// Only emit if at least one stay falls in [from,to).
		inWindow := false
		for _, i := range infos {
			if !i.start.Before(fromDate) && i.start.Before(toDate) {
				inWindow = true
				break
			}
		}
		if !inWindow {
			continue
		}
		row := ReturningGuestRow{
			NormalizedName: n,
			DisplayName:    displayByNorm[n],
			StayCount:      len(infos),
			FirstStay:      infos[0].start,
			LastStay:       infos[len(infos)-1].start,
		}
		all = append(all, row)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].StayCount != all[j].StayCount {
			return all[i].StayCount > all[j].StayCount
		}
		return all[i].NormalizedName < all[j].NormalizedName
	})
	total := len(all)
	if offset >= total {
		return []ReturningGuestRow{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *Store) legacyListReturningGuests(ctx context.Context, propertyID int64, fromDate, toDate time.Time, limit, offset int) ([]ReturningGuestRow, int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, start_at, COALESCE(guest_display_name, '')
		FROM occupancies
		WHERE property_id = ? AND `+analyticsActiveStatus+`
		  AND guest_display_name IS NOT NULL AND guest_display_name <> ''
		ORDER BY start_at ASC`, propertyID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type stayInfo struct {
		id      int64
		start   time.Time
		display string
	}
	byNorm := map[string][]stayInfo{}
	displayByNorm := map[string]string{}
	for rows.Next() {
		var id int64
		var start, name string
		if err := rows.Scan(&id, &start, &name); err != nil {
			return nil, 0, err
		}
		n := NormalizeGuestName(name)
		if len([]rune(n)) < 6 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, start)
		byNorm[n] = append(byNorm[n], stayInfo{id: id, start: t, display: name})
		if _, ok := displayByNorm[n]; !ok {
			displayByNorm[n] = name
		}
	}
	all := make([]ReturningGuestRow, 0, len(byNorm))
	for n, infos := range byNorm {
		if len(infos) < 2 {
			continue
		}
		inWindow := false
		for _, i := range infos {
			if !i.start.Before(fromDate) && i.start.Before(toDate) {
				inWindow = true
				break
			}
		}
		if !inWindow {
			continue
		}
		all = append(all, ReturningGuestRow{NormalizedName: n, DisplayName: displayByNorm[n], StayCount: len(infos), FirstStay: infos[0].start, LastStay: infos[len(infos)-1].start})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].StayCount != all[j].StayCount {
			return all[i].StayCount > all[j].StayCount
		}
		return all[i].NormalizedName < all[j].NormalizedName
	})
	total := len(all)
	if offset >= total {
		return []ReturningGuestRow{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// ReturningGuestCount — number of active stays in [from,to) whose
// normalized guest name appears on an earlier stay at this property.
func (s *Store) ReturningGuestCount(ctx context.Context, propertyID int64, fromDate, toDate time.Time) (int, int, error) {
	if !s.propertyHasNamedStays(ctx, propertyID) {
		return s.legacyReturningGuestCount(ctx, propertyID, fromDate, toDate)
	}
	loc := s.propertyLocation(ctx, propertyID)
	fromKey, toKey := dateRangeParams(fromDate, toDate, loc)
	var total int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.check_in_date >= ? AND ns.check_in_date < ?
		  AND (
		    ns.stay_type = 'booking_com'
		    OR (
		      ns.stay_type = 'external'
		      AND (
		        ns.manual_revenue_cents IS NOT NULL
		        OR EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)
		      )
		    )
		  )`, propertyID, fromKey, toKey).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.check_in_date, ns.display_name
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.display_name <> ''
		  AND (
		    ns.stay_type = 'booking_com'
		    OR (
		      ns.stay_type = 'external'
		      AND (
		        ns.manual_revenue_cents IS NOT NULL
		        OR EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id)
		      )
		    )
		  )
		ORDER BY ns.check_in_date ASC, ns.id ASC`, propertyID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	firstSeen := map[string]bool{}
	returning := 0
	for rows.Next() {
		var start, name string
		if err := rows.Scan(&start, &name); err != nil {
			return 0, 0, err
		}
		n := NormalizeGuestName(name)
		if len([]rune(n)) < 6 {
			continue
		}
		t := parsePropertyDate(start, loc)
		if firstSeen[n] {
			if !t.Before(fromDate) && t.Before(toDate) {
				returning++
			}
		}
		firstSeen[n] = true
	}
	return returning, total, rows.Err()
}

func (s *Store) legacyReturningGuestCount(ctx context.Context, propertyID int64, fromDate, toDate time.Time) (int, int, error) {
	var total int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM occupancies
		WHERE property_id = ? AND `+analyticsActiveStatus+`
		  AND start_at >= ? AND start_at < ?`, propertyID, fromDate.Format(time.RFC3339), toDate.Format(time.RFC3339)).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT start_at, COALESCE(guest_display_name, '')
		FROM occupancies
		WHERE property_id = ? AND `+analyticsActiveStatus+`
		  AND guest_display_name IS NOT NULL AND guest_display_name <> ''
		ORDER BY start_at ASC`, propertyID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	firstSeen := map[string]bool{}
	returning := 0
	for rows.Next() {
		var start, name string
		if err := rows.Scan(&start, &name); err != nil {
			return 0, 0, err
		}
		n := NormalizeGuestName(name)
		if len([]rune(n)) < 6 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, start)
		if firstSeen[n] && !t.Before(fromDate) && t.Before(toDate) {
			returning++
		}
		firstSeen[n] = true
	}
	return returning, total, rows.Err()
}
