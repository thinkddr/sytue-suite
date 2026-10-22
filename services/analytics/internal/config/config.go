// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Analytics desde el
// entorno (12-factor). La comparten los dos binarios —analytics-ingest y
// analytics-api— porque hablan con la misma base y comparten la sal; cada uno
// usa el subconjunto que le toca y arranca en su puerto (CONVENTIONS §6:
// colector 8094, API de lectura 8095).
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// Nombres lógicos de cada binario (atributo de logs/telemetría).
const (
	ServiceNameIngest = "sytue-analytics-ingest"
	ServiceNameAPI    = "sytue-analytics-api"
)

// Puertos por defecto (CONVENTIONS §6).
const (
	PortIngest = 8094
	PortAPI    = 8095
)

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Config es la configuración completa de Sytue Analytics.
type Config struct {
	pcfg.Base

	// DatabaseURL es el DSN de PostgreSQL (base sytue_analytics).
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// KeyEncryptionKey es la KEK AES-256 (32 bytes ya decodificados) con la que
	// se cifra la SAL DIARIA en reposo (platform/pkg/keycrypto). Origen:
	// SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY en base64 estándar.
	//
	// Vacía => la sal se guarda EN CLARO. keycrypto lo permite (modo dev) y por
	// eso el arranque avisa a gritos: sin KEK, un volcado de PostgreSQL basta
	// para recalcular los hashes del día, que es justo lo que el cifrado impide.
	KeyEncryptionKey []byte

	// GeoDBPath es la ruta al fichero DB-IP IP-to-Country Lite (.csv o .csv.gz)
	// empotrado en la imagen. Vacía o inexistente => todo el tráfico queda como
	// país "ZZ"; el servicio arranca igual.
	GeoDBPath string

	// TrustedProxies es la lista de saltos que pueden afirmar X-Forwarded-For.
	// Nunca se toma una cabecera de IP sin comprobar antes el peer TCP.
	TrustedProxies clientip.TrustedProxies
	// ReservedHostSuffixes son los dominios que Pages da de alta por la ruta
	// interna. La API de usuario no puede apropiárselos.
	ReservedHostSuffixes []string
	// PagesSecret protege la ruta interna que Pages usa para registrar sus hosts.
	PagesSecret string

	// RatePerSite y BurstPerSite acotan los eventos por segundo de un sitio.
	RatePerSite  float64
	BurstPerSite int
	// RatePerIP y BurstPerIP acotan los de un visitante concreto dentro de un sitio.
	RatePerIP  float64
	BurstPerIP int

	// BatchSize y BatchInterval gobiernan el firehose: se escribe con INSERT
	// multi-fila cada BatchInterval o cada BatchSize eventos, lo que llegue antes.
	// (Bajo FORCE RLS, PostgreSQL RECHAZA COPY … FROM: no hay opción más rápida.)
	BatchSize     int
	BatchInterval time.Duration
	// QueueSize es la cola en memoria entre los handlers y el escritor. Llena =>
	// se descartan eventos: el colector jamás bloquea la petición del visitante.
	QueueSize int

	// SiteCacheTTL es lo que se cachea en proceso la resolución Host -> sitio.
	SiteCacheTTL time.Duration

	// RawRetentionDays son los días de crudo conservados (2 = hoy y ayer, o sea
	// ninguna fila por encima de 48 h). Es la retención del ADR: corta a propósito,
	// porque es donde vive el hash seudónimo.
	RawRetentionDays int
	// PartitionsAhead son las particiones diarias que el cron crea por delante.
	PartitionsAhead int
	// RollupInterval es cada cuánto se recalculan los agregados.
	RollupInterval time.Duration
	// BreakdownTopN es el top por día y dimensión que se guarda; el resto cae en
	// el cajón "(otras)". Es lo que acota el crecimiento del agregado.
	BreakdownTopN int

	// OIDC configura la validación de tokens de Sytue ID en la API de lectura.
	OIDCIssuer            string
	OIDCJWKSURL           string
	OIDCAcceptedAudiences []string
	// CORSAllowedOrigins es la allowlist del panel (sección de cloud-dashboard).
	CORSAllowedOrigins []string
}

// Load construye la configuración leyendo el entorno.
func Load(serviceName string, defaultPort int) (Config, error) {
	base, err := pcfg.LoadBase("", serviceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")
	cfg := Config{
		Base:        base,
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		GeoDBPath:            l.GetDefault("GEO_DB_PATH", "/app/data/dbip-country-lite.csv.gz"),
		ReservedHostSuffixes: l.GetCSV("RESERVED_HOST_SUFFIXES"),
		PagesSecret:          l.SecretoOpcional(base.Env, "ANALYTICS_PAGES_SECRET", pcfg.DevAnalyticsPagesSecret),

		RatePerSite:  float64(l.GetIntDefault("RATE_PER_SITE", 200)),
		BurstPerSite: l.GetIntDefault("BURST_PER_SITE", 400),
		RatePerIP:    float64(l.GetIntDefault("RATE_PER_IP", 2)),
		BurstPerIP:   l.GetIntDefault("BURST_PER_IP", 20),

		BatchSize:     l.GetIntDefault("BATCH_SIZE", 500),
		BatchInterval: l.GetDurationDefault("BATCH_INTERVAL", time.Second),
		QueueSize:     l.GetIntDefault("QUEUE_SIZE", 10000),
		SiteCacheTTL:  l.GetDurationDefault("SITE_CACHE_TTL", time.Minute),

		RawRetentionDays: l.GetIntDefault("RAW_RETENTION_DAYS", 2),
		PartitionsAhead:  l.GetIntDefault("PARTITIONS_AHEAD", 7),
		RollupInterval:   l.GetDurationDefault("ROLLUP_INTERVAL", 5*time.Minute),
		BreakdownTopN:    l.GetIntDefault("BREAKDOWN_TOP_N", 100),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences("sytue-analytics")),
		CORSAllowedOrigins: l.GetCSVDefault("CORS_ALLOWED_ORIGINS",
			pcfg.FirstPartyCORSOrigins(base.Env)),
	}
	trusted, err := clientip.ParseTrustedProxies(l.Get("TRUSTED_PROXIES"))
	if err != nil {
		return Config{}, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	// Vacía conserva el valor seguro para Kubernetes: las redes privadas. El
	// operador puede indicar "none" para desactivar expresamente los proxies.
	if l.Get("TRUSTED_PROXIES") == "" {
		trusted = clientip.DefaultTrustedProxies()
	}
	cfg.TrustedProxies = trusted

	// PORT (compose) tiene prioridad sobre HTTP_ADDR.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = fmt.Sprintf(":%d", defaultPort)
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}

	// KEK de la sal diaria (base64 → 32 bytes AES-256). Fallo rápido si está mal
	// configurada, en vez de arrancar con el cifrado roto.
	if raw := l.Get("SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY: base64 inválido: %w", err)
		}
		if len(key) != 32 {
			return Config{}, fmt.Errorf("SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY: debe medir 32 bytes (AES-256), tiene %d", len(key))
		}
		cfg.KeyEncryptionKey = key
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.RawRetentionDays < 1 || c.RawRetentionDays > 7 {
		return fmt.Errorf("config: RAW_RETENTION_DAYS debe estar en 1..7 (el ADR fija 2 = 48 h)")
	}
	if c.PartitionsAhead < 1 || c.PartitionsAhead > 60 {
		return fmt.Errorf("config: PARTITIONS_AHEAD debe estar en 1..60")
	}
	if c.BatchSize < 1 || c.BatchSize > 5000 {
		return fmt.Errorf("config: BATCH_SIZE debe estar en 1..5000")
	}
	if c.QueueSize < c.BatchSize {
		return fmt.Errorf("config: QUEUE_SIZE (%d) no puede ser menor que BATCH_SIZE (%d)", c.QueueSize, c.BatchSize)
	}
	if c.BreakdownTopN < 1 || c.BreakdownTopN > 1000 {
		return fmt.Errorf("config: BREAKDOWN_TOP_N debe estar en 1..1000")
	}
	// Sin KEK, keycrypto NO falla: devuelve el texto plano y la sal del día queda
	// legible en la base. El colector avisaba en el log y la API ni eso, así que
	// el único cifrado que separa los identificadores de los visitantes de un
	// volcado de PostgreSQL podía estar apagado sin que nada lo dijera. En dev se
	// sigue permitiendo (no hay nada que proteger); en prod, no. La comprobación
	// vive aquí porque Load la comparten los DOS binarios y así no pueden
	// divergir.
	if c.Env.IsProd() && len(c.KeyEncryptionKey) == 0 {
		return fmt.Errorf("config: SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY es obligatoria en prod " +
			"(sin ella la sal diaria se guarda EN CLARO y un volcado de la base basta para recalcular los identificadores del día)")
	}
	return nil
}
