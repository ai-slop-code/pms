package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/ctxuser"
	"pms/backend/internal/middleware"
	pmstotp "pms/backend/internal/totp"
)

// --- DTOs ---------------------------------------------------------------

type twoFAStatusResponse struct {
	Enrolled                bool `json:"enrolled"`
	MFAPending              bool `json:"mfa_pending,omitempty"`
	RecoveryCodesRemaining  int  `json:"recovery_codes_remaining"`
}

type twoFAVerifyBody struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

type twoFAEnrollStartResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

type twoFAEnrollConfirmBody struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

type twoFAEnrollConfirmResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

type twoFADisableBody struct {
	Password string `json:"password"`
}

// --- Handlers -----------------------------------------------------------

// get2FAStatus serves both verified and pending sessions so the UI can
// decide whether to render the challenge or the enrolment panel.
func (s *Server) get2FAStatus(w http.ResponseWriter, r *http.Request) {
	u := ctxuser.From(r.Context())
	pending := false
	if u == nil {
		u = ctxuser.MFAPending(r.Context())
		pending = u != nil
	}
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	_, enrolled, err := s.Store.GetUserTOTPSecret(r.Context(), u.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	remaining, _ := s.Store.CountRecoveryCodesRemaining(r.Context(), u.ID)
	WriteJSON(w, http.StatusOK, twoFAStatusResponse{
		Enrolled:               enrolled,
		MFAPending:             pending,
		RecoveryCodesRemaining: remaining,
	})
}

// post2FAVerify consumes the pending-MFA session attached to the incoming
// cookie and, on success, flips mfa_verified=1 so subsequent requests see a
// normal authenticated session.
func (s *Server) post2FAVerify(w http.ResponseWriter, r *http.Request) {
	u := ctxuser.MFAPending(r.Context())
	if u == nil {
		WriteError(w, http.StatusBadRequest, "no pending 2fa session")
		return
	}
	var body twoFAVerifyBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Code = strings.TrimSpace(body.Code)
	body.RecoveryCode = strings.TrimSpace(body.RecoveryCode)
	if body.Code == "" && body.RecoveryCode == "" {
		WriteError(w, http.StatusBadRequest, "code or recovery_code required")
		return
	}
	// Rate-limit verify per client IP just like login so guessers can't
	// burn through the 6-digit space quickly.
	if s.LoginRateLimiter != nil {
		if !s.LoginRateLimiter.Allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "10")
			WriteError(w, http.StatusTooManyRequests, "too many attempts")
			return
		}
	}
	secret, enrolled, err := s.Store.GetUserTOTPSecret(r.Context(), u.ID)
	if err != nil || !enrolled {
		WriteError(w, http.StatusBadRequest, "2fa not enrolled")
		return
	}
	ok := false
	switch {
	case body.Code != "":
		ok = pmstotp.Verify(secret, body.Code, time.Now().UTC())
	case body.RecoveryCode != "":
		consumed, err := s.Store.ConsumeRecoveryCode(r.Context(), u.ID, pmstotp.HashRecoveryCode(body.RecoveryCode))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "database error")
			return
		}
		ok = consumed
	}
	if !ok {
		s.audit(r, u, "2fa_verify", "user", strconv.FormatInt(u.ID, 10), "failure")
		WriteError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil || c.Value == "" {
		WriteError(w, http.StatusUnauthorized, "no session")
		return
	}
	if err := s.Store.SetSessionMFAVerified(r.Context(), auth.HashSessionToken(c.Value)); err != nil {
		WriteError(w, http.StatusInternalServerError, "session error")
		return
	}
	s.audit(r, u, "2fa_verify", "user", strconv.FormatInt(u.ID, 10), "success")
	dto := userPublic(u)
	WriteJSON(w, http.StatusOK, loginResponse{
		User:                 &dto,
		ProvisioningRequired: s.provisioningRequiredFor(r, u),
	})
}

// post2FAEnrollStart issues a fresh secret + otpauth URL for the caller to
// paste / scan into their authenticator app. The secret is NOT persisted
// until /confirm succeeds with a correct code.
func (s *Server) post2FAEnrollStart(w http.ResponseWriter, r *http.Request) {
	u := ctxuser.From(r.Context())
	key, err := pmstotp.Generate(s.TOTPIssuer, u.Email)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "totp generate")
		return
	}
	WriteJSON(w, http.StatusOK, twoFAEnrollStartResponse{
		Secret:     key.Secret,
		OTPAuthURL: key.OTPAuthURL,
	})
}

// post2FAEnrollConfirm verifies the first code, persists the secret
// encrypted, and issues 10 single-use recovery codes (returned once).
func (s *Server) post2FAEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	u := ctxuser.From(r.Context())
	var body twoFAEnrollConfirmBody
	if err := ReadJSON(r, &body); err != nil || body.Secret == "" || body.Code == "" {
		WriteError(w, http.StatusBadRequest, "secret and code required")
		return
	}
	if !pmstotp.Verify(body.Secret, body.Code, time.Now().UTC()) {
		s.audit(r, u, "2fa_enroll", "user", strconv.FormatInt(u.ID, 10), "failure")
		WriteError(w, http.StatusBadRequest, "invalid code")
		return
	}
	if err := s.Store.SetUserTOTP(r.Context(), u.ID, body.Secret); err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	plain, hashes, err := pmstotp.GenerateRecoveryCodes(10)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "recovery code generation")
		return
	}
	if err := s.Store.ReplaceRecoveryCodes(r.Context(), u.ID, hashes); err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	// Invalidate every other session for this user — the user has just
	// upgraded their security posture; old cookies on other devices must
	// re-authenticate (they will pass the new 2FA challenge too).
	if c, err := r.Cookie(middleware.SessionCookieName); err == nil && c.Value != "" {
		_ = s.Store.DeleteSessionsForUserExcept(r.Context(), u.ID, auth.HashSessionToken(c.Value))
	}
	s.audit(r, u, "2fa_enroll", "user", strconv.FormatInt(u.ID, 10), "success")
	WriteJSON(w, http.StatusOK, twoFAEnrollConfirmResponse{RecoveryCodes: plain})
}

// post2FADisable clears the enrolment after re-confirming the user's
// password. Does not require a TOTP code — the assumption is the caller is
// already inside a fully-verified session (RequireAuthJSON enforced this)
// and may have permanently lost access to their authenticator.
func (s *Server) post2FADisable(w http.ResponseWriter, r *http.Request) {
	u := ctxuser.From(r.Context())
	var body twoFADisableBody
	if err := ReadJSON(r, &body); err != nil || body.Password == "" {
		WriteError(w, http.StatusBadRequest, "password required")
		return
	}
	fresh, err := s.Store.GetUserByID(r.Context(), u.ID)
	if err != nil || !auth.CheckPassword(fresh.PasswordHash, body.Password) {
		s.audit(r, u, "2fa_disable", "user", strconv.FormatInt(u.ID, 10), "failure")
		WriteError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	if err := s.Store.ClearUserTOTP(r.Context(), u.ID); err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.audit(r, u, "2fa_disable", "user", strconv.FormatInt(u.ID, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}
