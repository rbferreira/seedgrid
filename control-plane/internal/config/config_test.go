package config

import "testing"

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
