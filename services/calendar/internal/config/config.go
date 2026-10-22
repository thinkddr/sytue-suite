// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuracion tipada de Sytue Calendar desde el
// entorno del proceso (12-factor). Reutiliza el cargador comun de
// platform/pkg/config y respeta EXACTAMENTE los nombres de variable definidos en
// deploy/compose (PORT, DATABASE_URL, OIDC_ISSUER, OIDC_JWKS_URL). Sin secretos
// en git. Calendar es un servicio PG-only: no usa Object Storage S3.
package config

import (
	"fmt"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre logico del servicio (atributo de logs/telemetria).
const ServiceName = "sytue-calendar"

// DefaultPort es el puerto HTTP de Sytue Calendar (CONVENTIONS §6).
const DefaultPort = "8087"

// Config es la configuracion completa de Sytue Calendar.
type Config struct {
	pcfg.Base

	// PublicURL es la URL publica de cara al usuario (https://calendar.sytue.eu).
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Unica fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamano maximo del pool.
	DBMaxConns int

	// OIDC configura la validacion de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión (cookie única), así que el
	// token puede traer la audiencia de OTRO app de la suite. Se acepta si su aud
	// contiene ALGUNA (allowlist first-party). Aplica también a CalDAV.
	OIDCAcceptedAudiences []string

	// CalDAVPrefix es el prefijo de montaje del servidor CalDAV (RFC 4791).
	CalDAVPrefix string
	// DragonflyURL es la cache efímera compartida (redis://dragonfly:6379). VACÍA
	// es un estado válido: sin ella cada PROPFIND vuelve a listar colecciones
	// contra PostgreSQL, que es el comportamiento previo.
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string

	// MaxFreeBusyWindow acota la ventana temporal de una consulta free/busy.
	MaxFreeBusyWindow time.Duration
	// MaxRangeWindow acota la ventana de una consulta de instancias (calendar-query).
	MaxRangeWindow time.Duration
	// MaxObjectBytes limita el tamano de un recurso iCalendar (PUT CalDAV).
	MaxObjectBytes int64

	// ReminderScanInterval es el periodo del worker de avisos de recordatorios
	// (VALARM): cada tick drena la cola de disparos vencidos y publica el aviso en
	// el bus. Un valor <= 0 desactiva el worker.
	ReminderScanInterval time.Duration

	// SubscriptionScanInterval es el periodo del worker de suscripciones ICS: cada
	// tick reclama y refresca los feeds vencidos. Un valor <= 0 desactiva el worker.
	SubscriptionScanInterval time.Duration
	// SubscriptionRefreshInterval es la cadencia por defecto de refresco de un feed
	// nuevo (se acota a [15m, 7d] y respeta un Cache-Control mayor).
	SubscriptionRefreshInterval time.Duration
	// SubscriptionMaxBytes limita el tamaño de la respuesta de un feed ICS.
	SubscriptionMaxBytes int64

	// Hub configura la emisión de notificaciones unificadas hacia Sytue Hub.
	HubBaseURL string
	// IDBaseURL es Sytue ID por dentro: de ahí sale el correo del dueño de un
	// calendario cuando alguien le pide editarlo (platform/pkg/peticiones).
	IDBaseURL string
	// IDNombresOrgsSecret abre `POST /internal/orgs/names` de Sytue ID: el nombre
	// visible de la organización que convoca. Vacío ⇒ solo su identificador.
	IDNombresOrgsSecret string
	HubInternalSecret   string

	// SMTP del buzón con el que salen las invitaciones por correo (iMIP) a quien NO es
	// de la suite: el relé de Sytue Mail. Sin SMTPAddr o SMTPFrom no sale ninguna y el
	// resto de Calendar funciona igual (dentro de la suite la invitación ya llega a la
	// bandeja y a la campana).
	SMTPAddr, SMTPFrom, SMTPUser, SMTPPassword, SMTPTLSServerName string
	// MailInternalURL es la raíz interna de Sytue Mail, que da los dominios de correo
	// de la suite: quien tiene una dirección en ellos recibe la invitación dentro y no
	// por correo. Vacía: solo cuenta como de dentro el apex de PublicURL.
	MailInternalURL string
}

// Load construye la configuracion leyendo el entorno. Devuelve todos los errores
// de validacion juntos.
func Load() (Config, error) {
	// Prefijo vacio: las variables del compose no llevan prefijo (PORT, LOG_LEVEL,
	// ...). HTTP_ADDR se deriva de PORT mas abajo.
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://calendar.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-calendar"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		CalDAVPrefix:         l.GetDefault("CALDAV_PREFIX", "/dav"),
		DragonflyURL:         l.Get("DRAGONFLY_URL"),
		DragonflyPassword:    l.Get("DRAGONFLY_PASSWORD"),
		MaxFreeBusyWindow:    l.GetDurationDefault("MAX_FREEBUSY_WINDOW", 366*24*time.Hour),
		MaxRangeWindow:       l.GetDurationDefault("MAX_RANGE_WINDOW", 366*24*time.Hour),
		MaxObjectBytes:       int64(l.GetIntDefault("MAX_OBJECT_KB", 1024)) * 1024,
		ReminderScanInterval: l.GetDurationDefault("REMINDER_SCAN_INTERVAL", time.Minute),

		SubscriptionScanInterval:    l.GetDurationDefault("SUBSCRIPTION_SCAN_INTERVAL", time.Minute),
		SubscriptionRefreshInterval: l.GetDurationDefault("SUBSCRIPTION_REFRESH_INTERVAL", time.Hour),
		SubscriptionMaxBytes:        int64(l.GetIntDefault("SUBSCRIPTION_MAX_KB", 5120)) * 1024,

		HubBaseURL:          l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret:   l.HubInternalSecret(base.Env),
		IDBaseURL:           l.GetDefault("ID_BASE_URL", "http://id:8081"),
		IDNombresOrgsSecret: l.Get("ID_NOMBRES_ORGS_SECRET"),

		SMTPAddr:          l.Get("SMTP_ADDR"),
		SMTPFrom:          l.Get("SMTP_FROM"),
		SMTPUser:          l.Get("SMTP_USER"),
		SMTPPassword:      l.Get("SMTP_PASSWORD"),
		SMTPTLSServerName: l.Get("SMTP_TLS_SERVER_NAME"),
		MailInternalURL:   l.Get("MAIL_INTERNAL_URL"),
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
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.MaxObjectBytes <= 0 {
		return fmt.Errorf("config: MAX_OBJECT_KB debe ser > 0")
	}
	if c.MaxFreeBusyWindow <= 0 {
		return fmt.Errorf("config: MAX_FREEBUSY_WINDOW debe ser > 0")
	}
	if c.MaxRangeWindow <= 0 {
		return fmt.Errorf("config: MAX_RANGE_WINDOW debe ser > 0")
	}
	return nil
}
