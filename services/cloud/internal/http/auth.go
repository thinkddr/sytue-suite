// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// tokenCtxKey transporta el token de API resuelto (si la petición se autenticó
// con un Bearer programático en vez de un JWT de sesión). Su presencia distingue
// a un llamante-máquina (autoriza por scope) de un humano (autoriza por rol).
type tokenCtxKey struct{}

// apiCaller son los datos del token de API resuelto en el contexto.
type apiCaller struct {
	Scope domain.TokenScope
	// ProjectID != nil CONFINA al token a /api/v1/projects/{ProjectID}/* (ver
	// confineProjectToken). nil = tenant-wide: sin restricción de ruta (como hoy).
	ProjectID *uuid.UUID
}

func tokenFromCtx(ctx context.Context) *apiCaller {
	if ctx != nil {
		if c, ok := ctx.Value(tokenCtxKey{}).(*apiCaller); ok {
			return c
		}
	}
	return nil
}

// bearerValue extrae el token de la cabecera Authorization: Bearer <token>. Los
// tokens de API (CI/CLI) siempre llegan por cabecera; no se acepta ?access_token=.
func bearerValue(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// authenticate acepta un token de API programático (Authorization: Bearer scl_…)
// ADEMÁS del JWT de sesión de Sytue ID. Si el Bearer tiene el prefijo de token,
// se resuelve a tenant+scope y se fija app.tenant vía contexto; en otro caso se
// delega al verificador OIDC (comportamiento actual). Debe ir el primero en la
// cadena de /api/v1.
func (h *Handlers) authenticate(v *authn.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		jwtPath := v.RequireAuth(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerValue(r)
			if raw != "" && strings.HasPrefix(raw, service.TokenPrefix) {
				h.authenticateToken(w, r, next, raw)
				return
			}
			jwtPath.ServeHTTP(w, r)
		})
	}
}

// requireCloudMembership materializa el owner inicial y las invitaciones ya
// aceptadas, y después exige una membresía Cloud explícita a todo humano.
func (h *Handlers) requireCloudMembership() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokenFromCtx(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			if h.admitCloudHuman(w, r) {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// admitCloudHuman concentra la decisión del límite de acceso humano. Es bool
// para que el middleware tenga un único punto de continuación y ningún camino
// de error pueda ejecutar accidentalmente el handler protegido.
func (h *Handlers) admitCloudHuman(w http.ResponseWriter, r *http.Request) bool {
	claims, ok := authn.FromContext(r.Context())
	if !ok || claims.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return false
	}
	if _, ok := claims.TenantOrEmpty(); !ok {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("An active organisation is required."))
		return false
	}
	role, fromMembership, err := h.svc.EffectiveRole(r.Context(), claims.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return false
	}
	if fromMembership && role != "" {
		return true
	}
	if fromMembership {
		accepted, acceptErr := h.svc.AcceptPendingMember(r.Context(), claims.Subject, claims.Email)
		if acceptErr != nil {
			httpx.Fail(w, r, toAPIError(acceptErr))
			return false
		}
		if !accepted {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You do not have access to Sytue Cloud in this organisation."))
		}
		return accepted
	}

	// Tenant vacío: materializa al primer usuario autorizado por Sytue ID.
	bootstrapRole := roleFromClaims(claims)
	if bootstrapRole != domain.RoleOwner {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The first sign-in to Sytue Cloud must be done by the owner of the organisation."))
		return false
	}
	if _, err := h.svc.AddMember(r.Context(), claims.Subject, bootstrapRole); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return false
	}
	return true
}

func roleFromClaims(c authn.Claims) domain.Role {
	for _, role := range []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleBilling} {
		if c.HasRole(string(role)) {
			return role
		}
	}
	return ""
}

// authenticateToken resuelve el token y, si es válido, inyecta tenant+identidad y
// continúa. Aplica el suelo read/write por método: un token de solo lectura no
// puede usar métodos no seguros (las rutas de despliegue exigen además scope
// deploy vía requireScope). Cualquier fallo de resolución es 401 sin distinguir
// (no filtra qué tokens existen).
func (h *Handlers) authenticateToken(w http.ResponseWriter, r *http.Request, next http.Handler, raw string) {
	rt, err := h.svc.ResolveAPIToken(r.Context(), raw)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid or expired API token."))
		return
	}
	t, err := tenancy.New(rt.Tenant)
	if err != nil || rt.Scope == domain.ScopeHook {
		// Un token hook solo vale en /hooks/deploy/{token} (CLOUD-17).
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid or expired API token."))
		return
	}
	// Suelo global: un token read no muta nada (métodos no seguros → 403).
	if !isSafeMethod(r.Method) && !rt.Scope.AtLeast(domain.ScopeWrite) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This token has no write permission."))
		return
	}
	ctx := r.Context()
	ctx = authn.Into(ctx, authn.Claims{Subject: "apitoken:" + rt.ID.String(), Tenant: rt.Tenant})
	ctx = tenancy.Into(ctx, t)
	ctx = context.WithValue(ctx, tokenCtxKey{}, &apiCaller{Scope: rt.Scope, ProjectID: rt.ProjectID})
	next.ServeHTTP(w, r.WithContext(ctx))
}

// apiProjectsPrefix es el prefijo de las rutas por-proyecto de la API REST. En un
// middleware de chi r.URL.Path es SIEMPRE la ruta completa de la petición (chi no
// la reescribe), así que basta con inspeccionarla.
const apiProjectsPrefix = "/api/v1/projects/"

// confineProjectToken hace cumplir el confinamiento de un token de API acotado a un
// proyecto (apiCaller.ProjectID != nil). MODELO: un token por-proyecto es un
// "deploy key" de ESE proyecto — puede desplegar y leer sus deployments/env/domains/
// services/builds a través de las rutas bajo /api/v1/projects/{P}/*, y NADA más. Se
// DENIEGA (403) cualquier otra ruta: rutas de recurso desnudo (/deployments/{id},
// /builds/{id}, /services/{id}, /domains/{id}, /env/{id}) y rutas tenant-wide (/org,
// /members, /billing, /tokens, /alerts, /usage, /templates, y la lista /projects). El
// poll de estado se hace vía la LISTA del proyecto (GET /projects/{P}/deployments), no
// la ruta /deployments/{id} desnuda: limitación aceptable y documentada.
//
// Enforcement basado en RUTA (no en lookup por-recurso): simple, sin acceso a BD, y
// enforceable en el middleware. Se usa r.URL.Path en vez de chi.URLParam("id") porque
// en un middleware montado con Use() a nivel de grupo chi TODAVÍA no ha emparejado la
// ruta hoja (URLParam("id") está vacío y RoutePattern es "/api/v1/*"); r.URL.Path sí
// está poblada. El primer segmento tras /api/v1/projects/ debe ser el UUID del token.
//
// No-op para humanos (JWT) y para tokens tenant-wide (ProjectID == nil): pasan como hoy.
// Debe ir DESPUÉS de authenticate (que puebla el apiCaller) en el grupo /api/v1.
func (h *Handlers) confineProjectToken() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromCtx(r.Context())
			if tok == nil || tok.ProjectID == nil {
				next.ServeHTTP(w, r) // humano o token tenant-wide: sin cambio
				return
			}
			if !pathUnderProject(r.URL.Path, *tok.ProjectID) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This API token is confined to its own project."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// pathUnderProject indica si path está bajo /api/v1/projects/{pid}/... (o es el
// propio /api/v1/projects/{pid}). Compara el primer segmento como UUID (tolera
// mayúsculas/formato, igual que el parseo del handler); un segmento no-UUID o de
// otro proyecto => false (deny por defecto: no-escape).
func pathUnderProject(path string, pid uuid.UUID) bool {
	if !strings.HasPrefix(path, apiProjectsPrefix) {
		return false
	}
	seg := path[len(apiProjectsPrefix):]
	if i := strings.IndexByte(seg, '/'); i >= 0 {
		seg = seg[:i]
	}
	got, err := uuid.Parse(seg)
	return err == nil && got == pid
}

// requireScope exige, para llamantes-token, el scope de deploy (el más alto en la
// escalera read<write<deploy). Es no-op para humanos (autorizados por rol). Se usa
// en las rutas de despliegue. Tras authenticate.
func (h *Handlers) requireScope() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tok := tokenFromCtx(r.Context()); tok != nil && !tok.Scope.AtLeast(domain.ScopeDeploy) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This token has no permission to deploy."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireRole exige que un HUMANO tenga uno de los roles efectivos. Los tokens de
// API se rechazan (la gestión de identidad —miembros, tokens, ajustes de la org,
// borrados— es exclusiva de usuarios; evita escalada vía credencial de máquina).
// Tras authenticate.
func (h *Handlers) requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokenFromCtx(r.Context()) != nil {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This action requires a user, not an API token."))
				return
			}
			if !h.humanHasRole(r, roles...) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Insufficient role for this action."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// humanHasRole indica si el HUMANO autenticado tiene alguno de los roles: rol
// EFECTIVO desde memberships y, si no hay membership explícito, fallback al claim
// del JWT (para no romper al owner inicial). Falso para tokens. Un error de BD
// deniega (fail-closed). Reutilizado por requireRole y por la revelación del
// secreto de webhook en GetProject.
func (h *Handlers) humanHasRole(r *http.Request, roles ...string) bool {
	if tokenFromCtx(r.Context()) != nil {
		return false
	}
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return false
	}
	role, fromMembership, err := h.svc.EffectiveRole(r.Context(), claims.Subject)
	if err != nil {
		return false
	}
	if fromMembership {
		return roleIn(string(role), roles)
	}
	return claims.HasAnyRole(roles...)
}

func roleIn(role string, allowed []string) bool {
	for _, a := range allowed {
		if role == a {
			return true
		}
	}
	return false
}

// canRedefineExecution decide quién puede cambiar los campos de un proyecto que
// determinan QUÉ CÓDIGO SE EJECUTA (repositorio, rama, deploy key, directorios y
// los comandos de instalación, build y release) y A DÓNDE VA EL TRÁFICO PÚBLICO
// (las redirecciones del borde).
//
// Por qué existe: esos campos son un despliegue diferido. El comando de release
// acaba en `/bin/sh -c` dentro de un Job que monta el Secret ENTERO del proyecto
// (ver controller/release.go), y una redirección manda el dominio de producción a
// donde diga quien la escriba. Cambiarlos no ejecuta nada en el acto —lo hace el
// siguiente despliegue, que dispara un `git push` cualquiera—, y por eso la
// verja del despliegue no los cubría.
//
// El criterio es el mismo que ya se aplicaba en `PATCH /services/{id}` («cambia
// lo que corre en el clúster, así que va con el mismo rol que darlo de baja»),
// solo que aquí el cambio es más directo todavía:
//
//   - Humanos: admin/owner. Un `member` configura su proyecto (tamaño, puerto,
//     salud, pausa) pero no reescribe lo que se ejecuta; el rol `billing` no
//     tiene nada que hacer aquí —es el mismo motivo por el que se le niega el DB
//     Studio—.
//   - Tokens: scope `deploy`. No es una concesión: un token que puede desplegar
//     ya ejecuta el código del repositorio, así que negárselo aquí sería teatro.
//     Lo que se cierra es el token `write`, al que `requireScope` prohíbe
//     desplegar y que sin esto podía dejar escrito el comando que correrá el
//     despliegue de otro.
//
// Fail-closed: sin identidad reconocible, no.
func (h *Handlers) canRedefineExecution(r *http.Request) bool {
	if tok := tokenFromCtx(r.Context()); tok != nil {
		return tok.Scope.AtLeast(domain.ScopeDeploy)
	}
	return h.humanHasRole(r, string(domain.RoleAdmin), string(domain.RoleOwner))
}

// requireWriter cierra por defecto la escritura a quien no es al menos `member`.
//
// Hasta ahora la autorización era una lista de rutas marcadas a mano y lo que no
// se marcaba quedaba abierto a cualquier rol: el rol `billing`, que solo debe ver
// facturación, podía crear servicios, desplegar o cambiar variables (CLOUD-04).
// Invertir el valor por defecto deja que una ruta nueva nazca cerrada.
//
// Solo mira a las personas y solo los métodos que escriben. Los tokens de API ya
// tienen su suelo por scope en authenticateToken y requireScope.
func (h *Handlers) requireWriter() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isSafeMethod(r.Method) && tokenFromCtx(r.Context()) == nil &&
				!h.humanHasRole(r, string(domain.RoleOwner), string(domain.RoleAdmin), string(domain.RoleMember)) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Insufficient role for this action."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// canDeployProduction decide quién cambia lo que corre en producción: desplegar,
// volver atrás, promocionar una preview, crear un servicio que ejecuta un comando
// o tocar las variables con las que arranca.
//
// Decisión del dueño (2026-10-17): solo admin y owner. Un `member` despliega
// previews y entornos que no son producción. Los tokens, con scope `deploy`
// (un token que despliega ya ejecuta el código del repositorio).
func (h *Handlers) canDeployProduction(r *http.Request) bool {
	if tok := tokenFromCtx(r.Context()); tok != nil {
		return tok.Scope.AtLeast(domain.ScopeDeploy)
	}
	return h.humanHasRole(r, string(domain.RoleAdmin), string(domain.RoleOwner))
}

// requireProductionDeployer es canDeployProduction como middleware, para las
// rutas que siempre tocan producción (rollback, promoción).
func (h *Handlers) requireProductionDeployer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.canDeployProduction(r) {
				failProductionDeploy(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func failProductionDeploy(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an admin or owner can change what runs in production."))
}
