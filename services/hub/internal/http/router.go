// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package hubhttp implementa la superficie HTTP de Sytue Hub: API REST de
// integración (búsqueda federada, notificaciones unificadas, presencia, intents
// cross-app) y el stream SSE de notificaciones en tiempo real.
//
// Auth: middleware authn (JWT de Sytue ID vía JWKS, client sytue-hub).
// Aislamiento por tenant vía RLS (el tenant se inyecta desde los claims).
package hubhttp

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/services/hub/internal/config"
	"git.sytue.eu/sytue/one/services/hub/internal/connectors"
	"git.sytue.eu/sytue/one/services/hub/internal/instaladores"
	"git.sytue.eu/sytue/one/services/hub/internal/intents"
	"git.sytue.eu/sytue/one/services/hub/internal/interpretar"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/presence"
	"git.sytue.eu/sytue/one/services/hub/internal/search"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// Deps son las dependencias del router HTTP de Hub.
type Deps struct {
	Cfg      config.Config
	Verifier *authn.Verifier
	Search   *search.Service
	Notify   *notify.Service
	Hub      *notify.Hub
	Store    *store.Store
	Presence *presence.Service
	Intents  *intents.Resolver
	// Interprete traduce una frase a parámetros de búsqueda. Nil o inactivo =
	// la búsqueda sigue siendo literal, que es lo que hacía hasta ahora.
	Interprete *interpretar.Cliente
	// Instaladores es el catálogo de instaladores nativos. Nil = no configurado.
	Instaladores *instaladores.Catalogo
	// Correo cuenta los correos sin leer para la campana. Nil = sin Mail.
	Correo        *connectors.MailConnector
	ReadyCheckers []httpx.Checker
}

// NewRouter construye el router completo de Sytue Hub.
func NewRouter(d Deps) http.Handler {
	// CORS: la Sytue Shell (islas Preact) llama al Hub desde CADA subdominio de la
	// suite. Allowlist por comodín de subdominio; credenciales para el SSE (EventSource
	// withCredentials). Los headers/métodos por defecto de httpx cubren Authorization.
	r := httpx.NewRouter(httpx.RouterOptions{
		CORS: &httpx.CORSConfig{
			AllowedOrigins:   d.Cfg.CORSAllowedOrigins,
			AllowCredentials: true,
		},
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{
		cfg:        d.Cfg,
		search:     d.Search,
		notify:     d.Notify,
		hub:        d.Hub,
		store:      d.Store,
		presence:   d.Presence,
		intents:    d.Intents,
		interprete: d.Interprete,
	}
	if d.Instaladores != nil {
		h.instaladores = d.Instaladores
	}
	if d.Correo != nil {
		h.correo = d.Correo
	}
	if d.Cfg.IDNombresOrgsSecret != "" {
		h.orgs = personas.NuevosNombresOrgs(d.Cfg.IDBaseURL, d.Cfg.IDNombresOrgsSecret)
	}

	// ponytail: cubo en memoria, o sea POR RÉPLICA (el tope real se multiplica por
	// el número de instancias). Si hiciera falta que fuera del servicio entero,
	// pg.RateLimiter ya lo comparte por Postgres como hace Sytue ID.
	apiLimiter := httpx.NewTokenBucket(50, 100)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		// Tope por IDENTIDAD del token, DESPUÉS de RequireAuth (antes no hay sujeto
		// que leer). No por IP: detrás del ingress todas las peticiones llegan con
		// la misma, así que un cubo por IP sería un cubo para todos los clientes.
		// La búsqueda federada abanica a CINCO servicios por petición, así que aquí
		// el tope también protege a los de detrás.
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// Qué productos levanta esta instalación, para que el conmutador de
		// aplicaciones no ofrezca lo que no está desplegado.
		api.Get("/apps", h.Apps)

		// Búsqueda federada (consulta drive/mail/calendar/contacts/chat).
		// Timeout de petición acotado: la federación ya impone su propio plazo.
		// La búsqueda global lleva lo que escribe la persona, así que se ofrece
		// también por QUERY (en el cuerpo, no en el log de cada salto). GET se
		// queda como respaldo. El `With` se aplica una vez y sirve a los dos.
		httpx.GetYQuery(api.With(requestTimeout(30*time.Second)), "/search", h.Search)
		// Actividad reciente cross-app (mismo fan-out federado, sin término).
		api.With(requestTimeout(30*time.Second)).Get("/recent", h.Recent)

		// Notificaciones unificadas.
		api.Get("/notifications", h.ListNotifications)
		api.Get("/notifications/unread_count", h.UnreadCount)
		api.Post("/notifications", h.CreateNotification) // emitida por servicios/console
		// Preferencias por origen/tipo. Siempre del usuario del token: no hay ruta para
		// configurar las de otro, ni con scope.
		api.Get("/notifications/prefs", h.ListNotificationPrefs)
		api.Put("/notifications/prefs", h.SetNotificationPref)
		api.Post("/notifications/{id}/read", h.MarkRead)
		api.Post("/notifications/read_all", h.MarkAllRead)
		// Stream SSE en tiempo real (sin timeout de petición: conexión larga).
		api.Get("/notifications/stream", h.Stream)

		// Presencia.
		api.Put("/presence", h.Heartbeat)
		api.Delete("/presence", h.Offline)
		api.Get("/presence", h.QueryPresence) // ?subjects=a,b,c

		// Intents cross-app (devuelven deep-links).
		api.Post("/intents", h.ResolveIntent)

		// Exportar tus datos: un ZIP con la entrega de cada producto. Sin plazo de
		// petición: dura lo que tarde la persona en descargarlo (ver export.go).
		api.Get("/export", h.Export)

		// El actualizador de las apps de escritorio: qué versión hay de cada una y de
		// dónde bajarla. Tener sesión ya es tener derecho, como en la página de
		// descargas del portal (ver internal/instaladores).
		api.Get("/instaladores", h.ListarInstaladores)
		// Antes que `{fichero}`: «sumas» no es un nombre de instalador.
		api.Get("/instaladores/sumas", h.SumasInstaladores)
		api.Get("/instaladores/{fichero}", h.BajarInstalador)
	})

	// Endpoint INTERNO service-to-service. No público: autentica el secreto
	// compartido (Authorization: Bearer <HUB_INTERNAL_SECRET>), fuera de /api/v1,
	// y solo si InternalSecret está configurado.
	if d.Cfg.InternalSecret != "" {
		r.With(h.internalAuth).Post("/internal/notifications", h.CreateInternalNotification)
		r.With(h.internalAuth).Post("/internal/events", h.CreateInternalEvent)
		r.With(h.internalAuth).Post("/internal/notifications/forget", h.OlvidarBuzonInterno)
		r.With(h.internalAuth).Post("/internal/notifications/historic", h.CreateInternalHistoricNotification)
	}

	return r
}

// requestTimeout aplica un plazo a un subárbol de rutas concreto.
func requestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
