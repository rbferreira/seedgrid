// The seedgrid-controller command is the entry point for the SeedGrid Control Plane.
//
// On boot it: loads the config, applies the migrations, connects to the DB,
// ensures the initial admin operator and starts the HTTP API.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/admission"
	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/agentrelease"
	"github.com/rbferreira/seedgrid/control-plane/internal/alert"
	"github.com/rbferreira/seedgrid/control-plane/internal/analytics"
	"github.com/rbferreira/seedgrid/control-plane/internal/api"
	"github.com/rbferreira/seedgrid/control-plane/internal/audit"
	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/clusterstats"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/config"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/enrich"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/googlebooks"
	"github.com/rbferreira/seedgrid/control-plane/internal/health"
	"github.com/rbferreira/seedgrid/control-plane/internal/igdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/maintenance"
	"github.com/rbferreira/seedgrid/control-plane/internal/metrics"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/notify"
	"github.com/rbferreira/seedgrid/control-plane/internal/openlibrary"
	"github.com/rbferreira/seedgrid/control-plane/internal/promquery"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
	"github.com/rbferreira/seedgrid/control-plane/internal/tmdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

// version is defined at build time via -ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("starting seedgrid-controller", "version", version)

	// Migrations on boot (embedded goose).
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("migrations applied")

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	authSvc := auth.NewService(pool, cfg.AgentRegistrationToken)
	if err := authSvc.Bootstrap(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}
	logger.Info("bootstrap completed", "admin", cfg.AdminUsername)
	usersSvc := users.NewService(pool)

	emitter := events.NewEmitter(pool)
	nodeSvc := node.NewService(pool, emitter)
	if err := nodeSvc.EnsureDefaultCluster(ctx); err != nil {
		return err
	}
	commandSvc := command.NewService(pool)
	schedulerSvc := scheduler.NewService(pool)

	// Dynamic tunables (ADR-023): catalog in code, overrides in the database, editable
	// in the Portal with hot-reload. Load seeds the table from legacy envs on the 1st boot.
	settingsStore := settings.New(pool)
	if err := settingsStore.Load(ctx); err != nil {
		return err
	}
	logger.Info("settings loaded")
	// The Health Score uses the SAME grace period as the NODE_STORAGE alert (#177/#180): without this the
	// Node would turn red in the Portal due to an I/O hiccup that the alert already ignores.
	nodeSvc.SetSettings(settingsStore)
	// Agent/Engine behavior configuration per Node (ADR-039): cluster value
	// (catalog) + override per Node, delivered in the heartbeat response.
	agentConfigSvc := agentconfig.New(pool, settingsStore)
	agentConfigSvc.SetEvents(emitter)
	// Orchestration activity trail (ADR-022): records decisions (hold/promotion,
	// scheduling) and lifecycle transitions in its own trail (feed).
	activityRec := activity.NewRecorder(pool)
	// Audit trail (ADR-025): operator actions on the API (via middleware) + login.
	auditRec := audit.NewRecorder(pool, logger)
	// Free disk margin (ADR-032), read from Store per call → hot-reload.
	diskMinFree := func() int64 { return settingsStore.Int64("admission.disk_min_free") }
	// Re-delivery of orphaned commands (#111): DISPATCHED without Agent confirmation returns
	// to the queue after the window; exceeded the limit → FAILED. Tunables with hot-reload.
	commandSvc.SetRedispatch(
		func() time.Duration { return settingsStore.Duration("commands.redispatch_after") },
		func() int { return settingsStore.Int("commands.max_attempts") },
	)
	torrentSvc := torrent.NewService(pool, schedulerSvc, commandSvc, emitter)
	torrentSvc.EnablePublish(cfg.MigrationPort) // publish reception on the Agent (ADR-019)
	torrentSvc.SetActivity(activityRec)
	torrentSvc.SetDiskMinFree(diskMinFree)
	// Decimation of time series samples (#113), with hot-reload.
	torrentSvc.SetSampleInterval(func() time.Duration { return settingsStore.Duration("metrics.torrent_sample_interval") })
	trackerSvc := tracker.NewService(pool)
	// Providers for the screenshot chain (ADR-040, #193): the tracker service
	// provides the key and the recorded host; the profile service provides the image_host section.
	profileSvc := trackerprofile.NewService(pool)
	torrentSvc.SetTrackerProfileProvider(profileSvc)
	torrentSvc.SetImageHostKeyProvider(trackerSvc)
	alertSvc := alert.NewService(pool)
	migrationSvc := migration.NewService(pool, commandSvc, emitter, cfg.MigrationPort)
	migrationSvc.SetDiskMinFree(diskMinFree)

	// Telemetry out of the Portal (ADR-028). The main connects domain → output
	// channel: alert/events/activity declare the interface they need and don't know
	// `notify`, so changing (or removing) the channel doesn't touch the domain. Everything is read from the
	// catalog on send → configuring in the Portal takes effect immediately, no redeploy.
	notifySvc := notify.New(settingsStore, logger)
	alertSvc.SetNotifier(notify.AlertHook{S: notifySvc})
	emitter.SetNotifier(notify.EventHook{S: notifySvc})
	activityRec.SetNotifier(notify.ActivityHook{S: notifySvc})

	// Heartbeat record for internal Workers: each loop marks its last execution
	// and the health API exposes it to the Portal (confidence that the loops are alive).
	workerReg := health.NewRegistry()

	// Alert Worker: evaluates deterministic rules (ADR-008) in the background. The tunables
	// (interval, offline-after, retention, disk %) come from the Store (ADR-023) — hot-reload.
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	alertWorker := alert.NewWorker(pool, alertSvc, logger, settingsStore)
	go alertWorker.Run(workerCtx, workerReg.Register("alert-worker", settingsStore.Duration("alerting.worker_interval")))
	// Download admission controller (#21): keeps ≤ L torrents downloading in the
	// cluster (L from the tracker msg or config fallback), putting the excess on hold.
	admissionWorker := admission.NewWorker(pool, trackerSvc, commandSvc, logger, settingsStore)
	admissionWorker.SetActivity(activityRec)
	go admissionWorker.Run(workerCtx, workerReg.Register("admission-worker", settingsStore.Duration("admission.interval")))
	// Migration reconciler (ADR-018): advances/finalizes in-flight torrent migrations.
	reconcilerInterval := func() time.Duration { return settingsStore.Duration("workers.reconciler_interval") }
	migrationMaxConcurrent := func() int { return settingsStore.Int("migration.max_concurrent") }
	// Watchdog for the destination recheck (#87): the reconciler watches the recheck, not the Agent anymore.
	migrationSvc.SetRecheckWatchdog(
		func() time.Duration { return settingsStore.Duration("migration.recheck_stall_window") },
		func() time.Duration { return settingsStore.Duration("migration.recheck_max_wait") })
	go migrationSvc.RunReconciler(workerCtx, reconcilerInterval, migrationMaxConcurrent, logger, workerReg.Register("migration-reconciler", reconcilerInterval()))
	// Auto-registration in Prometheus (ADR-017): keeps the targets file (file_sd) up to date.
	if cfg.PromTargetsFile != "" {
		promWriterInterval := func() time.Duration { return settingsStore.Duration("workers.prom_writer_interval") }
		go nodeSvc.RunPrometheusTargetsWriter(workerCtx, cfg.PromTargetsFile, promWriterInterval, logger, workerReg.Register("prometheus-targets", promWriterInterval()))
	}
	// Cluster statistics (F2): periodic snapshot of the accumulated throughput
	// (download/upload) in cluster_stat_samples, the basis for the evolution chart in Metrics.
	clusterStatsWorker := clusterstats.NewWorker(pool, logger, settingsStore)
	go clusterStatsWorker.Run(workerCtx, workerReg.Register("cluster-stats", settingsStore.Duration("metrics.cluster_sample_interval")))
	// Maintenance worker (#112): retention pruning of append-only tables (terminal
	// commands, events, activities, audit, expired sessions) — tunable in the Portal.
	maintenanceWorker := maintenance.NewWorker(pool, logger, settingsStore)
	go maintenanceWorker.Run(workerCtx, workerReg.Register("maintenance-worker", settingsStore.Duration("workers.maintenance_interval")))

	tmdbClient := tmdb.NewClient(func() string { return settingsStore.String("integrations.tmdb_api_key") })
	igdbClient := igdb.NewClient(
		func() string { return settingsStore.String("integrations.igdb_client_id") },
		func() string { return settingsStore.String("integrations.igdb_client_secret") },
	)
	// Google Books: OPTIONAL key (the API responds anonymously with an IP limit) — it is the
	// second book provider, for what Open Library does not index (#110).
	gbooksClient := googlebooks.NewClient(func() string { return settingsStore.String("integrations.google_books_api_key") })

	handler := api.NewRouter(api.Deps{
		Logger:     logger,
		Pool:       pool,
		Auth:       authSvc,
		Users:      usersSvc,
		Nodes:      nodeSvc,
		Commands:   commandSvc,
		Torrents:   torrentSvc,
		Trackers:   trackerSvc,
		Profiles:   profileSvc,
		Migrations: migrationSvc,
		Alerts:     alertSvc,
		Analytics:  analytics.NewService(pool),
		Events:     emitter,
		Activities: activityRec,
		Audit:      auditRec,
		Settings:   settingsStore,
		Metrics:    metrics.NewHandler(pool, notifySvc.Collectors()...),
		Prom:       promquery.New(cfg.PrometheusURL),
		// Integration keys come from the catalog (ADR-029), read per call → editable in the
		// Portal with hot-reload. Seeded from .env on the 1st boot via EnvVar (ADR-023).
		TMDB: tmdbClient,
		IGDB: igdbClient,
		// Explore catalog enrichment (#12): TMDB for movies/series, IGDB for games,
		// and for books/audiobooks a CASCADE — Open Library (open, no token) and, when
		// it doesn't confirm, Google Books (#110). 24h cache + limit of 8 simultaneous
		// goroutines — we don't block the browse page for a slow external API.
		Enrich:           enrich.New(tmdbClient, igdbClient, openlibrary.NewClient(), gbooksClient, 8),
		Workers:          workerReg,
		AgentRelease:     agentrelease.New(settingsStore),
		AgentConfig:      agentConfigSvc,
		Notifier:         notifySvc,
		Version:          version,
		IntegrationToken: cfg.IntegrationToken,
		StartedAt:        time.Now(),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("HTTP listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		logger.Info("shutting down", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
