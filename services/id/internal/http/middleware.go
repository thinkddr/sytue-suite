// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwt"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/scim"
)

type ctxKeyClaims struct{}

// tokenClaims son los claims extraídos de un access token validado localmente.
type tokenClaims struct {
	Subject string
	Tenant  string
	Roles   []string
	// Perms son los permisos GRANULARES efectivos del sujeto en la org activa (claim
	// `perms`, mig 0019). ADITIVO al modelo owner/admin: owner/admin no lo necesitan
	// (superusuarios); un rol delegado autoriza por lo que aquí figure.
	Perms  []string
	Scopes []string
	// AMR son los métodos de autenticación de la sesión que emitió el token
	// (`pwd`, `mfa`, `otp`, `webauthn`). La administración de plataforma lo exige.
	AMR []string
	// AuthTime es cuándo se autenticó la PERSONA (claim `auth_time`), que no es
	// cuándo se emitió el token: con refresh, un access token recién hecho puede
	// respaldarse en una sesión de hace dos semanas. Es lo que mide la
	// reautenticación fuerte (ver requireStepUp). Cero si el token no lo trae.
	AuthTime time.Time
	// ClientID es el client OAuth2 al que se emitió el token: la entrada de `aud`
	// que no es el propio resource server (ver oidc.accessAudience). Dice QUIÉN
	// pidió la credencial, no solo quién la usa, y de eso depende que se pueda
	// administrar la identidad con ella (ver soloClientsDeLaSuite).
	ClientID string
}

func claimsFrom(ctx context.Context) (tokenClaims, bool) {
	c, ok := ctx.Value(ctxKeyClaims{}).(tokenClaims)
	return c, ok
}

// cspStyleHashes autoriza SOLO los <style> propios del IdP (login/cuenta) por
// hash sha256, calculado en runtime de las mismas constantes que se sirven:
// siempre en sync, sin 'unsafe-inline'. Si se añade otra hoja inline, añadirla aquí.
var cspStyleHashes = func() string {
	blocks := []string{
		loginCSS, accountCSS, sessionsCSS, securityCSS,
		accountShellCSS, profileCSS, passkeysCSS,
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		sum := sha256.Sum256([]byte(b))
		parts = append(parts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join(parts, " ")
}()

// cspScriptHashes autoriza SOLO los <script> propios del IdP por hash sha256
// (mismo patrón que los estilos): sin 'unsafe-inline'. Son cinco: el login por
// passkey, la normalización del campo de identidad —que va en los TRES
// formularios con correo—, el barrido de cookies del cierre de sesión, el alta
// de passkeys desde «Mi cuenta» y el captcha ALTCHA del alta.
var cspScriptHashes = func() string {
	blocks := []string{themeInitJS, loginScript, emailFieldScript, logoutSweepScript, passkeysScript, altchaScript}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		sum := sha256.Sum256([]byte(b))
		parts = append(parts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join(parts, " ")
}()

// securityHeaders aplica cabeceras de seguridad por defecto (docs/CONVENTIONS.md
// §0.7): CSP estricta (estilos propios solo por hash), HSTS (en https), nosniff,
// frame deny, referrer.
func securityHeaders(cfg Config) func(http.Handler) http.Handler {
	// form-action: Chromium lo aplica a TODA la cadena de redirects del envío del
	// formulario, y el POST de login acaba (via /oauth2/authorize) en el callback
	// del cliente en OTRO subdominio de la suite -> con solo 'self' el navegador
	// aborta la navegación (login "no hace nada"). Los redirect_uri reales ya los
	// valida el servidor; esto es solo la frontera del navegador.
	// script-src: solo los scripts propios, por hash. connect-src 'self': el del
	// login por passkey llama por fetch a /login/webauthn/{begin,finish} del mismo
	// origen. img-src: el cierre de sesión pide a cada superficie de la suite que
	// borre su cookie cargando su /auth/logout como imagen; sin esta línea el
	// navegador bloquea el barrido y solo se cierra la sesión del IdP.
	//
	// Las dos van por LISTA y no por `https://*.sytue.eu`. El comodín de host de CSP
	// casa con cualquier número de etiquetas, así que aquel valor incluía
	// `*.apps.sytue.eu`, `*.preview.sytue.eu` y el `<slug>.sytue.eu` que el alta de
	// organización publica para cada cliente: hosts donde corre código de CLIENTES,
	// autorizados como destino de formulario y de subrecurso en la página que teclea
	// contraseñas. La lista sale de platform/pkg/config, que ya es donde vive esta
	// frontera para CORS y para los WebSocket; la del IdP añade la forja y la consola
	// de plataforma porque el barrido de cierre les pide su /logout (logout.go).
	//
	// Y depende del ENTORNO: la lista era una sola constante con prod y dev
	// juntos, así que producción mandaba de verdad `form-action` con 17 orígenes
	// `*.sytue.localhost` dentro — en la página que teclea contraseñas.
	origenesSytue := strings.ReplaceAll(pcfg.SytueOperatedWebOrigins(cfg.Env), ",", " ")
	csp := "default-src 'none'; style-src " + cspStyleHashes +
		"; script-src " + cspScriptHashes +
		"; img-src " + origenesSytue +
		"; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; " +
		"form-action 'self' " + origenesSytue
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", csp)
			h.Set("Cache-Control", "no-store")
			if cfg.CookieSecure {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearer extrae el token Bearer de la cabecera Authorization.
func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):]), true
	}
	return "", false
}

// requireSCIMAuth protege los endpoints SCIM con un bearer estático (token del
// proveedor de provisioning). Comparación en tiempo constante.
func (s *Server) requireSCIMAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token creado POR UNA ORGANIZACIÓN desde su consola (mig 0055). Se mira
		// primero porque es la vía viva: el token único del despliegue existe solo
		// para no romper la instalación que ya lo tuviera configurado.
		//
		// Aquí se resuelve a QUÉ organización pertenece y se deja en el contexto;
		// el handler SCIM acota TODO a esa organización, así que un token de la
		// empresa A no puede ver ni tocar nada de la B. Sin este paso, SCIM solo
		// existía para la única organización nombrada en una variable de entorno.
		// El `s.store != nil` es por los tests que construyen el Server a mano para
		// probar esta verja sin base de datos: sin él, la consulta se queda colgada
		// contra un pool nil y el paquete entero muere por timeout.
		if tok, ok := bearer(r); ok && tok != "" && s.store != nil {
			if org, err := s.store.OrgBySCIMToken(r.Context(), domain.HashToken(tok)); err == nil {
				if org.Status != domain.StatusActive {
					httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("That organisation is not active."))
					return
				}
				next.ServeHTTP(w, r.WithContext(scim.WithOrg(r.Context(), org)))
				return
			}
		}
		// El token SCIM es UNO para todo el despliegue, así que sin organización
		// ligada no hay tenant al que acotar: /Users listaría el directorio entero
		// y {id} no tendría contra qué comprobar la pertenencia. Falta el slug ⇒
		// SCIM no está configurado, exactamente igual que si faltara el token.
		if s.cfg.SCIMBearerToken == "" || s.cfg.SCIMOrgSlug == "" {
			// En inglés como el resto del catálogo de httpx: quien lee esto es un
			// IdP externo (Entra, Okta), no una persona con idioma.
			httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("SCIM is not configured."))
			return
		}
		tok, ok := bearer(r)
		if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.SCIMBearerToken)) != 1 {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid SCIM token."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAccessToken valida un access token JWT emitido por este IdP usando el
// JWKS público local (autoconsumo). Inyecta los claims en el contexto.
func (s *Server) requireAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearer(r)
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Missing Bearer token."))
			return
		}
		set, err := s.keys.PublicJWKS()
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		tok, err := oidc.Verify(raw, s.cfg.Issuer, s.cfg.ResourceAudience, set)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid or expired token.").Wrap(err))
			return
		}
		// Un token de una cuenta que ya no está activa no sirve, aunque no haya
		// caducado: sin esto, en los 15 minutos de vida que le quedan tras una baja
		// se podían crear contraseñas de aplicación, subir una foto o activar un
		// TOTP en una cuenta borrada (ADR 0054). Solo se niega a una cuenta que
		// EXISTE y no está activa: un token de client_credentials no es de nadie.
		if fuera, uerr := s.cuentaNoActiva(r.Context(), tok.Subject()); uerr != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(uerr))
			return
		} else if fuera {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid or expired token."))
			return
		}
		c := tokenClaims{Subject: tok.Subject(), ClientID: clientDeAudiencia(tok.Audience(), s.cfg.ResourceAudience)}
		if v, ok := tok.Get("tenant"); ok {
			if sv, ok := v.(string); ok {
				c.Tenant = sv
			}
		}
		// El derecho de organización se comprueba al crear/canjear/renovar la
		// credencial. Aquí no se bloquea toda la API de identidad: onboarding,
		// facturación y reactivación administrativa deben seguir accesibles cuando
		// una suscripción u organización está suspendida. La ventana máxima de una
		// retirada es el TTL corto del access token (15 min por defecto).
		c.Roles = claimStrings(tok, "roles")
		// amr: lo exige la API de plataforma (siempre con segundo factor).
		c.AMR = claimStrings(tok, "amr")
		// auth_time: CUÁNDO se autenticó la persona, no cuándo se emitió el
		// token. Los dos se separan en cuanto hay refresh —un access token de
		// hace un minuto puede venir de una sesión de hace dos semanas—, así que
		// sin este claim no hay forma de exigir «acabas de identificarte».
		c.AuthTime = claimTime(tok, "auth_time")
		c.Perms = claimStrings(tok, "perms")
		if v, ok := tok.Get("scope"); ok {
			if sv, ok := v.(string); ok {
				c.Scopes = strings.Fields(sv)
			}
		}
		ctx := context.WithValue(r.Context(), ctxKeyClaims{}, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// clientDeAudiencia devuelve el client al que se emitió el token: la entrada de
// `aud` distinta del resource server. Vacío si no hay ninguna (un token que solo
// nombra al recurso), y ese caso lo trata soloClientsDeLaSuite como «no
// identificado» — que es un NO.
func clientDeAudiencia(aud []string, recurso string) string {
	for _, a := range aud {
		if a != "" && a != recurso {
			return a
		}
	}
	return ""
}

// rutaDeAdministracion indica si la petición va a la administración de identidad
// (de una organización o de la plataforma). Se mira la ruta y no el patrón de chi
// porque el middleware corre ANTES de resolver la ruta; ante la duda, incluir de
// más solo hace el gate más estricto.
func rutaDeAdministracion(path string) bool {
	return strings.Contains(path, "/admin/") || strings.Contains(path, "/platform/") ||
		path == "/api/v1/orgs" || strings.HasPrefix(path, "/api/v1/orgs/")
}

// rutaSoloDeLaSuite indica si la petición solo se atiende con una credencial pedida
// por un client de la suite. Además de la administración (rutaDeAdministracion):
//
//   - `/api/v1/onboarding…`: alta inicial, dominio e invitaciones. A diferencia
//     de `/orgs…`, es accesible antes de tener organización o segundo factor.
//   - `/api/v1/me/…` y `/api/v1/webauthn/…`: CÓMO SE ENTRA en la cuenta y qué
//     sesiones tiene —contraseña, segundo factor, passkeys, contraseñas de
//     aplicación, correo de recuperación, sesiones, dispositivos de confianza,
//     organización activa, elevación PIM— y la exportación de sus datos.
//   - `/api/v1/me` en todo lo que no sea leerlo.
//
// Lo que SÍ sigue abierto a una app de terceros es lo que pidió: leer el perfil de
// quien consintió (`GET /api/v1/me`), y las consultas de directorio, personas y
// etiquetas, que los productos reenvían con el token de quien llama.
func rutaSoloDeLaSuite(method, path string) bool {
	if rutaDeAdministracion(path) {
		return true
	}
	resto, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return false
	}
	if resto == "me" {
		return method != http.MethodGet
	}
	for _, p := range []string{"me", "webauthn", "onboarding"} {
		if resto == p || strings.HasPrefix(resto, p+"/") {
			return true
		}
	}
	return false
}

// soloClientsDeLaSuite cierra la administración de identidad a los clients
// GLOBALES de la suite (org_id NULL: los que siembra el despliegue).
//
// EL AGUJERO QUE TAPA. Un access token de este IdP vale para esta API venga del
// client que venga —`aud` lleva siempre el resource server—, y los claims `roles`
// y `perms` viajan en TODOS los tokens. Así que una app de terceros registrada
// por un administrador de la organización (RFC 7591 o desde la consola), con
// scopes tan inofensivos como `openid profile`, recibía en cuanto un owner
// iniciaba sesión en ella una credencial que abría la administración COMPLETA de
// esa organización: directorio, reseteo de contraseñas y de MFA, roles, log de
// auditoría, dominios de correo. El usuario consintió entrar en una app, no
// entregarle su organización.
//
// POR QUÉ POR CLIENT Y NO POR SCOPE. Exigir un scope `admin` es lo que dice el
// manual, pero hoy NINGUNA superficie lo pide (deploy/helm/…/values.yaml:
// `oidcScopes: "openid profile email offline_access"`), así que hacerlo obligatorio
// dejaría la consola entera en 403 al desplegar. El client sí distingue: los de la
// suite son globales y los de terceros nacen SIEMPRE acotados a una organización
// (dcr.go y admin_oidc_clients.go). El scope como segunda llave puede venir
// después, con la consola pidiéndolo primero.
//
// Y NO SOLO LA ADMINISTRACIÓN. El mismo token abría también la gestión de las
// credenciales de la PROPIA persona: con `openid profile`, la app podía dar de alta
// una passkey o una contraseña de aplicación en la cuenta de quien acababa de
// entrar en ella, que es una forma de entrar después sin ella. Y `/api/v1/orgs…`
// es administración con otra ruta. El alcance entero está en rutaSoloDeLaSuite.
//
// Fail-closed: sin client identificable, o si no se puede leer, no se administra.
func (s *Server) soloClientsDeLaSuite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rutaSoloDeLaSuite(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		c, ok := claimsFrom(r.Context())
		if !ok || c.ClientID == "" {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage(
				"Esta credencial no puede administrar la identidad.",
			))
			return
		}
		// Una lectura por clave primaria sobre una tabla diminuta, y solo en las
		// rutas de administración: el tráfico de verdad (/me y compañía) no la
		// paga. ponytail: sin caché; si algún día la administración fuera un
		// camino caliente, este client cabe en la caché efímera compartida.
		client, err := s.store.GetClient(r.Context(), c.ClientID)
		if err != nil || client.OrgID != "" {
			_ = s.store.Audit(r.Context(), nil,
				auditEntry(c.Subject, "", "auth.admin.client_denied", c.ClientID, r))
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage(
				"Esta credencial no puede administrar la identidad.",
			))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireRole exige al menos uno de los roles tras requireAccessToken.
func (s *Server) requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := claimsFrom(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			for _, want := range roles {
				for _, have := range c.Roles {
					if want == have {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Rol insuficiente."))
		})
	}
}

// requireOrgKind exige que la organización ACTIVA del sujeto (claim `tenant`)
// sea de una de las clases dadas. Es la verja de las cuentas personales en
// Sytue Admin: una persona sola es `owner` de su organización personal —lo es
// por construcción— y por rol entraría en todo; lo que no tiene es nada que
// administrar en «usuarios», «grupos», «roles», «dominios» o «invitaciones».
// Sí entra en «aplicaciones»: integrar otras apps y migrar sus datos es suyo.
func (s *Server) requireOrgKind(kinds ...domain.OrgKind) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := claimsFrom(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			// La organización activa: la del token y, si el token aún no la lleva
			// —el asistente de alta invita justo después de crearla—, la del alta.
			// Si no hay ninguna, se deja pasar: el handler es quien sabe decir «crea
			// tu organización primero» (400), y una verja que dijera 403 mentiría.
			org, err := s.store.GetOrgBySlug(r.Context(), c.Tenant)
			if err != nil || c.Tenant == "" {
				if org, err = s.onboardingOrg(r.Context(), c.Subject); err != nil {
					next.ServeHTTP(w, r)
					return
				}
			}
			kind := org.Kind
			if kind == "" {
				kind = domain.OrgKindOrg
			}
			for _, k := range kinds {
				if k == kind {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Not available for personal accounts."))
		})
	}
}

// requirePermission exige que el sujeto pueda ejercer AL MENOS UNO de los permisos
// dados (semántica any-of, p. ej. una lectura acepta users.read O users.manage). Es el
// enforcement de la admin delegada (mig 0019), aditivo a requireRole:
//
//   - owner/admin PASAN SIEMPRE (superusuarios built-in; domain.HasPermission lo
//     resuelve) — retrocompat: lo que hoy autoriza requireRole("owner","admin") lo
//     siguen autorizando estos gates.
//   - el resto pasa solo si su claim `perms` contiene alguno de los pedidos
//     (deny-by-default).
func (s *Server) requirePermission(perms ...domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := claimsFrom(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			for _, want := range perms {
				if domain.HasPermission(c.Roles, c.Perms, want) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Permiso insuficiente."))
		})
	}
}

// claimTime lee un claim numérico de segundos desde epoch (NumericDate de JWT).
// La librería lo entrega como float64 al venir de JSON y como time.Time cuando
// lo puso el emisor; los dos casos son el mismo dato. Cero si no está o no se
// entiende — y quien lo consume trata el cero como «no consta», nunca como
// «hace mucho tiempo» ni como «ahora».
func claimTime(tok jwt.Token, key string) time.Time {
	v, ok := tok.Get(key)
	if !ok {
		return time.Time{}
	}
	switch t := v.(type) {
	case time.Time:
		return t
	case float64:
		return time.Unix(int64(t), 0)
	case int64:
		return time.Unix(t, 0)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return time.Unix(n, 0)
		}
	}
	return time.Time{}
}

func claimStrings(tok jwt.Token, key string) []string {
	v, ok := tok.Get(key)
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Fields(t)
	default:
		return nil
	}
}

// cuentaNoActiva dice si el `sub` de un token es de una cuenta que existe y ya no
// está activa. Un `sub` que no es un uuid, o que no es de nadie, es de un client
// (client_credentials) y pasa. Cualquier otro fallo al leer es un error: sin
// poder mirar el estado no se da el token por bueno.
func (s *Server) cuentaNoActiva(ctx context.Context, sub string) (bool, error) {
	if uuid.Validate(sub) != nil {
		return false, nil //nolint:nilerr // un sub que no es uuid es de un client: no es un fallo.
	}
	u, err := s.store.GetUserByID(ctx, sub)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return false, nil //nolint:nilerr // un uuid que no es de ninguna cuenta es de un client: no hay estado que mirar.
	case err != nil:
		return false, err
	}
	return !u.Active(), nil
}
