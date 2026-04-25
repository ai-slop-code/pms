package api

type userDTO struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
}

type usersResponse struct {
	Users []userDTO `json:"users"`
}

type userResponse struct {
	User userDTO `json:"user"`
}

// loginResponse is what POST /api/auth/login returns.
// If MFARequired is true, User is omitted — the caller must POST the TOTP
// code to /api/auth/2fa/verify before the session can be used.
type loginResponse struct {
	User        *userDTO `json:"user,omitempty"`
	MFARequired bool     `json:"mfa_required,omitempty"`
}

// meResponse is what GET /api/auth/me returns. When a session is pending
// its 2FA challenge, User is nil and MFARequired is true so the SPA can
// redirect to the challenge screen.
type meResponse struct {
	User        *userDTO `json:"user,omitempty"`
	MFARequired bool     `json:"mfa_required,omitempty"`
}

type userWithPermissionsResponse struct {
	User                userDTO                 `json:"user"`
	PropertyPermissions []propertyPermissionDTO `json:"property_permissions"`
}

type propertyPermissionDTO struct {
	ID              int64  `json:"id"`
	PropertyID      int64  `json:"property_id"`
	Module          string `json:"module"`
	PermissionLevel string `json:"permission_level"`
}

type propertyDTO struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Timezone        string  `json:"timezone"`
	DefaultLanguage string  `json:"default_language"`
	DefaultCurrency string  `json:"default_currency"`
	InvoiceCode     *string `json:"invoice_code"`
	OwnerUserID     int64   `json:"owner_user_id"`
	AddressLine1    *string `json:"address_line1"`
	City            *string `json:"city"`
	PostalCode      *string `json:"postal_code"`
	Country         *string `json:"country"`
	WeekStartsOn    string  `json:"week_starts_on"`
	Active          bool    `json:"active"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type propertiesResponse struct {
	Properties []propertyDTO `json:"properties"`
}

type propertyResponse struct {
	Property propertyDTO `json:"property"`
}

type propertySettingsProfileDTO struct {
	LegalOwnerName      *string `json:"legal_owner_name"`
	BillingName         *string `json:"billing_name"`
	BillingAddress      *string `json:"billing_address"`
	City                *string `json:"city"`
	PostalCode          *string `json:"postal_code"`
	Country             *string `json:"country"`
	ICO                 *string `json:"ico"`
	DIC                 *string `json:"dic"`
	VATID               *string `json:"vat_id"`
	ContactPhone        *string `json:"contact_phone"`
	WifiSSID            *string `json:"wifi_ssid"`
	WifiPasswordSet     bool    `json:"wifi_password_set"`
	ParkingInstructions *string `json:"parking_instructions"`
	DefaultCheckInTime  string  `json:"default_check_in_time"`
	DefaultCheckOutTime string  `json:"default_check_out_time"`
	CleanerNukiAuthID   *string `json:"cleaner_nuki_auth_id"`
}

type propertyIntegrationsDTO struct {
	BookingICSConfigured bool `json:"booking_ics_configured"`
	NukiConfigured       bool `json:"nuki_configured"`
}

type propertySettingsResponse struct {
	Profile      propertySettingsProfileDTO `json:"profile"`
	Integrations propertyIntegrationsDTO    `json:"integrations"`
}

type syncStatusWidget struct {
	Occupancy string `json:"occupancy,omitempty"`
	Nuki      string `json:"nuki,omitempty"`
}

type dashboardUpcomingStayRow struct {
	OccupancyID int64   `json:"occupancy_id"`
	Summary     *string `json:"summary"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
	Status      string  `json:"status"`
}

type dashboardActiveNukiCodeRow struct {
	OccupancyID   int64   `json:"occupancy_id"`
	Summary       *string `json:"summary"`
	CodeLabel     *string `json:"code_label"`
	CodeMasked    *string `json:"code_masked"`
	Status        string  `json:"status"`
	ValidFrom     *string `json:"valid_from"`
	ValidUntil    *string `json:"valid_until"`
	LastUpdatedAt *string `json:"last_updated_at"`
	ErrorMessage  *string `json:"error_message"`
}

type cleaningMonthWidget struct {
	CountedDays int `json:"counted_days"`
	SalaryDraft int `json:"salary_draft"`
}

type financeMonthWidget struct {
	Incoming int `json:"incoming"`
	Outgoing int `json:"outgoing"`
	Net      int `json:"net"`
}

type dashboardInvoiceRow struct {
	InvoiceID     int64   `json:"invoice_id"`
	InvoiceNumber string  `json:"invoice_number"`
	CustomerName  *string `json:"customer_name"`
	AmountTotal   int     `json:"amount_total_cents"`
	IssueDate     string  `json:"issue_date"`
	Version       int     `json:"version"`
}

type dashboardWidgetsDTO struct {
	UpcomingStays   *[]dashboardUpcomingStayRow   `json:"upcoming_stays,omitempty"`
	ActiveNukiCodes *[]dashboardActiveNukiCodeRow `json:"active_nuki_codes,omitempty"`
	SyncStatus      *syncStatusWidget             `json:"sync_status,omitempty"`
	CleaningMonth   *cleaningMonthWidget          `json:"cleaning_month,omitempty"`
	FinanceMonth    *financeMonthWidget           `json:"finance_month,omitempty"`
	RecentInvoices  *[]dashboardInvoiceRow        `json:"recent_invoices,omitempty"`
}

type dashboardSummaryResponse struct {
	PropertyID int64               `json:"property_id"`
	Widgets    dashboardWidgetsDTO `json:"widgets"`
}
