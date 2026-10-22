// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Drive desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable definidos en deploy/compose
// (PORT, DATABASE_URL, OIDC_ISSUER, OIDC_JWKS_URL, S3_*). Sin secretos en git.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/correo"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-drive"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// DevCastInternalSecret es el secreto de desarrollo de la puerta por la que Sytue
// Cast declara su uso de la cuota. Está escrito aquí, así que es público: fuera de
// dev no vale. Tiene que ser EL MISMO que services/cast/internal/config.DevInternalSecret
// —Go no deja importar el `internal` de otro servicio, por eso está repetido— y
// TestDevCastInternalSecretEsElDeCast lo vigila leyendo aquel fichero.
const DevCastInternalSecret = "dev-cast-internal-secret" //nolint:gosec // G101: es el de desarrollo, público a propósito

// Config es la configuración completa de Sytue Drive.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://drive.sytue.eu).
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
	// contiene ALGUNA (allowlist first-party).
	OIDCAcceptedAudiences []string

	// IDBaseURL es la URL INTERNA de Sytue ID. Drive la usa SOLO al etiquetar un
	// fichero: resuelve el id de etiqueta contra la taxonomía viva de la organización
	// (GET /api/v1/sensitivity-labels) y guarda el snapshot resultante. Camino FRÍO;
	// el camino caliente (compartir, descargar) decide con el snapshot local y nunca
	// llama aquí. Mismo default que services/integrations.
	IDBaseURL string

	// RoomsURL / RoomsInternalSecret configuran la VERJA de los adjuntos de canal de
	// Sytue Rooms: Drive pregunta a services/rooms si el usuario sigue siendo miembro
	// del canal (GET /internal/channel-access) en cada descarga/miniatura/subida, con
	// un secreto compartido. Sin secreto configurado esa superficie queda CERRADA
	// (fail-closed): el resto de Drive no se ve afectado.
	RoomsURL            string
	RoomsInternalSecret string

	// DocsURL es la raíz INTERNA de Sytue Docs: ahí se entrega lo que le pasa en la
	// papelera al fichero de un documento (internal/docssync, mig 0027). Vacía, no se
	// entrega nada.
	DocsURL string
	// BasesURL es la raíz INTERNA de Sytue Bases: destino fijo de «exportar una vista a una
	// carpeta». El token de una conexión no se manda a ningún otro sitio.
	BasesURL string
	// DocsInternalSecret es el secreto PROPIO de las dos puertas entre Docs y Drive
	// (la cola hacia Docs y /internal/docs/files): no el de la suite, que tienen
	// todos los servicios, porque estas borran. Vacío, las dos quedan cerradas.
	DocsInternalSecret string

	// CastInternalSecret protege la puerta por la que Sytue Cast declara lo que
	// ocupa en cada organización (PUT /internal/quota/external/cast, mig 0026): la
	// cuota del plan es una para toda la suite y Drive es su dueño. OPCIONAL: vacío,
	// la ruta existe y no deja pasar a nadie (httpx.SecretoInterno rechaza con
	// secreto vacío) y la cuota sigue contando solo lo de Drive. El MISMO valor vive
	// en el Secret sytue-cast; en dev los dos extremos caen al de desarrollo.
	CastInternalSecret string

	// IDInternalSecret abre el WebDAV con Basic y una contraseña de aplicación (el
	// gestor de ficheros de Linux, «Archivos» de Android): Drive la canjea en Sytue ID
	// (IDBaseURL + /internal/app-passwords/token) por un token de la persona. Es el
	// MISMO valor que SYTUE_ID_INTERNAL_SECRET. Vacío, /dav solo admite el Bearer.
	IDInternalSecret string

	// Hub configura la emisión de notificaciones unificadas hacia Sytue Hub.
	// Correo es el buzón de los avisos por correo (avisos@<dominio>), el de
	// «X pide editar» (peticiones.go). Sin remitente no sale ninguno.
	Correo            correo.Config
	HubBaseURL        string
	HubInternalSecret string

	// S3 configura el Object Storage de blobs (drive-blobs).
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	// S3Remote es el almacén remoto (S3_REMOTE_*), donde va todo tenant salvo
	// los de S3LocalTenants; nil = todo a S3_*. Ver objectstore.Remote (ADR 0038).
	S3Remote       *objectstore.Config
	S3LocalTenants []string

	// WopiKey es la KEK con la que se sellan los tokens de acceso WOPI (32 bytes en
	// base64). VACÍA ⇒ la capa WOPI queda APAGADA: sin clave no se pueden acuñar tokens
	// opacos y autenticados, y emitirlos sin cifrar sería peor que no ofrecer el
	// protocolo. Ver internal/wopi.
	WopiKey string

	// ExternalStorageKey es la KEK (32 bytes en base64) con la que se sella la
	// clave secreta del almacén S3 de terceros de cada tenant (mig 0023). VACÍA ⇒
	// la función queda APAGADA: guardar credenciales ajenas sin sellar no es una
	// opción, así que o se sellan o no se aceptan. Mismo criterio que WopiKey.
	ExternalStorageKey string
	// IntegrationsURL / IntegrationsInternalSecret: el servicio de integraciones
	// presta el token OAuth de Google Drive u OneDrive (almacenamiento conectado
	// por consentimiento). Sin secreto, esas dos clases no están disponibles.
	IntegrationsURL            string
	IntegrationsInternalSecret string

	// Túnel de Sytue Cloud para un NAS en las instalaciones del cliente (clase
	// `nas` del almacén de terceros): Drive crea un sitio y un recurso en el
	// proxy de la plataforma (Pangolin) por su API de integración y le da al
	// cliente el agente a lanzar en su NAS. Sin llave, la clase no está.
	TunnelAPIURL string // p. ej. https://pangolin-api.sytue.eu/v1
	TunnelAPIKey string
	TunnelOrg    string // organización en el proxy (sytue-projects)
	// TunnelDomain es el dominio de CONTENIDO DE CLIENTES (SYTUE_CLIENT_CONTENT_DOMAIN,
	// `global.dominioDeClientes`), registrable aparte del de la suite: un NAS de
	// cliente publicado como nas-<tenant>.sytue.eu podía poner cookies a toda la
	// suite (FILE-09, NEG-24). TunnelDomainID es el id de ESE dominio en el proxy.
	// Sin los dos, la clase NAS no se ofrece.
	TunnelDomainID string
	TunnelDomain   string
	TunnelEndpoint string // a dónde se conecta el agente (https://pangolin.sytue.eu)

	// WopiDiscoveryURL es el `discovery.xml` del editor que abrirá los ficheros
	// (Collabora Online, Office for the web…). Lo pone quien despliega: la suite no
	// incorpora motores AGPL. VACÍA ⇒ Drive sirve el protocolo pero no ofrece «abrir en
	// el editor», porque sin el catálogo del editor no se sabe con qué URL se abre cada
	// tipo de fichero y adivinarla daría una pantalla en blanco.
	WopiDiscoveryURL string
	// WopiDiscoveryTTL es cada cuánto se relee ese catálogo (solo cambia cuando el
	// editor se actualiza).
	WopiDiscoveryTTL time.Duration

	// MaxUploadBytes limita el tamaño de subida directa de un fichero.
	MaxUploadBytes int64
	// PresignTTL es la validez de las URLs prefirmadas de descarga/subida.
	PresignTTL time.Duration
	// DefaultQuotaBytes es la cuota por defecto de un nuevo tenant (bytes).
	DefaultQuotaBytes int64

	// TrashRetention es cuánto permanece un elemento en la papelera antes de que el
	// worker de auto-purga lo borre DEFINITIVAMENTE (deleted_at < now()-retención).
	// <= 0 desactiva la auto-purga (la papelera se conserva indefinidamente).
	TrashRetention time.Duration
	// TrashGCInterval es el periodo del worker de auto-purga de papelera. <= 0
	// desactiva el worker.
	TrashGCInterval time.Duration
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
	// Si VERSION no viene del entorno, usa la versión inyectada por el linker.
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://drive.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-drive"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		IDBaseURL:        l.GetDefault("ID_URL", "http://id:8081"),
		IDInternalSecret: l.Get("ID_INTERNAL_SECRET"),

		// ROOMS_* son los nombres actuales; los CHAT_* del rename anterior se aceptan
		// como respaldo para no romper un despliegue a medio migrar. El default apunta
		// al servicio `rooms`: `chat` ya no resuelve, y con él la verja de adjuntos de
		// canal fallaba CERRADA sin que nada lo dijera.
		RoomsURL:            firstNonEmpty(l.Get("ROOMS_URL"), l.Get("CHAT_URL"), "http://rooms:8085"),
		RoomsInternalSecret: firstNonEmpty(l.Get("ROOMS_INTERNAL_SECRET"), l.Get("CHAT_INTERNAL_SECRET")),

		DocsURL:  l.GetDefault("DOCS_URL", "http://docs:8083"),
		BasesURL: l.GetDefault("BASES_URL", "http://bases:8102"),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		Correo:            l.Correo(),
		HubInternalSecret: l.HubInternalSecret(base.Env),

		S3Endpoint:  l.GetDefault("S3_ENDPOINT", "minio:9000"),
		S3Region:    l.GetDefault("S3_REGION", "fr-par"),
		S3AccessKey: l.Require("S3_ACCESS_KEY"),
		S3SecretKey: l.Require("S3_SECRET_KEY"),
		S3Bucket:    l.GetDefault("S3_BUCKET", "sytue-dev-drive-blobs"),
		S3UseSSL:    l.GetBoolDefault("S3_USE_SSL", false),
		S3Remote: objectstore.Remote(objectstore.Config{
			Endpoint:     l.Get("S3_REMOTE_ENDPOINT"),
			Region:       l.GetDefault("S3_REMOTE_REGION", "fr-par"),
			AccessKey:    l.Get("S3_REMOTE_ACCESS_KEY"),
			SecretKey:    l.Get("S3_REMOTE_SECRET_KEY"),
			Bucket:       l.Get("S3_REMOTE_BUCKET"),
			UseSSL:       l.GetBoolDefault("S3_REMOTE_USE_SSL", true),
			StorageClass: l.GetDefault("S3_REMOTE_STORAGE_CLASS", "ONEZONE_IA"),
		}),
		S3LocalTenants: l.GetCSV("S3_LOCAL_TENANTS"),

		WopiKey:                    l.Get("WOPI_KEY"),
		ExternalStorageKey:         l.Get("EXTERNAL_STORAGE_KEY"),
		IntegrationsURL:            l.GetDefault("INTEGRATIONS_URL", "http://integrations"),
		IntegrationsInternalSecret: l.Get("INTEGRATIONS_INTERNAL_SECRET"),
		TunnelAPIURL:               l.Get("TUNNEL_API_URL"),
		TunnelAPIKey:               l.Get("TUNNEL_API_KEY"),
		TunnelOrg:                  l.GetDefault("TUNNEL_ORG", "sytue-projects"),
		TunnelDomainID:             l.Get("TUNNEL_DOMAIN_ID"),
		TunnelDomain:               l.Get("SYTUE_CLIENT_CONTENT_DOMAIN"),
		TunnelEndpoint:             l.GetDefault("TUNNEL_ENDPOINT", "https://pangolin.sytue.eu"),
		WopiDiscoveryURL:           l.Get("WOPI_DISCOVERY_URL"),
		WopiDiscoveryTTL:           l.GetDurationDefault("WOPI_DISCOVERY_TTL", time.Hour),
		MaxUploadBytes:             int64(l.GetIntDefault("MAX_UPLOAD_MB", 5120)) * 1024 * 1024,
		PresignTTL:                 l.GetDurationDefault("PRESIGN_TTL", 15*time.Minute),
		DefaultQuotaBytes:          int64(l.GetIntDefault("DEFAULT_QUOTA_GB", 10)) * 1024 * 1024 * 1024,

		// Auto-purga de papelera: 30 días de retención, barrido cada hora (defaults
		// sensatos, paridad Google Drive). Cadencia horaria: la purga no es urgente y
		// el lote por ciclo acota el trabajo.
		TrashRetention:  l.GetDurationDefault("TRASH_RETENTION", 30*24*time.Hour),
		TrashGCInterval: l.GetDurationDefault("TRASH_GC_INTERVAL", time.Hour),
	}

	cfg.CastInternalSecret = l.SecretoOpcional(base.Env, "CAST_INTERNAL_SECRET", DevCastInternalSecret)
	cfg.DocsInternalSecret = l.SecretoOpcional(base.Env, "DOCS_INTERNAL_SECRET", pcfg.DevDocsDriveInternalSecret)

	// El endpoint S3 del compose viene como URL (http://minio:9000): el cliente
	// minio espera solo host:port + flag de TLS, así que lo normalizamos.
	cfg.S3Endpoint, cfg.S3UseSSL = pcfg.NormalizeS3Endpoint(cfg.S3Endpoint, cfg.S3UseSSL)

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8082"
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
	if c.MaxUploadBytes <= 0 {
		return fmt.Errorf("config: MAX_UPLOAD_MB debe ser > 0")
	}
	if c.DefaultQuotaBytes < 0 {
		return fmt.Errorf("config: DEFAULT_QUOTA_GB no puede ser negativo")
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
