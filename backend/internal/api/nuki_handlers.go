package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/ctxuser"
	"pms/backend/internal/permissions"
)

type nukiKeypadCodeRow struct {
	ID               int64   `json:"id"`
	ExternalNukiID   string  `json:"external_nuki_id"`
	AccountUserID    *string `json:"account_user_id"`
	Name             *string `json:"name"`
	AccessCodeMasked *string `json:"access_code_masked"`
	ValidFrom        *string `json:"valid_from"`
	ValidUntil       *string `json:"valid_until"`
	Enabled          bool    `json:"enabled"`
	PMSLinked        bool    `json:"pms_linked"`
	UpdatedAt        string  `json:"updated_at"`
}

type nukiCodesResponse struct {
	Codes []nukiKeypadCodeRow `json:"codes"`
}

type nukiUpcomingStayRow struct {
	OccupancyID         int64   `json:"occupancy_id"`
	SourceEventUID      string  `json:"source_event_uid"`
	Summary             *string `json:"summary"`
	SavedPinName        *string `json:"saved_pin_name"`
	StartAt             string  `json:"start_at"`
	EndAt               string  `json:"end_at"`
	OccupancyStatus     string  `json:"occupancy_status"`
	GeneratedCodeID     *int64  `json:"generated_code_id"`
	GeneratedLabel      *string `json:"generated_label"`
	GeneratedStatus     *string `json:"generated_status"`
	GeneratedMasked     *string `json:"generated_masked"`
	GeneratedValidFrom  *string `json:"generated_valid_from"`
	GeneratedValidUntil *string `json:"generated_valid_until"`
	GeneratedError      *string `json:"generated_error"`
	GeneratedUpdatedAt  *string `json:"generated_updated_at"`
}

type nukiUpcomingStaysResponse struct {
	Stays []nukiUpcomingStayRow `json:"stays"`
}

type nukiRunRow struct {
	ID             int64   `json:"id"`
	StartedAt      string  `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
	Status         string  `json:"status"`
	Trigger        string  `json:"trigger"`
	ErrorMessage   *string `json:"error_message"`
	ProcessedCount int     `json:"processed_count"`
	CreatedCount   int     `json:"created_count"`
	UpdatedCount   int     `json:"updated_count"`
	RevokedCount   int     `json:"revoked_count"`
	FailedCount    int     `json:"failed_count"`
}

type nukiRunsResponse struct {
	Runs    []nukiRunRow `json:"runs"`
	Page    int          `json:"page"`
	Limit   int          `json:"limit"`
	HasMore bool         `json:"has_more"`
}

func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func nullTimePtr(nt sql.NullTime) *string {
	if !nt.Valid {
		return nil
	}
	s := nt.Time.UTC().Format(time.RFC3339)
	return &s
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func (s *Server) listNukiCodes(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelRead)
	if !ok {
		return
	}
	var err error
	rows, err := s.Store.ListNukiKeypadCodes(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	pinsOnly := r.URL.Query().Get("pins_only") == "1" || r.URL.Query().Get("pins_only") == "true"
	out := make([]nukiKeypadCodeRow, 0, len(rows))
	for _, code := range rows {
		if pinsOnly {
			hasPinValue := code.AccessCodeMasked.Valid && strings.TrimSpace(code.AccessCodeMasked.String) != ""
			bookingLike := code.Name.Valid && strings.HasPrefix(strings.ToLower(strings.TrimSpace(code.Name.String)), "booking-")
			pinType := isPinTypeAuth(code.RawJSON)
			if !hasPinValue && !bookingLike && !pinType {
				continue
			}
		}
		out = append(out, nukiKeypadCodeRow{
			ID:               code.ID,
			ExternalNukiID:   code.ExternalNukiID,
			AccountUserID:    accountUserIDFromRaw(code.RawJSON),
			Name:             nullStringPtr(code.Name),
			AccessCodeMasked: nullStringPtr(code.AccessCodeMasked),
			ValidFrom:        nullTimePtr(code.ValidFrom),
			ValidUntil:       nullTimePtr(code.ValidUntil),
			Enabled:          code.Enabled,
			PMSLinked:        code.PMSLinked,
			UpdatedAt:        code.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	WriteJSON(w, http.StatusOK, nukiCodesResponse{Codes: out})
}

func isPinTypeAuth(raw sql.NullString) bool {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &m); err != nil {
		return false
	}
	v, ok := m["type"]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case float64:
		return int(t) == 13
	case int:
		return t == 13
	case string:
		return strings.TrimSpace(t) == "13"
	default:
		return false
	}
}

func accountUserIDFromRaw(raw sql.NullString) *string {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &m); err != nil {
		return nil
	}
	v, ok := m["accountUserId"]
	if !ok || v == nil {
		return nil
	}
	switch vv := v.(type) {
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			return nil
		}
		return &s
	case float64:
		s := strconv.FormatInt(int64(vv), 10)
		return &s
	case int64:
		s := strconv.FormatInt(vv, 10)
		return &s
	default:
		return nil
	}
}

func (s *Server) generateNukiCodes(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	type body struct {
		OccupancyID *int64  `json:"occupancy_id"`
		PinName     *string `json:"pin_name"`
	}
	var b body
	if err := ReadJSON(r, &b); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var genErr error
	if b.OccupancyID != nil {
		if b.PinName == nil || strings.TrimSpace(*b.PinName) == "" {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "pin_name required"})
			return
		}
		genErr = s.Nuki.GenerateCodeForOccupancy(r.Context(), pid, *b.OccupancyID, "generate_one", strings.TrimSpace(*b.PinName))
	} else {
		genErr = s.Nuki.GenerateCodes(r.Context(), pid, "generate_all")
	}
	if genErr != nil {
		WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: genErr.Error()})
		return
	}
	// Refresh keypad cache so generated PIN value can be surfaced if provider returns it only in listing.
	_ = s.Nuki.SyncProperty(r.Context(), pid, "after_generate_refresh")
	s.audit(r, actor, "nuki_generate", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) runNukiSync(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	if err := s.Nuki.SyncProperty(r.Context(), pid, "manual"); err != nil {
		WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: err.Error()})
		return
	}
	s.audit(r, actor, "nuki_sync", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) listNukiUpcomingStays(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelRead)
	if !ok {
		return
	}
	var err error
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Store.ListUpcomingStaysForNuki(r.Context(), pid, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]nukiUpcomingStayRow, 0, len(rows))
	for _, row := range rows {
		summary := nullStringPtr(row.GuestDisplayName)
		if summary == nil {
			summary = nullStringPtr(row.RawSummary)
		}
		out = append(out, nukiUpcomingStayRow{
			OccupancyID:         row.OccupancyID,
			SourceEventUID:      row.SourceEventUID,
			Summary:             summary,
			SavedPinName:        nullStringPtr(row.GuestDisplayName),
			StartAt:             row.StartAt.UTC().Format(time.RFC3339),
			EndAt:               row.EndAt.UTC().Format(time.RFC3339),
			OccupancyStatus:     row.OccupancyStatus,
			GeneratedCodeID:     nullInt64Ptr(row.GeneratedCodeID),
			GeneratedLabel:      nullStringPtr(row.GeneratedLabel),
			GeneratedStatus:     nullStringPtr(row.GeneratedStatus),
			GeneratedMasked:     nullStringPtr(row.GeneratedMasked),
			GeneratedValidFrom:  nullTimePtr(row.GeneratedValidFrom),
			GeneratedValidUntil: nullTimePtr(row.GeneratedValidUntil),
			GeneratedError:      nullStringPtr(row.GeneratedError),
			GeneratedUpdatedAt:  nullTimePtr(row.GeneratedUpdated),
		})
	}
	WriteJSON(w, http.StatusOK, nukiUpcomingStaysResponse{Stays: out})
}

func (s *Server) saveNukiStayName(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelWrite)
	if !ok {
		return
	}
	occupancyID, err := strconv.ParseInt(chi.URLParam(r, "occupancyId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid occupancy id")
		return
	}
	var body struct {
		PinName *string `json:"pin_name"`
	}
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.PinName == nil {
		WriteError(w, http.StatusBadRequest, "pin_name required")
		return
	}
	if err := s.Store.UpdateOccupancyGuestDisplayName(r.Context(), pid, occupancyID, body.PinName); err != nil {
		if s.Store.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	trimmed := strings.TrimSpace(*body.PinName)
	var saved *string
	if trimmed != "" {
		saved = &trimmed
	}
	s.audit(r, actor, "nuki_save_stay_name", "occupancy", strconv.FormatInt(occupancyID, 10), "success")
	WriteJSON(w, http.StatusOK, struct {
		OK           bool    `json:"ok"`
		SavedPinName *string `json:"saved_pin_name,omitempty"`
	}{OK: true, SavedPinName: saved})
}

func (s *Server) revokeNukiCode(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	codeID, err := strconv.ParseInt(chi.URLParam(r, "codeId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid code id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.NukiAccess, permissions.LevelWrite)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	if err := s.Nuki.RevokeCode(r.Context(), pid, codeID, "manual_revoke"); err != nil {
		if s.Store.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, actor, "nuki_revoke", "nuki_access_code", strconv.FormatInt(codeID, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}

// revealNukiCodePIN returns the plaintext guest PIN for a single Nuki access
// code and writes an audit entry every time it is invoked. The list/upcoming
// endpoints no longer include the PIN; this handler is the only way for the UI
// to obtain it after generation, and it requires the stricter write-level
// permission so read-only viewers cannot enumerate PINs.
func (s *Server) revealNukiCodePIN(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	codeID, err := strconv.ParseInt(chi.URLParam(r, "codeId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid code id")
		return
	}
	can, err := s.Store.UserCan(r.Context(), actor, pid, permissions.NukiAccess, permissions.LevelWrite)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	code, err := s.Store.GetNukiCodeByID(r.Context(), pid, codeID)
	if err != nil {
		if s.Store.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	if code == nil || !code.GeneratedPINPlain.Valid || strings.TrimSpace(code.GeneratedPINPlain.String) == "" {
		s.audit(r, actor, "nuki_reveal_pin", "nuki_access_code", strconv.FormatInt(codeID, 10), "empty")
		WriteError(w, http.StatusNotFound, "pin not available")
		return
	}
	s.audit(r, actor, "nuki_reveal_pin", "nuki_access_code", strconv.FormatInt(codeID, 10), "success")
	WriteJSON(w, http.StatusOK, struct {
		PIN string `json:"pin"`
	}{PIN: code.GeneratedPINPlain.String})
}

func (s *Server) deleteNukiKeypadCode(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	externalID := chi.URLParam(r, "externalId")
	if externalID == "" {
		WriteError(w, http.StatusBadRequest, "external id required")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.NukiAccess, permissions.LevelWrite)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	if err := s.Nuki.DeleteKeypadCode(r.Context(), pid, externalID, "manual_delete"); err != nil {
		if s.Store.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, actor, "nuki_delete_keypad_code", "nuki_keypad_code", externalID, "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) patchNukiKeypadCode(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	externalID := chi.URLParam(r, "externalId")
	if externalID == "" {
		WriteError(w, http.StatusBadRequest, "external id required")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.NukiAccess, permissions.LevelWrite)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Nuki == nil {
		WriteError(w, http.StatusInternalServerError, "nuki service not configured")
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Enabled == nil {
		WriteError(w, http.StatusBadRequest, "enabled required")
		return
	}
	if err := s.Nuki.SetKeypadCodeEnabled(r.Context(), pid, externalID, *body.Enabled, "manual_toggle"); err != nil {
		if s.Store.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, actor, "nuki_toggle_keypad_code", "nuki_keypad_code", externalID, "success")
	WriteJSON(w, http.StatusOK, struct {
		OK      bool `json:"ok"`
		Enabled bool `json:"enabled"`
	}{OK: true, Enabled: *body.Enabled})
}

func (s *Server) listNukiRuns(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.NukiAccess, permissions.LevelRead)
	if !ok {
		return
	}
	var err error
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	rows, err := s.Store.ListNukiSyncRuns(r.Context(), pid, limit+1, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]nukiRunRow, 0, len(rows))
	for _, rr := range rows {
		out = append(out, nukiRunRow{
			ID:             rr.ID,
			StartedAt:      rr.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:     nullTimePtr(rr.FinishedAt),
			Status:         rr.Status,
			Trigger:        rr.Trigger,
			ErrorMessage:   nullStringPtr(rr.ErrorMessage),
			ProcessedCount: rr.ProcessedCount,
			CreatedCount:   rr.CreatedCount,
			UpdatedCount:   rr.UpdatedCount,
			RevokedCount:   rr.RevokedCount,
			FailedCount:    rr.FailedCount,
		})
	}
	WriteJSON(w, http.StatusOK, nukiRunsResponse{
		Runs:    out,
		Page:    page,
		Limit:   limit,
		HasMore: hasMore,
	})
}
