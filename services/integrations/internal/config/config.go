// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada del Sytue Integration Gateway
// desde el entorno (12-factor). Reutiliza platform/pkg/config. Las variables de
// infraestructura compartidas por la suite (PORT, DATABASE_URL, OIDC_*) llegan sin
// prefijo (las fija el compose/Helm); el material propio del servicio usa el
// prefijo SYTUE_INTEGRATIONS_ (p. ej. la KEK). Sin secretos en git.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-integrations"

// Version la inyecta el linker en build; sirve de default cuando VERSION no está.
var Version = "dev"

// Config es la configuración completa del gateway.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario.
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL. DBMaxConns el tamaño del pool.
	DatabaseURL string
	DBMaxConns  int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer            string
	OIDCJWKSURL           string
	OIDCAudience          string
	OIDCAcceptedAudiences []string

	// IDBaseURL es la URL interna de Sytue ID (para futuras llamadas de servicio).
	IDBaseURL string

	// ID gateway client_credentials: credenciales del client `sytue-gateway` (lo
	// siembra la ola W2A) con las que la pasarela LEE el directorio de una org
	// (usuarios/grupos) de los endpoints admin de Sytue ID. Sin secret, el
	// aprovisionamiento SCIM saliente no puede leer el directorio (Sync fallará y el
	// outbox reintentará); el servicio arranca igual.
	IDClientID     string
	IDClientSecret string

	// Destino de la MIGRACIÓN de contenido: las URLs internas de Sytue Drive y
	// Sytue Contacts. La migración escribe por su API pública (nunca contra sus
	// bases de datos), así que solo necesita saber dónde están.
	DriveBaseURL    string
	ContactsBaseURL string
	// CalendarBaseURL es Sytue Calendar: destino de la agenda migrada.
	CalendarBaseURL string
	// CloudBaseURL es Sytue Cloud: destino de las aplicaciones, los buckets y las
	// tareas programadas que se migran desde Google Cloud o Firebase.
	CloudBaseURL string
	// MailBaseURL es Sytue Mail: destino del buzón que se migra desde otro
	// proveedor. Vacía ⇒ el contenido de tipo 'mail' se rechaza con un motivo
	// legible en vez de fallar a mitad de trabajo.
	MailBaseURL string
	// DocsBaseURL es Sytue Docs: destino de los documentos NATIVOS del origen (un
	// Google Doc no es un fichero, se convierte y se crea editable). Vacía ⇒ esos
	// documentos se omiten con su motivo en vez de aterrizar como adjuntos.
	DocsBaseURL string

	// RoomsBaseURL es la URL interna de Sytue Rooms, destino de los equipos y
	// canales que trae la migración desde Microsoft Teams. Vacía ⇒ el dominio
	// 'teams' se rechaza con un motivo legible en vez de fallar a mitad de trabajo.
	RoomsBaseURL string

	// Sytue Git: la forja de la organización, destino de los repositorios que trae
	// la migración desde GitHub. GitToken es un token de ADMINISTRADOR de la forja:
	// con él y la cabecera `Sudo`, cada repositorio se crea a nombre de la persona
	// destino y no de una cuenta de servicio compartida. Vacíos ⇒ el dominio 'repos'
	// se rechaza con un motivo legible en vez de fallar a mitad de trabajo.
	// Origen: GIT_URL y SYTUE_INTEGRATIONS_GIT_TOKEN.
	GitBaseURL string
	GitToken   string

	// OperatorToken es la credencial de PLATAFORMA que habilita
	// PUT /operator/delegated-client (instalar el client delegable de una org, con
	// el que la migración escribe en la cuenta de cada persona). Es deliberadamente
	// DISTINTA de cualquier credencial de usuario o de admin de organización: quien
	// puede conceder capacidad de impersonación es el operador del despliegue, no
	// una sesión de la Console. Vacía ⇒ el endpoint no existe (404).
	// Origen: SYTUE_INTEGRATIONS_OPERATOR_TOKEN.
	OperatorToken string
	// InternalSecret autentica a OTROS servicios de la suite (Drive) en los
	// endpoints /internal/v1/*: les presta el token OAuth de una instancia de
	// almacenamiento (Google Drive, OneDrive). Vacío ⇒ no se montan. Origen:
	// SYTUE_INTEGRATIONS_INTERNAL_SECRET.
	InternalSecret string

	// KeyEncryptionKey es la KEK AES-256 (32 bytes ya decodificados) con la que se
	// sellan los secretos de instancia at-rest (platform/pkg/keycrypto). Opcional:
	// sin ella el servicio arranca, pero rechaza escribir secretos. Origen:
	// SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY en base64 estándar.
	KeyEncryptionKey []byte

	// OAuthClients son las credenciales de las apps que SYTUE tiene registradas ante
	// cada proveedor, indexadas por clave de plantilla ("jira", "slack"…). Son la
	// diferencia entre un clic y varios: si Sytue tiene app registrada con el
	// proveedor, el admin solo aprueba una pantalla de permisos; si no la hay, el
	// asistente tiene que pedirle que registre la suya y pegue client_id/secret —
	// que sigue siendo mejor que un token estático (caduca, se acota por scope y se
	// revoca desde el proveedor), pero NO es 1 clic.
	//
	// Registrar la app de Sytue en cada proveedor es una tarea de OPERACIÓN, no de
	// código: hay que darse de alta como desarrollador, declarar el redirect_uri y
	// pasar su revisión. Ver docs/interoperabilidad.md.
	//
	// Origen: SYTUE_INTEGRATIONS_OAUTH_CLIENTS, JSON
	// {"jira":{"client_id":"…","client_secret":"…"}}. Vacío ⇒ todas BYO.
	OAuthClients map[string]OAuthClient

	// Egress: presupuesto del limitador por org:instance y límites del cliente.
	EgressRatePerSec float64
	EgressBurst      int
	EgressTimeout    time.Duration
	// MigrationEgressTimeout acota UNA petición de una migración de contenido. No es
	// el de las llamadas a API (15 s): un vídeo de 2 GB no sube en 15 segundos, y con
	// ese tope cualquier fichero grande fallaba en bucle en las dos direcciones.
	MigrationEgressTimeout time.Duration
	EgressMaxRedirects     int

	// PolicyCacheTTL es cuánto cachea el motor de gobernanza las reglas de una org
	// (una carga por org cada TTL, no una query por egress). Acota la ventana de
	// convergencia entre réplicas tras editar una regla.
	PolicyCacheTTL time.Duration

	// CORSAllowedOrigins es la allowlist de orígenes del navegador (consola de org).
	CORSAllowedOrigins []string

	// SyncSweepInterval es cada cuánto se encola una reconciliación SCIM para todas las
	// conexiones activas que aprovisionen. Es lo que hace que dar de baja a alguien en
	// Sytue ID llegue al SaaS del cliente sin que nadie pulse nada. 0 lo desactiva (solo
	// para depurar: sin él, el desaprovisionamiento depende de un humano).
	SyncSweepInterval time.Duration

	// ConsoleURL es adónde se devuelve al admin cuando vuelve del consentimiento del
	// proveedor. Sale de AQUÍ y no de la petición: al final del flujo hay una persona
	// mirando el navegador, y calcular el destino desde una cabecera sería dejar que
	// quien la falsifique elija adónde aterriza.
	ConsoleURL string
	// DrivePublicURL es el origen público de Drive. El consentimiento de
	// almacenamiento vuelve a su BFF, sin exigir acceso a Sytue Admin.
	DrivePublicURL string
	// RoomsPublicURL es el origen público del BFF que termina las conexiones de Rooms.
	RoomsPublicURL    string
	MailPublicURL     string
	CalendarPublicURL string
	ContactsPublicURL string
	NotesPublicURL    string
	// WhatsAppConfigID selecciona Embedded Signup v4 en Facebook Login for Business.
	WhatsAppConfigID           string
	WhatsAppGraphVersion       string
	WhatsAppWebhookVerifyToken string
}

// OAuthClient es la credencial con la que Sytue se presenta ante UN proveedor. El
// secret puede ir vacío: hay proveedores que registran la app como cliente público y
// se apoyan solo en PKCE.
type OAuthClient struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
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
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://integrations.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-integrations"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences("sytue-integrations")),

		IDBaseURL:      l.GetDefault("ID_URL", "http://id:8081"),
		IDClientID:     l.GetDefault("SYTUE_INTEGRATIONS_ID_CLIENT_ID", "sytue-gateway"),
		IDClientSecret: l.Get("SYTUE_INTEGRATIONS_ID_CLIENT_SECRET"),

		DriveBaseURL:    l.GetDefault("DRIVE_URL", "http://drive:8082"),
		ContactsBaseURL: l.GetDefault("CONTACTS_URL", "http://contacts:8088"),
		DocsBaseURL:     l.GetDefault("DOCS_URL", "http://docs:8084"),
		MailBaseURL:     l.GetDefault("MAIL_URL", "http://mail:8083"),
		CalendarBaseURL: l.GetDefault("CALENDAR_URL", "http://calendar:8086"),
		CloudBaseURL:    l.GetDefault("CLOUD_URL", "http://cloud-api:8090"),

		RoomsBaseURL: l.GetDefault("ROOMS_URL", "http://rooms:8085"),

		GitBaseURL: l.GetDefault("GIT_URL", ""),
		GitToken:   l.Get("SYTUE_INTEGRATIONS_GIT_TOKEN"),

		OperatorToken:  l.Get("SYTUE_INTEGRATIONS_OPERATOR_TOKEN"),
		InternalSecret: l.Get("SYTUE_INTEGRATIONS_INTERNAL_SECRET"),

		// Tasa entera de tokens/seg (una fracción no aporta a este límite); se
		// promueve a float64 para pg.RateLimiterConfig.
		EgressRatePerSec:       float64(l.GetIntDefault("EGRESS_RATE_PER_SEC", 20)),
		EgressBurst:            l.GetIntDefault("EGRESS_BURST", 40),
		EgressTimeout:          l.GetDurationDefault("EGRESS_TIMEOUT", 15*time.Second),
		MigrationEgressTimeout: l.GetDurationDefault("MIGRATION_EGRESS_TIMEOUT", time.Hour),
		EgressMaxRedirects:     l.GetIntDefault("EGRESS_MAX_REDIRECTS", 5),
		PolicyCacheTTL:         l.GetDurationDefault("POLICY_CACHE_TTL", 30*time.Second),

		CORSAllowedOrigins:         l.GetCSVDefault("CORS_ALLOWED_ORIGINS", pcfg.FirstPartyCORSOrigins(base.Env)),
		ConsoleURL:                 l.GetDefault("CONSOLE_URL", "https://console.sytue.localhost"),
		DrivePublicURL:             l.GetDefault("DRIVE_PUBLIC_URL", "https://drive.sytue.localhost"),
		RoomsPublicURL:             l.GetDefault("ROOMS_PUBLIC_URL", "https://rooms.sytue.localhost"),
		MailPublicURL:              l.GetDefault("MAIL_PUBLIC_URL", "https://mail.sytue.localhost"),
		CalendarPublicURL:          l.GetDefault("CALENDAR_PUBLIC_URL", "https://calendar.sytue.localhost"),
		ContactsPublicURL:          l.GetDefault("CONTACTS_PUBLIC_URL", "https://contacts.sytue.localhost"),
		NotesPublicURL:             l.GetDefault("NOTES_PUBLIC_URL", "https://notes.sytue.localhost"),
		WhatsAppConfigID:           l.Get("SYTUE_INTEGRATIONS_WHATSAPP_CONFIG_ID"),
		WhatsAppGraphVersion:       l.GetDefault("WHATSAPP_GRAPH_VERSION", "v26.0"),
		WhatsAppWebhookVerifyToken: l.Get("SYTUE_INTEGRATIONS_WHATSAPP_WEBHOOK_VERIFY_TOKEN"),
		SyncSweepInterval:          l.GetDurationDefault("SYNC_SWEEP_INTERVAL", 15*time.Minute),
	}

	// PORT (compose) tiene prioridad para fijar la escucha; si no, :8093 (CONVENTIONS §6).
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8093"
	}

	// KEK de secretos de instancia (base64 → 32 bytes AES-256). Fallo rápido si está
	// mal configurada, en vez de arrancar con cifrado roto.
	if raw := l.Get("SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY: base64 inválido: %w", err)
		}
		if len(key) != 32 {
			return Config{}, fmt.Errorf("SYTUE_INTEGRATIONS_KEY_ENCRYPTION_KEY: debe medir 32 bytes (AES-256), tiene %d", len(key))
		}
		cfg.KeyEncryptionKey = key
	}

	// Apps de Sytue registradas ante cada proveedor. Fallo rápido si el JSON está mal:
	// arrancar ignorándolo dejaría el asistente pidiendo credenciales a mano sin que
	// nadie entendiera por qué, que es el fallo más caro de diagnosticar de este flujo.
	if raw := l.Get("SYTUE_INTEGRATIONS_OAUTH_CLIENTS"); raw != "" {
		var clients map[string]OAuthClient
		if err := json.Unmarshal([]byte(raw), &clients); err != nil {
			return Config{}, fmt.Errorf("SYTUE_INTEGRATIONS_OAUTH_CLIENTS: JSON inválido: %w", err)
		}
		for key, c := range clients {
			if strings.TrimSpace(c.ClientID) == "" {
				return Config{}, fmt.Errorf("SYTUE_INTEGRATIONS_OAUTH_CLIENTS[%s]: falta client_id", key)
			}
		}
		cfg.OAuthClients = clients
	}
	cfg.OAuthClients = loadProviderOAuthEnv(l, cfg.OAuthClients)

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}

// loadProviderOAuthEnv permite configurar las credenciales OAuth de plataforma por
// variables individuales de entorno (además del JSON SYTUE_INTEGRATIONS_OAUTH_CLIENTS).
func loadProviderOAuthEnv(l *pcfg.Loader, dst map[string]OAuthClient) map[string]OAuthClient {
	type envGroup struct {
		id, secret string
		keys       []string
	}
	groups := []envGroup{
		{
			id:     l.Get("GOOGLE_OAUTH_CLIENT_ID"),
			secret: l.Get("GOOGLE_OAUTH_CLIENT_SECRET"),
			keys:   []string{"google-drive", "google-workspace", "google-chat", "google-chat-conversations", "google-meet", "gmail-mail"},
		},
		{
			id:     l.Get("MICROSOFT_OAUTH_CLIENT_ID"),
			secret: l.Get("MICROSOFT_OAUTH_CLIENT_SECRET"),
			keys:   []string{"onedrive", "microsoft365", "azure", "microsoft-mail", "microsoft-teams", "microsoft-teams-rooms", "microsoft-teams-chat"},
		},
		{
			id:     l.Get("MICROSOFT_TEAMS_OAUTH_CLIENT_ID"),
			secret: l.Get("MICROSOFT_TEAMS_OAUTH_CLIENT_SECRET"),
			keys:   []string{"microsoft-teams", "microsoft-teams-rooms", "microsoft-teams-chat"},
		},
		{
			id:     l.Get("SLACK_OAUTH_CLIENT_ID"),
			secret: l.Get("SLACK_OAUTH_CLIENT_SECRET"),
			keys:   []string{"slack", "slack-rooms", "slack-chat"},
		},
	}
	for _, g := range groups {
		id := strings.TrimSpace(g.id)
		if id == "" {
			continue
		}
		if dst == nil {
			dst = make(map[string]OAuthClient)
		}
		client := OAuthClient{
			ClientID:     id,
			ClientSecret: strings.TrimSpace(g.secret),
		}
		for _, k := range g.keys {
			dst[k] = client
		}
	}
	return dst
}
