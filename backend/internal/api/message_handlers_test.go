package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
)

func TestMessages_ListTemplatesCreatesDefaults(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	owner, _ := st.CreateUser(ctx, "msg-owner@example.com", hash, "owner")
	prop, _ := st.CreateProperty(ctx, owner.ID, "Msg Test", "UTC", "en")

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "msg-owner@example.com", "secret123")
	client := &http.Client{}

	var res struct {
		Templates            []messageTemplateDTO `json:"templates"`
		SupportedLanguages   []string             `json:"supported_languages"`
		SupportedPlaceholders []string            `json:"supported_placeholders"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/message-templates",
		cookies, nil, &res)

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	checkInTemplates := map[string]bool{}
	hasCleaningStaff := false
	for _, tpl := range res.Templates {
		if tpl.Title == "" {
			t.Errorf("template %s/%s has empty title", tpl.LanguageCode, tpl.TemplateType)
		}
		if tpl.Body == "" {
			t.Errorf("template %s/%s has empty body", tpl.LanguageCode, tpl.TemplateType)
		}
		if !tpl.Active {
			t.Errorf("template %s/%s should be active", tpl.LanguageCode, tpl.TemplateType)
		}
		switch tpl.TemplateType {
		case "check_in":
			checkInTemplates[tpl.LanguageCode] = true
		case "cleaning_staff":
			hasCleaningStaff = true
		}
	}
	if len(checkInTemplates) != 5 {
		t.Fatalf("expected 5 check_in templates, got %d", len(checkInTemplates))
	}
	if !hasCleaningStaff {
		t.Fatalf("expected cleaning_staff template to be seeded")
	}
	if len(res.SupportedLanguages) != 5 {
		t.Fatalf("expected 5 supported languages, got %d", len(res.SupportedLanguages))
	}
	if len(res.SupportedPlaceholders) < 5 {
		t.Fatalf("expected >=5 placeholders, got %d", len(res.SupportedPlaceholders))
	}
	for _, lang := range []string{"en", "sk", "de", "uk", "hu"} {
		if !checkInTemplates[lang] {
			t.Errorf("missing check_in template for language %s", lang)
		}
	}
}

func TestMessages_PatchTemplate(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	owner, _ := st.CreateUser(ctx, "msg-patch@example.com", hash, "owner")
	prop, _ := st.CreateProperty(ctx, owner.ID, "Msg Patch", "UTC", "en")

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "msg-patch@example.com", "secret123")
	client := &http.Client{}

	var listRes struct {
		Templates []messageTemplateDTO `json:"templates"`
	}
	doAuthedJSONRequest(t, client, http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/message-templates",
		cookies, nil, &listRes)

	tplID := listRes.Templates[0].ID
	patchBody, _ := json.Marshal(map[string]interface{}{
		"title": "Updated Title",
		"body":  "Hello {{property_name}}, code: {{nuki_code}}",
	})

	var patchRes struct {
		Template messageTemplateDTO `json:"template"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodPatch,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/message-templates/"+strconv.FormatInt(tplID, 10),
		cookies, bytes.NewReader(patchBody), &patchRes)

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if patchRes.Template.Title != "Updated Title" {
		t.Errorf("title=%q want 'Updated Title'", patchRes.Template.Title)
	}
}

func TestMessages_PatchRejectsInvalidPlaceholder(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	owner, _ := st.CreateUser(ctx, "msg-invalid@example.com", hash, "owner")
	prop, _ := st.CreateProperty(ctx, owner.ID, "Msg Invalid", "UTC", "en")

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "msg-invalid@example.com", "secret123")
	client := &http.Client{}

	var listRes struct {
		Templates []messageTemplateDTO `json:"templates"`
	}
	doAuthedJSONRequest(t, client, http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/message-templates",
		cookies, nil, &listRes)

	tplID := listRes.Templates[0].ID
	patchBody, _ := json.Marshal(map[string]interface{}{
		"body": "Hello {{invalid_field}}",
	})
	status := doAuthedJSONRequest(t, client, http.MethodPatch,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/message-templates/"+strconv.FormatInt(tplID, 10),
		cookies, bytes.NewReader(patchBody), nil)

	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestMessages_GenerateForOccupancy(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	owner, _ := st.CreateUser(ctx, "msg-gen@example.com", hash, "owner")
	prop, _ := st.CreateProperty(ctx, owner.ID, "Msg Gen", "UTC", "en")

	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"wifi_ssid":             "TestWiFi",
		"wifi_password":         "wifipass",
		"parking_instructions":  "Lot B",
		"contact_phone":         "+421111222333",
	}); err != nil {
		t.Fatal(err)
	}

	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	end := start.Add(3 * 24 * time.Hour)
	if err := st.UpsertOccupancy(ctx, &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "manual",
		SourceEventUID: "msg-test-uid-1",
		StartAt:        start,
		EndAt:          end,
		Status:         "active",
		RawSummary:     sql.NullString{String: "Test Guest", Valid: true},
		ContentHash:    "hash1",
	}, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOccupancySyncRun(ctx, runID, "success", nil, nil, 1, 1); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "msg-test-uid-1")
	if err != nil || occ == nil {
		t.Fatal("occupancy not found")
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "msg-gen@example.com", "secret123")
	client := &http.Client{}

	var genRes struct {
		OccupancyID   int64 `json:"occupancy_id"`
		Messages      []struct {
			LanguageCode  string `json:"language_code"`
			Title         string `json:"title"`
			Body          string `json:"body"`
			NukiAvailable bool   `json:"nuki_available"`
		} `json:"messages"`
		NukiAvailable bool `json:"nuki_available"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/messages/generate?occupancy_id="+strconv.FormatInt(occ.ID, 10),
		cookies, nil, &genRes)

	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if len(genRes.Messages) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(genRes.Messages))
	}
	if genRes.NukiAvailable {
		t.Error("expected nuki_available=false (no code generated)")
	}

	enFound := false
	for _, msg := range genRes.Messages {
		if msg.LanguageCode == "en" {
			enFound = true
			if msg.Title == "" {
				t.Error("EN title should not be empty")
			}
			if !containsStr(msg.Body, "TestWiFi") {
				t.Error("EN body should contain WiFi name")
			}
			if !containsStr(msg.Body, "+421111222333") {
				t.Error("EN body should contain phone")
			}
			if !containsStr(msg.Body, "Msg Gen") {
				t.Error("EN body should contain property name")
			}
			if !containsStr(msg.Body, "—") {
				t.Error("EN body should contain missing nuki placeholder")
			}
		}
	}
	if !enFound {
		t.Error("expected EN message in output")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
