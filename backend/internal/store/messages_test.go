package store

import (
	"testing"
)

func TestRenderMessageTemplate(t *testing.T) {
	body := `Welcome to {{property_name}}!
Stay: {{stay_start}} – {{stay_end}}
Code: {{nuki_code}}
Wi-Fi: {{wifi_name}} / {{wifi_password}}
Parking: {{parking_info}}
Phone: {{contact_phone}}
Address: {{property_address}}
Check-in: {{check_in_time}}, Check-out: {{check_out_time}}`

	vals := MessagePlaceholderValues{
		PropertyName:    "Alpine Lodge",
		PropertyAddress: "Main St 1, Bratislava, 81101",
		StayStart:       "15/04/2026",
		StayEnd:         "18/04/2026",
		CheckInTime:     "14:00",
		CheckOutTime:    "10:00",
		NukiCode:        "1234",
		WifiName:        "Lodge-WiFi",
		WifiPassword:    "s3cret",
		ParkingInfo:     "Spot #3",
		ContactPhone:    "+421900111222",
	}

	got := RenderMessageTemplate(body, vals)

	expects := []string{
		"Alpine Lodge",
		"15/04/2026",
		"18/04/2026",
		"1234",
		"Lodge-WiFi",
		"s3cret",
		"Spot #3",
		"+421900111222",
		"Main St 1, Bratislava, 81101",
		"14:00",
		"10:00",
	}

	for _, want := range expects {
		if !contains(got, want) {
			t.Errorf("rendered output missing %q\nGot:\n%s", want, got)
		}
	}

	if contains(got, "{{") {
		t.Errorf("unresolved placeholders remain in output:\n%s", got)
	}
}

func TestRenderMessageTemplate_MissingNuki(t *testing.T) {
	body := "Code: {{nuki_code}}"
	vals := MessagePlaceholderValues{NukiCode: "—"}
	got := RenderMessageTemplate(body, vals)
	if got != "Code: —" {
		t.Errorf("expected dash placeholder, got %q", got)
	}
}

func TestValidateTemplatePlaceholders_Valid(t *testing.T) {
	body := "Hello {{property_name}}, code {{nuki_code}}"
	invalid := ValidateTemplatePlaceholders(body)
	if len(invalid) != 0 {
		t.Errorf("expected no invalid placeholders, got %v", invalid)
	}
}

func TestValidateTemplatePlaceholders_Invalid(t *testing.T) {
	body := "Hello {{property_name}}, phone {{phone_number}}, extra {{foo}}"
	invalid := ValidateTemplatePlaceholders(body)
	if len(invalid) != 2 {
		t.Fatalf("expected 2 invalid placeholders, got %v", invalid)
	}
	found := map[string]bool{}
	for _, s := range invalid {
		found[s] = true
	}
	if !found["{{phone_number}}"] {
		t.Error("expected {{phone_number}} in invalid list")
	}
	if !found["{{foo}}"] {
		t.Error("expected {{foo}} in invalid list")
	}
}

func TestValidateTemplatePlaceholders_NoBraces(t *testing.T) {
	body := "Hello world, no placeholders at all."
	invalid := ValidateTemplatePlaceholders(body)
	if len(invalid) != 0 {
		t.Errorf("expected no invalid placeholders, got %v", invalid)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
