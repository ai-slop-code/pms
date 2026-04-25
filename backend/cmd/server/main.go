package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/time/rate"

	"pms/backend/internal/api"
	"pms/backend/internal/auth"
	"pms/backend/internal/backup"
	"pms/backend/internal/config"
	"pms/backend/internal/crypto/secretbox"
	"pms/backend/internal/dbconn"
	"pms/backend/internal/logging"
	"pms/backend/internal/metrics"
	pmsmw "pms/backend/internal/middleware"
	mig "pms/backend/internal/migrate"
	"pms/backend/internal/nuki"
	"pms/backend/internal/occupancy"
	"pms/backend/internal/otelx"
	"pms/backend/internal/sentryx"
	"pms/backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	logging.Init(cfg.Env)
	if err := sentryx.Init(cfg.Env, os.Getenv("PMS_RELEASE")); err != nil {
		log.Printf("sentryx: init failed: %v", err)
	}
	defer sentryx.Flush()
	otelShutdown, err := otelx.Init(context.Background(), cfg.Env, os.Getenv("PMS_RELEASE"))
	if err != nil {
		log.Printf("otelx: init failed: %v", err)
	}
	defer func() {
		if err := otelShutdown(context.Background()); err != nil {
			log.Printf("otelx: shutdown: %v", err)
		}
	}()
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/pms.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := dbconn.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := mig.Up(db); err != nil {
		log.Fatal("migrate: ", err)
	}
	if err := relocateLegacyFinanceAttachments(db, cfg.DataDir); err != nil {
		log.Printf("attachment migration: %v", err)
	}
	st := &store.Store{DB: db}
	if cfg.MasterKey != "" {
		box, err := secretbox.New(cfg.MasterKey)
		if err != nil {
			log.Fatal("PMS_MASTER_KEY: ", err)
		}
		st.Crypto = box
	}
	ctx := context.Background()
	if n, err := st.CountUsers(ctx); err == nil && n == 0 && cfg.FirstSuperadmin.Email != "" && cfg.FirstSuperadmin.Password != "" {
		hash, err := auth.HashPassword(cfg.FirstSuperadmin.Password)
		if err != nil {
			log.Fatal(err)
		}
		u, err := st.CreateUser(ctx, cfg.FirstSuperadmin.Email, hash, "super_admin")
		if err != nil {
			log.Fatal("bootstrap superadmin: ", err)
		}
		// The bootstrap password lives in env / .env / CI history. Force the
		// operator to rotate it on first login before they can do anything
		// else (see PMS_11 follow-up #4 / point 4).
		if err := st.SetMustChangePassword(ctx, u.ID, true); err != nil {
			log.Fatal("bootstrap superadmin must_change_password: ", err)
		}
		log.Printf("created first super_admin user %s (must change password on first login)", cfg.FirstSuperadmin.Email)
	}
	occSvc := &occupancy.Service{Store: st}
	nukiSvc := &nuki.Service{
		Store: st,
		Client: nuki.NewClient(nuki.Config{
			BaseURL:       cfg.NukiAPIBaseURL,
			Timeout:       time.Duration(cfg.NukiHTTPTimeoutSeconds) * time.Second,
			Mock:          cfg.NukiMockMode,
			LogFetchLimit: cfg.NukiLogFetchLimit,
			LogMaxPages:   cfg.NukiLogMaxPages,
		}),
	}
	var sameSite http.SameSite
	switch cfg.CookieSameSite {
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		sameSite = http.SameSiteNoneMode
	default:
		sameSite = http.SameSiteLaxMode
	}
	srv := &api.Server{
		Store:                   st,
		SessionTTL:              cfg.SessionTTL,
		Occ:                     occSvc,
		Nuki:                    nukiSvc,
		DataDir:                 cfg.DataDir,
		CookieSecure:            cfg.CookieSecure,
		CookieSameSite:          sameSite,
		LoginRateLimiter:        pmsmw.NewKeyedLimiter(rate.Every(2*time.Second), 5),
		AdminBackupLimiter:      pmsmw.NewKeyedLimiter(rate.Every(30*time.Second), 1),
		InvoiceRegenLimiter:     pmsmw.NewKeyedLimiter(rate.Every(5*time.Second), 3),
		AttachmentUploadLimiter: pmsmw.NewKeyedLimiter(rate.Every(2*time.Second), 5),
		TOTPIssuer:              cfg.TOTPIssuer,
		TOTPDevBypass:           cfg.TOTPDevBypass,
		AllowedOrigins:          cfg.CORSOrigins,
		TrustedProxy:            cfg.TrustedProxy,
	}
	instanceID := generateInstanceID()
	log.Printf("scheduler instance id: %s", instanceID)
	pmsmw.SetAccessObserver(func(method string, status int, elapsed time.Duration) {
		metrics.ObserveHTTPRequest(method, status, elapsed)
	})

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go runScheduler(rootCtx, "occupancy_sync", cfg.OccupancySyncInterval, func(bg context.Context) {
		ok, err := st.TryAcquireJobLease(bg, "occupancy_sync", instanceID, leaseTTL(cfg.OccupancySyncInterval))
		if err != nil {
			metrics.RecordSchedulerRun("occupancy_sync", "error")
			log.Printf("occupancy scheduler: lease error: %v", err)
			return
		}
		if !ok {
			metrics.RecordSchedulerRun("occupancy_sync", "skipped")
			return
		}
		ids, err := st.ListPropertyIDsWithICSURL(bg)
		if err != nil {
			metrics.RecordSchedulerRun("occupancy_sync", "error")
			log.Printf("occupancy scheduler: list properties: %v", err)
			return
		}
		for _, id := range ids {
			if err := occSvc.SyncProperty(bg, id, "scheduled"); err != nil {
				log.Printf("occupancy sync property %d: %v", id, err)
				continue
			}
		}
		metrics.RecordSchedulerRun("occupancy_sync", "ran")
	})

	go runScheduler(rootCtx, "nuki_cleanup", cfg.NukiCleanupInterval, func(bg context.Context) {
		ok, err := st.TryAcquireJobLease(bg, "nuki_cleanup", instanceID, leaseTTL(cfg.NukiCleanupInterval))
		if err != nil {
			metrics.RecordSchedulerRun("nuki_cleanup", "error")
			log.Printf("nuki cleanup scheduler: lease error: %v", err)
			return
		}
		if !ok {
			metrics.RecordSchedulerRun("nuki_cleanup", "skipped")
			return
		}
		ids, err := st.ListPropertyIDsWithNukiConfig(bg)
		if err != nil {
			metrics.RecordSchedulerRun("nuki_cleanup", "error")
			log.Printf("nuki cleanup scheduler: list properties: %v", err)
			return
		}
		for _, id := range ids {
			if err := nukiSvc.CleanupExpiredCodes(bg, id); err != nil {
				log.Printf("nuki cleanup property %d: %v", id, err)
			}
		}
		metrics.RecordSchedulerRun("nuki_cleanup", "ran")
	})

	go runScheduler(rootCtx, "cleaning_reconcile", cfg.CleaningReconcileInterval, func(bg context.Context) {
		ok, err := st.TryAcquireJobLease(bg, "cleaning_reconcile", instanceID, leaseTTL(cfg.CleaningReconcileInterval))
		if err != nil {
			metrics.RecordSchedulerRun("cleaning_reconcile", "error")
			log.Printf("cleaning reconcile scheduler: lease error: %v", err)
			return
		}
		if !ok {
			metrics.RecordSchedulerRun("cleaning_reconcile", "skipped")
			return
		}
		ids, err := st.ListPropertyIDsWithCleanerAuthID(bg)
		if err != nil {
			metrics.RecordSchedulerRun("cleaning_reconcile", "error")
			log.Printf("cleaning reconcile scheduler: list properties: %v", err)
			return
		}
		for _, id := range ids {
			if _, err := nukiSvc.ReconcileCleanerDailyLogs(bg, id); err != nil {
				log.Printf("cleaning reconcile property %d: %v", id, err)
			}
		}
		metrics.RecordSchedulerRun("cleaning_reconcile", "ran")
	})

	if cfg.BackupInterval > 0 {
		backupDir := cfg.BackupDir
		if backupDir == "" {
			backupDir = filepath.Join(cfg.DataDir, "backups")
		}
		go runScheduler(rootCtx, "backup_snapshot", cfg.BackupInterval, func(bg context.Context) {
			path, err := backup.Snapshot(bg, db, backupDir, time.Now().UTC())
			if err != nil {
				metrics.RecordSchedulerRun("backup_snapshot", "error")
				log.Printf("backup snapshot: %v", err)
				return
			}
			if _, err := backup.Prune(backupDir, time.Now().UTC(), 24, 7); err != nil {
				log.Printf("backup prune: %v", err)
			}
			metrics.RecordSchedulerRun("backup_snapshot", "ran")
			metrics.RecordBackupSuccess(time.Now().UTC())
			log.Printf("backup snapshot written: %s", path)
		})
	}

	if cfg.AuditRetentionDays > 0 {
		retentionInterval := 24 * time.Hour
		go runScheduler(rootCtx, "audit_retention", retentionInterval, func(bg context.Context) {
			cutoff := time.Now().UTC().AddDate(0, 0, -cfg.AuditRetentionDays)
			n, err := st.DeleteAuditLogsBefore(bg, cutoff)
			if err != nil {
				metrics.RecordSchedulerRun("audit_retention", "error")
				log.Printf("audit retention: %v", err)
				return
			}
			metrics.RecordAuditLogDeletion(n)
			metrics.RecordSchedulerRun("audit_retention", "ran")
			if n > 0 {
				log.Printf("audit retention: pruned %d rows older than %d days", n, cfg.AuditRetentionDays)
			}
		})
	}
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelx.Middleware("pms-backend"))
	r.Use(pmsmw.AccessLog)
	r.Use(pmsmw.SecurityHeaders)
	r.Use(chimw.Recoverer)
	r.Use(sentryx.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-PMS-Client"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	if cfg.MetricsBind == "" {
		r.Handle("/metrics", metricsHandler(cfg.MetricsToken))
	}
	r.Mount("/", srv.Routes())

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return rootCtx },
	}

	var metricsSrv *http.Server
	if cfg.MetricsBind != "" {
		mr := chi.NewRouter()
		mr.Handle("/metrics", metricsHandler(cfg.MetricsToken))
		metricsSrv = &http.Server{
			Addr:              cfg.MetricsBind,
			Handler:           mr,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return rootCtx },
		}
		go func() {
			log.Printf("metrics listener on %s", cfg.MetricsBind)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("metrics server: %v", err)
			}
		}()
	}

	go func() {
		log.Printf("listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-rootCtx.Done()
	log.Printf("shutdown signal received; draining")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}
}

// safeTick invokes fn and recovers from panics, logging them without killing
// the scheduler goroutine.
func safeTick(name string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("scheduler %s: panic: %v", name, rec)
		}
	}()
	fn()
}

// runScheduler runs tick every interval until ctx is cancelled. Each tick
// invocation is isolated from panics via safeTick.
func runScheduler(ctx context.Context, name string, interval time.Duration, tick func(ctx context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			safeTick(name, func() { tick(ctx) })
		}
	}
}

// generateInstanceID produces a short, random identifier for this process. It
// is used as the `owner` value in job_leases so a second replica can tell its
// own renewals apart from a peer's lease.
func generateInstanceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a timestamp-derived value if /dev/urandom is unavailable.
		return "pms-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return "pms-" + hex.EncodeToString(b[:])
}

// leaseTTL returns a lease duration slightly longer than the scheduler
// interval so a brief delay (GC pause, slow query) doesn't cause the lease to
// expire mid-run and let a peer start a second copy.
func leaseTTL(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 5 * time.Minute
	}
	return interval + interval/2
}

// metricsHandler returns the Prometheus exposition handler, optionally gated
// on a bearer token. An empty token returns the base handler unchanged;
// callers in production should always supply one or bind the registry to a
// private interface.
func metricsHandler(token string) http.Handler {
	base := metrics.Handler()
	if token == "" {
		return base
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix || auth[len(prefix):] != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		base.ServeHTTP(w, r)
	})
}
