package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/auth"
	"pms/backend/internal/ctxuser"
	"pms/backend/internal/middleware"
	"pms/backend/internal/nuki"
	"pms/backend/internal/occupancy"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

type Server struct {
	Store              *store.Store
	SessionTTL         time.Duration
	Occ                *occupancy.Service
	Nuki               *nuki.Service
	DataDir            string
	CookieSecure       bool
	CookieSameSite     http.SameSite
	LoginRateLimiter   *middleware.KeyedLimiter
	// TOTPIssuer is shown as the account label in authenticator apps.
	TOTPIssuer string
	// TOTPDevBypass short-circuits the 2FA challenge for enrolled users.
	// Must only be true when PMS_ENV=dev (config.Load enforces this).
	TOTPDevBypass bool
	// AdminBackupLimiter throttles the on-demand super_admin backup endpoint
	// (rate.Every(30s), burst 1). Optional; nil disables the check.
	AdminBackupLimiter *middleware.KeyedLimiter
	// InvoiceRegenLimiter throttles invoice PDF regeneration per user
	// (rate.Every(5s), burst 3). Optional; nil disables the check.
	InvoiceRegenLimiter *middleware.KeyedLimiter
	// AttachmentUploadLimiter throttles finance attachment uploads per user
	// (rate.Every(2s), burst 5). Optional; nil disables the check.
	AttachmentUploadLimiter *middleware.KeyedLimiter
	// AllowedOrigins mirrors the CORS allowlist. Used by the CSRF guard so
	// it can reject state-changing requests whose Origin header is not in
	// the allowlist (browsers always send Origin on POST/PATCH/DELETE).
	AllowedOrigins []string
	// TrustedProxy enables X-Forwarded-For parsing for the rate-limiter and
	// access-log client-IP key. Only enable when the deployment is fronted
	// by a reverse proxy you control (Caddy/nginx).
	TrustedProxy bool
}

func (s *Server) cookieSameSite() http.SameSite {
	if s.CookieSameSite != 0 {
		return s.CookieSameSite
	}
	return http.SameSiteLaxMode
}

func (s *Server) audit(r *http.Request, actor *store.User, action, entityType, entityID, outcome string) {
	var aid *int64
	if actor != nil {
		aid = &actor.ID
	}
	_ = s.Store.InsertAuditLog(r.Context(), aid, action, entityType, entityID, outcome, r.Method, r.URL.Path)
}

// clientIP returns a stable key for rate limiting. When the server is
// configured with TrustedProxy=true, the left-most entry of X-Forwarded-For
// is used (that's the originating client per RFC 7239 conventions when the
// proxy appends, not prepends). Otherwise we fall back to r.RemoteAddr
// minus its port. Without this guard, every request appears to come from
// the reverse-proxy loopback address and per-IP rate limiting collapses
// into a single global bucket.
func (s *Server) clientIP(r *http.Request) string {
	if s.TrustedProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if comma := strings.IndexByte(xff, ','); comma >= 0 {
				xff = xff[:comma]
			}
			if ip := strings.TrimSpace(xff); ip != "" {
				return ip
			}
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		addr = addr[:idx]
	}
	return strings.TrimPrefix(strings.TrimSuffix(addr, "]"), "[")
}

func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", s.getHealthz)
	r.Get("/healthz", s.getHealthz)
	r.Get("/readyz", s.getReadyz)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.CSRFGuard(s.AllowedOrigins))
		r.Post("/auth/login", s.postLogin)
		r.Get("/properties/{id}/occupancy-export", s.getOccupancyExportPublic)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(s.Store))
			r.Post("/auth/logout", s.postLogout)
			r.Get("/auth/me", s.getMe)
			// 2FA endpoints available for pending-MFA sessions too —
			// /verify upgrades the session, /status tells the UI whether
			// to show the challenge form.
			r.Post("/auth/2fa/verify", s.post2FAVerify)
			r.Get("/auth/2fa/status", s.get2FAStatus)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAuthJSON)
				// Funnel un-rotated bootstrap accounts and super_admins
				// without 2FA into the provisioning flow before they can
				// touch any other API. See PMS_11 follow-up #4 / #6.
				r.Use(s.ProvisioningGate)
				// Self-service enrolment and disable require a
				// fully-verified session.
				r.Post("/auth/2fa/enroll/start", s.post2FAEnrollStart)
				r.Post("/auth/2fa/enroll/confirm", s.post2FAEnrollConfirm)
				r.Post("/auth/2fa/disable", s.post2FADisable)
				r.Get("/users", s.listUsers)
				r.Post("/users", s.postUser)
				r.Get("/users/{id}", s.getUser)
				r.Patch("/users/{id}", s.patchUser)
				r.Post("/users/{id}/property-permissions", s.postUserPropertyPermission)
				r.Delete("/users/{id}/property-permissions/{permissionId}", s.deleteUserPropertyPermission)
				r.Get("/properties", s.listProperties)
				r.Post("/properties", s.postProperty)
				r.Get("/properties/{id}", s.getProperty)
				r.Patch("/properties/{id}", s.patchProperty)
				r.Get("/properties/{id}/settings", s.getPropertySettings)
				r.Patch("/properties/{id}/settings", s.patchPropertySettings)
				r.Get("/dashboard/summary", s.getDashboardSummary)
				r.Get("/properties/{id}/dashboard", s.getDashboardSummary)
				r.Get("/properties/{id}/occupancies", s.getOccupancies)
				r.Get("/properties/{id}/occupancies/calendar", s.getOccupanciesCalendar)
				r.Post("/properties/{id}/occupancy-sync/run", s.postOccupancySyncRun)
				r.Get("/properties/{id}/occupancy-sync/runs", s.listOccupancySyncRuns)
				r.Get("/properties/{id}/occupancy-source", s.getOccupancySource)
				r.Patch("/properties/{id}/occupancy-source", s.patchOccupancySource)
				r.Post("/properties/{id}/occupancy-api-tokens", s.postOccupancyAPIToken)
				r.Get("/properties/{id}/occupancy-api-tokens", s.listOccupancyAPITokens)
				r.Delete("/properties/{id}/occupancy-api-tokens/{tokenId}", s.deleteOccupancyAPIToken)
				r.Get("/properties/{id}/nuki/codes", s.listNukiCodes)
				r.Get("/properties/{id}/nuki/upcoming-stays", s.listNukiUpcomingStays)
				r.Patch("/properties/{id}/nuki/upcoming-stays/{occupancyId}", s.saveNukiStayName)
				r.Patch("/properties/{id}/nuki/keypad-codes/{externalId}", s.patchNukiKeypadCode)
				r.Delete("/properties/{id}/nuki/keypad-codes/{externalId}", s.deleteNukiKeypadCode)
				r.Post("/properties/{id}/nuki/codes/generate", s.generateNukiCodes)
				r.Get("/properties/{id}/nuki/codes/{codeId}/reveal-pin", s.revealNukiCodePIN)
				r.Post("/properties/{id}/nuki/codes/{codeId}/revoke", s.revokeNukiCode)
				r.Post("/properties/{id}/nuki/sync/run", s.runNukiSync)
				r.Get("/properties/{id}/nuki/runs", s.listNukiRuns)
				r.Get("/properties/{id}/cleaning/logs", s.getCleaningLogs)
				r.Get("/properties/{id}/cleaning/summary", s.getCleaningSummary)
				r.Get("/properties/{id}/cleaning/heatmap", s.getCleaningHeatmap)
				r.Get("/properties/{id}/cleaning/fees", s.getCleaningFees)
				r.Post("/properties/{id}/cleaning/fees", s.postCleaningFees)
				r.Get("/properties/{id}/cleaning/adjustments", s.getCleaningAdjustments)
				r.Post("/properties/{id}/cleaning/adjustments", s.postCleaningAdjustment)
				r.Post("/properties/{id}/cleaning/reconcile/run", s.runCleaningReconcile)
				r.Get("/properties/{id}/finance/transactions", s.listFinanceTransactions)
				r.Post("/properties/{id}/finance/transactions", s.postFinanceTransaction)
				r.Get("/properties/{id}/finance/booking-payouts", s.listFinanceBookingPayouts)
				r.Post("/properties/{id}/finance/booking-payouts/import", s.importFinanceBookingPayouts)
				r.Post("/properties/{id}/finance/booking-payouts/rematch", s.rematchFinanceBookingPayouts)
				r.Patch("/properties/{id}/finance/booking-payouts/{referenceNumber}/map", s.mapFinanceBookingPayout)
				r.Post("/properties/{id}/finance/booking-payouts/{referenceNumber}/create-stay", s.createFinanceBookingPayoutStay)
				r.Patch("/properties/{id}/finance/transactions/{transactionId}", s.patchFinanceTransaction)
				r.Delete("/properties/{id}/finance/transactions/{transactionId}", s.deleteFinanceTransaction)
				r.Post("/properties/{id}/finance/months/{month}/open", s.openFinanceMonth)
				r.Get("/properties/{id}/finance/summary", s.getFinanceSummary)
				r.Get("/properties/{id}/finance/categories", s.listFinanceCategories)
				r.Post("/properties/{id}/finance/categories", s.postFinanceCategory)
				r.Get("/properties/{id}/finance/recurring-rules", s.listFinanceRecurringRules)
				r.Post("/properties/{id}/finance/recurring-rules", s.postFinanceRecurringRule)
				r.Patch("/properties/{id}/finance/recurring-rules/{ruleId}", s.patchFinanceRecurringRule)
				r.Get("/properties/{id}/invoices/occupancy-candidates", s.listInvoiceOccupancyCandidates)
				r.Get("/properties/{id}/invoices/payout-link-candidates", s.listInvoicePayoutLinkCandidates)
				r.Get("/properties/{id}/invoices", s.listInvoices)
				r.Post("/properties/{id}/invoices", s.postInvoice)
				r.Get("/properties/{id}/invoices/{invoiceId}", s.getInvoice)
				r.Patch("/properties/{id}/invoices/{invoiceId}", s.patchInvoice)
				r.Post("/properties/{id}/invoices/{invoiceId}/regenerate", s.regenerateInvoice)
				r.Get("/properties/{id}/invoices/{invoiceId}/download", s.downloadInvoice)
				r.Get("/properties/{id}/invoice-sequence/next-preview", s.previewNextInvoiceSequence)
			r.Get("/properties/{id}/message-templates", s.listMessageTemplates)
			r.Post("/properties/{id}/message-templates", s.postMessageTemplate)
			r.Patch("/properties/{id}/message-templates/{templateId}", s.patchMessageTemplate)
			r.Delete("/properties/{id}/message-templates/{templateId}", s.deleteMessageTemplate)
			r.Get("/properties/{id}/messages/generate", s.generateMessage)
			r.Get("/properties/{id}/messages/cleaning", s.generateCleaningMessage)
			r.Get("/properties/{id}/analytics/freshness", s.getAnalyticsFreshness)
			r.Get("/properties/{id}/analytics/outlook", s.getAnalyticsOutlook)
			r.Get("/properties/{id}/analytics/performance", s.getAnalyticsPerformance)
			r.Get("/properties/{id}/analytics/demand", s.getAnalyticsDemand)
			r.Get("/properties/{id}/analytics/pace", s.getAnalyticsPace)
			r.Get("/properties/{id}/analytics/returning-guests", s.getAnalyticsReturningGuests)
			r.Get("/admin/backup", s.getAdminBackup)
			})
		})
	})
	return r
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	if s.LoginRateLimiter != nil {
		key := s.clientIP(r)
		if !s.LoginRateLimiter.Allow(key) {
			w.Header().Set("Retry-After", "10")
			WriteError(w, http.StatusTooManyRequests, "too many login attempts")
			return
		}
	}
	var body loginBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.Store.GetUserByEmail(r.Context(), body.Email)
	if err != nil || !u.Active || !auth.CheckPassword(u.PasswordHash, body.Password) {
		s.audit(r, nil, "login", "user", "", "failure")
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	_, enrolled, err := s.Store.GetUserTOTPSecret(r.Context(), u.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	// Only issue a fully-verified session when either (a) the user has not
	// enrolled in TOTP, or (b) the dev bypass is explicitly enabled.
	mfaVerified := !enrolled || s.TOTPDevBypass
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "session error")
		return
	}
	exp := time.Now().UTC().Add(s.SessionTTL)
	if err := s.Store.CreateSessionWithMFA(r.Context(), u.ID, hash, exp, mfaVerified); err != nil {
		WriteError(w, http.StatusInternalServerError, "session error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		SameSite: s.cookieSameSite(),
		Secure:   s.CookieSecure,
		Expires:  exp,
	})
	if mfaVerified {
		s.audit(r, u, "login", "user", strconv.FormatInt(u.ID, 10), "success")
		dto := userPublic(u)
		WriteJSON(w, http.StatusOK, loginResponse{User: &dto})
		return
	}
	// Quarantined session — UI must prompt for the TOTP code next.
	s.audit(r, u, "login", "user", strconv.FormatInt(u.ID, 10), "mfa_pending")
	WriteJSON(w, http.StatusOK, loginResponse{MFARequired: true})
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	// Accept either a verified or pending session — a user mid-challenge
	// must be able to abandon the attempt.
	u := ctxuser.From(r.Context())
	if u == nil {
		u = ctxuser.MFAPending(r.Context())
	}
	if c, err := r.Cookie(middleware.SessionCookieName); err == nil && c.Value != "" {
		hash := auth.HashSessionToken(c.Value)
		_ = s.Store.DeleteSessionByTokenHash(r.Context(), hash)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: s.cookieSameSite(),
		Secure:   s.CookieSecure,
		MaxAge:   -1,
	})
	if u != nil {
		s.audit(r, u, "logout", "user", strconv.FormatInt(u.ID, 10), "success")
	}
	w.WriteHeader(http.StatusNoContent)
}

func userPublic(u *store.User) userDTO {
	return userDTO{
		ID:                 u.ID,
		Email:              u.Email,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	}
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	if u := ctxuser.From(r.Context()); u != nil {
		dto := userPublic(u)
		WriteJSON(w, http.StatusOK, meResponse{User: &dto})
		return
	}
	if u := ctxuser.MFAPending(r.Context()); u != nil {
		// Tell the UI to show the TOTP challenge instead of booting
		// the user back to the login screen.
		_ = u
		WriteJSON(w, http.StatusOK, meResponse{MFARequired: true})
		return
	}
	WriteError(w, http.StatusUnauthorized, "authentication required")
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	if actor.Role != "super_admin" {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	list, err := s.Store.ListUsers(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]userDTO, 0, len(list))
	for _, u := range list {
		out = append(out, userPublic(&u))
	}
	WriteJSON(w, http.StatusOK, usersResponse{Users: out})
}

type createUserBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) postUser(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	if actor.Role != "super_admin" {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body createUserBody
	if err := ReadJSON(r, &body); err != nil || body.Email == "" || body.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password required")
		return
	}
	if body.Role == "" {
		body.Role = "owner"
	}
	switch body.Role {
	case "super_admin", "owner", "property_manager", "read_only":
	default:
		WriteError(w, http.StatusBadRequest, "invalid role")
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "hash error")
		return
	}
	u, err := s.Store.CreateUser(r.Context(), body.Email, hash, body.Role)
	if err != nil {
		WriteError(w, http.StatusConflict, "email already exists")
		return
	}
	s.audit(r, actor, "create", "user", strconv.FormatInt(u.ID, 10), "success")
	WriteJSON(w, http.StatusCreated, userResponse{User: userPublic(u)})
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if actor.Role != "super_admin" && actor.ID != id {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	u, err := s.Store.GetUserByID(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	perms, _ := s.Store.ListPermissionsForUser(r.Context(), id)
	var po []propertyPermissionDTO
	for _, p := range perms {
		po = append(po, propertyPermissionDTO{ID: p.ID, PropertyID: p.PropertyID, Module: p.Module, PermissionLevel: p.PermissionLevel})
	}
	WriteJSON(w, http.StatusOK, userWithPermissionsResponse{User: userPublic(u), PropertyPermissions: po})
}

type patchUserBody struct {
	Email    *string `json:"email"`
	Role     *string `json:"role"`
	Active   *bool   `json:"active"`
	Password *string `json:"password"`
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if actor.Role != "super_admin" && actor.ID != id {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body patchUserBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if actor.Role != "super_admin" {
		if body.Role != nil || body.Active != nil {
			WriteError(w, http.StatusForbidden, "cannot change role or active")
			return
		}
	}
	if body.Role != nil {
		switch *body.Role {
		case "super_admin", "owner", "property_manager", "read_only":
		default:
			WriteError(w, http.StatusBadRequest, "invalid role")
			return
		}
	}
	var hashPtr *string
	if body.Password != nil && *body.Password != "" {
		if err := auth.ValidatePassword(*body.Password); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		h, err := auth.HashPassword(*body.Password)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "hash error")
			return
		}
		hashPtr = &h
	}
	u, err := s.Store.UpdateUser(r.Context(), id, body.Email, body.Role, body.Active, hashPtr)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if hashPtr != nil {
		// Password changed: invalidate every other session for this user. Keep
		// the actor's own session alive so the UI doesn't log them out when
		// they rotate their own password. For admin-initiated changes on
		// another user, wipe all sessions. See PMS_11/T2.4.
		if actor != nil && actor.ID == id {
			var keepHash string
			if c, cerr := r.Cookie(middleware.SessionCookieName); cerr == nil {
				keepHash = auth.HashSessionToken(c.Value)
			}
			_ = s.Store.DeleteSessionsForUserExcept(r.Context(), id, keepHash)
			// Self password rotation always clears the forced-change flag.
			_ = s.Store.SetMustChangePassword(r.Context(), id, false)
		} else {
			_ = s.Store.DeleteSessionsForUser(r.Context(), id)
			// Admin-initiated reset: arm the forced-change flag so the
			// target user is funnelled into rotating the temp password.
			_ = s.Store.SetMustChangePassword(r.Context(), id, true)
		}
	}
	s.audit(r, actor, "update", "user", strconv.FormatInt(id, 10), "success")
	WriteJSON(w, http.StatusOK, userResponse{User: userPublic(u)})
}

type permBody struct {
	PropertyID      int64  `json:"property_id"`
	Module          string `json:"module"`
	PermissionLevel string `json:"permission_level"`
}

func (s *Server) postUserPropertyPermission(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body permBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.PropertyID == 0 || body.Module == "" || body.PermissionLevel == "" {
		WriteError(w, http.StatusBadRequest, "property_id, module, permission_level required")
		return
	}
	switch body.PermissionLevel {
	case permissions.LevelRead, permissions.LevelWrite, permissions.LevelAdmin:
	default:
		WriteError(w, http.StatusBadRequest, "invalid permission_level")
		return
	}
	can, err := s.actorCanManagePropertyPermissions(r.Context(), actor, body.PropertyID)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if actor.Role != "super_admin" && targetID == actor.ID {
		WriteError(w, http.StatusBadRequest, "cannot assign permissions to yourself via this endpoint")
		return
	}
	p, err := s.Store.UpsertPropertyPermission(r.Context(), targetID, body.PropertyID, body.Module, body.PermissionLevel)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "save failed")
		return
	}
	s.audit(r, actor, "upsert_permission", "property_user_permission", strconv.FormatInt(p.ID, 10), "success")
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":               p.ID,
		"user_id":          p.UserID,
		"property_id":      p.PropertyID,
		"module":           p.Module,
		"permission_level": p.PermissionLevel,
	})
}

func (s *Server) actorCanManagePropertyPermissions(ctx context.Context, actor *store.User, propertyID int64) (bool, error) {
	if actor.Role == "super_admin" {
		return true, nil
	}
	return s.Store.IsPropertyOwner(ctx, actor.ID, propertyID)
}

func (s *Server) deleteUserPropertyPermission(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	permID, err := strconv.ParseInt(chi.URLParam(r, "permissionId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid permission id")
		return
	}
	perms, err := s.Store.ListPermissionsForUser(r.Context(), targetID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	var propID int64
	found := false
	for _, p := range perms {
		if p.ID == permID {
			propID = p.PropertyID
			found = true
			break
		}
	}
	if !found {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	can, err := s.actorCanManagePropertyPermissions(r.Context(), actor, propID)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.Store.DeletePropertyPermission(r.Context(), permID); err != nil {
		WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	s.audit(r, actor, "delete_permission", "property_user_permission", strconv.FormatInt(permID, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listProperties(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	list, err := s.Store.ListPropertiesForUser(r.Context(), actor)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]propertyDTO, 0, len(list))
	for _, p := range list {
		out = append(out, propertySummary(&p))
	}
	WriteJSON(w, http.StatusOK, propertiesResponse{Properties: out})
}

func propertySummary(p *store.Property) propertyDTO {
	return propertyDTO{
		ID:              p.ID,
		Name:            p.Name,
		Timezone:        p.Timezone,
		DefaultLanguage: p.DefaultLanguage,
		DefaultCurrency: p.DefaultCurrency,
		InvoiceCode:     nullStringPtr(p.InvoiceCode),
		OwnerUserID:     p.OwnerUserID,
		AddressLine1:    nullStringPtr(p.AddressLine1),
		City:            nullStringPtr(p.City),
		PostalCode:      nullStringPtr(p.PostalCode),
		Country:         nullStringPtr(p.Country),
		WeekStartsOn:    p.WeekStartsOn,
		Active:          p.Active,
		CreatedAt:       p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

type createPropertyBody struct {
	Name            string `json:"name"`
	Timezone        string `json:"timezone"`
	DefaultLanguage string `json:"default_language"`
	OwnerUserID     *int64 `json:"owner_user_id"`
}

func (s *Server) postProperty(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	var body createPropertyBody
	if err := ReadJSON(r, &body); err != nil || body.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	ownerID := actor.ID
	if body.OwnerUserID != nil {
		if actor.Role != "super_admin" {
			WriteError(w, http.StatusForbidden, "only super_admin can set owner_user_id")
			return
		}
		ownerID = *body.OwnerUserID
	} else if actor.Role != "super_admin" && actor.Role != "owner" {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	p, err := s.Store.CreateProperty(r.Context(), ownerID, body.Name, body.Timezone, body.DefaultLanguage)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}
	s.audit(r, actor, "create", "property", strconv.FormatInt(p.ID, 10), "success")
	WriteJSON(w, http.StatusCreated, propertyResponse{Property: propertySummary(p)})
}

func (s *Server) getProperty(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := s.Store.UserCanSeeProperty(r.Context(), actor, id)
	if err != nil || !ok {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	p, err := s.Store.GetProperty(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, propertyResponse{Property: propertySummary(p)})
}

type patchPropertyBody struct {
	Name            *string `json:"name"`
	Timezone        *string `json:"timezone"`
	DefaultLanguage *string `json:"default_language"`
	InvoiceCode     *string `json:"invoice_code"`
	AddressLine1    *string `json:"address_line1"`
	City            *string `json:"city"`
	PostalCode      *string `json:"postal_code"`
	Country         *string `json:"country"`
	WeekStartsOn    *string `json:"week_starts_on"`
	Active          *bool   `json:"active"`
}

func (s *Server) patchProperty(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, err := s.Store.UserCan(r.Context(), actor, id, permissions.PropertySettings, permissions.LevelWrite)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body patchPropertyBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	p, err := s.Store.UpdateProperty(r.Context(), id, body.Name, body.Timezone, body.DefaultLanguage, body.InvoiceCode,
		body.AddressLine1, body.City, body.PostalCode, body.Country, body.WeekStartsOn, body.Active)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.audit(r, actor, "update", "property", strconv.FormatInt(id, 10), "success")
	WriteJSON(w, http.StatusOK, propertyResponse{Property: propertySummary(p)})
}

func (s *Server) getPropertySettings(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, err := s.Store.UserCan(r.Context(), actor, id, permissions.PropertySettings, permissions.LevelRead)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	pr, err := s.Store.GetPropertyProfile(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	sec, err := s.Store.GetPropertySecrets(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, propertySettingsResponse{
		Profile: propertySettingsProfileDTO{
			LegalOwnerName:      nullStringPtr(pr.LegalOwnerName),
			BillingName:         nullStringPtr(pr.BillingName),
			BillingAddress:      nullStringPtr(pr.BillingAddress),
			City:                nullStringPtr(pr.City),
			PostalCode:          nullStringPtr(pr.PostalCode),
			Country:             nullStringPtr(pr.Country),
			ICO:                 nullStringPtr(pr.ICO),
			DIC:                 nullStringPtr(pr.DIC),
			VATID:               nullStringPtr(pr.VATID),
			ContactPhone:        nullStringPtr(pr.ContactPhone),
			WifiSSID:            nullStringPtr(pr.WifiSSID),
			WifiPasswordSet:     pr.WifiPassword.Valid && pr.WifiPassword.String != "",
			ParkingInstructions: nullStringPtr(pr.ParkingInstructions),
			DefaultCheckInTime:  pr.DefaultCheckInTime,
			DefaultCheckOutTime: pr.DefaultCheckOutTime,
			CleanerNukiAuthID:   nullStringPtr(pr.CleanerNukiAuthID),
		},
		Integrations: propertyIntegrationsDTO{
			BookingICSConfigured: sec.BookingICSURL.Valid && sec.BookingICSURL.String != "",
			NukiConfigured:       sec.NukiAPIToken.Valid && sec.NukiAPIToken.String != "" && sec.NukiSmartlockID.Valid && sec.NukiSmartlockID.String != "",
		},
	})
}

type patchSettingsBody struct {
	Profile map[string]interface{} `json:"profile"`
	Secrets *patchSecrets          `json:"secrets"`
}

type patchSecrets struct {
	BookingICSURL   *string `json:"booking_ics_url"`
	NukiAPIToken    *string `json:"nuki_api_token"`
	NukiSmartlockID *string `json:"nuki_smartlock_id"`
}

func (s *Server) patchPropertySettings(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	can, err := s.Store.UserCan(r.Context(), actor, id, permissions.PropertySettings, permissions.LevelWrite)
	if err != nil || !can {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body patchSettingsBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Profile != nil {
		if err := s.Store.UpdatePropertyProfile(r.Context(), id, body.Profile); err != nil {
			WriteError(w, http.StatusInternalServerError, "profile update failed")
			return
		}
	}
	if body.Secrets != nil {
		if err := s.Store.UpdatePropertySecrets(r.Context(), id, body.Secrets.BookingICSURL, body.Secrets.NukiAPIToken, body.Secrets.NukiSmartlockID); err != nil {
			WriteError(w, http.StatusInternalServerError, "secrets update failed")
			return
		}
	}
	s.audit(r, actor, "update", "property_settings", strconv.FormatInt(id, 10), "success")
	s.getPropertySettings(w, r)
}

func (s *Server) getDashboardSummary(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	// Canonical path: /api/properties/{id}/dashboard. Legacy path:
	// /api/dashboard/summary?property_id=…. Both are accepted for backward
	// compatibility during the Phase C hardening transition.
	var (
		q   = strings.TrimSpace(chi.URLParam(r, "id"))
		pid int64
		err error
	)
	if q == "" {
		q = r.URL.Query().Get("property_id")
	}
	if q == "" {
		WriteError(w, http.StatusBadRequest, "property_id required")
		return
	}
	pid, err = strconv.ParseInt(q, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid property_id")
		return
	}
	ok, err := s.Store.UserCanSeeProperty(r.Context(), actor, pid)
	if err != nil || !ok {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	occCan, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.Occupancy, permissions.LevelRead)
	nukiCan, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.NukiAccess, permissions.LevelRead)
	cleaningCan, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.CleaningLog, permissions.LevelRead)
	financeCan, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.Finance, permissions.LevelRead)
	invoicesCan, _ := s.Store.UserCan(r.Context(), actor, pid, permissions.Invoices, permissions.LevelRead)

	widgets := dashboardWidgetsDTO{}

	if occCan || nukiCan {
		status := &syncStatusWidget{}
		if sec, err := s.Store.GetPropertySecrets(r.Context(), pid); err == nil {
			if occCan {
				occSync := "not_configured"
				if sec.BookingICSURL.Valid && sec.BookingICSURL.String != "" {
					occSync = "no_sync_yet"
				}
				if runs, err := s.Store.ListOccupancySyncRuns(r.Context(), pid, 1); err == nil && len(runs) > 0 {
					occSync = dashboardSyncState(runs[0].Status)
				}
				status.Occupancy = occSync
			}
			if nukiCan {
				nukiSync := "not_configured"
				if sec.NukiAPIToken.Valid && sec.NukiAPIToken.String != "" && sec.NukiSmartlockID.Valid && sec.NukiSmartlockID.String != "" {
					nukiSync = "no_sync_yet"
				}
				if runs, err := s.Store.ListNukiSyncRuns(r.Context(), pid, 1, 0); err == nil && len(runs) > 0 {
					nukiSync = dashboardSyncState(runs[0].Status)
				}
				status.Nuki = nukiSync
			}
		}
		if status.Occupancy != "" || status.Nuki != "" {
			widgets.SyncStatus = status
		}
	}

	if occCan {
		rows, err := s.Store.ListUpcomingOccupancies(r.Context(), pid, 5)
		if err == nil {
			out := make([]dashboardUpcomingStayRow, 0, len(rows))
			for _, row := range rows {
				summary := nullStringPtr(row.GuestDisplayName)
				if summary == nil {
					summary = nullStringPtr(row.RawSummary)
				}
				out = append(out, dashboardUpcomingStayRow{
					OccupancyID: row.ID,
					Summary:     summary,
					StartAt:     row.StartAt.UTC().Format(time.RFC3339),
					EndAt:       row.EndAt.UTC().Format(time.RFC3339),
					Status:      row.Status,
				})
			}
			widgets.UpcomingStays = &out
		}
	}

	if nukiCan {
		rows, err := s.Store.ListUpcomingStaysForNuki(r.Context(), pid, 5)
		if err == nil {
			out := make([]dashboardActiveNukiCodeRow, 0)
			for _, row := range rows {
				if !row.GeneratedStatus.Valid || row.GeneratedStatus.String != "generated" {
					continue
				}
				summary := nullStringPtr(row.GuestDisplayName)
				if summary == nil {
					summary = nullStringPtr(row.RawSummary)
				}
				out = append(out, dashboardActiveNukiCodeRow{
					OccupancyID:   row.OccupancyID,
					Summary:       summary,
					CodeLabel:     nullStringPtr(row.GeneratedLabel),
					CodeMasked:    nullStringPtr(row.GeneratedMasked),
					Status:        row.GeneratedStatus.String,
					ValidFrom:     nullTimePtr(row.GeneratedValidFrom),
					ValidUntil:    nullTimePtr(row.GeneratedValidUntil),
					LastUpdatedAt: nullTimePtr(row.GeneratedUpdated),
					ErrorMessage:  nullStringPtr(row.GeneratedError),
				})
			}
			widgets.ActiveNukiCodes = &out
		}
	}

	var loc *time.Location
	if cleaningCan || financeCan {
		prop, err := s.Store.GetProperty(r.Context(), pid)
		if err == nil {
			loc, err = time.LoadLocation(prop.Timezone)
			if err != nil {
				loc = time.UTC
			}
		}
		if loc == nil {
			loc = time.UTC
		}
	}

	if cleaningCan && loc != nil {
		now := time.Now().In(loc)
		if sum, err := s.Store.ComputeCleaningMonthlySummary(r.Context(), pid, now.Year(), int(now.Month()), loc); err == nil && sum != nil {
			widget := &cleaningMonthWidget{
				CountedDays: sum.CountedDays,
				SalaryDraft: sum.FinalSalaryCents,
			}
			widgets.CleaningMonth = widget
		}
	}

	if financeCan && loc != nil {
		month := time.Now().In(loc).Format("2006-01")
		if sum, err := s.Store.ComputeFinanceSummary(r.Context(), pid, month); err == nil && sum != nil {
			widget := &financeMonthWidget{
				Incoming: sum.MonthlyIncomingCents,
				Outgoing: sum.MonthlyOutgoingCents,
				Net:      sum.MonthlyNetCents,
			}
			widgets.FinanceMonth = widget
		}
	}

	if invoicesCan {
		if rows, err := s.Store.ListInvoices(r.Context(), pid); err == nil {
			out := make([]dashboardInvoiceRow, 0, 3)
			for _, row := range rows {
				if len(out) >= 3 {
					break
				}
				var customer struct {
					Name        string `json:"name"`
					CompanyName string `json:"company_name"`
				}
				_ = json.Unmarshal([]byte(row.CustomerSnapshotJSON), &customer)
				displayName := strings.TrimSpace(customer.CompanyName)
				if displayName == "" {
					displayName = strings.TrimSpace(customer.Name)
				}
				out = append(out, dashboardInvoiceRow{
					InvoiceID:     row.ID,
					InvoiceNumber: row.InvoiceNumber,
					CustomerName:  stringPtrOrNil(displayName),
					AmountTotal:   row.AmountTotalCents,
					IssueDate:     row.IssueDate.UTC().Format(time.RFC3339),
					Version:       row.Version,
				})
			}
			widgets.RecentInvoices = &out
		}
	}

	WriteJSON(w, http.StatusOK, dashboardSummaryResponse{
		PropertyID: pid,
		Widgets:    widgets,
	})
}

func dashboardSyncState(status string) string {
	switch status {
	case "success":
		return "ok"
	case "failure":
		return "error"
	case "partial":
		return "partial"
	case "running":
		return "running"
	default:
		return status
	}
}

func stringPtrOrNil(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
