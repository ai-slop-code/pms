package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

// PMS_14 / PMS_12 §2 manual occupancy labelling. Closing pulls a Booking.com
// block out of the analytics-active set; mark-external-sale records the row
// as occupied via a different channel with an operator-entered net amount.
// Reopen clears either label. Audit is written before the long write per
// PMS_13 §6 so a database failure still leaves a trail.

// closureCategoryAllowed gates the optional taxonomy used by Phase 1
// classification UI; the column itself is free-text so future categories
// don't need a migration.
var closureCategoryAllowed = map[string]struct{}{
	"owner_stay":  {},
	"maintenance": {},
	"soft_block":  {},
	"other":       {},
}

var externalChannelAllowed = map[string]struct{}{
	"airbnb":  {},
	"direct":  {},
	"walk_in": {},
	"other":   {},
}

const closureReasonMaxLen = 500

type closeOccupancyBody struct {
	Reason   string `json:"reason"`
	Category string `json:"category"`
	Night    string `json:"night"`
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
}

type externalSaleBody struct {
	NetAmountCents int64  `json:"net_amount_cents"`
	Currency       string `json:"currency"`
	Channel        string `json:"channel"`
	Reason         string `json:"reason"`
}

type splitNightsBody struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type stayOutcomeBody struct {
	Reason string `json:"reason"`
}

type cleaningCalendarExclusionBody struct {
	Reason string `json:"reason"`
}

func parsePropertyAndOccupancyIDs(r *http.Request) (int64, int64, error) {
	propID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid property id")
	}
	occID, err := strconv.ParseInt(chi.URLParam(r, "occupancyId"), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid occupancy id")
	}
	return propID, occID, nil
}

// postOccupancyClose handles POST /properties/{id}/occupancies/{occupancyId}/close.
func (s *Server) postOccupancyClose(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body closeOccupancyBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	body.Category = strings.TrimSpace(body.Category)
	body.Night = strings.TrimSpace(body.Night)
	body.CheckIn = strings.TrimSpace(body.CheckIn)
	body.CheckOut = strings.TrimSpace(body.CheckOut)
	if len(body.Reason) > closureReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "reason too long")
		return
	}
	if body.Category != "" {
		if _, ok := closureCategoryAllowed[body.Category]; !ok {
			WriteError(w, http.StatusBadRequest, "invalid category")
			return
		}
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_close", "occupancy", idStr, "attempt")
	if body.CheckIn != "" || body.CheckOut != "" {
		err = s.Store.CloseOccupancyRange(r.Context(), propID, occID, actor.ID, body.CheckIn, body.CheckOut, body.Reason, body.Category)
	} else if body.Night != "" {
		err = s.Store.CloseOccupancyNight(r.Context(), propID, occID, actor.ID, body.Night, body.Reason, body.Category)
	} else {
		err = s.Store.CloseOccupancy(r.Context(), propID, occID, actor.ID, body.Reason, body.Category)
	}
	if err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	s.audit(r, actor, "occupancy_close", "occupancy", idStr, "success")
	s.audit(r, actor, "occupancy_block_marked_no_guest", "occupancy", idStr, "success")
	// PMS_19 §5.3: closing a night must remove its provisional/named cleaning
	// event immediately, not on the next scheduled sync.
	s.reconcileCleaningBestEffort(r, propID, "occupancy_close")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

// postOccupancyExternalSale handles POST /properties/{id}/occupancies/{occupancyId}/external-sale.
func (s *Server) postOccupancyExternalSale(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body externalSaleBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.NetAmountCents < 0 {
		WriteError(w, http.StatusBadRequest, "net_amount_cents must be >= 0")
		return
	}
	body.Currency = strings.TrimSpace(strings.ToUpper(body.Currency))
	if body.Currency != "" && len(body.Currency) != 3 {
		WriteError(w, http.StatusBadRequest, "currency must be ISO-4217 (3 letters)")
		return
	}
	body.Channel = strings.TrimSpace(strings.ToLower(body.Channel))
	if body.Channel != "" {
		if _, ok := externalChannelAllowed[body.Channel]; !ok {
			WriteError(w, http.StatusBadRequest, "invalid channel")
			return
		}
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) > closureReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "reason too long")
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_mark_external_sale", "occupancy", idStr, "attempt")
	err = s.Store.MarkOccupancyExternalSale(r.Context(), propID, occID, actor.ID, body.NetAmountCents, body.Currency, body.Channel, body.Reason)
	if err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	s.audit(r, actor, "occupancy_mark_external_sale", "occupancy", idStr, "success")
	// PMS_19 §5.6: external-sale nights keep cleaning; reconcile so the checkout
	// event reflects the labelled range immediately.
	s.reconcileCleaningBestEffort(r, propID, "occupancy_external_sale")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) postOccupancySplitNights(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body splitNightsBody
	if r.ContentLength != 0 {
		if err := ReadJSON(r, &body); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	body.StartDate = strings.TrimSpace(body.StartDate)
	body.EndDate = strings.TrimSpace(body.EndDate)
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_split_nights", "occupancy", idStr, "attempt")
	err = s.Store.SplitOccupancyIntoNightRange(r.Context(), propID, occID, body.StartDate, body.EndDate)
	if err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	s.audit(r, actor, "occupancy_split_nights", "occupancy", idStr, "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

// postOccupancyReopen handles POST /properties/{id}/occupancies/{occupancyId}/reopen.
func (s *Server) postOccupancyReopen(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_reopen", "occupancy", idStr, "attempt")
	err = s.Store.ReopenOccupancy(r.Context(), propID, occID)
	if err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	s.audit(r, actor, "occupancy_reopen", "occupancy", idStr, "success")
	s.audit(r, actor, "occupancy_block_reopened", "occupancy", idStr, "success")
	// PMS_19 §5.3/§13.12: reopening restores unnamed-block coverage, which must
	// recreate the provisional cleaning event immediately.
	s.reconcileCleaningBestEffort(r, propID, "occupancy_reopen")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) postOccupancyOutcomeCancelledNonRefundable(w http.ResponseWriter, r *http.Request) {
	s.postOccupancyOutcome(w, r, store.StayOutcomeCancelledNonRefundable, "occupancy_outcome_cancelled_non_refundable")
}

func (s *Server) postOccupancyOutcomeNoShow(w http.ResponseWriter, r *http.Request) {
	s.postOccupancyOutcome(w, r, store.StayOutcomeNoShow, "occupancy_outcome_no_show")
}

func (s *Server) postOccupancyOutcome(w http.ResponseWriter, r *http.Request, outcome, auditAction string) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body stayOutcomeBody
	if r.ContentLength != 0 {
		if err := ReadJSON(r, &body); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) > closureReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "reason too long")
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, auditAction, "occupancy", idStr, "attempt")
	if err := s.Store.MarkOccupancyStayOutcome(r.Context(), propID, occID, actor.ID, outcome, body.Reason); err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	if s.CleaningCalendar != nil {
		if _, err := s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "stay_outcome"); err != nil {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "outcome saved, cleaning calendar failed: " + err.Error()})
			return
		}
	}
	s.audit(r, actor, auditAction, "occupancy", idStr, "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) postOccupancyOutcomeClear(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_outcome_clear", "occupancy", idStr, "attempt")
	if err := s.Store.ClearOccupancyStayOutcome(r.Context(), propID, occID); err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	if s.CleaningCalendar != nil {
		if _, err := s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "stay_outcome_clear"); err != nil {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "outcome cleared, cleaning calendar failed: " + err.Error()})
			return
		}
	}
	s.audit(r, actor, "occupancy_outcome_clear", "occupancy", idStr, "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) postOccupancyCleaningCalendarExclude(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body cleaningCalendarExclusionBody
	if r.ContentLength != 0 {
		if err := ReadJSON(r, &body); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) > closureReasonMaxLen {
		WriteError(w, http.StatusBadRequest, "reason too long")
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_cleaning_calendar_exclude", "occupancy", idStr, "attempt")
	if err := s.Store.MarkOccupancyCleaningCalendarExcluded(r.Context(), propID, occID, actor.ID, body.Reason); err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	if s.CleaningCalendar != nil {
		if _, err := s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "cleaning_calendar_exclusion"); err != nil {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "cleaning calendar exclusion saved, cleaning calendar failed: " + err.Error()})
			return
		}
	}
	s.audit(r, actor, "occupancy_cleaning_calendar_exclude", "occupancy", idStr, "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) postOccupancyCleaningCalendarInclude(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	_, occID, err := parsePropertyAndOccupancyIDs(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	idStr := strconv.FormatInt(occID, 10)
	s.audit(r, actor, "occupancy_cleaning_calendar_include", "occupancy", idStr, "attempt")
	if err := s.Store.ClearOccupancyCleaningCalendarExcluded(r.Context(), propID, occID); err != nil {
		writeOccupancyLabelError(w, err)
		return
	}
	if s.CleaningCalendar != nil {
		if _, err := s.CleaningCalendar.ReconcileProperty(r.Context(), propID, "cleaning_calendar_inclusion"); err != nil {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "cleaning calendar inclusion saved, cleaning calendar failed: " + err.Error()})
			return
		}
	}
	s.audit(r, actor, "occupancy_cleaning_calendar_include", "occupancy", idStr, "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func writeOccupancyLabelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOccupancyAlreadyLabelled):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrOccupancyOutcomeConflict), errors.Is(err, store.ErrOccupancyOutcomeIneligible):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrInvalidStayOutcome):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrOccupancyCleaningCalendarExclusionIneligible):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrInvalidOccupancySplit):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "occupancy not found")
	default:
		WriteError(w, http.StatusInternalServerError, "update failed")
	}
}
