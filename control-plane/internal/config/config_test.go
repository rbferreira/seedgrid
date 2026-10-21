package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("SEEDGRID_DATABASE_URL", "postgres://localhost/seedgrid")
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "tok")
	t.Setenv("SEEDGRID_ADMIN_PASSWORD", "pw")
	// Empty optionals should fall back to defaults.
	t.Setenv("SEEDGRID_HTTP_ADDR", "")
	t.Setenv("SEEDGRID_ADMIN_USERNAME", "")
	t.Setenv("SEEDGRID_LOG_LEVEL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want :8080", c.HTTPAddr)
	}
	if c.AdminUsername != "admin" {
		t.Errorf("AdminUsername default = %q, want admin", c.AdminUsername)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", c.LogLevel)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("SEEDGRID_DATABASE_URL", "")
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "")
	t.Setenv("SEEDGRID_ADMIN_PASSWORD", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing required variables")
	}
}

// writeSecret writes a secret file the way an operator or the Compose init service would.
func writeSecret(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_SecretsFromFiles(t *testing.T) {
	t.Setenv("SEEDGRID_DATABASE_URL", "")
	t.Setenv("SEEDGRID_DATABASE_URL_FILE", writeSecret(t, "postgres://db/seedgrid\n"))
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "")
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN_FILE", writeSecret(t, "tok\r\n"))
	t.Setenv("SEEDGRID_INTEGRATION_TOKEN", "")
	t.Setenv("SEEDGRID_INTEGRATION_TOKEN_FILE", writeSecret(t, "hook"))
	t.Setenv("SEEDGRID_ADMIN_PASSWORD", "")
	t.Setenv("SEEDGRID_ADMIN_PASSWORD_FILE", writeSecret(t, " pw with spaces \n"))

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.DatabaseURL != "postgres://db/seedgrid" || c.AgentRegistrationToken != "tok" || c.IntegrationToken != "hook" {
		t.Errorf("secrets = %q, %q, %q", c.DatabaseURL, c.AgentRegistrationToken, c.IntegrationToken)
	}
	// Only the line ending goes: spaces are part of a password the operator chose.
	if c.AdminPassword != " pw with spaces " {
		t.Errorf("AdminPassword = %q", c.AdminPassword)
	}
}

func TestLoad_VariableWinsOverFile(t *testing.T) {
	t.Setenv("SEEDGRID_DATABASE_URL", "postgres://env/seedgrid")
	t.Setenv("SEEDGRID_DATABASE_URL_FILE", writeSecret(t, "postgres://file/seedgrid"))
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "tok")
	t.Setenv("SEEDGRID_ADMIN_PASSWORD", "pw")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.DatabaseURL != "postgres://env/seedgrid" {
		t.Errorf("DatabaseURL = %q, want the variable's value", c.DatabaseURL)
	}
}

func TestLoad_UnreadableFileIsAnError(t *testing.T) {
	t.Setenv("SEEDGRID_DATABASE_URL", "postgres://localhost/seedgrid")
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "tok")
	t.Setenv("SEEDGRID_ADMIN_PASSWORD", "pw")
	t.Setenv("SEEDGRID_INTEGRATION_TOKEN", "")
	t.Setenv("SEEDGRID_INTEGRATION_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SEEDGRID_INTEGRATION_TOKEN_FILE") {
		t.Fatalf("err = %v, want one naming SEEDGRID_INTEGRATION_TOKEN_FILE", err)
	}
}
