package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pms/backend/internal/permissions"
)

type cleaningLogRow struct {
	DayDate            string  `json:"day_date"`
	FirstEntryAt       *string `json:"first_entry_at"`
	NukiEventReference *string `json:"nuki_event_reference"`
	CountedForSalary   bool    `json:"counted_for_salary"`
}

type cleaningLogsResponse struct {
	Month string           `json:"month"`
	Logs  []cleaningLogRow `json:"logs"`
}

type cleaningSummaryResponse struct {
	Month                 string `json:"month"`
	CountedDays           int    `json:"counted_days"`
	BaseSalaryCents       int    `json:"base_salary_cents"`
	AdjustmentsTotalCents int    `json:"adjustments_total_cents"`
	FinalSalaryCents      int    `json:"final_salary_cents"`
}

type cleaningHeatmapBucket struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type cleaningHeatmapResponse struct {
	Month   string                  `json:"month"`
	Buckets []cleaningHeatmapBucket `json:"buckets"`
}

type cleaningFeeRow struct {
	ID                     int64  `json:"id"`
	CleaningFeeAmountCents int    `json:"cleaning_fee_amount_cents"`
	WashingFeeAmountCents  int    `json:"washing_fee_amount_cents"`
	EffectiveFrom          string `json:"effective_from"`
	CreatedAt              string `json:"created_at"`
}

type cleaningFeesResponse struct {
	Fees []cleaningFeeRow `json:"fees"`
}

type cleaningAdjustmentRow struct {
	ID                    int64  `json:"id"`
	AdjustmentAmountCents int    `json:"adjustment_amount_cents"`
	Reason                string `json:"reason"`
	CreatedAt             string `json:"created_at"`
}

type cleaningAdjustmentsResponse struct {
	Month       string                  `json:"month"`
	Adjustments []cleaningAdjustmentRow `json:"adjustments"`
}

type cleaningReconcileStatsResponse struct {
	FetchedEvents     int    `json:"fetched_events"`
	AuthMatchedEvents int    `json:"auth_matched_events"`
	EntryLikeEvents   int    `json:"entry_like_events"`
	UpsertedDays      int    `json:"upserted_days"`
	FallbackAnyEvent  bool   `json:"fallback_any_event"`
	CleanerAliasCount int    `json:"cleaner_alias_count"`
	RequestedSinceUTC string `json:"requested_since_utc"`
}

type cleaningReconcileResponse struct {
	OK    bool                            `json:"ok"`
	Error string                          `json:"error,omitempty"`
	Stats *cleaningReconcileStatsResponse `json:"stats,omitempty"`
}

func (s *Server) parseMonthInPropertyTZ(r *http.Request, loc *time.Location) (string, int, int) {
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		now := time.Now().In(loc)
		month = now.Format("2006-01")
	}
	var y, m int
	if _, err := fmtSscanfMonth(month, &y, &m); err != nil {
		return "", 0, 0
	}
	if m < 1 || m > 12 {
		return "", 0, 0
	}
	return month, y, m
}

func fmtSscanfMonth(month string, y, m *int) (int, error) {
	return fmt.Sscanf(month, "%d-%d", y, m)
}

func (s *Server) getCleaningLogs(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	month, _, _ := s.parseMonthInPropertyTZ(r, loc)
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	logs, err := s.Store.ListCleaningDailyLogsForMonth(r.Context(), pid, month)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]cleaningLogRow, 0, len(logs))
	for _, l := range logs {
		out = append(out, cleaningLogRow{
			DayDate:            l.DayDate,
			FirstEntryAt:       nullTimePtr(l.FirstEntryAt),
			NukiEventReference: nullStringPtr(l.NukiEventReference),
			CountedForSalary:   l.CountedForSalary,
		})
	}
	WriteJSON(w, http.StatusOK, cleaningLogsResponse{Month: month, Logs: out})
}

func (s *Server) getCleaningSummary(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	month, y, m := s.parseMonthInPropertyTZ(r, loc)
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	summary, err := s.Store.ComputeCleaningMonthlySummary(r.Context(), pid, y, m, loc)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	WriteJSON(w, http.StatusOK, cleaningSummaryResponse{
		Month:                 month,
		CountedDays:           summary.CountedDays,
		BaseSalaryCents:       summary.BaseSalaryCents,
		AdjustmentsTotalCents: summary.AdjustmentsTotalCents,
		FinalSalaryCents:      summary.FinalSalaryCents,
	})
}

func (s *Server) getCleaningHeatmap(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	month, _, _ := s.parseMonthInPropertyTZ(r, loc)
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	logs, err := s.Store.ListCleaningDailyLogsForMonth(r.Context(), pid, month)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	counts := make([]int, 24)
	for _, l := range logs {
		if !l.CountedForSalary || !l.FirstEntryAt.Valid {
			continue
		}
		h := l.FirstEntryAt.Time.In(loc).Hour()
		if h >= 0 && h < 24 {
			counts[h]++
		}
	}
	buckets := make([]cleaningHeatmapBucket, 0, 24)
	for h := 0; h < 24; h++ {
		buckets = append(buckets, cleaningHeatmapBucket{Hour: h, Count: counts[h]})
	}
	WriteJSON(w, http.StatusOK, cleaningHeatmapResponse{Month: month, Buckets: buckets})
}

func (s *Server) getCleaningFees(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	rows, err := s.Store.ListCleanerFeeHistory(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]cleaningFeeRow, 0, len(rows))
	for _, rr := range rows {
		out = append(out, cleaningFeeRow{
			ID:                     rr.ID,
			CleaningFeeAmountCents: rr.CleaningFeeAmountCents,
			WashingFeeAmountCents:  rr.WashingFeeAmountCents,
			EffectiveFrom:          rr.EffectiveFrom.UTC().Format(time.RFC3339),
			CreatedAt:              rr.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	WriteJSON(w, http.StatusOK, cleaningFeesResponse{Fees: out})
}

func (s *Server) postCleaningFees(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelWrite)
	if !ok {
		return
	}
	var body struct {
		CleaningFeeAmountCents int    `json:"cleaning_fee_amount_cents"`
		WashingFeeAmountCents  int    `json:"washing_fee_amount_cents"`
		EffectiveFrom          string `json:"effective_from"`
	}
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.CleaningFeeAmountCents < 0 || body.WashingFeeAmountCents < 0 {
		WriteError(w, http.StatusBadRequest, "fee amounts must be >= 0")
		return
	}
	effectiveFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(body.EffectiveFrom))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "effective_from must be RFC3339")
		return
	}
	var by *int64
	if actor != nil {
		by = &actor.ID
	}
	if err := s.Store.CreateCleanerFeeHistoryRow(r.Context(), pid, body.CleaningFeeAmountCents, body.WashingFeeAmountCents, effectiveFrom, by); err != nil {
		WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}
	s.audit(r, actor, "cleaning_fee_create", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusCreated, actionResponse{OK: true})
}

func (s *Server) getCleaningAdjustments(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	month, y, m := s.parseMonthInPropertyTZ(r, loc)
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	rows, err := s.Store.ListCleaningAdjustmentsForMonth(r.Context(), pid, y, m)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]cleaningAdjustmentRow, 0, len(rows))
	for _, rr := range rows {
		out = append(out, cleaningAdjustmentRow{
			ID:                    rr.ID,
			AdjustmentAmountCents: rr.AdjustmentAmountCents,
			Reason:                rr.Reason,
			CreatedAt:             rr.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	WriteJSON(w, http.StatusOK, cleaningAdjustmentsResponse{Month: month, Adjustments: out})
}

func (s *Server) postCleaningAdjustment(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelWrite)
	if !ok {
		return
	}
	var body struct {
		Month                 string `json:"month"`
		AdjustmentAmountCents int    `json:"adjustment_amount_cents"`
		Reason                string `json:"reason"`
	}
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		WriteError(w, http.StatusBadRequest, "reason required")
		return
	}
	var y, m int
	if _, err := fmtSscanfMonth(strings.TrimSpace(body.Month), &y, &m); err != nil || m < 1 || m > 12 {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	var by *int64
	if actor != nil {
		by = &actor.ID
	}
	if err := s.Store.CreateCleaningAdjustment(r.Context(), pid, y, m, body.AdjustmentAmountCents, strings.TrimSpace(body.Reason), by); err != nil {
		WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}
	s.audit(r, actor, "cleaning_adjustment_create", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusCreated, actionResponse{OK: true})
}

func (s *Server) runCleaningReconcile(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, tzErr := time.LoadLocation(prop.Timezone)
	if tzErr != nil {
		loc = time.UTC
	}
	month, y, m := s.parseMonthInPropertyTZ(r, loc)
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	since := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, loc).UTC()
	stats, err := s.Nuki.ReconcileCleanerDailyLogsSince(r.Context(), pid, since)
	if err != nil {
		WriteJSON(w, http.StatusOK, cleaningReconcileResponse{OK: false, Error: err.Error()})
		return
	}
	s.audit(r, actor, "cleaning_reconcile", "property", strconv.FormatInt(pid, 10), "success")
	var outStats *cleaningReconcileStatsResponse
	if stats != nil {
		outStats = &cleaningReconcileStatsResponse{
			FetchedEvents:     stats.FetchedEvents,
			AuthMatchedEvents: stats.AuthMatchedEvents,
			EntryLikeEvents:   stats.EntryLikeEvents,
			UpsertedDays:      stats.UpsertedDays,
			FallbackAnyEvent:  stats.FallbackAnyEvent,
			CleanerAliasCount: stats.CleanerAliasCount,
			RequestedSinceUTC: stats.RequestedSinceUTC,
		}
	}
	WriteJSON(w, http.StatusOK, cleaningReconcileResponse{OK: true, Stats: outStats})
}
