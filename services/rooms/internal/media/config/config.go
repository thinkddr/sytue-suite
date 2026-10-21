// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Rooms desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable definidos en deploy/compose
// (PORT, DATABASE_URL, OIDC_*, LIVEKIT_*). Sin secretos en git.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-meet"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// Config es la configuración completa de Sytue Rooms.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de la API de señalización (https://meet-api.sytue.eu).
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
	// token del usuario puede traer la audiencia de OTRO app de la suite. El backend
	// acepta el token si su aud contiene ALGUNA de estas (allowlist first-party).
	OIDCAcceptedAudiences []string

	// DocsURL es la base de Sytue Docs: de ahí sale quién puede entrar a una sala nacida en
	// un documento. Vacía = esas salas quedan cerradas.
	DocsURL string
	// CalendarURL es la base de Sytue Calendar: de ahí sale quién está invitado a la
	// reunión de un evento (GET /api/v1/events/invited). Vacío = solo entran quien la
	// convocó, quien ya estuvo dentro y quien trae la clave del enlace.
	CalendarURL string

	// LiveKit configura el SFU de medios (Apache-2.0).
	//
	// LiveKitURL es la URL pública de señalización WebRTC que el cliente web usa
	// (wss://meet-sfu.sytue.eu). LiveKitHost es la URL de la API de servidor
	// (http(s)://livekit:7880) para gestión de salas y verificación de webhooks.
	LiveKitURL       string
	LiveKitHost      string
	LiveKitAPIKey    string
	LiveKitAPISecret string

	// AccessTokenTTL es la validez del JWT de LiveKit minteado para el participante.
	AccessTokenTTL time.Duration
	// RoomEmptyTimeout es el tiempo que LiveKit mantiene una sala vacía antes de
	// cerrarla (se pasa como sugerencia al crear la sala).
	RoomEmptyTimeout time.Duration
	// MaxParticipants es el límite por defecto de participantes en una sala.
	MaxParticipants int

	// --- Grabación (egress) --------------------------------------------------
	//
	// Todo esto es OPCIONAL a propósito: sin bucket configurado la grabación se
	// rechaza con un error claro en vez de arrancar un egress que no sabe dónde
	// dejar el fichero. Un servicio que no puede grabar tiene que poder arrancar
	// igual —la mensajería y las llamadas no dependen de esto—.
	RecordingBucket    string
	RecordingEndpoint  string
	RecordingRegion    string
	RecordingAccessKey string
	RecordingSecretKey string
	// RecordingLayout es la plantilla del compositor: "grid" (mosaico) o
	// "speaker" (el que habla, grande). Vacío deja la de LiveKit.
	RecordingLayout string
}

// Load construye la configuración leyendo el entorno. Devuelve todos los
// errores de validación juntos.
func Load() (Config, error) {
	// Base usa prefijo vacío: las variables del compose no llevan prefijo
	// (PORT, LOG_LEVEL, ...). HTTP_ADDR se deriva de PORT más abajo.
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
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://meet-api.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:   l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:  l.Get("OIDC_JWKS_URL"),
		OIDCAudience: l.GetDefault("OIDC_AUDIENCE", "sytue-meet"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES",
			"sytue-one,sytue-mail,sytue-drive,sytue-docs,sytue-calendar,sytue-contacts,sytue-rooms,sytue-chat,sytue-meet,sytue-cloud,sytue-console,sytue-hub"),

		DocsURL:          l.GetDefault("DOCS_URL", "http://docs:8083"),
		CalendarURL:      l.GetDefault("CALENDAR_URL", "http://calendar:8087"),
		LiveKitURL:       l.GetDefault("LIVEKIT_URL", "wss://meet-sfu.sytue.localhost"),
		LiveKitHost:      l.GetDefault("LIVEKIT_HOST", "http://livekit:7880"),
		LiveKitAPIKey:    l.Require("LIVEKIT_API_KEY"),
		LiveKitAPISecret: l.Require("LIVEKIT_API_SECRET"),

		AccessTokenTTL:   l.GetDurationDefault("MEET_ACCESS_TOKEN_TTL", 6*time.Hour),
		RoomEmptyTimeout: l.GetDurationDefault("MEET_ROOM_EMPTY_TIMEOUT", 5*time.Minute),
		MaxParticipants:  l.GetIntDefault("MEET_MAX_PARTICIPANTS", 100),

		// Destino de las grabaciones. Sin `RECORDING_BUCKET` la función queda
		// apagada; ver `PuedeGrabar()`.
		RecordingBucket:    l.GetDefault("RECORDING_BUCKET", ""),
		RecordingEndpoint:  l.GetDefault("RECORDING_ENDPOINT", ""),
		RecordingRegion:    l.GetDefault("RECORDING_REGION", "fr-par"),
		RecordingAccessKey: l.GetDefault("RECORDING_ACCESS_KEY", ""),
		RecordingSecretKey: l.GetDefault("RECORDING_SECRET_KEY", ""),
		RecordingLayout:    l.GetDefault("RECORDING_LAYOUT", "grid"),

		// Aviso meet→chat (tarjeta «llamada finalizada»). Vacíos => deshabilitado.
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8089"
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
	if c.LiveKitAPIKey == "" || c.LiveKitAPISecret == "" {
		return fmt.Errorf("config: LIVEKIT_API_KEY y LIVEKIT_API_SECRET son obligatorios")
	}
	if c.AccessTokenTTL <= 0 {
		return fmt.Errorf("config: MEET_ACCESS_TOKEN_TTL debe ser > 0")
	}
	if c.MaxParticipants <= 0 {
		return fmt.Errorf("config: MEET_MAX_PARTICIPANTS debe ser > 0")
	}
	return nil
}

// PuedeGrabar indica si el servicio tiene destino donde dejar una grabación.
//
// Se comprueban las TRES piezas y no solo el bucket: con el nombre pero sin
// credenciales, el egress arrancaría, se uniría a la sala —con su aviso de
// grabación a todo el mundo— y fallaría al subir el fichero. Se habría grabado a
// la gente para nada, que es el peor de los desenlaces.
func (c Config) PuedeGrabar() bool {
	return c.RecordingBucket != "" && c.RecordingAccessKey != "" && c.RecordingSecretKey != ""
}

// RecordingEndpointURL devuelve el endpoint del bucket CON esquema, que es como
// lo quiere el egress de LiveKit.
//
// La misma variable la consume también el cliente S3 de este proceso, que lo
// quiere justo al revés (solo host). En vez de dos variables que alguien acabará
// dejando descuadradas, se guarda una y cada consumidor la adapta. Sin esquema se
// asume TLS: en la UE, un bucket en claro no es una opción por defecto.
func (c Config) RecordingEndpointURL() string {
	if c.RecordingEndpoint == "" || strings.Contains(c.RecordingEndpoint, "://") {
		return c.RecordingEndpoint
	}
	return "https://" + c.RecordingEndpoint
}
