package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// getHealthz is a pure liveness probe. It answers 200 as long as the process
// is scheduled and can run a handler; it does NOT touch the database, so a
// corrupt or missing DB will not fail the liveness check (preventing crash
// loops from triggering pod replacement on transient issues).
func (s *Server) getHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// getReadyz is a readiness probe. It returns 200 only when the database is
// reachable. We deliberately keep this narrow — the server can accept traffic
// once the primary dependency is healthy, even if a scheduler is lagging.
// Scheduler lag is surfaced separately via Prometheus gauges.
func (s *Server) getReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.Store == nil || s.Store.DB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "reason": "no database"})
		return
	}
	if err := s.Store.DB.PingContext(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "reason": "db unreachable"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
