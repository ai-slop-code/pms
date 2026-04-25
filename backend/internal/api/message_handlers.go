package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

type messageTemplateDTO struct {
	ID           int64  `json:"id"`
	PropertyID   int64  `json:"property_id"`
	LanguageCode string `json:"language_code"`
	TemplateType string `json:"template_type"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func templateToDTO(t *store.MessageTemplate) messageTemplateDTO {
	return messageTemplateDTO{
		ID:           t.ID,
		PropertyID:   t.PropertyID,
		LanguageCode: t.LanguageCode,
		TemplateType: t.TemplateType,
		Title:        t.Title,
		Body:         t.Body,
		Active:       t.Active,
		CreatedAt:    t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) listMessageTemplates(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelRead)
	if !ok {
		return
	}
	if err := s.Store.EnsureDefaultMessageTemplates(r.Context(), pid); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to initialize templates")
		return
	}
	list, err := s.Store.ListMessageTemplates(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]messageTemplateDTO, 0, len(list))
	for _, t := range list {
		out = append(out, templateToDTO(&t))
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"templates":            out,
		"supported_languages":  store.SupportedMessageLanguages,
		"supported_placeholders": store.AllPlaceholders,
	})
}

type createTemplateBody struct {
	LanguageCode string `json:"language_code"`
	TemplateType string `json:"template_type"`
	Title        string `json:"title"`
	Body         string `json:"body"`
}

func (s *Server) postMessageTemplate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelWrite)
	if !ok {
		return
	}
	var body createTemplateBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.LanguageCode == "" || body.Title == "" {
		WriteError(w, http.StatusBadRequest, "language_code and title required")
		return
	}
	if body.TemplateType == "" {
		body.TemplateType = "check_in"
	}
	if body.Body == "" {
		body.Body = "{{property_name}}\n\n"
	}
	if invalid := store.ValidateTemplatePlaceholders(body.Body); len(invalid) > 0 {
		WriteError(w, http.StatusBadRequest, "unsupported placeholders: "+joinStrings(invalid))
		return
	}
	t, err := s.Store.CreateMessageTemplate(r.Context(), pid, body.LanguageCode, body.TemplateType, body.Title, body.Body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "create", "message_template", strconv.FormatInt(t.ID, 10), "success")
	WriteJSON(w, http.StatusCreated, map[string]interface{}{"template": templateToDTO(t)})
}

func (s *Server) deleteMessageTemplate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelWrite)
	if !ok {
		return
	}
	tid, err := strconv.ParseInt(chi.URLParam(r, "templateId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	if err := s.Store.DeleteMessageTemplate(r.Context(), pid, tid); err != nil {
		WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	s.audit(r, actor, "delete", "message_template", strconv.FormatInt(tid, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}

type patchTemplateBody struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Active *bool   `json:"active"`
}

func (s *Server) patchMessageTemplate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelWrite)
	if !ok {
		return
	}
	tid, err := strconv.ParseInt(chi.URLParam(r, "templateId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	var body patchTemplateBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Body != nil {
		if invalid := store.ValidateTemplatePlaceholders(*body.Body); len(invalid) > 0 {
			WriteError(w, http.StatusBadRequest, "unsupported placeholders: "+joinStrings(invalid))
			return
		}
	}
	t, err := s.Store.UpdateMessageTemplate(r.Context(), pid, tid, body.Title, body.Body, body.Active)
	if err != nil {
		WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	s.audit(r, actor, "update", "message_template", strconv.FormatInt(tid, 10), "success")
	WriteJSON(w, http.StatusOK, map[string]interface{}{"template": templateToDTO(t)})
}

func (s *Server) generateMessage(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelRead)
	if !ok {
		return
	}
	occIDStr := r.URL.Query().Get("occupancy_id")
	if occIDStr == "" {
		WriteError(w, http.StatusBadRequest, "occupancy_id required")
		return
	}
	occID, err := strconv.ParseInt(occIDStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid occupancy_id")
		return
	}
	vals, err := s.Store.BuildPlaceholderValues(r.Context(), pid, occID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "occupancy not found or data incomplete")
		return
	}

	if err := s.Store.EnsureDefaultMessageTemplates(r.Context(), pid); err != nil {
		WriteError(w, http.StatusInternalServerError, "template initialization failed")
		return
	}
	templates, err := s.Store.ListMessageTemplates(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}

	nukiAvailable := vals.NukiCode != "—"

	type renderedMessage struct {
		LanguageCode  string `json:"language_code"`
		Title         string `json:"title"`
		Body          string `json:"body"`
		NukiAvailable bool   `json:"nuki_available"`
	}
	out := make([]renderedMessage, 0, len(templates))
	for _, t := range templates {
		if !t.Active {
			continue
		}
		if t.TemplateType != "" && t.TemplateType != store.TemplateTypeCheckIn {
			continue
		}
		out = append(out, renderedMessage{
			LanguageCode:  t.LanguageCode,
			Title:         t.Title,
			Body:          store.RenderMessageTemplate(t.Body, *vals),
			NukiAvailable: nukiAvailable,
		})
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"occupancy_id":  occID,
		"messages":      out,
		"nuki_available": nukiAvailable,
		"placeholders":  vals,
	})
}

func (s *Server) generateCleaningMessage(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Messages, permissions.LevelRead)
	if !ok {
		return
	}
	if err := s.Store.EnsureDefaultMessageTemplates(r.Context(), pid); err != nil {
		WriteError(w, http.StatusInternalServerError, "template initialization failed")
		return
	}
	tpl, err := s.Store.GetCleaningStaffTemplate(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "cleaning template not found")
		return
	}
	vals, staysCount, err := s.Store.BuildCleaningPlaceholderValues(r.Context(), pid, time.Now())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to build cleaning message")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"language_code": tpl.LanguageCode,
		"title":         tpl.Title,
		"body":          store.RenderMessageTemplate(tpl.Body, *vals),
		"stays_count":   staysCount,
	})
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
