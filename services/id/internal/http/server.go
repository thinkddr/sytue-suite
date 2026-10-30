// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/mollie"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/dns"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/federation"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/scim"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Server agrupa las dependencias del servicio y construye el router HTTP.
type Server struct {
	cfg    Config
	log    *slog.Logger
	store  *store.Store
	keys   *oidc.KeyManager
	issuer *oidc.Issuer
	scim   *scim.Handler
	// fed es el broker RP para el login federado (discovery/canje/validación del
	// id_token contra el IdP upstream), con cliente HTTP SSRF-safe y caché de JWKS.
	fed *federation.Broker
	// hub avisa a las pantallas abiertas de un cambio de estado (la foto). nil = sin Hub.
	hub *hubnotify.Client

	// Limitadores anti-fuerza-bruta. login y acct son pg.RateLimiter (bucket
	// compartido entre réplicas vía Postgres: seguridad-crítico y decenas de
	// req/s, el UPSERT por intento es despreciable). token queda en memoria
	// (miles de req/s: un write PG por refresh sería un cuello; el límite
	// efectivo con N réplicas es N×burst, aceptable para ese endpoint).
	loginLimiter httpx.RateLimiter
	tokenLimiter httpx.RateLimiter
	// acctLimiter limita intentos por cuenta (email), para frenar la fuerza bruta
	// distribuida (muchas IP contra una misma cuenta) que un límite por IP no cubre.
	acctLimiter httpx.RateLimiter
	// smsPhoneLimiter y smsIPLimiter frenan los SMS del alta por número y por IP.
	// Cada SMS cuesta dinero y un formulario que los manda a cualquier número es
	// la puerta del fraude de SMS a números de tarificación especial: el captcha
	// frena bots baratos, estos frenan a quien lo resuelve.
	smsPhoneLimiter httpx.RateLimiter
	smsIPLimiter    httpx.RateLimiter
	// directorioLimiter frena /internal/directory/resolve por organización: quien
	// tenga su secreto no puede barrer el directorio de nadie a golpe de llamadas.
	directorioLimiter httpx.RateLimiter
	// directorioGlobal es el techo de todas las organizaciones juntas: el de arriba
	// solo, se rodea repartiendo entre organizaciones.
	directorioGlobal httpx.RateLimiter
	// directorioCandidatos cobra la vía global (quien colabora desde otra organización)
	// por par dirección-candidato: la que más podría revelar, la más frenada.
	directorioCandidatos *pg.RateLimiter
	// directorioSubs cobra por `sub` la pregunta de quién sigue en la organización:
	// también es un oráculo (de pertenencia) para quien tenga el secreto.
	directorioSubs *pg.RateLimiter
	// organizacionesGlobal es el techo de /internal/people/orgs: una llamada por
	// petición de firma a alguien de fuera, que ya tienen su propio tope por persona.
	organizacionesGlobal httpx.RateLimiter

	// mailer envía el email de reseteo de contraseña (SMTP en prod, log en dev).
	mailer Mailer
	// sms manda los códigos del alta (Sweego; log en dev). nil = sin SMS, y
	// entonces no hay alta autoservicio (ver signupDisponible).
	sms SMS
	// resetKey es la clave HMAC que firma los tokens de reseteo. Se deriva de la KEK
	// (separación de claves: NO es la KEK) para no añadir configuración nueva y
	// heredar su exigencia en prod; estable entre réplicas y reinicios.
	resetKey []byte
	// deviceKey es la clave HMAC que firma la cookie de dispositivo y deriva el
	// device_id (dispositivos de confianza, mig 0022). Se deriva de la KEK con su
	// propia etiqueta de dominio (separación de claves), como resetKey: sin config
	// nueva, estable entre réplicas y reinicios.
	deviceKey []byte
	// consentKey firma el «Permitir» de la pantalla de consentimiento de las apps
	// nativas (NAT-02): sin él, una página ajena podría enlazar la autorización ya
	// consentida. Derivada de la KEK con su etiqueta, como las de arriba.
	consentKey []byte

	// captcha genera y verifica los retos ALTCHA PoW para altas de cuentas.
	captcha *altcha.Verifier
	// loginBypassKey es la clave HMAC del bypass de pruebas del captcha de login
	// (vació = sin bypass; ver altcha.VerifyTestBypass). No deriva de la KEK a
	// propósito: es un secreto con ciclo de vida propio, rotable sin tocar nada más.
	loginBypassKey []byte

	// resolver consulta el DNS público para verificar el dominio propio de una
	// organización (alta inicial, mig 0027). En producción es net.DefaultResolver
	// —la comprobación es DNS de verdad—; en test se inyecta un resolver de prueba.
	resolver domain.Resolver

	// cf publica el subdominio de cada organización en NUESTRA zona de Cloudflare
	// y consulta disponibilidad/precio de dominios. Es nil cuando no hay token
	// configurado: el alta sigue funcionando y esa parte se degrada diciéndolo.
	cf *dns.Client
	// zoneDNS publica `<slug>.<dominio>` en la zona de LA SUITE. Es una interfaz
	// porque hay dos proveedores de verdad: la zona `sytue.eu` la sirve Scaleway
	// (`dig NS sytue.eu` → ns0.dom.scw.cloud) y el único cliente que existía era
	// de Cloudflare, así que el subdominio por organización no lo publicaba nadie
	// —`ensureTenantDNS` salía siempre por `unconfigured`—. Se prefiere Scaleway
	// cuando está configurado y se cae a Cloudflare para no romper un despliegue
	// que ya lo tuviera. El resto de usos de `cf` (Registrar, zona DEL CLIENTE)
	// siguen siendo de Cloudflare y no pasan por aquí.
	zoneDNS dns.ZonePublisher
	// billing verifica cobros en Mollie sin SDK externo. La API key nunca sale
	// del proceso y el webhook solo aporta un identificador opaco.
	billing *mollie.Mollie

	// cache es la cache efímera compartida (Dragonfly). Guarda el derecho de
	// suscripción de cada organización, que se pregunta en CADA emisión y refresco
	// de token con una consulta que cruza suscripción, miembros activos y asientos
	// concedidos.
	//
	// Es NIL cuando no hay Dragonfly, y eso no es un caso degradado a medias: el
	// tipo tolera el receptor nil y `Enabled()` responde false, así que se consulta
	// PostgreSQL igual que antes. La cache acelera, nunca decide.
	cache *cache.Client
}

// UsarCache conecta la cache efímera compartida. Se llama en el arranque, después
// de construir el Server y solo si Dragonfly está disponible; sin ella el IdP
// funciona exactamente como antes.
func (s *Server) UsarCache(c *cache.Client) { s.cache = c }

// NewServer construye el Server con sus dependencias ya inicializadas.
func NewServer(cfg Config, logger *slog.Logger, st *store.Store, km *oidc.KeyManager, iss *oidc.Issuer) *Server {
	s := &Server{
		cfg:    cfg,
		log:    logger,
		store:  st,
		keys:   km,
		issuer: iss,
		hub:    clienteHub(cfg),
		// Login interactivo: 10 intentos/min por IP con ráfaga de 10 (anti
		// credential-stuffing / fuerza bruta de contraseñas). Compartido entre
		// réplicas (Postgres): un límite por proceso se multiplicaría por N.
		loginLimiter: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 10.0 / 60.0, Burst: 10}),
		// Token endpoint: 60 req/min por IP con ráfaga de 30 (protege canje de
		// código, refresh, client_credentials y el registro dinámico de clients
		// sin ahogar el tráfico legítimo). Compartido entre réplicas (Postgres)
		// igual que el login: en memoria el techo real era N×30 con N = réplicas
		// (el HPA de id va de 2 a 8), así que el límite crecía justo cuando más
		// tráfico había y repartir las peticiones lo multiplicaba. El coste es
		// un UPSERT de una fila, marginal frente a lo que /token ya escribe al
		// rotar el refresh. La clave lleva PREFIJO propio: ver oauth2RateKey.
		tokenLimiter: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1.0, Burst: 30}),
		// Por cuenta: ~5 intentos/min con ráfaga de 5 (lockout suave anti fuerza
		// bruta distribuida contra un mismo email). Compartido entre réplicas:
		// dividido por N el lockout por cuenta pierde su sentido.
		acctLimiter: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 5.0 / 60.0, Burst: 5}),
		// SMS del alta: 3 seguidos a un mismo número y después uno cada 20 minutos;
		// 10 seguidos desde una IP y después uno cada 5 minutos (una oficina detrás
		// de una sola IP puede darse de alta entera en una mañana).
		smsPhoneLimiter: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1.0 / 1200, Burst: 3}),
		smsIPLimiter:    pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1.0 / 300, Burst: 10}),
		// Una reconstrucción de menciones de una organización son pocas tandas de 100.
		directorioLimiter: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1.0 / 60, Burst: 20}),
		directorioGlobal:  pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1.0 / 6, Burst: 100}),
		// Una reconstrucción da pocos pares (solo quienes entraron por un enlace de otra
		// organización); 400 de ráfaga y uno por segundo después.
		directorioCandidatos: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 1, Burst: 400}),
		// Los destinatarios de una organización caben de sobra; 5 por segundo después.
		directorioSubs: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 5, Burst: 3000}),
		// Diez por segundo sostenidos para toda la instalación.
		organizacionesGlobal: pg.NewRateLimiter(st.DB(), pg.RateLimiterConfig{PerSecond: 10, Burst: 200}),
		sms:                  newSMS(cfg, logger),
		mailer: newMailer(cfg, logger, func() (string, error) {
			// Identidad de servicio del propio IdP para el MSA de Sytue Mail.
			return iss.IssueAccessToken(claimsDelRemitente(cfg), time.Now())
		}),
		resetKey:       deriveResetKey(cfg.KeyEncryptionKey),
		deviceKey:      deriveDeviceKey(cfg.KeyEncryptionKey),
		consentKey:     deriveConsentKey(cfg.KeyEncryptionKey),
		captcha:        altcha.New(deriveCaptchaKey(cfg.KeyEncryptionKey), altcha.ConRepeticiones(st.RegistrarCaptchaResuelto)),
		loginBypassKey: cfg.LoginBypassKey,
		resolver:       net.DefaultResolver,
		// nil si no hay token: «sin Cloudflare» es un estado válido, no un fallo.
		cf:      dns.New(cfg.CloudflareAPIToken, cfg.CloudflareAccountID, cfg.CloudflareZoneID, ""),
		billing: mollie.New(cfg.MollieAPIKey, ""),
	}
	// Quién publica el subdominio de cada organización. Scaleway manda cuando está
	// configurado porque es quien sirve la zona de la suite; si no lo está, se
	// mantiene Cloudflare. Una interfaz nil-safe no basta aquí: un *Scaleway nil
	// metido en la interfaz NO es una interfaz nil, así que se comprueba HasZone.
	s.zoneDNS = s.cf
	if scw := dns.NewScaleway(cfg.ScalewaySecretKey, cfg.ScalewayDNSZone, ""); scw.HasZone() {
		s.zoneDNS = scw
	}
	s.scim = scim.New(st, cfg.Issuer, cfg.SCIMOrgSlug)
	s.fed = federation.New()
	// El dominio de la suite baja al store porque quien asigna la dirección de
	// correo al dar de alta cuelga de UpsertMembership, tres capas por debajo de la
	// configuración. Se deduce del issuer igual que las coordenadas de IMAP/SMTP
	// (ver suiteDomain en config.go): así una organización sin dominio propio
	// verificado tiene correo en su subdominio sin que nadie configure nada.
	st.SetSuiteDomain(suiteDomain(cfg.Issuer))
	return s
}

// deriveResetKey deriva la clave HMAC de los tokens de reseteo de la KEK con una
// etiqueta de dominio (separación de claves: NO se firma con la clave de cifrado).
// En prod la KEK es obligatoria (ver cmd/id); en dev, sin KEK, la clave es fija —
// aceptable en local (el token sigue siendo aleatorio de 256 bits y su hash en BD
// gobierna el un-solo-uso y la caducidad).
// ponytail: derivación de la KEK, sin config nueva; si algún día el reseteo debe
// sobrevivir a una rotación de KEK, promoverla a su propio secreto en Secret Manager.
func deriveResetKey(kek []byte) []byte {
	h := sha256.Sum256(append([]byte("sytue-id/pwreset-hmac/v1\x00"), kek...))
	return h[:]
}

// deriveDeviceKey deriva la clave HMAC de la cookie/identificador de dispositivo de la
// KEK con su etiqueta de dominio (separación de claves: NO firma con la de reseteo ni
// con la de cifrado). Igual criterio que deriveResetKey: en prod la KEK es obligatoria;
// en dev, sin KEK, es fija — aceptable en local (la cookie sigue siendo httpOnly y su
// secreto de navegador es aleatorio de 256 bits).
func deriveDeviceKey(kek []byte) []byte {
	h := sha256.Sum256(append([]byte("sytue-id/device-hmac/v1\x00"), kek...))
	return h[:]
}

// deriveConsentKey deriva la clave HMAC del consentimiento de las apps nativas.
func deriveConsentKey(kek []byte) []byte {
	h := sha256.Sum256(append([]byte("sytue-id/consent-hmac/v1\x00"), kek...))
	return h[:]
}

// deriveCaptchaKey deriva la clave HMAC de los retos ALTCHA PoW de la KEK con su
// etiqueta de dominio (separación de claves: NO firma con la de reseteo ni con la
// de dispositivo ni con la de cifrado). En prod la KEK es obligatoria; en dev/test,
// sin KEK, es fija pero funcional.
func deriveCaptchaKey(kek []byte) []byte {
	h := sha256.Sum256(append([]byte("sytue-id/captcha-hmac/v1\x00"), kek...))
	return h[:]
}

// rateKeyPorCliente compone la clave del cubo por IP con un discriminador de
// contexto cuando la dirección no identifica a una persona (INF-04: par privado
// detrás de un túnel sin X-Forwarded-For público). Cuando la IP sí identifica al
// cliente, devuelve `<prefijo><ip>` sin tocar nada.
func rateKeyPorCliente(prefijo string, r *http.Request) string {
	ip := clientIP(r)
	if domain.TrustedProxiesFrom(r.Context()).Identifica(ip) {
		return prefijo + ip
	}
	h := sha256.New()
	_, _ = h.Write([]byte(ip))
	for _, hdr := range []string{"User-Agent", "Accept-Language", "Sec-CH-UA", "Cookie", "Authorization"} {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(r.Header.Get(hdr)))
	}
	sum := h.Sum(nil)
	return prefijo + ip + ":" + hex.EncodeToString(sum[:8])
}

// oauth2RateKey es la clave de bucket de los endpoints OAuth2 (/token, /revoke,
// /introspect, /register): la IP del cliente con PREFIJO propio.
//
// El prefijo NO es decorativo. Los buckets compartidos entre réplicas viven en
// una sola tabla (platform_rate_limits, clave primaria bucket_key), así que dos
// limitadores distintos que usen la IP «desnuda» —el defecto de httpx.RateLimit—
// escribirían la MISMA fila: el login (10/min, ráfaga 10) y OAuth2 (60/min,
// ráfaga 30) se pisarían el presupuesto y el tráfico normal de refresh acabaría
// recargando el bucket anti-fuerza-bruta del login. Con prefijo son dos filas.
func oauth2RateKey(r *http.Request) string { return rateKeyPorCliente("oauth2:", r) }

// loginRateKey es la clave del bucket por IP del login interactivo.
//
// Existe por lo mismo que oauth2RateKey —espacio de nombres propio en la tabla
// compartida— y, sobre todo, para que la IP la resuelva NUESTRO clientIP y no el
// de httpx, que lee la primera entrada de X-Forwarded-For sin mirar quién
// conecta: con aquel, un atacante estrenaba bucket en cada intento poniendo una
// cabecera distinta, y el tope de 10/min no frenaba absolutamente nada.
func loginRateKey(r *http.Request) string { return rateKeyPorCliente("login-ip:", r) }

// conProxiesDeConfianza deja la lista de proxies de confianza en el contexto de
// cada petición. Va lo PRIMERO del router: de ella depende clientIP, y clientIP
// lo consulta ya el middleware de rate-limiting.
func (s *Server) conProxiesDeConfianza(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(domain.WithTrustedProxies(r.Context(), s.cfg.TrustedProxies)))
	})
}

// Router construye el *chi.Mux con todas las rutas del servicio.
func (s *Server) Router() http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{
		Logger:         s.log,
		RequestTimeout: 30 * time.Second,
	})
	r.NotFound(barraFinalSobra)

	// Sondas de salud (sobreescriben las por defecto de NewRouter con checkers reales).
	httpx.MountHealth(r, s.cfg.Version, 3*time.Second, httpx.CheckerFunc{
		Label: "postgres", Fn: s.store.Healthz,
	})

	// Cabeceras de seguridad específicas del IdP (CSP estricta, HSTS, no-store).
	// chi exige que los middlewares se declaren ANTES de registrar rutas en un
	// mismo mux; como NewRouter ya monta rutas, montamos las rutas del IdP en un
	// sub-router (Group) que sí admite Use antes de sus propias rutas.
	r.Group(func(r chi.Router) {
		r.Use(s.conProxiesDeConfianza)
		r.Use(securityHeaders(s.cfg))

		// --- Discovery + JWKS (públicos) ---
		// La marca, sin sesión: es la pantalla de login. Ver favicon.go.
		r.Get("/favicon.svg", handleFavicon)
		r.Get("/fonts/{name}", handleFont)
		r.Get("/favicon.ico", handleFaviconICO)
		r.Get("/apple-touch-icon.png", handleAppleTouchIcon)
		r.Get("/.well-known/openid-configuration", s.handleDiscovery)
		r.Get("/.well-known/jwks.json", s.handleJWKS)
		// Coordenadas públicas de los clientes nativos (IMAP/SMTP, CalDAV,
		// CardDAV, WebDAV) que consume el asistente «Conectar tus dispositivos».
		// Sin datos de usuario => sin sesión (ver clientconfig.go).
		r.Get("/.well-known/sytue-clients.json", s.handleClientEndpoints)
		// El reto que una CA pública viene a leer para creerse que este dominio es
		// nuestro, y poder emitir el certificado S/MIME de esa organización. Sin
		// sesión: quien viene es la CA. Ver dcv.go.
		r.Get("/.well-known/pki-validation/{nombre}", s.handleDCVChallenge)
		// Mollie envía application/x-www-form-urlencoded con un payment id. No se
		// confía en ese estado: el handler consulta de nuevo la API autenticada.
		r.Post("/billing/webhooks/mollie", s.handleMollieWebhook)

		// Logo de la organización, SIN sesión: lo pinta un <img> de la barra superior
		// en cada carga de las once superficies, así que no puede depender de una
		// cabecera de autorización. Lo que se sirve es el PNG que produjo nuestro
		// propio reencuadre, nunca el fichero que subió nadie (ver admin_branding.go).
		r.Get("/branding/{slug}/logo.png", s.handleLogoPublico)

		// --- Raíz + Mi cuenta (self-service; validan la sesión SSO por sí mismos) ---
		r.Get("/", s.handleHome)
		r.Get("/account", s.handleAccount)
		// Avatar por COOKIE de sesión: es la URL del claim `picture`
		// (oidc.PictureURL). Las dos llegan aquí; `/account/avatar` es la de los
		// tokens ya emitidos, que hasta el 2026-09-24 tapaba el Ingress comodín
		// (ver deploy/production/tests/ingress-comodin-test.sh).
		r.Get("/avatar", s.handleAccountAvatarGet)
		r.Get("/account/avatar", s.handleAccountAvatarGet)
		// Cierre de sesiones desde /account (formularios POST con verificación
		// de Origin; la autorización por usuario vive en el store).
		r.Post("/account/sessions/revoke", s.handleSessionRevoke)
		r.Post("/account/sessions/revoke-others", s.handleSessionRevokeOthers)
		// Perfil editable por su dueño (nombre, puesto, departamento, idioma).
		r.Post("/account/profile", s.handleAccountProfile)
		// Perfil de configuración de Apple (.mobileconfig) con Mail/CalDAV/CardDAV
		// del usuario: el «conector de SO» de iOS/macOS. Exige sesión.
		r.Get("/account/apple.mobileconfig", s.handleAppleProfile)
		// El mismo perfil con su contraseña de aplicación, por el enlace de un solo uso
		// que la app del iPhone abre en Safari (me_app_passwords.go). Sin sesión: el
		// token es la credencial.
		r.Get("/account/apple-profile/{token}", s.handleAppleProfileLink)

		// Middleware de rate-limiting anti-fuerza-bruta para endpoints sensibles.
		tokenRL := httpx.RateLimit(httpx.RateLimitConfig{Limiter: s.tokenLimiter, KeyFunc: oauth2RateKey})
		loginRL := httpx.RateLimit(httpx.RateLimitConfig{Limiter: s.loginLimiter, KeyFunc: loginRateKey})

		// Seguridad self-service desde /account (formularios POST, sin JS): verifican
		// contraseña/código, así que van rate-limitados como el login (anti fuerza
		// bruta). El guard de sesión y Origin vive en cada handler.
		r.With(loginRL).Post("/account/password", s.handleAccountPassword)
		r.With(loginRL).Post("/account/mfa/totp/enroll", s.handleAccountTOTPEnroll)
		r.With(loginRL).Post("/account/mfa/totp/confirm", s.handleAccountTOTPConfirm)
		r.With(loginRL).Post("/account/mfa/totp/disable", s.handleAccountTOTPDisable)
		r.With(loginRL).Post("/account/mfa/totp/recovery/regenerate", s.handleAccountRecoveryRegenerate)
		// Contraseñas de aplicación (mig 0056): la credencial con la que entra un
		// cliente de correo. Rate-limitadas como el resto de seguridad self-service:
		// crear es barato, pero el formulario está detrás de una sesión y no hay
		// motivo para dejar que alguien genere mil.
		r.With(loginRL).Post("/account/app-passwords", s.handleAccountAppPasswordCreate)
		r.With(loginRL).Post("/account/app-passwords/revoke", s.handleAccountAppPasswordRevoke)
		// Alta y baja de passkeys desde la interfaz. begin/finish son los MISMOS
		// handlers que la API con Bearer: ahí resuelven el usuario por el token y
		// aquí por la cookie de sesión, exigiendo Origin permitido (ver
		// webauthnUser). Van limitados como el resto del self-service de
		// seguridad: cada begin emite un reto que se persiste, así que sin tope
		// una pestaña abierta podría llenar la tabla de retos.
		r.With(loginRL).Post("/account/passkeys/begin", s.handleWebAuthnRegisterBegin)
		r.With(loginRL).Post("/account/passkeys/finish", s.handleWebAuthnRegisterFinish)
		r.With(loginRL).Post("/account/passkeys/delete", s.handlePasskeyDelete)
		// Marcar de confianza el dispositivo ACTUAL, por su cookie. Es de Go y no de
		// la app porque la app no ve las cookies del IdP (ver me_devices.go).
		r.With(loginRL).Post("/account/devices/trust", s.handleAccountDeviceTrust)
		// «Borrar mi cuenta» (ADR 0054). Su URL la publica Google Play: no se mueve. Va
		// literal y no con rutaBorrarCuenta porque el test de Ingress la lee de aquí.
		// El POST verifica la contraseña, así que va limitado como el login.
		r.Get("/account/delete", s.handleAccountDeleteGet)
		r.With(loginRL).Post("/account/delete", s.handleAccountDeletePost)

		// --- OAuth2 / OIDC ---
		r.Route("/oauth2", func(r chi.Router) {
			r.Get("/authorize", s.handleAuthorize)  // Authorization Code + PKCE
			r.Post("/authorize", s.handleAuthorize) // consentimiento
			// Token y revoke: limitados (canje de código, refresh, client_credentials).
			r.With(tokenRL).Post("/token", s.handleToken) // code|refresh_token|client_credentials
			r.With(tokenRL).Post("/revoke", s.handleRevoke)
			// Introspección (RFC 7662): exige auth de client como /token; limitada igual.
			r.With(tokenRL).Post("/introspect", s.handleIntrospect)
			// Registro dinámico de clients (RFC 7591): valida el token de acceso
			// inicial de la org y crea el client org-scoped. Rate-limitado.
			r.With(tokenRL).Post("/register", s.handleDCRRegister)
			r.Get("/userinfo", s.handleUserinfo)
			r.Post("/userinfo", s.handleUserinfo)
			r.Get("/logout", s.handleLogout) // end_session
			r.Post("/logout", s.handleLogout)
		})

		// --- Cuenta de Google emulada: discovery, auth, userinfo, tokeninfo y la
		// Directory API de solo lectura con la forma de las de Google ---
		s.mountGoogleCompat(r, tokenRL)

		// --- Login / autenticación interactiva (formularios del AS) ---
		r.Route("/login", func(r chi.Router) {
			// El POST de credenciales/MFA se limita por IP (anti fuerza bruta).
			r.Get("/", s.handleLoginForm)
			r.Get("/captcha", s.handleLoginCaptcha)
			r.With(loginRL).Post("/", s.handleLoginSubmit)
			r.With(loginRL).Post("/mfa", s.handleMFAVerify)
			// Ceremonia de login por passkey (WebAuthn assertion). Pública (aún no hay
			// sesión) y rate-limitada como el login por contraseña.
			r.With(loginRL).Post("/webauthn/begin", s.handleWebAuthnLoginBegin)
			r.With(loginRL).Post("/webauthn/finish", s.handleWebAuthnLoginFinish)
			// Enrutado por dominio de email hacia el IdP corporativo ("Entrar con tu
			// organización"). GET pinta el formulario; POST resuelve el provider por
			// dominio y redirige a su flujo (rate-limitado, sin sesión).
			r.Get("/sso", s.handleSSOForm)
			r.With(loginRL).Post("/sso", s.handleSSOSubmit)
		})

		// --- Login federado (Sytue ID como broker/RP OIDC) ---
		// Flujo Authorization Code contra el IdP upstream del provider {id}. Público
		// (aún no hay sesión) y rate-limitado como el login (anti abuso del start y del
		// canje). El callback valida state (cookie+BD), el id_token y abre la sesión SSO.
		r.Route("/federated", func(r chi.Router) {
			r.With(loginRL).Get("/{id}/start", s.handleFederatedStart)
			r.With(loginRL).Get("/{id}/callback", s.handleFederatedCallback)
		})

		// --- Login federado SAML 2.0 (Sytue ID como Service Provider) ---
		// Una empresa con EntraID/ADFS/Okta federa su login por SAML. metadata es
		// pública (los IdP la descargan); login/acs van rate-limitados como el login.
		// El ACS valida la Response con crewjam (firma/condiciones/audiencia/recipient/
		// InResponseTo single-use) y abre la sesión SSO reaprovechando el JIT OIDC.
		r.Route("/saml", func(r chi.Router) {
			// La metadata va rate-limitada como el resto del flujo (ID-02): es pública
			// y sin tope dejaba martillear el endpoint. La del IdP, además, ya no acuña
			// la clave de firma por una petición anónima (ver handleSAMLIdPMetadata).
			r.With(loginRL).Get("/{id}/metadata", s.handleSAMLMetadata)
			r.With(loginRL).Get("/{id}/login", s.handleSAMLLogin)
			r.With(loginRL).Post("/{id}/acs", s.handleSAMLACS)

			// --- Sytue ID como IdP (identidad SALIENTE, SSO 1-click) --- metadata
			// pública; sso/initiate/slo exigen sesión de id (rate-limitados como el
			// login). El SP-initiated casa el AuthnRequest con un SP registrado en la
			// org de la SESIÓN (DENY-BY-DEFAULT) y emite una Assertion FIRMADA a su ACS;
			// cada emisión se audita. El segmento estático `idp` tiene prioridad sobre
			// el parámetro `{id}` de arriba (radix de chi).
			r.With(loginRL).Get("/idp/metadata", s.handleSAMLIdPMetadata)
			r.With(loginRL).Get("/idp/sso", s.handleSAMLIdPSSO)
			r.With(loginRL).Post("/idp/sso", s.handleSAMLIdPSSO)
			r.With(loginRL).Get("/idp/initiate/{spID}", s.handleSAMLIdPInitiate)
			r.With(loginRL).Post("/idp/slo", s.handleSAMLIdPSLO)
		})

		// --- Recuperación de contraseña ("olvidé mi contraseña", sin sesión) ---
		// El POST de solicitud (envío de email) va rate-limitado como el login para
		// frenar el abuso; el POST de fijado consume el token (un solo uso).
		// Alta autoservicio de cuentas personales. Mismo limitador por IP que el
		// resto del autoservicio: crear cuentas a mansalva es el ataque obvio.
		r.Get("/signup", s.handleSignupForm)
		r.Get("/signup/captcha", s.handleSignupCaptcha)
		r.With(loginRL).Post("/signup", s.handleSignupSubmit)
		r.With(loginRL).Post("/signup/code", s.handleSignupCode)
		r.With(loginRL).Post("/signup/resend", s.handleSignupResend)
		r.Get("/forgot-password", s.handleForgotPasswordForm)
		r.With(loginRL).Post("/forgot-password", s.handleForgotPasswordSubmit)
		r.Get("/reset-password", s.handleResetPasswordForm)
		r.With(loginRL).Post("/reset-password", s.handleResetPasswordSubmit)

		// --- Aceptación de invitación a una organización (pública, sin sesión) ---
		// El invitado valida el enlace firmado y fija su contraseña. El POST consume
		// el token (un solo uso) y va rate-limitado como el login.
		r.Get("/accept-invitation", s.handleAcceptInvitationForm)
		r.With(loginRL).Post("/accept-invitation", s.handleAcceptInvitationSubmit)

		// --- Verificación de email (pública, sin sesión) --- El enlace del correo:
		// GET valida la firma y pinta el botón; el POST consume el token (un solo uso,
		// rate-limitado) y marca el correo verificado.
		r.Get("/verify-email", s.handleVerifyEmailForm)
		r.With(loginRL).Post("/verify-email", s.handleVerifyEmailSubmit)

		// --- Recolector de logs de plataforma, protegido por bearer estático ---
		// Lo llaman los OTROS servicios, no una persona: por eso vive fuera de
		// /api/v1 (donde manda el JWT del usuario). Ver servicelogs_ingest.go.
		r.With(httpx.SinPasarPorElIngress).Post("/service-logs", s.handleIngestServiceLogs)

		// --- Verificación de contraseñas de aplicación (service-to-service) ---
		// La consulta Sytue Mail en cada AUTH de IMAP/SMTP. Fuera de /api/v1 por lo
		// mismo que el recolector: quien pregunta es un servicio y todavía no hay
		// identidad. Sin secreto configurado la ruta NO se monta: una verja abierta
		// aquí entrega el correo de cualquiera. Ver internal_app_passwords.go.
		if s.cfg.InternalSecret != "" {
			// Sin rate limit: el limitador de login cuenta por IP y aquí TODAS las
			// peticiones vienen del mismo pod de correo, así que estrangularía cada
			// conexión IMAP de todo el mundo. La verja es el secreto.
			r.With(s.internalSecretAuth).
				Post("/internal/app-passwords/verify", s.handleInternalVerifyAppPassword)
			// El canje por un token de la persona, para el WebDAV de Drive con Basic.
			r.With(s.internalSecretAuth).
				Post("/internal/app-passwords/token", s.handleInternalAppPasswordToken)
		}

		// --- De una dirección a la persona, para la reconstrucción de avisos ---
		// Ver internal_directorio.go. Sin su secreto propio no se monta.
		if s.cfg.DirectorioSecret != "" {
			r.With(httpx.SoloDesdeDentro(s.cfg.DirectorioSecret)).
				Post("/internal/directory/resolve", s.handleInternalDirectoryResolve)
		}
		// Las organizaciones de una persona, para las peticiones de firma de fuera
		// (ADR 0059). Ver internal_organizaciones.go. Sin su secreto no se monta.
		if s.cfg.OrgsPersonaSecret != "" {
			r.With(httpx.SoloDesdeDentro(s.cfg.OrgsPersonaSecret)).
				Post("/internal/people/orgs", s.handleInternalPeopleOrgs)
			r.With(httpx.SoloDesdeDentro(s.cfg.OrgsPersonaSecret)).Post("/internal/people/call-access", s.handleInternalCallIdentity)
		}
		// El estado de ciclo de vida de un conjunto de organizaciones por slug,
		// para Pages (A11, §6.5). Ver internal_estado_orgs.go. Sin secreto no se monta.
		if s.cfg.EstadoOrgsSecret != "" {
			r.With(httpx.SoloDesdeDentro(s.cfg.EstadoOrgsSecret)).
				Post("/internal/orgs/status", s.handleInternalOrgsStatus)
		}
		// El nombre visible de las organizaciones que avisan a una persona, para la
		// campana del Hub. Ver internal_nombres_orgs.go. Sin secreto no se monta.
		if s.cfg.NombresOrgsSecret != "" {
			r.With(httpx.SoloDesdeDentro(s.cfg.NombresOrgsSecret)).
				Post("/internal/orgs/names", s.handleInternalOrgsNames)
		}

		// --- SCIM 2.0 (provisioning), protegido por bearer estático ---
		r.Route("/scim/v2", func(r chi.Router) {
			r.Use(s.requireSCIMAuth)
			r.Get("/ServiceProviderConfig", s.scim.ServiceProviderConfig)
			r.Get("/ResourceTypes", s.scim.ResourceTypes)
			r.Get("/Schemas", s.scim.Schemas)
			r.Route("/Users", func(r chi.Router) {
				r.Get("/", s.scim.ListUsers)
				r.Post("/", s.scim.CreateUser)
				r.Get("/{id}", s.scim.GetUser)
				r.Put("/{id}", s.scim.ReplaceUser)
				r.Patch("/{id}", s.scim.PatchUser)
				r.Delete("/{id}", s.scim.DeleteUser)
			})
			// Groups: aprovisionamiento entrante de grupos (Entra/Okta). Acotado a
			// la org del token (SCIMOrgSlug). PATCH es el camino de members.
			r.Route("/Groups", func(r chi.Router) {
				r.Get("/", s.scim.ListGroups)
				r.Post("/", s.scim.CreateGroup)
				r.Get("/{id}", s.scim.GetGroup)
				r.Put("/{id}", s.scim.ReplaceGroup)
				r.Patch("/{id}", s.scim.PatchGroup)
				r.Delete("/{id}", s.scim.DeleteGroup)
			})
		})

		// --- API de identidad protegida por JWT propio (consola/admin/self-service) ---
		r.Route("/api/v1", func(r chi.Router) {
			r.Use(s.requireAccessToken)
			// …y, para TODO lo que administre (/admin/* y /platform/*), que la
			// credencial la haya pedido un client de la suite y no una app de
			// terceros a la que alguien dio a «permitir». Va aquí arriba, y no en
			// cada grupo, para que una ruta admin nueva nazca cubierta.
			r.Use(s.soloClientsDeLaSuite)
			r.Use(s.mfaParaAdministracion)
			r.Get("/me", s.handleMe)
			r.Patch("/me", s.handleMeUpdate) // edición de perfil self-service (solo name)
			// Avatar propio: predefinido o foto (ver me_avatar.go). Se propaga, mejor
			// esfuerzo, a las fichas de Contactos que coincidan por correo.
			r.Get("/me/avatar", s.handleMeAvatarGet)
			r.Put("/me/avatar", s.handleMeAvatarPut)
			r.Delete("/me/avatar", s.handleMeAvatarDelete)
			// «Exportar tus datos»: la entrega de la cuenta (platform/pkg/portabilidad).
			r.Get("/me/export", s.handleMeExport)
			r.Get("/me/sessions", s.handleMeSessions)
			// El otro lado de la lectura de arriba. Existían solo como formulario
			// HTML (`/account/sessions/revoke*`), así que quien no renderiza ese
			// HTML podía ver sus sesiones pero no cerrarlas.
			r.Delete("/me/sessions/{id}", s.handleMeSessionRevoke)
			r.Delete("/me/sessions", s.handleMeSessionRevokeOthers)
			// Cambiar a otra de MIS organizaciones (multi-org real). Self-service:
			// la verja es la membresía, no un rol. Ver me_orgs.go.
			r.Put("/me/active-organization", s.handleSetActiveOrg)
			// Dispositivos de confianza self-service (mig 0022): mis dispositivos (con el
			// actual marcado si llega la cookie de dispositivo), marcar de confianza el
			// actual y revocar uno. La confianza se acota por usuario en el store.
			r.Get("/me/devices", s.handleMeDevices)
			r.Post("/me/devices/{id}/trust", s.handleMeDeviceTrust)
			r.Delete("/me/devices/{id}", s.handleMeDeviceRevoke)
			// Ciclo de vida de credenciales self-service. El cambio de contraseña y
			// la confirmación/baja de TOTP verifican secretos → rate-limitados como
			// el login (anti fuerza bruta). enroll solo genera un secreto: sin límite.
			r.With(loginRL).Post("/me/password", s.handleMePassword)
			// Pide la contraseña actual: va limitado como el login por la misma razón.
			r.With(loginRL).Put("/me/recovery-email", s.handleMeRecoveryEmail)
			// Las cuatro de TOTP son la superficie JSON del contrato publicado
			// (`services/id/openapi.yaml`), con sus tests de integración, y NO tienen
			// consumidor en `apps/`: quien activa el segundo factor desde el navegador
			// lo hace por las rutas HTML `/account/mfa/totp/*` de más arriba, que sirve
			// este mismo binario. Existen para el cliente que NO renderiza ese HTML —las
			// apps nativas y cualquier integración contra el API—, así que se quedan a
			// propósito: retirar un contrato publicado y probado para ahorrarse un grep
			// sale más caro. Queda dicho aquí para que la próxima auditoría de cableado
			// no las levante otra vez como «superficie sin puerta».
			r.Post("/me/mfa/totp/enroll", s.handleMFATOTPEnroll)
			r.With(loginRL).Post("/me/mfa/totp/confirm", s.handleMFATOTPConfirm)
			r.With(loginRL).Post("/me/mfa/totp/recovery-codes", s.handleMFARecoveryRegenerate)
			r.With(loginRL).Delete("/me/mfa/totp", s.handleMFATOTPDisable)
			// Contraseñas de aplicación (mig 0056). Eran la ÚNICA parte de la
			// cuenta que solo se podía gestionar renderizando el HTML de
			// `/account`, y es la credencial con la que entra un cliente de correo.
			// El alta va rate-limitada como el resto del self-service sensible:
			// crear es barato, pero no hay motivo para dejar generar mil.
			r.Get("/me/app-passwords", s.handleMeAppPasswords)
			r.With(loginRL).Post("/me/app-passwords", s.handleMeAppPasswordCreate)
			r.With(loginRL).Delete("/me/app-passwords/{id}", s.handleMeAppPasswordRevoke)
			// El perfil de Apple con su propia contraseña de aplicación: el «un clic» de
			// las apps nativas en macOS e iOS (me_app_passwords.go).
			r.With(loginRL).Post("/me/apple-profile", s.handleMeAppleProfile)
			r.With(loginRL).Post("/me/apple-profile/link", s.handleMeAppleProfileLink)
			// Verificación del PROPIO email: envía/reenvía el enlace a la dirección de
			// la cuenta. Rate-limitado como el login (anti abuso de envío). Cualquier
			// usuario autenticado puede verificar su correo (no requiere rol admin).
			r.With(loginRL).Post("/me/email/verify/send", s.handleSendEmailVerification)
			// PIM self-service (mig 0020): elevación de privilegios JIT. Cualquier usuario
			// autenticado ve sus elegibilidades y activa/desactiva un rol elegible en la org
			// activa (la elegibilidad, no un rol admin, gobierna). Auditado. El claim `perms`
			// solo refleja los roles con activación ACTIVA (buildTokens → EffectivePerms).
			// Taxonomía de etiquetas de sensibilidad (mig 0024) — SOLO LECTURA para
			// cualquier miembro autenticado de la org activa: la consumen el selector de
			// etiqueta del frontend y OTROS SERVICIOS (Drive resuelve el id de etiqueta a
			// su snapshot al etiquetar un fichero) con el Bearer del propio usuario. La
			// administración de la taxonomía está más abajo, reservada a owner/admin.
			r.Get("/sensitivity-labels", s.handleListSensitivityLabels)
			// Resolver un correo de MI organización al identificador con el que se
			// guardan los permisos. Lo necesita cualquier «compartir con una
			// persona» de la suite: se escribe una dirección y por dentro se guarda
			// un `sub`. Va limitado POR PERSONA —no por IP, que dejaría fuera a una
			// oficina entera detrás del mismo NAT— porque uno a uno y sin tope esto
			// sería un barrido del directorio a base de probar nombres. Ver
			// directory.go.
			porPersona := httpx.RateLimit(httpx.RateLimitConfig{
				Limiter: s.tokenLimiter,
				KeyFunc: func(r *http.Request) string {
					c, _ := claimsFrom(r.Context())
					return "directory:" + c.Subject
				},
			})
			r.With(porPersona).Post("/directory:resolve", s.handleDirectoryResolve)
			// La agenda de la organización, para cualquiera de la casa: es lo que
			// alimenta la libreta «Organización» de Sytue Contacts. Va bajo el
			// MISMO limitador por persona, y no por cortesía: es un listado, así
			// que sin tope sería la forma barata de llevarse el directorio entero
			// una y otra vez.
			httpx.GetYQuery(r.With(porPersona), "/directory", s.handleDirectoryList)
			// Encontrar a alguien de OTRA organización por su @usuario o su correo,
			// para escribirle o llamarle (ADR 0040). Exacto y bajo el mismo
			// limitador: ver personas.go.
			r.With(porPersona).Post("/people:resolve", s.handlePeopleResolve)
			r.Get("/me/pim", s.handleMePIM)
			r.Post("/me/pim/activate", s.handleMePIMActivate)
			r.Post("/me/pim/deactivate", s.handleMePIMDeactivate)
			r.Route("/webauthn", func(r chi.Router) {
				r.Post("/register/begin", s.handleWebAuthnRegisterBegin)
				r.Post("/register/finish", s.handleWebAuthnRegisterFinish)
				r.Get("/credentials", s.handleWebAuthnList)
				// El otro lado del listado. Existía solo como formulario
				// (`/account/passkeys/delete`), así que quien no renderiza ese HTML
				// podía registrar llaves y no retirarlas nunca.
				r.Delete("/credentials/{id}", s.handleMePasskeyDelete)
			})
			// --- Alta inicial guiada (mig 0027) ---
			// Sin requireRole a propósito: quien se da de alta aún no tiene org ni
			// rol. El aislamiento lo ancla su PROPIA fila de onboarding (org_id) y la
			// re-verificación de OWNER contra la BD en cada operación (onboardingOrg).
			r.Route("/onboarding", func(r chi.Router) {
				// Los dos endpoints que hacen trabajo hacia fuera (consulta DNS y
				// envío de correo) van limitados POR USUARIO, no por IP: el bucket
				// por IP es el del login y agotarlo desde el asistente dejaría sin
				// entrar a toda una oficina detrás del mismo NAT.
				byUser := httpx.RateLimit(httpx.RateLimitConfig{
					Limiter: s.acctLimiter,
					KeyFunc: func(r *http.Request) string {
						c, _ := claimsFrom(r.Context())
						return "onboarding:" + c.Subject
					},
				})
				r.Get("/", s.handleOnboardingGet)
				r.Patch("/", s.handleOnboardingPatch)
				r.Get("/slug", s.handleOnboardingSlug)
				r.Post("/organization", s.handleOnboardingCreateOrg)
				r.Get("/subscription", s.handleSubscriptionStatus)
				r.Post("/subscription/checkout", s.handleSubscriptionCheckout)
				r.With(byUser).Post("/domain", s.handleOnboardingDomain)
				// Alternativa al paso de dominio: si el cliente ya tiene el suyo en
				// Cloudflare, Sytue le crea los registros con un token SUYO (ver
				// onboarding_cloudflare.go). Habla varias veces con una API externa:
				// limitado por usuario como el resto.
				r.With(byUser).Post("/domain/cloudflare", s.handleOnboardingDomainCloudflare)
				r.With(byUser).Get("/domain-availability", s.handleOnboardingDomainAvailability)
				r.With(byUser, s.requireOrgKind(domain.OrgKindOrg)).Post("/invitations", s.handleOnboardingInvite)
				r.With(s.requireOrgKind(domain.OrgKindOrg)).Post("/roles", s.handleOnboardingRoles)
			})
			// --- Administración con permisos GRANULARES (admin delegada, mig 0019) ---
			// Cada bloque exige el permiso de su recurso vía requirePermission. owner/admin
			// PASAN SIEMPRE (superusuarios built-in: retrocompat con requireRole); un rol
			// personalizado autoriza solo lo que su permiso concede (deny-by-default).

			// --- ELEVACIÓN AL ALCANCE DE PLATAFORMA (PIM, mig 0043) ---
			// FUERA del grupo de abajo, y no es un descuido: ese gate exige una
			// activación viva, así que pedirla desde detrás de él sería imposible por
			// construcción. Estas tres comprueban lo que sí se puede comprobar sin
			// estar elevado —allowlist y segundo factor— dentro de cada handler.
			r.Get("/platform/activation", s.handlePlatformActivationGet)
			r.With(s.requireStepUp).Post("/platform/activate", s.handlePlatformActivate)
			r.Post("/platform/deactivate", s.handlePlatformDeactivate)

			// --- ADMINISTRACIÓN DE PLATAFORMA (alcance global) ---
			// La única superficie que atraviesa organizaciones. No cuelga de
			// requireRole: el rol de una organización no concede NADA aquí. El gate
			// es la allowlist `platform_admins` (consultada en cada petición), segundo
			// factor obligatorio y una elevación viva. Ver platform.go.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePlatformAdmin)
				r.Get("/platform/activations", s.handlePlatformActivations)
				// Qué está fallando, agrupado. Detrás del gate porque un registro de
				// errores enseña rutas internas y a veces el nombre de un cliente.
				r.Get("/platform/logs", s.handlePlatformLogs)
				r.Get("/platform/logs/stats", s.handlePlatformLogStats)
				r.Get("/platform/audit", s.handlePlatformAudit)
				// Segunda persona para lo irreversible (mig 0046). Aprobar EJECUTA:
				// no queda una solicitud aprobada esperando a que alguien la lance.
				r.Get("/platform/approvals", s.handlePlatformApprovals)
				r.Post("/platform/approvals/{id}/approve", s.handlePlatformApprove)
				r.Post("/platform/approvals/{id}/reject", s.handlePlatformReject)
				r.Get("/platform/usage", s.handlePlatformUsage)
				// La cola de denuncias del operador vive en Cast y en Rooms; antes de
				// enseñar o tocar una, el servicio pregunta aquí y deja la traza.
				r.Post("/platform/moderation/access", s.handlePlatformModeracionAcceso)
				// Qué versión corre cada pieza de la suite. Detrás del gate de
				// plataforma como el resto: dice qué hay desplegado y dónde se quedó
				// a medias, que es reconocimiento útil para quien no debería tenerlo.
				r.Get("/platform/versions", s.handlePlatformVersions)
				httpx.GetYQuery(r, "/platform/orgs", s.handlePlatformListOrgs)
				r.Post("/platform/orgs", s.handlePlatformCreateOrg)
				r.Get("/platform/orgs/{id}", s.handlePlatformGetOrg)
				r.Post("/platform/orgs/{id}/status", s.handlePlatformSetOrgStatus)
				// Propietarios y cuentas de cualquier organización (platform_usuarios.go).
				// Lo que escribe pide step-up, como las mismas acciones en la consola
				// de una organización: aquí además valen para todas.
				r.Get("/platform/orgs/{id}/members", s.handlePlatformListMembers)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/owners", s.handlePlatformAssignOwner)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/owner-invitations", s.handlePlatformInviteOwner)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/members/{userID}/password", s.handlePlatformResetPassword)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/members/{userID}/block", s.handlePlatformBlockUser)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/members/{userID}/unblock", s.handlePlatformUnblockUser)
				r.With(s.requireStepUp).Post("/platform/orgs/{id}/members/{userID}/delete", s.handlePlatformDeleteUser)
				// Institución educativa (mig 0064): el Pro a mitad de precio. Lo concede
				// la plataforma con un motivo, como el resto de gestos comerciales.
				r.Post("/platform/orgs/{id}/education", s.handlePlatformSetOrgEducation)
				r.Post("/platform/orgs/{id}/seat-grants", s.handlePlatformGrantSeats)
				r.Delete("/platform/seat-grants/{id}", s.handlePlatformRevokeSeatGrant)
				r.Get("/platform/merges", s.handlePlatformListMerges)
				r.Post("/platform/merges", s.handlePlatformCreateMerge)
				r.Get("/platform/merges/{id}", s.handlePlatformGetMerge)
				r.Post("/platform/merges/{id}/resume", s.handlePlatformResumeMerge)
				r.Get("/platform/admins", s.handlePlatformListAdmins)
				r.Post("/platform/admins", s.handlePlatformGrantAdmin)
				r.Post("/platform/admins/{id}/review", s.handlePlatformReviewAdmin)
				r.Delete("/platform/admins/{id}", s.handlePlatformRevokeAdmin)
			})

			// Ciclo de vida de organizaciones, política de contraseñas y GESTIÓN DE ROLES:
			// RESERVADO a owner/admin (no delegable). roles.manage no se concede a roles
			// personalizados (anti auto-escalada); definir/asignar roles queda aquí.
			//
			// adminByUser limita POR PERSONA las operaciones que salen a la red (DNS y
			// la API de Cloudflare), no por IP: el bucket por IP es el del login, y
			// agotarlo desde esta pantalla dejaría sin entrar a toda una oficina detrás
			// del mismo NAT. Mismo criterio que el del asistente de alta.
			adminByUser := httpx.RateLimit(httpx.RateLimitConfig{
				Limiter: s.acctLimiter,
				KeyFunc: func(r *http.Request) string {
					c, _ := claimsFrom(r.Context())
					return "admin-domain:" + c.Subject
				},
			})
			r.Group(func(r chi.Router) {
				r.Use(s.requireRole("owner", "admin"))
				// Una cuenta personal no administra una organización: ver requireOrgKind.
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				// Las dos altas de organización son la misma: /orgs tenía su copia, sin
				// transacción —podía dejar una organización sin owner— y sin normalizar el slug.
				r.Post("/orgs", s.handleAdminCreateTenant)
				r.Post("/orgs/{slug}/members", s.handleAddMember)
				// Gestión de organizaciones desde Console: listar las que gestiona
				// (owner/admin, keyset), crear (creador⇒owner) y editar ajustes
				// (nombre/estado) con autorización por-org fail-closed. Slug inmutable.
				r.Get("/admin/tenants", s.handleAdminListTenants)
				r.Post("/admin/tenants", s.handleAdminCreateTenant)
				r.Patch("/admin/tenants/{slug}", s.handleAdminUpdateTenant)
				// Dominio de correo de la organización. Antes SOLO se podía conectar
				// dentro del asistente de alta, que se pasa una vez: quien compraba el
				// dominio después —lo normal— no tenía ninguna puerta. Ver
				// admin_domain.go. Reservado a owner/admin y no delegable: cambia la
				// dirección de TODA la plantilla y es una afirmación sobre la
				// titularidad de un dominio.
				//
				// Los dos que hablan hacia fuera —consulta DNS y API de Cloudflare—
				// van limitados por usuario, igual que en el asistente y por lo mismo:
				// agotar el bucket por IP dejaría fuera a toda una oficina detrás del
				// mismo NAT.
				r.Get("/admin/domain", s.handleAdminGetDomain)
				r.With(adminByUser, s.requireStepUp).Post("/admin/domain", s.handleAdminSetDomain)
				r.With(adminByUser, s.requireStepUp).Post("/admin/domain/cloudflare", s.handleAdminSetDomainCloudflare)
				r.With(adminByUser).Get("/admin/domain-availability", s.handleOnboardingDomainAvailability)
				r.With(s.requireStepUp).Delete("/admin/domain", s.handleAdminDeleteDomain)
				// LOS dominios, en plural (mig 0052). Las de arriba siguen hablando
				// del PRINCIPAL —son las que usa el asistente de alta, que solo
				// conoce uno— y estas gestionan el conjunto. Ver admin_maildomains.go.
				//
				// La lista consulta el DNS de los pendientes en cada carga, así que
				// también va limitada por persona: es igual de saliente que reclamar.
				r.With(adminByUser).Get("/admin/mail-domains", s.handleAdminListMailDomains)
				r.With(adminByUser, s.requireStepUp).Post("/admin/mail-domains", s.handleAdminAddMailDomain)
				r.Post("/admin/mail-domains/{id}/primary", s.handleAdminSetPrimaryMailDomain)
				r.With(s.requireStepUp).Delete("/admin/mail-domains/{id}", s.handleAdminDeleteMailDomain)
				// Logo de la organización. `?save=0` analiza y devuelve el reencuadre
				// SIN escribir: es lo que deja a la consola enseñar lo que va a quedar
				// antes de confirmarlo, en vez de que el administrador descubra
				// después que el sistema le recortó algo. Reservado a owner/admin
				// porque el logo lo ve toda la plantilla en las once superficies.
				r.Put("/admin/org/logo", s.handleAdminPutLogo)
				r.Delete("/admin/org/logo", s.handleAdminDeleteLogo)
				// Color por defecto de la organización: lo que ve quien no ha elegido
				// el suyo. Owner/admin por lo mismo que el logo.
				r.Put("/admin/org/color", s.handleAdminPutApariencia(ejeColor))
				// Y la forma («Marco» o «Carril»): el mismo trato.
				r.Put("/admin/org/forma", s.handleAdminPutApariencia(ejeForma))
				// Política de contraseñas de la org activa (se aplica en cada alta/cambio).
				r.Get("/admin/org/password-policy", s.handleGetPasswordPolicy)
				r.Patch("/admin/org/password-policy", s.handlePatchPasswordPolicy)
				// Roles personalizados / admin delegada: definir roles (nombre+permisos) y
				// asignarlos a usuarios. Solo owner/admin (anti auto-escalada).
				r.Get("/admin/roles", s.handleAdminListRoles)
				r.Post("/admin/roles", s.handleAdminCreateRole)
				r.Get("/admin/roles/{id}", s.handleAdminGetRole)
				r.Patch("/admin/roles/{id}", s.handleAdminUpdateRole)
				r.Delete("/admin/roles/{id}", s.handleAdminDeleteRole)
				r.Post("/admin/roles/{id}/members", s.handleAdminAssignRole)
				r.Delete("/admin/roles/{id}/members/{userID}", s.handleAdminUnassignRole)
				// PIM (mig 0020): acceso privilegiado just-in-time — administración. Definir
				// elegibilidades (quién eleva a qué rol, con/ sin aprobación y ventana máx.),
				// aprobar/denegar solicitudes y ver activaciones vigentes. Reservado a
				// owner/admin como la gestión de roles (gobernar el acceso privilegiado no
				// se delega). El enforcement de los perms elevados vive en el JWT/requirePermission.
				r.Get("/admin/pim/eligibilities", s.handleAdminListEligibilities)
				r.Post("/admin/pim/eligibilities", s.handleAdminCreateEligibility)
				r.Delete("/admin/pim/eligibilities/{id}", s.handleAdminDeleteEligibility)
				r.Get("/admin/pim/activations", s.handleAdminListActivations)
				r.Post("/admin/pim/activations/{id}/approve", s.handleAdminApproveActivation)
				r.Post("/admin/pim/activations/{id}/deny", s.handleAdminDenyActivation)
				// Etiquetas de sensibilidad (mig 0024): la TAXONOMÍA de clasificación de la
				// org. Gobierno de cumplimiento, como roles/PIM: NO delegable con el catálogo
				// FIJO de permisos actual, así que owner/admin. La lectura sí es abierta a
				// cualquier miembro (arriba). Cambiar la taxonomía NO reescribe lo etiquetado.
				r.Get("/admin/sensitivity-labels", s.handleAdminListSensitivityLabels)
				r.Post("/admin/sensitivity-labels", s.handleAdminCreateSensitivityLabel)
				r.Patch("/admin/sensitivity-labels/{id}", s.handleAdminUpdateSensitivityLabel)
				r.Delete("/admin/sensitivity-labels/{id}", s.handleAdminDeleteSensitivityLabel)
			})

			// Usuarios — LECTURA (directorio + sesiones): users.read o users.manage.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermUsersRead, domain.PermUsersManage))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				// Directorio de usuarios de la org activa (Console), keyset por
				// (created_at,id) DESC. Distinto del listado SCIM (RFC 7644).
				httpx.GetYQuery(r, "/admin/users", s.handleAdminListUsers)
				r.Get("/admin/users/{id}/sessions", s.handleAdminUserSessions)
				// Qué pasa de verdad si se toca a este usuario, ANTES de tocarlo.
				r.Get("/admin/users/{id}/impact", s.handleAdminUserImpact)
				// Y qué puede hacer de verdad: rol integrado + roles
				// personalizados + elevaciones PIM vivas, resueltos en una sola
				// respuesta y cada permiso con la fuente de la que sale.
				r.Get("/admin/users/{id}/effective-access", s.handleAdminUserEffectiveAccess)
				// Las cuentas dadas de baja que aún se pueden recuperar: sin
				// este listado la restauración exigiría saberse un uuid de
				// memoria, porque el directorio no las enseña. Ruta LITERAL
				// antes del comodín {id} (chi da prioridad a los estáticos, pero
				// se escribe aquí por legibilidad).
				r.Get("/admin/users/closed", s.handleAdminListClosedUsers)
				// Qué hay que mirar HOY en esta organización (portada accionable).
				r.Get("/admin/insights", s.handleAdminInsights)
				// Dispositivos de la org (vista admin, mig 0022): visibilidad de los
				// dispositivos registrados. Lectura como el directorio/sesiones.
				r.Get("/admin/devices", s.handleAdminListDevices)
			})

			// Usuarios — MUTACIONES (alta, ciclo de vida, membresía, credenciales): users.manage.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermUsersManage))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				// Invitar por email a la org activa (alta de usuario): emite un token
				// firmado de un solo uso y envía el enlace de aceptación.
				r.Post("/admin/invitations", s.handleAdminInviteUser)
				// Y verlas: sin listado, la consola no podía ofrecer revocar —no
				// tenía de dónde sacar el correo de una invitación pendiente—.
				r.Get("/admin/invitations", s.handleAdminListInvitations)
				// Resolución/revocación auxiliares para superficies de la suite que
				// delegan la invitación canónica en Sytue ID (p. ej. Cloud).
				r.Post("/admin/members:resolve", s.handleAdminResolveMemberByEmail)
				r.Post("/admin/invitations:revoke", s.handleAdminRevokeInvitation)
				// Ciclo de vida y sesiones de un usuario de la org activa.
				r.Post("/admin/users/{id}/suspend", s.handleAdminSuspendUser)
				r.Post("/admin/users/{id}/reactivate", s.handleAdminReactivateUser)
				// Membresía en la org activa: cambiar rol y quitar (revoca sesiones).
				r.Post("/admin/users/{id}/role", s.handleAdminSetMemberRole)
				// Varias personas a la vez (baja de departamento, incidente). El
				// resultado va por usuario: en un lote, el fallo de uno es un dato.
				r.Post("/admin/users/bulk", s.handleAdminBulkUsers)
				// Y el lote como TRABAJO: mirarlo mientras corre y pararlo. Sin
				// esto, equivocarse de selección no tenía más salida que cerrar
				// la pestaña, que no para nada.
				r.Get("/admin/jobs", s.handleAdminListJobs)
				r.Get("/admin/jobs/{id}", s.handleAdminGetJob)
				r.Post("/admin/jobs/{id}/cancel", s.handleAdminCancelJob)
				r.With(s.requireStepUp).Delete("/admin/users/{id}/membership", s.handleAdminRemoveMember)
				// Baja de la CUENTA (no solo de la membresía) y su vuelta atrás.
				// Antes esto solo lo podía hacer el desaprovisionamiento SCIM, y
				// era de un solo sentido. Las dos piden reautenticación: una
				// apaga una cuenta y la otra resucita datos apagados.
				r.With(s.requireStepUp).Post("/admin/users/{id}/close", s.handleAdminCloseUser)
				r.With(s.requireStepUp).Post("/admin/users/{id}/restore", s.handleAdminRestoreUser)
				// Atributos de perfil (department/title/location) que alimentan las reglas
				// de grupos dinámicos; al fijarlos se reevalúa la membresía dinámica del usuario.
				r.Patch("/admin/users/{id}/profile", s.handleAdminSetUserProfile)
				// Direcciones de correo de la organización: la que se asignó sola al dar
				// de alta, su cambio y los alias. Ver admin_mailboxes.go.
				r.Get("/admin/mailboxes", s.handleAdminListMailboxes)
				r.Post("/admin/users/{id}/mailbox", s.handleAdminSetMailbox)
				r.Post("/admin/users/{id}/aliases", s.handleAdminAddMailboxAlias)
				r.Delete("/admin/mailboxes/{mailboxID}", s.handleAdminDeleteMailboxAlias)
				// Reset de contraseña por admin: fija la dada (handleAdminSetPassword) o
				// dispara el enlace por email (handleAdminTriggerReset). Auditadas; la
				// contraseña no se registra.
				r.With(s.requireStepUp).Post("/admin/users/{id}/password", s.handleAdminSetPassword)
				r.With(s.requireStepUp).Post("/admin/users/{id}/reset-password", s.handleAdminTriggerReset)
				// Reset del segundo factor (TOTP + passkeys + recovery) para re-enrolar.
				r.With(s.requireStepUp).Post("/admin/users/{id}/reset-mfa", s.handleAdminResetMFA)
				r.Post("/admin/users/{id}/sessions/revoke", s.handleAdminRevokeUserSessions)
				// Revocar (borrar) un dispositivo de la org (mig 0022): aislamiento por org.
				r.Delete("/admin/devices/{id}", s.handleAdminRevokeDevice)
			})

			// Grupos de la org activa (CRUD + membresía): groups.manage. Base del claim
			// `groups` del JWT (share por-grupo de Drive). Aislamiento por org del token.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermGroupsManage))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				r.Get("/admin/groups", s.handleAdminListGroups)
				r.Post("/admin/groups", s.handleAdminCreateGroup)
				// Previsualiza a quién incluiría una regla dinámica antes de guardarla.
				r.Post("/admin/groups/preview", s.handleAdminPreviewDynamicGroup)
				// Recálculo de la membresía dinámica contra el estado actual: repara la
				// deriva sin esperar al barrido periódico (toda la org / un grupo).
				r.Post("/admin/groups/recompute", s.handleAdminRecomputeGroups)
				r.Get("/admin/groups/{id}", s.handleAdminGetGroup)
				r.Post("/admin/groups/{id}/recompute", s.handleAdminRecomputeGroup)
				r.Patch("/admin/groups/{id}", s.handleAdminUpdateGroup)
				r.Delete("/admin/groups/{id}", s.handleAdminDeleteGroup)
				r.Post("/admin/groups/{id}/members", s.handleAdminAddGroupMember)
				r.Delete("/admin/groups/{id}/members/{userID}", s.handleAdminRemoveGroupMember)
			})

			// Log de auditoría de la org activa: audit.view. Listado keyset
			// (occurred_at,id) DESC con filtros, y exportación CSV en streaming.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermAuditView))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				r.Get("/admin/audit", s.handleAdminListAudit)
				r.With(s.requireStepUp).Get("/admin/audit/export.csv", s.handleAdminExportAudit)
				// Destino SIEM: la auditoría en PUSH, además del export bajo demanda.
				r.Get("/admin/audit/sink", s.handleAdminGetAuditSink)
				r.With(s.requireStepUp).Put("/admin/audit/sink", s.handleAdminSetAuditSink)
				r.Delete("/admin/audit/sink", s.handleAdminDeleteAuditSink)
			})

			// Inicio de sesión único (SSO / federación OIDC upstream) de la org activa:
			// sso.manage. CRUD de identity_providers, discovery SSRF-safe y prueba. El
			// client_secret solo se escribe; nunca se devuelve. Aislamiento por org.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermSSOManage))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				r.Get("/admin/sso/providers", s.handleAdminListProviders)
				r.With(s.requireStepUp).Post("/admin/sso/providers", s.handleAdminCreateProvider)
				r.Get("/admin/sso/providers/{id}", s.handleAdminGetProvider)
				r.With(s.requireStepUp).Patch("/admin/sso/providers/{id}", s.handleAdminUpdateProvider)
				r.Post("/admin/sso/providers/{id}/test", s.handleAdminTestProvider)
				r.With(s.requireStepUp).Delete("/admin/sso/providers/{id}", s.handleAdminDeleteProvider)
				// Estado REAL del aprovisionamiento SCIM de esta organización, para
				// sustituir al distintivo «SCIM activo» que estaba escrito en el JSX y
				// no consultaba nada. Va con sso.manage porque es la otra mitad de la
				// conexión con el IdP corporativo. Ver admin_scim.go.
				r.Get("/admin/scim/status", s.handleAdminSCIMStatus)
				// El token de aprovisionamiento de ESTA organización, creado desde
				// su propia consola: antes era una variable de entorno del
				// despliegue, así que conectar Entra u Okta era un ticket a Sytue.
				r.With(s.requireStepUp).Post("/admin/scim/tokens", s.handleAdminCreateSCIMToken)
				r.Delete("/admin/scim/tokens/{id}", s.handleAdminRevokeSCIMToken)
			})

			// Acceso Condicional (paridad Entra): access_policies.manage. Políticas
			// evaluadas en el login (block / require_mfa / allow según usuarios·grupos·IP·
			// app·horario, con exclusiones break-glass). CRUD org-scoped fail-closed; el
			// enforcement vive en el propio flujo de login (no en el cliente).
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermAccessPoliciesManage))
				r.Use(s.requireOrgKind(domain.OrgKindOrg))
				r.Get("/admin/access-policies", s.handleAdminListAccessPolicies)
				r.Post("/admin/access-policies", s.handleAdminCreateAccessPolicy)
				r.Get("/admin/access-policies/{id}", s.handleAdminGetAccessPolicy)
				r.Patch("/admin/access-policies/{id}", s.handleAdminUpdateAccessPolicy)
				r.Delete("/admin/access-policies/{id}", s.handleAdminDeleteAccessPolicy)
			})

			// Apps de terceros (identidad SALIENTE): apps.manage. SP SAML registrados por
			// la org (Sytue ID como IdP) y tokens de acceso inicial del registro dinámico
			// OIDC (RFC 7591). Aislamiento por org (activa) fail-closed.
			r.Group(func(r chi.Router) {
				r.Use(s.requirePermission(domain.PermAppsManage))
				r.Get("/admin/apps/saml-sps", s.handleAdminListSAMLSPs)
				r.Post("/admin/apps/saml-sps", s.handleAdminCreateSAMLSP)
				r.Get("/admin/apps/saml-sps/{id}", s.handleAdminGetSAMLSP)
				r.Patch("/admin/apps/saml-sps/{id}", s.handleAdminUpdateSAMLSP)
				r.Delete("/admin/apps/saml-sps/{id}", s.handleAdminDeleteSAMLSP)
				r.With(s.requireStepUp).Post("/admin/apps/dcr-tokens", s.handleAdminCreateDCRToken)
				// Clients OIDC de la org creados desde la Console (la app de terceros
				// no necesita saber hablar RFC 7591). Secreto visible UNA sola vez.
				r.Get("/admin/apps/oidc-clients", s.handleAdminListOIDCClients)
				r.Post("/admin/apps/oidc-clients", s.handleAdminCreateOIDCClient)
				r.With(s.requireStepUp).Post("/admin/apps/oidc-clients/{id}/rotate-secret", s.handleAdminRotateOIDCClientSecret)
				r.Delete("/admin/apps/oidc-clients/{id}", s.handleAdminDeleteOIDCClient)
			})
		})
	})

	return r
}
