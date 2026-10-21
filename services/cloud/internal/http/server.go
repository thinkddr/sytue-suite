// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cloudhttp implementa la superficie HTTP del control plane de Sytue
// Cloud (la API 8090): organizaciones/tenants, proyectos, builds, deployments,
// dominios, env/secretos, servicios gestionados, uso/facturación y el endpoint
// público de webhooks git.
//
// Auth: middleware que acepta el JWT de Sytue ID (vía JWKS) O un token de API
// programático (Authorization: Bearer scl_…) para CI/CLI. Autorización de doble
// eje: los humanos por rol efectivo de membership (RequireRole), los tokens por
// scope (read<write<deploy). Aislamiento por tenant vía RLS (el tenant se inyecta
// en el contexto desde los claims o desde el token resuelto). El audience de los
// tokens del panel es sytue-cloud-api (CONVENTIONS §8).
package cloudhttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/runtime"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// Deps son las dependencias del router HTTP de Sytue Cloud.
type Deps struct {
	Cfg           config.Config
	Service       *service.Service
	Verifier      *authn.Verifier
	ReadyCheckers []httpx.Checker
	ExecSession   runtime.ExecSession
}

// requestTimeout es el plazo por petición de la API REST y del webhook git. Las
// rutas de STREAMING (SSE de logs, WS de exec) van EXENTAS: son conexiones de
// larga duración cuyo ciclo de vida gobierna el propio stream (disconnect del
// cliente y, en exec, un WithTimeout de 30min), no un plazo de request.
const requestTimeout = 30 * time.Second

// NewRouter construye el router completo de la API de Sytue Cloud.
func NewRouter(d Deps) http.Handler {
	// Rate-limit de defensa en profundidad (CONVENTIONS §10, frontera de confianza):
	//  1. Backstop global por IP (en memoria, por réplica): techo anti-DoS para
	//     TODA la superficie, incluidos 404 y el webhook sin auth. Generoso: solo
	//     frena inundaciones groseras desde una misma IP.
	//  2. Webhook público: cubo compartido entre réplicas (Postgres) con clave FIJA
	//     de endpoint. Acota el trabajo NO autenticado (leer cuerpo, resolver repo)
	//     con independencia del origen — los proveedores git entregan desde IPs
	//     compartidas, así que limitar por IP sería a la vez inútil (atacante multi-IP)
	//     y dañino (ahogaría a GitHub/GitLab). ponytail: techo global; si el volumen
	//     legítimo lo requiere, pre-filtro por allowlist de IPs de proveedor.
	//  3. API autenticada: cubo por IDENTIDAD (sub del usuario o id del token), en
	//     memoria (la API puede ser de alto volumen; un write PG por request sería
	//     un cuello). El despliegue real solo ocurre tras verificar firma/tenant.
	globalLimiter := httpx.NewTokenBucket(100, 200)
	pool := d.Service.Store().Pool()
	webhookLimiter := pg.NewRateLimiter(pool, pg.RateLimiterConfig{PerSecond: 50, Burst: 200})
	apiLimiter := httpx.NewTokenBucket(30, 60)

	// El plazo por petición NO es global: se aplica por grupo (ver más abajo), de
	// modo que las rutas de streaming queden EXENTAS. El backstop de rate-limit sí
	// es global.
	r := httpx.NewRouter(httpx.RouterOptions{
		RateLimit: &httpx.RateLimitConfig{Limiter: globalLimiter},
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{svc: d.Service, cfg: d.Cfg, execSession: d.ExecSession}

	// Contrato de la API: público y FUERA de /api/v1, como el discovery de OIDC.
	// Describe la superficie, no datos; pedir credenciales para leerlo solo
	// estorbaría a quien genera un SDK. Sirve la spec del binario en ejecución.
	r.Get("/openapi.yaml", h.ServeOpenAPI)

	// API REST bajo /api/v1 (audience sytue-cloud-api). authenticate acepta JWT o
	// token de API; apiLimiter va DESPUÉS (su clave lee la identidad del contexto).
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(h.authenticate(d.Verifier))
		// Materializa owner inicial/invitaciones aceptadas y niega a humanos
		// retirados antes de cualquier ruta del control-plane.
		api.Use(h.requireCloudMembership())
		// Confinamiento por-proyecto: un token acotado a P solo pasa bajo
		// /api/v1/projects/{P}/*; no afecta a humanos ni a tokens tenant-wide.
		api.Use(h.confineProjectToken())
		// Escribir exige al menos `member` a las personas (CLOUD-04): una ruta
		// nueva nace cerrada para el rol `billing` aunque nadie se acuerde de ella.
		api.Use(h.requireWriter())
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: apiRateKey}))

		// Logs en vivo + terminal (SSE / WS): conexiones de larga duración,
		// EXENTAS del plazo de 30s (que las mataría a mitad de stream). Su ciclo de
		// vida lo gobiernan el disconnect del cliente (r.Context) y el WithTimeout
		// del propio exec. Autorización: SOLO HUMANOS — requireRole rechaza a los
		// tokens de API. Los logs de prod contienen secretos/PII ⇒ member+ (excluye
		// billing y todo token). El exec abre /bin/sh en el pod de prod ⇒ admin/owner.
		api.With(h.requireRole("owner", "admin", "member")).
			Get("/projects/{id}/deployments/{deploymentId}/logs", h.StreamDeploymentLogs)
		api.With(h.requireRole("admin", "owner")).
			Get("/projects/{id}/deployments/{deploymentId}/exec", h.ExecDeployment)

		// Resto de la API REST bajo el plazo de 30s por petición (los streams van
		// exentos, arriba). middleware.Timeout cancela el contexto de la petición.
		api.Group(func(api chi.Router) {
			api.Use(middleware.Timeout(requestTimeout))

			// Organización (tenant) del usuario autenticado.
			api.Get("/org", h.GetOrg)
			api.With(h.requireRole("admin", "owner")).Patch("/org", h.UpdateOrg)

			// Plantillas de arranque (catálogo estático del producto: presets de
			// configuración por stack). Lectura para cualquier miembro autenticado.
			api.Get("/templates", h.ListTemplates)
			api.Get("/app-catalog", h.ListAppCatalog)

			// Catálogo de tamaños de instancia: lo consumen el formulario de
			// escala del dashboard y el CLI para no llevar su propia copia.
			api.Get("/instance-sizes", h.ListInstanceSizes)

			// Mira un repositorio público y deduce cómo desplegarlo, para que el
			// alta no pregunte lo que está escrito en el repo. Nunca falla el alta:
			// lo que no se puede mirar se responde con detected:false.
			api.Get("/inspect-repo", h.InspectRepo)

			// Alta creando el repositorio del usuario en Sytue Git. Son dos pasos
			// con un rodeo por el consentimiento de la forja en medio.
			//
			// requireScope en el primero: al volver, el alta encola un despliegue,
			// así que sin él un token de scope 'write' desplegaría (write→deploy).
			// El callback NO lo lleva porque a él llega el navegador con la sesión
			// del humano, nunca un token de API — y el `state` que trae lo emitió
			// este servidor y va atado a su sub.
			api.With(h.requireScope()).Post("/projects:scaffold", h.StartScaffold)
			api.Get("/forge/callback", h.ForgeCallback)

			// Nombre libre sugerido para un proyecto nuevo (el slug es el subdominio
			// público y es único en TODA la plataforma, así que se comprueba antes).
			api.Get("/suggest-slug", h.SuggestProjectSlug)
			// Disponibilidad EN VIVO del nombre que el usuario escribe, con
			// alternativas libres derivadas de él cuando ya está cogido. Lo consume
			// el formulario mientras se teclea, así que es GET y sin efectos.
			api.Get("/check-slug", h.CheckSlug)

			// Proyectos.
			api.Get("/projects", h.ListProjects)
			api.Post("/projects", h.CreateProject)
			// Alta desde plantilla: proyecto + servicios + variables + primer
			// despliegue en UNA transacción. requireScope no es adorno: encola un
			// despliegue, así que sin él un token de scope 'write' desplegaría.
			api.With(h.requireScope()).Post("/projects:from-template", h.CreateProjectFromTemplate)
			api.Get("/projects/{id}", h.GetProject)
			// Recomendación de tamaño según el uso REAL. No hay endpoint de
			// «aplicar»: aplicarla es PATCH /projects/{id}, que ya valida, audita y
			// respeta la cuota. El «1 clic» es la interfaz, no un verbo nuevo.
			api.Get("/projects/{id}/sizing", h.GetProjectSizing)
			// Consumo observado (CPU/memoria) del proyecto: lectura, cualquier
			// miembro. Es el dato del que sale la recomendación de tamaño, así que
			// negárselo a quien puede verla no protegería nada.
			api.Get("/projects/{id}/metrics", h.GetProjectMetrics)
			api.Patch("/projects/{id}", h.UpdateProject)
			// Detección de la ruta de salud: sondea la app del proyecto con las
			// rutas de convención. Hace peticiones salientes, así que va POST (no
			// es cacheable ni idempotente en efectos) y con el mismo rol que
			// configurar el proyecto.
			api.Post("/projects/{id}/probe-health", h.ProbeHealthPaths)
			api.With(h.requireRole("admin", "owner")).Delete("/projects/{id}", h.DeleteProject)
			// Clonar: CREA un proyecto (cuenta para la cuota y puede costar dinero
			// en cuanto se despliegue), así que el mismo rol que darlo de alta.
			api.With(h.requireRole("admin", "owner")).Post("/projects/{id}/clone", h.CloneProject)
			// Rotar el secreto del webhook: gestión de una credencial, así que
			// admin/owner y solo humanos (requireRole rechaza tokens de API: una
			// credencial de máquina no debe poder invalidar otra).
			api.With(h.requireRole("admin", "owner")).
				Post("/projects/{id}/webhook-secret:rotate", h.RotateWebhookSecret)
			// Cerrar/abrir las previews del proyecto. Gestión de una credencial y
			// decisión de exposición pública: admin/owner y solo humanos.
			api.With(h.requireRole("admin", "owner")).
				Post("/projects/{id}/preview-protection", h.ProtectPreviews)
			api.With(h.requireRole("admin", "owner")).
				Delete("/projects/{id}/preview-protection", h.UnprotectPreviews)

			// GitHub App: conectar/desconectar la instalación del repo (admin/owner).
			// Habilita clone de repo privado sin deploy-key + comentario de preview en PR.
			api.With(h.requireRole("admin", "owner")).Post("/projects/{id}/github", h.ConnectGitHubApp)
			api.With(h.requireRole("admin", "owner")).Delete("/projects/{id}/github", h.DisconnectGitHubApp)
			// Callback OAuth de instalación de la App (Setup URL): liga la instalación
			// SOLO tras probar propiedad. El `state` lo emite github:install, de un
			// solo uso y atado a quien lo pidió (CLOUD-18).
			api.With(h.requireRole("admin", "owner")).Post("/projects/{id}/github:install", h.StartGitHubInstall)
			api.With(h.requireRole("admin", "owner")).Get("/github/callback", h.GitHubAppCallback)

			// Despliegues (build + deploy) y rollback: disparar/rollback/cancelar exige
			// scope deploy a los tokens (no-op para humanos, que autorizan por rol).
			api.With(h.requireScope()).Post("/projects/{id}/deployments", h.CreateDeployment)
			api.Get("/projects/{id}/deployments", h.ListDeployments)
			api.Get("/deployments/{id}", h.GetDeployment)
			// Volver atrás y promocionar cambian lo que sirve producción: admin/owner
			// o token deploy (decisión del dueño, 2026-10-17). Desplegar comprueba el
			// destino dentro del handler, porque una preview sí la lanza un member.
			api.With(h.requireProductionDeployer()).Post("/deployments/{id}/rollback", h.Rollback)
			// Publicar en producción la imagen de una preview. Cambia lo que ve el
			// público, igual que desplegar: mismo scope.
			api.With(h.requireProductionDeployer()).Post("/deployments/{id}/promote", h.PromotePreview)
			// Reiniciar la producción: no cambia versión ni compila nada, pero
			// interrumpe procesos en marcha, así que lleva la misma verja que
			// desplegar (scope deploy para tokens, rol para humanos).
			api.With(h.requireScope()).Post("/projects/{id}/restart", h.RestartApp)
			api.With(h.requireScope()).Delete("/deployments/{id}", h.CancelDeployment)

			// Builds.
			api.Get("/projects/{id}/builds", h.ListBuilds)
			api.Get("/builds/{id}", h.GetBuild)
			// Los logs de BUILD contienen lo mismo que los de despliegue —secretos y PII
			// del proceso de construcción— así que llevan la misma verja que su hermano
			// de la línea 92: solo humanos con member+ (excluye al rol billing y a TODO
			// token de API). Era una asimetría, no una decisión: esta ruta se quedó sin
			// requireRole mientras el stream de logs de despliegue sí lo documentaba.
			api.With(h.requireRole("owner", "admin", "member")).Get("/builds/{id}/logs", h.GetBuildLogs)
			// Búsqueda en los logs de RUNTIME ya guardados (WS4/O1). No es un
			// stream —va bajo el plazo de 30 s como el resto de la API REST—, pero
			// lleva la MISMA verja que su hermano en vivo: enseña el mismo
			// contenido (secretos y PII de producción), solo que en diferido.
			api.With(h.requireRole("owner", "admin", "member")).Get("/projects/{id}/logs", h.SearchLogs)
			api.With(h.requireScope()).Delete("/builds/{id}", h.CancelBuild)

			// Dominios + TLS.
			api.Get("/projects/{id}/domains", h.ListDomains)
			api.Post("/projects/{id}/domains", h.AddDomain)
			api.Patch("/domains/{id}", h.PatchDomain)
			api.Delete("/domains/{id}", h.RemoveDomain)

			// Monitorización de uptime (sondas HTTP por proyecto).
			api.Get("/projects/{id}/uptime-checks", h.ListUptimeChecks)
			api.Post("/projects/{id}/uptime-checks", h.CreateUptimeCheck)
			api.Get("/uptime-checks/{id}", h.GetUptimeCheck)
			api.Patch("/uptime-checks/{id}", h.UpdateUptimeCheck)
			api.Delete("/uptime-checks/{id}", h.DeleteUptimeCheck)

			// Alertas por evento (policies → webhook saliente). Gestión de
			// notificaciones de la organización: humanos admin/owner.
			api.With(h.requireRole("admin", "owner")).Get("/alerts", h.ListAlertPolicies)
			api.With(h.requireRole("admin", "owner")).Post("/alerts", h.CreateAlertPolicy)
			api.With(h.requireRole("admin", "owner")).Patch("/alerts/{id}", h.UpdateAlertPolicy)
			api.With(h.requireRole("admin", "owner")).Delete("/alerts/{id}", h.DeleteAlertPolicy)

			// Storage del proyecto (Object Storage gestionado).
			// Listar los objetos es leer datos del cliente, como el DB Studio:
			// member+ y solo personas, no el rol billing.
			api.With(h.requireRole("owner", "admin", "member")).Get("/projects/{id}/storage/objects", h.ListStorageObjects)
			api.Post("/projects/{id}/storage/presign", h.PresignStorage)
			// Borrar es destructivo y no tiene deshacer (el bucket no versiona):
			// admin/owner, como el resto de bajas. Listar y prefirmar siguen
			// abiertos a cualquier miembro.
			api.With(h.requireRole("admin", "owner")).
				Delete("/projects/{id}/storage/objects", h.DeleteStorageObject)

			// Variables de entorno / secretos.
			api.Get("/projects/{id}/env", h.ListEnv)
			// Descargar el .env REVELA los secretos (el listado los oculta): mismo
			// listón que la consola SQL o la contraseña de la base — admin/owner,
			// solo humanos— y queda en la auditoría.
			api.With(h.requireRole("admin", "owner")).
				Get("/projects/{id}/env/export", h.ExportEnv)
			api.Put("/projects/{id}/env", h.SetEnv)
			api.Delete("/env/{id}", h.DeleteEnv)

			// Servicios gestionados (Sytue Postgres, Storage, Functions, Cron, Auth).
			api.Get("/projects/{id}/services", h.ListServices)
			api.Post("/projects/{id}/services", h.ProvisionService)
			api.Get("/services/{id}", h.GetService)
			// Editar un servicio cambia lo que corre en el clúster (y lo que se
			// factura), así que va con el mismo rol que darlo de baja.
			api.With(h.requireRole("admin", "owner")).Patch("/services/{id}", h.UpdateService)
			api.With(h.requireRole("admin", "owner")).Delete("/services/{id}", h.DeleteService)

			// Sytue DB Studio (explorador + consola SQL del Postgres del proyecto).
			// Autorización: SOLO HUMANOS con rol member+ (requireRole rechaza tokens
			// de API). Es el mínimo razonable: aquí salen DATOS DE NEGOCIO del
			// cliente —no metadatos de despliegue—, así que ni el rol billing (que
			// solo debe ver facturas) ni una credencial de máquina de CI tienen nada
			// que hacer leyendo sus tablas. La consola SQL sube a admin/owner: ejecuta
			// sentencias arbitrarias (incluidas escrituras y DDL) sobre esa BD.
			api.With(h.requireRole("owner", "admin", "member")).
				Get("/projects/{id}/db/tables", h.ListDbTables)
			api.With(h.requireRole("owner", "admin", "member")).
				Get("/projects/{id}/db/tables/{table}/rows", h.GetDbTableRows)
			api.With(h.requireRole("owner", "admin", "member")).
				Get("/projects/{id}/db/api", h.GetDbAPIInfo)
			api.With(h.requireRole("owner", "admin")).
				Post("/projects/{id}/db/query", h.RunDbQuery)
			// La cadena de conexión CON contraseña: son datos del cliente y puede
			// verlos, pero es la llave de su base entera. Mismo listón que la
			// consola SQL —admin/owner y solo humanos: un token de CI no se lleva
			// la contraseña— y queda escrito en la auditoría.
			api.With(h.requireRole("owner", "admin")).
				Get("/projects/{id}/db/credentials", h.GetDbCredentials)

			// Backups de Sytue Postgres: listar es lectura (member+, como GET /services/{id});
			// restaurar es DESTRUCTIVO (admin/owner + scope deploy, como el resto de acciones
			// destructivas/de despliegue). El objeto S3 se compone en servidor a partir del
			// prefijo propio del servicio; el cliente solo pasa el nombre de fichero.
			api.Get("/services/{id}/backups", h.ListServiceBackups)
			// Descargar una copia: es la base de datos ENTERA saliendo de la
			// plataforma, así que el mismo rol que restaurarla (admin/owner) y solo
			// humanos —una credencial de máquina no se lleva el dump—.
			api.With(h.requireRole("admin", "owner")).Get("/services/{id}/backups/download", h.DownloadServiceBackup)
			api.With(h.requireRole("admin", "owner"), h.requireScope()).Post("/services/{id}/restore", h.RestoreServiceBackup)
			// Copia a demanda: escribe (consume almacenamiento y CPU de la base),
			// pero no destruye nada. Mismo rol que el resto de acciones que
			// cuestan dinero; el scope 'write' basta, no hace falta 'deploy'.
			api.With(h.requireRole("owner", "admin", "member")).Post("/services/{id}/backup", h.BackupServiceNow)
			// Ejecutar una tarea a mano: corre el mismo código que ya corría sola,
			// así que el permiso es el de quien puede configurarla, no uno mayor.
			api.With(h.requireRole("owner", "admin", "member")).Post("/services/{id}/run", h.RunServiceNow)
			// Registro de un servicio gestionado (función, cron, job, app del
			// catálogo). Mismo criterio que los logs de la app: member+ y solo
			// humanos, porque ahí sale lo que imprima el código del cliente.
			api.With(h.requireRole("owner", "admin", "member")).Get("/services/{id}/logs", h.ServiceLogs)

			// Uso / facturación.
			// Cuánto queda del plan. Lectura para cualquier miembro: el tope ya se
			// nota al chocar con él (402), así que esconderlo no protege nada.
			api.Get("/quota", h.GetQuota)
			api.Get("/usage", h.GetUsage)
			api.Get("/usage/detail", h.GetUsageDetail)
			api.With(h.requireRole("admin", "owner")).Get("/audit", h.ListAudit)

			// Facturación (motor de factura/plan/límite). Precios y cuenta (plan +
			// gasto del mes, en paridad con /usage) son lectura para cualquier miembro;
			// las FACTURAS (documentos financieros con importes), el tope de gasto y la
			// generación de factura (CronJob mensual) exigen admin/owner.
			api.Get("/billing/account", h.GetBillingAccount)
			api.Get("/billing/pricing", h.GetPricing)
			api.With(h.requireRole("admin", "owner")).Get("/billing/invoices", h.ListInvoices)
			api.With(h.requireRole("admin", "owner")).Get("/billing/invoices/{id}", h.GetInvoice)
			api.With(h.requireRole("admin", "owner")).Put("/billing/spend-limit", h.SetSpendLimit)
			api.With(h.requireRole("admin", "owner")).Post("/billing/invoices:generate", h.GenerateInvoice)

			// Tokens de API (credenciales de CI/CLI). La gestión de credenciales es
			// exclusiva de usuarios admin/owner: requireRole rechaza a los tokens (evita
			// que una credencial de máquina emita/revoque otras — escalada de privilegio).
			api.With(h.requireRole("admin", "owner")).Get("/tokens", h.ListTokens)
			api.With(h.requireRole("admin", "owner")).Post("/tokens", h.CreateToken)
			api.With(h.requireRole("admin", "owner")).Delete("/tokens/{id}", h.RevokeToken)

			// Colaboradores (RBAC). Gestión de identidad: humanos admin/owner.
			api.With(h.requireRole("admin", "owner")).Get("/members", h.ListMembers)
			api.With(h.requireRole("admin", "owner")).Post("/members", h.AddMember)
			api.With(h.requireRole("admin", "owner")).Delete("/members/{sub}", h.RemoveMember)
			api.With(h.requireRole("admin", "owner")).Get("/invitations", h.ListMemberInvitations)
			api.With(h.requireRole("admin", "owner")).Post("/invitations", h.InviteMember)
			api.With(h.requireRole("admin", "owner")).Delete("/invitations/{id}", h.RevokeMemberInvitation)
		})
	})

	// Webhooks git (sin auth de Sytue ID; se valida la firma HMAC POR PROYECTO).
	// Limitador de endpoint dedicado (más estricto que el backstop global) + el
	// plazo de 30s por petición (antes heredado del timeout global, ahora por-ruta).
	r.With(middleware.Timeout(requestTimeout), httpx.RateLimit(httpx.RateLimitConfig{Limiter: webhookLimiter, KeyFunc: webhookRateKey})).
		Post("/webhooks/git", h.GitWebhook)

	// Disparador de despliegue por URL (Deploy Hook). Va con el webhook de git y
	// no bajo /api/v1 por la misma razón: la credencial es el token de la ruta, no
	// una sesión. Comparte su limitador —es el otro endpoint sin sesión— para que
	// probar URLs a lo bruto cueste lo mismo que inundar el webhook.
	r.With(middleware.Timeout(requestTimeout), httpx.RateLimit(httpx.RateLimitConfig{Limiter: webhookLimiter, KeyFunc: webhookRateKey})).
		Post("/hooks/deploy/{token}", h.DeployHook)

	return r
}

// apiRateKey deriva la clave de rate-limit de la API de la identidad autenticada
// (sub del usuario o "apitoken:<id>", ambos fijados por authenticate). Cae a la IP
// remota solo si no hubiera identidad (no debería, tras authenticate).
func apiRateKey(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok && c.Subject != "" {
		return "api:" + c.Subject
	}
	return "api-ip:" + r.RemoteAddr
}

// webhookRateKey es una clave fija: el cubo del webhook es global de endpoint
// (ver NewRouter), no por origen.
func webhookRateKey(*http.Request) string { return "webhook:git" }
