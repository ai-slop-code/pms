package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/ctxuser"
	"pms/backend/internal/permissions"
)

type occupancySourceDTO struct {
	PropertyID int64  `json:"property_id"`
	SourceType string `json:"source_type"`
	Active     bool   `json:"active"`
}

type occupancySourceResponse struct {
	Source occupancySourceDTO `json:"source"`
}

type occupancyRunsResponse struct {
	Runs    []occupancySyncRunRow `json:"runs"`
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	HasMore bool                  `json:"has_more"`
}

type occupancySyncRunRow struct {
	ID                         int64   `json:"id"`
	StartedAt                  string  `json:"started_at"`
	FinishedAt                 *string `json:"finished_at,omitempty"`
	Status                     string  `json:"status"`
	ErrorMessage               *string `json:"error_message,omitempty"`
	EventsSeen                 int     `json:"events_seen"`
	HTTPStatus                 *int    `json:"http_status,omitempty"`
	Trigger                    string  `json:"trigger"`
	RawBlocksInserted          int     `json:"raw_blocks_inserted"`
	RawBlocksUpdated           int     `json:"raw_blocks_updated"`
	RawBlocksUnchanged         int     `json:"raw_blocks_unchanged"`
	RawBlocksDeletedFromSource int     `json:"raw_blocks_deleted_from_source"`
	RawBlockConflicts          int     `json:"raw_block_conflicts"`
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
	if err := s.Occ.SyncProperty(r.Context(), id, "manual"); err != nil {
		WriteJSON(w, http.StatusOK, actionResponse{OK: false, Error: err.Error()})
		return
	}
	s.audit(r, actor, "occupancy_sync", "property", strconv.FormatInt(id, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) listOccupancySyncRuns(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
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
		var finishedAt *string
		if run.FinishedAt.Valid {
			value := run.FinishedAt.Time.UTC().Format(time.RFC3339)
			finishedAt = &value
		}
		var errorMessage *string
		if run.ErrorMessage.Valid {
			errorMessage = &run.ErrorMessage.String
		}
		var httpStatus *int
		if run.HTTPStatus.Valid {
			value := int(run.HTTPStatus.Int64)
			httpStatus = &value
		}
		out = append(out, occupancySyncRunRow{
			ID:                         run.ID,
			StartedAt:                  run.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:                 finishedAt,
			Status:                     run.Status,
			ErrorMessage:               errorMessage,
			EventsSeen:                 run.EventsSeen,
			HTTPStatus:                 httpStatus,
			Trigger:                    run.Trigger,
			RawBlocksInserted:          run.RawBlocksInserted,
			RawBlocksUpdated:           run.RawBlocksUpdated,
			RawBlocksUnchanged:         run.RawBlocksUnchanged,
			RawBlocksDeletedFromSource: run.RawBlocksDeletedFromSource,
			RawBlockConflicts:          run.RawBlockConflicts,
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
