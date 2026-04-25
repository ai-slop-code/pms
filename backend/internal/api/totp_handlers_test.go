package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pquernaotp "github.com/pquerna/otp/totp"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
)

// Tiny registry so helpers can reach *store.Store from an httptest.Server URL.
var (
	totpServersMu sync.Mutex
	totpServers   = map[string]*Server{}
)

func newTOTPTestServer(t *testing.T) (*Server, *httptest.Server, int64) {
	t.Helper()
	st := testDB(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(context.Background(), "mfa@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour, TOTPIssuer: "PMS-Test"}
	ts := httptest.NewServer(srv.Routes())
	totpServersMu.Lock()
	totpServers[ts.URL] = srv
	totpServersMu.Unlock()
	t.Cleanup(func() {
		ts.Close()
		totpServersMu.Lock()
		delete(totpServers, ts.URL)
		totpServersMu.Unlock()
	})
	return srv, ts, u.ID
}

func storeFor(t *testing.T, ts *httptest.Server) *store.Store {
	t.Helper()
	totpServersMu.Lock()
	defer totpServersMu.Unlock()
	if s, ok := totpServers[ts.URL]; ok {
		return s.Store
	}
	t.Fatalf("no server registered for %s", ts.URL)
	return nil
}

func loginJSON(t *testing.T, ts *httptest.Server, email, pw string) (status int, resp loginResponse, cookies []*http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_ = json.NewDecoder(res.Body).Decode(&resp)
	return res.StatusCode, resp, res.Cookies()
}

func postJSONAuthed(t *testing.T, ts *httptest.Server, path string, payload interface{}, cookies []*http.Cookie, out interface{}) int {
	t.Helper()
	var rdr *bytes.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func getJSONAuthed(t *testing.T, ts *httptest.Server, path string, cookies []*http.Cookie, out interface{}) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestTOTP_EnrollAndChallengeFlow(t *testing.T) {
	_, ts, _ := newTOTPTestServer(t)
	const pw = "correct-horse-battery"

	// 1) Login — no TOTP yet, expect a verified session right away.
	status, login, cookies := loginJSON(t, ts, "mfa@example.com", pw)
	if status != http.StatusOK || login.MFARequired || login.User == nil {
		t.Fatalf("initial login unexpected: %d %+v", status, login)
	}

	// 2) Start enrolment → get secret + otpauth URL.
	var start twoFAEnrollStartResponse
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/enroll/start", nil, cookies, &start); code != http.StatusOK {
		t.Fatalf("enroll/start status %d", code)
	}
	if start.Secret == "" || !strings.HasPrefix(start.OTPAuthURL, "otpauth://totp/") {
		t.Fatalf("unexpected enrol payload: %+v", start)
	}

	// 3) Confirm enrolment with a valid code → receive 10 recovery codes.
	goodCode, err := pquernaotp.GenerateCode(start.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var confirm twoFAEnrollConfirmResponse
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/enroll/confirm",
		map[string]string{"secret": start.Secret, "code": goodCode}, cookies, &confirm); code != http.StatusOK {
		t.Fatalf("enroll/confirm status %d", code)
	}
	if len(confirm.RecoveryCodes) != 10 {
		t.Fatalf("want 10 recovery codes, got %d", len(confirm.RecoveryCodes))
	}

	// 4) Fresh login for enrolled user must return mfa_required=true and
	//    NOT leak the user payload until the challenge is solved.
	status, login2, cookies2 := loginJSON(t, ts, "mfa@example.com", pw)
	if status != http.StatusOK || !login2.MFARequired || login2.User != nil {
		t.Fatalf("login after enrol should challenge: %d %+v", status, login2)
	}

	// 4a) /auth/me on a pending session reports MFARequired rather than 200+user.
	var me meResponse
	if code := getJSONAuthed(t, ts, "/api/auth/me", cookies2, &me); code != http.StatusOK || !me.MFARequired || me.User != nil {
		t.Fatalf("/me on pending session: %d %+v", code, me)
	}

	// 4b) Protected endpoint must 401 while pending.
	if code := getJSONAuthed(t, ts, "/api/users", cookies2, nil); code != http.StatusUnauthorized {
		t.Fatalf("/users on pending session want 401 got %d", code)
	}

	// 5) Verify with a bad code → 401, session still pending.
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/verify",
		map[string]string{"code": "000000"}, cookies2, nil); code != http.StatusUnauthorized {
		t.Fatalf("bad code want 401 got %d", code)
	}

	// 6) Verify with a good code → 200; cookie is now verified.
	newCode, _ := pquernaotp.GenerateCode(start.Secret, time.Now().UTC())
	var verify loginResponse
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/verify",
		map[string]string{"code": newCode}, cookies2, &verify); code != http.StatusOK {
		t.Fatalf("good code verify: %d", code)
	}
	if verify.User == nil || verify.User.Email != "mfa@example.com" {
		t.Fatalf("verify did not return user: %+v", verify)
	}

	// 6a) Protected endpoint now returns something other than 401.
	if code := getJSONAuthed(t, ts, "/api/users", cookies2, nil); code == http.StatusUnauthorized {
		t.Fatalf("/users after verify want !=401 got %d", code)
	}
}

func TestTOTP_RecoveryCodeConsumesSingleUse(t *testing.T) {
	_, ts, userID := newTOTPTestServer(t)
	const pw = "correct-horse-battery"
	_, _, cookies := loginJSON(t, ts, "mfa@example.com", pw)

	var start twoFAEnrollStartResponse
	postJSONAuthed(t, ts, "/api/auth/2fa/enroll/start", nil, cookies, &start)
	good, _ := pquernaotp.GenerateCode(start.Secret, time.Now().UTC())
	var confirm twoFAEnrollConfirmResponse
	postJSONAuthed(t, ts, "/api/auth/2fa/enroll/confirm",
		map[string]string{"secret": start.Secret, "code": good}, cookies, &confirm)

	// Trigger a new pending session.
	_, _, pending := loginJSON(t, ts, "mfa@example.com", pw)

	recovery := confirm.RecoveryCodes[0]
	// 1st use → 200
	var verify loginResponse
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/verify",
		map[string]string{"recovery_code": recovery}, pending, &verify); code != http.StatusOK {
		t.Fatalf("recovery first use: %d", code)
	}
	if verify.User == nil {
		t.Fatalf("recovery verify missing user: %+v", verify)
	}

	// Remaining count must drop by one.
	remaining, err := storeFor(t, ts).CountRecoveryCodesRemaining(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 9 {
		t.Fatalf("remaining want 9 got %d", remaining)
	}

	// 2nd use on a new pending session must fail — the code was consumed.
	_, _, pending2 := loginJSON(t, ts, "mfa@example.com", pw)
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/verify",
		map[string]string{"recovery_code": recovery}, pending2, nil); code != http.StatusUnauthorized {
		t.Fatalf("recovery reuse want 401 got %d", code)
	}
}

func TestTOTP_DisableClearsEnrolment(t *testing.T) {
	_, ts, userID := newTOTPTestServer(t)
	const pw = "correct-horse-battery"
	_, _, cookies := loginJSON(t, ts, "mfa@example.com", pw)

	var start twoFAEnrollStartResponse
	postJSONAuthed(t, ts, "/api/auth/2fa/enroll/start", nil, cookies, &start)
	good, _ := pquernaotp.GenerateCode(start.Secret, time.Now().UTC())
	var confirm twoFAEnrollConfirmResponse
	postJSONAuthed(t, ts, "/api/auth/2fa/enroll/confirm",
		map[string]string{"secret": start.Secret, "code": good}, cookies, &confirm)

	// Disable with wrong password → 401, enrolment intact.
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/disable",
		map[string]string{"password": "nope"}, cookies, nil); code != http.StatusUnauthorized {
		t.Fatalf("disable wrong pw: want 401 got %d", code)
	}
	_, enrolled, _ := storeFor(t, ts).GetUserTOTPSecret(context.Background(), userID)
	if !enrolled {
		t.Fatal("disable should not have cleared enrolment on wrong pw")
	}

	// Disable with correct password → 204; no recovery codes remaining.
	if code := postJSONAuthed(t, ts, "/api/auth/2fa/disable",
		map[string]string{"password": pw}, cookies, nil); code != http.StatusNoContent {
		t.Fatalf("disable: want 204 got %d", code)
	}
	_, enrolled, _ = storeFor(t, ts).GetUserTOTPSecret(context.Background(), userID)
	if enrolled {
		t.Fatal("disable should have cleared enrolment")
	}
	if n, _ := storeFor(t, ts).CountRecoveryCodesRemaining(context.Background(), userID); n != 0 {
		t.Fatalf("recovery codes should be gone, got %d", n)
	}
}

func TestTOTP_DevBypassSkipsChallenge(t *testing.T) {
	st := testDB(t)
	hash, _ := auth.HashPassword("correct-horse-battery")
	u, _ := st.CreateUser(context.Background(), "bypass@example.com", hash, "owner")
	// Pre-enrol the user directly in the DB.
	if err := st.SetUserTOTP(context.Background(), u.ID, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour, TOTPIssuer: "PMS", TOTPDevBypass: true}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	status, login, _ := loginJSON(t, ts, "bypass@example.com", "correct-horse-battery")
	if status != http.StatusOK || login.MFARequired || login.User == nil {
		t.Fatalf("dev bypass login: %d %+v", status, login)
	}
}
