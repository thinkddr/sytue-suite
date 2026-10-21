// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package calendarhttp implementa la superficie HTTP de Sytue Calendar: la API
// REST propia (calendarios, eventos, instancias, invitaciones, free/busy) y el
// montaje del servidor CalDAV (RFC 4791) bajo /dav.
//
// Auth: middleware authn (JWT de Sytue ID via JWKS). Aislamiento por tenant via
// RLS (el tenant se inyecta en el contexto desde los claims).
package calendarhttp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
)

// Deps son las dependencias del router HTTP de Calendar.
type Deps struct {
	Cfg           config.Config
	Service       *service.Service
	Verifier      *authn.Verifier
	CalDAV        http.Handler
	ReadyCheckers []httpx.Checker
}

// Registrar los verbos WebDAV en chi ANTES de montar cualquier router CalDAV
// (si no, PROPFIND/REPORT caen en 405). Idempotente.
func init() { httpx.EnableWebDAVMethods() }

// NewRouter construye el router completo de Sytue Calendar.
func NewRouter(d Deps) http.Handler {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 60 * time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)

	h := &Handlers{svc: d.Service, cfg: d.Cfg}
	tenantAuth := func(next http.Handler) http.Handler {
		return d.Verifier.RequireAuth(d.Verifier.RequireTenant(next))
	}
	// ponytail: cubo en memoria, o sea POR RÉPLICA (el tope real se multiplica por
	// el número de instancias). Si hiciera falta que fuera del servicio entero,
	// pg.RateLimiter ya lo comparte por Postgres como hace Sytue ID.
	apiLimiter := httpx.NewTokenBucket(50, 100)

	// El REPLY de una invitación contestada por correo, de Sytue Mail (imip.go).
	r.With(httpx.SecretoInterno(d.Cfg.HubInternalSecret)).Post(RutaRespuestaIMIP, h.respuestaIMIP())

	// API REST autenticada bajo /api/v1.
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Verifier.RequireAuth)
		api.Use(d.Verifier.RequireTenant)
		// Tope por IDENTIDAD del token, DESPUÉS de RequireAuth (antes no hay sujeto
		// que leer). No por IP: detrás del ingress todas las peticiones llegan con
		// la misma, así que un cubo por IP sería un cubo para todos los clientes.
		// Alto a propósito: corta el bucle desbocado y el volcado de la agenda
		// entera, no el uso normal.
		api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: apiLimiter, KeyFunc: authn.RateKey}))

		// Calendarios (colecciones).
		// «Exportar tus datos»: la entrega de la agenda (platform/pkg/portabilidad).
		api.Get("/export", h.Export)
		api.Get("/calendars", h.ListCalendars)
		api.Post("/calendars", h.CreateCalendar)
		// «Compartido conmigo»: calendarios compartidos con el usuario. ANTES de
		// /calendars/{id} para que "shared-with-me" no lo capture chi como un {id}.
		api.Get("/calendars/shared-with-me", h.SharedWithMe)
		api.Get("/calendars/{id}", h.GetCalendar)
		api.Patch("/calendars/{id}", h.UpdateCalendar)
		api.Delete("/calendars/{id}", h.DeleteCalendar)
		// «Descargar .ics» del menú de un calendario: el calendario entero, no un
		// evento suelto (ese se lleva su ical en la respuesta y se descarga en
		// cliente, sin ir al servicio).
		api.Get("/calendars/{id}/export.ics", h.ExportCalendarICS)

		// Compartir un calendario con ACL (solo el dueño gestiona sus shares).
		api.Post("/calendars/{id}/shares", h.CreateShare)
		api.Get("/calendars/{id}/shares", h.ListShares)
		api.Delete("/calendars/{id}/shares/{shareId}", h.RevokeShare)
		// Pedir acceso de edición a un calendario compartido, y la respuesta del dueño
		// (peticiones.go, contrato de platform/pkg/peticiones).
		api.Post("/calendars/{id}/edit-requests", h.PedirEdicion)
		api.Get("/calendars/{id}/edit-requests", h.ListarPeticiones)
		api.Post("/calendars/{id}/edit-requests/{sub}/grant", h.ConcederPeticion)
		api.Delete("/calendars/{id}/edit-requests/{sub}", h.RechazarPeticion)
		// Un calendario compartido se lee y se edita por las rutas normales de
		// abajo: `requireCalendarRead`/`requireCalendarWrite` resuelven por
		// CalendarAccess, que contempla al dueño Y a los shares (sin acceso →
		// 404 sin filtrar existencia; un reader que escribe → 403). Hubo aquí un
		// juego paralelo `/shared-instances` y `/shared-events` de cuando esa
		// verja aún no estaba en los endpoints primarios; retirado al quedar
		// duplicado —comprobaba lo mismo dos veces y ningún cliente lo llamaba—.

		// Suscripciones a calendarios ICS externos («Añadir calendario por URL»).
		api.Post("/subscriptions", h.Subscribe)
		api.Post("/calendars/{id}/refresh", h.RefreshSubscription)

		// Eventos del calendario.
		api.Post("/calendars/{id}/events", h.CreateEvent)
		api.Get("/calendars/{id}/instances", h.ListInstances) // expansion de recurrencias en rango
		// Búsqueda server-side por texto (título/descripción/lugar/asistentes) con
		// rango + calendario opcionales; keyset. ANTES de /events/{id} para que
		// "search" no lo capture chi como un {id}.
		api.Get("/events/search", h.SearchEvents)
		api.Get("/events/invited", h.EventInvited)
		api.Get("/events/{id}", h.GetEvent)
		api.Put("/events/{id}", h.UpdateEvent)
		api.Delete("/events/{id}", h.DeleteEvent)

		// Asistentes / RSVP.
		api.Put("/events/{id}/attendees/partstat", h.SetPartStat)

		// Edición por alcance de series recurrentes (este / este y siguientes).
		api.Post("/calendars/{id}/amend-recurrence", h.AmendRecurrence)

		// Recordatorios in-app (VALARM): cola de vencidos + descartar.
		api.Get("/reminders/due", h.ListDueReminders)
		api.Post("/reminders/{id}/dismiss", h.DismissReminder)

		// Free/Busy (disponibilidad).
		api.Get("/freebusy", h.FreeBusy)
		// Buscar hueco: huecos libres comunes de varios participantes (GET, lectura).
		api.Get("/freebusy/suggest", h.SuggestSlots)

		// Invitaciones (scheduling inbox iTIP/RFC 6638).
		api.Get("/invitations", h.ListInbox)
		api.Post("/invitations/{id}/respond", h.RespondInvitation)

		// Recursos reservables (salas, equipos) — patrón resource-calendar. Listar
		// con disponibilidad y reservar/liberar: cualquier usuario del tenant. Alta:
		// solo admin/owner; edición/borrado: gestor del recurso (o admin).
		api.Get("/resources", h.ListResources)
		api.Post("/resources", h.CreateResource)
		api.Get("/resources/{id}", h.GetResource)
		api.Patch("/resources/{id}", h.UpdateResource)
		api.Delete("/resources/{id}", h.DeleteResource)
		api.Post("/resources/{id}/book", h.BookResource)      // reservar → accept/decline
		api.Delete("/resources/{id}/book", h.ReleaseResource) // liberar (?source_uid=)
	})

	// Capa de compatibilidad ENTRANTE: Google Calendar v3 y Microsoft Graph sobre
	// el MISMO servicio y el MISMO middleware de auth que /api/v1 (ver
	// handlers_compat.go). Prefijos /compat/google y /compat/graph.
	mountCompat(r, &compatHandlers{svc: d.Service, cfg: d.Cfg}, func(next http.Handler) http.Handler {
		return d.Verifier.RequireGoogleAPIs(googleScopesLectura, googleScopesEscritura)(d.Verifier.RequireTenant(next))
	}, tenantAuth)

	// CalDAV (RFC 4791) bajo el prefijo configurado (por defecto /dav),
	// autenticado con el mismo JWT de Sytue ID.
	if d.CalDAV != nil {
		prefix := d.Cfg.CalDAVPrefix
		r.Handle(prefix, tenantAuth(d.CalDAV))
		r.Handle(prefix+"/*", tenantAuth(d.CalDAV))
		// Descubrimiento de servicio (RFC 6764): redirige /.well-known/caldav al
		// raíz DAV. Es lo que permite a Apple Calendario, Thunderbird y los
		// clientes DAV configurarse escribiendo solo el dominio.
		r.Get("/.well-known/caldav", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, prefix+"/", http.StatusMovedPermanently)
		})
	}

	return r
}
