// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"testing"
	"time"
)

// clearMeetEnv limpia las variables que Load puede leer (prefijo vacío).
func clearMeetEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENV", "HTTP_ADDR", "LOG_LEVEL", "LOG_FORMAT", "SHUTDOWN_TIMEOUT", "VERSION",
		"PORT", "PUBLIC_URL", "DATABASE_URL", "DB_MAX_CONNS",
		"OIDC_ISSUER", "OIDC_JWKS_URL", "OIDC_AUDIENCE",
		"LIVEKIT_URL", "LIVEKIT_HOST", "LIVEKIT_API_KEY", "LIVEKIT_API_SECRET",
		"MEET_ACCESS_TOKEN_TTL", "MEET_ROOM_EMPTY_TIMEOUT", "MEET_MAX_PARTICIPANTS",
	} {
		t.Setenv(k, "")
	}
}

// setRequired fija las variables obligatorias con valores válidos.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/meet")
	t.Setenv("LIVEKIT_API_KEY", "key")
	t.Setenv("LIVEKIT_API_SECRET", "secret")
}

func TestLoadDefaults(t *testing.T) {
	clearMeetEnv(t)
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.ServiceName != ServiceName {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	// Sin PORT ni HTTP_ADDR: cae al puerto canónico de Meet.
	if cfg.HTTPAddr != ":8089" {
		t.Errorf("HTTPAddr default = %q, want :8089", cfg.HTTPAddr)
	}
	if cfg.AccessTokenTTL != 6*time.Hour {
		t.Errorf("AccessTokenTTL default = %v", cfg.AccessTokenTTL)
	}
	if cfg.MaxParticipants != 100 {
		t.Errorf("MaxParticipants default = %d", cfg.MaxParticipants)
	}
	// JWKS derivado del issuer cuando no se define.
	if cfg.OIDCJWKSURL != cfg.OIDCIssuer+"/.well-known/jwks.json" {
		t.Errorf("OIDCJWKSURL derivado = %q", cfg.OIDCJWKSURL)
	}
}

func TestLoadPortOverridesAddr(t *testing.T) {
	clearMeetEnv(t)
	setRequired(t)
	t.Setenv("PORT", "7000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.HTTPAddr != ":7000" {
		t.Errorf("PORT debe fijar HTTPAddr, got %q", cfg.HTTPAddr)
	}
}

func TestLoadRequiredMissing(t *testing.T) {
	clearMeetEnv(t)
	// Sin DATABASE_URL ni claves de LiveKit: Load debe fallar.
	if _, err := Load(); err == nil {
		t.Fatal("Load() debe fallar sin variables obligatorias")
	}
}

func TestValidate(t *testing.T) {
	clearMeetEnv(t)
	setRequired(t)
	t.Setenv("MEET_MAX_PARTICIPANTS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("MEET_MAX_PARTICIPANTS=0 debe fallar la validación")
	}
}
