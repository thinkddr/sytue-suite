// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del cargador de configuración de Sytue Mail: defaults, derivaciones
// (PORT/HTTP_ADDR, JWKS, endpoint S3), validación y errores. Deterministas,
// sin BD ni red (solo lectura de variables de entorno con t.Setenv).
package config

import (
	"strings"
	"testing"
	"time"
)

// setRequired fija las variables obligatorias para que Load no falle por su
// ausencia; cada test ajusta el resto con t.Setenv según lo que quiera probar.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/mail")
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
	if cfg.PublicURL != "https://mail.sytue.localhost" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.PrimaryDomain != "sytue.localhost" {
		t.Errorf("PrimaryDomain = %q", cfg.PrimaryDomain)
	}
	if cfg.Hostname != "mx.sytue.localhost" {
		t.Errorf("Hostname = %q", cfg.Hostname)
	}
	// 50 MB: lo que cabe en la memoria del pod. Si este número sube, tiene que subir
	// con él la memoria del chart (COM-06).
	if cfg.MaxMessageBytes != 50*1024*1024 {
		t.Errorf("MaxMessageBytes = %d", cfg.MaxMessageBytes)
	}
	// La cuota de buzón era una constante de 10 GiB en `delivery` y no había
	// forma de bajarla sin recompilar. Sigue siendo 10 GiB por defecto —quien
	// no configure nada se comporta igual que antes— pero ahora se puede fijar.
	if cfg.DefaultMailboxQuotaBytes != 10*1024*1024*1024 {
		t.Errorf("DefaultMailboxQuotaBytes = %d", cfg.DefaultMailboxQuotaBytes)
	}
	if cfg.MaxRecipients != 100 {
		t.Errorf("MaxRecipients = %d", cfg.MaxRecipients)
	}
	if cfg.QueueWorkers != 4 || cfg.QueueMaxAttempts != 8 {
		t.Errorf("cola por defecto = %d/%d", cfg.QueueWorkers, cfg.QueueMaxAttempts)
	}
	if cfg.QueueBaseBackoff != 30*time.Second {
		t.Errorf("QueueBaseBackoff = %v", cfg.QueueBaseBackoff)
	}
	// JWKS derivado del issuer cuando OIDC_JWKS_URL está vacío.
	if cfg.OIDCJWKSURL != "https://id.sytue.localhost/.well-known/jwks.json" {
		t.Errorf("OIDCJWKSURL derivado = %q", cfg.OIDCJWKSURL)
	}
	// HTTP_ADDR por defecto de mail (:8086) cuando no hay PORT.
	if cfg.HTTPAddr != ":8086" {
		t.Errorf("HTTPAddr = %q, quiero :8086", cfg.HTTPAddr)
	}
	// Anti-abuso: defaults activos.
	if !cfg.VerifySPF || !cfg.VerifyDKIM || !cfg.EnforceDMARC {
		t.Errorf("verificaciones por defecto = %v/%v/%v", cfg.VerifySPF, cfg.VerifyDKIM, cfg.EnforceDMARC)
	}
	if cfg.RejectOnDMARCFail {
		t.Error("RejectOnDMARCFail por defecto debe ser false")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	// Sin DATABASE_URL/S3_* debe fallar con errores acumulados.
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
	t.Setenv("PORT", "9099")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9099" {
		t.Fatalf("PORT debe fijar HTTPAddr, got %q", cfg.HTTPAddr)
	}
}

func TestLoadExplicitJWKS(t *testing.T) {
	setRequired(t)
	t.Setenv("OIDC_JWKS_URL", "https://jwks.example/keys")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OIDCJWKSURL != "https://jwks.example/keys" {
		t.Fatalf("OIDC_JWKS_URL explícito no respetado: %q", cfg.OIDCJWKSURL)
	}
}

func TestLoadS3EndpointNormalization(t *testing.T) {
	setRequired(t)
	t.Setenv("S3_ENDPOINT", "https://s3.example:443")
	t.Setenv("S3_USE_SSL", "false") // el esquema https tiene prioridad.
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "s3.example:443" {
		t.Fatalf("endpoint sin esquema = %q", cfg.S3Endpoint)
	}
	if !cfg.S3UseSSL {
		t.Fatal("esquema https debe activar S3UseSSL")
	}
}

func TestLoadS3EndpointHTTPScheme(t *testing.T) {
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

func TestLoadInvalidMaxMessage(t *testing.T) {
	setRequired(t)
	t.Setenv("MAX_MESSAGE_MB", "0")
	if _, err := Load(); err == nil {
		t.Fatal("MAX_MESSAGE_MB=0 debe fallar la validación")
	}
}

func TestLoadInvalidInteger(t *testing.T) {
	setRequired(t)
	t.Setenv("MAX_RECIPIENTS", "no-numero")
	if _, err := Load(); err == nil {
		t.Fatal("entero inválido debe reportar error")
	}
}

func TestValidate(t *testing.T) {
	base := func() Config {
		return Config{
			PublicURL:                "https://mail",
			Hostname:                 "mx",
			MTASTSMode:               "testing",
			MaxMessageBytes:          1,
			DefaultMailboxQuotaBytes: 1,
			MaxRecipients:            1,
			QueueWorkers:             1,
			QueueMaxAttempts:         1,
		}
	}
	if err := base().validate(); err != nil {
		t.Fatalf("config válida rechazada: %v", err)
	}
	cases := map[string]func(*Config){
		// La errata de un modo MTA-STS no la delata nada en marcha: el fichero se
		// sirve igual y el remitente descarta la política entera.
		"mta-sts mode": func(c *Config) { c.MTASTSMode = "enfore" },
		"max message":  func(c *Config) { c.MaxMessageBytes = 0 },
		"recipients":   func(c *Config) { c.MaxRecipients = 0 },
		"workers":      func(c *Config) { c.QueueWorkers = 0 },
		"attempts":     func(c *Config) { c.QueueMaxAttempts = 0 },
		"hostname":     func(c *Config) { c.Hostname = "" },
	}
	for name, mut := range cases {
		c := base()
		mut(&c)
		if err := c.validate(); err == nil {
			t.Errorf("caso %q debía fallar la validación", name)
		}
	}
}

func TestValidateProdRequiresTLS(t *testing.T) {
	c := Config{
		Hostname:                 "mx",
		MTASTSMode:               "testing",
		MaxMessageBytes:          1,
		DefaultMailboxQuotaBytes: 1,
		MaxRecipients:            1,
		QueueWorkers:             1,
		QueueMaxAttempts:         1,
	}
	c.Env = "prod"
	if err := c.validate(); err == nil {
		t.Fatal("prod sin TLS debe fallar")
	}
	c.TLSCertFile = "/c.pem"
	c.TLSKeyFile = "/k.pem"
	if err := c.validate(); err != nil {
		t.Fatalf("prod con TLS válida: %v", err)
	}
	if !c.TLSConfigured() {
		t.Fatal("TLSConfigured debe ser true con cert+key")
	}
}

// TestValidateProdSMIMESinKEK: S/MIME está encendido por defecto y su KEK llega
// del secreto como `optional`. Sin ella el servicio arrancaba igual y el correo
// saliente dejaba de firmarse con un aviso en el log; una función de seguridad
// apagada en silencio no es una degradación, es un fallo. En dev sigue valiendo.
func TestValidateProdSMIMESinKEK(t *testing.T) {
	c := Config{
		Hostname: "mx", MTASTSMode: "testing", MaxMessageBytes: 1, MaxRecipients: 1,
		DefaultMailboxQuotaBytes: 1,
		QueueWorkers:             1, QueueMaxAttempts: 1,
		TLSCertFile: "/c.pem", TLSKeyFile: "/k.pem",
		SMIMEEnabled: true,
	}
	c.Env = "prod"
	if err := c.validate(); err == nil {
		t.Fatal("prod con S/MIME encendido y sin KEK debe fallar")
	}
	// Con la clave, adelante.
	c.SMIMEKeyEncKeyB64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	if err := c.validate(); err != nil {
		t.Fatalf("prod con S/MIME y KEK: %v", err)
	}
	// Apagarlo a propósito también vale: lo que no vale es creer que está.
	c.SMIMEKeyEncKeyB64, c.SMIMEEnabled = "", false
	if err := c.validate(); err != nil {
		t.Fatalf("prod con S/MIME apagado explícitamente: %v", err)
	}
	// En dev no se exige: no hay correo real que firmar.
	c.Env, c.SMIMEEnabled = "dev", true
	if err := c.validate(); err != nil {
		t.Fatalf("dev sin KEK debe seguir valiendo: %v", err)
	}
}

// TestCuotaDeBuzonConfigurable fija lo que hace posible desplegar en un
// servidor pequeño: poder bajar la cuota sin recompilar. Los cuerpos MIME viven
// en el MISMO disco que PostgreSQL y que los blobs de Drive, así que cuatro
// buzones a 10 GiB son 40 GiB que antes nadie podía acotar.
func TestCuotaDeBuzonConfigurable(t *testing.T) {
	setRequired(t)
	t.Setenv("DEFAULT_MAILBOX_QUOTA_GB", "4")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if quiero := int64(4) * 1024 * 1024 * 1024; cfg.DefaultMailboxQuotaBytes != quiero {
		t.Errorf("DefaultMailboxQuotaBytes = %d, quiero %d", cfg.DefaultMailboxQuotaBytes, quiero)
	}
	// Cero no desactiva la cuota: sería un buzón que no admite ni un mensaje.
	c := cfg
	c.DefaultMailboxQuotaBytes = 0
	if err := c.validate(); err == nil {
		t.Error("una cuota de 0 debería rechazarse, no aceptarse como 'sin límite'")
	}
}
