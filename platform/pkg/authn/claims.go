// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package authn valida los JWT emitidos por Sytue ID (el IdP nuclear) en cada
// servicio Go de la suite. Descarga el JWKS de Sytue ID (con caché y rotación),
// verifica firma/issuer/audiencia/expiración y extrae los claims sub/roles/
// tenant, exponiéndolos por contexto y mediante middlewares RequireAuth /
// RequireRole.
//
// Clean-room: implementado sobre OIDC/OAuth2 (RFC 7519/8414) y la librería
// permisiva lestrrat-go/jwx (MIT). No incorpora código AGPL.
package authn

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// GoogleAPIsAudience es la audiencia que Sytue ID añade a un access token con scopes
// de APIs de Google (cuenta de Google emulada). La aceptan SOLO las rutas
// /compat/google de los productos, con el scope de cada operación (RequireGoogleAPIs).
const GoogleAPIsAudience = "sytue-googleapis"

// Claims son los claims de Sytue ID relevantes para autorización en la suite.
type Claims struct {
	// Subject es el identificador estable del usuario (claim "sub").
	Subject string
	// Email del usuario (claim "email"), si está presente. Es su login GLOBAL en la
	// suite y puede estar en cualquier dominio, incluido uno que no servimos.
	Email string
	// Mailbox es la dirección de correo del usuario en la organización activa
	// (claim "mailbox"), la que Sytue ID le asignó al darlo de alta. Vacía si su
	// organización todavía no reparte buzones. Para el correo manda esta y no
	// Email: ver MailAddress.
	Mailbox string
	// Mailboxes son sus direcciones principales en TODAS sus organizaciones activas
	// (claim "mailboxes"), que Sytue ID deriva en cada emisión del token (ADR 0053).
	Mailboxes []string
	// Delegado: el token lleva `act` (RFC 8693), lo usa Actor en nombre del sujeto.
	// Lo que es de la PERSONA en todas sus organizaciones (sus avisos, ADR 0053) no se
	// abre a un token delegado.
	Delegado bool
	Actor    string
	// Emitido es el `iat`: cuándo lo firmó Sytue ID. Sirve para no fiarse de lo que un
	// token de ANTES de un cambio dice que es suyo (un buzón retirado, ADR 0053).
	Emitido time.Time
	// Expira es el `exp`. Una conexión que dura más que su token (un WebSocket) se
	// cierra al llegar: al reconectar, el token nuevo dice lo que vale AHORA.
	Expira time.Time
	// MailDomains son los dominios de correo VERIFICADOS de la organización activa
	// (claim "mail_domains"). Una organización puede tener varios y de una
	// dirección solo se deduce el suyo, así que sin esta lista no hay forma de
	// saber si una dirección está en OTRO dominio de la misma organización o en uno
	// ajeno. Vacío cuando la organización no tiene dominio propio: entonces su
	// correo vive en el subdominio de la suite, que se deduce del tenant.
	MailDomains []string
	// Name es el nombre legible (claim "name"), si está presente.
	Name string
	// Roles son los roles del usuario en el tenant activo (owner|admin|member|billing).
	Roles []string
	// Groups son los NOMBRES de los grupos del usuario en el tenant activo (claim
	// "groups"), tal como estaban al emitir el token: un cambio de pertenencia se ve al
	// renovarlo. Vacío si no tiene grupos (Sytue ID no emite el claim vacío).
	Groups []string
	// Tenant es el slug de la organización activa (claim "tenant" / "org").
	Tenant string
	// OrgKind es la clase de la organización activa (claim "org_kind"): "org"
	// para una organización de verdad y "personal" para una cuenta personal —una
	// persona cuya organización es su correo—. Vacío en tokens anteriores a esa
	// distinción, que se leen como "org": lo que había antes eran organizaciones.
	OrgKind string
	// Plan es el contrato comercial de la organización activa (claim "plan").
	// Vacío cuando el emisor no lo publica.
	Plan string
	// StorageBytes es la cuota de almacenamiento (Drive) que el plan concede a la
	// organización activa (claim "storage_bytes"). 0 = el servicio aplica su
	// propio valor por defecto. Solo la publica Sytue ID para cuentas personales.
	StorageBytes int64
	// MailboxBytes es la cuota del buzón que concede el plan (claim
	// "mailbox_bytes"). Misma regla que StorageBytes.
	MailboxBytes int64
	// Scopes son los scopes OAuth2 concedidos (claim "scope").
	Scopes []string
	// Issuer y Audience del token validado.
	Issuer   string
	Audience []string
}

// LogValue implementa slog.LogValuer para que, al registrar Claims, NUNCA se
// filtre PII (email/name). Solo se exponen identificadores no personales aptos
// para auditoría/correlación: sub, tenant y roles. (RGPD art. 25: logs sin PII.)
func (c Claims) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("sub", c.Subject),
		slog.String("tenant", c.Tenant),
		slog.Any("roles", c.Roles),
	)
}

// MailAddress devuelve la dirección con la que aprovisionar y resolver el buzón
// del usuario: la que le asignó su organización y, si aún no tiene, el correo de
// registro. El orden importa y por eso está aquí y no repetido en cada llamante:
// aprovisionar por el correo de registro crea buzones en dominios que nuestro MX
// no sirve —quien entra invitado con su correo personal— y esos buzones no reciben
// nada. La reserva a Email mantiene funcionando los despliegues y las cuentas de
// servicio anteriores a que existieran los buzones de organización.
func (c Claims) MailAddress() string {
	if c.Mailbox != "" {
		return c.Mailbox
	}
	return c.Email
}

// HasRole indica si el usuario posee el rol dado (comparación exacta).
func (c Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole indica si el usuario posee al menos uno de los roles dados.
func (c Claims) HasAnyRole(roles ...string) bool {
	for _, want := range roles {
		if c.HasRole(want) {
			return true
		}
	}
	return false
}

// HasScope indica si el token incluye el scope dado.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// TenantOrEmpty devuelve un tenancy.Tenant a partir del claim, validado.
// OrgKindPersonal es el valor de OrgKind de una cuenta personal.
const OrgKindPersonal = "personal"

// Personal indica si la organización activa es una cuenta personal. Un token sin
// el claim NO lo es: la ausencia significa «anterior a la distinción», y lo que
// había antes eran organizaciones.
func (c Claims) Personal() bool { return c.OrgKind == OrgKindPersonal }

func (c Claims) TenantOrEmpty() (tenancy.Tenant, bool) {
	if c.Tenant == "" || !tenancy.ValidSlug(c.Tenant) {
		return tenancy.Tenant{}, false
	}
	return tenancy.Tenant{Slug: c.Tenant}, true
}

type ctxKey struct{}

// Into inserta los claims en el contexto.
func Into(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext extrae los claims del contexto.
func FromContext(ctx context.Context) (Claims, bool) {
	if ctx != nil {
		if c, ok := ctx.Value(ctxKey{}).(Claims); ok {
			return c, true
		}
	}
	return Claims{}, false
}

// RateKey deriva la clave de rate-limit de la IDENTIDAD del token (sujeto y
// organización), no de la IP. La IP no sirve detrás del ingress: todas las
// peticiones llegan con la misma y un único cubo compartido castigaría a todos
// los clientes por el exceso de uno solo, mientras que un atacante con varias
// salidas lo esquivaría igual.
//
// Va DESPUÉS del middleware de autenticación (si no, no hay claims). Cuando no
// las hay —ruta pública o token inválido— cae a la dirección de QUIEN CONECTA.
//
// Y no a `X-Forwarded-For`, que es lo que hacía antes. Esa cabecera la escribe
// quien quiera: cada salto le AÑADE por la derecha la dirección de quien le habló,
// así que la primera entrada —la que se leía aquí— es exactamente la que controla
// el atacante. Un cubo por IP con esa clave no limita nada: basta con mandar una
// cabecera distinta en cada intento para estrenar cubo. Leerla bien exige saber en
// qué proxies se cree, y esta función no tiene esa lista (Sytue ID sí, en
// `domain.TrustedProxies`, y por eso sus cubos de /login y /oauth2 no usan esto).
//
// Detrás del ingress, `RemoteAddr` es la IP de Traefik y todo el tráfico sin token
// comparte un cubo. Es restrictivo, y en un limitador ese es el lado seguro del
// error; además hoy no lo pisa nadie: los seis servicios que usan `RateKey` montan
// `RequireAuth` ANTES, así que esta rama no se ejecuta. Lo que había aquí no era
// un agujero abierto sino una mina: la heredaba la primera ruta sin autenticar que
// se pusiera detrás de este limitador. Una ruta así necesita una clave por IP DE
// VERDAD —con lista de proxies de confianza—, no esta.
//
// Se usa como httpx.RateLimitConfig.KeyFunc; vive aquí y no en httpx para que
// httpx no tenga que conocer los claims.
func RateKey(r *http.Request) string {
	if c, ok := FromContext(r.Context()); ok && c.Subject != "" {
		return "sub:" + c.Tenant + ":" + c.Subject
	}
	return "peer:" + r.RemoteAddr
}

// splitSpace divide por espacios ignorando vacíos (formato de "scope").
func splitSpace(s string) []string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil
	}
	return parts
}

// toStringSlice convierte claims que pueden venir como []any, []string o string.
// toInt64 lee un claim numérico. jwx entrega los números como float64 al
// deserializar JSON, y como int/int64 cuando el token se construyó en proceso
// (tests); un claim ausente o de otro tipo es 0, que es «sin valor».
func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	default:
		return 0
	}
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return t
	case string:
		return splitSpace(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// HastaQueCaduque acota ctx al `exp` del token: una conexión larga (WebSocket, SSE)
// se corta cuando su token deja de valer, y el cliente reconecta con uno nuevo, que
// dice lo que la persona puede ver AHORA. Sin esto, quien salía de la organización o
// cerraba sesión seguía recibiendo mientras no cerrara el socket (COM-08).
func HastaQueCaduque(ctx context.Context, c Claims) (context.Context, context.CancelFunc) {
	if c.Expira.IsZero() {
		return context.WithCancel(ctx)
	}
	return context.WithDeadline(ctx, c.Expira)
}
