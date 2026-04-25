package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                  string
	DatabaseURL               string
	CORSOrigins               []string
	SessionTTL                time.Duration
	FirstSuperadmin           FirstSuperadmin
	DataDir                   string
	Env                       string
	CookieSecure              bool
	CookieSameSite            string
	OccupancySyncInterval     time.Duration
	NukiCleanupInterval       time.Duration
	CleaningReconcileInterval time.Duration
	NukiMockMode              bool
	NukiAPIBaseURL            string
	NukiHTTPTimeoutSeconds    int
	NukiLogFetchLimit         int
	NukiLogMaxPages           int
	MetricsToken              string
	MetricsBind               string
	MasterKey                 string
	BackupDir                 string
	BackupInterval            time.Duration
	AuditRetentionDays        int
	TOTPIssuer                string
	TOTPDevBypass             bool
	TrustedProxy              bool
}

type FirstSuperadmin struct {
	Email    string
	Password string
}

func Load() (*Config, error) {
	origins := []string{"http://localhost:5173"}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		origins = strings.Split(v, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}
	ttl := 7 * 24 * time.Hour
	if v := os.Getenv("SESSION_TTL_HOURS"); v != "" {
		h, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("SESSION_TTL_HOURS: %w", err)
		}
		ttl = time.Duration(h) * time.Hour
	}
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/pms.db"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	occMin := 60
	if v := os.Getenv("OCCUPANCY_SYNC_INTERVAL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			occMin = n
		}
	}
	nukiCleanupMin := 24 * 60
	if v := os.Getenv("NUKI_CLEANUP_INTERVAL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nukiCleanupMin = n
		}
	}
	cleaningReconcileMin := 24 * 60
	if v := os.Getenv("CLEANING_RECONCILE_INTERVAL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cleaningReconcileMin = n
		}
	}
	nukiTimeout := 15
	if v := os.Getenv("NUKI_HTTP_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nukiTimeout = n
		}
	}
	nukiLogFetchLimit := 500
	if v := os.Getenv("NUKI_LOG_FETCH_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nukiLogFetchLimit = n
		}
	}
	nukiLogMaxPages := 10
	if v := os.Getenv("NUKI_LOG_MAX_PAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nukiLogMaxPages = n
		}
	}
	nukiMock := false
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("NUKI_MOCK_MODE"))); v == "1" || v == "true" || v == "yes" {
		nukiMock = true
	}
	nukiBaseURL := strings.TrimSpace(os.Getenv("NUKI_API_BASE_URL"))
	if nukiBaseURL == "" {
		nukiBaseURL = "https://api.nuki.io"
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("PMS_ENV")))
	if env == "" {
		env = "production"
	}
	cookieSecure := env != "dev" && env != "development" && env != "test"
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("PMS_COOKIE_SECURE"))); v != "" {
		switch v {
		case "1", "true", "yes", "on":
			cookieSecure = true
		case "0", "false", "no", "off":
			cookieSecure = false
		default:
			return nil, fmt.Errorf("PMS_COOKIE_SECURE: invalid value %q (expected true/false)", v)
		}
	}
	cookieSameSite := "lax"
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("PMS_COOKIE_SAMESITE"))); v != "" {
		switch v {
		case "lax", "strict", "none":
			cookieSameSite = v
		default:
			return nil, fmt.Errorf("PMS_COOKIE_SAMESITE: invalid value %q (expected lax|strict|none)", v)
		}
	}
	if cookieSameSite == "none" && !cookieSecure {
		return nil, fmt.Errorf("PMS_COOKIE_SAMESITE=none requires PMS_COOKIE_SECURE=true")
	}
	// Reject the common misconfig CORS_ORIGINS="*" when credentials are
	// forwarded — this combination is specified as invalid by the Fetch
	// standard and chi/cors will silently decline the request anyway.
	for _, o := range origins {
		if o == "*" {
			return nil, fmt.Errorf("CORS_ORIGINS cannot be \"*\" because credentials are sent with every request")
		}
	}
	metricsToken := strings.TrimSpace(os.Getenv("PMS_METRICS_TOKEN"))
	metricsBind := strings.TrimSpace(os.Getenv("PMS_METRICS_BIND"))
	if env == "production" && metricsToken == "" && metricsBind == "" {
		return nil, fmt.Errorf("PMS_METRICS_TOKEN or PMS_METRICS_BIND is required when PMS_ENV=production")
	}
	masterKey := strings.TrimSpace(os.Getenv("PMS_MASTER_KEY"))
	if env == "production" && masterKey == "" {
		return nil, fmt.Errorf("PMS_MASTER_KEY is required when PMS_ENV=production (32-byte key, base64-encoded)")
	}
	backupDir := strings.TrimSpace(os.Getenv("PMS_BACKUP_DIR"))
	backupMin := 60
	if v := strings.TrimSpace(os.Getenv("PMS_BACKUP_INTERVAL_MINUTES")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("PMS_BACKUP_INTERVAL_MINUTES: %w", err)
		}
		backupMin = n
	}
	auditRetention := 365
	if v := strings.TrimSpace(os.Getenv("AUDIT_LOG_RETENTION_DAYS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("AUDIT_LOG_RETENTION_DAYS: invalid value %q", v)
		}
		auditRetention = n
	}
	// TOTP / 2FA configuration. Per-user opt-in; no user is forced to enrol.
	totpIssuer := strings.TrimSpace(os.Getenv("PMS_2FA_ISSUER"))
	if totpIssuer == "" {
		totpIssuer = "PMS"
	}
	// PMS_2FA_DEV_BYPASS lets operators skip the TOTP prompt on enrolled
	// users. Only honoured when PMS_ENV=dev so it cannot leak to production
	// deployments; accepting it in test is explicit rather than implicit.
	totpBypass := false
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("PMS_2FA_DEV_BYPASS"))); v == "1" || v == "true" || v == "yes" {
		totpBypass = true
	}
	if totpBypass && env != "dev" && env != "development" && env != "test" {
		return nil, fmt.Errorf("PMS_2FA_DEV_BYPASS is only allowed when PMS_ENV=dev|development|test")
	}
	// TrustedProxy lets the rate-limiter and access log key on the original
	// client IP exposed by a reverse proxy via X-Forwarded-For. Default off
	// so a misconfigured deployment can't be tricked by spoofed headers.
	trustedProxy := false
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("PMS_TRUSTED_PROXY"))); v == "1" || v == "true" || v == "yes" {
		trustedProxy = true
	}
	return &Config{
		HTTPAddr:                  getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:               "sqlite://" + dbPath + "?_pragma=foreign_keys(1)",
		CORSOrigins:               origins,
		SessionTTL:                ttl,
		OccupancySyncInterval:     time.Duration(occMin) * time.Minute,
		NukiCleanupInterval:       time.Duration(nukiCleanupMin) * time.Minute,
		CleaningReconcileInterval: time.Duration(cleaningReconcileMin) * time.Minute,
		NukiMockMode:              nukiMock,
		NukiAPIBaseURL:            nukiBaseURL,
		NukiHTTPTimeoutSeconds:    nukiTimeout,
		NukiLogFetchLimit:         nukiLogFetchLimit,
		NukiLogMaxPages:           nukiLogMaxPages,
		FirstSuperadmin: FirstSuperadmin{
			Email:    strings.TrimSpace(os.Getenv("FIRST_SUPERADMIN_EMAIL")),
			Password: os.Getenv("FIRST_SUPERADMIN_PASSWORD"),
		},
		DataDir:        dataDir,
		Env:            env,
		CookieSecure:   cookieSecure,
		CookieSameSite: cookieSameSite,
		MetricsToken:   metricsToken,
		MetricsBind:    metricsBind,
		MasterKey:      masterKey,
		BackupDir:      backupDir,
		BackupInterval: time.Duration(backupMin) * time.Minute,
		AuditRetentionDays: auditRetention,
		TOTPIssuer:         totpIssuer,
		TOTPDevBypass:      totpBypass,
		TrustedProxy:       trustedProxy,
	}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
