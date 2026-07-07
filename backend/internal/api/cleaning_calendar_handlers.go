package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

type cleaningCalendarSettingsDTO struct {
	Enabled                bool    `json:"enabled"`
	CalendarID             *string `json:"calendar_id"`
	DefaultDurationMinutes int     `json:"default_duration_minutes"`
	TitlePrefix            string  `json:"title_prefix"`
	SameDayLabel           string  `json:"same_day_label"`
	NoGuestLabel           string  `json:"no_guest_label"`
	ConnectedAccountID     *string `json:"connected_account_id"`
	GoogleClientConfigured bool    `json:"google_client_configured"`
	UpdatedAt              *string `json:"updated_at"`
}

type cleaningCalendarSettingsResponse struct {
	Settings cleaningCalendarSettingsDTO `json:"settings"`
}

type cleaningCalendarEventDTO struct {
	ID               int64   `json:"id"`
	OccupancyID      int64   `json:"occupancy_id"`
	GoogleCalendarID string  `json:"google_calendar_id"`
	GoogleEventID    *string `json:"google_event_id"`
	CleaningDate     string  `json:"cleaning_date"`
	StartsAt         string  `json:"starts_at"`
	EndsAt           string  `json:"ends_at"`
	SameDayArrival   bool    `json:"same_day_arrival"`
	NextOccupancyID  *int64  `json:"next_occupancy_id"`
	Title            string  `json:"title"`
	Status           string  `json:"status"`
	WarningMessage   *string `json:"warning_message"`
	ErrorMessage     *string `json:"error_message"`
	LastSyncedAt     *string `json:"last_synced_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type cleaningCalendarEventsResponse struct {
	Month  string                     `json:"month"`
	Events []cleaningCalendarEventDTO `json:"events"`
}

type cleaningCalendarReconcileResponse struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Stats interface{} `json:"stats,omitempty"`
}

type cleaningCalendarRunDTO struct {
	ID             int64   `json:"id"`
	StartedAt      string  `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
	Status         string  `json:"status"`
	ErrorMessage   *string `json:"error_message"`
	EventsSeen     int     `json:"events_seen"`
	EventsUpserted int     `json:"events_upserted"`
	EventsRemoved  int     `json:"events_removed"`
	Trigger        string  `json:"trigger"`
}

type cleaningCalendarRunsResponse struct {
	Runs []cleaningCalendarRunDTO `json:"runs"`
}

func (s *Server) getCleaningCalendarSettings(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	settings, err := s.Store.GetGoogleCleaningSettings(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	WriteJSON(w, http.StatusOK, cleaningCalendarSettingsResponse{Settings: s.cleaningCalendarSettingsDTO(settings)})
}

func (s *Server) patchCleaningCalendarSettings(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelAdmin)
	if !ok {
		return
	}
	var body struct {
		Enabled                *bool   `json:"enabled"`
		CalendarID             *string `json:"calendar_id"`
		DefaultDurationMinutes *int    `json:"default_duration_minutes"`
		TitlePrefix            *string `json:"title_prefix"`
		SameDayLabel           *string `json:"same_day_label"`
		NoGuestLabel           *string `json:"no_guest_label"`
		ConnectedAccountID     *string `json:"connected_account_id"`
	}
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DefaultDurationMinutes != nil && *body.DefaultDurationMinutes <= 0 {
		WriteError(w, http.StatusBadRequest, "default_duration_minutes must be positive")
		return
	}
	settings, err := s.Store.UpdateGoogleCleaningSettings(r.Context(), pid, store.CleaningCalendarSettingsPatch{
		Enabled:                body.Enabled,
		CalendarID:             body.CalendarID,
		DefaultDurationMinutes: body.DefaultDurationMinutes,
		TitlePrefix:            body.TitlePrefix,
		SameDayLabel:           body.SameDayLabel,
		NoGuestLabel:           body.NoGuestLabel,
		ConnectedAccountID:     body.ConnectedAccountID,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "cleaning_calendar_settings_update", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, cleaningCalendarSettingsResponse{Settings: s.cleaningCalendarSettingsDTO(settings)})
}

func (s *Server) listCleaningCalendarEvents(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.Store.ListCleaningCalendarEventsForMonth(r.Context(), pid, month)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]cleaningCalendarEventDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, cleaningCalendarEventDTOFromStore(row))
	}
	WriteJSON(w, http.StatusOK, cleaningCalendarEventsResponse{Month: month, Events: out})
}

func (s *Server) runCleaningCalendarReconcile(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.CleaningCalendar == nil {
		WriteError(w, http.StatusInternalServerError, "cleaning calendar service not configured")
		return
	}
	stats, err := s.CleaningCalendar.ReconcileProperty(r.Context(), pid, "manual")
	if err != nil {
		WriteJSON(w, http.StatusOK, cleaningCalendarReconcileResponse{OK: false, Error: err.Error(), Stats: stats})
		return
	}
	s.audit(r, actor, "cleaning_calendar_reconcile", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, cleaningCalendarReconcileResponse{OK: true, Stats: stats})
}

func (s *Server) retryCleaningCalendarEvent(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.CleaningCalendar == nil {
		WriteError(w, http.StatusInternalServerError, "cleaning calendar service not configured")
		return
	}
	eventID, err := strconv.ParseInt(chi.URLParam(r, "eventId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	if err := s.CleaningCalendar.RetryEvent(r.Context(), pid, eventID); err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, actor, "cleaning_calendar_event_retry", "cleaning_calendar_event", strconv.FormatInt(eventID, 10), "success")
	WriteJSON(w, http.StatusOK, actionResponse{OK: true})
}

func (s *Server) listCleaningCalendarRuns(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	runs, err := s.Store.ListCleaningCalendarSyncRuns(r.Context(), pid, 20)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]cleaningCalendarRunDTO, 0, len(runs))
	for _, row := range runs {
		out = append(out, cleaningCalendarRunDTO{
			ID:             row.ID,
			StartedAt:      row.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:     nullTimePtr(row.FinishedAt),
			Status:         row.Status,
			ErrorMessage:   nullStringPtr(row.ErrorMessage),
			EventsSeen:     row.EventsSeen,
			EventsUpserted: row.EventsUpserted,
			EventsRemoved:  row.EventsRemoved,
			Trigger:        row.Trigger,
		})
	}
	WriteJSON(w, http.StatusOK, cleaningCalendarRunsResponse{Runs: out})
}

func (s *Server) getCleaningCalendarGoogleCalendars(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelRead)
	if !ok {
		return
	}
	configured := s.CleaningCalendar != nil && s.CleaningCalendar.Client != nil && s.CleaningCalendar.Client.Configured()
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"google_client_configured": configured,
		"calendars":                []interface{}{},
		"note":                     "Calendar listing is not available for service-account mode; enter the calendar ID shared with the service account.",
	})
}

func (s *Server) postCleaningCalendarGoogleConnect(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelAdmin)
	if !ok {
		return
	}
	configured := s.CleaningCalendar != nil && s.CleaningCalendar.Client != nil && s.CleaningCalendar.Client.Configured()
	if !configured {
		WriteError(w, http.StatusBadRequest, "google service account not configured")
		return
	}
	account := "service_account"
	settings, err := s.Store.UpdateGoogleCleaningSettings(r.Context(), pid, store.CleaningCalendarSettingsPatch{ConnectedAccountID: &account})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "cleaning_calendar_google_connect", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, cleaningCalendarSettingsResponse{Settings: s.cleaningCalendarSettingsDTO(settings)})
}

func (s *Server) postCleaningCalendarGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.CleaningLog, permissions.LevelAdmin)
	if !ok {
		return
	}
	enabled := false
	blank := ""
	settings, err := s.Store.UpdateGoogleCleaningSettings(r.Context(), pid, store.CleaningCalendarSettingsPatch{Enabled: &enabled, ConnectedAccountID: &blank})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "cleaning_calendar_google_disconnect", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, cleaningCalendarSettingsResponse{Settings: s.cleaningCalendarSettingsDTO(settings)})
}

func (s *Server) cleaningCalendarSettingsDTO(settings *store.GoogleCleaningSettings) cleaningCalendarSettingsDTO {
	var updated *string
	if !settings.UpdatedAt.IsZero() {
		v := settings.UpdatedAt.UTC().Format(time.RFC3339)
		updated = &v
	}
	configured := s.CleaningCalendar != nil && s.CleaningCalendar.Client != nil && s.CleaningCalendar.Client.Configured()
	return cleaningCalendarSettingsDTO{
		Enabled:                settings.Enabled,
		CalendarID:             nullStringPtr(settings.CalendarID),
		DefaultDurationMinutes: settings.DefaultDurationMinutes,
		TitlePrefix:            settings.TitlePrefix,
		SameDayLabel:           settings.SameDayLabel,
		NoGuestLabel:           settings.NoGuestLabel,
		ConnectedAccountID:     nullStringPtr(settings.ConnectedAccountID),
		GoogleClientConfigured: configured,
		UpdatedAt:              updated,
	}
}

func cleaningCalendarEventDTOFromStore(row store.CleaningCalendarEvent) cleaningCalendarEventDTO {
	var next *int64
	if row.NextOccupancyID.Valid {
		v := row.NextOccupancyID.Int64
		next = &v
	}
	return cleaningCalendarEventDTO{
		ID:               row.ID,
		OccupancyID:      row.OccupancyID,
		GoogleCalendarID: row.GoogleCalendarID,
		GoogleEventID:    nullStringPtr(row.GoogleEventID),
		CleaningDate:     row.CleaningDate,
		StartsAt:         row.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:           row.EndsAt.UTC().Format(time.RFC3339),
		SameDayArrival:   row.SameDayArrival,
		NextOccupancyID:  next,
		Title:            row.Title,
		Status:           row.Status,
		WarningMessage:   nullStringPtr(row.WarningMessage),
		ErrorMessage:     nullStringPtr(row.ErrorMessage),
		LastSyncedAt:     nullTimePtr(row.LastSyncedAt),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
