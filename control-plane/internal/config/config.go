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

// secretVars are the variables that also accept the `_FILE` form (ADR-042): the value is read
// from the file the variable names, the Docker convention for secrets kept out of the environment.
var secretVars = []string{
	"SEEDGRID_DATABASE_URL",
	"SEEDGRID_AGENT_REGISTRATION_TOKEN",
	"SEEDGRID_INTEGRATION_TOKEN",
	"SEEDGRID_ADMIN_PASSWORD",
}

// Load reads the configuration from the environment and validates the required fields.
func Load() (Config, error) {
	secrets := make(map[string]string, len(secretVars))
	for _, key := range secretVars {
		v, err := getsecret(key)
		if err != nil {
			return Config{}, err
		}
		secrets[key] = v
	}

	c := Config{
		DatabaseURL:            secrets["SEEDGRID_DATABASE_URL"],
		HTTPAddr:               getenv("SEEDGRID_HTTP_ADDR", ":8080"),
		LogLevel:               getenv("SEEDGRID_LOG_LEVEL", "info"),
		AgentRegistrationToken: secrets["SEEDGRID_AGENT_REGISTRATION_TOKEN"],
		IntegrationToken:       secrets["SEEDGRID_INTEGRATION_TOKEN"],
		AdminUsername:          getenv("SEEDGRID_ADMIN_USERNAME", "admin"),
		AdminPassword:          secrets["SEEDGRID_ADMIN_PASSWORD"],
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

// getsecret reads a secret: KEY wins when set, otherwise the file named by KEY_FILE. The trailing
// newline an editor or `echo` leaves at the end of the file is not part of the value. A KEY_FILE
// that cannot be read is an error, not an empty value: the operator pointed at a file, and
// starting without it (an open webhook, a missing password) would hide the mistake.
func getsecret(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	path := os.Getenv(key + "_FILE")
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: the path comes from the operator's own configuration
	if err != nil {
		return "", fmt.Errorf("%s_FILE: %w", key, err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
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
