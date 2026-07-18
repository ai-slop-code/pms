package cleaningcalendar

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const calendarScope = "https://www.googleapis.com/auth/calendar.events"

type ServiceAccountClient struct {
	HTTP        *http.Client
	ClientEmail string
	PrivateKey  *rsa.PrivateKey
	TokenURI    string

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type serviceAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func NewServiceAccountClient(rawJSON []byte, httpClient *http.Client) (*ServiceAccountClient, error) {
	var cfg serviceAccountJSON
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ClientEmail) == "" || strings.TrimSpace(cfg.PrivateKey) == "" {
		return nil, errors.New("google service account client_email and private_key required")
	}
	if strings.TrimSpace(cfg.TokenURI) == "" {
		cfg.TokenURI = "https://oauth2.googleapis.com/token"
	}
	key, err := parseRSAPrivateKey([]byte(cfg.PrivateKey))
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &ServiceAccountClient{HTTP: httpClient, ClientEmail: strings.TrimSpace(cfg.ClientEmail), PrivateKey: key, TokenURI: strings.TrimSpace(cfg.TokenURI)}, nil
}

func (c *ServiceAccountClient) Configured() bool {
	return c != nil && c.ClientEmail != "" && c.PrivateKey != nil
}

func (c *ServiceAccountClient) UpsertEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error) {
	if strings.TrimSpace(googleEventID) != "" {
		id, err := c.patchEvent(ctx, event, googleEventID)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, errGoogleNotFound) {
			return "", err
		}
	}
	return c.insertEvent(ctx, event)
}

func (c *ServiceAccountClient) ListEvents(ctx context.Context, calendarID string, timeMin, timeMax time.Time) ([]GoogleCalendarEvent, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("timeMin", timeMin.Format(time.RFC3339))
	q.Set("timeMax", timeMax.Format(time.RFC3339))
	q.Set("singleEvents", "true")
	q.Set("showDeleted", "false")
	endpoint := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?%s", url.PathEscape(calendarID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, googleAPIError(res)
	}
	var out struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Status      string `json:"status"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
			ExtendedProperties struct {
				Private map[string]string `json:"private"`
			} `json:"extendedProperties"`
		} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	events := make([]GoogleCalendarEvent, 0, len(out.Items))
	for _, item := range out.Items {
		start, _ := parseGoogleEventTime(item.Start.DateTime, item.Start.Date)
		end, _ := parseGoogleEventTime(item.End.DateTime, item.End.Date)
		events = append(events, GoogleCalendarEvent{
			ID:                item.ID,
			Summary:           item.Summary,
			Description:       item.Description,
			Status:            item.Status,
			Start:             start,
			End:               end,
			PrivateProperties: item.ExtendedProperties.Private,
		})
	}
	return events, nil
}

func (c *ServiceAccountClient) DeleteEvent(ctx context.Context, calendarID, googleEventID string) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", url.PathEscape(calendarID), url.PathEscape(googleEventID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone {
		return nil
	}
	return googleAPIError(res)
}

func (c *ServiceAccountClient) insertEvent(ctx context.Context, event CalendarEventPayload) (string, error) {
	return c.writeEvent(ctx, http.MethodPost, fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", url.PathEscape(event.CalendarID)), event)
}

func (c *ServiceAccountClient) patchEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error) {
	return c.writeEvent(ctx, http.MethodPatch, fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", url.PathEscape(event.CalendarID), url.PathEscape(googleEventID)), event)
}

func (c *ServiceAccountClient) writeEvent(ctx context.Context, method, endpoint string, event CalendarEventPayload) (string, error) {
	token, err := c.token(ctx)
	if err != nil {
		return "", err
	}
	private := map[string]string{
		"pms_property_id":           fmt.Sprintf("%d", event.PropertyID),
		"pms_occupancy_id":          fmt.Sprintf("%d", event.OccupancyID),
		"pms_cleaning_event_id":     fmt.Sprintf("%d", event.LocalEventID),
		"pms_managed_event_version": "1",
	}
	if event.NamedStayID > 0 {
		private["pms_named_stay_id"] = fmt.Sprintf("%d", event.NamedStayID)
	}
	if event.RawBlockID > 0 {
		private["pms_raw_booking_block_id"] = fmt.Sprintf("%d", event.RawBlockID)
	}
	if strings.TrimSpace(event.Identity) != "" {
		private["pms_cleaning_identity"] = strings.TrimSpace(event.Identity)
	}
	body := map[string]interface{}{
		"summary":     event.Summary,
		"description": event.Description,
		"start": map[string]string{
			"dateTime": event.Start.Format(time.RFC3339),
			"timeZone": event.TimeZone,
		},
		"end": map[string]string{
			"dateTime": event.End.Format(time.RFC3339),
			"timeZone": event.TimeZone,
		},
		"extendedProperties": map[string]interface{}{
			"private": private,
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return "", errGoogleNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", googleAPIError(res)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.ID) == "" {
		return "", errors.New("google calendar response missing event id")
	}
	return out.ID, nil
}

func parseGoogleEventTime(dateTimeValue, dateValue string) (time.Time, error) {
	if strings.TrimSpace(dateTimeValue) != "" {
		return time.Parse(time.RFC3339, strings.TrimSpace(dateTimeValue))
	}
	if strings.TrimSpace(dateValue) != "" {
		return time.Parse("2006-01-02", strings.TrimSpace(dateValue))
	}
	return time.Time{}, nil
}

func (c *ServiceAccountClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Before(c.expiresAt.Add(-time.Minute)) {
		tok := c.accessToken
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()
	jwt, err := c.jwtAssertion(time.Now())
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", jwt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", googleAPIError(res)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("google token response missing access_token")
	}
	expires := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	c.mu.Lock()
	c.accessToken = out.AccessToken
	c.expiresAt = expires
	c.mu.Unlock()
	return out.AccessToken, nil
}

func (c *ServiceAccountClient) jwtAssertion(now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]interface{}{
		"iss":   c.ClientEmail,
		"scope": calendarScope,
		"aud":   c.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.PrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseRSAPrivateKey(raw []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("google service account private_key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("google service account private_key is not RSA")
	}
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rsaKey, nil
	}
	return nil, err
}

var errGoogleNotFound = errors.New("google calendar event not found")

func googleAPIError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = res.Status
	}
	return fmt.Errorf("google calendar api %d: %s", res.StatusCode, msg)
}
