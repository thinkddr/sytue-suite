// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos de dominio nucleares de Sytue ID: usuarios,
// organizaciones (tenants), membresías/roles, grupos, clients OAuth2/OIDC,
// sesiones, tokens y credenciales MFA/WebAuthn. Es independiente del transporte
// (HTTP/SCIM) y del almacenamiento (PostgreSQL): contiene solo reglas e
// invariantes de identidad.
//
// Clean-room: modelado a partir de los estándares OIDC Core 1.0, OAuth2
// (RFC 6749/7636), SCIM 2.0 (RFC 7643/7644) y WebAuthn L2. No deriva de ningún
// motor AGPL (Keycloak queda fuera, docs/CONVENTIONS.md §3).
package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
)

// SanitizeDisplayName normaliza un nombre visible de usuario para que sea seguro
// de mostrar en cualquier superficie: recorta espacios, elimina caracteres de
// control (incluidos los separadores de línea/párrafo U+2028/U+2029) y limita la
// longitud. Es defensa en profundidad frente a inyección en contextos HTML/JS;
// el escape en el punto de salida sigue siendo obligatorio.
func SanitizeDisplayName(name string) string {
	const maxLen = 256
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r == '\u2028' || r == '\u2029' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}

// ProfileEdit son los campos de perfil que el propio usuario edita desde
// /account. Es un tipo aparte de User a propósito: lo que no está aquí no se
// puede tocar por esa vía, así que la frontera entre «lo que edita su dueño» y
// «lo que solo cambian un administrador o SCIM» es visible en el tipo y no
// depende de que cada handler recuerde filtrar campos.
type ProfileEdit struct {
	Name       string
	GivenName  string
	FamilyName string
	Title      string
	Department string
	Location   string
	Locale     string
}

// Locales son los idiomas de producto (docs/CONVENTIONS.md §13.1): inglés base
// más los cuatro de la UE. Mismo conjunto y mismo orden canónico que
// apps/frontend/packages/i18n (el base primero, el resto alfabético).
var Locales = []string{"en", "de", "es", "fr", "it"}

// LocaleNames da el nombre de cada idioma EN SU PROPIO IDIOMA. No se traduce
// nunca: es lo que permite volver a tu idioma a quien no entiende el actual.
var LocaleNames = map[string]string{
	"en": "English",
	"de": "Deutsch",
	"es": "Español",
	"fr": "Français",
	"it": "Italiano",
}

// NormalizeLocale reduce lo que llegue a uno de los idiomas soportados. Acepta
// etiquetas BCP-47 completas ("es-ES", "en-GB") quedándose con la subetiqueta
// de idioma, porque es lo que mandan los navegadores y lo que puede traer un
// IdP externo por SCIM. Lo que no reconoce cae al inglés en vez de guardarse
// tal cual: una columna de idioma con un valor que ningún catálogo entiende
// deja la interfaz sin traducir y sin explicación.
func NormalizeLocale(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if base, _, ok := strings.Cut(v, "-"); ok {
		v = base
	}
	for _, l := range Locales {
		if v == l {
			return v
		}
	}
	return "en"
}

// Role es un rol canónico dentro de una organización (docs/CONVENTIONS.md §8).
type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleMember  Role = "member"
	RoleBilling Role = "billing"
)

// Valid indica si el rol es uno de los canónicos.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleBilling:
		return true
	default:
		return false
	}
}

// RequiresMFA indica si el rol obliga a MFA/WebAuthn (owner/admin, §8).
func (r Role) RequiresMFA() bool { return r == RoleOwner || r == RoleAdmin }

// Errores de dominio reutilizables.
var (
	ErrNotFound     = errors.New("domain: recurso no encontrado")
	ErrConflict     = errors.New("domain: conflicto de estado")
	ErrInvalidInput = errors.New("domain: entrada inválida")
	// ErrAliasReservado: la dirección suplantaría a la plataforma en el dominio raíz
	// (admin@, soporte@, sytue-*@). La lista vive en platform/pkg/mailaddr.
	ErrAliasReservado = errors.New("domain: alias reservado")
	ErrInvalidClient  = errors.New("domain: client inválido")
	ErrInvalidGrant   = errors.New("domain: grant inválido")
	ErrAccessDenied   = errors.New("domain: acceso denegado")
	ErrMFARequired    = errors.New("domain: MFA requerido")
	ErrInvalidCreds   = errors.New("domain: credenciales inválidas")
	ErrUserSuspended  = errors.New("domain: usuario suspendido")
	ErrExpired        = errors.New("domain: expirado")
	ErrReuseDetected  = errors.New("domain: reutilización de token detectada")
)

// Status es el estado de ciclo de vida de usuarios y organizaciones.
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

// Organization es un tenant. slug ⇄ grupo `org/<slug>` en Sytue ID.
type Organization struct {
	ID     string
	Slug   string
	Name   string
	Status Status
	// Kind distingue una organización de verdad (OrgKindOrg) de una cuenta
	// personal (OrgKindPersonal): una persona sola, cuya organización es su
	// correo y que no tiene Sytue Admin ni SyERP. Vacío se lee como OrgKindOrg.
	Kind OrgKind
	// Education marca a una institución educativa: tiene el plan Pro con
	// commerce.EducationDiscountPercent de descuento. Lo concede la plataforma.
	Education bool
	// Color es el color de la suite que ve quien no ha elegido el suyo. Lo fija
	// el administrador; uno de OrgColors.
	Color string
	// Forma es la forma de la suite («Marco» o «Carril») que ve quien no ha elegido
	// la suya. La fija el administrador; una de OrgFormas.
	Forma     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrgColorDefault es el color de una organización que no ha elegido ninguno:
// cada app usa su color. Los clientes lo aplican también sin claim.
const OrgColorDefault = "app"

// OrgColors son los colores que la suite ofrece (ADR 0071). La lista vive aquí y
// no en un CHECK de la base: cada valor es un bloque `[data-color='…']` de
// branding/tokens/colors.css, y `TestLaAparienciaDeOrganizacionExisteEnLaHoja`
// falla si uno de los dos lados se queda atrás.
var OrgColors = []string{"petroleo", "cobalto", OrgColorDefault}

// ColorClaim es lo que viaja en el token: nada si es el predeterminado. Petróleo
// sí viaja: sin él los clientes pintarían el de cada app y perderían esa elección.
func (o Organization) ColorClaim() string { return claimDeApariencia(o.Color, OrgColorDefault) }

// OrgFormaDefault es la forma de una organización que no ha elegido ninguna, y la
// que la web pinta sin atributo: por eso NO viaja en el token.
const OrgFormaDefault = "marco"

// OrgFormas son las formas que la suite ofrece. Como OrgColors, la lista vive aquí
// y no en un CHECK: cada alternativa es un bloque `[data-forma='…']` de
// branding/tokens/appearance.css, y `TestLaAparienciaDeOrganizacionExisteEnLaHoja`
// falla si uno de los dos lados se queda atrás.
var OrgFormas = []string{OrgFormaDefault, "carril"}

// FormaClaim es lo que viaja en el token: nada si la forma es la de la casa.
func (o Organization) FormaClaim() string { return claimDeApariencia(o.Forma, OrgFormaDefault) }

// claimDeApariencia calla el predeterminado: un token sin el claim ya lo dice.
func claimDeApariencia(valor, predeterminado string) string {
	if valor == predeterminado {
		return ""
	}
	return valor
}

// OrgKind es la clase de una organización.
type OrgKind string

const (
	OrgKindOrg      OrgKind = "org"
	OrgKindPersonal OrgKind = "personal"
)

// Personal indica si la organización es una cuenta personal.
func (o Organization) Personal() bool { return o.Kind == OrgKindPersonal }

// User es un usuario de la suite (login global por email).
type User struct {
	ID    string
	Email string
	// Username es el @usuario, global y único (mig 0065, ADR 0040). Vacío al
	// crear = CreateUser le asigna uno libre derivado del nombre.
	Username      string
	EmailVerified bool
	// Phone es el móvil verificado por SMS en el alta, en E.164 (mig 0066, ADR
	// 0041). RecoveryEmail, el correo de recuperación que la persona confirmó en
	// «Mi cuenta». Los dos son identificadores de acceso y "" si no hay.
	Phone         string
	RecoveryEmail string
	Name          string
	GivenName     string
	FamilyName    string
	Locale        string
	PasswordHash  string // Argon2id (PHC). Vacío si solo MFA/SSO.
	MFAEnabled    bool
	Status        Status
	ExternalID    string // identificador del IdP origen (SCIM)
	// Atributos de perfil que alimentan las reglas de grupos dinámicos (mig 0021).
	// Nullable en BD; "" cuando no están fijados.
	Department string
	Title      string
	Location   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// PasswordMustChange: la contraseña es TEMPORAL (la fijó un operador de
	// plataforma) y hay que cambiarla al entrar con ella (mig 0113).
	PasswordMustChange bool
}

// Active indica si el usuario puede autenticarse.
func (u User) Active() bool { return u.Status == StatusActive }

// Membership asocia un usuario a una organización con un rol.
type Membership struct {
	OrgID     string
	UserID    string
	Role      Role
	CreatedAt time.Time
}

// Group es un grupo arbitrario dentro de una organización (más allá del rol
// primario). Su Name es único por-tenant (UNIQUE(org_id,name), mig 0001) y es la
// forma que Sytue Drive espera en el claim `groups` del JWT para su share por-grupo.
type Group struct {
	ID          string
	OrgID       string
	Name        string
	Description string
	ExternalID  string
	// MembershipRule, si no es nil, marca el grupo como DINÁMICO: su membresía la
	// calcula la regla (mig 0021) y NO admite asignación manual. nil ⇒ grupo manual.
	MembershipRule *Rule
	CreatedAt      time.Time
}

// Dynamic indica si la membresía del grupo la gobierna una regla (grupo dinámico).
func (g Group) Dynamic() bool { return g.MembershipRule != nil }

// ClientType distingue clients confidenciales (con secreto) de públicos (PKCE puro).
type ClientType string

const (
	ClientConfidential ClientType = "confidential"
	ClientPublic       ClientType = "public"
)

// Client es un registro de client OAuth2/OIDC (BFF por superficie).
type Client struct {
	ID                      string
	SecretHash              string // Argon2id; vacío para públicos o sin secreto fijado
	Name                    string
	Type                    ClientType
	RedirectURIs            []string
	PostLogoutRedirectURIs  []string
	GrantTypes              []string
	ResponseTypes           []string
	Scopes                  []string
	RequirePKCE             bool
	TokenEndpointAuthMethod string
	// OrgID acota el client a una organización (mig 0017). Vacío = client GLOBAL
	// de la suite (first-party). Gobierna sobre quién puede impersonar.
	OrgID string
	// MayImpersonate / MayImpersonateAdmins son la puerta DENY-BY-DEFAULT del token
	// exchange (RFC 8693, mig 0025). No las escribe ninguna API: solo el operador
	// de plataforma fuera de banda (ver la migración).
	MayImpersonate       bool
	MayImpersonateAdmins bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AllowsGrant indica si el client tiene habilitado el grant type dado.
func (c Client) AllowsGrant(grant string) bool { return contains(c.GrantTypes, grant) }

// AllowsScope indica si el scope solicitado está permitido para el client.
func (c Client) AllowsScope(scope string) bool { return contains(c.Scopes, scope) }

// ValidRedirectURI hace coincidencia EXACTA del redirect_uri (sin comodines,
// OIDC/OAuth2 BCP: never substring/prefix matching).
func (c Client) ValidRedirectURI(uri string) bool { return contains(c.RedirectURIs, uri) }

// IsPublic indica si el client es público (no autentica con secreto).
func (c Client) IsPublic() bool {
	return c.Type == ClientPublic || c.TokenEndpointAuthMethod == "none"
}

// UsableByOrg indica si el client puede emitir/canjear credenciales para una
// sesión de la organización dada. Un client GLOBAL de la suite (OrgID "") sirve a
// cualquiera —es la base de la sesión compartida entre apps—; uno acotado a una
// organización (las apps de terceros que registra un administrador, mig 0017) SOLO
// a la suya. Sin esta atadura, cargar la dirección de autorización con la sesión
// de OTRA organización emitía un código con el tenant de esa sesión, y con él un
// token con alcance sobre sus datos (NEG-05). Una sesión sin organización activa
// (orgID "") tampoco sirve a un client acotado: no hay organización que casar.
// EsNativa dice si es una app nativa de la suite: pública (sin secreto que la
// identifique) y global. Son `sytue-desktop` y `sytue-mobile`; a ellas Sytue ID les pide
// consentimiento la primera vez en cada dispositivo (NAT-02).
func (c Client) EsNativa() bool { return c.IsPublic() && c.OrgID == "" }

// ValidDeviceHash: el SHA-256 en base64url sin relleno del secreto de un dispositivo.
func ValidDeviceHash(h string) bool { return deviceHashRe.MatchString(h) }

var deviceHashRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (c Client) UsableByOrg(orgID string) bool {
	return c.OrgID == "" || c.OrgID == orgID
}

// AuthCode es un código de autorización (un solo uso, corta vida).
type AuthCode struct {
	CodeHash string
	ClientID string
	UserID   string
	OrgID    string
	// SessionID liga el código (y los refresh tokens que emita) a la sesión SSO
	// del navegador, para que la revocación de la sesión (logout) invalide en
	// cascada los refresh derivados.
	SessionID           string
	RedirectURI         string
	Scope               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	AuthTime            time.Time
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	// Dispositivo es el hash del secreto del dispositivo de una app nativa (NAT-02,
	// mig 0114). Con él, el código solo se canjea presentando ese secreto.
	Dispositivo string
}

// Session es una sesión de navegador del Authorization Server (SSO).
type Session struct {
	ID        string
	TokenHash string
	UserID    string
	OrgID     string
	AuthTime  time.Time
	AMR       []string
	UserAgent string
	// IP existe solo EN MEMORIA, el tiempo de atender la petición: se usa para
	// resolver la ubicación y no se persiste. Ver LugarCifrado.
	IP string
	// LugarCifrado es la ubicación aproximada cifrada con la clave derivada del
	// token de ESTA sesión (paquete sesscrypto). En la base de datos no hay nada
	// legible: ni la IP, ni el lugar, ni el token con el que se abriría.
	LugarCifrado []byte
	// Lugar es el texto ya descifrado. Solo se rellena en la sesión desde la que
	// se consulta, porque es la única cuyo token presenta el navegador.
	Lugar     string
	ExpiresAt time.Time
	RevokedAt *time.Time
	// Current marca, solo en listados (ListSessionsForUser), la sesión desde la
	// que se hace la petición. No se persiste.
	Current bool
}

// Valid indica si la sesión está vigente (no revocada ni expirada).
func (s Session) Valid(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// RefreshToken es un refresh token con rotación.
type RefreshToken struct {
	ID        string
	TokenHash string
	ClientID  string
	UserID    string
	OrgID     string
	SessionID string
	Scope     string
	AuthTime  time.Time
	RotatedTo string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// MFAContinuation liga el paso 1 (contraseña) y el paso 2 (TOTP/WebAuthn) del
// login interactivo. Es de un solo uso, corta vida y está ligada a IP/UA (F8).
type MFAContinuation struct {
	TokenHash string
	UserID    string
	OrgID     string
	ReturnTo  string
	IP        string
	UserAgent string
	ExpiresAt time.Time
}

// Valid indica si la continuación está vigente y sigue en el MISMO navegador.
//
// Antes exigía que coincidieran exactamente la IP y el User-Agent, y eso dejaba
// fuera a gente legítima en situaciones cotidianas: un móvil que salta de wifi a
// datos cambia de IP entre pedir el código y escribirlo; una red con IPv4 e IPv6
// puede resolver cada petición por una familia distinta; y el User-Agent cambia
// solo en cuanto el navegador se actualiza —o, en la app de escritorio, entre el
// arranque y el navegador del sistema que abre el flujo OIDC—. El resultado era
// un código correcto rechazado como si fuera incorrecto, sin forma de salir del
// bucle: exactamente el lockout que el segundo factor no debe provocar.
//
// Lo que se conserva es lo que de verdad protege este token: un solo uso, vida
// de minutos y cookie httpOnly + Secure + SameSite. Robarlo ya exige control del
// navegador de la víctima, y en ese escenario el atacante trae también su IP y
// su User-Agent, así que el binding estricto no lo paraba y sí paraba al dueño.
//
// Se sigue comparando el navegador, pero por su parte ESTABLE: un token llevado
// a otro navegador o a otro sistema operativo se rechaza igual, y una
// actualización de versión ya no.
func (m MFAContinuation) Valid(now time.Time, ua string) bool {
	return now.Before(m.ExpiresAt) && mismoNavegador(m.UserAgent, ua)
}

// mismoNavegador compara dos User-Agent ignorando los números de versión.
//
// Quitar los dígitos deja la huella que importa —familia del navegador y
// plataforma— y borra justo lo que cambia solo. Es deliberadamente tosco: no es
// identificación, es comprobar que el segundo paso ocurre donde empezó el
// primero.
func mismoNavegador(a, b string) bool {
	return huellaUA(a) == huellaUA(b)
}

func huellaUA(ua string) string {
	var sb strings.Builder
	sb.Grow(len(ua))
	for _, r := range ua {
		// Fuera los números y los separadores de versión. "Chrome/141.0.7390.55"
		// y "Chrome/140.0.7259.2" quedan igual; "Firefox/" sigue siendo distinto
		// de "Chrome/", y "Windows NT" de "Macintosh".
		if (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// WebAuthnCredential es una passkey registrada (WebAuthn L2).
type WebAuthnCredential struct {
	ID         string
	UserID     string
	CredID     []byte
	PublicKey  []byte // clave pública COSE serializada
	AAGUID     []byte
	SignCount  uint32
	Transports []string
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// NormalizeIP deja la dirección en la forma que acepta la columna `inet` de
// Postgres, o "" si no es una IP.
//
// Hacía falta porque una dirección IPv6 llega entre corchetes —RemoteAddr es
// "[::1]:54321" y hostOnly devuelve "[::1]"—, y esa forma con corchetes es
// sintaxis de URL, no de dirección: Postgres la rechaza con SQLSTATE 22P02. El
// efecto era un 500 en el login de cualquier cliente que llegara por IPv6, que
// en local es el caso NORMAL (localhost resuelve a ::1 antes que a 127.0.0.1).
//
// Devolver "" ante algo que no parsea es deliberado: la IP es un dato de
// contexto para auditoría y listados, y perderla vale mucho menos que impedir
// un login. Las columnas que la reciben admiten NULL.
func NormalizeIP(v string) string { return clientip.Normalize(v) }

// Ubicacion es la ubicación aproximada de una dirección IP, para que quien mire
// sus accesos recientes pueda reconocerlos: una IP no le dice a nadie si un
// acceso fue suyo, «Valencia, España» sí.
//
// Aproximada de verdad: la geolocalización por IP acierta el país casi siempre y
// la ciudad bastante menos —una VPN, un móvil o el proxy de una operadora la
// desplazan cientos de kilómetros—, así que la interfaz la presenta como pista y
// nunca como prueba de dónde estuvo alguien.
type Ubicacion struct {
	Pais   string
	Ciudad string
}

// String compone «Ciudad, País», o solo el país si no se conoce la ciudad.
func (u Ubicacion) String() string {
	switch {
	case u.Ciudad != "" && u.Pais != "":
		return u.Ciudad + ", " + u.Pais
	case u.Pais != "":
		return u.Pais
	}
	return u.Ciudad
}
