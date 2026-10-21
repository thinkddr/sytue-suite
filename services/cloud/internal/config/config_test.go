// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// setRequiredEnv fija las variables obligatorias mínimas para que Load no falle.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/cloud")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")
	t.Setenv("SECRET_ENC_KEY", "0000000000000000000000000000000000000000000=")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":"+DefaultAPIPort {
		t.Fatalf("HTTPAddr=%q, se esperaba :%s", cfg.HTTPAddr, DefaultAPIPort)
	}
	if cfg.AppsDomain != "apps.sytue.localhost" {
		t.Fatalf("AppsDomain=%q", cfg.AppsDomain)
	}
	// `sytue-cloud`, no `sytue-cloud-api`: es el `resource_audience` que Sytue ID
	// pone de verdad en el `aud` del access token. Exigir el otro dejaba el
	// control-plane en «modo demostración» con todo lo demás sano.
	if cfg.OIDCAudience != "sytue-cloud" {
		t.Fatalf("OIDCAudience=%q", cfg.OIDCAudience)
	}
	// JWKS URL se deriva del issuer cuando no se define.
	if cfg.OIDCJWKSURL != cfg.OIDCIssuer+"/.well-known/jwks.json" {
		t.Fatalf("OIDCJWKSURL=%q", cfg.OIDCJWKSURL)
	}
	if cfg.MaxProjectsPerOrg != 100 || cfg.DBMaxConns != 10 {
		t.Fatalf("defaults numéricos: %+v", cfg)
	}
	// CIDR del clúster: defaults de Scaleway Kapsule (pods + services).
	if len(cfg.ClusterCIDRs) != 2 || cfg.ClusterCIDRs[0] != "100.64.0.0/14" || cfg.ClusterCIDRs[1] != "10.32.0.0/12" {
		t.Fatalf("ClusterCIDRs=%v, se esperaban los defaults de Kapsule", cfg.ClusterCIDRs)
	}
}

func TestLoadInvalidClusterCIDRs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CLUSTER_CIDRS", "10.0.0.0/8,no-es-un-cidr")
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("CLUSTER_CIDRS inválido debía fallar la validación")
	}
}

func TestLoadPortOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "9999")
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Fatalf("PORT no aplicado: %q", cfg.HTTPAddr)
	}
}

func TestLoadExplicitJWKS(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OIDC_JWKS_URL", "https://keys.example/jwks")
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OIDCJWKSURL != "https://keys.example/jwks" {
		t.Fatalf("JWKS explícito ignorado: %q", cfg.OIDCJWKSURL)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	// Solo se fija DATABASE_URL; faltan las claves S3/enc.
	t.Setenv("DATABASE_URL", "postgres://x/y")
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("Load debía fallar por variables obligatorias ausentes")
	}
}

func TestLoadGitHubApp(t *testing.T) {
	setRequiredEnv(t)
	// Sin configurar: campos vacíos (integración deshabilitada).
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitHubAppID != 0 || cfg.GitHubAppPrivateKey != "" {
		t.Fatalf("GitHub App debe estar vacía por defecto: id=%d key=%q", cfg.GitHubAppID, cfg.GitHubAppPrivateKey)
	}
	// Configurada: App ID (int64) + clave PEM.
	t.Setenv("GITHUB_APP_ID", "424242")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----")
	cfg, err = Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitHubAppID != 424242 || cfg.GitHubAppPrivateKey == "" {
		t.Fatalf("GitHub App no cargada: id=%d keyLen=%d", cfg.GitHubAppID, len(cfg.GitHubAppPrivateKey))
	}
}

func TestLoadInvalidGitHubAppID(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GITHUB_APP_ID", "no-es-entero")
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("GITHUB_APP_ID no numérico debía fallar la validación")
	}
}

// TestLoadCNBDefaults verifica que las imágenes de CNB tienen defaults pinneados
// (no :latest) y que la run image es opcional (vacía por defecto).
func TestLoadCNBDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CNBBuilderImage == "" || hasLatest(cfg.CNBBuilderImage) {
		t.Errorf("CNB_BUILDER_IMAGE debe estar pinneada (no :latest): %q", cfg.CNBBuilderImage)
	}
	if cfg.CNBGitImage == "" || hasLatest(cfg.CNBGitImage) {
		t.Errorf("CNB_GIT_IMAGE debe estar pinneada (no :latest): %q", cfg.CNBGitImage)
	}
	if cfg.CNBRunImage != "" {
		t.Errorf("CNB_RUN_IMAGE debe ser opcional (vacía por defecto): %q", cfg.CNBRunImage)
	}
}

func hasLatest(image string) bool {
	return len(image) >= 7 && image[len(image)-7:] == ":latest"
}

func TestLoadInvalidBuildTimeout(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BUILD_TIMEOUT", "0s")
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("BUILD_TIMEOUT=0 debía fallar la validación")
	}
}

func TestLoadInvalidMaxProjects(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAX_PROJECTS_PER_ORG", "0")
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("MAX_PROJECTS_PER_ORG=0 debía fallar la validación")
	}
}

func TestLoadProdRequiresPrivateRegistryCredentials(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("HUB_INTERNAL_SECRET", "de-prueba") // fuera de dev es obligatorio
	if _, err := Load(DefaultAPIPort); err == nil {
		t.Fatal("producción debe fallar cerrada sin credenciales del registry privado")
	}
	t.Setenv("REGISTRY_USERNAME", "nologin")
	t.Setenv("REGISTRY_PASSWORD", "read-only-key")
	t.Setenv("WEBHOOK_SECRET", "hmac-de-webhooks")
	if _, err := Load(DefaultAPIPort); err != nil {
		t.Fatalf("credenciales privadas válidas rechazadas: %v", err)
	}
}

// El endpoint de webhooks git es público y sin autenticar: sin secreto, la firma
// no se puede exigir y un POST fabricado dispara un build y un despliegue. En
// producción no puede faltar, y falla al ARRANCAR y no en la primera entrega.
//
// La exigencia es de QUIEN SIRVE esa ruta, no del paquete: el builder y el
// controller comparten esta configuración, no atienden webhooks, y exigírselo
// los dejó en CrashLoopBackOff en el despliegue de v0.1.16.
func TestRequireWebhookSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("HUB_INTERNAL_SECRET", "de-prueba") // fuera de dev es obligatorio
	t.Setenv("REGISTRY_USERNAME", "nologin")
	t.Setenv("REGISTRY_PASSWORD", "read-only-key")

	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("sin WEBHOOK_SECRET la carga debe funcionar: la usan tres binarios: %v", err)
	}
	if err := cfg.RequireWebhookSecret(); err == nil {
		t.Fatal("quien sirve webhooks debe fallar cerrado sin WEBHOOK_SECRET")
	}

	t.Setenv("WEBHOOK_SECRET", "hmac-de-webhooks")
	cfg, err = Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("configuración completa rechazada: %v", err)
	}
	if err := cfg.RequireWebhookSecret(); err != nil {
		t.Fatalf("con secreto no debe protestar: %v", err)
	}
}

// Fuera de producción no se exige: en local nadie monta un HMAC para levantar
// la suite, y el endpoint no está expuesto a Internet.
func TestRequireWebhookSecretSoloEnProduccion(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENV", "dev")
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("carga de dev rechazada: %v", err)
	}
	if err := cfg.RequireWebhookSecret(); err != nil {
		t.Fatalf("dev no debe exigir WEBHOOK_SECRET: %v", err)
	}
}

func TestLoadNormalizesS3FromURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("S3_ENDPOINT", "https://s3.example:443")
	cfg, err := Load(DefaultAPIPort)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3Endpoint != "s3.example:443" || !cfg.S3UseSSL {
		t.Fatalf("S3 endpoint no normalizado: %q ssl=%v", cfg.S3Endpoint, cfg.S3UseSSL)
	}
}

func TestUseKubeRunner(t *testing.T) {
	cases := []struct {
		name string
		mode string
		env  pcfg.Environment
		want bool
	}{
		{"explicit kube", "kube", pcfg.EnvDev, true},
		{"explicit prod alias", "prod", pcfg.EnvDev, true},
		{"explicit stub", "stub", pcfg.EnvProd, false},
		{"explicit dev alias", "dev", pcfg.EnvProd, false},
		{"default in prod", "", pcfg.EnvProd, true},
		{"default in dev", "", pcfg.EnvDev, false},
		{"default in staging", "", pcfg.EnvStaging, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Config{RunnerMode: tc.mode}
			c.Env = tc.env
			if got := c.UseKubeRunner(); got != tc.want {
				t.Fatalf("UseKubeRunner()=%v, se esperaba %v", got, tc.want)
			}
		})
	}
}
