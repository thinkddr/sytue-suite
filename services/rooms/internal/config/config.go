// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Rooms desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable definidos en deploy/compose
// (PORT, DATABASE_URL, OIDC_ISSUER, OIDC_JWKS_URL, DRIVE_URL, DRAGONFLY_URL).
// Sin secretos en git.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-rooms"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// Config es la configuración completa de Sytue Rooms.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://rooms.sytue.eu).
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

	// DriveURL es la base de la API de Sytue Drive (para resolver adjuntos).
	DriveURL string
	// Internal OAuth gateway. Provider credentials never enter the browser;
	// WhatsApp numbers reuse Rooms' encrypted account storage.
	IntegrationsURL            string
	IntegrationsInternalSecret string
	MatrixInternalURL          string
	MatrixHomeserverURL        string
	MatrixSourceURL            string
	MatrixServerName           string
	MatrixSSOProviderID        string
	MatrixAdminToken           string
	MatrixRequired             bool
	MatrixLifecycleSecret      string
	MatrixIdentitySecret       string
	DocumentCallAccessSecret   string
	EncryptedDocumentCalls     bool

	// IDURL es la base INTERNA de Sytue ID: de ahí sale a quién se le escribe
	// cuando se teclea el correo o el @usuario de alguien de otra organización
	// (people:resolve, ADR 0040). Vacía = no se puede añadir a nadie de fuera.
	IDURL string
	// IDNombresOrgsSecret abre `POST /internal/orgs/names` de Sytue ID: el nombre
	// visible de la organización que invita. Vacío ⇒ solo su identificador.
	IDNombresOrgsSecret string

	// WhatsAppGraphURL es la raíz versionada de la Graph API de Meta con la que se
	// habla con WhatsApp Business. Configurable para fijar la versión sin release y
	// para las pruebas.
	WhatsAppGraphURL string

	// DragonflyURL (redis://...) es el pub/sub EFÍMERO para el fan-out
	// cross-réplica de typing/presencia. Vacía => deshabilitado: los efímeros
	// quedan intra-réplica (Postgres sigue siendo la única fuente durable).
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string

	// AllowedOrigins son los patrones de origen aceptados en el handshake WS.
	AllowedOrigins []string

	// MaxMessageBytes acota el tamaño de un mensaje (texto) entrante.
	MaxMessageBytes int
	// MessagePageSize es el tamaño de página por defecto al listar mensajes.
	MessagePageSize int
	// PresenceTTL es la ventana en la que un usuario se considera "en línea"
	// tras su último heartbeat (presencia efímera, sin Redis: en memoria + NOTIFY).
	PresenceTTL time.Duration

	// ScheduledScanInterval es el periodo del worker de mensajes programados que
	// drena los vencidos (send_at <= now). <= 0 lo deshabilita.
	ScheduledScanInterval time.Duration

	// HookRatePerSecond y HookRateBurst configuran el rate-limit (token-bucket sobre
	// Postgres) del endpoint público de webhooks entrantes POST /hooks/{token}, por
	// token+IP (Ola 15). Acotan el sondeo de tokens y el abuso.
	HookRatePerSecond float64
	HookRateBurst     int

	// InternalSecret autentica el endpoint INTERNO service-to-service
	// POST /internal/call-ended (Sytue Rooms avisa de que una llamada ligada a un canal
	// terminó, para publicar la tarjeta «Llamada finalizada · duración»). NO es público
	// (no /api/v1, sin JWT): el secreto compartido —cabecera Authorization: Bearer— ES la
	// autorización, comparado en tiempo constante. Vacío => el endpoint no se monta. Debe
	// coincidir con el de Sytue Drive, que consulta la verja de los adjuntos de canal.
	// Nunca en git ni en logs. Se lee de ROOMS_INTERNAL_SECRET y, por compatibilidad con
	// los despliegues anteriores al rename, de CHAT_INTERNAL_SECRET.
	InternalSecret string

	// VAPID configura el Web Push (Ola 18): par de claves ECDSA P-256 en base64url sin
	// relleno (pública = punto sin comprimir de 65 bytes = applicationServerKey del
	// navegador; privada = escalar de 32 bytes) y el contacto `sub` (mailto:) del JWT
	// VAPID. Ambas vacías => push deshabilitado. Genéralas con openssl (ver webpush.go)
	// y guárdalas FUERA de git (Secret Manager/Sealed Secrets). Sin secretos en logs.
	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string
	// APNs (aviso «solo despertar» de la app nativa de iOS, ver service/apns.go).
	// APNSKeyP8 es el fichero .p8 ENTERO que descarga Apple (PEM, PKCS#8); los otros
	// tres los da el portal de desarrollador. Los cuatro vacíos => iOS sin push; a
	// medias => error de arranque. APNSHost apunta al servidor de pruebas
	// (api.sandbox.push.apple.com) cuando la app es una compilación de desarrollo.
	// La clave es un SECRETO: fuera de git y nunca en logs.
	APNSKeyP8    string
	APNSKeyID    string
	APNSTeamID   string
	APNSBundleID string
	APNSHost     string
	// KeyEncryptionKey es la KEK AES-256 (32 bytes ya decodificados) con la que se
	// cifran en reposo los secretos de FIRMA de las integraciones salientes
	// (outgoing_webhooks y slash_commands). Vacía = sin cifrado: se admite en
	// desarrollo, pero entonces crear una integración con secreto falla en vez de
	// escribirlo en claro. Origen: SYTUE_ROOMS_KEY_ENCRYPTION_KEY (base64).
	KeyEncryptionKey []byte

	// ChannelMailDomain es el dominio del CORREO A CANAL: escribir a
	// `<canal>+<token>@<dominio>` publica en el canal del webhook entrante. Quien lo
	// recibe y entrega es Sytue Mail (su CHANNEL_MAIL_DOMAIN, el MISMO valor); aquí solo
	// se PUBLICA, con el secreto de cada entrante, para que la web y las apps nativas
	// enseñen la dirección cuando de verdad funciona. Vacío = apagado: no se ofrece.
	ChannelMailDomain string

	// Hub configura la emisión de notificaciones unificadas hacia Sytue Hub.
	HubBaseURL        string
	HubInternalSecret string
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

	origins := l.GetCSV("WS_ALLOWED_ORIGINS")
	if len(origins) == 0 {
		origins = strings.Split(pcfg.FirstPartyWebSocketHosts(base.Env), ",")
	}

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://rooms.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-chat"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		DriveURL:                   l.GetDefault("DRIVE_URL", "http://drive:8082"),
		IntegrationsURL:            l.GetDefault("INTEGRATIONS_URL", "http://integrations:8093"),
		IntegrationsInternalSecret: l.Get("INTEGRATIONS_INTERNAL_SECRET"),
		MatrixInternalURL:          l.Get("SYTUE_ROOMS_MATRIX_INTERNAL_URL"),
		MatrixSourceURL:            l.Get("MATRIX_SOURCE_URL"),
		MatrixHomeserverURL:        l.Get("SYTUE_ROOMS_MATRIX_HOMESERVER_URL"),
		MatrixServerName:           l.GetDefault("SYTUE_ROOMS_MATRIX_SERVER_NAME", "matrix.sytue.eu"),
		MatrixSSOProviderID:        l.GetDefault("SYTUE_ROOMS_MATRIX_SSO_PROVIDER_ID", "oidc-sytue"),
		MatrixAdminToken:           l.Get("SYTUE_ROOMS_MATRIX_ADMIN_TOKEN"),
		MatrixRequired:             l.GetBoolDefault("SYTUE_ROOMS_MATRIX_REQUIRED", true),
		MatrixLifecycleSecret:      l.Get("SYTUE_ROOMS_MATRIX_LIFECYCLE_SECRET"),
		MatrixIdentitySecret:       l.Get("SYTUE_ROOMS_MATRIX_IDENTITY_SECRET"),
		DocumentCallAccessSecret:   l.Get("SYTUE_ROOMS_DOCUMENT_CALL_ACCESS_SECRET"),
		EncryptedDocumentCalls:     l.GetBoolDefault("SYTUE_ROOMS_ENCRYPTED_DOCUMENT_CALLS", false),
		IDURL:                      l.GetDefault("ID_URL", "http://id:8081"),
		IDNombresOrgsSecret:        l.Get("ID_NOMBRES_ORGS_SECRET"),

		WhatsAppGraphURL: strings.TrimRight(l.GetDefault("WHATSAPP_GRAPH_URL", "https://graph.facebook.com/v23.0"), "/"),

		DragonflyURL:      l.Get("DRAGONFLY_URL"),
		DragonflyPassword: l.Get("DRAGONFLY_PASSWORD"),

		AllowedOrigins:  origins,
		MaxMessageBytes: l.GetIntDefault("MAX_MESSAGE_KB", 64) * 1024,
		MessagePageSize: l.GetIntDefault("MESSAGE_PAGE_SIZE", 50),
		PresenceTTL:     l.GetDurationDefault("PRESENCE_TTL", 45*time.Second),

		ScheduledScanInterval: l.GetDurationDefault("SCHEDULED_SCAN_INTERVAL", 20*time.Second),

		// Un webhook entrante legítimo publica a ráfagas moderadas; 2/s sostenido con
		// pico de 20 basta y frena el sondeo de tokens (la clave incluye token+IP).
		HookRatePerSecond: float64(l.GetIntDefault("HOOK_RATE_PER_SEC", 2)),
		HookRateBurst:     l.GetIntDefault("HOOK_RATE_BURST", 20),

		// Secreto del endpoint interno chat←meet (tarjeta «llamada finalizada»). Vacío
		// => no se monta. Fuera de git (Secret Manager/Sealed Secrets); igual en Meet.
		InternalSecret: firstNonEmpty(l.Get("ROOMS_INTERNAL_SECRET"), l.Get("CHAT_INTERNAL_SECRET")),

		// Web Push (Ola 18): par VAPID por entorno (nunca en git). Vacío => deshabilitado.
		VAPIDPublic:  l.Get("SYTUE_VAPID_PUBLIC"),
		VAPIDPrivate: l.Get("SYTUE_VAPID_PRIVATE"),
		VAPIDSubject: l.GetDefault("SYTUE_VAPID_SUBJECT", "mailto:ops@sytue.eu"),
		// APNs: iOS no se puede despertar de otra forma. Vacío => deshabilitado.
		APNSKeyP8:    l.Get("SYTUE_APNS_KEY_P8"),
		APNSKeyID:    l.Get("SYTUE_APNS_KEY_ID"),
		APNSTeamID:   l.Get("SYTUE_APNS_TEAM_ID"),
		APNSBundleID: l.Get("SYTUE_APNS_BUNDLE_ID"),
		APNSHost:     l.Get("SYTUE_APNS_HOST"),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret: l.HubInternalSecret(base.Env),

		// Normalizado como lo normaliza Mail (`NewChannelBridge`): una variable copiada
		// con «@» o un punto final no puede ofrecer una dirección que Mail no reconoce.
		ChannelMailDomain: strings.ToLower(strings.Trim(strings.TrimSpace(l.Get("CHANNEL_MAIL_DOMAIN")), "@.")),
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8085"
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

	// KEK de los secretos de firma (base64 → 32 bytes AES-256). Fallo rápido si está
	// mal puesta, en vez de arrancar con el cifrado roto y no enterarse.
	if raw := l.Get("SYTUE_ROOMS_KEY_ENCRYPTION_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SYTUE_ROOMS_KEY_ENCRYPTION_KEY: base64 inválido: %w", err)
		}
		if len(key) != 32 {
			return Config{}, fmt.Errorf("SYTUE_ROOMS_KEY_ENCRYPTION_KEY: debe medir 32 bytes (AES-256), tiene %d", len(key))
		}
		cfg.KeyEncryptionKey = key
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.EncryptedDocumentCalls && c.DocumentCallAccessSecret == "" {
		return fmt.Errorf("encrypted document calls require SYTUE_ROOMS_DOCUMENT_CALL_ACCESS_SECRET")
	}
	if c.MaxMessageBytes <= 0 {
		return fmt.Errorf("config: MAX_MESSAGE_KB debe ser > 0")
	}
	if c.MessagePageSize <= 0 || c.MessagePageSize > 200 {
		return fmt.Errorf("config: MESSAGE_PAGE_SIZE debe estar en (0,200]")
	}
	return nil
}

// firstNonEmpty devuelve el primer valor no vacío. Sostiene el rename de las variables
// de entorno sin romper un despliegue a medio actualizar.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
