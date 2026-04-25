package nuki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pms/backend/internal/otelx"
)

type Config struct {
	BaseURL       string
	Timeout       time.Duration
	Mock          bool
	LogFetchLimit int
	LogMaxPages   int
}

type Credentials struct {
	APIToken    string
	SmartLockID string
}

type UpsertAccessRequest struct {
	Label         string
	ValidFrom     time.Time
	ValidUntil    time.Time
	AccountUserID string
	AccessCode    string
}

type UpsertAccessResponse struct {
	ExternalID string
	AccessCode string
}

type KeypadAccessCode struct {
	ExternalID       string
	Name             string
	AccessCodeMasked string
	ValidFrom        *time.Time
	ValidUntil       *time.Time
	Enabled          bool
	PayloadJSON      string
}

type SmartlockEvent struct {
	ExternalID  string
	OccurredAt  time.Time
	AuthID      string
	IsEntryLike bool
	PayloadJSON string
}

type Client interface {
	ListKeypadCodes(ctx context.Context, cred Credentials) ([]KeypadAccessCode, error)
	ListSmartlockEvents(ctx context.Context, cred Credentials, since time.Time, authID string) ([]SmartlockEvent, error)
	CreateAccess(ctx context.Context, cred Credentials, req UpsertAccessRequest) (*UpsertAccessResponse, error)
	UpdateAccess(ctx context.Context, cred Credentials, externalID string, req UpsertAccessRequest) (*UpsertAccessResponse, error)
	SetAccessEnabled(ctx context.Context, cred Credentials, externalID string, payload map[string]interface{}) error
	RevokeAccess(ctx context.Context, cred Credentials, externalID string) error
}

func NewClient(cfg Config) Client {
	if cfg.Mock {
		return &mockClient{}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.nuki.io"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &httpClient{
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		http:          &http.Client{Timeout: cfg.Timeout, Transport: otelx.HTTPTransport(nil)},
		logFetchLimit: positiveOrDefault(cfg.LogFetchLimit, 500),
		logMaxPages:   positiveOrDefault(cfg.LogMaxPages, 10),
	}
}

type mockClient struct{}

func (m *mockClient) ListKeypadCodes(ctx context.Context, cred Credentials) ([]KeypadAccessCode, error) {
	return []KeypadAccessCode{
		{
			ExternalID:       "mock-remote-1",
			Name:             "Mock Stay",
			AccessCodeMasked: "****56",
			Enabled:          true,
			PayloadJSON:      `{"mock":true}`,
		},
	}, nil
}

func (m *mockClient) ListSmartlockEvents(ctx context.Context, cred Credentials, since time.Time, authID string) ([]SmartlockEvent, error) {
	tt := time.Now().UTC().Add(-2 * time.Hour)
	return []SmartlockEvent{
		{
			ExternalID:  "mock-log-1",
			OccurredAt:  tt,
			AuthID:      "cleaner-1",
			IsEntryLike: true,
			PayloadJSON: `{"mock":true,"name":"lock.action.unlock"}`,
		},
	}, nil
}

func (m *mockClient) CreateAccess(ctx context.Context, cred Credentials, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return &UpsertAccessResponse{
		ExternalID: "mock-" + raw[:12],
		AccessCode: raw[:6],
	}, nil
}

func (m *mockClient) UpdateAccess(ctx context.Context, cred Credentials, externalID string, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	return &UpsertAccessResponse{ExternalID: externalID}, nil
}

func (m *mockClient) SetAccessEnabled(ctx context.Context, cred Credentials, externalID string, payload map[string]interface{}) error {
	return nil
}

func (m *mockClient) RevokeAccess(ctx context.Context, cred Credentials, externalID string) error {
	return nil
}

type httpClient struct {
	baseURL       string
	http          *http.Client
	logFetchLimit int
	logMaxPages   int
}

func (c *httpClient) ListKeypadCodes(ctx context.Context, cred Credentials) ([]KeypadAccessCode, error) {
	paths := []string{
		"/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth",
		"/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth?enabled=true",
		"/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth?enabled=false",
		"/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth/advanced",
	}
	seen := map[string]KeypadAccessCode{}
	var lastErr error
	for _, path := range paths {
		var out []map[string]interface{}
		err := c.do(ctx, http.MethodGet, path, cred.APIToken, nil, &out)
		if err != nil {
			lastErr = err
			// Optional path variants can fail on different Nuki deployments.
			// Keep collecting from other variants and only fail if all fail.
			continue
		}
		for _, row := range out {
			code := rowToKeypadAccessCode(row)
			code.ExternalID = strings.TrimSpace(code.ExternalID)
			if code.ExternalID == "" {
				continue
			}
			existing, ok := seen[code.ExternalID]
			if !ok {
				seen[code.ExternalID] = code
				continue
			}
			seen[code.ExternalID] = mergeKeypadAccessCode(existing, code)
		}
	}
	if len(seen) == 0 && lastErr != nil {
		return nil, lastErr
	}
	events := make([]KeypadAccessCode, 0, len(seen))
	for _, code := range seen {
		events = append(events, code)
	}
	return events, nil
}

func (c *httpClient) ListSmartlockEvents(ctx context.Context, cred Credentials, since time.Time, authID string) ([]SmartlockEvent, error) {
	path := "/smartlock/" + url.PathEscape(cred.SmartLockID) + "/log"
	sinceUnix := since.UTC().Unix()
	authID = strings.TrimSpace(authID)
	limit := positiveOrDefault(c.logFetchLimit, 500)
	maxPages := positiveOrDefault(c.logMaxPages, 10)
	withParams := func(base string, extra map[string]string) string {
		q := url.Values{}
		for k, v := range extra {
			if strings.TrimSpace(v) != "" {
				q.Set(k, strings.TrimSpace(v))
			}
		}
		return base + "?" + q.Encode()
	}
	baseParams := map[string]string{
		"limit": strconv.Itoa(limit),
	}
	if authID != "" {
		baseParams["authId"] = authID
	}
	paramVariants := []map[string]string{
		{
			"fromDate": strconv.FormatInt(sinceUnix, 10),
			"limit":    baseParams["limit"],
			"authId":   baseParams["authId"],
		},
		{
			"dateSince": strconv.FormatInt(sinceUnix, 10),
			"limit":     baseParams["limit"],
			"authId":    baseParams["authId"],
		},
		{
			"limit":  baseParams["limit"],
			"authId": baseParams["authId"],
		},
	}
	seen := map[string]SmartlockEvent{}
	seenKeys := map[string]struct{}{}
	var sawSuccess bool
	var lastErr error
	for _, variant := range paramVariants {
		for page := 0; page < maxPages; page++ {
			params := map[string]string{}
			for k, v := range variant {
				if strings.TrimSpace(v) != "" {
					params[k] = v
				}
			}
			if page > 0 {
				params["offset"] = strconv.Itoa(page * limit)
			}
			p := withParams(path, params)
			var out []map[string]interface{}
			if err := c.do(ctx, http.MethodGet, p, cred.APIToken, nil, &out); err != nil {
				if page == 0 {
					lastErr = err
				}
				break
			}
			sawSuccess = true
			pageNew := 0
			for _, row := range out {
				ev, ok := rowToSmartlockEvent(row)
				if !ok {
					continue
				}
				if ev.OccurredAt.Before(since.UTC()) {
					continue
				}
				key := strings.TrimSpace(ev.ExternalID) + "|" + ev.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(ev.AuthID)
				if strings.TrimSpace(ev.ExternalID) == "" {
					key = ev.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(ev.AuthID)
				}
				if _, ok := seenKeys[key]; ok {
					continue
				}
				seenKeys[key] = struct{}{}
				seen[key] = ev
				pageNew++
			}
			// Stop paging when source exhausted or does not support offset.
			if len(out) < limit || (page > 0 && pageNew == 0) {
				break
			}
		}
	}
	if !sawSuccess {
		// Final compatibility fallback for deployments that reject query params.
		var out []map[string]interface{}
		if err := c.do(ctx, http.MethodGet, path, cred.APIToken, nil, &out); err == nil {
			sawSuccess = true
			for _, row := range out {
				ev, ok := rowToSmartlockEvent(row)
				if !ok {
					continue
				}
				if ev.OccurredAt.Before(since.UTC()) {
					continue
				}
				key := strings.TrimSpace(ev.ExternalID) + "|" + ev.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(ev.AuthID)
				if strings.TrimSpace(ev.ExternalID) == "" {
					key = ev.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(ev.AuthID)
				}
				seen[key] = ev
			}
		}
	}
	if !sawSuccess && lastErr != nil {
		return nil, lastErr
	}
	events := make([]SmartlockEvent, 0, len(seen))
	for _, ev := range seen {
		events = append(events, ev)
	}
	return events, nil
}

func rowToKeypadAccessCode(row map[string]interface{}) KeypadAccessCode {
	b, _ := json.Marshal(row)
	var vf *time.Time
	var vu *time.Time
	if t := parseAnyTime(pickAny(row, "allowedFromDate", "allowedFromDateTime", "allowedFromTime")); t != nil {
		vf = t
	}
	if t := parseAnyTime(pickAny(row, "allowedUntilDate", "allowedUntilDateTime", "allowedUntilTime")); t != nil {
		vu = t
	}
	enabled := true
	if v, ok := row["enabled"]; ok {
		if b, ok := v.(bool); ok {
			enabled = b
		}
	}
	return KeypadAccessCode{
		ExternalID:       pickString(row, "id", "authId", "smartlockAuthId"),
		Name:             pickString(row, "name"),
		AccessCodeMasked: pickString(row, "code", "smartlockCode", "accessCode"),
		ValidFrom:        vf,
		ValidUntil:       vu,
		Enabled:          enabled,
		PayloadJSON:      string(b),
	}
}

func mergeKeypadAccessCode(base, cand KeypadAccessCode) KeypadAccessCode {
	out := base
	if strings.TrimSpace(out.ExternalID) == "" {
		out.ExternalID = strings.TrimSpace(cand.ExternalID)
	}
	if strings.TrimSpace(out.Name) == "" && strings.TrimSpace(cand.Name) != "" {
		out.Name = cand.Name
	}
	if strings.TrimSpace(out.AccessCodeMasked) == "" && strings.TrimSpace(cand.AccessCodeMasked) != "" {
		out.AccessCodeMasked = cand.AccessCodeMasked
	}
	if out.ValidFrom == nil && cand.ValidFrom != nil {
		out.ValidFrom = cand.ValidFrom
	}
	if out.ValidUntil == nil && cand.ValidUntil != nil {
		out.ValidUntil = cand.ValidUntil
	}
	// If only one variant has proper validity window, prefer that payload snapshot.
	if (base.ValidFrom == nil || base.ValidUntil == nil) && cand.ValidFrom != nil && cand.ValidUntil != nil {
		out.PayloadJSON = cand.PayloadJSON
	}
	if strings.TrimSpace(out.PayloadJSON) == "" && strings.TrimSpace(cand.PayloadJSON) != "" {
		out.PayloadJSON = cand.PayloadJSON
	}
	out.Enabled = cand.Enabled
	return out
}

func pickAny(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func rowToSmartlockEvent(row map[string]interface{}) (SmartlockEvent, bool) {
	b, _ := json.Marshal(row)
	when := parseAnyTime(pickAny(row, "date", "createdAt", "timestamp", "time", "eventDate"))
	if when == nil {
		return SmartlockEvent{}, false
	}
	authID := pickString(row, "authId", "accountUserId", "userId", "smartlockAuthId")
	if strings.TrimSpace(authID) == "" {
		if authObj, ok := row["auth"].(map[string]interface{}); ok {
			authID = pickString(authObj, "id", "authId", "accountUserId", "userId")
		}
	}
	name := strings.ToLower(strings.TrimSpace(pickString(row, "name", "action", "event", "typeName")))
	eventTypeNum := int64(0)
	switch vv := pickAny(row, "type", "action").(type) {
	case float64:
		eventTypeNum = int64(vv)
	case int64:
		eventTypeNum = vv
	case int:
		eventTypeNum = int64(vv)
	}
	isEntry := strings.Contains(name, "unlock") ||
		strings.Contains(name, "lock.action.unlock") ||
		strings.Contains(name, "entry") ||
		strings.Contains(name, "opened") ||
		strings.Contains(name, "open") ||
		eventTypeNum == 1 || eventTypeNum == 2 || eventTypeNum == 3 || eventTypeNum == 4 || eventTypeNum == 5
	return SmartlockEvent{
		ExternalID:  pickString(row, "id", "logId", "eventId"),
		OccurredAt:  when.UTC(),
		AuthID:      strings.TrimSpace(authID),
		IsEntryLike: isEntry,
		PayloadJSON: string(b),
	}, true
}

func (c *httpClient) CreateAccess(ctx context.Context, cred Credentials, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	body := map[string]interface{}{
		"name":                req.Label,
		"allowedFromDate":     req.ValidFrom.UTC().Format(time.RFC3339),
		"allowedUntilDate":    req.ValidUntil.UTC().Format(time.RFC3339),
		"allowedWeekDays":     127,
		"allowedFromTime":     0,
		"allowedUntilTime":    0,
		"enabled":             true,
		"type":                13, // keypad PIN code authorization
		"remoteAllowed":       false,
		"smartActionsEnabled": false,
	}
	if strings.TrimSpace(req.AccountUserID) != "" {
		body["accountUserId"] = strings.TrimSpace(req.AccountUserID)
	}
	if strings.TrimSpace(req.AccessCode) != "" {
		body["code"] = strings.TrimSpace(req.AccessCode)
	}
	var out map[string]interface{}
	if err := c.do(ctx, http.MethodPut, "/smartlock/"+url.PathEscape(cred.SmartLockID)+"/auth", cred.APIToken, body, &out); err != nil {
		return nil, err
	}
	return &UpsertAccessResponse{
		ExternalID: pickString(out, "id", "authId", "smartlockAuthId"),
		AccessCode: pickString(out, "code", "smartlockCode", "accessCode"),
	}, nil
}

func (c *httpClient) UpdateAccess(ctx context.Context, cred Credentials, externalID string, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	body := map[string]interface{}{
		"name":                req.Label,
		"allowedFromDate":     req.ValidFrom.UTC().Format(time.RFC3339),
		"allowedUntilDate":    req.ValidUntil.UTC().Format(time.RFC3339),
		"allowedWeekDays":     127,
		"allowedFromTime":     0,
		"allowedUntilTime":    0,
		"enabled":             true,
		"type":                13, // keypad PIN code authorization
		"remoteAllowed":       false,
		"smartActionsEnabled": false,
	}
	if strings.TrimSpace(req.AccountUserID) != "" {
		body["accountUserId"] = strings.TrimSpace(req.AccountUserID)
	}
	if strings.TrimSpace(req.AccessCode) != "" {
		body["code"] = strings.TrimSpace(req.AccessCode)
	}
	var out map[string]interface{}
	path := "/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth/" + url.PathEscape(externalID)
	if err := c.do(ctx, http.MethodPut, path, cred.APIToken, body, &out); err != nil {
		return nil, err
	}
	id := pickString(out, "id", "authId", "smartlockAuthId")
	if id == "" {
		id = externalID
	}
	return &UpsertAccessResponse{
		ExternalID: id,
		AccessCode: pickString(out, "code", "smartlockCode", "accessCode"),
	}, nil
}

func (c *httpClient) RevokeAccess(ctx context.Context, cred Credentials, externalID string) error {
	path := "/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth/" + url.PathEscape(externalID)
	return c.do(ctx, http.MethodDelete, path, cred.APIToken, nil, nil)
}

func (c *httpClient) SetAccessEnabled(ctx context.Context, cred Credentials, externalID string, payload map[string]interface{}) error {
	path := "/smartlock/" + url.PathEscape(cred.SmartLockID) + "/auth/" + url.PathEscape(externalID)
	err := c.do(ctx, http.MethodPut, path, cred.APIToken, payload, nil)
	if !isMethodNotAllowed(err) {
		return err
	}
	err = c.do(ctx, http.MethodPost, path, cred.APIToken, payload, nil)
	if !isMethodNotAllowed(err) {
		return err
	}
	return c.do(ctx, http.MethodPatch, path, cred.APIToken, payload, nil)
}

func (c *httpClient) do(ctx context.Context, method, path, token string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("nuki http_%d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}
	return nil
}

func pickString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch vv := v.(type) {
			case string:
				if vv != "" {
					return vv
				}
			case float64:
				return strconv.FormatInt(int64(vv), 10)
			case int64:
				return strconv.FormatInt(vv, 10)
			}
		}
	}
	return ""
}

func parseAnyTime(v interface{}) *time.Time {
	switch vv := v.(type) {
	case string:
		if vv == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, vv); err == nil {
			return &t
		}
		if t, err := time.Parse("2006-01-02T15:04:05", vv); err == nil {
			return &t
		}
	case float64:
		if vv <= 0 {
			return nil
		}
		sec := int64(vv)
		// Heuristic: values above 1e12 are likely milliseconds.
		if sec > 1_000_000_000_000 {
			sec = sec / 1000
		}
		t := time.Unix(sec, 0).UTC()
		return &t
	}
	return nil
}

func isMethodNotAllowed(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "http_405") || strings.Contains(s, "method not allowed")
}

func positiveOrDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
