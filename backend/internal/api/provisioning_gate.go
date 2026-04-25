package api

import (
	"net/http"
	"strconv"
	"strings"

	"pms/backend/internal/ctxuser"
)

// ProvisioningGate funnels under-provisioned accounts into the
// password-rotation / 2FA-enrolment flow before they can touch any other
// API. Two cases trigger the gate:
//
//  1. user.MustChangePassword == true (e.g. bootstrap super_admin, or an
//     account whose password an admin has just reset).
//  2. user.Role == "super_admin" and the user has not yet enrolled in TOTP.
//     Super-admins are the highest-privilege accounts; we refuse to let
//     them operate without a second factor (PMS_11 follow-up #6).
//
// Allowlist below covers the endpoints required to *clear* the gate
// (rotate password, enrol 2FA), plus the read-only context endpoints the
// SPA needs to render the prompt screen and the logout endpoint.
func (s *Server) ProvisioningGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := ctxuser.From(r.Context())
		if u == nil {
			next.ServeHTTP(w, r)
			return
		}
		if isProvisioningAllowedRoute(r.Method, r.URL.Path, u.ID) {
			next.ServeHTTP(w, r)
			return
		}
		if u.MustChangePassword {
			WriteError(w, http.StatusForbidden, "password_change_required")
			return
		}
		if u.Role == "super_admin" {
			_, enrolled, err := s.Store.GetUserTOTPSecret(r.Context(), u.ID)
			if err == nil && !enrolled {
				WriteError(w, http.StatusForbidden, "two_factor_enrolment_required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isProvisioningAllowedRoute returns true for endpoints reachable while a
// user is gated. Keep this list minimal — anything reachable here is
// reachable while the user has an unrotated bootstrap password or a
// super_admin without 2FA.
func isProvisioningAllowedRoute(method, path string, actorID int64) bool {
	switch {
	case method == http.MethodGet && path == "/api/auth/me":
		return true
	case method == http.MethodGet && path == "/api/auth/2fa/status":
		return true
	case method == http.MethodPost && path == "/api/auth/logout":
		return true
	case method == http.MethodPost && path == "/api/auth/2fa/verify":
		return true
	case method == http.MethodPost && path == "/api/auth/2fa/enroll/start":
		return true
	case method == http.MethodPost && path == "/api/auth/2fa/enroll/confirm":
		return true
	case method == http.MethodPatch && path == "/api/users/"+strconv.FormatInt(actorID, 10):
		// Self-PATCH is the only way to rotate an unrotated bootstrap
		// password. PATCH on any other user is blocked.
		return true
	case method == http.MethodGet && path == "/api/users/"+strconv.FormatInt(actorID, 10):
		// SPA may want to display the current profile while the user is
		// in the password-change screen.
		return true
	}
	// Treat trailing slash as equivalent for chi parity.
	if strings.HasSuffix(path, "/") {
		return isProvisioningAllowedRoute(method, strings.TrimRight(path, "/"), actorID)
	}
	return false
}
