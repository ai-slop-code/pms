package store

import (
	"context"
	"time"
)

// nightsUTC returns the civil night labels covered by an all-day ICS event.
func nightsUTC(start, end time.Time) []string {
	start = toUTCMidnight(start)
	end = toUTCMidnight(end)
	var out []string
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		out = append(out, date.Format("2006-01-02"))
	}
	return out
}

// OccupancyMetricNights returns final-model unavailable and guest nights for
// [fromDate, toDate). Both values include only sold-eligible named-stay nights.
func (s *Store) OccupancyMetricNights(ctx context.Context, propertyID int64, fromDate, toDate string) (availability, guest int, err error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT n.local_night_date,
		       ns.stay_type,
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
	availabilityDates := map[string]struct{}{}
	guestDates := map[string]struct{}{}
	for rows.Next() {
		var night, stayType, reviewStatus, stayOutcome string
		var hasRevenue int
		if err := rows.Scan(&night, &stayType, &reviewStatus, &hasRevenue, &stayOutcome); err != nil {
			return 0, 0, err
		}
		soldOutcome := stayOutcome == StayOutcomeCancelledNonRefundable || stayOutcome == StayOutcomeNoShow
		if reviewStatus == "confirmed" && (soldOutcome || stayOutcome == "") && (stayType == StayTypeBookingCom || (stayType == StayTypeExternal && hasRevenue == 1)) {
			availabilityDates[night] = struct{}{}
			guestDates[night] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return len(availabilityDates), len(guestDates), nil
}
