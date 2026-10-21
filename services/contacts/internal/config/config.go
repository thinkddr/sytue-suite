// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Contacts desde el
// entorno del proceso (12-factor). Reutiliza el cargador común de
// platform/pkg/config y respeta EXACTAMENTE los nombres de variable del compose
// (PORT, DATABASE_URL, OIDC_ISSUER, OIDC_JWKS_URL, OIDC_AUDIENCE). Sin secretos
// en git.
package config

import (
	"os"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-contacts"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// Config es la configuración completa de Sytue Contacts.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://contacts.sytue.eu).
	// DragonflyURL es la cache efímera compartida (redis://dragonfly:6379). VACÍA
	// es un estado válido: sin ella cada PROPFIND vuelve a listar contra PostgreSQL.
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string

	PublicURL string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión (cookie única), así que el
	// token puede traer la audiencia de OTRO app de la suite. Se acepta si su aud
	// contiene ALGUNA (allowlist first-party). Aplica también a CardDAV.
	OIDCAcceptedAudiences []string

	// InternalSecret autoriza el endpoint service-to-service que refleja el
	// avatar propio de Sytue ID en las fichas de contacto que coincidan por
	// correo (`POST /internal/avatar-sync`). Mismo valor que
	// CONTACTS_INTERNAL_SECRET en el chart de id; del Secret Manager, NUNCA en
	// git. Vacía ⇒ la ruta no se monta (fail-closed).
	InternalSecret string
}

// Load construye la configuración leyendo el entorno. Devuelve todos los
// errores de validación juntos.
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
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://contacts.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		DragonflyURL:          l.Get("DRAGONFLY_URL"),
		DragonflyPassword:     l.Get("DRAGONFLY_PASSWORD"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-contacts"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),
		InternalSecret:        l.Get("INTERNAL_SECRET"),
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8088"
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}
