package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type MessageTemplate struct {
	ID           int64
	PropertyID   int64
	LanguageCode string
	TemplateType string
	Title        string
	Body         string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var defaultTemplates = map[string]struct {
	Title string
	Body  string
}{
	"en": {
		Title: "Check-in Instructions",
		Body: `Hello,

Welcome to {{property_name}}!

Your stay: {{stay_start}} – {{stay_end}}
Check-in: {{check_in_time}}, Check-out: {{check_out_time}}

Address: {{property_address}}

Door access code: {{nuki_code}}

Wi-Fi: {{wifi_name}}
Password: {{wifi_password}}

Parking: {{parking_info}}

Contact: {{contact_phone}}

We wish you a pleasant stay!`,
	},
	"sk": {
		Title: "Pokyny k príchodu",
		Body: `Dobrý deň,

Vitajte v {{property_name}}!

Váš pobyt: {{stay_start}} – {{stay_end}}
Check-in: {{check_in_time}}, Check-out: {{check_out_time}}

Adresa: {{property_address}}

Prístupový kód: {{nuki_code}}

Wi-Fi: {{wifi_name}}
Heslo: {{wifi_password}}

Parkovanie: {{parking_info}}

Kontakt: {{contact_phone}}

Prajeme Vám príjemný pobyt!`,
	},
	"de": {
		Title: "Check-in Anweisungen",
		Body: `Hallo,

Willkommen in {{property_name}}!

Ihr Aufenthalt: {{stay_start}} – {{stay_end}}
Check-in: {{check_in_time}}, Check-out: {{check_out_time}}

Adresse: {{property_address}}

Zugangscode: {{nuki_code}}

WLAN: {{wifi_name}}
Passwort: {{wifi_password}}

Parken: {{parking_info}}

Kontakt: {{contact_phone}}

Wir wünschen Ihnen einen angenehmen Aufenthalt!`,
	},
	"uk": {
		Title: "Інструкції для заселення",
		Body: `Вітаємо,

Ласкаво просимо до {{property_name}}!

Ваше перебування: {{stay_start}} – {{stay_end}}
Заїзд: {{check_in_time}}, Виїзд: {{check_out_time}}

Адреса: {{property_address}}

Код доступу: {{nuki_code}}

Wi-Fi: {{wifi_name}}
Пароль: {{wifi_password}}

Паркування: {{parking_info}}

Контакт: {{contact_phone}}

Бажаємо приємного перебування!`,
	},
	"hu": {
		Title: "Bejelentkezési útmutató",
		Body: `Üdvözöljük,

Üdvözöljük a {{property_name}} szálláshelyen!

Tartózkodás: {{stay_start}} – {{stay_end}}
Bejelentkezés: {{check_in_time}}, Kijelentkezés: {{check_out_time}}

Cím: {{property_address}}

Hozzáférési kód: {{nuki_code}}

Wi-Fi: {{wifi_name}}
Jelszó: {{wifi_password}}

Parkolás: {{parking_info}}

Kapcsolat: {{contact_phone}}

Kellemes tartózkodást kívánunk!`,
	},
}

var SupportedMessageLanguages = []string{"en", "sk", "de", "uk", "hu"}

const (
	TemplateTypeCheckIn       = "check_in"
	TemplateTypeCleaningStaff = "cleaning_staff"
)

var defaultCleaningStaffTemplate = struct {
	LanguageCode string
	Title        string
	Body         string
}{
	LanguageCode: "sk",
	Title:        "Rozpis upratovania",
	Body: `Ahoj, posielam upratovanie platné od {{cleaning_from_date}}, máme to poznačené aj v kalendári:
{{cleaning_list}}`,
}

var AllPlaceholders = []string{
	"property_name",
	"property_address",
	"stay_start",
	"stay_end",
	"check_in_time",
	"check_out_time",
	"nuki_code",
	"wifi_name",
	"wifi_password",
	"parking_info",
	"contact_phone",
	"cleaning_from_date",
	"cleaning_list",
}

func (s *Store) ListMessageTemplates(ctx context.Context, propertyID int64) ([]MessageTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, language_code, template_type, title, body, active, created_at, updated_at
		FROM message_templates
		WHERE property_id = ?
		ORDER BY language_code`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageTemplate
	for rows.Next() {
		var t MessageTemplate
		var active int
		var created, updated string
		if err := rows.Scan(&t.ID, &t.PropertyID, &t.LanguageCode, &t.TemplateType, &t.Title, &t.Body, &active, &created, &updated); err != nil {
			return nil, err
		}
		t.Active = active == 1
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetMessageTemplate(ctx context.Context, propertyID, templateID int64) (*MessageTemplate, error) {
	var t MessageTemplate
	var active int
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, language_code, template_type, title, body, active, created_at, updated_at
		FROM message_templates
		WHERE property_id = ? AND id = ?`, propertyID, templateID).
		Scan(&t.ID, &t.PropertyID, &t.LanguageCode, &t.TemplateType, &t.Title, &t.Body, &active, &created, &updated)
	if err != nil {
		return nil, err
	}
	t.Active = active == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &t, nil
}

func (s *Store) CreateMessageTemplate(ctx context.Context, propertyID int64, languageCode, templateType, title, body string) (*MessageTemplate, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO message_templates (property_id, language_code, template_type, title, body, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		propertyID, languageCode, templateType, title, body, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetMessageTemplate(ctx, propertyID, id)
}

func (s *Store) UpdateMessageTemplate(ctx context.Context, propertyID, templateID int64, title, body *string, active *bool) (*MessageTemplate, error) {
	t, err := s.GetMessageTemplate(ctx, propertyID, templateID)
	if err != nil {
		return nil, err
	}
	if title != nil {
		t.Title = *title
	}
	if body != nil {
		t.Body = *body
	}
	if active != nil {
		t.Active = *active
	}
	now := time.Now().UTC().Format(time.RFC3339)
	activeInt := 0
	if t.Active {
		activeInt = 1
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE message_templates SET title = ?, body = ?, active = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`,
		t.Title, t.Body, activeInt, now, propertyID, templateID)
	if err != nil {
		return nil, err
	}
	return s.GetMessageTemplate(ctx, propertyID, templateID)
}

func (s *Store) DeleteMessageTemplate(ctx context.Context, propertyID, templateID int64) error {
	res, err := s.DB.ExecContext(ctx,
		`DELETE FROM message_templates WHERE property_id = ? AND id = ?`, propertyID, templateID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

func (s *Store) EnsureDefaultMessageTemplates(ctx context.Context, propertyID int64) error {
	for _, lang := range SupportedMessageLanguages {
		def, ok := defaultTemplates[lang]
		if !ok {
			continue
		}
		var exists int
		err := s.DB.QueryRowContext(ctx,
			`SELECT 1 FROM message_templates WHERE property_id = ? AND language_code = ? AND template_type = 'check_in'`,
			propertyID, lang).Scan(&exists)
		if err == nil {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO message_templates (property_id, language_code, template_type, title, body, active, created_at, updated_at)
			VALUES (?, ?, 'check_in', ?, ?, 1, ?, ?)`,
			propertyID, lang, def.Title, def.Body, now, now)
		if err != nil {
			return err
		}
	}

	var exists int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM message_templates WHERE property_id = ? AND template_type = 'cleaning_staff' LIMIT 1`,
		propertyID).Scan(&exists)
	if err != nil {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO message_templates (property_id, language_code, template_type, title, body, active, created_at, updated_at)
			VALUES (?, ?, 'cleaning_staff', ?, ?, 1, ?, ?)`,
			propertyID, defaultCleaningStaffTemplate.LanguageCode,
			defaultCleaningStaffTemplate.Title, defaultCleaningStaffTemplate.Body, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetCleaningStaffTemplate(ctx context.Context, propertyID int64) (*MessageTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, language_code, template_type, title, body, active, created_at, updated_at
		FROM message_templates
		WHERE property_id = ? AND template_type = 'cleaning_staff' AND active = 1
		ORDER BY id ASC
		LIMIT 1`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("cleaning_staff template not found")
	}
	var t MessageTemplate
	var active int
	var created, updated string
	if err := rows.Scan(&t.ID, &t.PropertyID, &t.LanguageCode, &t.TemplateType, &t.Title, &t.Body, &active, &created, &updated); err != nil {
		return nil, err
	}
	t.Active = active == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &t, nil
}

type MessagePlaceholderValues struct {
	PropertyName     string
	PropertyAddress  string
	StayStart        string
	StayEnd          string
	CheckInTime      string
	CheckOutTime     string
	NukiCode         string
	WifiName         string
	WifiPassword     string
	ParkingInfo      string
	ContactPhone     string
	CleaningFromDate string
	CleaningList     string
}

func RenderMessageTemplate(templateBody string, vals MessagePlaceholderValues) string {
	r := strings.NewReplacer(
		"{{property_name}}", vals.PropertyName,
		"{{property_address}}", vals.PropertyAddress,
		"{{stay_start}}", vals.StayStart,
		"{{stay_end}}", vals.StayEnd,
		"{{check_in_time}}", vals.CheckInTime,
		"{{check_out_time}}", vals.CheckOutTime,
		"{{nuki_code}}", vals.NukiCode,
		"{{wifi_name}}", vals.WifiName,
		"{{wifi_password}}", vals.WifiPassword,
		"{{parking_info}}", vals.ParkingInfo,
		"{{contact_phone}}", vals.ContactPhone,
		"{{cleaning_from_date}}", vals.CleaningFromDate,
		"{{cleaning_list}}", vals.CleaningList,
	)
	return r.Replace(templateBody)
}

func ValidateTemplatePlaceholders(body string) []string {
	var invalid []string
	validSet := map[string]bool{}
	for _, p := range AllPlaceholders {
		validSet["{{"+p+"}}"] = true
	}
	idx := 0
	for {
		start := strings.Index(body[idx:], "{{")
		if start == -1 {
			break
		}
		start += idx
		end := strings.Index(body[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2
		token := body[start:end]
		if !validSet[token] {
			invalid = append(invalid, token)
		}
		idx = end
	}
	return uniqueStrings(invalid)
}

func uniqueStrings(input []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (s *Store) BuildPlaceholderValues(ctx context.Context, propertyID, occupancyID int64) (*MessagePlaceholderValues, error) {
	prop, err := s.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, fmt.Errorf("property: %w", err)
	}
	profile, err := s.GetPropertyProfile(ctx, propertyID)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	occ, err := s.GetOccupancyByID(ctx, propertyID, occupancyID)
	if err != nil {
		return nil, fmt.Errorf("occupancy: %w", err)
	}

	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}

	var address string
	if prop.AddressLine1.Valid {
		parts := []string{prop.AddressLine1.String}
		if prop.City.Valid {
			parts = append(parts, prop.City.String)
		}
		if prop.PostalCode.Valid {
			parts = append(parts, prop.PostalCode.String)
		}
		address = strings.Join(parts, ", ")
	}

	nukiCode := "—"
	code, err := s.GetNukiCodeByOccupancyID(ctx, propertyID, occupancyID)
	if err == nil && code != nil && code.Status == "generated" && code.GeneratedPINPlain.Valid && code.GeneratedPINPlain.String != "" {
		nukiCode = code.GeneratedPINPlain.String
	}

	wifiName := ""
	if profile.WifiSSID.Valid {
		wifiName = profile.WifiSSID.String
	}
	wifiPass := ""
	if profile.WifiPassword.Valid {
		wifiPass = profile.WifiPassword.String
	}
	parking := ""
	if profile.ParkingInstructions.Valid {
		parking = profile.ParkingInstructions.String
	}
	phone := ""
	if profile.ContactPhone.Valid {
		phone = profile.ContactPhone.String
	}

	return &MessagePlaceholderValues{
		PropertyName:    prop.Name,
		PropertyAddress: address,
		StayStart:       occ.StartAt.In(loc).Format("02/01/2006"),
		StayEnd:         occ.EndAt.In(loc).Format("02/01/2006"),
		CheckInTime:     profile.DefaultCheckInTime,
		CheckOutTime:    profile.DefaultCheckOutTime,
		NukiCode:        nukiCode,
		WifiName:        wifiName,
		WifiPassword:    wifiPass,
		ParkingInfo:     parking,
		ContactPhone:    phone,
	}, nil
}

// BuildCleaningPlaceholderValues constructs placeholder values for the cleaning-staff
// template: a list of upcoming stays formatted as bullet points with the cleaning
// window (check-out → check-in, from property profile config) and a fixed "2x hostia"
// label for the guest count. Occupancies with end_at before `now` are excluded.
func (s *Store) BuildCleaningPlaceholderValues(ctx context.Context, propertyID int64, now time.Time) (*MessagePlaceholderValues, int, error) {
	prop, err := s.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, 0, fmt.Errorf("property: %w", err)
	}
	profile, err := s.GetPropertyProfile(ctx, propertyID)
	if err != nil {
		return nil, 0, fmt.Errorf("profile: %w", err)
	}

	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}

	nowLocal := now.In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	// End of current month (exclusive upper bound = first day of next month).
	nextMonthStart := time.Date(nowLocal.Year(), nowLocal.Month()+1, 1, 0, 0, 0, 0, loc)
	horizon := nextMonthStart
	// If we're within the last 3 days of the current month, extend the horizon
	// into the first 7 days of the next month so the cleaning lady gets the
	// hand-off visibility.
	daysRemaining := int(nextMonthStart.Sub(today).Hours() / 24)
	if daysRemaining <= 3 {
		horizon = nextMonthStart.AddDate(0, 0, 7)
	}

	// Widen the SQL window by one day on the lower bound to safely include stays
	// whose end_at lands on UTC midnight of today (ICS all-day DTEND convention),
	// regardless of the property's UTC offset. The precise per-day filtering is
	// done in the loop below using the local checkout date.
	queryStart := today.AddDate(0, 0, -1).UTC()
	queryEnd := horizon.UTC()

	stays, err := s.ListOccupanciesBetween(ctx, propertyID, queryStart, queryEnd)
	if err != nil {
		return nil, 0, fmt.Errorf("occupancies: %w", err)
	}

	checkOut := profile.DefaultCheckOutTime
	checkIn := profile.DefaultCheckInTime

	seen := map[string]bool{}
	var lines []string
	for _, occ := range stays {
		if occ.Status == "deleted_from_source" || occ.Status == "cancelled" {
			continue
		}
		endLocal := occ.EndAt.In(loc)
		// Compare on the local calendar date so stays that check out today are
		// included even if end_at is stored at UTC midnight.
		endDay := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)
		if endDay.Before(today) {
			continue
		}
		if !endDay.Before(horizon) {
			continue
		}
		key := endDay.Format("2006-01-02")
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, fmt.Sprintf("* %d.%d. po %s do %s, 2x hostia",
			endDay.Day(), int(endDay.Month()), checkOut, checkIn))
	}

	list := strings.Join(lines, "\n")
	if list == "" {
		list = "(žiadne nadchádzajúce pobyty)"
	}

	return &MessagePlaceholderValues{
		PropertyName:     prop.Name,
		CheckInTime:      checkIn,
		CheckOutTime:     checkOut,
		CleaningFromDate: fmt.Sprintf("%d.%d.", today.Day(), int(today.Month())),
		CleaningList:     list,
	}, len(lines), nil
}
