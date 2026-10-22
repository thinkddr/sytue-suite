// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Mail desde el entorno
// del proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable definidos en deploy/compose
// (PORT, DATABASE_URL, OIDC_*, S3_*, SMTP_*, IMAP_*, DKIM_*). Sin secretos en git.
package config

import (
	"fmt"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/correo"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-mail"

// Config es la configuración completa de Sytue Mail.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://mail.sytue.eu).
	PublicURL string
	// PrimaryDomain es el dominio de correo principal servido (sytue.eu).
	// El MX entrante (mx.sytue.eu) acepta correo para este y los dominios de tenant.
	PrimaryDomain string
	// RootTenant es la única organización que puede crear alias en el dominio raíz
	// desnudo. Ahí viven las direcciones institucionales (security@, abuse@), así
	// que no puede ser por orden de llegada. Vacío = nadie, que es el lado seguro.
	RootTenant string
	// Hostname es el FQDN del MTA, usado en el saludo EHLO y en Received: (mx.sytue.eu).
	Hostname string
	// MTASTSMode es el `mode` de la política MTA-STS que se sirve en
	// mta-sts.<dominio>/.well-known/mta-sts.txt: none, testing o enforce.
	// Arranca en testing porque enforce exige que el certificado del MX cubra
	// mx.<dominio>; ponerlo antes deja fuera al remitente que sí valida.
	MTASTSMode string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Única fuente de verdad
	// (metadatos de mensajes + cola de salida sobre Postgres; sin Redis/Valkey).
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// OIDC configura la validación de tokens de Sytue ID (JWKS) para la API JMAP.
	OIDCIssuer string
	// IDInternalURL / IDInternalSecret abren el segundo camino de AUTH: las
	// CONTRASEÑAS DE APLICACIÓN que la persona crea en /account de Sytue ID. Sin
	// ellas el IMAP y el SMTP solo aceptan un JWT con scope `mail` —minutos de
	// vida, imposible de teclear en Thunderbird—, así que el correo no se puede
	// usar desde ningún cliente. El secreto es el MISMO que SYTUE_ID_INTERNAL_SECRET;
	// del Secret Manager, NUNCA en git. La URL, vacía ⇒ el propio issuer.
	IDInternalURL    string
	IDInternalSecret string
	OIDCJWKSURL      string
	OIDCAudience     string
	// OIDCAcceptedAudiences: la suite comparte sesión (cookie única), así que el
	// token puede traer la audiencia de OTRO app de la suite. Se acepta si su aud
	// contiene ALGUNA (allowlist first-party).
	OIDCAcceptedAudiences []string

	// Hub configura la emisión de notificaciones unificadas hacia Sytue Hub.
	// Correo es el buzón de los avisos por correo (avisos@<dominio>), el de «X pide
	// acceso a un buzón compartido». No es el relé del correo de la gente
	// (SMTP_RELAY_*): sale por la cuenta de avisos, como los de Drive y Notepads.
	Correo            correo.Config
	HubBaseURL        string
	HubInternalSecret string

	// S3 configura el Object Storage de blobs (mail-blobs): cuerpos MIME crudos
	// y adjuntos. Los metadatos (sobres, flags, hilos) viven en PostgreSQL.
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

	// --- Servidores de protocolo de correo ---

	// SMTPSubmissionAddr es la dirección de escucha del Submission (MSA, :587),
	// donde los clientes autenticados (vía Sytue ID) envían correo saliente.
	SMTPSubmissionAddr string
	// SMTPMXAddr es la dirección de escucha del MX entrante (MTA, :25):
	// recepción de correo de otros servidores hacia los dominios servidos.
	SMTPMXAddr string
	// SMTPSImplicitAddr es la dirección de submission con TLS implícito (:465).
	SMTPSImplicitAddr string
	// IMAPSAddr es la dirección de escucha de IMAPS (:993, TLS implícito).
	IMAPSAddr string

	// IMAPPublicHost / SMTPPublicHost son los nombres PÚBLICOS que se anuncian a
	// los clientes en la autoconfiguración (Thunderbird, Outlook Autodiscover).
	// Por defecto imap.<MAIL_DOMAIN> y smtp.<MAIL_DOMAIN>; se sobreescriben con
	// IMAP_PUBLIC_HOST / SMTP_PUBLIC_HOST cuando el despliegue publica otro FQDN.
	IMAPPublicHost string
	SMTPPublicHost string

	// TLSCertFile / TLSKeyFile son las rutas a cert/clave PEM (STARTTLS/implicit TLS).
	// Vacías en dev (Traefik termina TLS); obligatorias en prod para 465/993/25.
	TLSCertFile string
	TLSKeyFile  string

	// MaxMessageBytes limita el tamaño de un mensaje entrante/saliente.
	MaxMessageBytes int64
	// Relay* configura un smarthost por el que sale TODO el correo, en vez de
	// entregar contra el MX de cada destino.
	//
	// Vacío = entrega directa, que es el comportamiento de siempre. Se activa
	// donde la entrega directa no puede funcionar: una IP doméstica no controla
	// su PTR, el ISP suele bloquear el 25 saliente y un rango residencial arrastra
	// mala reputación. El síntoma sin relé no es un rebote —eso se vería— sino
	// acabar en spam en silencio y solo en algunos destinatarios.
	RelayHost     string
	RelayPort     string
	RelayUsername string
	RelayPassword string

	// DefaultMailboxQuotaBytes es la cuota de un buzón recién aprovisionado.
	//
	// Era una constante de 10 GiB en `delivery`, y eso la volvía inservible en
	// un servidor pequeño: los cuerpos MIME viven en el MISMO disco que
	// PostgreSQL y que los blobs de Drive, así que cuatro buzones a 10 GiB ya
	// son 40 GiB que nadie puede acotar sin recompilar.
	DefaultMailboxQuotaBytes int64
	// MaxRecipients limita destinatarios por transacción SMTP (anti-abuso).
	MaxRecipients int
	// SMTPReadTimeout / SMTPWriteTimeout son los plazos por conexión SMTP.
	SMTPReadTimeout  time.Duration
	SMTPWriteTimeout time.Duration

	// --- DKIM (firma de salientes) ---

	// S/MIME: firma del correo saliente con un certificado por buzón, emitido y
	// renovado solo por la autoridad de la organización. Activado por defecto —es
	// gratis y no depende de ninguna CA de pago—, pero necesita KEK: sin ella no
	// se guarda ninguna clave privada y el correo sale sin firmar.
	SMIMEEnabled bool
	// SMIMEKeyEncKeyB64 es la KEK AES-256 (base64) con la que se sellan las claves
	// privadas S/MIME at-rest. Origen: SYTUE_MAIL_KEY_ENCRYPTION_KEY.
	SMIMEKeyEncKeyB64 string

	// DKIMSelector es el selector DNS del registro DKIM (p. ej. "sytue").
	DKIMSelector string
	// DKIMPrivateKeyPEM es la clave privada Ed25519/RSA en PEM (desde Secret Manager,
	// NUNCA en git). Si está vacía, la firma DKIM se omite (solo dev).
	DKIMPrivateKeyPEM string

	// --- Cola de salida (sobre PostgreSQL) ---

	// QueueWorkers es el número de workers que drenan la cola de salida.
	QueueWorkers int
	// QueueMaxAttempts es el número máximo de intentos de entrega antes de bounce.
	QueueMaxAttempts int
	// QueueBaseBackoff es el backoff base entre reintentos (exponencial con jitter).
	QueueBaseBackoff time.Duration
	// QueuePollInterval es el intervalo de respaldo del poll de la cola (LISTEN/NOTIFY
	// despierta antes; el poll garantiza progreso si se pierde un NOTIFY).
	QueuePollInterval time.Duration

	// --- Anti-abuso / hardening de recepción ---

	// VerifySPF activa la verificación SPF del remitente en el MX entrante.
	VerifySPF bool
	// VerifyDKIM activa la verificación de firmas DKIM en el MX entrante.
	VerifyDKIM bool
	// EnforceDMARC aplica la política DMARC publicada del dominio del remitente.
	EnforceDMARC bool
	// RejectOnDMARCFail rechaza (en lugar de cuarentena) cuando DMARC=reject.
	RejectOnDMARCFail bool

	// InboundWebhookToken activa la recepción de correo por HTTPS cuando el
	// puerto 25 no es alcanzable desde internet —proveedores de acceso
	// residenciales lo bloquean de serie—. Un tercero pone el MX y nos entrega
	// el mensaje por el webhook. VACÍO (el defecto) ⇒ no se monta: una entrada
	// de correo sin autenticar sería un relé abierto.
	InboundWebhookToken string
	// MaxInboundPerMinPerIP limita conexiones/mensajes por IP en el MX (rate-limit).
	MaxInboundPerMinPerIP int
	// MaxAuthFailuresPerMinPerIP limita los FALLOS de autenticación por IP y minuto
	// en submission (587/465) e IMAP (993). A 0 se desactiva.
	//
	// No es lo mismo que MaxInboundPerMinPerIP y por eso es otro ajuste: aquel
	// cuenta mensajes de otros MTA, donde 60/min es tráfico normal; este cuenta
	// contraseñas falladas por un cliente, donde 10/min ya es un diccionario. Un
	// cliente de correo legítimo falla una o dos veces —el usuario se equivoca al
	// teclear— y luego acierta.
	MaxAuthFailuresPerMinPerIP int

	// --- Correo a canal de Sytue Rooms ---

	// ChannelMailDomain es el dominio de las direcciones de canal: el correo enviado a
	// `<token-del-webhook>@<este dominio>` se publica en su canal en vez de entregarse a
	// un buzón. Es como entran las alertas de las herramientas que solo saben mandar
	// email. VACÍO ⇒ apagado (esas direcciones no existen para el MX).
	ChannelMailDomain string
	// RoomsURL es la raíz interna de Sytue Rooms, a donde se publica. Sin ella el correo
	// a canal queda apagado aunque haya dominio: no habría a quién entregar.
	RoomsURL string
	// CalendarURL es la raíz interna de Sytue Calendar, a donde se pasan los REPLY de
	// las invitaciones que contestan por correo (iMIP). Vacía: se quedan en el buzón.
	CalendarURL string
	// AliasSistema son los alias de la suite que no son de ninguna persona, como
	// «alias=buzón» (MAIL_SYSTEM_ALIASES, separados por comas): el arranque los deja
	// entregando a ese buzón. Sin ellos, el remitente de Calendar devolvía 550.
	AliasSistema []string
}

// Load construye la configuración leyendo el entorno. Devuelve todos los
// errores de validación juntos.
func Load() (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:          base,
		PublicURL:     l.GetDefault("PUBLIC_URL", "https://mail.sytue.localhost"),
		PrimaryDomain: l.GetDefault("MAIL_DOMAIN", "sytue.localhost"),
		RootTenant:    l.GetDefault("MAIL_ROOT_TENANT", ""),
		Hostname:      l.GetDefault("MAIL_HOSTNAME", "mx.sytue.localhost"),
		MTASTSMode:    l.GetDefault("MTA_STS_MODE", "testing"),

		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		IDInternalURL:         l.Get("ID_INTERNAL_URL"),
		IDInternalSecret:      l.Get("ID_INTERNAL_SECRET"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-mail"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		Correo:            l.Correo(),
		HubInternalSecret: l.HubInternalSecret(base.Env),

		S3Endpoint:  l.GetDefault("S3_ENDPOINT", "minio:9000"),
		S3Region:    l.GetDefault("S3_REGION", "fr-par"),
		S3AccessKey: l.Require("S3_ACCESS_KEY"),
		S3SecretKey: l.Require("S3_SECRET_KEY"),
		S3Bucket:    l.GetDefault("S3_BUCKET", "sytue-dev-mail-blobs"),
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

		SMTPSubmissionAddr: l.GetDefault("SMTP_SUBMISSION_ADDR", ":587"),
		SMTPMXAddr:         l.GetDefault("SMTP_MX_ADDR", ":25"),
		SMTPSImplicitAddr:  l.GetDefault("SMTPS_ADDR", ":465"),
		IMAPSAddr:          l.GetDefault("IMAPS_ADDR", ":993"),

		IMAPPublicHost: l.Get("IMAP_PUBLIC_HOST"),
		SMTPPublicHost: l.Get("SMTP_PUBLIC_HOST"),

		TLSCertFile: l.Get("TLS_CERT_FILE"),
		TLSKeyFile:  l.Get("TLS_KEY_FILE"),

		// 50 MB, no 2048: el mensaje se procesa en memoria (parseo, firma, DKIM) y
		// el pod tiene 512 Mi. Con el defecto anterior un solo correo grande —de
		// cualquiera, por el MX— mataba el proceso y con él IMAP, envío y webmail.
		// Los ficheros grandes van por enlace de Drive, no dentro del correo.
		MaxMessageBytes: int64(l.GetIntDefault("MAX_MESSAGE_MB", 50)) * 1024 * 1024,
		RelayHost:       l.GetDefault("SMTP_RELAY_HOST", ""),
		RelayPort:       l.GetDefault("SMTP_RELAY_PORT", "587"),
		RelayUsername:   l.GetDefault("SMTP_RELAY_USERNAME", ""),
		RelayPassword:   l.Get("SMTP_RELAY_PASSWORD"),
		// El defecto sigue siendo 10 GiB: quien no configure nada se comporta
		// exactamente igual que antes.
		DefaultMailboxQuotaBytes: int64(l.GetIntDefault("DEFAULT_MAILBOX_QUOTA_GB", 10)) * 1024 * 1024 * 1024,
		MaxRecipients:            l.GetIntDefault("MAX_RECIPIENTS", 100),
		SMTPReadTimeout:          l.GetDurationDefault("SMTP_READ_TIMEOUT", 60*time.Second),
		SMTPWriteTimeout:         l.GetDurationDefault("SMTP_WRITE_TIMEOUT", 60*time.Second),

		SMIMEEnabled:      l.GetBoolDefault("SMIME_ENABLED", true),
		SMIMEKeyEncKeyB64: l.Get("SYTUE_MAIL_KEY_ENCRYPTION_KEY"),
		DKIMSelector:      l.GetDefault("DKIM_SELECTOR", "sytue"),
		DKIMPrivateKeyPEM: l.Get("DKIM_PRIVATE_KEY"),

		QueueWorkers:      l.GetIntDefault("QUEUE_WORKERS", 4),
		QueueMaxAttempts:  l.GetIntDefault("QUEUE_MAX_ATTEMPTS", 8),
		QueueBaseBackoff:  l.GetDurationDefault("QUEUE_BASE_BACKOFF", 30*time.Second),
		QueuePollInterval: l.GetDurationDefault("QUEUE_POLL_INTERVAL", 15*time.Second),

		VerifySPF:             l.GetBoolDefault("VERIFY_SPF", true),
		VerifyDKIM:            l.GetBoolDefault("VERIFY_DKIM", true),
		EnforceDMARC:          l.GetBoolDefault("ENFORCE_DMARC", true),
		RejectOnDMARCFail:     l.GetBoolDefault("REJECT_ON_DMARC_FAIL", false),
		InboundWebhookToken:   l.Get("INBOUND_WEBHOOK_TOKEN"),
		ChannelMailDomain:     l.Get("CHANNEL_MAIL_DOMAIN"),
		RoomsURL:              l.Get("ROOMS_URL"),
		CalendarURL:           l.Get("CALENDAR_INTERNAL_URL"),
		AliasSistema:          l.GetCSV("MAIL_SYSTEM_ALIASES"),
		MaxInboundPerMinPerIP: l.GetIntDefault("MAX_INBOUND_PER_MIN_PER_IP", 60),

		MaxAuthFailuresPerMinPerIP: l.GetIntDefault("MAX_AUTH_FAILURES_PER_MIN_PER_IP", 10),
	}

	cfg.S3Endpoint, cfg.S3UseSSL = pcfg.NormalizeS3Endpoint(cfg.S3Endpoint, cfg.S3UseSSL)

	// Hosts públicos de correo: derivados del dominio servido salvo override.
	if cfg.IMAPPublicHost == "" {
		cfg.IMAPPublicHost = "imap." + cfg.PrimaryDomain
	}
	if cfg.SMTPPublicHost == "" {
		cfg.SMTPPublicHost = "smtp." + cfg.PrimaryDomain
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para la API JMAP/HTTP.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8086"
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.IDInternalURL == "" {
		cfg.IDInternalURL = cfg.OIDCIssuer
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
	// Un `mode` fuera de los tres del RFC 8461 se sirve igual y el remitente
	// descarta la política entera: se pierde la protección sin que falle nada
	// visible. Una errata («enfore») costaría eso, así que se corta al arrancar.
	switch c.MTASTSMode {
	case "none", "testing", "enforce":
	default:
		return fmt.Errorf("config: MTA_STS_MODE debe ser none, testing o enforce (es %q)", c.MTASTSMode)
	}
	if c.MaxMessageBytes <= 0 {
		return fmt.Errorf("config: MAX_MESSAGE_MB debe ser > 0")
	}
	// Cero significaría un buzón que no admite ni un mensaje: es un error de
	// configuración, no una forma de desactivar la cuota.
	if c.DefaultMailboxQuotaBytes <= 0 {
		return fmt.Errorf("config: DEFAULT_MAILBOX_QUOTA_GB debe ser > 0")
	}
	if c.MaxRecipients <= 0 {
		return fmt.Errorf("config: MAX_RECIPIENTS debe ser > 0")
	}
	if c.QueueWorkers <= 0 {
		return fmt.Errorf("config: QUEUE_WORKERS debe ser > 0")
	}
	if c.QueueMaxAttempts <= 0 {
		return fmt.Errorf("config: QUEUE_MAX_ATTEMPTS debe ser > 0")
	}
	if c.Hostname == "" {
		return fmt.Errorf("config: MAIL_HOSTNAME es obligatorio")
	}
	// TLS implícito (465/993) requiere certificado. En dev se desactivan esos
	// listeners si no hay cert (ver cmd/mail/main.go).
	if c.Env.IsProd() && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
		return fmt.Errorf("config: TLS_CERT_FILE y TLS_KEY_FILE son obligatorios en prod")
	}
	// S/MIME viene ENCENDIDO por defecto y su KEK llega del secreto marcada como
	// `optional`. Sin ella el servicio arrancaba igual y la firma quedaba apagada
	// con un aviso en el log: una función de seguridad desactivada en silencio,
	// que es la peor clase de las dos. Si se pide S/MIME se pide su clave; y si no
	// se quiere, SMIME_ENABLED=false lo dice en voz alta.
	if c.Env.IsProd() && c.SMIMEEnabled && strings.TrimSpace(c.SMIMEKeyEncKeyB64) == "" {
		return fmt.Errorf("config: SYTUE_MAIL_KEY_ENCRYPTION_KEY es obligatoria en prod con SMIME_ENABLED=true " +
			"(sin ella el correo saliente NO se firma): pon la clave o desactiva S/MIME explícitamente")
	}
	return nil
}

// TLSConfigured indica si hay material TLS para los listeners de TLS implícito.
func (c Config) TLSConfigured() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}
