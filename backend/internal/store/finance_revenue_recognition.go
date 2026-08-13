package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type FinanceRevenueRecognitionBooking struct {
	BookingID            int64
	ReferenceNumber      string
	GuestName            string
	CheckInDate          string
	CheckOutDate         string
	GrossCents           int
	StayNights           int
	RecognizedNights     int
	RecognizedGrossCents int
	Unmatched            bool
	Cancelled            bool
	NoShow               bool
}

type FinanceRevenueRecognitionIssue struct {
	BookingID       int64
	ReferenceNumber string
	GuestName       string
	CheckInDate     string
	CheckOutDate    string
	Reason          string
}

type FinanceRevenueRecognitionReport struct {
	Month             string
	GrossRevenueCents int
	Bookings          []FinanceRevenueRecognitionBooking
	ExcludedBookings  []FinanceRevenueRecognitionIssue
}

// ComputeFinanceRevenueRecognition allocates payout-backed gross booking
// revenue over property-local stay nights. It does not alter the cash ledger.
func (s *Store) ComputeFinanceRevenueRecognition(ctx context.Context, propertyID int64, month string, loc *time.Location) (*FinanceRevenueRecognitionReport, error) {
	if loc == nil {
		loc = time.UTC
	}
	monthStart, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil || monthStart.Format("2006-01") != month {
		return nil, fmt.Errorf("month must be YYYY-MM")
	}
	monthEnd := monthStart.AddDate(0, 1, 0)
	report := &FinanceRevenueRecognitionReport{
		Month:            month,
		Bookings:         []FinanceRevenueRecognitionBooking{},
		ExcludedBookings: []FinanceRevenueRecognitionIssue{},
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT fb.id, fb.reference_number, COALESCE(fb.guest_name, ''),
		       COALESCE(fb.check_in_date, ''), COALESCE(fb.check_out_date, ''),
		       COALESCE(fb.amount_cents, 0), fb.named_stay_id,
		       COALESCE(fb.status, ''), COALESCE(fb.reservation_status, ''),
		       COALESCE(fb.outcome_override, ns.stay_outcome, '')
		FROM finance_bookings fb
		LEFT JOIN named_stays ns ON ns.id = fb.named_stay_id AND ns.property_id = fb.property_id
		WHERE fb.property_id = ? AND fb.has_payout_data = 1
		ORDER BY fb.check_in_date DESC, fb.id DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			bookingID            int64
			reference, guestName string
			checkIn, checkOut    string
			grossCents           int
			namedStayID          sql.NullInt64
			status               string
			reservationStatus    string
			outcome              string
		)
		if err := rows.Scan(&bookingID, &reference, &guestName, &checkIn, &checkOut, &grossCents, &namedStayID, &status, &reservationStatus, &outcome); err != nil {
			return nil, err
		}

		stayStart, stayEnd, reason := parseFinanceStayWindow(checkIn, checkOut, loc)
		if reason != "" {
			report.ExcludedBookings = append(report.ExcludedBookings, FinanceRevenueRecognitionIssue{
				BookingID:       bookingID,
				ReferenceNumber: reference,
				GuestName:       guestName,
				CheckInDate:     checkIn,
				CheckOutDate:    checkOut,
				Reason:          reason,
			})
			continue
		}
		canonicalStatus := status
		if strings.TrimSpace(canonicalStatus) == "" {
			canonicalStatus = reservationStatus
		}
		canonicalStatus = normalizeFinanceBookingStatus(canonicalStatus)
		canonicalOutcome := normalizeFinanceBookingStatus(outcome)
		cancelled := strings.HasPrefix(canonicalStatus, "CANCEL") || canonicalOutcome == "CANCELLED_NON_REFUNDABLE"
		noShow := canonicalStatus == "NO_SHOW" || canonicalStatus == "NOSHOW" || canonicalOutcome == "NO_SHOW"
		stayNights := financeCalendarDays(stayStart, stayEnd)

		if cancelled || noShow {
			if stayStart.Format("2006-01") != month {
				continue
			}
			report.Bookings = append(report.Bookings, FinanceRevenueRecognitionBooking{
				BookingID:            bookingID,
				ReferenceNumber:      reference,
				GuestName:            guestName,
				CheckInDate:          checkIn,
				CheckOutDate:         checkOut,
				GrossCents:           grossCents,
				StayNights:           stayNights,
				RecognizedGrossCents: grossCents,
				Unmatched:            !namedStayID.Valid,
				Cancelled:            cancelled,
				NoShow:               noShow,
			})
			report.GrossRevenueCents += grossCents
			continue
		}

		overlapStart := stayStart
		if overlapStart.Before(monthStart) {
			overlapStart = monthStart
		}
		overlapEnd := stayEnd
		if overlapEnd.After(monthEnd) {
			overlapEnd = monthEnd
		}
		if !overlapStart.Before(overlapEnd) {
			continue
		}

		startOffset := financeCalendarDays(stayStart, overlapStart)
		endOffset := financeCalendarDays(stayStart, overlapEnd)
		recognizedGross := allocateFinanceGross(grossCents, stayNights, startOffset, endOffset)

		report.Bookings = append(report.Bookings, FinanceRevenueRecognitionBooking{
			BookingID:            bookingID,
			ReferenceNumber:      reference,
			GuestName:            guestName,
			CheckInDate:          checkIn,
			CheckOutDate:         checkOut,
			GrossCents:           grossCents,
			StayNights:           stayNights,
			RecognizedNights:     endOffset - startOffset,
			RecognizedGrossCents: recognizedGross,
			Unmatched:            !namedStayID.Valid,
			Cancelled:            false,
			NoShow:               false,
		})
		report.GrossRevenueCents += recognizedGross
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return report, nil
}

func parseFinanceStayWindow(checkIn, checkOut string, loc *time.Location) (time.Time, time.Time, string) {
	checkIn = strings.TrimSpace(checkIn)
	checkOut = strings.TrimSpace(checkOut)
	if checkIn == "" {
		return time.Time{}, time.Time{}, "missing_check_in"
	}
	if checkOut == "" {
		return time.Time{}, time.Time{}, "missing_check_out"
	}
	start, err := time.ParseInLocation("2006-01-02", checkIn, loc)
	if err != nil || start.Format("2006-01-02") != checkIn {
		return time.Time{}, time.Time{}, "invalid_check_in"
	}
	end, err := time.ParseInLocation("2006-01-02", checkOut, loc)
	if err != nil || end.Format("2006-01-02") != checkOut {
		return time.Time{}, time.Time{}, "invalid_check_out"
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, "invalid_stay_window"
	}
	return start, end, ""
}

func financeCalendarDays(start, end time.Time) int {
	days := 0
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		days++
	}
	return days
}

// allocateFinanceGross splits cents evenly by night and assigns any remainder
// to the earliest nights, while allowing a month to request a contiguous slice.
func allocateFinanceGross(grossCents, totalNights, startOffset, endOffset int) int {
	if totalNights <= 0 || startOffset < 0 || endOffset <= startOffset || endOffset > totalNights {
		return 0
	}
	base := grossCents / totalNights
	remainder := grossCents % totalNights
	amount := base * (endOffset - startOffset)
	remainderNights := remainder
	if remainderNights < 0 {
		remainderNights = -remainderNights
	}
	extraStart := startOffset
	if extraStart < 0 {
		extraStart = 0
	}
	extraEnd := endOffset
	if extraEnd > remainderNights {
		extraEnd = remainderNights
	}
	if extraEnd > extraStart {
		extra := extraEnd - extraStart
		if remainder < 0 {
			amount -= extra
		} else {
			amount += extra
		}
	}
	return amount
}

func normalizeFinanceBookingStatus(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", "_", " ", "_")
	return replacer.Replace(value)
}
