// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"cmp"
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Deps son las dependencias del router HTTP del gateway.
type Deps struct {
	Cfg      config.Config
	Verifier *authn.Verifier
	Store    *store.Store
	Registry *connector.Registry
	Limiter  httpx.RateLimiter
	Audit    policy.AuditSink
	Engine   *policy.Engine
	IDApps   manifest.IDApps
	// Cipher sella el `state` del consentimiento OAuth (misma KEK que los secretos
	// de instancia). Nil ⇒ el flujo de consentimiento responde 503 en vez de
	// arrancar con un estado que no se puede autenticar.
	Cipher *keycrypto.Cipher
	// Directory lee usuarios de la organización en Sytue ID: el asistente automático
	// de migración lo necesita para emparejar cada cuenta del origen con su cuenta de
	// Sytue. Es el MISMO cliente que usa el aprovisionamiento saliente.
	Directory     *directory.Client
	ReadyCheckers []httpx.Checker
	// Fondo es el contexto del proceso (se cancela al apagar). Nil ⇒ Background.
	Fondo context.Context
}

// NewRouter construye el router completo del Sytue Integration Gateway.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{
		// Las subidas a las nubes personales llevan el fichero en el cuerpo; los JSON
		// siguen topados en 1 MiB por DecodeJSON.
		MaxBodyBytes: personalStorageMaxBytes + (1 << 20),
		RequestTimeoutFor: func(r *http.Request) time.Duration {
			if r.Method == http.MethodPost && r.URL.Path == "/internal/v1/rooms/whatsapp/activate" {
				return 60 * time.Second
			}
			return 0
		},
		CORS: &httpx.CORSConfig{
			AllowedOrigins:   d.Cfg.CORSAllowedOrigins,
			AllowCredentials: true,
		},
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{
		store:                      d.Store,
		registry:                   d.Registry,
		limiter:                    d.Limiter,
		audit:                      d.Audit,
		engine:                     d.Engine,
		idApps:                     d.IDApps,
		egressTimeout:              d.Cfg.EgressTimeout,
		egressMaxRedirect:          d.Cfg.EgressMaxRedirects,
		idBaseURL:                  d.Cfg.IDBaseURL,
		driveBaseURL:               d.Cfg.DriveBaseURL,
		contactsBaseURL:            d.Cfg.ContactsBaseURL,
		mailBaseURL:                d.Cfg.MailBaseURL,
		calendarBaseURL:            d.Cfg.CalendarBaseURL,
		fondo:                      cmp.Or(d.Fondo, context.Background()),
		migrationTimeout:           d.Cfg.MigrationEgressTimeout,
		dir:                        d.Directory,
		oauthClients:               newOAuthClientResolver(d.Store, d.Cfg.OAuthClients),
		oauthCipher:                d.Cipher,
		publicURL:                  d.Cfg.PublicURL,
		consoleURL:                 d.Cfg.ConsoleURL,
		drivePublicURL:             d.Cfg.DrivePublicURL,
		roomsPublicURL:             d.Cfg.RoomsPublicURL,
		mailPublicURL:              d.Cfg.MailPublicURL,
		calendarPublicURL:          d.Cfg.CalendarPublicURL,
		contactsPublicURL:          d.Cfg.ContactsPublicURL,
		notesPublicURL:             d.Cfg.NotesPublicURL,
		whatsAppConfigID:           d.Cfg.WhatsAppConfigID,
		whatsAppGraphVersion:       d.Cfg.WhatsAppGraphVersion,
		whatsAppWebhookVerifyToken: d.Cfg.WhatsAppWebhookVerifyToken,
	}

	// Superficie de OPERADOR DE PLATAFORMA (fuera de /api/v1 y del JWT de la suite):
	// instalar el client delegable de una org con el que la migración escribe en la
	// cuenta de cada persona. Sin credencial de operador configurada NO se monta: no
	// existe endpoint que sondear. Ver handlers_operator.go.
	if d.Cfg.OperatorToken != "" {
		r.Group(func(op chi.Router) {
			// Ni el operador ni los servicios llegan por el Ingress (la consola
			// y Drive usan el Service): desde internet, 404 antes de mirar el token
			// (NEG-23).
			op.Use(httpx.SinPasarPorElIngress, requireOperator(d.Cfg.OperatorToken))
			op.Put("/operator/delegated-client", h.PutDelegatedClient)
			// Credenciales con las que Sytue se presenta ante cada proveedor. Aquí y
			// no en /api/v1 porque son de la PLATAFORMA: ningún miembro de un tenant
			// debe poder leerlas ni saber cuáles hay. Ver handlers_platform_oauth.go.
			// El catálogo, también aquí: la superficie de plataforma necesita saber
			// ante qué proveedores se puede registrar una app, y no tiene —ni debe
			// tener— un tenant con el que entrar por /api/v1.
			op.Get("/operator/templates", h.OperatorListTemplates)
			op.Put("/operator/templates/{template}/enabled", h.OperatorSetTemplateEnabled)
			op.Get("/operator/oauth-clients", h.ListPlatformOAuthClients)
			op.Put("/operator/oauth-clients/{template}", h.PutPlatformOAuthClient)
			op.Delete("/operator/oauth-clients/{template}", h.DeletePlatformOAuthClient)
		})
	}

	// Servicio a servicio: Drive recibe el token de almacenamiento; Rooms recibe
	// destinos, entrega mensajes o crea reuniones sin recibir tokens ni webhooks.
	if d.Cfg.InternalSecret != "" {
		r.Group(func(in chi.Router) {
			in.Use(httpx.SinPasarPorElIngress, requireInternal(d.Cfg.InternalSecret))
			in.Get("/internal/v1/storage/instances", h.InternalStorageInstances)
			in.Get("/internal/v1/storage/token", h.InternalStorageToken)
			in.Get("/internal/v1/rooms/destinations", h.InternalRoomsDestinations)
			in.Post("/internal/v1/rooms/messages", h.InternalRoomsMessage)
			in.Post("/internal/v1/rooms/meetings", h.InternalRoomsMeeting)
			in.Get("/internal/v1/rooms/whatsapp/account", h.InternalWhatsAppAccount)
			in.Post("/internal/v1/rooms/whatsapp/activate", h.InternalWhatsAppActivate)
		})
	}

	// Vuelta del consentimiento OAuth. Va FUERA del grupo autenticado a propósito: aquí
	// aterriza el navegador por un redirect del proveedor, sin Authorization posible.
	// No canjea nada: reenvía al BFF de Admin o Drive, que termina por /api/v1/oauth/finish con la
	// sesión de quien vuelve. Ver OAuthCallback.
	r.Get("/api/v1/oauth/callback", h.OAuthCallback)
	r.Get("/consent/whatsapp", h.WhatsAppConsent)
	r.Get("/api/v1/whatsapp/webhook", h.VerifyWhatsAppPlatformWebhook)
	r.Post("/api/v1/whatsapp/webhook", h.ReceiveWhatsAppPlatformWebhook)

	r.Route("/api/v1", func(api chi.Router) {
		// Toda la API exige JWT de Sytue ID con una organización activa (RLS).
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)

		// Lecturas: cualquier miembro de la org.
		api.Get("/templates", h.ListTemplates)
		api.Get("/my-apps", h.MyApps) // tiles de apps conectadas para el launcher del portal.
		// Libertad de elección por producto: quién sirve cada producto de la suite.
		// `resolved` lo pide CADA carga de shell de cada usuario (mínimo y barato);
		// el otro es la pantalla de administración, con las alternativas.
		api.Get("/substitutions", h.ListSubstitutions)
		api.Get("/substitutions/resolved", h.ResolvedSubstitutions)
		api.Get("/instances", h.ListInstances)
		api.Post("/oauth/finish", h.OAuthFinish)
		api.Group(func(mail chi.Router) {
			mail.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(2, 10), KeyFunc: authn.RateKey}))
			mail.Get("/external-mail/accounts", h.ExternalMailAccounts)
			mail.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.1, 4), KeyFunc: authn.RateKey})).Post("/external-mail/connect", h.ExternalMailConnect)
			mail.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.1, 4), KeyFunc: authn.RateKey})).Post("/external-mail/accounts", h.ExternalMailConfigure)
			mail.Get("/external-mail/{instance}/folders", h.ExternalMailFolders)
			mail.Get("/external-mail/{instance}/messages", h.ExternalMailMessages)
			mail.Get("/external-mail/{instance}/message", h.ExternalMailMessage)
			mail.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.2, 5), KeyFunc: authn.RateKey})).Post("/external-mail/{instance}/send", h.ExternalMailSend)
			mail.Post("/external-mail/{instance}/actions", h.ExternalMailAction)
			mail.Delete("/external-mail/{instance}", h.ExternalMailDisconnect)
			h.linkedServicesRoutes(mail)
		})
		api.Get("/external-conversations/accounts", h.ExternalChatAccounts)
		api.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.2, 5), KeyFunc: authn.RateKey})).Post("/external-conversations/meeting", h.ExternalChatMeeting)
		api.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.2, 5), KeyFunc: authn.RateKey})).Patch("/external-conversations/meeting", h.ExternalChatMeeting)
		api.Get("/personal-storage/accounts", h.PersonalStorage)
		api.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.1, 4), KeyFunc: authn.RateKey})).Post("/personal-storage/connect", h.PersonalStorageConnect)
		api.Delete("/personal-storage/{instance}", h.PersonalStorageDisconnect)
		api.Get("/personal-storage/{instance}/items", h.PersonalStorageItems)
		api.Get("/personal-storage/{instance}/content", h.PersonalStorageDownload)
		api.Post("/personal-storage/{instance}/content", h.PersonalStorageUpload)
		api.With(httpx.RateLimit(httpx.RateLimitConfig{Limiter: httpx.NewTokenBucket(0.1, 4), KeyFunc: authn.RateKey})).Post("/external-conversations/connect", h.ExternalChatConnect)
		api.Get("/external-conversations/{instance}/channels", h.ExternalChatChannels)
		api.Get("/external-conversations/{instance}/messages", h.ExternalChatMessages)
		api.Post("/external-conversations/{instance}/messages", h.ExternalChatSend)
		api.Post("/external-conversations/{instance}/actions", h.ExternalChatAction)
		api.Delete("/external-conversations/{instance}", h.ExternalChatDisconnect)
		// Búsqueda federada sobre los SaaS conectados de la org: fan-out concurrente a
		// las instancias activas con SearchCapability (Graph/GWS…), degradación por-conector.
		// La búsqueda lleva lo que escribe la persona: por GET acaba en el log de
		// accesos de cada salto, en el `Referer` y en el historial. Por QUERY va en
		// el cuerpo. GET se queda como respaldo para los clientes ya desplegados.
		httpx.GetYQuery(api, "/search", h.Search)
		api.Get("/instances/{id}", h.GetInstance)
		api.Get("/instances/{id}/sync-runs", h.ListSyncRuns)
		// Plano de datos L3: consulta tabular (lectura). POST por llevar la spec en
		// el cuerpo (dataset/modelo + filtros/paginación), pero es una lectura.
		// Consultar sale con la credencial de la ORGANIZACIÓN: lo hacen los
		// administradores, quien creó la conexión y quien esté en su lista (NEG-06,
		// handlers_lectores.go). La búsqueda de arriba y el embed, igual.
		api.Post("/instances/{id}/data/query", h.QueryData)
		// Plano de presentación L4: mint de un artefacto de embed acotado (token de
		// vida corta) para incrustar un informe en la suite. Gobernamos QUIÉN mintea
		// (la lista de la conexión), no el contenido embebido.
		api.Post("/instances/{id}/embed/token", h.EmbedToken)
		// La lista de quién consulta. Fuera del grupo de administración a propósito:
		// la gestiona también quien creó la conexión; la verja está en el handler.
		api.Get("/instances/{id}/readers", h.GetReaders)
		api.Put("/instances/{id}/readers", h.PutReaders)

		// Traerse UNO SUS PROPIOS datos. Fuera del grupo de administración a propósito:
		// quien llega a Sytue y abre Mail con el buzón vacío no puede depender de que su
		// administrador le migre el correo. Lo que hace segura esta puerta es que no
		// acepta a quién migrar —destino y origen son siempre quien llama—, así que no
		// hay nada que manipular para leer los datos de otra persona. La conexión con el
		// origen la sigue autorizando un admin: sin instancia activa no se migra nada.
		api.Post("/migrations/self", h.CreateSelfMigration)
		api.Get("/migrations/self", h.ListSelfMigrations)

		// Escrituras: admin/owner de la org.
		api.Group(func(wr chi.Router) {
			wr.Use(d.Verifier.RequireRole("owner", "admin"))
			wr.Post("/instances", h.CreateInstance)
			wr.Get("/whatsapp/signup", h.WhatsAppSignupInfo)
			wr.Post("/whatsapp/signup/start", h.WhatsAppSignupStart)
			wr.Post("/whatsapp/signup/finish", h.WhatsAppSignupFinish)
			wr.Get("/whatsapp/numbers", h.WhatsAppNumbers)
			wr.Patch("/instances/{id}", h.UpdateInstance)
			wr.Delete("/instances/{id}", h.DeleteInstance)
			wr.Put("/instances/{id}/secrets", h.PutSecrets)
			// PATCH = escritura PARCIAL: añade una credencial sin borrar las otras.
			wr.Patch("/instances/{id}/secrets", h.PatchSecrets)
			wr.Post("/instances/{id}/test", h.TestInstance)
			wr.Post("/instances/{id}/sync", h.PostSync)
			// Activación 1-click: materializa la identidad en Sytue ID (SP SAML / client
			// OIDC), resuelve el lanzamiento y agenda el sync. Deactivate lo revierte.
			// Cambia lo que ve TODA la organización al abrir el lanzador, no una
			// preferencia personal ⇒ owner/admin.
			wr.Put("/substitutions/{product}", h.SetSubstitution)
			wr.Post("/instances/{id}/oauth/start", h.OAuthStart)
			// Fin del consentimiento: lo llama la consola con la sesión de quien vuelve
			// del proveedor, y solo canjea si es la de quien lo empezó.
			wr.Post("/instances/{id}/activate", h.Activate)
			wr.Post("/instances/{id}/deactivate", h.Deactivate)

			// Migración de contenido (on-ramp): traerse Drive y Contacts desde
			// Microsoft 365 / Google Workspace. Owner/admin: es acceso a los datos de
			// un usuario del origen con la credencial del conector de la org. El
			// contenido aterriza en la cuenta de CADA persona (token canjeado por
			// usuario), no en la de quien lanza el trabajo.
			// Asistente AUTOMÁTICO: descubrir con la conexión ya autorizada y lanzarlo
			// todo. El plan es de lectura, pero vive con las escrituras porque enseña el
			// directorio del origen y a quién pertenece cada cuenta.
			wr.Get("/instances/{id}/migration-plan", h.MigrationPlan)
			wr.Post("/migrations/auto", h.AutoMigrate)
			wr.Post("/migrations", h.CreateMigration)
			// Llevarse lo suyo sin delegación (cuentas personales, owners y admins).
			wr.Post("/migrations/self/out", h.CreateSelfOutbound)
			wr.Get("/migrations", h.ListMigrations)
			wr.Get("/migrations/{id}", h.GetMigration)
			wr.Post("/migrations/{id}/cancel", h.CancelMigration)
			wr.Post("/migrations/{id}/resume", h.ResumeMigration)
		})

		// Gobernanza L5 (owner/admin): visor + export del registro de egress, CRUD de
		// reglas de política y visor de decisiones (block/warn). El enforcement vive
		// en el RoundTripper; esto es su plano de administración y auditoría.
		api.Group(func(adm chi.Router) {
			adm.Use(d.Verifier.RequireRole("owner", "admin"))
			adm.Get("/admin/egress-audit", h.ListEgressAudit) // ?format=csv exporta.
			adm.Get("/admin/integrations", h.ListIntegrations)
			adm.Put("/admin/integrations/{provider}", h.SetIntegration)
			adm.Get("/admin/policy-decisions", h.ListPolicyDecisions)
			adm.Get("/admin/policies", h.ListPolicies)
			adm.Post("/admin/policies", h.CreatePolicy)
			adm.Patch("/admin/policies/{id}", h.UpdatePolicy)
			adm.Delete("/admin/policies/{id}", h.DeletePolicy)
		})
	})

	return r
}
