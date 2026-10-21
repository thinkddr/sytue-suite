// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Cast desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta los nombres de variable del compose (PORT, DATABASE_URL, OIDC_ISSUER,
// OIDC_JWKS_URL, OIDC_AUDIENCE, S3_*). Sin secretos en git.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-cast"

// DefaultPort es el puerto HTTP de Sytue Cast (CAST-ENCARGO §2, CONVENTIONS §6):
// el siguiente libre tras SyERP (8098).
const DefaultPort = "8099"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Secretos de desarrollo. Están escritos aquí, así que son públicos: fuera de dev
// no valen y el servicio no arranca con ellos.
const (
	DevMediaSigningKey = "dev-cast-media-signing-key" //nolint:gosec // G101: es el de desarrollo, público a propósito
	DevInternalSecret  = "dev-cast-internal-secret"   //nolint:gosec // G101: ídem
)

// MinSweepStaleUpload es lo mínimo que se admite en CAST_SWEEP_STALE_UPLOAD: más
// que el plazo de una copia desde Drive (plazoDeCopia, 2 h, en service/drive.go),
// que durante toda la copia deja el vídeo en `uploading` sin tocarlo.
const MinSweepStaleUpload = 3 * time.Hour

// Config es la configuración completa de Sytue Cast.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://cast.sytue.eu). Con
	// ella se componen el `<iframe>` de oEmbed y los enlaces de los avisos.
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión, así que el token puede traer
	// la audiencia de OTRA app. Se acepta si su aud contiene ALGUNA.
	OIDCAcceptedAudiences []string

	// TrustedProxies son las redes cuyo X-Forwarded-For se cree (TRUSTED_PROXIES):
	// la IP de quien denuncia sin cuenta por la web la trae el BFF en esa cabecera,
	// y solo vale si llega desde una de ellas. Un cliente que llama directo con su
	// propia cabecera se queda con la IP desde la que conecta.
	TrustedProxies clientip.TrustedProxies
	// IDURL es Sytue ID por dentro: de ahí sale quién es «@ana» cuando se comparte
	// un vídeo con personas concretas (people:resolve, ADR 0040, §9). Vacía = no se
	// comparte con nadie.
	IDURL string
	// DriveURL es Drive por dentro: la cuota del plan (§8). Vacía = sin cuota.
	//
	// El Hub (avisos de «está en directo», §6) lo trae el paquete de directo con su
	// chart: `HubInternalSecret` exige que el chart lo dé y aún no hay chart.
	DriveURL string

	// S3 es el bucket de Cast (`…cast-medios`): original, HLS, WebM y pósteres. Con
	// S3Remote, los tenants nuevos van al almacén remoto (ADR 0038), como en Photos.
	S3             objectstore.Config
	S3Remote       *objectstore.Config
	S3LocalTenants []string

	// MediaSigningKey firma los enlaces de /media (§5.1). Del Secret sytue-cast.
	MediaSigningKey string
	// InternalSecret es el secreto compartido con Drive: con él Cast declara su uso
	// en `PUT /internal/quota/external/cast` (§8). Del Secret sytue-cast; Drive lo
	// tiene como `cast-internal-secret`.
	InternalSecret string

	// AllowedOrigins son los orígenes admitidos en el handshake del WebSocket del chat,
	// además del host de la petición y el de la organización del token.
	AllowedOrigins []string

	// ChatCleanupInterval es cada cuánto se retira el chat más viejo que la
	// retención. 0 o negativo = no se limpia (la retención queda para otra réplica).
	ChatCleanupInterval time.Duration

	// El barrido periódico (service/barrido.go), en el proceso `cast`, una réplica
	// a la vez. SweepInterval: cada cuánto; 0 o negativo = no se barre.
	// SweepDeletedGrace: cuánto se conserva un vídeo borrado antes de purgarlo y
	// eliminar su fila. SweepStaleUpload: cuánto puede estar un vídeo en
	// `uploading` sin cambiar antes de darlo por fallido.
	SweepInterval     time.Duration
	SweepDeletedGrace time.Duration
	SweepStaleUpload  time.Duration
	// SweepBackfillQualities: hasta cuántos trabajos de calidades puede tener en
	// cola el relleno de los vídeos de antes de la escalera WebM
	// (CAST_SWEEP_BACKFILL_QUALITIES, 8 por defecto; 0 lo apaga).
	SweepBackfillQualities int

	// Transcoder: cuántos hilos da a ffmpeg y cuánto puede durar un trabajo (§7).
	TranscoderThreads    int
	TranscoderMaxMinutes int
	// TranscoderTmpDir es dónde se descargan los originales (el emptyDir del chart).
	TranscoderTmpDir string

	// Directo (§6). Sin LIVEKIT_URL el directo no se monta y sus rutas contestan
	// 503; con ella, las claves de la API son obligatorias. Las mismas del SFU de
	// Rooms (Secret sytue-livekit-keys).
	LiveKitURL       string
	LiveKitHost      string
	LiveKitAPIKey    string
	LiveKitAPISecret string
	// LiveKitPollInterval es cada cuánto se contrasta el estado con LiveKit por si
	// se pierde un webhook; 0 apaga el sondeo.
	LiveKitPollInterval time.Duration
	// IngestRTMPURL es el «servidor» que se pega en OBS (rtmp://cast-ingest.<dominio>/live).
	IngestRTMPURL string
	// Recording* es el destino S3 del egress que graba los directos: el bucket de
	// Cast. Por defecto, el mismo S3 del servicio; el endpoint va CON esquema, que
	// es como lo quiere el egress.
	RecordingBucket    string
	RecordingEndpoint  string
	RecordingRegion    string
	RecordingAccessKey string
	RecordingSecretKey string
	// HubBaseURL y HubInternalSecret: los avisos de «está en directo». Sin URL no
	// se avisa; con ella, el secreto es obligatorio fuera de desarrollo.
	HubBaseURL        string
	HubInternalSecret string
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
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://cast.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", ServiceName),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		// §9: ID_URL, o SYTUE_ID_INTERNAL_URL como en otros servicios.
		IDURL:    primero(l.Get("ID_URL"), l.Get("SYTUE_ID_INTERNAL_URL"), "http://id:8081"),
		DriveURL: l.GetDefault("DRIVE_URL", "http://drive:8082"),

		S3: objectstore.Config{
			Endpoint:     l.GetDefault("S3_ENDPOINT", "minio:9000"),
			Region:       l.GetDefault("S3_REGION", "fr-par"),
			AccessKey:    l.Require("S3_ACCESS_KEY"),
			SecretKey:    l.Require("S3_SECRET_KEY"),
			Bucket:       l.GetDefault("S3_BUCKET", "sytue-dev-cast-medios"),
			UseSSL:       l.GetBoolDefault("S3_USE_SSL", false),
			StorageClass: l.Get("S3_STORAGE_CLASS"),
		},
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

		AllowedOrigins:      l.GetCSV("ALLOWED_ORIGINS"),
		ChatCleanupInterval: l.GetDurationDefault("CHAT_CLEANUP_INTERVAL", time.Hour),

		SweepInterval:     l.GetDurationDefault("CAST_SWEEP_INTERVAL", time.Hour),
		SweepDeletedGrace: l.GetDurationDefault("CAST_SWEEP_DELETED_GRACE", 7*24*time.Hour),
		SweepStaleUpload:  l.GetDurationDefault("CAST_SWEEP_STALE_UPLOAD", 6*time.Hour),

		SweepBackfillQualities: l.GetIntDefault("CAST_SWEEP_BACKFILL_QUALITIES", 8),

		TranscoderThreads:    l.GetIntDefault("TRANSCODER_THREADS", 2),
		TranscoderMaxMinutes: l.GetIntDefault("TRANSCODER_MAX_MINUTES", 120),
		TranscoderTmpDir:     l.GetDefault("TRANSCODER_TMPDIR", os.TempDir()),
	}

	cfg.MediaSigningKey, err = secreto(l, base.Env, "MEDIA_SIGNING_KEY", DevMediaSigningKey)
	if err != nil {
		return Config{}, err
	}
	cfg.InternalSecret, err = secreto(l, base.Env, "CAST_INTERNAL_SECRET", DevInternalSecret)
	if err != nil {
		return Config{}, err
	}

	// Directo (§6): solo si hay SFU.
	cfg.LiveKitURL = l.Get("LIVEKIT_URL")
	cfg.LiveKitHost = l.GetDefault("LIVEKIT_HOST", "http://livekit:7880")
	cfg.LiveKitAPIKey = l.Get("LIVEKIT_API_KEY")
	cfg.LiveKitAPISecret = l.Get("LIVEKIT_API_SECRET")
	cfg.LiveKitPollInterval = l.GetDurationDefault("LIVEKIT_POLL_INTERVAL", 5*time.Second)
	cfg.IngestRTMPURL = l.GetDefault("CAST_INGEST_RTMP_URL", "rtmp://cast-ingest.sytue.localhost/live")
	cfg.RecordingBucket = l.GetDefault("RECORDING_BUCKET", cfg.S3.Bucket)
	cfg.RecordingEndpoint = conEsquema(l.GetDefault("RECORDING_ENDPOINT", cfg.S3.Endpoint), cfg.S3.UseSSL)
	cfg.RecordingRegion = l.GetDefault("RECORDING_REGION", cfg.S3.Region)
	cfg.RecordingAccessKey = l.GetDefault("RECORDING_ACCESS_KEY", cfg.S3.AccessKey)
	cfg.RecordingSecretKey = l.GetDefault("RECORDING_SECRET_KEY", cfg.S3.SecretKey)
	if cfg.LiveKitURL != "" && (cfg.LiveKitAPIKey == "" || cfg.LiveKitAPISecret == "") {
		return Config{}, fmt.Errorf("con LIVEKIT_URL, LIVEKIT_API_KEY y LIVEKIT_API_SECRET son obligatorias")
	}
	if cfg.SweepInterval > 0 && cfg.SweepStaleUpload > 0 && cfg.SweepStaleUpload < MinSweepStaleUpload {
		return Config{}, fmt.Errorf("CAST_SWEEP_STALE_UPLOAD (%s) ha de ser al menos %s: por debajo, el barrido daría por fallida una copia desde Drive que sigue en marcha", cfg.SweepStaleUpload, MinSweepStaleUpload)
	}
	cfg.HubBaseURL = l.Get("HUB_BASE_URL")
	if cfg.HubBaseURL != "" {
		cfg.HubInternalSecret = l.HubInternalSecret(base.Env)
	}
	// De quién se cree el X-Forwarded-For: vacía, las redes privadas (Traefik y el
	// BFF de la web, que viven dentro del clúster); "none", de nadie.
	if cfg.TrustedProxies, err = clientip.ParseTrustedProxies(l.Get("TRUSTED_PROXIES")); err != nil {
		return Config{}, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	if l.Get("TRUSTED_PROXIES") == "" {
		cfg.TrustedProxies = clientip.DefaultTrustedProxies()
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

// secreto lee un secreto obligatorio fuera de desarrollo: en dev cae al de
// desarrollo; fuera, falta o es el de desarrollo = el servicio no arranca. Mismo
// régimen que HubInternalSecret.
func secreto(l *pcfg.Loader, env pcfg.Environment, nombre, dev string) (string, error) {
	v := l.Get(nombre)
	switch {
	case env == pcfg.EnvDev && v == "":
		return dev, nil
	case env != pcfg.EnvDev && v == "":
		return "", fmt.Errorf("%s es obligatorio fuera de desarrollo", nombre)
	case env != pcfg.EnvDev && v == dev:
		return "", fmt.Errorf("%s es el secreto de desarrollo, que es público", nombre)
	}
	return v, nil
}

// conEsquema deja un endpoint S3 con esquema, que es como lo quiere el egress de
// LiveKit; el cliente S3 del servicio lo quiere sin él, así que se guarda una
// variable y cada consumidor la adapta (mismo criterio que Rooms).
func conEsquema(endpoint string, ssl bool) string {
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return endpoint
	}
	if ssl {
		return "https://" + endpoint
	}
	return "http://" + endpoint
}

func primero(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
