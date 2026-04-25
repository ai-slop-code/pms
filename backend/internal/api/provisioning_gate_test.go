package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/auth"
)

// TestProvisioningGate_BlocksUnrotatedBootstrap verifies that a user with
// must_change_password=1 cannot reach a normal protected endpoint until
// they self-PATCH a new password.
func TestProvisioningGate_BlocksUnrotatedBootstrap(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("temp-pass-1234")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "newop@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMustChangePassword(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, TOTPDevBypass: true}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "newop@example.com", "temp-pass-1234")

	// Listing properties is a protected endpoint that should be blocked.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 while gated, got %d", res.StatusCode)
	}

	// Self-PATCH on /api/users/{id} with a new password must be allowed
	// and must clear the flag.
	body, _ := json.Marshal(map[string]string{"password": "Brand-new-pass-99"})
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/users/"+strconv.FormatInt(u.ID, 10), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("self password change status=%d", res.StatusCode)
	}

	// Now the gate should let the user through.
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/properties", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after rotation, got %d", res.StatusCode)
	}

	got, err := st.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MustChangePassword {
		t.Fatalf("must_change_password not cleared after self password change")
	}
}

// TestProvisioningGate_BlocksSuperAdminWithout2FA verifies that
// super_admin accounts must enrol in TOTP before reaching general APIs.
// The TOTP dev bypass is intentionally NOT enabled here — when it is,
// the provisioning gate also waives the enrolment requirement (the whole
// point of the dev bypass is "no 2FA in dev/test"); see
// TestProvisioningGate_DevBypassWaivesEnrolment for that path.
func TestProvisioningGate_BlocksSuperAdminWithout2FA(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("super-pass-1234")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "root@example.com", hash, "super_admin"); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "root@example.com", "super-pass-1234")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for super_admin without 2FA, got %d", res.StatusCode)
	}
}

// TestProvisioningGate_DevBypassWaivesEnrolment proves that a super_admin
// without TOTP enrolment can still reach protected endpoints when
// PMS_2FA_DEV_BYPASS is on (TOTPDevBypass=true). The dev bypass is the
// "no 2FA in dev/test" escape hatch, so the gate must not contradict it.
// Production deployments cannot enable the bypass — config rejects it
// unless PMS_ENV ∈ {dev,development,test}.
func TestProvisioningGate_DevBypassWaivesEnrolment(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("super-pass-1234")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "root@example.com", hash, "super_admin"); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, TOTPDevBypass: true}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "root@example.com", "super-pass-1234")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for super_admin with dev bypass, got %d", res.StatusCode)
	}
}
