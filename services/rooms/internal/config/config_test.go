// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"testing"
	"time"
)

func setMinimalEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PUBLIC_URL", "DB_MAX_CONNS", "OIDC_ISSUER", "OIDC_JWKS_URL", "OIDC_AUDIENCE",
		"DRIVE_URL", "INTEGRATIONS_URL", "INTEGRATIONS_INTERNAL_SECRET", "WS_ALLOWED_ORIGINS", "MAX_MESSAGE_KB", "MESSAGE_PAGE_SIZE",
		"PRESENCE_TTL", "PORT", "HTTP_ADDR", "ENV", "VERSION", "CHANNEL_MAIL_DOMAIN",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/chat")
}

func TestLoadDefaults(t *testing.T) {
	setMinimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/chat" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	// `rooms` y no `chat`: el host canónico cambió en la migración 0023 y el
	// legado se retiró el 2026-09-11. Solo afecta a desarrollo —en el clúster
	// PUBLIC_URL viene del chart—, pero un valor por defecto que apunta a un
	// nombre que ya no existe es una trampa para quien levante esto en local.
	if cfg.PublicURL != "https://rooms.sytue.localhost" {
		t.Errorf("PublicURL default = %q", cfg.PublicURL)
	}
	if cfg.DriveURL != "http://drive:8082" {
		t.Errorf("DriveURL default = %q", cfg.DriveURL)
	}
	if cfg.IntegrationsURL != "http://integrations:8093" || cfg.IntegrationsInternalSecret != "" {
		t.Fatalf("unexpected integrations defaults: %q, secret configured: %v", cfg.IntegrationsURL, cfg.IntegrationsInternalSecret != "")
	}
	if cfg.OIDCAudience != "sytue-chat" {
		t.Errorf("OIDCAudience default = %q", cfg.OIDCAudience)
	}
	// Orígenes por defecto cuando WS_ALLOWED_ORIGINS no está.
	if len(cfg.AllowedOrigins) < 10 || cfg.AllowedOrigins[0] != "sytue.eu" {
		t.Errorf("AllowedOrigins default = %v", cfg.AllowedOrigins)
	}
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*.sytue.eu" || origin == "*.sytue.localhost" {
			t.Fatalf("la allowlist WebSocket no debe confiar en publicaciones de usuarios: %v", cfg.AllowedOrigins)
		}
	}
	if cfg.MaxMessageBytes != 64*1024 {
		t.Errorf("MaxMessageBytes default = %d", cfg.MaxMessageBytes)
	}
	if cfg.MessagePageSize != 50 {
		t.Errorf("MessagePageSize default = %d", cfg.MessagePageSize)
	}
	if cfg.PresenceTTL != 45*time.Second {
		t.Errorf("PresenceTTL default = %v", cfg.PresenceTTL)
	}
	if cfg.HTTPAddr != ":8085" {
		t.Errorf("HTTPAddr default = %q, want :8085", cfg.HTTPAddr)
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
	t.Setenv("DRIVE_URL", "http://drive.local")
	t.Setenv("WS_ALLOWED_ORIGINS", "a.com, b.com ,")
	t.Setenv("MAX_MESSAGE_KB", "32")
	t.Setenv("MESSAGE_PAGE_SIZE", "100")
	t.Setenv("PRESENCE_TTL", "1m")
	t.Setenv("PORT", "9005")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DriveURL != "http://drive.local" {
		t.Errorf("DriveURL = %q", cfg.DriveURL)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[1] != "b.com" {
		t.Errorf("AllowedOrigins = %v (debe recortar espacios y vacíos)", cfg.AllowedOrigins)
	}
	if cfg.MaxMessageBytes != 32*1024 {
		t.Errorf("MaxMessageBytes = %d", cfg.MaxMessageBytes)
	}
	if cfg.MessagePageSize != 100 {
		t.Errorf("MessagePageSize = %d", cfg.MessagePageSize)
	}
	if cfg.PresenceTTL != time.Minute {
		t.Errorf("PresenceTTL = %v", cfg.PresenceTTL)
	}
	if cfg.HTTPAddr != ":9005" {
		t.Errorf("HTTPAddr = %q, want :9005", cfg.HTTPAddr)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	tests := map[string]Config{
		"MaxMessageBytes<=0":  {MaxMessageBytes: 0, MessagePageSize: 50},
		"MessagePageSize<=0":  {MaxMessageBytes: 1, MessagePageSize: 0},
		"MessagePageSize>200": {MaxMessageBytes: 1, MessagePageSize: 201},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			if err := c.validate(); err == nil {
				t.Fatal("validate debe fallar")
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	c := Config{MaxMessageBytes: 1024, MessagePageSize: 50}
	if err := c.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestLoadInvalidMessagePageSizeFails(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("MESSAGE_PAGE_SIZE", "500")
	if _, err := Load(); err == nil {
		t.Fatal("Load debe fallar con MESSAGE_PAGE_SIZE fuera de rango")
	}
}

// El dominio del correo a canal va APAGADO por defecto y se normaliza como en Mail.
func TestChannelMailDomain(t *testing.T) {
	setMinimalEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ChannelMailDomain != "" {
		t.Errorf("sin CHANNEL_MAIL_DOMAIN el correo a canal sale encendido: %q", cfg.ChannelMailDomain)
	}
	t.Setenv("CHANNEL_MAIL_DOMAIN", " @Rooms.Sytue.EU. ")
	if cfg, err = Load(); err != nil || cfg.ChannelMailDomain != "rooms.sytue.eu" {
		t.Errorf("ChannelMailDomain = %q, %v", cfg.ChannelMailDomain, err)
	}
}
