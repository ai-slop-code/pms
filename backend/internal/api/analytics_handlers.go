package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

// ---------------- Response types ----------------

type analyticsFreshnessResponse struct {
	GeneratedAt           time.Time `json:"generated_at"`
	LastICSSyncAt         *string   `json:"last_ics_sync_at,omitempty"`
	LastPayoutDate        *string   `json:"last_payout_date,omitempty"`
	UnmatchedPayoutsCount int       `json:"unmatched_payouts_count"`
	StalenessLevel        string    `json:"staleness_level"`
}

type analyticsKPIWindow struct {
	Days             int   `json:"days"`
	NightsSold       int   `json:"nights_sold"`
	AvailableNights  int   `json:"available_nights"`
	ConfirmedCents   int64 `json:"confirmed_cents"`
	EstimatedCents   int64 `json:"estimated_cents"`
	TotalRevenueCents int64 `json:"total_revenue_cents"`
}

type analyticsOutlookResponse struct {
	GeneratedAt    time.Time                `json:"generated_at"`
	Windows        []analyticsKPIWindow     `json:"windows"`
	PacingSeries   []analyticsPacePoint     `json:"pacing_series"`
	UnsoldNights   []analyticsUnsoldNight   `json:"unsold_nights"`
	NewBookings    []analyticsCountByDay    `json:"new_bookings"`
	RevenueAsOf    *string                  `json:"revenue_as_of,omitempty"`
	TrailingADR    int64                    `json:"trailing_adr_cents"`
}

type analyticsPacePoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type analyticsUnsoldNight struct {
	Date       string `json:"date"`
	PrevGuest  string `json:"prev_guest,omitempty"`
	NextGuest  string `json:"next_guest,omitempty"`
}

type analyticsCountByDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type analyticsPerformanceKPIs struct {
	NightsSold              int     `json:"nights_sold"`
	AvailableNights         int     `json:"available_nights"`
	OccupancyRate           float64 `json:"occupancy_rate"`
	ADRCents                int64   `json:"adr_cents"`
	RevPARCents             int64   `json:"revpar_cents"`
	GrossCents              int64   `json:"gross_cents"`
	NetCents                int64   `json:"net_cents"`
	CommissionCents         int64   `json:"commission_cents"`
	PaymentFeesCents        int64   `json:"payment_fees_cents"`
	EffectiveTakeRate       float64 `json:"effective_take_rate"`
	MatchedNights           int     `json:"matched_nights"`
}

type analyticsMonthlyTrendRow struct {
	Month           string  `json:"month"`
	OccupancyRate   float64 `json:"occupancy_rate"`
	ADRCents        int64   `json:"adr_cents"`
	GrossCents      int64   `json:"gross_cents"`
	NightsSold      int     `json:"nights_sold"`
	AvailableNights int     `json:"available_nights"`
}

type analyticsHeatmapCell struct {
	Year          int     `json:"year"`
	Week          int     `json:"week"`
	OccupancyRate float64 `json:"occupancy_rate"`
}

type analyticsDOWCell struct {
	DOW             int     `json:"dow"`
	NightsSold      int     `json:"nights_sold"`
	AvailableNights int     `json:"available_nights"`
	OccupancyRate   float64 `json:"occupancy_rate"`
}

type analyticsCancellationStat struct {
	Rate      float64            `json:"rate"`
	Buckets   []analyticsBucket  `json:"buckets"`
	Total     int                `json:"total_active_plus_cancelled"`
	Cancelled int                `json:"total_cancelled"`
}

type analyticsBucket struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

type analyticsNetPerStayRow struct {
	StayID                 int64  `json:"stay_id"`
	StartAt                string `json:"start_at"`
	EndAt                  string `json:"end_at"`
	GuestName              string `json:"guest_name"`
	GrossCents             int64  `json:"gross_cents"`
	CommissionCents        int64  `json:"commission_cents"`
	PaymentFeeCents        int64  `json:"payment_fee_cents"`
	CleaningAllocatedCents int64  `json:"cleaning_allocated_cents"`
	NetCents               int64  `json:"net_cents"`
}

type analyticsYearlyCleaningRow struct {
	Month int `json:"month"`
	Count int `json:"count"`
}

type analyticsYearlyCleaningBlock struct {
	Year   int                          `json:"year"`
	Series []analyticsYearlyCleaningRow `json:"series"`
}

type analyticsYearlyFinanceBlock struct {
	Year          int   `json:"year"`
	IncomingCents int64 `json:"incoming_cents"`
	OutgoingCents int64 `json:"outgoing_cents"`
	NetCents      int64 `json:"net_cents"`
}

type analyticsPerformanceResponse struct {
	GeneratedAt     time.Time                   `json:"generated_at"`
	From            string                      `json:"from"`
	To              string                      `json:"to"`
	KPIs            analyticsPerformanceKPIs    `json:"kpis"`
	PriorKPIs       *analyticsPerformanceKPIs   `json:"prior_kpis,omitempty"`
	MonthlyTrend    []analyticsMonthlyTrendRow  `json:"monthly_trend"`
	SeasonalityHeatmap []analyticsHeatmapCell   `json:"seasonality_heatmap"`
	DOWOccupancy    []analyticsDOWCell          `json:"dow_occupancy"`
	Cancellation    analyticsCancellationStat   `json:"cancellation"`
	NetPerStay      []analyticsNetPerStayRow    `json:"net_per_stay"`
	YearlyCleaning  analyticsYearlyCleaningBlock `json:"yearly_cleaning"`
	YearlyFinance   analyticsYearlyFinanceBlock `json:"yearly_finance"`
	RevenueAsOf     *string                     `json:"revenue_as_of,omitempty"`
}

type analyticsDemandResponse struct {
	GeneratedAt     time.Time              `json:"generated_at"`
	From            string                 `json:"from"`
	To              string                 `json:"to"`
	LeadTime        []analyticsBucket      `json:"lead_time"`
	LengthOfStay    []analyticsBucket      `json:"length_of_stay"`
	ADRByMonth      []analyticsADRRow      `json:"adr_by_month"`
	ADRByDOW        []analyticsADRRow      `json:"adr_by_dow"`
	ADRByLeadBucket []analyticsADRRow      `json:"adr_by_lead_bucket"`
	GapNights       []analyticsGapRow      `json:"gap_nights"`
	OrphanMidweek   []analyticsGapRow      `json:"orphan_midweek"`
	ReturningGuests analyticsReturningStat `json:"returning_guests"`
}

type analyticsADRRow struct {
	Bucket        string `json:"bucket"`
	ADRCents      int64  `json:"adr_cents"`
	MatchedNights int    `json:"matched_nights"`
}

type analyticsGapRow struct {
	Date             string `json:"date"`
	PrevStayID       int64  `json:"prev_stay_id,omitempty"`
	NextStayID       int64  `json:"next_stay_id,omitempty"`
	PrevCheckoutDate string `json:"prev_checkout_date,omitempty"`
	NextCheckinDate  string `json:"next_checkin_date,omitempty"`
}

type analyticsReturningStat struct {
	TotalActive    int     `json:"total_active"`
	Returning      int     `json:"returning"`
	ReturningRate  float64 `json:"returning_rate"`
}

type analyticsReturningGuestRow struct {
	DisplayName string `json:"display_name"`
	Normalized  string `json:"normalized"`
	StayCount   int    `json:"stay_count"`
	FirstStay   string `json:"first_stay"`
	LastStay    string `json:"last_stay"`
}

type analyticsReturningGuestsResponse struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	Total       int                          `json:"total"`
	Limit       int                          `json:"limit"`
	Offset      int                          `json:"offset"`
	Guests      []analyticsReturningGuestRow `json:"guests"`
}

type analyticsPaceResponse struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Window      string               `json:"window"`
	ThisYear    []analyticsPacePoint `json:"this_year"`
	LastYear    []analyticsPacePoint `json:"last_year,omitempty"`
	LYAvailable bool                 `json:"ly_available"`
}

// ---------------- Helpers ----------------

func (s *Server) analyticsLocation(r *http.Request, pid int64) *time.Location {
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil || prop == nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

func parseDateParam(q string, fallback time.Time, loc *time.Location) time.Time {
	if q == "" {
		return fallback
	}
	t, err := time.ParseInLocation("2006-01-02", q, loc)
	if err != nil {
		return fallback
	}
	return t
}

func formatDayInLoc(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02")
}

func safeDiv(num, den float64) float64 {
	if den == 0 {
		return 0
	}
	v := num / den
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// ---------------- Handlers ----------------

func (s *Server) getAnalyticsFreshness(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	f, err := s.Store.GetAnalyticsFreshness(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	now := time.Now().In(loc)
	level := "stale"
	if f.LastPayoutDate != nil {
		age := now.Sub(*f.LastPayoutDate) / (24 * time.Hour)
		switch {
		case age <= 45:
			level = "ok"
		case age <= 75:
			level = "warn"
		default:
			level = "stale"
		}
	}
	out := analyticsFreshnessResponse{
		GeneratedAt:           now.UTC(),
		UnmatchedPayoutsCount: f.UnmatchedPayoutsCount,
		StalenessLevel:        level,
	}
	if f.LastICSSyncAt != nil {
		ts := f.LastICSSyncAt.UTC().Format(time.RFC3339)
		out.LastICSSyncAt = &ts
	}
	if f.LastPayoutDate != nil {
		d := f.LastPayoutDate.In(loc).Format("2006-01-02")
		out.LastPayoutDate = &d
	}
	WriteJSON(w, http.StatusOK, out)
}

func (s *Server) getAnalyticsOutlook(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	today := time.Now().In(loc)
	todayMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)

	trailingADR, err := s.Store.TrailingADR(r.Context(), pid, todayMidnight)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Preload active stays covering up to 90 days out for reuse.
	end90 := todayMidnight.AddDate(0, 0, 90)
	stays, err := s.Store.ListActiveOccupanciesInDateRange(r.Context(), pid, todayMidnight, end90)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	windows := []analyticsKPIWindow{}
	for _, days := range []int{30, 60, 90} {
		winEnd := todayMidnight.AddDate(0, 0, days)
		nights := store.NightsSoldInRange(stays, todayMidnight, winEnd)
		avail := store.AvailableNightsInRange(todayMidnight, winEnd)
		gross, _, _, _, matchedIDs, err := s.Store.SumPayoutGrossNetForStays(r.Context(), pid, todayMidnight, winEnd)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "database error")
			return
		}
		matched := map[int64]bool{}
		for _, id := range matchedIDs {
			matched[id] = true
		}
		matchedNights := 0
		for _, st := range stays {
			if !matched[st.ID] {
				continue
			}
			matchedNights += store.NightsSoldInRange([]store.OccupancyLite{st}, todayMidnight, winEnd)
		}
		unmatchedNights := nights - matchedNights
		if unmatchedNights < 0 {
			unmatchedNights = 0
		}
		estimated := int64(unmatchedNights) * trailingADR
		windows = append(windows, analyticsKPIWindow{
			Days: days, NightsSold: nights, AvailableNights: avail,
			ConfirmedCents: gross, EstimatedCents: estimated,
			TotalRevenueCents: gross + estimated,
		})
	}

	pace, err := s.Store.PaceSeriesCumulative(r.Context(), pid, todayMidnight, end90)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	pacePoints := make([]analyticsPacePoint, 0, len(pace))
	for _, p := range pace {
		pacePoints = append(pacePoints, analyticsPacePoint{Date: p.Date, Count: p.Count})
	}

	unsold, err := s.Store.ListUnsoldNightsWithContext(r.Context(), pid, todayMidnight, todayMidnight.AddDate(0, 0, 14))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	unsoldOut := make([]analyticsUnsoldNight, 0, len(unsold))
	for _, u := range unsold {
		unsoldOut = append(unsoldOut, analyticsUnsoldNight{
			Date: u.Date, PrevGuest: u.PrevGuest, NextGuest: u.NextGuest,
		})
	}

	newBookings, err := s.Store.NewBookingsByDay(r.Context(), pid, todayMidnight.AddDate(0, 0, -7))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	newBookingsOut := make([]analyticsCountByDay, 0, len(newBookings))
	for _, b := range newBookings {
		newBookingsOut = append(newBookingsOut, analyticsCountByDay{Date: b.Date, Count: b.Count})
	}

	fresh, _ := s.Store.GetAnalyticsFreshness(r.Context(), pid)
	resp := analyticsOutlookResponse{
		GeneratedAt:  time.Now().UTC(),
		Windows:      windows,
		PacingSeries: pacePoints,
		UnsoldNights: unsoldOut,
		NewBookings:  newBookingsOut,
		TrailingADR:  trailingADR,
	}
	if fresh != nil && fresh.LastPayoutDate != nil {
		d := fresh.LastPayoutDate.In(loc).Format("2006-01-02")
		resp.RevenueAsOf = &d
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) getAnalyticsPerformance(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	today := time.Now().In(loc)
	todayMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	defFrom := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
	defTo := defFrom.AddDate(0, 1, 0)
	from := parseDateParam(r.URL.Query().Get("from"), defFrom, loc)
	to := parseDateParam(r.URL.Query().Get("to"), defTo, loc)
	if !to.After(from) {
		WriteError(w, http.StatusBadRequest, "to must be after from")
		return
	}
	yoy := strings.EqualFold(r.URL.Query().Get("yoy"), "true")
	year := today.Year()
	if q := strings.TrimSpace(r.URL.Query().Get("year")); q != "" {
		if y, err := strconv.Atoi(q); err == nil && y >= 2000 && y <= 3000 {
			year = y
		}
	}

	kpis, err := s.computePerformanceKPIs(r, pid, from, to, loc)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	resp := analyticsPerformanceResponse{
		GeneratedAt: time.Now().UTC(),
		From:        formatDayInLoc(from, loc),
		To:          formatDayInLoc(to, loc),
		KPIs:        *kpis,
	}
	if yoy {
		pFrom := from.AddDate(-1, 0, 0)
		pTo := to.AddDate(-1, 0, 0)
		if prior, err := s.computePerformanceKPIs(r, pid, pFrom, pTo, loc); err == nil && prior != nil {
			resp.PriorKPIs = prior
		}
	}

	// Monthly trend — up to 24 months back from `to`.
	trendFrom := to.AddDate(-2, 0, 0)
	rows, err := s.Store.ListMonthlyOccupancyAndADR(r.Context(), pid, trendFrom.Format("2006-01"), to.AddDate(0, -1, 0).Format("2006-01"), loc)
	if err == nil {
		for _, row := range rows {
			occ := safeDiv(float64(row.NightsSold), float64(row.AvailableNights))
			adr := int64(0)
			if row.MatchedNights > 0 {
				adr = row.GrossCents / int64(row.MatchedNights)
			}
			resp.MonthlyTrend = append(resp.MonthlyTrend, analyticsMonthlyTrendRow{
				Month: row.Month, OccupancyRate: occ, ADRCents: adr,
				GrossCents: row.GrossCents, NightsSold: row.NightsSold, AvailableNights: row.AvailableNights,
			})
		}
	}

	// Seasonality heatmap — last 3 full years.
	fromYear := to.AddDate(-3, 0, 0).Year()
	toYear := to.Year()
	weekly, err := s.Store.ListWeeklyOccupancy(r.Context(), pid, fromYear, toYear, loc)
	if err == nil {
		for _, c := range weekly {
			occ := safeDiv(float64(c.NightsSold), float64(c.AvailableN))
			resp.SeasonalityHeatmap = append(resp.SeasonalityHeatmap, analyticsHeatmapCell{
				Year: c.Year, Week: c.Week, OccupancyRate: occ,
			})
		}
	}

	// DOW occupancy
	dow, err := s.Store.ListDOWOccupancy(r.Context(), pid, from, to)
	if err == nil {
		for _, d := range dow {
			resp.DOWOccupancy = append(resp.DOWOccupancy, analyticsDOWCell{
				DOW: d.DOW, NightsSold: d.NightsSold, AvailableNights: d.AvailableNights,
				OccupancyRate: safeDiv(float64(d.NightsSold), float64(d.AvailableNights)),
			})
		}
	}

	// Cancellations
	cans, err := s.Store.ListCancellationsInArrivalWindow(r.Context(), pid, from, to)
	if err == nil {
		active, _ := s.Store.CountActiveArrivalsInWindow(r.Context(), pid, from, to)
		buckets := map[string]int{"0-3": 0, "4-14": 0, "15-45": 0, "46+": 0}
		order := []string{"0-3", "4-14", "15-45", "46+"}
		for _, c := range cans {
			switch {
			case c.LeadDays <= 3:
				buckets["0-3"]++
			case c.LeadDays <= 14:
				buckets["4-14"]++
			case c.LeadDays <= 45:
				buckets["15-45"]++
			default:
				buckets["46+"]++
			}
		}
		var bs []analyticsBucket
		for _, k := range order {
			bs = append(bs, analyticsBucket{Bucket: k, Count: buckets[k]})
		}
		total := active + len(cans)
		resp.Cancellation = analyticsCancellationStat{
			Rate:      safeDiv(float64(len(cans)), float64(total)),
			Buckets:   bs,
			Total:     total,
			Cancelled: len(cans),
		}
	}

	// Net per stay
	nps, err := s.Store.ListNetPerStay(r.Context(), pid, from, to, loc)
	if err == nil {
		for _, n := range nps {
			resp.NetPerStay = append(resp.NetPerStay, analyticsNetPerStayRow{
				StayID: n.StayID, StartAt: n.StartAt.UTC().Format(time.RFC3339),
				EndAt: n.EndAt.UTC().Format(time.RFC3339), GuestName: n.GuestName,
				GrossCents: n.GrossCents, CommissionCents: n.CommissionCents,
				PaymentFeeCents: n.PaymentFeeCents, CleaningAllocatedCents: n.CleaningAllocatedCents,
				NetCents: n.NetCents,
			})
		}
	}

	// Yearly cleaning (migrated from Cleaning module).
	cleaningRows, err := s.Store.ListCleaningYearMonthCounts(r.Context(), pid, year)
	if err == nil {
		series := make([]analyticsYearlyCleaningRow, 12)
		for i := 0; i < 12; i++ {
			series[i] = analyticsYearlyCleaningRow{Month: i + 1, Count: 0}
		}
		for _, c := range cleaningRows {
			if c.Month >= 1 && c.Month <= 12 {
				series[c.Month-1].Count = c.Count
			}
		}
		resp.YearlyCleaning = analyticsYearlyCleaningBlock{Year: year, Series: series}
	}

	// Yearly finance (migrated from Finance module).
	if roll, err := s.Store.YearlyFinanceRollup(r.Context(), pid, year); err == nil && roll != nil {
		resp.YearlyFinance = analyticsYearlyFinanceBlock{
			Year: year, IncomingCents: roll.IncomingCents,
			OutgoingCents: roll.OutgoingCents, NetCents: roll.NetCents,
		}
	} else {
		resp.YearlyFinance = analyticsYearlyFinanceBlock{Year: year}
	}

	fresh, _ := s.Store.GetAnalyticsFreshness(r.Context(), pid)
	if fresh != nil && fresh.LastPayoutDate != nil {
		d := fresh.LastPayoutDate.In(loc).Format("2006-01-02")
		resp.RevenueAsOf = &d
	}

	_ = todayMidnight
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) computePerformanceKPIs(r *http.Request, pid int64, from, to time.Time, loc *time.Location) (*analyticsPerformanceKPIs, error) {
	stays, err := s.Store.ListActiveOccupanciesInDateRange(r.Context(), pid, from, to)
	if err != nil {
		return nil, err
	}
	nights := store.NightsSoldInRange(stays, from, to)
	avail := store.AvailableNightsInRange(from, to)
	gross, net, commission, fees, matchedIDs, err := s.Store.SumPayoutGrossNetForStays(r.Context(), pid, from, to)
	if err != nil {
		return nil, err
	}
	matched := map[int64]bool{}
	for _, id := range matchedIDs {
		matched[id] = true
	}
	matchedNights := 0
	for _, st := range stays {
		if !matched[st.ID] {
			continue
		}
		matchedNights += store.NightsSoldInRange([]store.OccupancyLite{st}, from, to)
	}
	var adr int64
	if matchedNights > 0 {
		adr = gross / int64(matchedNights)
	}
	var revpar int64
	if avail > 0 {
		revpar = gross / int64(avail)
	}
	takeRate := 0.0
	if gross > 0 {
		takeRate = float64(commission+fees) / float64(gross)
	}
	return &analyticsPerformanceKPIs{
		NightsSold: nights, AvailableNights: avail,
		OccupancyRate: safeDiv(float64(nights), float64(avail)),
		ADRCents:      adr, RevPARCents: revpar,
		GrossCents: gross, NetCents: net,
		CommissionCents: commission, PaymentFeesCents: fees,
		EffectiveTakeRate: takeRate, MatchedNights: matchedNights,
	}, nil
}

func (s *Server) getAnalyticsDemand(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	today := time.Now().In(loc)
	defFrom := today.AddDate(-1, 0, 0)
	defTo := today
	from := parseDateParam(r.URL.Query().Get("from"), defFrom, loc)
	to := parseDateParam(r.URL.Query().Get("to"), defTo, loc)
	if !to.After(from) {
		WriteError(w, http.StatusBadRequest, "to must be after from")
		return
	}

	resp := analyticsDemandResponse{
		GeneratedAt: time.Now().UTC(),
		From:        formatDayInLoc(from, loc),
		To:          formatDayInLoc(to, loc),
	}
	if lt, err := s.Store.ListLeadTimeBuckets(r.Context(), pid, from, to); err == nil {
		for _, b := range lt {
			resp.LeadTime = append(resp.LeadTime, analyticsBucket{Bucket: b.Bucket, Count: b.Count})
		}
	}
	if los, err := s.Store.ListLengthOfStayBuckets(r.Context(), pid, from, to); err == nil {
		for _, b := range los {
			resp.LengthOfStay = append(resp.LengthOfStay, analyticsBucket{Bucket: b.Bucket, Count: b.Count})
		}
	}
	for _, dim := range []string{"month", "dow", "lead_bucket"} {
		rows, err := s.Store.ADRByDimension(r.Context(), pid, from, to, dim, loc)
		if err != nil {
			continue
		}
		out := []analyticsADRRow{}
		for _, row := range rows {
			adr := int64(0)
			if row.MatchedNights > 0 {
				adr = row.GrossCents / int64(row.MatchedNights)
			}
			out = append(out, analyticsADRRow{Bucket: row.Bucket, ADRCents: adr, MatchedNights: row.MatchedNights})
		}
		switch dim {
		case "month":
			resp.ADRByMonth = out
		case "dow":
			resp.ADRByDOW = out
		case "lead_bucket":
			resp.ADRByLeadBucket = out
		}
	}
	if gaps, err := s.Store.ListGapNights(r.Context(), pid, from, to); err == nil {
		for _, g := range gaps {
			resp.GapNights = append(resp.GapNights, analyticsGapRow{
				Date: g.Date, PrevStayID: g.PrevStayID, NextStayID: g.NextStayID,
				PrevCheckoutDate: g.PrevCheckoutDate, NextCheckinDate: g.NextCheckinDate,
			})
		}
	}
	if orph, err := s.Store.ListOrphanMidweek(r.Context(), pid, from, to); err == nil {
		for _, g := range orph {
			resp.OrphanMidweek = append(resp.OrphanMidweek, analyticsGapRow{
				Date: g.Date, PrevStayID: g.PrevStayID, NextStayID: g.NextStayID,
				PrevCheckoutDate: g.PrevCheckoutDate, NextCheckinDate: g.NextCheckinDate,
			})
		}
	}
	returning, total, err := s.Store.ReturningGuestCount(r.Context(), pid, from, to)
	if err == nil {
		resp.ReturningGuests = analyticsReturningStat{
			TotalActive: total, Returning: returning,
			ReturningRate: safeDiv(float64(returning), float64(total)),
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) getAnalyticsPace(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	windowParam := strings.TrimSpace(r.URL.Query().Get("window"))
	if windowParam == "" {
		// default: current month
		now := time.Now().In(loc)
		windowParam = fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
	}
	t, err := time.ParseInLocation("2006-01", windowParam, loc)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "window must be YYYY-MM")
		return
	}
	winStart := t
	winEnd := t.AddDate(0, 1, 0)

	thisYear, lastYear, err := s.Store.PaceCurveForWindow(r.Context(), pid, winStart, winEnd)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	// Require >=13 months of history for LY overlay.
	oldest := time.Time{}
	_ = s.Store.DB.QueryRowContext(r.Context(), `SELECT MIN(imported_at) FROM occupancies WHERE property_id = ?`, pid).Scan(new(string))
	lyAvailable := true
	// If window minus 1 year is before earliest data we don't have LY.
	var oldestStr string
	_ = s.Store.DB.QueryRowContext(r.Context(), `SELECT COALESCE(MIN(imported_at), '') FROM occupancies WHERE property_id = ?`, pid).Scan(&oldestStr)
	if oldestStr != "" {
		if ot, err := time.Parse(time.RFC3339, oldestStr); err == nil {
			oldest = ot
			if winStart.AddDate(-1, 0, 0).Before(oldest.AddDate(0, 0, -1)) {
				lyAvailable = false
			}
		}
	}
	mapTo := func(series []store.PaceCurveRow) []analyticsPacePoint {
		out := make([]analyticsPacePoint, 0, len(series))
		for _, p := range series {
			label := fmt.Sprintf("T-%d", p.DaysBefore)
			out = append(out, analyticsPacePoint{Date: label, Count: p.Count})
		}
		return out
	}
	resp := analyticsPaceResponse{
		GeneratedAt: time.Now().UTC(),
		Window:      windowParam,
		ThisYear:    mapTo(thisYear),
		LYAvailable: lyAvailable,
	}
	if lyAvailable {
		resp.LastYear = mapTo(lastYear)
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) getAnalyticsReturningGuests(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)
	if !ok {
		return
	}
	loc := s.analyticsLocation(r, pid)
	today := time.Now().In(loc)
	defFrom := today.AddDate(-1, 0, 0)
	defTo := today
	from := parseDateParam(r.URL.Query().Get("from"), defFrom, loc)
	to := parseDateParam(r.URL.Query().Get("to"), defTo, loc)
	limit := 50
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	guests, total, err := s.Store.ListReturningGuests(r.Context(), pid, from, to, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]analyticsReturningGuestRow, 0, len(guests))
	for _, g := range guests {
		out = append(out, analyticsReturningGuestRow{
			DisplayName: g.DisplayName, Normalized: g.NormalizedName,
			StayCount: g.StayCount,
			FirstStay: g.FirstStay.UTC().Format(time.RFC3339),
			LastStay:  g.LastStay.UTC().Format(time.RFC3339),
		})
	}
	WriteJSON(w, http.StatusOK, analyticsReturningGuestsResponse{
		GeneratedAt: time.Now().UTC(),
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		Guests:      out,
	})
}
