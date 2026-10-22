// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Hub desde el entorno del
// proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable del compose (PORT, DATABASE_URL,
// OIDC_*, *_BASE_URL de los servicios upstream). Sin secretos en git.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-hub"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// Config es la configuración completa de Sytue Hub.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://hub.sytue.eu).
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Fuente de verdad durable
	// (notificaciones, outbox; y presencia cuando no hay Dragonfly).
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// DragonflyURL es la URL de la cache efímera (redis://dragonfly:6379), usada
	// para la presencia como claves con TTL. Opcional: vacía => la presencia
	// sigue en PostgreSQL (degradación sin pérdida de funcionalidad).
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string

	// OIDC configura la validación de tokens de Sytue ID (JWKS), client sytue-hub.
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences es la allowlist de audiencias aceptadas: la Sytue Shell
	// llama al Hub desde cada app con el token de ESE app (sytue-one, sytue-mail…),
	// no con uno específico del Hub. El token es válido si su aud contiene alguna.
	OIDCAcceptedAudiences []string

	// Upstreams son las URLs base de los servicios consultados por la búsqueda
	// federada y los intents cross-app. Cada conector llama a su API REST propia.
	DriveBaseURL    string
	MailBaseURL     string
	CalendarBaseURL string
	ContactsBaseURL string
	NotesBaseURL    string
	RoomsBaseURL    string
	// Solo para la exportación de datos (GET /api/v1/export): no se buscan, pero
	// guardan datos de la persona que se lleva lo suyo.
	DocsBaseURL   string
	PhotosBaseURL string
	IDBaseURL     string
	ERPBaseURL    string
	// CastBaseURL es Sytue Cast (vídeo bajo demanda y directo): entrega sus vídeos y
	// comentarios en la exportación.
	CastBaseURL string
	// PagesBaseURL es Sytue Web (webs y tiendas): entrega los sitios, versiones y
	// trabajos que creó la persona.
	PagesBaseURL string
	// BasesBaseURL es Sytue Bases (tablas relacionales): federa la búsqueda de bases.
	BasesBaseURL string
	// IntegrationsBaseURL es la URL interna del Sytue Integration Gateway, cuyo
	// endpoint de búsqueda federa el contenido de los SaaS conectados de la org.
	IntegrationsBaseURL string
	// AIBaseURL es la URL interna del servicio de IA, que traduce una frase a
	// parámetros de búsqueda. VACÍA POR DEFECTO, y a propósito: sin ella la
	// búsqueda funciona exactamente como hasta ahora, y una instalación que no
	// despliegue el asistente no arranca con un upstream que no existe.
	//
	// La variable se llama SYTUE_AI_URL y no AI_BASE_URL, que sería lo que pide
	// la convención de aquí (`DRIVE_BASE_URL`, `NOTES_BASE_URL`): dentro del
	// contenedor del propio servicio de IA, `AI_BASE_URL` ya significa otra cosa
	// —el proveedor de inferencia de fuera—, y dos variables con el mismo nombre
	// y distinto significado en el mismo chart se confunden a la primera. Es
	// además el nombre que ya usa el BFF de cada app para lo mismo.
	AIBaseURL string

	// PublicBaseURLs son las URLs de cara al usuario de cada superficie, usadas
	// para construir deep-links de los intents (no las APIs internas).
	DrivePublicURL    string
	MailPublicURL     string
	CalendarPublicURL string
	ContactsPublicURL string
	NotesPublicURL    string
	RoomsPublicURL    string
	ERPPublicURL      string
	PagesPublicURL    string
	BasesPublicURL    string

	// SearchTimeout acota cada fan-out de búsqueda a un conector.
	SearchTimeout time.Duration

	// AppsDesplegadas enumera los productos que ESTA instalación levanta
	// («one,mail,drive,…»). El frontal lo consulta para no ofrecer teselas que
	// llevan a un dominio que aquí no existe: el perfil piloto, por ejemplo, no
	// despliega Sytue Cloud, y su acceso seguía en el conmutador de las once
	// aplicaciones y en el Launcher del portal.
	//
	// VACÍA significa «no se sabe», no «no hay nada»: el frontal presenta la suite
	// entera, que es como se ha comportado siempre. Apagar una tesela tiene que
	// ser una decisión escrita en el despliegue, no lo que pasa cuando falta una
	// variable —o cuando el Hub aún no se ha actualizado—.
	AppsDesplegadas []string
	// SearchLimit es el máximo de resultados devueltos por conector.
	SearchLimit int
	// UpstreamTimeout es el timeout por defecto de las llamadas HTTP a upstreams.
	UpstreamTimeout time.Duration

	// PresenceTTL es la ventana tras la que un heartbeat se considera obsoleto
	// (el usuario pasa a offline si no renueva).
	PresenceTTL time.Duration
	// NotifyRetention es la retención de notificaciones leídas antes de purga.
	NotifyRetention time.Duration

	// CORSAllowedOrigins es la allowlist de orígenes del navegador que llaman al
	// Hub cross-origin (la Sytue Shell corre en TODOS los subdominios de la suite).
	// Admite comodín de subdominio ("https://*.sytue.eu"). Configurable por entorno
	// para restringir en prod al apex real.
	CORSAllowedOrigins []string

	// InternalSecret es el secreto compartido para llamadas internas service-to-service
	// (POST /internal/notifications).
	InternalSecret string
	// IDNombresOrgsSecret abre `POST /internal/orgs/names` de Sytue ID: el nombre
	// visible de las organizaciones de fuera que avisan a la persona. Vacío ⇒ la
	// campana enseña su identificador.
	IDNombresOrgsSecret string

	// Descargas es el bucket de los instaladores nativos, el MISMO que sirve el portal
	// (`apps/frontend/apps/portal/src/lib/descargas.ts`) y con la misma credencial de
	// solo lectura. Lo pide el actualizador de las apps de escritorio, que hablan con el
	// Hub con su token y no tienen la cookie del portal. Sin bucket o sin credencial,
	// `/api/v1/instaladores` responde 503 y las apps siguen como están.
	Descargas Descargas
}

// Descargas configura la lectura del bucket de instaladores.
type Descargas struct {
	// Endpoint es el S3 que ve el Hub. Admite esquema (`https://s3.fr-par.scw.cloud`,
	// `http://seaweedfs…:8333`), como la variable del portal.
	Endpoint string
	// EndpointPublico es con el que se FIRMA la URL de descarga cuando el de arriba solo
	// existe dentro del clúster. Vacío: se firma con Endpoint si es https; si no, la app
	// baja el instalador por el propio Hub.
	EndpointPublico string
	Region          string
	Bucket          string
	// Prefijo es la carpeta que llena `traer-instaladores.sh` (`latest/`).
	Prefijo              string
	AccessKey, SecretKey string
}

// Configurada dice si hay de dónde leer los instaladores.
func (d Descargas) Configurada() bool {
	return d.Endpoint != "" && d.Bucket != "" && d.AccessKey != "" && d.SecretKey != ""
}

// Load construye la configuración leyendo el entorno. Devuelve todos los errores
// de validación juntos.
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
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://hub.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		DragonflyURL:      l.Get("DRAGONFLY_URL"),
		DragonflyPassword: l.Get("DRAGONFLY_PASSWORD"),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-hub"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		DriveBaseURL:    l.GetDefault("DRIVE_BASE_URL", "http://drive:8082"),
		MailBaseURL:     l.GetDefault("MAIL_BASE_URL", "http://mail:8086"),
		CalendarBaseURL: l.GetDefault("CALENDAR_BASE_URL", "http://calendar:8087"),
		ContactsBaseURL: l.GetDefault("CONTACTS_BASE_URL", "http://contacts:8088"),
		NotesBaseURL:    l.GetDefault("NOTES_BASE_URL", "http://notes:8089"),
		// ROOMS_*: el servicio se llama `rooms` desde que mensajería y videollamada son
		// uno. Se acepta el nombre viejo para no romper un despliegue a medio migrar.
		RoomsBaseURL: firstNonEmpty(l.Get("ROOMS_BASE_URL"), l.Get("CHAT_BASE_URL"), "http://rooms:8085"),
		ERPBaseURL:   firstNonEmpty(l.Get("ERP_BASE_URL"), l.Get("SYERP_BASE_URL"), "http://syerp:8098"),

		DocsBaseURL:   l.GetDefault("DOCS_BASE_URL", "http://docs:8083"),
		PhotosBaseURL: l.GetDefault("PHOTOS_BASE_URL", "http://syphotos:8097"),
		IDBaseURL:     l.GetDefault("ID_BASE_URL", "http://id:8081"),
		CastBaseURL:   l.GetDefault("CAST_BASE_URL", "http://cast:8099"),
		PagesBaseURL:  l.GetDefault("PAGES_BASE_URL", "http://pages:8100"),
		BasesBaseURL:  l.GetDefault("BASES_BASE_URL", "http://bases:8102"),

		IntegrationsBaseURL: l.GetDefault("INTEGRATIONS_BASE_URL", "http://integrations:8093"),
		AIBaseURL:           l.GetDefault("SYTUE_AI_URL", ""),

		DrivePublicURL:    l.GetDefault("DRIVE_PUBLIC_URL", "https://drive.sytue.localhost/drive"),
		MailPublicURL:     l.GetDefault("MAIL_PUBLIC_URL", "https://mail.sytue.localhost/mail"),
		CalendarPublicURL: l.GetDefault("CALENDAR_PUBLIC_URL", "https://calendar.sytue.localhost/calendar"),
		ContactsPublicURL: l.GetDefault("CONTACTS_PUBLIC_URL", "https://contacts.sytue.localhost/contacts"),
		NotesPublicURL:    l.GetDefault("NOTES_PUBLIC_URL", "https://notes.sytue.localhost/notes"),
		RoomsPublicURL:    firstNonEmpty(l.Get("ROOMS_PUBLIC_URL"), l.Get("CHAT_PUBLIC_URL"), "https://rooms.sytue.localhost/rooms"),
		ERPPublicURL:      l.GetDefault("ERP_PUBLIC_URL", "https://erp.sytue.localhost/erp"),
		PagesPublicURL:    l.GetDefault("PAGES_PUBLIC_URL", "https://web.sytue.localhost/web"),
		BasesPublicURL:    l.GetDefault("BASES_PUBLIC_URL", "https://bases.sytue.localhost/bases"),

		SearchTimeout:   l.GetDurationDefault("SEARCH_TIMEOUT", 2500*time.Millisecond),
		AppsDesplegadas: listaDeApps(l.Get("APPS")),
		SearchLimit:     l.GetIntDefault("SEARCH_LIMIT", 20),
		UpstreamTimeout: l.GetDurationDefault("UPSTREAM_TIMEOUT", 5*time.Second),

		PresenceTTL:     l.GetDurationDefault("PRESENCE_TTL", 45*time.Second),
		NotifyRetention: l.GetDurationDefault("NOTIFY_RETENTION", 720*time.Hour),

		CORSAllowedOrigins:  l.GetCSVDefault("CORS_ALLOWED_ORIGINS", pcfg.FirstPartyCORSOrigins(base.Env)),
		InternalSecret:      l.HubInternalSecret(base.Env),
		IDNombresOrgsSecret: l.Get("ID_NOMBRES_ORGS_SECRET"),

		Descargas: Descargas{
			Endpoint:        strings.TrimSpace(l.Get("DOWNLOADS_ENDPOINT")),
			EndpointPublico: strings.TrimSpace(l.Get("DOWNLOADS_PUBLIC_ENDPOINT")),
			Region:          l.GetDefault("DOWNLOADS_REGION", "fr-par"),
			Bucket:          strings.TrimSpace(l.Get("DOWNLOADS_BUCKET")),
			Prefijo:         l.GetDefault("DOWNLOADS_PREFIX", "latest/"),
			AccessKey:       strings.TrimSpace(l.Get("DOWNLOADS_ACCESS_KEY")),
			SecretKey:       strings.TrimSpace(l.Get("DOWNLOADS_SECRET_KEY")),
		},
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8092"
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
	if c.SearchLimit <= 0 || c.SearchLimit > 200 {
		return fmt.Errorf("config: SEARCH_LIMIT debe estar en 1..200")
	}
	if c.PresenceTTL <= 0 {
		return fmt.Errorf("config: PRESENCE_TTL debe ser > 0")
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

// listaDeApps parte «one, mail ,drive» en claves limpias. Tolera espacios y
// entradas vacías porque esto lo escribe una plantilla de Helm o un script de
// despliegue, donde una coma de más es lo normal.
func listaDeApps(v string) []string {
	var out []string
	for _, parte := range strings.Split(v, ",") {
		if clave := strings.ToLower(strings.TrimSpace(parte)); clave != "" {
			out = append(out, clave)
		}
	}
	return out
}
