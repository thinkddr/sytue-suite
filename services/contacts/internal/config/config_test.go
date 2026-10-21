// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import "testing"

func setMinimalEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PUBLIC_URL", "DB_MAX_CONNS", "OIDC_ISSUER", "OIDC_JWKS_URL", "OIDC_AUDIENCE",
		"PORT", "HTTP_ADDR", "ENV", "VERSION",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/contacts")
}

func TestLoadDefaults(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/contacts" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.PublicURL != "https://contacts.sytue.localhost" {
		t.Errorf("PublicURL default = %q", cfg.PublicURL)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns default = %d", cfg.DBMaxConns)
	}
	if cfg.OIDCAudience != "sytue-contacts" {
		t.Errorf("OIDCAudience default = %q", cfg.OIDCAudience)
	}
	if want := "https://id.sytue.localhost/.well-known/jwks.json"; cfg.OIDCJWKSURL != want {
		t.Errorf("OIDCJWKSURL derived = %q, want %q", cfg.OIDCJWKSURL, want)
	}
	if cfg.HTTPAddr != ":8088" {
		t.Errorf("HTTPAddr default = %q, want :8088", cfg.HTTPAddr)
	}
	// Sin VERSION en el entorno se usa el default del paquete.
	if cfg.Version != Version {
		t.Errorf("Version = %q, want %q", cfg.Version, Version)
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
	t.Setenv("PUBLIC_URL", "https://c.example.com")
	t.Setenv("DB_MAX_CONNS", "7")
	t.Setenv("OIDC_ISSUER", "https://id.example.com")
	t.Setenv("OIDC_AUDIENCE", "aud-c")
	t.Setenv("PORT", "9001")
	t.Setenv("VERSION", "1.2.3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PublicURL != "https://c.example.com" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.DBMaxConns != 7 {
		t.Errorf("DBMaxConns = %d", cfg.DBMaxConns)
	}
	// JWKS derivado del issuer sobreescrito.
	if want := "https://id.example.com/.well-known/jwks.json"; cfg.OIDCJWKSURL != want {
		t.Errorf("OIDCJWKSURL = %q, want %q", cfg.OIDCJWKSURL, want)
	}
	if cfg.OIDCAudience != "aud-c" {
		t.Errorf("OIDCAudience = %q", cfg.OIDCAudience)
	}
	if cfg.HTTPAddr != ":9001" {
		t.Errorf("HTTPAddr = %q, want :9001", cfg.HTTPAddr)
	}
	if cfg.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", cfg.Version)
	}
}
