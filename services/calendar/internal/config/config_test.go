// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"testing"
	"time"
)

// setMinimalEnv fija las variables mínimas para que Load() tenga éxito y limpia
// las que podrían filtrarse del entorno del test runner.
func setMinimalEnv(t *testing.T) {
	t.Helper()
	// Aísla de cualquier valor heredado del entorno del CI/dev.
	for _, k := range []string{
		"PUBLIC_URL", "DB_MAX_CONNS", "OIDC_ISSUER", "OIDC_JWKS_URL", "OIDC_AUDIENCE",
		"CALDAV_PREFIX", "MAX_FREEBUSY_WINDOW", "MAX_RANGE_WINDOW", "MAX_OBJECT_KB",
		"PORT", "HTTP_ADDR", "ENV",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/cal")
}

func TestLoadDefaults(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DatabaseURL != "postgres://localhost/cal" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.PublicURL != "https://calendar.sytue.localhost" {
		t.Errorf("PublicURL default = %q", cfg.PublicURL)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns default = %d", cfg.DBMaxConns)
	}
	if cfg.OIDCIssuer != "https://id.sytue.localhost" {
		t.Errorf("OIDCIssuer default = %q", cfg.OIDCIssuer)
	}
	// JWKS derivado del issuer cuando no se aporta.
	if want := "https://id.sytue.localhost/.well-known/jwks.json"; cfg.OIDCJWKSURL != want {
		t.Errorf("OIDCJWKSURL derived = %q, want %q", cfg.OIDCJWKSURL, want)
	}
	if cfg.OIDCAudience != "sytue-calendar" {
		t.Errorf("OIDCAudience default = %q", cfg.OIDCAudience)
	}
	if cfg.CalDAVPrefix != "/dav" {
		t.Errorf("CalDAVPrefix default = %q", cfg.CalDAVPrefix)
	}
	if cfg.MaxFreeBusyWindow != 366*24*time.Hour {
		t.Errorf("MaxFreeBusyWindow default = %v", cfg.MaxFreeBusyWindow)
	}
	if cfg.MaxRangeWindow != 366*24*time.Hour {
		t.Errorf("MaxRangeWindow default = %v", cfg.MaxRangeWindow)
	}
	if cfg.MaxObjectBytes != 1024*1024 {
		t.Errorf("MaxObjectBytes default = %d", cfg.MaxObjectBytes)
	}
	// Sin PORT ni HTTP_ADDR válido => puerto por defecto del servicio.
	if cfg.HTTPAddr != ":"+DefaultPort {
		t.Errorf("HTTPAddr default = %q, want :%s", cfg.HTTPAddr, DefaultPort)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load debe fallar sin DATABASE_URL")
	}
}

func TestLoadOverrides(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("PUBLIC_URL", "https://cal.example.com")
	t.Setenv("DB_MAX_CONNS", "42")
	t.Setenv("OIDC_ISSUER", "https://id.example.com")
	t.Setenv("OIDC_JWKS_URL", "https://id.example.com/jwks")
	t.Setenv("OIDC_AUDIENCE", "aud-x")
	t.Setenv("CALDAV_PREFIX", "/caldav")
	t.Setenv("MAX_FREEBUSY_WINDOW", "48h")
	t.Setenv("MAX_RANGE_WINDOW", "24h")
	t.Setenv("MAX_OBJECT_KB", "2048")
	t.Setenv("PORT", "9099")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PublicURL != "https://cal.example.com" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.DBMaxConns != 42 {
		t.Errorf("DBMaxConns = %d", cfg.DBMaxConns)
	}
	// JWKS explícito NO se deriva del issuer.
	if cfg.OIDCJWKSURL != "https://id.example.com/jwks" {
		t.Errorf("OIDCJWKSURL = %q", cfg.OIDCJWKSURL)
	}
	if cfg.OIDCAudience != "aud-x" {
		t.Errorf("OIDCAudience = %q", cfg.OIDCAudience)
	}
	if cfg.CalDAVPrefix != "/caldav" {
		t.Errorf("CalDAVPrefix = %q", cfg.CalDAVPrefix)
	}
	if cfg.MaxFreeBusyWindow != 48*time.Hour {
		t.Errorf("MaxFreeBusyWindow = %v", cfg.MaxFreeBusyWindow)
	}
	if cfg.MaxRangeWindow != 24*time.Hour {
		t.Errorf("MaxRangeWindow = %v", cfg.MaxRangeWindow)
	}
	if cfg.MaxObjectBytes != 2048*1024 {
		t.Errorf("MaxObjectBytes = %d", cfg.MaxObjectBytes)
	}
	// PORT tiene prioridad sobre HTTP_ADDR.
	if cfg.HTTPAddr != ":9099" {
		t.Errorf("HTTPAddr = %q, want :9099", cfg.HTTPAddr)
	}
}

func TestValidateRejectsNonPositiveWindows(t *testing.T) {
	tests := map[string]Config{
		"MaxObjectBytes<=0": {
			MaxObjectBytes: 0, MaxFreeBusyWindow: time.Hour, MaxRangeWindow: time.Hour,
		},
		"MaxFreeBusyWindow<=0": {
			MaxObjectBytes: 1, MaxFreeBusyWindow: 0, MaxRangeWindow: time.Hour,
		},
		"MaxRangeWindow<=0": {
			MaxObjectBytes: 1, MaxFreeBusyWindow: time.Hour, MaxRangeWindow: 0,
		},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			if err := c.validate(); err == nil {
				t.Fatal("validate debe fallar")
			}
		})
	}
}

func TestValidateAcceptsPositive(t *testing.T) {
	c := Config{MaxObjectBytes: 1, MaxFreeBusyWindow: time.Hour, MaxRangeWindow: time.Hour}
	if err := c.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
