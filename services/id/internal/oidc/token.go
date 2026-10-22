// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// TokenClaims describe el sujeto y autorización a embeber en un JWT.
type TokenClaims struct {
	Subject  string
	Audience string   // client_id (access token) o el del ID token
	Scope    string   // scopes separados por espacio (access token)
	Tenant   string   // slug de la organización activa
	Roles    []string // roles en el tenant (owner|admin|member|billing)
	Perms    []string // permisos granulares efectivos del sujeto (roles personalizados, mig 0019)
	Groups   []string // nombres de los grupos del sujeto en el tenant (share por-grupo de Drive)
	// OrgLogo es el hash del logotipo propio de la organización, vacío si no tiene.
	// No es el logo: es lo que permite componer su URL (`?v=<hash>`) y servirlo como
	// inmutable durante un año sin dejar de verse al minuto siguiente de cambiarlo.
	// Va SOLO en el ID token —quien lo necesita es la barra superior, no un servicio
	// aguas abajo— y por eso mismo no engorda el access token, que viaja en cada
	// llamada a la API.
	OrgLogo string
	// OrgColor es el color por defecto de la organización, vacío si es el de la
	// casa. SOLO en el ID token, como org_logo y por lo mismo: lo consume la web
	// para pintar, no un servicio para decidir.
	OrgColor string
	// OrgForma es la forma por defecto de la organización, vacía si es la de la
	// casa. SOLO en el ID token, como org_color.
	OrgForma string
	// AvatarVersion es la marca de actualización del avatar PROPIO del usuario
	// (updated_at de user_avatars, vacío si no ha puesto ninguno). Con él la
	// barra superior compone el `picture` (`/api/v1/me/avatar?v=<versión>`) y el
	// cambio de foto se ve al instante en todas las apps sin esperar a que
	// expire la caché. Va SOLO en el ID token —el consumidor es la barra
	// superior, igual que org_logo— y el access token no engorda.
	AvatarVersion string
	// OrgKind, Plan, StorageBytes y MailboxBytes describen la organización
	// activa: su clase ("org" | "personal"), su contrato y las cuotas que ese
	// contrato concede. Van en el ACCESS token porque son los servicios quienes
	// las aplican —Drive fija su cuota, Mail la del buzón, SyERP y Sytue Admin
	// rechazan a las personales— y en el ID token porque el portal decide con
	// ellas qué teselas pinta. Cuota 0 = «el servicio aplica su defecto».
	OrgKind      string
	Plan         string
	StorageBytes int64
	MailboxBytes int64
	Nonce        string // ID token (replay de la petición de autorización)
	AuthTime     time.Time
	AMR          []string // métodos de autenticación (pwd|mfa|webauthn)
	// Actor es el client que obtuvo el token ACTUANDO EN NOMBRE del sujeto
	// (RFC 8693 §4.1, claim `act`). Vacío en la emisión normal: el sujeto se
	// autenticó él mismo.
	Actor string
	// TTL acorta la vida del access token (impersonación). Solo puede ACORTAR:
	// 0 —o un valor mayor que el TTL configurado— deja el configurado.
	TTL time.Duration
	// Perfil (ID token, scope profile/email).
	Email         string
	EmailVerified bool
	Name          string
	// Mailbox es la dirección de correo del sujeto EN LA ORGANIZACIÓN ACTIVA
	// (nombre.apellidos@dominio-de-la-org), que no tiene por qué coincidir con
	// Email: el correo de registro es su login global en la suite y puede ser
	// personal o de otra empresa. Sytue Mail aprovisiona el buzón con ESTA y no
	// con aquella, porque una cuenta en un dominio que nuestro MX no sirve no
	// recibiría nada. Vacío si la organización aún no reparte buzones.
	Mailbox string
	// Mailboxes son sus direcciones principales en TODAS sus organizaciones activas
	// (claim `mailboxes`): con ellas el Hub le enseña sus avisos de cualquiera (ADR 0053).
	Mailboxes []string
	// MailDomains son los dominios de correo VERIFICADOS de la organización
	// activa. Viajan porque el resto de la suite no puede deducirlos: de una
	// dirección se sabe su dominio, no los OTROS que sirve esa organización, y sin
	// la lista Sytue Mail rechazaba como ajena una dirección en un dominio que el
	// cliente acababa de conectar. Vacío si la organización no tiene ninguno
	// propio: entonces su correo vive en el subdominio de la suite, que se deduce
	// del tenant y no hace falta enumerar.
	MailDomains []string
	// ExtraAudience son audiencias ADICIONALES del access token. La única hoy es la
	// de las APIs de Google emuladas (domain.GoogleAPIsAudience), que se añade solo
	// cuando el token lleva scopes de Google: es la que abre las rutas /compat/google
	// de los productos y ninguna otra.
	ExtraAudience []string
	// HostedDomain es el claim `hd` de Google: el dominio de la organización. Solo
	// para organizaciones con dominio verificado; una cuenta personal no lo lleva,
	// igual que una cuenta de gmail.com.
	HostedDomain string
}

// Issuer emite tokens firmados con la clave activa del KeyManager.
type Issuer struct {
	km             *KeyManager
	issuer         string
	resourceAud    string
	accessTokenTTL time.Duration
	idTokenTTL     time.Duration
}

// NewIssuer construye un Issuer. resourceAud es el identificador canónico del
// resource server (la API de identidad); se añade como audiencia adicional de los
// access tokens para que los servicios consumidores puedan exigir `aud` (RFC 9068
// §4). Si está vacío, se usa el issuer como identificador del recurso.
func NewIssuer(km *KeyManager, issuer, resourceAud string, accessTTL, idTTL time.Duration) *Issuer {
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if idTTL <= 0 {
		idTTL = time.Hour
	}
	if resourceAud == "" {
		resourceAud = issuer
	}
	return &Issuer{km: km, issuer: issuer, resourceAud: resourceAud, accessTokenTTL: accessTTL, idTokenTTL: idTTL}
}

// AccessTokenTTL expone la vida del access token (para la respuesta `expires_in`).
func (i *Issuer) AccessTokenTTL() time.Duration { return i.accessTokenTTL }

// IssueAccessToken emite un access token JWT (RFC 9068-style) firmado.
//
// El `aud` incluye SIEMPRE el identificador del resource server (resourceAud) y,
// si difiere, el client destinatario. Así el resource server (la API de identidad
// u otros servicios que compartan este identificador) puede exigir su audiencia
// (RFC 9068 §4, ASVS L2 §3.5) sin rechazar tokens legítimos por client.
func (i *Issuer) IssueAccessToken(c TokenClaims, now time.Time) (string, error) {
	b := jwt.NewBuilder().
		Issuer(i.issuer).
		Subject(c.Subject).
		Audience(accessAudience(i.resourceAud, c.Audience, c.ExtraAudience...)).
		IssuedAt(now).
		NotBefore(now).
		Expiration(now.Add(i.EffectiveAccessTokenTTL(c.TTL))).
		JwtID(uuid.NewString()).
		Claim("scope", c.Scope).
		Claim("typ", "at+jwt")
	// `act` (RFC 8693 §4.1): quién obtuvo este token en nombre del sujeto. Su
	// presencia MARCA el token como delegado y el token endpoint lo rechaza como
	// subject_token: la delegación no se encadena.
	if c.Actor != "" {
		b = b.Claim("act", map[string]any{"sub": c.Actor})
	}
	if c.Tenant != "" {
		b = b.Claim("tenant", c.Tenant)
	}
	// `amr` también en el ACCESS token, no solo en el ID Token. La administración
	// de plataforma exige SIEMPRE segundo factor y su gate vive en la API, que solo
	// ve el Bearer: sin este claim no habría forma de distinguir un token nacido de
	// una sesión con MFA de otro nacido de solo contraseña.
	if len(c.AMR) > 0 {
		b = b.Claim("amr", c.AMR)
	}
	// `auth_time` VA CON `amr`, y por el mismo motivo: el gate está en la API y
	// la API solo ve el Bearer.
	//
	// Faltaba, y no era cosmético. `requireStepUp` (step-up de las acciones
	// sensibles) exige LAS DOS cosas —segundo factor y haberse identificado hace
	// menos de cinco minutos— y lee las dos del access token. Con `auth_time`
	// solo en el ID Token, `c.AuthTime` llegaba SIEMPRE a cero, la condición
	// `fresca` era SIEMPRE falsa y las veinte rutas protegidas quedaban cerradas
	// para todo el mundo: también para un owner con MFA recién identificado.
	// `buildTokens` ya venía pasando el dato (`common.AuthTime = authTime`); lo
	// que no había era quien lo escribiera aquí.
	//
	// Se vio porque tumbó doce tests de integración con
	// `insufficient_user_authentication`, no porque nadie pudiera usar la
	// consola: el CI de integración no corre en pull request, así que el fallo
	// viajó a `main` y a cinco tags antes de salir.
	if !c.AuthTime.IsZero() {
		b = b.Claim("auth_time", c.AuthTime.Unix())
	}
	if len(c.Roles) > 0 {
		b = b.Claim("roles", c.Roles)
	}
	// `perms` (permisos granulares) en el ACCESS token: es la superficie de
	// enforcement (requirePermission lo lee del Bearer). ADITIVO a `roles` (no la
	// toca): los servicios que aún no opten simplemente lo ignoran. Se omite si el
	// sujeto no tiene permisos delegados (owner/admin no lo necesitan; no emitir vacíos).
	if len(c.Perms) > 0 {
		b = b.Claim("perms", c.Perms)
	}
	// `groups` (nombres) en el ACCESS token: Sytue Drive lo lee del Bearer para su
	// share por-grupo. Se omite si el sujeto no tiene grupos (no emitir vacíos).
	if len(c.Groups) > 0 {
		b = b.Claim("groups", c.Groups)
	}
	// Identidad de perfil también en el ACCESS token, condicionada al scope
	// concedido (RFC 9068 admite claims adicionales): los resource servers
	// first-party de la suite (Mail aprovisiona la cuenta con la dirección,
	// Chat muestra el nombre del autor) la leen del access token, no del ID token.
	if c.Email != "" && hasScope(c.Scope, "email") {
		b = b.Claim("email", c.Email).Claim("email_verified", c.EmailVerified)
	}
	// `mailbox` va bajo el mismo scope que `email` porque es el mismo tipo de dato
	// —una dirección de correo del sujeto— y quien lo consume (Sytue Mail) ya pide
	// ese scope para leer `email`. Un claim propio, y no reescribir `email`, porque
	// las dos direcciones significan cosas distintas y hay quien necesita las dos:
	// el login sigue siendo el correo de registro.
	// Nunca en un token delegado: quien actúa en nombre de otro no tiene por qué
	// saber en qué otras organizaciones está (ADR 0040, 0053).
	if len(c.Mailboxes) > 0 && hasScope(c.Scope, "email") && c.Actor == "" {
		b = b.Claim("mailboxes", c.Mailboxes)
	}
	if c.Mailbox != "" && hasScope(c.Scope, "email") {
		b = b.Claim("mailbox", c.Mailbox)
	}
	// Bajo el mismo scope y por el mismo motivo que `mailbox`: son direcciones de
	// correo del sujeto, y quien los consume ya pide `email`.
	if len(c.MailDomains) > 0 && hasScope(c.Scope, "email") {
		b = b.Claim("mail_domains", c.MailDomains)
	}
	if c.Name != "" && hasScope(c.Scope, "profile") {
		b = b.Claim("name", c.Name)
	}
	b = withPlanClaims(b, c)
	tok, err := b.Build()
	if err != nil {
		return "", err
	}
	return i.sign(tok)
}

// withPlanClaims añade la clase de la organización, su plan y sus cuotas. Solo
// lo que hay: un token sin organización activa no lleva ninguno, y las cuotas a
// cero no se publican porque «cero» significaría «sin espacio», no «por defecto».
func withPlanClaims(b *jwt.Builder, c TokenClaims) *jwt.Builder {
	if c.OrgKind != "" {
		b = b.Claim("org_kind", c.OrgKind)
	}
	if c.Plan != "" {
		b = b.Claim("plan", c.Plan)
	}
	if c.StorageBytes > 0 {
		b = b.Claim("storage_bytes", c.StorageBytes)
	}
	if c.MailboxBytes > 0 {
		b = b.Claim("mailbox_bytes", c.MailboxBytes)
	}
	return b
}

// EffectiveAccessTokenTTL aplica el TTL pedido SOLO si acorta el configurado: un
// llamante no puede ALARGAR la vida de un token pidiendo más. La expone el
// emisor para que quien encarga el token (p. ej. el canje RFC 8693) pueda decir
// la verdad en `expires_in` sin reimplementar la regla.
func (i *Issuer) EffectiveAccessTokenTTL(want time.Duration) time.Duration {
	if want > 0 && want < i.accessTokenTTL {
		return want
	}
	return i.accessTokenTTL
}

// hasScope indica si un scope concreto figura en la lista (separada por espacios).
func hasScope(scope, want string) bool {
	for _, s := range strings.Fields(scope) {
		if s == want {
			return true
		}
	}
	return false
}

// accessAudience compone el `aud` de un access token: el resource server siempre,
// el client destinatario si es distinto y no vacío, y las extras al final (sin
// duplicados). El ORDEN importa: quien busca el client lo toma de la primera
// entrada que no es el resource server.
func accessAudience(resourceAud, clientID string, extra ...string) []string {
	aud := []string{resourceAud}
	for _, a := range append([]string{clientID}, extra...) {
		if a != "" && !slices.Contains(aud, a) {
			aud = append(aud, a)
		}
	}
	return aud
}

// IssueIDToken emite un ID Token OIDC firmado.
func (i *Issuer) IssueIDToken(c TokenClaims, now time.Time) (string, error) {
	b := jwt.NewBuilder().
		Issuer(i.issuer).
		Subject(c.Subject).
		Audience([]string{c.Audience}).
		IssuedAt(now).
		Expiration(now.Add(i.idTokenTTL)).
		Claim("auth_time", c.AuthTime.Unix())
	if c.Nonce != "" {
		b = b.Claim("nonce", c.Nonce)
	}
	if len(c.AMR) > 0 {
		b = b.Claim("amr", c.AMR)
	}
	if c.Tenant != "" {
		b = b.Claim("tenant", c.Tenant)
	}
	if c.HostedDomain != "" {
		b = b.Claim("hd", c.HostedDomain)
	}
	if c.OrgLogo != "" {
		b = b.Claim("org_logo", c.OrgLogo)
	}
	if c.OrgColor != "" {
		b = b.Claim("org_color", c.OrgColor)
	}
	if c.OrgForma != "" {
		b = b.Claim("org_forma", c.OrgForma)
	}
	b = withPlanClaims(b, c)
	if len(c.Roles) > 0 {
		b = b.Claim("roles", c.Roles)
	}
	if len(c.Groups) > 0 {
		b = b.Claim("groups", c.Groups)
	}
	if c.Email != "" {
		b = b.Claim("email", c.Email).Claim("email_verified", c.EmailVerified)
	}
	if c.Name != "" {
		b = b.Claim("name", c.Name)
	}
	// `picture` (OIDC Core §5.2.4): URL absoluta del avatar propio, versionada
	// con su updated_at — al cambiar la foto cambia la URL y todas las apps la
	// ven al refrescar la sesión, sin pujar por la caché.
	if c.AvatarVersion != "" {
		b = b.Claim("picture", PictureURL(i.issuer, c.AvatarVersion))
	}
	tok, err := b.Build()
	if err != nil {
		return "", err
	}
	return i.sign(tok)
}

// PictureURL es el claim `picture`: la foto propia servida por la COOKIE de sesión
// de Sytue ID (`/avatar`), con la versión en la URL. Es la misma en el id_token y
// en el userinfo.
//
// `/avatar` y no `/account/avatar`: el `/account` de `id.<dominio>` se lo quedaba
// la app «Mi cuenta» por el Ingress comodín hasta el 2026-09-24, y una URL fuera de
// `/account` no depende de ese reparto. Las apps de la suite no la piden: su BFF se
// queda con la versión y trae la foto con el token (`@sytue/auth`, `/auth/avatar`).
func PictureURL(issuer, version string) string {
	return strings.TrimRight(issuer, "/") + "/avatar?v=" + url.QueryEscape(version)
}
