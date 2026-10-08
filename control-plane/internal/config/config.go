// Package config loads the Control Plane configuration from environment
// variables. Configuration is always external to the code (Coding Standards §12).
//
// Only INFRA and SECRET live here — immutable at runtime (read once at boot).
// BEHAVIOR tunables (worker intervals, admission/alert thresholds,
// retention) migrated to the dynamic catalog (internal/settings, ADR-023): they are
// editable via the Portal UI with hot-reload; the corresponding env var only
// SEEDS the database on the 1st boot (see settings.Store.Load).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config gathers the infra/secret parameters of the Control Plane (boot-time).
type Config struct {
	DatabaseURL            string
	HTTPAddr               string
	LogLevel               string
	AgentRegistrationToken string
	// IntegrationToken protects internal webhooks (/integrations/*, #116).
	// Optional: empty leaves the endpoints open (compat) with a WARNING in the log.
	IntegrationToken string
	AdminUsername    string
	AdminPassword    string
	PrometheusURL    string
	PromTargetsFile  string
	MigrationPort    int
	// Third-party integration keys (TMDB, IGDB, ptpimg) DO NOT live here since
	// ADR-029: they live in the tunables catalog (settings), editable in the Portal. The seed
	// from the same SEEDGRID_*_* on the 1st boot is done by the Store itself (EnvVar of the
	// catalog), not by this config — which is why they are no longer read here.
}

// Load reads the configuration from the environment and validates the required fields.
func Load() (Config, error) {
	c := Config{
		DatabaseURL:            os.Getenv("SEEDGRID_DATABASE_URL"),
		HTTPAddr:               getenv("SEEDGRID_HTTP_ADDR", ":8080"),
		LogLevel:               getenv("SEEDGRID_LOG_LEVEL", "info"),
		AgentRegistrationToken: os.Getenv("SEEDGRID_AGENT_REGISTRATION_TOKEN"),
		IntegrationToken:       os.Getenv("SEEDGRID_INTEGRATION_TOKEN"),
		AdminUsername:          getenv("SEEDGRID_ADMIN_USERNAME", "admin"),
		AdminPassword:          os.Getenv("SEEDGRID_ADMIN_PASSWORD"),
		PrometheusURL:          getenv("SEEDGRID_PROMETHEUS_URL", "http://seedgrid-prometheus:9090"),
		PromTargetsFile:        getenv("SEEDGRID_PROM_TARGETS_FILE", ""),
		MigrationPort:          getenvInt("SEEDGRID_MIGRATION_PORT", 6900),
	}

	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "SEEDGRID_DATABASE_URL")
	}
	if c.AgentRegistrationToken == "" {
		missing = append(missing, "SEEDGRID_AGENT_REGISTRATION_TOKEN")
	}
	if c.AdminPassword == "" {
		missing = append(missing, "SEEDGRID_ADMIN_PASSWORD")
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvInt reads an integer; missing or invalid value uses def.
func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
