// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package http expone Sytue ID por HTTP: endpoints OIDC/OAuth2 (authorize,
// token, userinfo, jwks, discovery, logout), login/consent, gestión de usuarios
// y grupos, SCIM 2.0 y MFA/WebAuthn. Usa el andamiaje común de platform/pkg/httpx.
package http

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Config es la configuración del servicio Sytue ID.
type Config struct {
	config.Base

	// DatabaseDSN es la cadena de conexión a PostgreSQL.
	DatabaseDSN string
	// DBMaxConns es el tamaño máximo del pool de conexiones (SYTUE_ID_DB_MAX_CONNS).
	// En prod, detrás de PgBouncer, se dimensiona bajo por réplica (3-5).
	DBMaxConns int
	// Issuer es la URL pública base del IdP (https://id.sytue.eu), sin barra final.
	Issuer string
	// ResourceAudience es el identificador canónico del resource server de la API
	// de identidad (aud exigido en los access tokens que consume /api/v1 y
	// /userinfo). Por defecto es el propio Issuer. Cada servicio consumidor exige
	// SU identificador de recurso; ver docs/security.md §F6.
	ResourceAudience string
	// CookieSecure fuerza el flag Secure de las cookies (true salvo dev http).
	CookieSecure bool
	// TrustedProxies son las redes cuyo `X-Forwarded-For` se cree al resolver la IP
	// del cliente (SYTUE_ID_TRUSTED_PROXIES, CSV de CIDR o direcciones sueltas).
	// Por defecto, las redes privadas: es donde vive Traefik en el clúster. El
	// valor `none` no cree a nadie y usa siempre la del peer TCP — lo correcto si
	// algún día el servicio se publica sin proxy delante. De esta lista dependen
	// los límites por IP, las condiciones por IP del Acceso Condicional y la IP del
	// registro de auditoría (ver domain.TrustedProxies).
	TrustedProxies domain.TrustedProxies
	// SessionTTL es la vida de la sesión SSO de navegador.
	SessionTTL time.Duration
	// AccessTokenTTL es la vida del access token JWT.
	AccessTokenTTL time.Duration
	// IDTokenTTL es la vida del ID token.
	IDTokenTTL time.Duration
	// RefreshTokenTTL es la vida del refresh token.
	RefreshTokenTTL time.Duration
	// AuthCodeTTL es la vida del código de autorización.
	AuthCodeTTL time.Duration
	// MFAContinuationTTL es la vida del token de continuación MFA (paso 1→2, F8).
	// Corta por diseño (limita la ventana entre contraseña y segundo factor).
	MFAContinuationTTL time.Duration
	// SCIMBearerToken protege los endpoints SCIM (token estático del provisioner).
	// En prod debe provenir del Secret Manager; nunca en git.
	SCIMBearerToken string
	// SCIMOrgSlug liga el token SCIM a UNA organización (aislamiento por tenant):
	// todos los recursos se acotan a ella y el usuario aprovisionado por SCIM Users
	// pasa a ser miembro suyo. OBLIGATORIO si hay token: vacío ⇒ SCIM responde 503
	// «no configurado», porque un token sin org acotaría a NINGUNA y el listado de
	// usuarios sería el del despliegue entero. Una conexión de aprovisionamiento de
	// Entra/Okta = un token = una org.
	SCIMOrgSlug string
	// LogIngestToken protege `POST /service-logs`, por donde los OTROS servicios
	// mandan sus avisos y errores a la tabla que lee la consola de plataforma
	// (`platform/pkg/log`, collector.go). Vacío ⇒ la ruta responde 503 y cada
	// servicio se queda como estaba: escribiendo a stdout. Mismo valor que
	// SYTUE_LOG_COLLECTOR_TOKEN en el resto de charts; del Secret Manager.
	LogIngestToken string
	// InternalSecret autoriza el endpoint service-to-service que verifica las
	// contraseñas de aplicación (`POST /internal/app-passwords/verify`), que es
	// como Sytue Mail autentica IMAP/SMTP. Mismo valor que ID_INTERNAL_SECRET
	// en el chart de correo; del Secret Manager, NUNCA en git. Vacío ⇒ la ruta no
	// se monta y ningún cliente de correo puede entrar (fail-closed).
	InternalSecret         string
	RoomsCallAccessURL     string
	RoomsCallAccessSecret  string
	EncryptedDocumentCalls bool
	// ContactsInternalURL es la raíz del servicio de Sytue Contacts (sin
	// /api/v1), para propagar el avatar propio a las fichas de contacto que
	// coincidan por correo (POST /internal/avatar-sync). Vacía ⇒ no se propaga
	// (el avatar se guarda igual; solo falta el efecto en Contactos).
	ContactsInternalURL string
	// ContactsInternalSecret autoriza esa llamada. Mismo valor que
	// SYTUE_CONTACTS_INTERNAL_SECRET en el chart de contacts; del Secret
	// Manager, NUNCA en git.
	ContactsInternalSecret string
	RoomsAccessURL         string
	MatrixLifecycleSecret  string
	// HubBaseURL/HubInternalSecret: la puerta interna de Sytue Hub, por la que se
	// avisa a las barras abiertas de que la foto cambió (hubnotify.EventoAvatar).
	// Cualquiera de las dos vacía ⇒ no se avisa: la foto nueva se ve al entrar.
	HubBaseURL        string
	HubInternalSecret string
	// DirectorioSecret autoriza `POST /internal/directory/resolve`: de una dirección
	// a la persona de UNA organización, sin sesión. Lo usa solo la reconstrucción de
	// avisos de Docs (`docs avisos-historicos`) para saber a quién iba una mención.
	// Secreto PROPIO (sytue-id-directorio), no el de las contraseñas de aplicación ni
	// el del Hub, que tienen todos los servicios. Vacío ⇒ la ruta no se monta.
	DirectorioSecret string
	// OrgsPersonaSecret autoriza `POST /internal/people/orgs`: las organizaciones
	// activas de una persona, para la política de firmas de fuera de Docs (ADR 0059).
	// Secreto PROPIO (sytue-id-orgs-persona). Vacío ⇒ la ruta no se monta.
	OrgsPersonaSecret string
	// EstadoOrgsSecret autoriza `POST /internal/orgs/status`: estado de ciclo de vida
	// de hasta 500 organizaciones por slug, para que Pages sepa si siguen activas y
	// puede seguir sirviéndolas (A11). Secreto PROPIO (sytue-id-estado-orgs). Vacío ⇒
	// la ruta no se monta: sin secreto ningún servicio puede consultarlo.
	EstadoOrgsSecret string
	// NombresOrgsSecret autoriza `POST /internal/orgs/names`: el nombre visible de
	// las organizaciones que mandan avisos a una persona, para la campana del Hub.
	// Secreto PROPIO (sytue-id-nombres-orgs), compartido con el Hub, Rooms y Calendar.
	// Vacío ⇒ la ruta no se monta y se enseña el identificador.
	NombresOrgsSecret string
	// KeyEncryptionKey es la KEK (32 bytes) para cifrar en reposo la clave privada
	// de firma (private_pem) con AES-256-GCM (F9). Se aporta en base64 estándar
	// (32 bytes ⇒ 44 chars) o hex (64 chars) vía SYTUE_ID_KEY_ENCRYPTION_KEY, desde
	// el Secret Manager/KMS; NUNCA en git. Vacía en dev ⇒ sin cifrado.
	KeyEncryptionKey []byte
	// RotationInterval es la cadencia de rotación programada de la clave de firma
	// JWKS (F10). 0 ⇒ rotación deshabilitada (solo manual).
	RotationInterval time.Duration
	// KeyRetireAfter es la ventana de gracia tras degradar una clave a 'next' antes
	// de retirarla del JWKS (F10). DEBE ser >= vida del access token para no
	// invalidar tokens en vuelo. Por defecto: máx(2·AccessTokenTTL, 30m).
	KeyRetireAfter time.Duration
	// AccountAppPath es el prefijo bajo el que la app «Mi cuenta» se sirve en el
	// host de organización (`/account`). VACÍO = no está desplegada, y entonces
	// `/account` sigue sirviendo el HTML de este servicio.
	//
	// Es una variable y no una constante porque la redirección tiene que poder
	// APAGARSE: una instalación que no despliegue la app y reciba un 302 hacia
	// ella deja a su gente sin pantalla de cuenta, y sin forma de cambiar la
	// contraseña. El chart la pone donde la app existe.
	AccountAppPath string
	// RPID es el Relying Party ID de WebAuthn (dominio efectivo, p. ej. sytue.eu).
	RPID string
	// RPOrigins son los orígenes permitidos para WebAuthn (https://id.sytue.eu, ...).
	RPOrigins []string
	// RPDisplayName es el nombre mostrado del Relying Party.
	RPDisplayName string

	// ResetTokenTTL es la vida del token de reseteo de contraseña. Corta por diseño
	// (limita la ventana de un enlace de recuperación filtrado).
	ResetTokenTTL time.Duration
	// InviteTokenTTL es la vida del token de invitación a una organización. Más
	// larga que la del reseteo (el invitado tarda en aceptar): días, no minutos.
	// Sigue siendo firmada, de un solo uso y con caducidad.
	InviteTokenTTL time.Duration
	// EmailVerifyTokenTTL es la vida del token de verificación de email. Media: el
	// usuario suele confirmar en horas, no minutos. Firmada, de un solo uso y con
	// caducidad, igual que el reseteo/invitación.
	EmailVerifyTokenTTL time.Duration
	// PostLogoutDefaultURI es a dónde vuelve quien cierra sesión sin un destino
	// registrado: el sitio público. Vacío ⇒ se deriva del issuer (www.<dominio>).
	PostLogoutDefaultURI string
	// PlatformOrgSlug es la organización que OPERA la plataforma (la de Sytue).
	// Es el tenant del remitente transaccional: el MSA de Sytue Mail exige un
	// tenant válido en el token y encola bajo él. Debe coincidir con el slug que
	// creó `id bootstrap-owner`.
	PlatformOrgSlug string
	// SMTPAddr es host:port del relay de correo saliente para el email de reseteo.
	// Vacío ⇒ Mailer de log (dev): el enlace se registra y el flujo no se bloquea.
	// En producción es obligatoria y Load() lo exige: ese fallback es mudo y deja
	// al invitado esperando un correo que nadie llegó a mandar.
	SMTPAddr string
	// SMTPUsername/SMTPPassword autentican en el relay (PLAIN sobre STARTTLS).
	SMTPUsername string
	SMTPPassword string
	// SMTPTLSServerName es el nombre que se EXIGE en el certificado del relay,
	// cuando no coincide con la dirección de conexión. Dentro del clúster se
	// llama al MSA por el nombre de su Service, que ningún certificado público
	// cubre; el certificado real es el del host de correo. Separar «a dónde me
	// conecto» de «qué nombre exijo» permite verificar de verdad en vez de
	// desactivar la comprobación. Vacío ⇒ el host de SMTPAddr.
	SMTPTLSServerName string
	// MailFrom es la cabecera From (y el envelope-from) del correo de reseteo.
	MailFrom string

	// --- Perfil de configuración de Apple (.mobileconfig) ---
	// Coordenadas PÚBLICAS de los servicios que el perfil configura en iOS/macOS.
	// Por defecto se derivan del dominio de la suite (el del Issuer sin la
	// etiqueta "id"); se sobreescriben cuando el despliegue publica otros FQDN.
	// MailIMAPAddr / MailSMTPAddr son host:puerto (TLS implícito, como anuncia la
	// autoconfiguración de Sytue Mail). CalDAVURL / CardDAVURL son la URL del
	// principal DAV (RFC 6764) de Calendar y Contacts.
	MailIMAPAddr string
	MailSMTPAddr string
	CalDAVURL    string
	CardDAVURL   string
	// DriveWebDAVURL es la raíz WebDAV de Sytue Drive (RFC 4918). No entra en el
	// perfil de Apple (iOS/macOS no monta WebDAV por perfil), pero sí se publica
	// en /.well-known/sytue-clients.json para el asistente de dispositivos.
	DriveWebDAVURL string
	// MailMXHost es el FQDN del MX de Sytue Mail (mx.sytue.eu): el valor que se
	// pone en el registro MX de un dominio propio del cliente para que su correo
	// entre por aquí. Por defecto mx.<dominio de la suite>, que es la misma
	// convención que usa services/mail (MAIL_HOSTNAME). Vacío ⇒ el asistente NO
	// ofrece configurar el correo del dominio propio.
	MailMXHost string
	// MailAPIURL es la URL INTERNA de la API de Sytue Mail, la que se usa para
	// dejar creada la cuenta de correo de quien acaba de darse de alta antes de
	// que entre por primera vez (ver internal/mailprov). Interna y no la pública
	// porque es tráfico entre servicios de la suite y no tiene por qué salir a
	// dar la vuelta por el ingress. Vacía ⇒ no se crean cuentas en Sytue Mail: las
	// direcciones quedan asignadas y en cola hasta que se configure.
	MailAPIURL string

	// Components es el catálogo de piezas desplegadas a las que el panel de
	// plataforma les pregunta la versión (ver platform_versions.go). Lo rellena el
	// chart paraguas, que es quien sabe qué subcharts y qué superficies web hay
	// desplegados; el binario no deduce nombres de Service.
	//
	// Vacío ⇒ la tabla de versiones sale vacía y lo dice. No es un fallo de
	// arranque: es diagnóstico, y un IdP que no levanta porque le falta una lista
	// de diagnóstico sería un intercambio pésimo.
	Components []Component

	// --- Cloudflare: DNS de la suite y consulta de dominios (alta inicial) ---
	// UN SOLO token de PLATAFORMA (nunca por-organización, nunca en el navegador),
	// desde el Secret Manager. Vacío ⇒ la parte de DNS del alta se degrada con un
	// mensaje honesto: el alta NO se rompe y nada se finge.
	//
	// CloudflareAPIToken necesita el permiso «Zone · DNS · Edit» sobre la zona de
	// la suite para publicar <slug>.sytue.eu, y permiso de Registrar para consultar
	// disponibilidad/precio de un dominio. Ninguna de las dos cosas mueve dinero.
	CloudflareAPIToken string
	// CloudflareZoneID es la zona de la suite (la de sytue.eu) donde se publica el
	// subdominio de cada organización. Vacío ⇒ no se publica nada.
	CloudflareZoneID string
	// CloudflareAccountID es la cuenta contra la que se consulta el Registrar
	// (disponibilidad + precio). Vacío ⇒ no se ofrece esa consulta.
	CloudflareAccountID string
	// ScalewaySecretKey es la clave de la API de Scaleway con la que se publica el
	// subdominio de cada organización en la zona de la SUITE. Es quien sirve
	// `sytue.eu` de verdad (`dig NS sytue.eu` → ns0.dom.scw.cloud), así que sin
	// esto el publicador de Cloudflare no encuentra la zona y el subdominio no lo
	// crea nadie. Del Secret Manager; NUNCA en git.
	ScalewaySecretKey string
	// ScalewayDNSZone es el nombre de la zona (p. ej. `sytue.eu`). Vacío ⇒ no se
	// publica por Scaleway y se mantiene el camino de Cloudflare.
	ScalewayDNSZone string
	// TenantDNSTarget es a dónde apunta el CNAME de <slug>.<suite> (el host de
	// entrada de la suite, p. ej. apps.sytue.eu). Vacío ⇒ por defecto el propio
	// dominio de la suite.
	TenantDNSTarget string
	// TenantDNSAddress son las IP del ORIGEN con las que se publica la dirección
	// de una organización que ya tiene correo en su propio nombre. Lista separada
	// por comas; las v4 salen como A y las v6 como AAAA.
	//
	// Hace falta porque un CNAME no puede convivir con el MX y el SPF de sus
	// buzones (RFC 1034 §3.6.2, RFC 2181 §10.1), y A/AAAA sí. Vacío ⇒ esas altas
	// se quedan en «failed» con su motivo, que es la verdad: no hay forma legal de
	// publicarlas. Nunca se degradan a un CNAME que el proveedor va a rechazar.
	TenantDNSAddress string
	// TenantDNSProxied publica el registro por el proxy de Cloudflare (naranja).
	// Por defecto true: con el proxy activo el TLS de <slug>.sytue.eu lo cubre el
	// certificado comodín de la zona. Ponerlo a false SOLO si el origen no está
	// detrás de Cloudflare (un CNAME «gris» hacia una IP del proxy da error 1000).
	TenantDNSProxied bool

	// --- Suscripción de la suite (Mollie, empresa neerlandesa) ---
	// En producción la verja está activa por defecto: sin derecho pagado no se
	// emite ningún token con tenant. El precio no es configuración; permanece
	// fijado en domain.SuiteSeatPriceCents.
	RequirePaidSubscription bool
	// SelfHosted marca una instancia que alguien opera para sí mismo, no la
	// suite comercial. Es lo ÚNICO que permite arrancar en producción sin
	// suscripción de pago, y no relaja ninguna otra guarda: cifrado, SMTP y
	// TLS siguen exigiéndose igual que en la suite de pago.
	SelfHosted bool
	// SignupEnabled publica el alta autoservicio (`/signup`): cuentas personales
	// gratuitas, con 5 GB, sin organización propia. Apagado, la página no existe
	// y el enlace desaparece del login; las cuentas ya creadas siguen entrando.
	SignupEnabled bool
	// SignupCaptchaEnabled exige resolver el captcha ALTCHA (Proof of Work) al
	// registrar una nueva cuenta (/signup). Protege contra bots sin cookies ni
	// telemetría de terceros.
	SignupCaptchaEnabled bool
	// LoginCaptchaEnabled pide el mismo captcha ALTCHA al entrar con contraseña
	// (/login/). Se resuelve solo al pulsar «Continuar»: frena el relleno de
	// credenciales en bloque sin pedirle nada a la persona.
	LoginCaptchaEnabled bool
	// LoginBypassKey es la clave HMAC del bypass de pruebas del captcha de login:
	// quien la conoce puede firmar un mensaje «timestamp|email» (cabecera
	// X-Sytue-Test-Auth) que sustituye al PoW. Solo la tienen el despliegue y los
	// secretos del CI — NUNCA un navegador —, así que los usuarios normales no
	// pueden saltarse el captcha: les sigue siendo obligatorio. Vacía, el bypass
	// no existe.
	LoginBypassKey []byte
	// SMSSweegoAPIKey es la clave de la API de Sweego con la que salen los códigos
	// de verificación del alta (ADR 0041). Sin ella no hay SMS, y sin SMS no hay
	// alta autoservicio en producción: el formulario dice que no está disponible.
	SMSSweegoAPIKey string
	// SMSSender es el remitente que ve quien recibe el código («Sytue»). Tiene que
	// estar dado de alta en Sweego; vacío, Sweego usa el que haya.
	SMSSender    string
	MollieAPIKey string
	// DragonflyURL es la cache efímera compartida (redis://dragonfly:6379). VACÍA
	// es un estado válido: sin ella el IdP consulta PostgreSQL como siempre. Solo
	// se usa para acelerar lo que se pregunta en cada token, nunca para decidir.
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string
	// CacheSigningKey sella el derecho de suscripción que se guarda en la cache.
	// Sin sello, quien escriba en Dragonfly regala la suite poniendo un "1" —o se
	// la quita a una organización entera poniendo un "0", que deja de poder emitir
	// y refrescar tokens—. VACÍA => no se cachea y se consulta PostgreSQL.
	CacheSigningKey   string
	MollieRedirectURL string
	MollieWebhookURL  string
}

// Load carga la configuración de Sytue ID desde el entorno (prefijo SYTUE_ID).
func Load() (Config, error) {
	// baseErr se acumula con los errores del resto de la carga (no se aborta
	// para reportar todos los problemas de configuración a la vez).
	base, baseErr := config.LoadBase("SYTUE_ID", "sytue-id")
	l := config.NewLoader("SYTUE_ID")
	cfg := Config{
		Base:                    base,
		DatabaseDSN:             l.Require("DATABASE_DSN"),
		DBMaxConns:              l.GetIntDefault("DB_MAX_CONNS", 10),
		Issuer:                  l.GetDefault("ISSUER", "https://id.sytue.localhost"),
		ResourceAudience:        l.Get("RESOURCE_AUDIENCE"),
		CookieSecure:            l.GetBoolDefault("COOKIE_SECURE", true),
		SessionTTL:              l.GetDurationDefault("SESSION_TTL", 12*time.Hour),
		AccessTokenTTL:          l.GetDurationDefault("ACCESS_TOKEN_TTL", 15*time.Minute),
		IDTokenTTL:              l.GetDurationDefault("ID_TOKEN_TTL", time.Hour),
		RefreshTokenTTL:         l.GetDurationDefault("REFRESH_TOKEN_TTL", 720*time.Hour), // 30 días
		AuthCodeTTL:             l.GetDurationDefault("AUTH_CODE_TTL", 60*time.Second),
		MFAContinuationTTL:      l.GetDurationDefault("MFA_CONTINUATION_TTL", 5*time.Minute),
		SCIMBearerToken:         l.Get("SCIM_BEARER_TOKEN"),
		SCIMOrgSlug:             l.Get("SCIM_ORG_SLUG"),
		LogIngestToken:          l.Get("LOG_INGEST_TOKEN"),
		InternalSecret:          l.Get("INTERNAL_SECRET"),
		RoomsCallAccessURL:      l.GetDefault("ROOMS_CALL_ACCESS_URL", "http://rooms:8085"),
		RoomsCallAccessSecret:   l.Get("ROOMS_CALL_ACCESS_SECRET"),
		EncryptedDocumentCalls:  l.GetBoolDefault("ENCRYPTED_DOCUMENT_CALLS", false),
		ContactsInternalURL:     l.Get("CONTACTS_INTERNAL_URL"),
		ContactsInternalSecret:  l.Get("CONTACTS_INTERNAL_SECRET"),
		RoomsAccessURL:          l.Get("ROOMS_ACCESS_URL"),
		MatrixLifecycleSecret:   l.Get("MATRIX_LIFECYCLE_SECRET"),
		HubBaseURL:              l.Get("HUB_BASE_URL"),
		HubInternalSecret:       l.Get("HUB_INTERNAL_SECRET"),
		DirectorioSecret:        l.Get("DIRECTORIO_SECRET"),
		OrgsPersonaSecret:       l.Get("ORGS_PERSONA_SECRET"),
		EstadoOrgsSecret:        l.Get("ESTADO_ORGS_SECRET"),
		NombresOrgsSecret:       l.Get("NOMBRES_ORGS_SECRET"),
		AccountAppPath:          l.Get("ACCOUNT_APP_PATH"),
		RPID:                    l.GetDefault("WEBAUTHN_RP_ID", "sytue.localhost"),
		RPOrigins:               l.GetCSV("WEBAUTHN_RP_ORIGINS"),
		RPDisplayName:           l.GetDefault("WEBAUTHN_RP_DISPLAY_NAME", "Sytue"),
		ResetTokenTTL:           l.GetDurationDefault("RESET_TOKEN_TTL", 30*time.Minute),
		InviteTokenTTL:          l.GetDurationDefault("INVITE_TOKEN_TTL", 72*time.Hour),
		EmailVerifyTokenTTL:     l.GetDurationDefault("EMAIL_VERIFY_TOKEN_TTL", 24*time.Hour),
		PostLogoutDefaultURI:    l.Get("POST_LOGOUT_DEFAULT_URI"),
		PlatformOrgSlug:         l.GetDefault("PLATFORM_ORG_SLUG", "corporate"),
		SMTPAddr:                l.Get("SMTP_ADDR"),
		SMTPUsername:            l.Get("SMTP_USERNAME"),
		SMTPPassword:            l.Get("SMTP_PASSWORD"),
		SMTPTLSServerName:       l.Get("SMTP_TLS_SERVER_NAME"),
		MailFrom:                l.GetDefault("MAIL_FROM", "Sytue <no-reply@sytue.localhost>"),
		MailIMAPAddr:            l.Get("MAIL_IMAP_ADDR"),
		MailSMTPAddr:            l.Get("MAIL_SMTP_ADDR"),
		CalDAVURL:               l.Get("CALDAV_URL"),
		CardDAVURL:              l.Get("CARDDAV_URL"),
		DriveWebDAVURL:          l.Get("WEBDAV_URL"),
		MailMXHost:              l.Get("MAIL_MX_HOST"),
		MailAPIURL:              l.GetDefault("MAIL_API_URL", "http://mail:8086"),
		Components:              ParseComponents(l.Get("COMPONENTS")),
		CloudflareAPIToken:      l.Get("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:        l.Get("CLOUDFLARE_ZONE_ID"),
		CloudflareAccountID:     l.Get("CLOUDFLARE_ACCOUNT_ID"),
		ScalewaySecretKey:       l.Get("SCALEWAY_SECRET_KEY"),
		ScalewayDNSZone:         l.Get("SCALEWAY_DNS_ZONE"),
		TenantDNSTarget:         l.Get("TENANT_DNS_TARGET"),
		TenantDNSAddress:        l.Get("TENANT_DNS_ADDRESS"),
		TenantDNSProxied:        l.GetBoolDefault("TENANT_DNS_PROXIED", true),
		RequirePaidSubscription: l.GetBoolDefault("REQUIRE_PAID_SUBSCRIPTION", base.Env.IsProd()),
		SelfHosted:              l.GetBoolDefault("SELF_HOSTED", false),
		SignupEnabled:           l.GetBoolDefault("SIGNUP_ENABLED", true),
		SignupCaptchaEnabled:    l.GetBoolDefault("SIGNUP_CAPTCHA_ENABLED", true),
		LoginCaptchaEnabled:     l.GetBoolDefault("LOGIN_CAPTCHA_ENABLED", true),
		LoginBypassKey:          []byte(l.Get("LOGIN_BYPASS_KEY")),
		SMSSweegoAPIKey:         l.Get("SMS_SWEEGO_API_KEY"),
		SMSSender:               l.Get("SMS_SENDER"),
		MollieAPIKey:            l.Get("MOLLIE_API_KEY"),
		DragonflyURL:            l.Get("DRAGONFLY_URL"),
		DragonflyPassword:       l.Get("DRAGONFLY_PASSWORD"),
		CacheSigningKey:         l.Get("CACHE_SIGNING_KEY"),
		MollieRedirectURL:       l.Get("MOLLIE_REDIRECT_URL"),
		MollieWebhookURL:        l.Get("MOLLIE_WEBHOOK_URL"),
	}
	// Sin variable, las redes privadas (el despliegue detrás de Traefik). Un valor
	// ilegible NO cae en silencio a «cree a cualquiera»: aborta el arranque, porque
	// de esta lista dependen los límites por IP y el Acceso Condicional por red.
	if raw := l.Get("TRUSTED_PROXIES"); raw != "" {
		tp, err := domain.ParseTrustedProxies(raw)
		if err != nil {
			baseErr = mergeErr(baseErr, fmt.Errorf("SYTUE_ID_TRUSTED_PROXIES: %w", err))
		}
		cfg.TrustedProxies = tp
	} else {
		cfg.TrustedProxies = domain.DefaultTrustedProxies()
	}
	if cfg.HTTPAddr == ":8080" {
		// Puerto canónico de Sytue ID en dev (docs/CONVENTIONS.md §6).
		cfg.HTTPAddr = l.GetDefault("HTTP_ADDR", ":8081")
	}
	if len(cfg.RPOrigins) == 0 {
		cfg.RPOrigins = []string{cfg.Issuer}
	}
	if cfg.ResourceAudience == "" {
		// Identificador canónico del recurso: el propio issuer (RFC 9068).
		cfg.ResourceAudience = cfg.Issuer
	}

	// Coordenadas por defecto de los servicios que configura el perfil de Apple.
	suite := suiteDomain(cfg.Issuer)
	setDefault(&cfg.MailIMAPAddr, "imap."+suite+":993")
	setDefault(&cfg.MailSMTPAddr, "smtp."+suite+":465")
	setDefault(&cfg.CalDAVURL, "https://calendar."+suite+"/dav/")
	setDefault(&cfg.CardDAVURL, "https://contacts."+suite+"/dav/")
	setDefault(&cfg.DriveWebDAVURL, "https://drive."+suite+"/dav/")
	setDefault(&cfg.MailMXHost, "mx."+suite)
	// El sitio público de la suite: a donde vuelve quien cierra sesión.
	setDefault(&cfg.PostLogoutDefaultURI, "https://www."+suite+"/")
	// A dónde apunta el subdominio de cada organización si el despliegue no lo dice.
	setDefault(&cfg.TenantDNSTarget, suite)
	// Una cookie Domain=.sytue.eu también se enviaría a las publicaciones no
	// confiables de *.apps/preview.sytue.eu. El IdP usa cookies host-only y OAuth
	// mantiene el SSO entre superficies; se rechaza la opción antigua para evitar
	// que una configuración heredada reabra esa frontera.
	if l.Get("COOKIE_DOMAIN") != "" {
		baseErr = mergeErr(baseErr, fmt.Errorf("SYTUE_ID_COOKIE_DOMAIN no está permitido; las cookies deben ser host-only"))
	}
	setDefault(&cfg.MollieRedirectURL, "https://one."+suite+"/bienvenida?payment=return")
	setDefault(&cfg.MollieWebhookURL, strings.TrimSuffix(cfg.Issuer, "/")+"/billing/webhooks/mollie")
	// La suscripción de pago es obligatoria en producción, con UNA excepción que
	// hay que pedir a propósito: una instancia autoalojada.
	//
	// POR QUÉ NO BASTABA CON BAJAR EL ENTORNO A staging. Es lo que haría
	// cualquiera al chocar con esto, y es peor de lo que parece: IsProd() no
	// gobierna solo el cobro. También decide que Docs, Rooms y Analytics EXIJAN
	// sus claves de cifrado, y que el SMTP de Mail ponga AllowInsecureAuth a
	// true. En un despliegue expuesto a Internet, «staging» significa aceptar
	// credenciales de correo en claro y arrancar sin cifrado en reposo, sin que
	// nada lo diga. Un interruptor concreto para lo concreto evita que alguien
	// pague ese precio por no poder desactivar una pasarela de pago.
	//
	// Sigue siendo explícito y no un valor por defecto: quien no lo ponga se
	// encuentra el error de siempre.
	if cfg.Env.IsProd() && !cfg.RequirePaidSubscription && !cfg.SelfHosted {
		baseErr = mergeErr(baseErr, fmt.Errorf(
			"SYTUE_ID_REQUIRE_PAID_SUBSCRIPTION no puede desactivarse en producción (usa SYTUE_ID_SELF_HOSTED=true en una instancia autoalojada)",
		))
	}
	// Sin dirección de SMTP el mailer cae al de desarrollo, que escribe el enlace
	// en el log y devuelve nil como si hubiera enviado. Aguas arriba nada lo
	// distingue de un envío bueno: la invitación responde 201, la consola pinta el
	// aviso verde y el invitado no recibe nunca nada. El chart lleva tiempo
	// inyectando la variable y hasta hay un test de render que lo exige, pero la
	// única forma de que eso no se pueda deshacer en silencio es que el proceso se
	// niegue a arrancar. Se prefiere un despliegue que falla a uno que finge
	// enviar, porque además el correo transaccional es el ÚNICO camino de vuelta a
	// una cuenta cuya contraseña se ha perdido.
	if cfg.Env.IsProd() && cfg.SMTPAddr == "" {
		baseErr = mergeErr(baseErr, fmt.Errorf(
			"SYTUE_ID_SMTP_ADDR es obligatoria en producción: sin ella el reseteo, "+
				"las invitaciones y la verificación se escriben en el log en vez de enviarse",
		))
	}
	baseErr = mergeErr(baseErr, validarCobros(cfg, suite))

	// KEK para el cifrado en reposo de la clave de firma (F9). Base64 std o hex.
	if raw := l.Get("KEY_ENCRYPTION_KEY"); raw != "" {
		kek, kerr := decodeKEK(raw)
		if kerr != nil {
			baseErr = mergeErr(baseErr, fmt.Errorf("SYTUE_ID_KEY_ENCRYPTION_KEY: %w", kerr))
		} else {
			cfg.KeyEncryptionKey = kek
		}
	}

	// Rotación programada de JWKS (F10).
	cfg.RotationInterval = l.GetDurationDefault("KEY_ROTATION_INTERVAL", 0)
	cfg.KeyRetireAfter = l.GetDurationDefault("KEY_RETIRE_AFTER", 0)
	if cfg.KeyRetireAfter <= 0 {
		// Ventana de retiro por defecto: >= vida del access token, con margen.
		cfg.KeyRetireAfter = 2 * cfg.AccessTokenTTL
		if cfg.KeyRetireAfter < 30*time.Minute {
			cfg.KeyRetireAfter = 30 * time.Minute
		}
	}

	return cfg, mergeErr(baseErr, l.Err())
}

// validarCobros comprueba la configuración de Mollie cuando la verja de pago
// está activa. Sale de `Load` porque allí eran cinco ramas de las veintiuna que
// el linter contaba, y ninguna tiene que ver con cargar variables: son reglas de
// negocio sobre lo ya cargado.
func validarCobros(cfg Config, suite string) error {
	if !cfg.RequirePaidSubscription {
		return nil
	}
	var err error
	switch {
	case cfg.MollieAPIKey == "":
		err = mergeErr(err, fmt.Errorf("SYTUE_ID_MOLLIE_API_KEY es obligatoria con REQUIRE_PAID_SUBSCRIPTION"))
	case cfg.Env.IsProd() && !strings.HasPrefix(cfg.MollieAPIKey, "live_"):
		err = mergeErr(err, fmt.Errorf("SYTUE_ID_MOLLIE_API_KEY debe ser live_ en producción"))
	}
	if e := validateBillingURL(cfg.MollieWebhookURL, suite); e != nil {
		err = mergeErr(err, fmt.Errorf("SYTUE_ID_MOLLIE_WEBHOOK_URL: %w", e))
	}
	if e := validateBillingURL(cfg.MollieRedirectURL, suite); e != nil {
		err = mergeErr(err, fmt.Errorf("SYTUE_ID_MOLLIE_REDIRECT_URL: %w", e))
	}
	return err
}

func setDefault(dst *string, def string) {
	if *dst == "" {
		*dst = def
	}
}

func validateBillingURL(raw, suite string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("debe ser una URL HTTPS absoluta")
	}
	host := strings.ToLower(u.Hostname())
	if host != suite && !strings.HasSuffix(host, "."+strings.ToLower(suite)) {
		return fmt.Errorf("debe pertenecer al dominio de la suite %s", suite)
	}
	return nil
}

// suiteDomain deduce el dominio de la suite a partir del issuer del IdP
// (https://id.sytue.eu → sytue.eu): el resto de servicios cuelgan de él
// (imap., smtp., calendar., contacts.). Cualquiera de esas coordenadas se puede
// fijar explícitamente por entorno si el despliegue no sigue el patrón.
func suiteDomain(issuer string) string {
	host := issuer
	if u, err := url.Parse(issuer); err == nil && u.Host != "" {
		host = u.Hostname()
	}
	if rest, ok := strings.CutPrefix(host, "id."); ok {
		return rest
	}
	return host
}

func mergeErr(a, b error) error {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return errJoin{a, b}
	}
}

type errJoin [2]error

func (e errJoin) Error() string { return e[0].Error() + "; " + e[1].Error() }

// decodeKEK decodifica la clave de cifrado de claves (32 bytes) desde base64
// estándar o hexadecimal y valida su longitud (AES-256).
func decodeKEK(raw string) ([]byte, error) {
	var key []byte
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == keycrypto.KeyLen {
		key = b
	} else if b, err := hex.DecodeString(raw); err == nil && len(b) == keycrypto.KeyLen {
		key = b
	} else {
		return nil, fmt.Errorf("debe ser %d bytes en base64 estándar o hex", keycrypto.KeyLen)
	}
	return key, nil
}
