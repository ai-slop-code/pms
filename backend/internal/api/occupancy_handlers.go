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

	"pms/backend/internal/auth"
	"pms/backend/internal/ctxuser"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

// exportTokenSource indicates where a token value came from, so we can apply
// policy (deprecation warnings, audit logs) without changing validation logic.
type exportTokenSource int

const (
	exportTokenSourceNone exportTokenSource = iota
	exportTokenSourceAuthorizationBearer
	exportTokenSourceHeader
)

// extractExportToken pulls the automation token from the
// `Authorization: Bearer <token>` header or the `X-Export-Token` header.
// The legacy `?token=` query parameter was removed in PMS_11/T2.6 — it ended
// up in access/observability logs and reverse-proxy traces, defeating the
// credential's purpose.
func extractExportToken(r *http.Request) (string, exportTokenSource) {
	if v := strings.TrimSpace(r.Header.Get("Authorization")); v != "" {
		if parts := strings.SplitN(v, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if t := strings.TrimSpace(parts[1]); t != "" {
				return t, exportTokenSourceAuthorizationBearer
			}
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-Export-Token")); v != "" {
		return v, exportTokenSourceHeader
	}
	return "", exportTokenSourceNone
}

type occupancyRow struct {
	ID               int64  `json:"id"`
	PropertyID       int64  `json:"property_id"`
	SourceType       string `json:"source_type"`
	SourceEventUID   string `json:"source_event_uid"`
	StartAt          string `json:"start_at"`
	EndAt            string `json:"end_at"`
	Status           string `json:"status"`
	RawSummary       string `json:"raw_summary"`
	GuestDisplayName string `json:"guest_display_name"`
	LastSyncedAt     string `json:"last_synced_at"`
	LastSyncRunID    *int64 `json:"last_sync_run_id,omitempty"`
	ContentHash      string `json:"content_hash"`
	HasPayoutData    bool   `json:"has_payout_data"`
	// PMS_19 durable upstream identity + night-level truth.
	UpstreamSourceType *string  `json:"upstream_source_type,omitempty"`
	UpstreamEventUID   *string  `json:"upstream_event_uid,omitempty"`
	RepresentationKind *string  `json:"representation_kind,omitempty"`
	CoveredNights      []string `json:"covered_nights"`
	Superseded         bool     `json:"superseded"`
	// Closure / external-sale labelling (PMS_14). NULL JSON omitted.
	ClosureState                     *string `json:"closure_state,omitempty"`
	ClosureReason                    *string `json:"closure_reason,omitempty"`
	ClosureCategory                  *string `json:"closure_category,omitempty"`
	ClosedAt                         *string `json:"closed_at,omitempty"`
	ClosedByUserID                   *int64  `json:"closed_by_user_id,omitempty"`
	ExternalNetAmountCents           *int64  `json:"external_net_amount_cents,omitempty"`
	ExternalCurrency                 *string `json:"external_currency,omitempty"`
	ExternalChannel                  *string `json:"external_channel,omitempty"`
	StayOutcome                      *string `json:"stay_outcome,omitempty"`
	StayOutcomeReason                *string `json:"stay_outcome_reason,omitempty"`
	StayOutcomeMarkedAt              *string `json:"stay_outcome_marked_at,omitempty"`
	StayOutcomeMarkedByUserID        *int64  `json:"stay_outcome_marked_by_user_id,omitempty"`
	CleaningCalendarExcluded         bool    `json:"cleaning_calendar_excluded"`
	CleaningCalendarExclusionReason  *string `json:"cleaning_calendar_exclusion_reason,omitempty"`
	CleaningCalendarExcludedAt       *string `json:"cleaning_calendar_excluded_at,omitempty"`
	CleaningCalendarExcludedByUserID *int64  `json:"cleaning_calendar_excluded_by_user_id,omitempty"`
}

type occupancyListResponse struct {
	Occupancies []occupancyRow `json:"occupancies"`
}

type occupancySourceDTO struct {
	PropertyID int64  `json:"property_id"`
	SourceType string `json:"source_type"`
	Active     bool   `json:"active"`
}

type occupancySourceResponse struct {
	Source occupancySourceDTO `json:"source"`
}

type occupancyTokenCreateResponse struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

type occupancyTokenRow struct {
	ID         int64   `json:"id"`
	Label      *string `json:"label,omitempty"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

type occupancyTokensResponse struct {
	Tokens []occupancyTokenRow `json:"tokens"`
}

type occupancyRunsResponse struct {
	Runs    []occupancySyncRunRow `json:"runs"`
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	HasMore bool                  `json:"has_more"`
}

type occupancySyncRunRow struct {
	ID                               int64   `json:"id"`
	StartedAt                        string  `json:"started_at"`
	FinishedAt                       *string `json:"finished_at,omitempty"`
	Status                           string  `json:"status"`
	ErrorMessage                     *string `json:"error_message,omitempty"`
	EventsSeen                       int     `json:"events_seen"`
	OccupanciesUpserted              int     `json:"occupancies_upserted"`
	HTTPStatus                       *int    `json:"http_status,omitempty"`
	Trigger                          string  `json:"trigger"`
	DeletionEnabled                  bool    `json:"deletion_enabled"`
	RepresentationsDeletedFromSource int     `json:"representations_deleted_from_source"`
	DuplicateNightsResolved          int     `json:"duplicate_nights_resolved"`
	NamedStaysDeletedFromSource      int     `json:"named_stays_deleted_from_source"`
	RawBlocksInserted                int     `json:"raw_blocks_inserted"`
	RawBlocksUpdated                 int     `json:"raw_blocks_updated"`
	RawBlocksUnchanged               int     `json:"raw_blocks_unchanged"`
	RawBlocksDeletedFromSource       int     `json:"raw_blocks_deleted_from_source"`
	RawBlockConflicts                int     `json:"raw_block_conflicts"`
}

func (s *Server) getOccupancyExportPublic(w http.ResponseWriter, r *http.Request) {
	addDeprecatedAPIHeaders(w, "Deprecated public occupancy export; use native Google Calendar cleaning sync instead.")
	if s.OccupancyExportDisabled {
		WriteError(w, http.StatusGone, "occupancy export is disabled")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid property id")
		return
	}
	token, _ := extractExportToken(r)
	if token == "" {
		WriteError(w, http.StatusUnauthorized, "token required")
		return
	}
	hash := auth.HashSessionToken(token)
	ok, err := s.Store.ValidateOccupancyExportToken(r.Context(), id, hash)
	if err != nil || !ok {
		WriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	list, err := s.Store.ListOccupanciesForExport(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	type row struct {
		ID               int64  `json:"id"`
		PropertyID       int64  `json:"property_id"`
		PropertyName     string `json:"property_name"`
		SourceType       string `json:"source_type"`
		ExternalEventUID string `json:"external_event_uid"`
		StayStart        string `json:"stay_start"`
		StayEnd          string `json:"stay_end"`
		Status           string `json:"status"`
		RawSummary       string `json:"raw_summary,omitempty"`
		LastSyncedAt     string `json:"last_synced_at"`
		// Categories mirrors the iCal CATEGORIES field per PMS_14 §3.5
		// so downstream consumers can filter labelled nights without
		// leaking the operator-entered amount/channel.
		Categories []string `json:"categories,omitempty"`
	}
	out := make([]row, 0, len(list))
	for _, o := range list {
		rs := occupancySummary(o)
		var cats []string
		switch o.ClosureState.String {
		case "closed":
			cats = []string{"PMS-CLOSURE"}
		case "external_sale":
			cats = []string{"PMS-EXTERNAL-SALE"}
		}
		out = append(out, row{
			ID:               o.ID,
			PropertyID:       o.PropertyID,
			PropertyName:     prop.Name,
			SourceType:       o.SourceType,
			ExternalEventUID: o.SourceEventUID,
			StayStart:        o.StartAt.UTC().Format(time.RFC3339),
			StayEnd:          o.EndAt.UTC().Format(time.RFC3339),
			Status:           o.Status,
			RawSummary:       rs,
			LastSyncedAt:     o.LastSyncedAt.UTC().Format(time.RFC3339),
			Categories:       cats,
		})
	}
	WriteJSON(w, http.StatusOK, struct {
		Occupancies []row `json:"occupancies"`
	}{Occupancies: out})
}

func (s *Server) getOccupancies(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
	var err error
	prop, err := s.Store.GetProperty(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	month := r.URL.Query().Get("month")
	status := r.URL.Query().Get("status")
	var stPtr *string
	if status != "" {
		stPtr = &status
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := s.Store.ListOccupancies(r.Context(), id, month, loc, stPtr, limit, offset)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]int64, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	payoutMap, _ := s.Store.OccupancyIDsWithPayoutData(r.Context(), id, ids)
	coveredMap, _ := s.Store.CoveredNightsForOccupancies(r.Context(), ids)
	WriteJSON(w, http.StatusOK, occupancyListResponse{Occupancies: occupancyRows(list, payoutMap, coveredMap)})
}

func (s *Server) getOccupanciesCalendar(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("month") == "" {
		WriteError(w, http.StatusBadRequest, "month=YYYY-MM required")
		return
	}
	s.getOccupancies(w, r)
}

func occupancyRows(list []store.Occupancy, payoutMap map[int64]bool, coveredMap map[int64][]string) []occupancyRow {
	out := make([]occupancyRow, 0, len(list))
	for _, o := range list {
		rs := occupancySummary(o)
		covered := coveredMap[o.ID]
		if covered == nil {
			covered = []string{}
		}
		row := occupancyRow{
			ID:                       o.ID,
			PropertyID:               o.PropertyID,
			SourceType:               o.SourceType,
			SourceEventUID:           o.SourceEventUID,
			StartAt:                  o.StartAt.UTC().Format(time.RFC3339),
			EndAt:                    o.EndAt.UTC().Format(time.RFC3339),
			Status:                   o.Status,
			RawSummary:               rs,
			GuestDisplayName:         nullStringValue(o.GuestDisplayName),
			LastSyncedAt:             o.LastSyncedAt.UTC().Format(time.RFC3339),
			ContentHash:              o.ContentHash,
			HasPayoutData:            payoutMap[o.ID],
			CleaningCalendarExcluded: o.CleaningCalendarExcluded,
			CoveredNights:            covered,
			Superseded:               o.SupersededAt.Valid,
		}
		if o.LastSyncRunID.Valid {
			v := o.LastSyncRunID.Int64
			row.LastSyncRunID = &v
		}
		if o.UpstreamSourceType.Valid {
			row.UpstreamSourceType = optString(o.UpstreamSourceType.String)
		}
		if o.UpstreamEventUID.Valid {
			row.UpstreamEventUID = optString(o.UpstreamEventUID.String)
		}
		if o.RepresentationKind.Valid {
			row.RepresentationKind = optString(o.RepresentationKind.String)
		}
		if o.ClosureState.Valid {
			row.ClosureState = optString(o.ClosureState.String)
		}
		if o.ClosureReason.Valid {
			row.ClosureReason = optString(o.ClosureReason.String)
		}
		if o.ClosureCategory.Valid {
			row.ClosureCategory = optString(o.ClosureCategory.String)
		}
		if o.ClosedAt.Valid {
			s := o.ClosedAt.Time.UTC().Format(time.RFC3339)
			row.ClosedAt = &s
		}
		if o.ClosedByUserID.Valid {
			v := o.ClosedByUserID.Int64
			row.ClosedByUserID = &v
		}
		if o.ExternalNetAmountCents.Valid {
			v := o.ExternalNetAmountCents.Int64
			row.ExternalNetAmountCents = &v
		}
		if o.ExternalCurrency.Valid {
			row.ExternalCurrency = optString(o.ExternalCurrency.String)
		}
		if o.ExternalChannel.Valid {
			row.ExternalChannel = optString(o.ExternalChannel.String)
		}
		if o.StayOutcome.Valid {
			row.StayOutcome = optString(o.StayOutcome.String)
		}
		if o.StayOutcomeReason.Valid {
			row.StayOutcomeReason = optString(o.StayOutcomeReason.String)
		}
		if o.StayOutcomeMarkedAt.Valid {
			s := o.StayOutcomeMarkedAt.Time.UTC().Format(time.RFC3339)
			row.StayOutcomeMarkedAt = &s
		}
		if o.StayOutcomeMarkedByUserID.Valid {
			v := o.StayOutcomeMarkedByUserID.Int64
			row.StayOutcomeMarkedByUserID = &v
		}
		if o.CleaningCalendarExclusionReason.Valid {
			row.CleaningCalendarExclusionReason = optString(o.CleaningCalendarExclusionReason.String)
		}
		if o.CleaningCalendarExcludedAt.Valid {
			s := o.CleaningCalendarExcludedAt.Time.UTC().Format(time.RFC3339)
			row.CleaningCalendarExcludedAt = &s
		}
		if o.CleaningCalendarExcludedByUserID.Valid {
			v := o.CleaningCalendarExcludedByUserID.Int64
			row.CleaningCalendarExcludedByUserID = &v
		}
		out = append(out, row)
	}
	return out
}

func optString(s string) *string { return &s }

func nullStringValue(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func occupancySummary(o store.Occupancy) string {
	if o.GuestDisplayName.Valid && o.GuestDisplayName.String != "" {
		return o.GuestDisplayName.String
	}
	if o.RawSummary.Valid {
		return o.RawSummary.String
	}
	return ""
}

func (s *Server) postOccupancySyncRun(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.Occ == nil {
		WriteError(w, http.StatusInternalServerError, "sync not configured")
		return
	}
	err := s.Occ.SyncProperty(r.Context(), id, "manual")
	if err != nil {
		WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: err.Error()})
		return
	}
	if s.CleaningCalendar != nil {
		if _, err := s.CleaningCalendar.ReconcileProperty(r.Context(), id, "occupancy_sync"); err != nil {
			WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: "occupancy synced, cleaning calendar failed: " + err.Error()})
			return
		}
	}
	s.audit(r, actor, "occupancy_sync", "property", strconv.FormatInt(id, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) listOccupancySyncRuns(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
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
	runs, err := s.Store.ListOccupancySyncRunsPaged(r.Context(), id, limit+1, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	hasMore := len(runs) > limit
	if hasMore {
		runs = runs[:limit]
	}
	out := make([]occupancySyncRunRow, 0, len(runs))
	for _, run := range runs {
		var fin *string
		if run.FinishedAt.Valid {
			s := run.FinishedAt.Time.UTC().Format(time.RFC3339)
			fin = &s
		}
		var em *string
		if run.ErrorMessage.Valid {
			em = &run.ErrorMessage.String
		}
		var hs *int
		if run.HTTPStatus.Valid {
			v := int(run.HTTPStatus.Int64)
			hs = &v
		}
		out = append(out, occupancySyncRunRow{
			ID:                               run.ID,
			StartedAt:                        run.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:                       fin,
			Status:                           run.Status,
			ErrorMessage:                     em,
			EventsSeen:                       run.EventsSeen,
			OccupanciesUpserted:              run.OccupanciesUpserted,
			HTTPStatus:                       hs,
			Trigger:                          run.Trigger,
			DeletionEnabled:                  run.DeletionEnabled,
			RepresentationsDeletedFromSource: run.RepresentationsDeletedFromSource,
			DuplicateNightsResolved:          run.DuplicateNightsResolved,
			NamedStaysDeletedFromSource:      run.NamedStaysDeletedFromSource,
			RawBlocksInserted:                run.RawBlocksInserted,
			RawBlocksUpdated:                 run.RawBlocksUpdated,
			RawBlocksUnchanged:               run.RawBlocksUnchanged,
			RawBlocksDeletedFromSource:       run.RawBlocksDeletedFromSource,
			RawBlockConflicts:                run.RawBlockConflicts,
		})
	}
	WriteJSON(w, http.StatusOK, occupancyRunsResponse{Runs: out, Page: page, Limit: limit, HasMore: hasMore})
}

type patchOccupancySourceBody struct {
	Active     *bool   `json:"active"`
	SourceType *string `json:"source_type"`
}

func (s *Server) patchOccupancySource(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, id, permissions.Occupancy, permissions.LevelWrite)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body patchOccupancySourceBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.Store.UpdateOccupancySource(r.Context(), id, body.Active, body.SourceType); err != nil {
		WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.audit(r, actor, "update", "occupancy_source", strconv.FormatInt(id, 10), "success")
	src, err := s.Store.GetOccupancySource(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, occupancySourceResponse{
		Source: occupancySourceDTO{PropertyID: src.PropertyID, SourceType: src.SourceType, Active: src.Active},
	})
}

func (s *Server) getOccupancySource(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, id, permissions.Occupancy, permissions.LevelRead)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	src, err := s.Store.GetOccupancySource(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, occupancySourceResponse{
		Source: occupancySourceDTO{PropertyID: src.PropertyID, SourceType: src.SourceType, Active: src.Active},
	})
}

type createOccTokenBody struct {
	Label *string `json:"label"`
}

func (s *Server) postOccupancyAPIToken(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, id, permissions.Occupancy, permissions.LevelAdmin)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body createOccTokenBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "token error")
		return
	}
	tid, err := s.Store.CreateOccupancyAPIToken(r.Context(), id, hash, body.Label)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "create", "occupancy_api_token", strconv.FormatInt(tid, 10), "success")
	WriteJSON(w, http.StatusCreated, occupancyTokenCreateResponse{ID: tid, Token: raw})
}

func (s *Server) listOccupancyAPITokens(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, id, permissions.Occupancy, permissions.LevelRead)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	toks, err := s.Store.ListOccupancyAPITokens(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]occupancyTokenRow, 0, len(toks))
	for _, t := range toks {
		var lab *string
		if t.Label.Valid {
			lab = &t.Label.String
		}
		var lu *string
		if t.LastUsedAt.Valid {
			s := t.LastUsedAt.Time.UTC().Format(time.RFC3339)
			lu = &s
		}
		out = append(out, occupancyTokenRow{
			ID:         t.ID,
			Label:      lab,
			CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
			LastUsedAt: lu,
		})
	}
	WriteJSON(w, http.StatusOK, occupancyTokensResponse{Tokens: out})
}

func (s *Server) deleteOccupancyAPIToken(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid property id")
		return
	}
	tid, err := strconv.ParseInt(chi.URLParam(r, "tokenId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid token id")
		return
	}
	can, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.Occupancy, permissions.LevelAdmin)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.Store.DeleteOccupancyAPIToken(r.Context(), tid, pid); err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	s.audit(r, actor, "delete", "occupancy_api_token", strconv.FormatInt(tid, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}
