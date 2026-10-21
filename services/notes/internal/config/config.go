// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Notepads desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable del compose (PORT, DATABASE_URL,
// OIDC_ISSUER, OIDC_JWKS_URL, OIDC_AUDIENCE). Sin secretos en git.
package config

import (
	"os"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-notes"

// DefaultPort es el puerto HTTP de Sytue Notepads (CONVENTIONS §6). Reaprovecha el
// hueco que dejó la señalización de Meet al unificarse Chat+Meet en Rooms.
const DefaultPort = "8089"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Config es la configuración completa de Sytue Notepads.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://notes.sytue.eu).
	PublicURL string
	// HubBaseURL y HubInternalSecret avisan en el centro de notificaciones de las
	// peticiones de edición de un cuaderno. Mismos defaults que calendar.
	HubBaseURL        string
	HubInternalSecret string
	// Correo es el buzón de los avisos por correo (avisos@<dominio>); IDBaseURL, Sytue
	// ID por dentro, de donde sale el correo del dueño con la sesión de quien pide.
	Correo    correo.Config
	IDBaseURL string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión, así que el token puede traer
	// la audiencia de OTRA app. Se acepta si su aud contiene ALGUNA (allowlist
	// first-party).
	OIDCAcceptedAudiences []string

	// TrashRetention es cuánto permanece en la papelera lo borrado antes de que el
	// worker lo destruya. Mismo valor por defecto que Drive (30 días): que dos
	// productos de la misma suite olviden a ritmos distintos es difícil de
	// explicar y más difícil de auditar.
	//
	// A CERO SE APAGA la purga, y es la salida deliberada: una instalación que
	// deba conservarlo todo —un litigio, una obligación sectorial— no tiene que
	// parchear código para conseguirlo.
	TrashRetention time.Duration
	// TrashGCInterval es cada cuánto despierta el worker. La retención se mide en
	// días, así que la frecuencia solo decide cuánto tarda en notarse.
	TrashGCInterval time.Duration
}

// Load construye la configuración leyendo el entorno.
func Load() (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://notes.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-notes"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret: l.HubInternalSecret(base.Env),
		Correo:            l.Correo(),
		IDBaseURL:         l.GetDefault("ID_BASE_URL", "http://id:8081"),

		TrashRetention:  l.GetDurationDefault("TRASH_RETENTION", 30*24*time.Hour),
		TrashGCInterval: l.GetDurationDefault("TRASH_GC_INTERVAL", time.Hour),
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":" + DefaultPort
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}
