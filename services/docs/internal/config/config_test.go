// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import "testing"

// clearDocsEnv limpia todas las variables que Load puede leer, para que los
// tests sean deterministas frente al entorno del CI.
func clearDocsEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SYTUE_DOCS_ENV", "SYTUE_DOCS_HTTP_ADDR", "SYTUE_DOCS_LOG_LEVEL",
		"SYTUE_DOCS_LOG_FORMAT", "SYTUE_DOCS_SHUTDOWN_TIMEOUT", "SYTUE_DOCS_VERSION",
		"SYTUE_DOCS_DATABASE_URL", "SYTUE_DOCS_OIDC_ISSUER", "SYTUE_DOCS_OIDC_JWKS_URL",
		"SYTUE_DOCS_OIDC_AUDIENCE", "SYTUE_DOCS_S3_ENDPOINT", "SYTUE_DOCS_S3_REGION",
		"SYTUE_DOCS_S3_ACCESS_KEY", "SYTUE_DOCS_S3_SECRET_KEY", "SYTUE_DOCS_S3_USE_SSL",
		"SYTUE_DOCS_S3_BUCKET", "SYTUE_DOCS_SNAPSHOT_INLINE_LIMIT",
		"SYTUE_DOCS_SECRET_ENC_KEY", "SYTUE_DOCS_DRAGONFLY_URL",
		"SYTUE_DOCS_INTEGRATIONS_URL",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://localhost/docs")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error inesperado: %v", err)
	}
	if cfg.Base.ServiceName != "sytue-docs" {
		t.Errorf("ServiceName = %q", cfg.Base.ServiceName)
	}
	if cfg.Base.HTTPAddr != ":8083" {
		t.Errorf("HTTPAddr default = %q, want :8083", cfg.Base.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://localhost/docs" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.OIDCIssuer != "https://id.sytue.eu" {
		t.Errorf("OIDCIssuer default = %q", cfg.OIDCIssuer)
	}
	if cfg.OIDCAudience != "sytue-docs" {
		t.Errorf("OIDCAudience default = %q", cfg.OIDCAudience)
	}
	if cfg.S3Region != "fr-par" {
		t.Errorf("S3Region default = %q", cfg.S3Region)
	}
	if cfg.S3Bucket != "docs-blobs" {
		t.Errorf("S3Bucket default = %q", cfg.S3Bucket)
	}
	if cfg.S3UseSSL {
		t.Error("S3UseSSL default debe ser false")
	}
	if cfg.SnapshotInlineLimit != 256*1024 {
		t.Errorf("SnapshotInlineLimit default = %d", cfg.SnapshotInlineLimit)
	}
	if cfg.IntegrationsURL != "http://integrations:8093" {
		t.Errorf("IntegrationsURL default = %q", cfg.IntegrationsURL)
	}
	if cfg.S3Configured() {
		t.Error("S3Configured debe ser false sin credenciales")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://db/docs")
	t.Setenv("SYTUE_DOCS_HTTP_ADDR", ":9000")
	t.Setenv("SYTUE_DOCS_OIDC_ISSUER", "https://id.example.com")
	t.Setenv("SYTUE_DOCS_OIDC_JWKS_URL", "https://id.example.com/jwks")
	t.Setenv("SYTUE_DOCS_S3_ENDPOINT", "https://s3.example.com")
	t.Setenv("SYTUE_DOCS_S3_ACCESS_KEY", "ak")
	t.Setenv("SYTUE_DOCS_S3_SECRET_KEY", "sk")
	t.Setenv("SYTUE_DOCS_S3_USE_SSL", "true")
	t.Setenv("SYTUE_DOCS_SNAPSHOT_INLINE_LIMIT", "1024")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error inesperado: %v", err)
	}
	if cfg.Base.HTTPAddr != ":9000" {
		t.Errorf("HTTPAddr override = %q, want :9000", cfg.Base.HTTPAddr)
	}
	if cfg.OIDCJWKSURL != "https://id.example.com/jwks" {
		t.Errorf("OIDCJWKSURL = %q", cfg.OIDCJWKSURL)
	}
	if !cfg.S3UseSSL {
		t.Error("S3UseSSL debe ser true")
	}
	if cfg.SnapshotInlineLimit != 1024 {
		t.Errorf("SnapshotInlineLimit = %d", cfg.SnapshotInlineLimit)
	}
	if !cfg.S3Configured() {
		t.Error("S3Configured debe ser true con endpoint+credenciales")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearDocsEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("Load() debe fallar sin DATABASE_URL")
	}
}

func TestLoadSecretEncKey(t *testing.T) {
	// Sin la variable: SecretEncKey nil (cifrado deshabilitado, arranque OK).
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://localhost/docs")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() sin clave: %v", err)
	}
	if cfg.SecretEncKey != nil {
		t.Errorf("SecretEncKey debe ser nil sin la variable, got %d bytes", len(cfg.SecretEncKey))
	}

	// Clave válida (base64 estándar de 32 bytes) → 32 bytes decodificados.
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://localhost/docs")
	t.Setenv("SYTUE_DOCS_SECRET_ENC_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEE=")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() con clave válida: %v", err)
	}
	if len(cfg.SecretEncKey) != 32 {
		t.Fatalf("SecretEncKey = %d bytes, want 32", len(cfg.SecretEncKey))
	}

	// Longitud incorrecta tras decodificar (16 bytes) → error de configuración.
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://localhost/docs")
	t.Setenv("SYTUE_DOCS_SECRET_ENC_KEY", "AAAAAAAAAAAAAAAAAAAAAA==") // 16 bytes
	if _, err := Load(); err == nil {
		t.Fatal("Load() debe fallar con clave de longitud != 32")
	}

	// Base64 inválido → error de configuración.
	clearDocsEnv(t)
	t.Setenv("SYTUE_DOCS_DATABASE_URL", "postgres://localhost/docs")
	t.Setenv("SYTUE_DOCS_SECRET_ENC_KEY", "no-es-base64-válido!!!")
	if _, err := Load(); err == nil {
		t.Fatal("Load() debe fallar con base64 inválido")
	}
}
