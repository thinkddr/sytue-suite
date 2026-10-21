// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del cargador de configuración de Sytue Drive: defaults, derivaciones
// (PORT/HTTP_ADDR, JWKS, endpoint S3), validación y errores. Deterministas, sin
// BD ni red (solo lectura de variables de entorno con t.Setenv).
package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/drive")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServiceName != ServiceName {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.PublicURL != "https://drive.sytue.localhost" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.MaxUploadBytes != 5120*1024*1024 {
		t.Errorf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.PresignTTL != 15*time.Minute {
		t.Errorf("PresignTTL = %v", cfg.PresignTTL)
	}
	if cfg.DefaultQuotaBytes != int64(10)*1024*1024*1024 {
		t.Errorf("DefaultQuotaBytes = %d", cfg.DefaultQuotaBytes)
	}
	if cfg.OIDCAudience != "sytue-drive" {
		t.Errorf("OIDCAudience = %q", cfg.OIDCAudience)
	}
	if cfg.OIDCJWKSURL != "https://id.sytue.localhost/.well-known/jwks.json" {
		t.Errorf("OIDCJWKSURL derivado = %q", cfg.OIDCJWKSURL)
	}
	if cfg.HTTPAddr != ":8082" {
		t.Errorf("HTTPAddr = %q, quiero :8082", cfg.HTTPAddr)
	}
}

func TestLoadVersionFromLinker(t *testing.T) {
	setRequired(t)
	// Sin VERSION en el entorno se usa la variable de paquete Version.
	old := Version
	Version = "1.2.3"
	defer func() { Version = old }()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "1.2.3" {
		t.Fatalf("Version = %q, quiero la inyectada por el linker", cfg.Version)
	}
}

func TestLoadVersionEnvOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("VERSION", "env-ver")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "env-ver" {
		t.Fatalf("VERSION del entorno tiene prioridad, got %q", cfg.Version)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("Load sin variables obligatorias debe fallar")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error debe mencionar DATABASE_URL: %v", err)
	}
}

func TestLoadPortOverridesHTTPAddr(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "9090")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("PORT debe fijar HTTPAddr, got %q", cfg.HTTPAddr)
	}
}

func TestLoadS3EndpointNormalization(t *testing.T) {
	setRequired(t)
	t.Setenv("S3_ENDPOINT", "http://minio:9000")
	t.Setenv("S3_USE_SSL", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "minio:9000" || cfg.S3UseSSL {
		t.Fatalf("esquema http => %q ssl=%v", cfg.S3Endpoint, cfg.S3UseSSL)
	}
}

func TestLoadInvalidUpload(t *testing.T) {
	setRequired(t)
	t.Setenv("MAX_UPLOAD_MB", "0")
	if _, err := Load(); err == nil {
		t.Fatal("MAX_UPLOAD_MB=0 debe fallar la validación")
	}
}

func TestLoadInvalidInteger(t *testing.T) {
	setRequired(t)
	t.Setenv("DEFAULT_QUOTA_GB", "no-numero")
	if _, err := Load(); err == nil {
		t.Fatal("entero inválido debe reportar error")
	}
}

func TestValidate(t *testing.T) {
	valid := Config{MaxUploadBytes: 1, DefaultQuotaBytes: 0}
	if err := valid.validate(); err != nil {
		t.Fatalf("config válida rechazada: %v", err)
	}
	if err := (Config{MaxUploadBytes: 0}).validate(); err == nil {
		t.Error("MaxUploadBytes=0 debe fallar")
	}
	if err := (Config{MaxUploadBytes: 1, DefaultQuotaBytes: -1}).validate(); err == nil {
		t.Error("DefaultQuotaBytes negativo debe fallar")
	}
}

// El secreto de la puerta de Cast es OPCIONAL y compartido: en dev cae al de
// desarrollo (los dos extremos casan sin configurar nada); fuera de dev, vacío se
// queda vacío y el de desarrollo —que está en el código— se rechaza.
func TestCastInternalSecret(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CastInternalSecret != DevCastInternalSecret {
		t.Fatalf("en dev sin variable = %q, quiero el de desarrollo", cfg.CastInternalSecret)
	}

	t.Setenv("ENV", "prod")
	// El del Hub sí es obligatorio fuera de dev; aquí se prueba el de Cast.
	t.Setenv("HUB_INTERNAL_SECRET", "hub-de-verdad")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("en prod sin CAST_INTERNAL_SECRET debe arrancar (la puerta queda cerrada): %v", err)
	}
	if cfg.CastInternalSecret != "" {
		t.Fatalf("en prod sin variable = %q, quiero vacío", cfg.CastInternalSecret)
	}

	t.Setenv("CAST_INTERNAL_SECRET", DevCastInternalSecret)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CAST_INTERNAL_SECRET") {
		t.Fatalf("en prod con el secreto de desarrollo debe fallar nombrándolo: %v", err)
	}

	t.Setenv("CAST_INTERNAL_SECRET", "de-verdad")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CastInternalSecret != "de-verdad" {
		t.Fatalf("en prod = %q", cfg.CastInternalSecret)
	}
}

// Los dos extremos del secreto de desarrollo tienen que ser el MISMO literal: Drive
// no puede importar services/cast/internal/config, así que se lee el fichero. Si Cast
// cambia el suyo, en dev Cast declararía su uso y Drive contestaría 401 sin que nada
// más lo dijera.
func TestDevCastInternalSecretEsElDeCast(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "cast", "internal", "config", "config.go"))
	if err != nil {
		t.Fatalf("leer la config de Cast: %v", err)
	}
	m := regexp.MustCompile(`DevInternalSecret\s*=\s*"([^"]+)"`).FindSubmatch(src)
	if m == nil {
		t.Fatal("services/cast/internal/config/config.go ya no declara DevInternalSecret: actualiza este test y DevCastInternalSecret")
	}
	if got := string(m[1]); got != DevCastInternalSecret {
		t.Fatalf("Cast usa %q y Drive %q: en dev la declaración de cuota daría 401", got, DevCastInternalSecret)
	}
}
