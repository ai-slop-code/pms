package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"pms/backend/internal/crypto/secretbox"
	"pms/backend/internal/permissions"
)

// Store wraps the database connection. Crypto, if set, is applied to
// at-rest-sensitive columns (property secrets, generated PINs) transparently
// via the encryptNS / decryptNS helpers.
type Store struct {
	DB     *sql.DB
	Crypto *secretbox.Box
}

// encryptNS encrypts a NullString in place, returning a value safe to pass
// into placeholder binds. No-op when Crypto is nil or the value is empty.
func (s *Store) encryptNS(v sql.NullString) (sql.NullString, error) {
	if s.Crypto == nil || !v.Valid || v.String == "" {
		return v, nil
	}
	ct, err := s.Crypto.Encrypt(v.String)
	if err != nil {
		return v, err
	}
	return sql.NullString{String: ct, Valid: true}, nil
}

// decryptNS decrypts a NullString in place, transparently passing legacy
// plaintext through. No-op when Crypto is nil.
func (s *Store) decryptNS(v *sql.NullString) error {
	if s.Crypto == nil || v == nil || !v.Valid || v.String == "" {
		return nil
	}
	pt, err := s.Crypto.Decrypt(v.String)
	if err != nil {
		return err
	}
	v.String = pt
	return nil
}

type User struct {
	ID           int64
	Email        string
	Role         string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	PasswordHash string
	// MustChangePassword forces the user through the password-change flow
	// on next login. Set to true for the bootstrap super_admin and cleared
	// automatically when the user updates their own password.
	MustChangePassword bool
}

type Property struct {
	ID              int64
	Name            string
	Timezone        string
	DefaultLanguage string
	DefaultCurrency string
	InvoiceCode     sql.NullString
	OwnerUserID     int64
	AddressLine1    sql.NullString
	City            sql.NullString
	PostalCode      sql.NullString
	Country         sql.NullString
	WeekStartsOn    string
	Active          bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PropertyProfile struct {
	PropertyID          int64
	LegalOwnerName      sql.NullString
	BillingName         sql.NullString
	BillingAddress      sql.NullString
	City                sql.NullString
	PostalCode          sql.NullString
	Country             sql.NullString
	ICO                 sql.NullString
	DIC                 sql.NullString
	VATID               sql.NullString
	ContactPhone        sql.NullString
	WifiSSID            sql.NullString
	WifiPassword        sql.NullString
	ParkingInstructions sql.NullString
	DefaultCheckInTime  string
	DefaultCheckOutTime string
	CleanerNukiAuthID   sql.NullString
	UpdatedAt           time.Time
}

type PropertySecrets struct {
	PropertyID      int64
	BookingICSURL   sql.NullString
	NukiAPIToken    sql.NullString
	NukiSmartlockID sql.NullString
	UpdatedAt       time.Time
}

type PropertyUserPermission struct {
	ID              int64
	UserID          int64
	PropertyID      int64
	Module          string
	PermissionLevel string
	CreatedAt       time.Time
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, role string) (*User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, active, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
		email, passwordHash, role, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	var active, mustChange int
	var created, updated string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, active, created_at, updated_at, must_change_password FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &active, &created, &updated, &mustChange)
	if err != nil {
		return nil, err
	}
	u.Active = active == 1
	u.MustChangePassword = mustChange == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	var active, mustChange int
	var created, updated string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, active, created_at, updated_at, must_change_password FROM users WHERE email = ? COLLATE NOCASE`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &active, &created, &updated, &mustChange)
	if err != nil {
		return nil, err
	}
	u.Active = active == 1
	u.MustChangePassword = mustChange == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, email, role, active, created_at, updated_at FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var active int
		var created, updated string
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &active, &created, &updated); err != nil {
			return nil, err
		}
		u.Active = active == 1
		u.CreatedAt, _ = time.Parse(time.RFC3339, created)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, email *string, role *string, active *bool, newPasswordHash *string) (*User, error) {
	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if email != nil {
		u.Email = *email
	}
	if role != nil {
		u.Role = *role
	}
	if active != nil {
		u.Active = *active
	}
	if newPasswordHash != nil {
		u.PasswordHash = *newPasswordHash
		// Self-service password rotation always clears the forced-change flag.
		// Admin-initiated password resets keep it set so the operator hands
		// out a temporary password.
	}
	now := time.Now().UTC().Format(time.RFC3339)
	activeInt := 0
	if u.Active {
		activeInt = 1
	}
	_, err = s.DB.ExecContext(ctx,
		`UPDATE users SET email = ?, role = ?, active = ?, password_hash = ?, updated_at = ? WHERE id = ?`,
		u.Email, u.Role, activeInt, u.PasswordHash, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

// SetMustChangePassword toggles the forced-change flag on a user. Used by
// the bootstrap routine and admin-initiated password resets.
func (s *Store) SetMustChangePassword(ctx context.Context, id int64, must bool) error {
	v := 0
	if must {
		v = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET must_change_password = ?, updated_at = ? WHERE id = ?`, v, now, id)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO auth_sessions (user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC().Format(time.RFC3339), now)
	return err
}

func (s *Store) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteSessionsForUserExcept removes every auth_sessions row for the user
// except the one identified by keepTokenHash. Used to invalidate all other
// sessions when a password is changed (PMS_11/T2.4).
func (s *Store) DeleteSessionsForUserExcept(ctx context.Context, userID int64, keepTokenHash string) error {
	_, err := s.DB.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE user_id = ? AND token_hash <> ?`,
		userID, keepTokenHash)
	return err
}

// DeleteSessionsForUser removes every auth_sessions row for the user. Used by
// admin-initiated revoke-all and when a user's password is changed by an
// administrator.
func (s *Store) DeleteSessionsForUser(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) GetUserIDBySessionHash(ctx context.Context, tokenHash string) (int64, error) {
	var uid int64
	var exp string
	err := s.DB.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM auth_sessions WHERE token_hash = ?`, tokenHash).Scan(&uid, &exp)
	if err != nil {
		return 0, err
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil || time.Now().UTC().After(t) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, tokenHash)
		return 0, sql.ErrNoRows
	}
	return uid, nil
}

func (s *Store) IsPropertyOwner(ctx context.Context, userID, propertyID int64) (bool, error) {
	var oid int64
	err := s.DB.QueryRowContext(ctx, `SELECT owner_user_id FROM properties WHERE id = ?`, propertyID).Scan(&oid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return oid == userID, nil
}

func (s *Store) PermissionForModule(ctx context.Context, userID, propertyID int64, module string) (string, error) {
	var level string
	err := s.DB.QueryRowContext(ctx,
		`SELECT permission_level FROM property_user_permissions WHERE user_id = ? AND property_id = ? AND module = ?`,
		userID, propertyID, module).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return level, nil
}

func (s *Store) UserCan(ctx context.Context, user *User, propertyID int64, module, minLevel string) (bool, error) {
	if user == nil || !user.Active {
		return false, nil
	}
	if user.Role == "super_admin" {
		return true, nil
	}
	owner, err := s.IsPropertyOwner(ctx, user.ID, propertyID)
	if err != nil {
		return false, err
	}
	if owner {
		return permissions.Meets(minLevel, permissions.LevelAdmin), nil
	}
	level, err := s.PermissionForModule(ctx, user.ID, propertyID, module)
	if err != nil || level == "" {
		return false, err
	}
	return permissions.Meets(minLevel, level), nil
}

func (s *Store) ListPropertyIDsForUser(ctx context.Context, user *User) ([]int64, error) {
	if user.Role == "super_admin" {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM properties WHERE active = 1 ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT p.id FROM properties p
		WHERE p.active = 1 AND (
			p.owner_user_id = ?
			OR EXISTS (SELECT 1 FROM property_user_permissions pup WHERE pup.property_id = p.id AND pup.user_id = ?)
		)
		ORDER BY p.name`, user.ID, user.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ListPropertiesForUser(ctx context.Context, user *User) ([]Property, error) {
	if user == nil || !user.Active {
		return nil, nil
	}
	query := `
		SELECT DISTINCT p.id, p.name, p.timezone, p.default_language, p.default_currency, p.invoice_code, p.owner_user_id,
			p.address_line1, p.city, p.postal_code, p.country, COALESCE(p.week_starts_on, 'monday'), p.active, p.created_at, p.updated_at
		FROM properties p
		WHERE p.active = 1`
	args := []interface{}{}
	if user.Role != "super_admin" {
		query += ` AND (
			p.owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM property_user_permissions pup
				WHERE pup.property_id = p.id AND pup.user_id = ?
			)
		)`
		args = append(args, user.ID, user.ID)
	}
	query += " ORDER BY p.name"
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Property
	for rows.Next() {
		var p Property
		var active int
		var created, updated string
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Timezone, &p.DefaultLanguage, &p.DefaultCurrency, &p.InvoiceCode, &p.OwnerUserID,
			&p.AddressLine1, &p.City, &p.PostalCode, &p.Country, &p.WeekStartsOn, &active, &created, &updated,
		); err != nil {
			return nil, err
		}
		p.Active = active == 1
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProperty(ctx context.Context, id int64) (*Property, error) {
	var p Property
	var active int
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, name, timezone, default_language, default_currency, invoice_code, owner_user_id,
			address_line1, city, postal_code, country, COALESCE(week_starts_on, 'monday'), active, created_at, updated_at
		FROM properties WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Timezone, &p.DefaultLanguage, &p.DefaultCurrency, &p.InvoiceCode, &p.OwnerUserID,
			&p.AddressLine1, &p.City, &p.PostalCode, &p.Country, &p.WeekStartsOn, &active, &created, &updated)
	if err != nil {
		return nil, err
	}
	p.Active = active == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &p, nil
}

func (s *Store) UserCanSeeProperty(ctx context.Context, user *User, propertyID int64) (bool, error) {
	if user == nil || !user.Active {
		return false, nil
	}
	if user.Role == "super_admin" {
		return true, nil
	}
	var found int
	err := s.DB.QueryRowContext(ctx, `
		SELECT 1
		FROM properties p
		WHERE p.id = ? AND p.active = 1 AND (
			p.owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM property_user_permissions pup
				WHERE pup.property_id = p.id AND pup.user_id = ?
			)
		)
		LIMIT 1`,
		propertyID, user.ID, user.ID,
	).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return found == 1, nil
}

func (s *Store) CreateProperty(ctx context.Context, ownerUserID int64, name, timezone, defaultLang string) (*Property, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if timezone == "" {
		timezone = "Europe/Bratislava"
	}
	if defaultLang == "" {
		defaultLang = "sk"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO properties (name, timezone, default_language, default_currency, owner_user_id, active, created_at, updated_at)
		VALUES (?, ?, ?, 'EUR', ?, 1, ?, ?)`,
		name, timezone, defaultLang, ownerUserID, now, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	pid, _ := res.LastInsertId()
	_, err = tx.ExecContext(ctx, `INSERT INTO property_profiles (property_id, updated_at) VALUES (?, ?)`, pid, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO property_secrets (property_id, updated_at) VALUES (?, ?)`, pid, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := s.InsertOccupancySourceTx(ctx, tx, pid, now); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetProperty(ctx, pid)
}

func (s *Store) UpdateProperty(ctx context.Context, id int64, name *string, timezone *string, defaultLang *string,
	invoiceCode *string, addressLine1, city, postalCode, country *string, weekStartsOn *string, active *bool) (*Property, error) {
	p, err := s.GetProperty(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		p.Name = *name
	}
	if timezone != nil {
		p.Timezone = *timezone
	}
	if defaultLang != nil {
		p.DefaultLanguage = *defaultLang
	}
	if invoiceCode != nil {
		raw := strings.TrimSpace(*invoiceCode)
		if raw == "" {
			p.InvoiceCode = sql.NullString{}
		} else {
			p.InvoiceCode = sql.NullString{String: raw, Valid: true}
		}
	}
	if addressLine1 != nil {
		p.AddressLine1 = sql.NullString{String: *addressLine1, Valid: true}
	}
	if city != nil {
		p.City = sql.NullString{String: *city, Valid: true}
	}
	if postalCode != nil {
		p.PostalCode = sql.NullString{String: *postalCode, Valid: true}
	}
	if country != nil {
		p.Country = sql.NullString{String: *country, Valid: true}
	}
	if weekStartsOn != nil {
		v := strings.ToLower(strings.TrimSpace(*weekStartsOn))
		if v != "monday" && v != "sunday" {
			v = "monday"
		}
		p.WeekStartsOn = v
	}
	if active != nil {
		p.Active = *active
	}
	now := time.Now().UTC().Format(time.RFC3339)
	activeInt := 0
	if p.Active {
		activeInt = 1
	}
	if p.WeekStartsOn == "" {
		p.WeekStartsOn = "monday"
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE properties SET name = ?, timezone = ?, default_language = ?, invoice_code = ?, address_line1 = ?, city = ?, postal_code = ?, country = ?, week_starts_on = ?, active = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Timezone, p.DefaultLanguage, nullStr(p.InvoiceCode), nullStr(p.AddressLine1), nullStr(p.City), nullStr(p.PostalCode), nullStr(p.Country), p.WeekStartsOn, activeInt, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetProperty(ctx, id)
}

func nullStr(ns sql.NullString) interface{} {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func (s *Store) GetPropertyProfile(ctx context.Context, propertyID int64) (*PropertyProfile, error) {
	var pr PropertyProfile
	var updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT property_id, legal_owner_name, billing_name, billing_address, city, postal_code, country,
			ico, dic, vat_id, contact_phone, wifi_ssid, wifi_password, parking_instructions,
			default_check_in_time, default_check_out_time, cleaner_nuki_auth_id, updated_at
		FROM property_profiles WHERE property_id = ?`, propertyID).
		Scan(&pr.PropertyID, &pr.LegalOwnerName, &pr.BillingName, &pr.BillingAddress, &pr.City, &pr.PostalCode, &pr.Country,
			&pr.ICO, &pr.DIC, &pr.VATID, &pr.ContactPhone, &pr.WifiSSID, &pr.WifiPassword, &pr.ParkingInstructions,
			&pr.DefaultCheckInTime, &pr.DefaultCheckOutTime, &pr.CleanerNukiAuthID, &updated)
	if err != nil {
		return nil, err
	}
	pr.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &pr, nil
}

func (s *Store) GetPropertySecrets(ctx context.Context, propertyID int64) (*PropertySecrets, error) {
	var sec PropertySecrets
	var updated string
	err := s.DB.QueryRowContext(ctx,
		`SELECT property_id, booking_ics_url, nuki_api_token, nuki_smartlock_id, updated_at FROM property_secrets WHERE property_id = ?`, propertyID).
		Scan(&sec.PropertyID, &sec.BookingICSURL, &sec.NukiAPIToken, &sec.NukiSmartlockID, &updated)
	if err != nil {
		return nil, err
	}
	if err := s.decryptNS(&sec.BookingICSURL); err != nil {
		return nil, err
	}
	if err := s.decryptNS(&sec.NukiAPIToken); err != nil {
		return nil, err
	}
	sec.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &sec, nil
}

func (s *Store) UpdatePropertyProfile(ctx context.Context, propertyID int64, patch map[string]interface{}) error {
	pr, err := s.GetPropertyProfile(ctx, propertyID)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// apply known keys
	apply := func(key string, dest *sql.NullString) {
		if v, ok := patch[key]; ok {
			if v == nil {
				dest.Valid = false
				return
			}
			if s, ok := v.(string); ok {
				dest.String = s
				dest.Valid = true
			}
		}
	}
	apply("legal_owner_name", &pr.LegalOwnerName)
	apply("billing_name", &pr.BillingName)
	apply("billing_address", &pr.BillingAddress)
	apply("city", &pr.City)
	apply("postal_code", &pr.PostalCode)
	apply("country", &pr.Country)
	apply("ico", &pr.ICO)
	apply("dic", &pr.DIC)
	apply("vat_id", &pr.VATID)
	apply("contact_phone", &pr.ContactPhone)
	apply("wifi_ssid", &pr.WifiSSID)
	apply("wifi_password", &pr.WifiPassword)
	apply("parking_instructions", &pr.ParkingInstructions)
	apply("cleaner_nuki_auth_id", &pr.CleanerNukiAuthID)
	if v, ok := patch["default_check_in_time"]; ok {
		if s, ok := v.(string); ok {
			pr.DefaultCheckInTime = s
		}
	}
	if v, ok := patch["default_check_out_time"]; ok {
		if s, ok := v.(string); ok {
			pr.DefaultCheckOutTime = s
		}
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE property_profiles SET
			legal_owner_name = ?, billing_name = ?, billing_address = ?, city = ?, postal_code = ?, country = ?,
			ico = ?, dic = ?, vat_id = ?, contact_phone = ?, wifi_ssid = ?, wifi_password = ?, parking_instructions = ?,
			default_check_in_time = ?, default_check_out_time = ?, cleaner_nuki_auth_id = ?, updated_at = ?
		WHERE property_id = ?`,
		nullStr(pr.LegalOwnerName), nullStr(pr.BillingName), nullStr(pr.BillingAddress), nullStr(pr.City), nullStr(pr.PostalCode), nullStr(pr.Country),
		nullStr(pr.ICO), nullStr(pr.DIC), nullStr(pr.VATID), nullStr(pr.ContactPhone), nullStr(pr.WifiSSID), nullStr(pr.WifiPassword), nullStr(pr.ParkingInstructions),
		pr.DefaultCheckInTime, pr.DefaultCheckOutTime, nullStr(pr.CleanerNukiAuthID), now, propertyID)
	return err
}

func (s *Store) UpdatePropertySecrets(ctx context.Context, propertyID int64, bookingICS, nukiToken, nukiLockID *string) error {
	sec, err := s.GetPropertySecrets(ctx, propertyID)
	if err != nil {
		return err
	}
	if bookingICS != nil {
		sec.BookingICSURL = sql.NullString{String: *bookingICS, Valid: *bookingICS != ""}
	}
	if nukiToken != nil {
		sec.NukiAPIToken = sql.NullString{String: *nukiToken, Valid: *nukiToken != ""}
	}
	if nukiLockID != nil {
		sec.NukiSmartlockID = sql.NullString{String: *nukiLockID, Valid: *nukiLockID != ""}
	}
	icsEnc, err := s.encryptNS(sec.BookingICSURL)
	if err != nil {
		return err
	}
	tokEnc, err := s.encryptNS(sec.NukiAPIToken)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.DB.ExecContext(ctx, `
		UPDATE property_secrets SET booking_ics_url = ?, nuki_api_token = ?, nuki_smartlock_id = ?, updated_at = ?
		WHERE property_id = ?`,
		nullStr(icsEnc), nullStr(tokEnc), nullStr(sec.NukiSmartlockID), now, propertyID)
	return err
}

func (s *Store) ListPermissionsForUser(ctx context.Context, userID int64) ([]PropertyUserPermission, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_id, property_id, module, permission_level, created_at FROM property_user_permissions WHERE user_id = ? ORDER BY property_id, module`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PropertyUserPermission
	for rows.Next() {
		var p PropertyUserPermission
		var created string
		if err := rows.Scan(&p.ID, &p.UserID, &p.PropertyID, &p.Module, &p.PermissionLevel, &created); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpsertPropertyPermission(ctx context.Context, userID, propertyID int64, module, level string) (*PropertyUserPermission, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO property_user_permissions (user_id, property_id, module, permission_level, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, property_id, module) DO UPDATE SET permission_level = excluded.permission_level`,
		userID, propertyID, module, level, now)
	if err != nil {
		return nil, err
	}
	var p PropertyUserPermission
	var created string
	err = s.DB.QueryRowContext(ctx,
		`SELECT id, user_id, property_id, module, permission_level, created_at FROM property_user_permissions
		 WHERE user_id = ? AND property_id = ? AND module = ?`, userID, propertyID, module).
		Scan(&p.ID, &p.UserID, &p.PropertyID, &p.Module, &p.PermissionLevel, &created)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &p, nil
}

func (s *Store) DeletePropertyPermission(ctx context.Context, permissionID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM property_user_permissions WHERE id = ?`, permissionID)
	return err
}

func (s *Store) InsertAuditLog(ctx context.Context, actorID *int64, action, entityType, entityID, outcome, method, path string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO api_audit_logs (actor_user_id, action, entity_type, entity_id, outcome, request_method, request_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actorID, action, entityType, entityID, outcome, method, path, now)
	return err
}

// DeleteAuditLogsBefore removes audit entries older than `cutoff`. Returns the
// number of rows deleted so callers can emit retention metrics.
func (s *Store) DeleteAuditLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx,
		`DELETE FROM api_audit_logs WHERE created_at < ?`,
		cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
