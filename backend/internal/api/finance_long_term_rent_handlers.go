package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

type financeLongTermRentRateRow struct {
	ID                 int64  `json:"id"`
	EffectiveFromMonth string `json:"effective_from_month"`
	MonthlyRentCents   int64  `json:"monthly_rent_cents"`
	Currency           string `json:"currency"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type financeLongTermRentRatesResponse struct {
	Rates     []financeLongTermRentRateRow `json:"rates"`
	CanManage bool                         `json:"can_manage"`
}

type financeLongTermRentRateRequest struct {
	EffectiveFromMonth string `json:"effective_from_month"`
	MonthlyRentCents   int64  `json:"monthly_rent_cents"`
}

func financeLongTermRentRateToRow(rate store.FinanceLongTermRentRate) financeLongTermRentRateRow {
	return financeLongTermRentRateRow{ID: rate.ID, EffectiveFromMonth: rate.EffectiveFromMonth,
		MonthlyRentCents: rate.MonthlyRentCents, Currency: rate.Currency,
		CreatedAt: rate.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: rate.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
}

func validateFinanceLongTermRentRate(body financeLongTermRentRateRequest) error {
	if _, _, err := parseFinanceMonth(body.EffectiveFromMonth); err != nil {
		return err
	}
	if body.MonthlyRentCents <= 0 || body.MonthlyRentCents > 9007199254740991 {
		return fmt.Errorf("monthly_rent_cents must be positive")
	}
	return nil
}

func (s *Server) listFinanceLongTermRentRates(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelRead)
	if !ok {
		return
	}
	rates, err := s.Store.ListFinanceLongTermRentRates(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	canManage, err := s.Store.UserCan(r.Context(), actor, pid, permissions.Finance, permissions.LevelAdmin)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	rows := make([]financeLongTermRentRateRow, 0, len(rates))
	for _, rate := range rates {
		rows = append(rows, financeLongTermRentRateToRow(rate))
	}
	WriteJSON(w, http.StatusOK, financeLongTermRentRatesResponse{Rates: rows, CanManage: canManage})
}

func (s *Server) postFinanceLongTermRentRate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelAdmin)
	if !ok {
		return
	}
	var body financeLongTermRentRateRequest
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.EffectiveFromMonth = strings.TrimSpace(body.EffectiveFromMonth)
	if err := validateFinanceLongTermRentRate(body); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rate, err := s.Store.CreateFinanceLongTermRentRate(r.Context(), pid, body.EffectiveFromMonth, body.MonthlyRentCents)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			WriteError(w, http.StatusConflict, "effective month already has a rate")
			return
		}
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.audit(r, actor, "finance_long_term_rent_rate_created", "finance_long_term_rent_rate", strconv.FormatInt(rate.ID, 10), "success")
	WriteJSON(w, http.StatusCreated, map[string]interface{}{"rate": financeLongTermRentRateToRow(*rate)})
}

func parseFinanceRateID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "rateId"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid rate id")
		return 0, false
	}
	return id, true
}

func (s *Server) patchFinanceLongTermRentRate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelAdmin)
	if !ok {
		return
	}
	rateID, ok := parseFinanceRateID(w, r)
	if !ok {
		return
	}
	var body financeLongTermRentRateRequest
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.EffectiveFromMonth = strings.TrimSpace(body.EffectiveFromMonth)
	if err := validateFinanceLongTermRentRate(body); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rate, err := s.Store.UpdateFinanceLongTermRentRate(r.Context(), pid, rateID, body.EffectiveFromMonth, body.MonthlyRentCents)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "rate not found")
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			WriteError(w, http.StatusConflict, "effective month already has a rate")
			return
		}
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.audit(r, actor, "finance_long_term_rent_rate_updated", "finance_long_term_rent_rate", strconv.FormatInt(rateID, 10), "success")
	WriteJSON(w, http.StatusOK, map[string]interface{}{"rate": financeLongTermRentRateToRow(*rate)})
}

func (s *Server) deleteFinanceLongTermRentRate(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelAdmin)
	if !ok {
		return
	}
	rateID, ok := parseFinanceRateID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteFinanceLongTermRentRate(r.Context(), pid, rateID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "rate not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.audit(r, actor, "finance_long_term_rent_rate_deleted", "finance_long_term_rent_rate", strconv.FormatInt(rateID, 10), "success")
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
