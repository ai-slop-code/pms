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

type occupancyCalendarV2Response struct {
	Calendar *store.OccupancyCalendarView `json:"calendar"`
}

type calendarRawBlocksResponse struct {
	RawBlocks []store.CalendarRawBookingBlock `json:"raw_blocks"`
}

type calendarNamedStaysResponse struct {
	NamedStays []store.CalendarNamedStay `json:"named_stays"`
}

type calendarAvailabilityBlocksResponse struct {
	AvailabilityBlocks []store.CalendarAvailabilityBlock `json:"availability_blocks"`
}

type availabilityBlockBody struct {
	BlockType string `json:"block_type"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
}

type availabilityBlockMutationResponse struct {
	OK                bool                             `json:"ok"`
	AvailabilityBlock *store.CalendarAvailabilityBlock `json:"availability_block"`
}

func (s *Server) getOccupancyCalendarV2(w http.ResponseWriter, r *http.Request) {
	_, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month=YYYY-MM required")
		return
	}
	view, err := s.Store.OccupancyCalendarView(r.Context(), propID, month)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid month")
		return
	}
	WriteJSON(w, http.StatusOK, occupancyCalendarV2Response{Calendar: view})
}

func (s *Server) getBookingBlocks(w http.ResponseWriter, r *http.Request) {
	_, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month=YYYY-MM required")
		return
	}
	view, err := s.Store.OccupancyCalendarView(r.Context(), propID, month)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid month")
		return
	}
	WriteJSON(w, http.StatusOK, calendarRawBlocksResponse{RawBlocks: view.RawBlocks})
}

func (s *Server) getStays(w http.ResponseWriter, r *http.Request) {
	_, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month=YYYY-MM required")
		return
	}
	view, err := s.Store.OccupancyCalendarView(r.Context(), propID, month)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid month")
		return
	}
	WriteJSON(w, http.StatusOK, calendarNamedStaysResponse{NamedStays: view.NamedStays})
}

func (s *Server) getAvailabilityBlocks(w http.ResponseWriter, r *http.Request) {
	_, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelRead)
	if !ok {
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		WriteError(w, http.StatusBadRequest, "month=YYYY-MM required")
		return
	}
	view, err := s.Store.OccupancyCalendarView(r.Context(), propID, month)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid month")
		return
	}
	WriteJSON(w, http.StatusOK, calendarAvailabilityBlocksResponse{AvailabilityBlocks: view.AvailabilityBlocks})
}

func (s *Server) postAvailabilityBlock(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	var body availabilityBlockBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	block, err := s.Store.CreateAvailabilityBlock(r.Context(), propID, store.AvailabilityBlockInput{
		BlockType:    body.BlockType,
		StartDate:    body.StartDate,
		EndDate:      body.EndDate,
		Reason:       body.Reason,
		ActingUserID: actor.ID,
	})
	if err != nil {
		writeAvailabilityBlockError(w, err)
		return
	}
	s.audit(r, actor, "availability_block_created", "property_availability_block", strconv.FormatInt(block.ID, 10), "success")
	WriteJSON(w, http.StatusOK, availabilityBlockMutationResponse{OK: true, AvailabilityBlock: block})
}

func (s *Server) patchAvailabilityBlock(w http.ResponseWriter, r *http.Request) {
	actor, propID, ok := s.requirePropertyModuleAccess(w, r, permissions.Occupancy, permissions.LevelWrite)
	if !ok {
		return
	}
	blockID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "blockId")), 10, 64)
	if err != nil || blockID <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid availability block id")
		return
	}
	var body availabilityBlockBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	block, err := s.Store.UpdateAvailabilityBlock(r.Context(), propID, blockID, store.AvailabilityBlockInput{
		BlockType:    body.BlockType,
		StartDate:    body.StartDate,
		EndDate:      body.EndDate,
		Reason:       body.Reason,
		ActingUserID: actor.ID,
	})
	if err != nil {
		writeAvailabilityBlockError(w, err)
		return
	}
	s.audit(r, actor, "availability_block_updated", "property_availability_block", strconv.FormatInt(block.ID, 10), "success")
	WriteJSON(w, http.StatusOK, availabilityBlockMutationResponse{OK: true, AvailabilityBlock: block})
}

func writeAvailabilityBlockError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNamedStayOverlap):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNamedStayInvalidRange):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "not found")
	default:
		WriteError(w, http.StatusInternalServerError, "availability block update failed")
	}
}
