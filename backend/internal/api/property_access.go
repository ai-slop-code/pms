package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/ctxuser"
	"pms/backend/internal/store"
)

func parsePropertyIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	pid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return pid, true
}

func (s *Server) requirePropertyModuleAccess(w http.ResponseWriter, r *http.Request, module, minLevel string) (*store.User, int64, bool) {
	actor := ctxuser.From(r.Context())
	pid, ok := parsePropertyIDParam(w, r)
	if !ok {
		return nil, 0, false
	}
	can, _ := s.Store.UserCan(r.Context(), actor, pid, module, minLevel)
	if !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return nil, 0, false
	}
	return actor, pid, true
}
