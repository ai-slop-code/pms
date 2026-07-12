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

func writeNamedStayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNamedStayOutsideBlock), errors.Is(err, store.ErrNamedStayOverlap):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNamedStayInvalidRange):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrUpstreamBlockNotFound), errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "not found")
	default:
		WriteError(w, http.StatusInternalServerError, "update failed")
	}
}
