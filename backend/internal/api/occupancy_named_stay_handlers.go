package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

const stayReasonMaxLen = 500

type stayCreateBody struct {
	DisplayName      string `json:"display_name"`
	StayType         string `json:"stay_type"`
	CheckIn          string `json:"check_in"`
	CheckOut         string `json:"check_out"`
	CleaningRequired *bool  `json:"cleaning_required"`
}

type stayPatchBody struct {
	DisplayName           *string `json:"display_name"`
	StayType              *string `json:"stay_type"`
	CheckIn               *string `json:"check_in"`
	CheckOut              *string `json:"check_out"`
	CleaningRequired      *bool   `json:"cleaning_required"`
	ManualRevenueCents    *int64  `json:"manual_revenue_cents"`
	ManualRevenueCurrency *string `json:"manual_revenue_currency"`
	ManualRevenueNote     *string `json:"manual_revenue_note"`
}

type stayStatusBody struct {
	Status string `json:"status"`
}

type stayOutcomePatchBody struct {
	Outcome json.RawMessage `json:"outcome"`
	Reason  string          `json:"reason"`
}

type stayReviewPatchBody struct {
	ReviewStatus string `json:"review_status"`
	Reason       string `json:"reason"`
}

type namedStayV2Response struct {
	OK                   bool                             `json:"ok"`
	StaySaved            bool                             `json:"stay_saved"`
	NamedStayID          int64                            `json:"named_stay_id"`
	NukiGenerationStatus string                           `json:"nuki_generation_status"`
	NukiGenerationError  *string                          `json:"nuki_generation_error,omitempty"`
	CleaningCalendar     cleaningCalendarMutationResponse `json:"cleaning_calendar"`
	Error                string                           `json:"error,omitempty"`
}

type cleaningCalendarMutationResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type cleaningCalendarSkip struct{ reason string }

func (e cleaningCalendarSkip) Error() string { return e.reason }

func (s *Server) postBookingBlockPromote(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	blockID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "blockId")), 10, 64)
	if err != nil || blockID <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid block id")
		return
	}
	var body stayCreateBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	stayType := strings.TrimSpace(body.StayType)
	if stayType == "" {
		stayType = store.StayTypeBookingCom
	}
	s.audit(r, actor, "named_stay_promoted", "raw_booking_block", strconv.FormatInt(blockID, 10), "attempt")
	stay, err := s.Store.PromoteRawBookingBlockToNamedStay(r.Context(), propID, blockID, store.NamedStayCreateInput{
		DisplayName:      strings.TrimSpace(body.DisplayName),
		StayType:         stayType,
		CheckInDate:      body.CheckIn,
		CheckOutDate:     body.CheckOut,
		CleaningRequired: body.CleaningRequired,
		CreatedByUserID:  actor.ID,
	})
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	stay = s.triggerNamedStayNukiGeneration(r, propID, stay)
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_promote", stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_promoted", "named_stay", strconv.FormatInt(stay.ID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

func (s *Server) postStay(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	var body stayCreateBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.audit(r, actor, "named_stay_created", "property", strconv.FormatInt(propID, 10), "attempt")
	stay, err := s.Store.CreateNamedStayRecord(r.Context(), store.NamedStayCreateInput{
		PropertyID:       propID,
		DisplayName:      strings.TrimSpace(body.DisplayName),
		StayType:         body.StayType,
		CheckInDate:      body.CheckIn,
		CheckOutDate:     body.CheckOut,
		CleaningRequired: body.CleaningRequired,
		CreatedByUserID:  actor.ID,
		SourceChannel:    "manual",
	})
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	stay = s.triggerNamedStayNukiGeneration(r, propID, stay)
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_create", stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_created", "named_stay", strconv.FormatInt(stay.ID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

func (s *Server) patchStay(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	stayID, err := parseStayID(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body stayPatchBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	before, _ := s.Store.GetNamedStay(r.Context(), propID, stayID)
	s.audit(r, actor, "named_stay_updated", "named_stay", strconv.FormatInt(stayID, 10), "attempt")
	stay, err := s.Store.UpdateNamedStayRecord(r.Context(), propID, stayID, store.NamedStayUpdateInput{
		DisplayName:           body.DisplayName,
		StayType:              body.StayType,
		CheckInDate:           body.CheckIn,
		CheckOutDate:          body.CheckOut,
		CleaningRequired:      body.CleaningRequired,
		ManualRevenueCents:    body.ManualRevenueCents,
		ManualRevenueCurrency: body.ManualRevenueCurrency,
		ManualRevenueNote:     body.ManualRevenueNote,
		UpdatedByUserID:       actor.ID,
	})
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	if namedStayNukiFieldsChanged(before, stay) {
		stay = s.reconcileNamedStayNuki(r, propID, stay, "named_stay_update")
	}
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_update", stayRangeFromNamedStay(before), stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_updated", "named_stay", strconv.FormatInt(stayID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

func (s *Server) patchStayStatus(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	stayID, err := parseStayID(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	before, _ := s.Store.GetNamedStay(r.Context(), propID, stayID)
	var body stayStatusBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.audit(r, actor, "named_stay_status_changed", "named_stay", strconv.FormatInt(stayID, 10), "attempt")
	stay, err := s.Store.UpdateNamedStayStatus(r.Context(), propID, stayID, body.Status, actor.ID)
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	stay = s.reconcileNamedStayNuki(r, propID, stay, "named_stay_status")
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_status", stayRangeFromNamedStay(before), stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_status_changed", "named_stay", strconv.FormatInt(stayID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

func (s *Server) patchStayOutcome(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	stayID, err := parseStayID(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body stayOutcomePatchBody
	if err := ReadJSON(r, &body); err != nil || len(body.Outcome) == 0 {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) > stayReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "reason too long")
		return
	}
	var outcome *string
	if strings.TrimSpace(string(body.Outcome)) != "null" {
		var value string
		if err := json.Unmarshal(body.Outcome, &value); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid outcome")
			return
		}
		value = strings.TrimSpace(value)
		outcome = &value
	}
	before, _ := s.Store.GetNamedStay(r.Context(), propID, stayID)
	id := strconv.FormatInt(stayID, 10)
	s.audit(r, actor, "named_stay_outcome_changed", "named_stay", id, "attempt")
	stay, err := s.Store.UpdateNamedStayOutcome(r.Context(), propID, stayID, actor.ID, outcome, reason)
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	stay = s.reconcileNamedStayNuki(r, propID, stay, "named_stay_outcome")
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_outcome", stayRangeFromNamedStay(before), stayRangeFromNamedStay(stay))
	s.audit(r, actor, "named_stay_outcome_changed", "named_stay", id, "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

func (s *Server) patchStayReview(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	stayID, err := parseStayID(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body stayReviewPatchBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.ReviewStatus = strings.TrimSpace(body.ReviewStatus)
	body.Reason = strings.TrimSpace(body.Reason)
	if (body.ReviewStatus != "confirmed" && body.ReviewStatus != "rejected") || len(body.Reason) > stayReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "invalid review")
		return
	}
	before, _ := s.Store.GetNamedStay(r.Context(), propID, stayID)
	id := strconv.FormatInt(stayID, 10)
	s.audit(r, actor, "named_stay_review_changed", "named_stay", id, "attempt")
	stay, err := s.Store.UpdateNamedStayReview(r.Context(), propID, stayID, actor.ID, body.ReviewStatus, body.Reason)
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	stay = s.reconcileNamedStayNuki(r, propID, stay, "named_stay_review")
	cleaningErr := s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_review", stayRangeFromNamedStay(before), stayRangeFromNamedStay(stay))
	s.audit(r, actor, "named_stay_review_changed", "named_stay", id, "success")
	WriteJSON(w, http.StatusOK, namedStayResponseWithCleaning(stay, cleaningErr))
}

type stayRange struct {
	checkIn  string
	checkOut string
}

func stayRangeFromNamedStay(stay *store.NamedStay) stayRange {
	if stay == nil {
		return stayRange{}
	}
	return stayRange{checkIn: stay.CheckInDate, checkOut: stay.CheckOutDate}
}

func (s *Server) reconcileCleaningStayRangesBestEffort(r *http.Request, propID int64, trigger string, ranges ...stayRange) error {
	if s.CleaningCalendar == nil {
		return cleaningCalendarSkip{reason: "disabled"}
	}
	settings, err := s.Store.GetGoogleCleaningSettings(r.Context(), propID)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return cleaningCalendarSkip{reason: "disabled"}
	}
	if !settings.CalendarID.Valid || strings.TrimSpace(settings.CalendarID.String) == "" {
		return cleaningCalendarSkip{reason: "calendar_not_set"}
	}
	from, to := affectedCleaningDateRange(ranges...)
	if from == "" || to == "" {
		_, err := s.CleaningCalendar.ReconcileProperty(r.Context(), propID, trigger)
		return err
	}
	_, err = s.CleaningCalendar.ReconcilePropertyDateRange(r.Context(), propID, from, to, trigger)
	return err
}

func affectedCleaningDateRange(ranges ...stayRange) (string, string) {
	var from, to string
	for _, r := range ranges {
		ci := strings.TrimSpace(r.checkIn)
		co := strings.TrimSpace(r.checkOut)
		if ci == "" || co == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", ci); err != nil {
			continue
		}
		if _, err := time.Parse("2006-01-02", co); err != nil {
			continue
		}
		// Include the whole affected stay window so arrival turnover is refreshed.
		if from == "" || ci < from {
			from = ci
		}
		if to == "" || co > to {
			to = co
		}
	}
	return from, to
}

func (s *Server) triggerNamedStayNukiGeneration(r *http.Request, propID int64, stay *store.NamedStay) *store.NamedStay {
	if stay == nil {
		return stay
	}
	reviewStatus := "confirmed"
	if stay.ReviewStatus.Valid && strings.TrimSpace(stay.ReviewStatus.String) != "" {
		reviewStatus = strings.TrimSpace(stay.ReviewStatus.String)
	}
	if !store.NamedStayNukiEligible(stay.StayType, reviewStatus) {
		_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationNotApplicable, "")
		return s.refreshedNamedStayOrOriginal(r, propID, stay)
	}
	if s.Nuki == nil {
		_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationError, "nuki_service_unavailable")
		return s.refreshedNamedStayOrOriginal(r, propID, stay)
	}
	if err := s.Nuki.GenerateCodeForNamedStay(r.Context(), propID, stay.ID, "named_stay_create", stay.DisplayName); err != nil {
		_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationError, err.Error())
		return s.refreshedNamedStayOrOriginal(r, propID, stay)
	}
	_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationGenerated, "")
	return s.refreshedNamedStayOrOriginal(r, propID, stay)
}

func namedStayNukiFieldsChanged(before, after *store.NamedStay) bool {
	if before == nil || after == nil {
		return true
	}
	return before.DisplayName != after.DisplayName || before.StayType != after.StayType ||
		before.CheckInDate != after.CheckInDate || before.CheckOutDate != after.CheckOutDate ||
		before.Status != after.Status || before.ReviewStatus != after.ReviewStatus || before.StayOutcome != after.StayOutcome
}

func (s *Server) reconcileNamedStayNuki(r *http.Request, propID int64, stay *store.NamedStay, trigger string) *store.NamedStay {
	if stay == nil {
		return stay
	}
	if s.Nuki == nil {
		_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationError, "nuki_service_unavailable")
		return s.refreshedNamedStayOrOriginal(r, propID, stay)
	}
	if err := s.Nuki.ReconcileNamedStay(r.Context(), propID, stay.ID, trigger); err != nil {
		_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, store.NukiGenerationError, err.Error())
		return s.refreshedNamedStayOrOriginal(r, propID, stay)
	}
	reviewStatus := "confirmed"
	if stay.ReviewStatus.Valid && strings.TrimSpace(stay.ReviewStatus.String) != "" {
		reviewStatus = strings.TrimSpace(stay.ReviewStatus.String)
	}
	status := store.NukiGenerationNotApplicable
	if stay.Status == store.NamedStayStatusActive && store.NamedStayNukiEligible(stay.StayType, reviewStatus) && !stay.StayOutcome.Valid {
		status = store.NukiGenerationGenerated
	}
	_ = s.Store.MarkNamedStayNukiGeneration(r.Context(), propID, stay.ID, status, "")
	return s.refreshedNamedStayOrOriginal(r, propID, stay)
}

func (s *Server) refreshedNamedStayOrOriginal(r *http.Request, propID int64, stay *store.NamedStay) *store.NamedStay {
	refreshed, err := s.Store.GetNamedStay(r.Context(), propID, stay.ID)
	if err == nil {
		return refreshed
	}
	return stay
}

func namedStayResponse(stay *store.NamedStay) namedStayV2Response {
	resp := namedStayV2Response{OK: true, StaySaved: true, CleaningCalendar: cleaningCalendarMutationResponse{Status: "synced"}}
	if stay == nil {
		return resp
	}
	resp.NamedStayID = stay.ID
	if stay.NukiGenerationStatus.Valid {
		resp.NukiGenerationStatus = stay.NukiGenerationStatus.String
	}
	if resp.NukiGenerationStatus == "" {
		resp.NukiGenerationStatus = store.NukiGenerationNotApplicable
	}
	if stay.NukiGenerationError.Valid && strings.TrimSpace(stay.NukiGenerationError.String) != "" {
		errText := stay.NukiGenerationError.String
		resp.NukiGenerationError = &errText
	}
	return resp
}

func namedStayResponseWithCleaning(stay *store.NamedStay, err error) namedStayV2Response {
	resp := namedStayResponse(stay)
	if err == nil {
		return resp
	}
	var skipped cleaningCalendarSkip
	if errors.As(err, &skipped) {
		resp.CleaningCalendar = cleaningCalendarMutationResponse{Status: "skipped", Reason: skipped.reason}
		return resp
	}
	resp.OK = false
	resp.CleaningCalendar = cleaningCalendarMutationResponse{Status: "error", Error: "Google Calendar cleaning synchronization failed"}
	resp.Error = "Stay saved, but cleaning calendar sync failed. Retry calendar synchronization."
	return resp
}

func parseStayID(r *http.Request) (int64, error) {
	stayID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "stayId")), 10, 64)
	if err != nil || stayID <= 0 {
		return 0, errors.New("invalid stay id")
	}
	return stayID, nil
}

func writeNamedStayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNamedStayOutsideBlock), errors.Is(err, store.ErrNamedStayOverlap):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNamedStayInvalidRange), errors.Is(err, store.ErrNamedStayInvalidType):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrUpstreamBlockNotFound), errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "not found")
	default:
		WriteError(w, http.StatusInternalServerError, "update failed")
	}
}
