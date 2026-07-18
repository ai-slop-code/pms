package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

// PMS_19 §11A named-stay + repair endpoints. All are property-scoped and gated
// on Occupancy/LevelWrite, matching the existing occupancy override actions.

type namedStayCreateBody struct {
	CheckIn          string `json:"check_in"`
	CheckOut         string `json:"check_out"`
	GuestDisplayName string `json:"guest_display_name"`
}

type namedStayPatchBody struct {
	CheckIn          *string `json:"check_in"`
	CheckOut         *string `json:"check_out"`
	GuestDisplayName *string `json:"guest_display_name"`
}

type stayCreateBody struct {
	DisplayName      string `json:"display_name"`
	GuestDisplayName string `json:"guest_display_name"`
	StayType         string `json:"stay_type"`
	CheckIn          string `json:"check_in"`
	CheckOut         string `json:"check_out"`
	CleaningRequired *bool  `json:"cleaning_required"`
}

type stayPatchBody struct {
	DisplayName           *string `json:"display_name"`
	GuestDisplayName      *string `json:"guest_display_name"`
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

type namedStayV2Response struct {
	OK                   bool    `json:"ok"`
	NamedStayID          int64   `json:"named_stay_id"`
	LegacyOccupancyID    *int64  `json:"legacy_occupancy_id,omitempty"`
	NukiGenerationStatus string  `json:"nuki_generation_status"`
	NukiGenerationError  *string `json:"nuki_generation_error,omitempty"`
}

func (s *Server) postNamedStay(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	upstreamUID := strings.TrimSpace(chi.URLParam(r, "upstreamUid"))
	if upstreamUID == "" {
		WriteError(w, http.StatusBadRequest, "invalid upstream uid")
		return
	}
	var body namedStayCreateBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.audit(r, actor, "occupancy_named_stay_created", "occupancy_block", upstreamUID, "attempt")
	occID, err := s.Store.CreateNamedStay(r.Context(), propID, upstreamUID, body.CheckIn, body.CheckOut, body.GuestDisplayName, actor.ID)
	if err != nil {
		writeNamedStayError(w, err)
		return
	}
	s.reconcileCleaningBestEffort(r, propID, "named_stay")
	s.audit(r, actor, "occupancy_named_stay_created", "occupancy", strconv.FormatInt(occID, 10), "success")
	WriteJSON(w, http.StatusOK, struct {
		OK          bool  `json:"ok"`
		OccupancyID int64 `json:"occupancy_id"`
	}{OK: true, OccupancyID: occID})
}

func (s *Server) patchNamedStay(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body namedStayPatchBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.audit(r, actor, "occupancy_named_stay_range_changed", "occupancy", strconv.FormatInt(occID, 10), "attempt")
	if err := s.Store.UpdateNamedStay(r.Context(), propID, occID, body.CheckIn, body.CheckOut, body.GuestDisplayName); err != nil {
		writeNamedStayError(w, err)
		return
	}
	s.reconcileCleaningBestEffort(r, propID, "named_stay_edit")
	s.audit(r, actor, "occupancy_named_stay_range_changed", "occupancy", strconv.FormatInt(occID, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) deleteNamedStay(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, actor, "occupancy_named_stay_deleted", "occupancy", strconv.FormatInt(occID, 10), "attempt")
	if err := s.Store.DeleteNamedStay(r.Context(), propID, occID); err != nil {
		writeNamedStayError(w, err)
		return
	}
	s.reconcileCleaningBestEffort(r, propID, "named_stay_delete")
	s.audit(r, actor, "occupancy_named_stay_deleted", "occupancy", strconv.FormatInt(occID, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

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
		DisplayName:      firstNonEmpty(body.DisplayName, body.GuestDisplayName),
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
	s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_promote", stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_promoted", "named_stay", strconv.FormatInt(stay.ID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponse(stay))
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
		DisplayName:      firstNonEmpty(body.DisplayName, body.GuestDisplayName),
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
	s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_create", stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_created", "named_stay", strconv.FormatInt(stay.ID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponse(stay))
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
	displayName := body.DisplayName
	if displayName == nil {
		displayName = body.GuestDisplayName
	}
	s.audit(r, actor, "named_stay_updated", "named_stay", strconv.FormatInt(stayID, 10), "attempt")
	stay, err := s.Store.UpdateNamedStayRecord(r.Context(), propID, stayID, store.NamedStayUpdateInput{
		DisplayName:           displayName,
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
	s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_update", stayRangeFromNamedStay(before), stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_updated", "named_stay", strconv.FormatInt(stayID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponse(stay))
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
	s.reconcileCleaningStayRangesBestEffort(r, propID, "named_stay_status", stayRangeFromNamedStay(before), stayRange{stay.CheckInDate, stay.CheckOutDate})
	s.audit(r, actor, "named_stay_status_changed", "named_stay", strconv.FormatInt(stayID, 10), "success")
	WriteJSON(w, http.StatusOK, namedStayResponse(stay))
}

func (s *Server) postOccupancyRepairDryRun(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	report, err := s.Store.OccupancyRepairPlan(r.Context(), propID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "repair plan failed")
		return
	}
	s.audit(r, actor, "occupancy_repair_dry_run", "property", strconv.FormatInt(propID, 10), "success")
	WriteJSON(w, http.StatusOK, report)
}

func (s *Server) postOccupancyRepairApply(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	if actor.Role != "super_admin" && actor.Role != "owner" {
		WriteError(w, http.StatusForbidden, "repair requires super_admin or owner")
		return
	}
	s.audit(r, actor, "occupancy_repair_applied", "property", strconv.FormatInt(propID, 10), "attempt")
	report, err := s.Store.OccupancyRepairApply(r.Context(), propID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "repair apply failed")
		return
	}
	for _, res := range report.Resolutions {
		s.audit(r, actor, "occupancy_duplicate_resolved", "occupancy", strconv.FormatInt(res.WinnerOccID, 10), "success")
	}
	s.reconcileCleaningBestEffort(r, propID, "occupancy_repair")
	s.audit(r, actor, "occupancy_repair_applied", "property", strconv.FormatInt(propID, 10), "success")
	WriteJSON(w, http.StatusOK, report)
}

func (s *Server) reconcileCleaningBestEffort(r *http.Request, propID int64, trigger string) {
	if s.CleaningCalendar != nil {
		_, _ = s.CleaningCalendar.ReconcileProperty(r.Context(), propID, trigger)
	}
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

func (s *Server) reconcileCleaningStayRangesBestEffort(r *http.Request, propID int64, trigger string, ranges ...stayRange) {
	if s.CleaningCalendar == nil {
		return
	}
	from, to := affectedCleaningDateRange(ranges...)
	if from == "" || to == "" {
		_, _ = s.CleaningCalendar.ReconcileProperty(r.Context(), propID, trigger)
		return
	}
	_, _ = s.CleaningCalendar.ReconcilePropertyDateRange(r.Context(), propID, from, to, trigger)
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
		// Include the whole stay window because raw provisional placeholders are
		// checkout dates derived from every covered night.
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

func (s *Server) refreshedNamedStayOrOriginal(r *http.Request, propID int64, stay *store.NamedStay) *store.NamedStay {
	refreshed, err := s.Store.GetNamedStay(r.Context(), propID, stay.ID)
	if err == nil {
		return refreshed
	}
	return stay
}

func namedStayResponse(stay *store.NamedStay) namedStayV2Response {
	resp := namedStayV2Response{OK: true}
	if stay == nil {
		return resp
	}
	resp.NamedStayID = stay.ID
	if stay.LegacyOccupancyID.Valid {
		id := stay.LegacyOccupancyID.Int64
		resp.LegacyOccupancyID = &id
	}
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

func parseStayID(r *http.Request) (int64, error) {
	stayID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "stayId")), 10, 64)
	if err != nil || stayID <= 0 {
		return 0, errors.New("invalid stay id")
	}
	return stayID, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
