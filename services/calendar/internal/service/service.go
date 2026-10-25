// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service orquesta la logica de aplicacion de Sytue Calendar: combina el
// store (PostgreSQL) con el puente iCalendar (go-ical), la expansion de
// recurrencias (RFC 5545), el calculo de free/busy y el scheduling iTIP/iMIP
// (entrega de invitaciones a los destinatarios del tenant). Publica eventos de
// dominio en el outbox (sobre PostgreSQL; sin Redis/Valkey).
package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
	"git.sytue.eu/sytue/one/services/calendar/internal/subscriptions"
)

// eventChannel es el canal logico de eventos de Calendar.
const eventChannel events.Channel = "calendar"

// EventPublisher publica eventos de dominio (outbox).
type EventPublisher interface {
	Publish(ctx context.Context, ev events.Event) error
}

// Service implementa los casos de uso de Calendar.
type Service struct {
	cfg   config.Config
	store *store.Store
	pub   EventPublisher
	// subClient obtiene los feeds ICS suscritos SSRF-safe (barrera doble). Se
	// construye una vez (pool de conexiones) y lo comparten el worker y el
	// «refrescar ahora».
	subClient *subscriptions.Client
	// hub deja en la campana de cada invitado el aviso de su invitación. Nil: no avisa.
	hub *hubnotify.Client
	// correo manda las invitaciones iMIP a quien es de fuera (imip.go); nil, ninguna.
	correo EnviarCorreo
	// cupoCorreo acota cuántos de esos correos manda cada persona por hora.
	cupoCorreo cupoHora
	// internos son los dominios de correo de Sytue Mail; se renuevan en caliente.
	internos atomic.Pointer[[]string]
	// avisador avisa (Hub y correo) de las peticiones de edición (peticiones.go).
	avisador peticiones.Avisador
	// nombresOrgs da el nombre visible de la organización que convoca; nil, ninguno.
	nombresOrgs NombresOrgs
}

// NombresOrgs da el nombre visible de una organización por su slug (personas.NombresOrgs).
type NombresOrgs interface {
	Nombres(ctx context.Context, slugs []string) map[string]string
}

// SetNombresOrgs pone de dónde sale el nombre visible de la organización que convoca.
func (s *Service) SetNombresOrgs(n NombresOrgs) { s.nombresOrgs = n }

// SetHubNotifier cablea el cliente del Hub para los avisos de invitación.
func (s *Service) SetHubNotifier(c *hubnotify.Client) { s.hub = c }

// InvitarNuevos entrega la invitación a los asistentes que un evento guardado por
// CalDAV trae y antes no tenía. Es el camino de la web —guarda por CalDAV, no por la
// API con `send`—, y hasta el 2026-09-24 un invitado añadido allí no recibía nada: ni
// fila en su bandeja de invitaciones ni aviso.
//
// Solo invita quien ORGANIZA (la verja está en deliverInvitations, que es por donde
// pasan todas las entregas).
func (s *Service) InvitarNuevos(ctx context.Context, ev domain.Event, antes []domain.Attendee) {
	var nuevos []domain.Attendee
	for _, a := range ev.Attendees {
		if !slices.ContainsFunc(antes, func(b domain.Attendee) bool { return strings.EqualFold(b.Email, a.Email) }) {
			nuevos = append(nuevos, a)
		}
	}
	if len(nuevos) == 0 {
		return
	}
	ev.Attendees = nuevos
	s.deliverInvitations(ctx, ev, domain.MethodRequest)
}

// avisarInvitacion deja el aviso de una invitación en la campana del invitado. Se le
// conoce por su dirección, así que el aviso va a ella (`hubnotify.PorCorreo`) y abre
// su bandeja de invitaciones: el evento todavía no es suyo.
func (s *Service) avisarInvitacion(ctx context.Context, ev domain.Event, email string) {
	if s.hub == nil {
		return
	}
	t, _ := tenancy.From(ctx)
	actor := cleanAddr(ev.Organizer)
	if c, ok := authn.FromContext(ctx); ok {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email, actor)
	}
	s.hub.SendAsync(ctx, avisoDeInvitacion(s.cfg.PublicURL, t.Slug, email, actor, ev))
}

// avisoDeInvitacion es el aviso de una invitación: el mismo en vivo que en la
// reconstrucción de avisos (AvisosHistoricos).
func avisoDeInvitacion(publicURL, tenant, email, actor string, ev domain.Event) hubnotify.Notification {
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: hubnotify.PorCorreo(email),
		Source:  "calendar",
		Type:    "calendar.invitation",
		Level:   "info",
		Title:   fmt.Sprintf("%s te ha invitado a «%s»", actor, ev.Summary),
		// Sin la organización: lleva a la bandeja de quien lo abre, que es también donde
		// ve una invitación de otra organización (COM-15); el enlace no abre nada de la
		// que convoca.
		URL:    hubnotify.EnlaceInvitaciones(publicURL, ""),
		Params: map[string]string{"actor": actor, "title": ev.Summary, "start": ev.DTStart.UTC().Format(time.RFC3339)},
	}
}

// New construye el servicio.
func New(cfg config.Config, st *store.Store, pub EventPublisher) *Service {
	return &Service{
		cfg:       cfg,
		store:     st,
		pub:       pub,
		subClient: subscriptions.NewClient(cfg.SubscriptionMaxBytes),
	}
}

// Store expone el store subyacente (lo consume el backend CalDAV).
func (s *Service) Store() *store.Store { return s.store }

// publish emite un evento de dominio (best effort).
func (s *Service) publish(ctx context.Context, eventType string, payload any) {
	if s.pub == nil {
		return
	}
	t, _ := tenancy.From(ctx)
	ev, err := events.NewEvent(eventChannel, eventType, t.Slug, payload)
	if err != nil {
		log.From(ctx).Warn("calendar: construir evento", "type", eventType, "error", err)
		return
	}
	if err := s.pub.Publish(ctx, ev); err != nil {
		log.From(ctx).Warn("calendar: publicar evento", "type", eventType, "error", err)
	}
}

// --- Verja de acceso de la API REST primaria ---------------------------------

// caller extrae la identidad (sub, email) del JWT verificado del contexto de la
// peticion. Los endpoints REST primarios llegan siempre tras authn.RequireAuth;
// un contexto sin claims produce identidad vacia, que nunca casa en la verja
// (deny-by-default). CalDAV y los workers NO pasan por estos metodos: van
// directos al store (CalDAV ata la ruta al sub; los workers operan por tenant).
func caller(ctx context.Context) (sub, email string) {
	c, _ := authn.FromContext(ctx)
	return c.Subject, c.Email
}

// requireCalendarRead exige acceso de LECTURA del llamante al calendario
// (dueño o share reader|writer|manage). Sin acceso → ErrNotFound (404, sin
// filtrar existencia). Es la MISMA verja CalendarAccess de los Shared*,
// aplicada a los endpoints primarios (/calendars/{id}, /events/{id}).
func (s *Service) requireCalendarRead(ctx context.Context, calendarID uuid.UUID) error {
	sub, email := caller(ctx)
	_, err := s.store.CalendarAccess(ctx, sub, email, calendarID)
	return err
}

// requireCalendarWrite exige ESCRITURA (dueño|writer|manage); un reader recibe
// ErrReadOnly (403) y sin acceso, ErrNotFound (404). Reusa assertShareWritable.
func (s *Service) requireCalendarWrite(ctx context.Context, calendarID uuid.UUID) error {
	sub, email := caller(ctx)
	return s.assertShareWritable(ctx, sub, email, calendarID)
}

// --- Calendarios ------------------------------------------------------------

// EnsureDefaultCalendar garantiza el calendario por defecto del usuario.
func (s *Service) EnsureDefaultCalendar(ctx context.Context, ownerSub string) (domain.Calendar, error) {
	return s.store.EnsureDefaultCalendar(ctx, ownerSub)
}

// ListCalendars lista los calendarios del usuario.
func (s *Service) ListCalendars(ctx context.Context, ownerSub string) ([]domain.Calendar, error) {
	return s.store.ListCalendars(ctx, ownerSub)
}

// ListObjects devuelve los objetos iCalendar GUARDADOS de un calendario, tal
// cual los recibió CalDAV (para exportarlos sin perder lo que Sytue no interpreta).
func (s *Service) ListObjects(ctx context.Context, calendarID uuid.UUID) ([]domain.CalendarObject, error) {
	if err := s.requireCalendarRead(ctx, calendarID); err != nil {
		return nil, err
	}
	return s.store.ListObjects(ctx, calendarID)
}

// GetCalendar devuelve un calendario por ID, si el llamante tiene acceso
// (dueño o share); si no, ErrNotFound (404, anti-enumeracion).
func (s *Service) GetCalendar(ctx context.Context, id uuid.UUID) (domain.Calendar, error) {
	if err := s.requireCalendarRead(ctx, id); err != nil {
		return domain.Calendar{}, err
	}
	return s.store.GetCalendar(ctx, id)
}

// CreateCalendar crea un calendario y emite un evento.
func (s *Service) CreateCalendar(ctx context.Context, nc store.NewCalendar) (domain.Calendar, error) {
	c, err := s.store.CreateCalendar(ctx, nc)
	if err != nil {
		return domain.Calendar{}, err
	}
	s.publish(ctx, "calendar.collection.created", map[string]any{"calendar_id": c.ID, "name": c.Name})
	return c, nil
}

// requireCalendarOwner exige ser el DUEÑO del calendario, no solo poder escribir en él.
//
// La diferencia importa: un `writer` compartido puede crear y editar eventos, que es
// para lo que se le compartió. Renombrar o BORRAR el calendario destruye el contenedor
// entero y con él los eventos del dueño — y eso se estaba gateando con el mismo permiso
// de escritura, así que cualquiera con un share de escritura podía borrar el calendario
// de otra persona. Escribir dentro de algo no es ser dueño de ese algo.
func (s *Service) requireCalendarOwner(ctx context.Context, calendarID uuid.UUID) error {
	sub, _ := caller(ctx)
	// Primero la verja de LECTURA: sin acceso, el calendario «no existe» (404
	// anti-enumeración), en vez de un 403 que confirmaría que ese id es de alguien.
	if err := s.requireCalendarRead(ctx, calendarID); err != nil {
		return err
	}
	cal, err := s.store.GetCalendar(ctx, calendarID)
	if err != nil {
		return err
	}
	if cal.OwnerSub != sub {
		return domain.ErrReadOnly
	}
	return nil
}

// UpdateCalendar aplica un patch a un calendario. Exige escritura (dueño|
// writer|manage); reader → 403; sin acceso → 404.
// ponytail: writer/manage tambien edita metadatos; afinar a owner-only si hace falta.
func (s *Service) UpdateCalendar(ctx context.Context, id uuid.UUID, p store.CalendarPatch) (domain.Calendar, error) {
	// Propiedad, no escritura: renombrar el calendario de otra persona o cambiarle el
	// color no es parte de «poder escribir eventos en él».
	if err := s.requireCalendarOwner(ctx, id); err != nil {
		return domain.Calendar{}, err
	}
	return s.store.UpdateCalendar(ctx, id, p)
}

// DeleteCalendar borra un calendario. Exige ser su DUEÑO: borrarlo destruye los eventos
// de esa persona, y un share de escritura se concede para escribir eventos, no para eso.
func (s *Service) DeleteCalendar(ctx context.Context, id uuid.UUID) error {
	if err := s.requireCalendarOwner(ctx, id); err != nil {
		return err
	}
	if err := s.store.DeleteCalendar(ctx, id); err != nil {
		return err
	}
	s.publish(ctx, "calendar.collection.deleted", map[string]any{"calendar_id": id})
	return nil
}

// --- Eventos ----------------------------------------------------------------

// EventInput son los datos de alta/edicion de un evento via API REST. El
// servicio sintetiza el iCalendar canonico si no se aporta (cliente REST) o lo
// reusa verbatim (cliente CalDAV via PutObject).
type EventInput struct {
	CalendarID     uuid.UUID
	UID            string // vacio => se genera
	Summary        string
	Description    string
	Location       string
	Organizer      string
	DTStart        time.Time
	DTEnd          time.Time
	TZID           string
	AllDay         bool
	Status         domain.EventStatus
	Classification domain.Classification
	Transparency   domain.Transparency
	// Color es el color del evento (hex #RRGGBB o nombre CSS3); se sanea al guardar.
	Color string
	// ConferenceURL es la videollamada del evento (RFC 7986 CONFERENCE). Se sanea al
	// guardar: solo http(s), porque acaba en un ICS que leen clientes de terceros.
	ConferenceURL string
	RRule         string
	RDate         []time.Time
	ExDate        []time.Time
	Attendees     []domain.Attendee
	// Send indica si, al guardar, se entregan invitaciones iTIP a los asistentes.
	Send bool
	// Reminders son los recordatorios del evento, en minutos ANTES del inicio (0, en el
	// momento), los que la web deja editar. nil: la petición no los trae y los que
	// tuviera el evento se conservan TODOS. No nil —aunque vacío—: sustituyen a los
	// editables, y los que la API no sabe expresar (correo, hora fija…) se conservan.
	Reminders *[]int
}

// Límites de los recordatorios que se aceptan por REST: la interfaz pone unos pocos, y
// el más lejano que ofrece son semanas. Un año cubre cualquier «personalizado».
const (
	maxReminders       = 20
	maxReminderMinutes = 366 * 24 * 60
)

// CreateEvent crea un evento puntual o recurrente desde la API REST. Exige
// escritura sobre el calendario destino (dueño|writer|manage) ANTES de validar
// el cuerpo: un calendario ajeno responde 404 sin oraculo de validacion.
func (s *Service) CreateEvent(ctx context.Context, in EventInput) (domain.Event, error) {
	if err := s.requireCalendarWrite(ctx, in.CalendarID); err != nil {
		return domain.Event{}, err
	}
	in.Organizer = organizador(ctx, in.Organizer, "")
	if in.Send {
		if err := s.puedeInvitar(ctx, in); err != nil {
			return domain.Event{}, err
		}
	}
	storeIn, err := s.toStoreInput(in)
	if err != nil {
		return domain.Event{}, err
	}
	ev, _, err := s.store.CreateEvent(ctx, storeIn)
	if err != nil {
		return domain.Event{}, err
	}
	if in.Send {
		s.deliverInvitations(ctx, ev, domain.MethodRequest)
	}
	s.publish(ctx, "calendar.event.created", map[string]any{
		"calendar_id": ev.CalendarID, "event_id": ev.ID, "uid": ev.UID,
	})
	return ev, nil
}

// UpdateEvent actualiza un evento existente (por UID en su calendario). Exige
// escritura sobre el calendario (dueño|writer|manage); reader → 403; ajeno → 404.
func (s *Service) UpdateEvent(ctx context.Context, in EventInput) (domain.Event, error) {
	if err := s.requireCalendarWrite(ctx, in.CalendarID); err != nil {
		return domain.Event{}, err
	}
	if in.UID == "" {
		return domain.Event{}, domain.ErrInvalidEvent
	}
	cur, errCur := s.store.GetObjectByUID(ctx, in.CalendarID, in.UID)
	// Editar no cambia de organizador: se conserva el que el evento ya tenía.
	organizaba := ""
	if errCur == nil {
		if po, err := caldav.DecodeCalendarObject(cur.Data, in.CalendarID.String()); err == nil {
			organizaba = po.Master.Organizer
		}
	}
	in.Organizer = organizador(ctx, in.Organizer, organizaba)
	if in.Send {
		if err := s.puedeInvitar(ctx, in); err != nil {
			return domain.Event{}, err
		}
	}
	storeIn, err := s.toStoreInput(in)
	if err != nil {
		return domain.Event{}, err
	}
	// Incrementa SEQUENCE para reflejar el cambio a los asistentes (RFC 5546).
	if errCur == nil {
		storeIn.SequenceNo = cur.SequenceNo + 1
		// Esta API no sabe de recordatorios: sin esto, editar por REST un evento se los
		// quitaba a quien los puso en la web (ver caldav.CarryOver).
		conservado, err := caldav.CarryOver(storeIn.ICalData, cur.Data, in.Reminders != nil)
		if err != nil {
			return domain.Event{}, err
		}
		storeIn.ICalData = conservado
	}
	ev, _, err := s.store.UpdateEvent(ctx, storeIn, uuid.Nil)
	if err != nil {
		return domain.Event{}, err
	}
	if in.Send {
		s.deliverInvitations(ctx, ev, domain.MethodRequest)
	}
	s.publish(ctx, "calendar.event.updated", map[string]any{
		"calendar_id": ev.CalendarID, "event_id": ev.ID, "uid": ev.UID,
	})
	return ev, nil
}

// EventReminders son los recordatorios editables de un evento YA LEÍDO con GetEvent (que
// es quien pasa la verja de lectura), en minutos antes del inicio. La fila del evento no
// los guarda: salen de su recurso iCalendar, igual que los lee la web por CalDAV.
func (s *Service) EventReminders(ctx context.Context, ev domain.Event) ([]int, error) {
	obj, err := s.store.GetObjectByUID(ctx, ev.CalendarID, ev.UID)
	if err != nil {
		return nil, err
	}
	return caldav.Recordatorios(obj.Data), nil
}

// GetEvent devuelve un evento por ID, si el llamante tiene acceso a su
// calendario (dueño o share); si no, ErrNotFound (404, anti-enumeracion).
// EsInvitadoDeEvento: ¿quien llama organiza el evento de ese UID o está invitado a él?
// Lo pregunta Rooms para abrir la reunión del evento a sus invitados.
func (s *Service) EsInvitadoDeEvento(ctx context.Context, uid string) (bool, error) {
	c, _ := authn.FromContext(ctx)
	var dirs []string
	for _, d := range append([]string{c.Email, c.Mailbox}, c.Mailboxes...) {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 && c.Subject == "" {
		return false, nil
	}
	return s.store.EsInvitadoDeEvento(ctx, strings.TrimSpace(uid), c.Subject, dirs)
}

func (s *Service) GetEvent(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	ev, err := s.store.GetEvent(ctx, id)
	if err != nil {
		return domain.Event{}, err
	}
	if err := s.requireCalendarRead(ctx, ev.CalendarID); err != nil {
		return domain.Event{}, err
	}
	return ev, nil
}

// SearchEvents busca los eventos del usuario por texto (título/descripción/lugar/
// organizador/asistentes) con rango y calendario opcionales, paginado por keyset.
// Fino passthrough al store (aislamiento por tenant+dueño, término parametrizado;
// devuelve el maestro de cada serie, no las ocurrencias expandidas).
func (s *Service) SearchEvents(ctx context.Context, q store.EventSearchQuery) ([]domain.Event, error) {
	return s.store.SearchEvents(ctx, q)
}

// DeleteEvent borra un evento por ID y, si tenia asistentes, entrega CANCEL.
// Exige escritura sobre el calendario del evento; reader → 403; ajeno → 404.
func (s *Service) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	return s.DeleteEventNotifying(ctx, id, true)
}

// DeleteEventNotifying es DeleteEvent eligiendo si se entrega el CANCEL. Existe por la
// emulación de Google Calendar, donde borrar sin `sendUpdates=all` no avisa a nadie.
func (s *Service) DeleteEventNotifying(ctx context.Context, id uuid.UUID, send bool) error {
	ev, err := s.store.GetEvent(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireCalendarWrite(ctx, ev.CalendarID); err != nil {
		return err
	}
	if err := s.store.DeleteEventByID(ctx, id); err != nil {
		return err
	}
	if send {
		s.deliverInvitations(ctx, ev, domain.MethodCancel)
	}
	s.publish(ctx, "calendar.event.deleted", map[string]any{"event_id": id, "uid": ev.UID})
	return nil
}

// ListInstances expande los eventos de un calendario en instancias concretas
// dentro de [from, to) (calendar-query, RFC 4791 4.1).
func (s *Service) ListInstances(ctx context.Context, calendarID uuid.UUID, from, to time.Time) ([]domain.Instance, error) {
	if !to.After(from) {
		return nil, domain.ErrInvalidRange
	}
	// Verja de lectura: dueño o share; sin acceso → 404 sin filtrar existencia.
	if err := s.requireCalendarRead(ctx, calendarID); err != nil {
		return nil, err
	}
	evts, err := s.store.ListEventsInRange(ctx, calendarID, from, to)
	if err != nil {
		return nil, err
	}
	return domain.ExpandEvents(evts, from.UTC(), to.UTC()), nil
}

// ListEventsInRange devuelve los eventos de un calendario que pueden tener
// instancias en [from, to) SIN expandir las series (el maestro con su RRULE, mas
// sus overrides). Es lo que piden los dialectos de compatibilidad cuando el
// cliente NO pide ocurrencias (Google sin singleEvents=true, Graph /me/events).
// Misma verja de lectura que ListInstances: sin acceso → ErrNotFound (404).
func (s *Service) ListEventsInRange(ctx context.Context, calendarID uuid.UUID, from, to time.Time) ([]domain.Event, error) {
	if !to.After(from) {
		return nil, domain.ErrInvalidRange
	}
	if err := s.requireCalendarRead(ctx, calendarID); err != nil {
		return nil, err
	}
	return s.store.ListEventsInRange(ctx, calendarID, from, to)
}

// SetAttendeePartStat fija el PARTSTAT de un asistente de un evento (RSVP). Es
// un fino passthrough al store que evita que la capa HTTP dependa del store.
func (s *Service) SetAttendeePartStat(ctx context.Context, eventID uuid.UUID, email string, partstat domain.ParticipationStatus) (domain.Attendee, error) {
	return s.store.SetAttendeePartStat(ctx, eventID, email, partstat)
}

// --- Recordatorios in-app ---------------------------------------------------

// DueReminders devuelve los recordatorios vencidos y no descartados del usuario
// (entrega in-app), paginados por keyset. Fino passthrough al store.
func (s *Service) DueReminders(ctx context.Context, ownerSub string, now time.Time, before *store.ReminderCursor, limit int) ([]store.DueReminder, error) {
	return s.store.ListDueReminders(ctx, ownerSub, now, before, limit)
}

// DismissReminder descarta un recordatorio in-app del usuario.
func (s *Service) DismissReminder(ctx context.Context, ownerSub string, id uuid.UUID) error {
	return s.store.DismissReminder(ctx, ownerSub, id)
}

// --- Edición de series recurrentes ------------------------------------------

// AmendRecurrence aplica un límite de alcance a una serie recurrente sin
// corromperla, reutilizando el escritor CalDAV (PutObject recompute índice,
// recurrence_end y cola de recordatorios):
//   - mode "exclude" (alcance «solo este»): añade EXDATE=recurrenceID al maestro.
//   - mode "until"   (alcance «este y siguientes»): recorta el UNTIL del RRULE del
//     maestro justo antes de esta instancia.
//
// La nueva instancia/serie suelta (para una edición, no un borrado) la crea el
// cliente como un objeto independiente por el mismo PUT CalDAV. Conserva VALARM/
// ATTENDEE, EXDATE/RDATE previos y los overrides RECURRENCE-ID del maestro.
func (s *Service) AmendRecurrence(ctx context.Context, calendarID uuid.UUID, uid string, recurrenceID time.Time, mode string, expectETag uuid.UUID) (domain.CalendarObject, error) {
	// Verja de escritura ANTES de leer el objeto: calendario ajeno → 404.
	if err := s.requireCalendarWrite(ctx, calendarID); err != nil {
		return domain.CalendarObject{}, err
	}
	obj, err := s.store.GetObjectByUID(ctx, calendarID, uid)
	if err != nil {
		return domain.CalendarObject{}, err
	}
	amended, err := caldav.AmendMasterRecurrence(obj.Data, recurrenceID, caldav.AmendMode(mode))
	if err != nil {
		return domain.CalendarObject{}, fmt.Errorf("%w: %w", domain.ErrInvalidEvent, err)
	}
	return s.PutObject(ctx, calendarID, amended, expectETag, false)
}

// --- Free/Busy --------------------------------------------------------------

// FreeBusy calcula los periodos ocupados de un usuario (por su sub) en una
// ventana, agregando todos sus calendarios (RFC 4791 free-busy-query / RFC 5545
// FREEBUSY).
func (s *Service) FreeBusy(ctx context.Context, ownerSub string, from, to time.Time) ([]domain.FreeBusyPeriod, error) {
	if !to.After(from) {
		return nil, domain.ErrInvalidRange
	}
	if to.Sub(from) > s.cfg.MaxFreeBusyWindow {
		return nil, fmt.Errorf("%w: ventana free/busy demasiado amplia (max %s)", domain.ErrInvalidRange, s.cfg.MaxFreeBusyWindow)
	}
	evts, err := s.store.ListEventsForOwnerInRange(ctx, ownerSub, from, to)
	if err != nil {
		return nil, err
	}
	instances := domain.ExpandEvents(evts, from.UTC(), to.UTC())
	return domain.FreeBusy(instances, from.UTC(), to.UTC()), nil
}

// maxSuggestParticipants acota cuantos participantes se consultan en un «buscar
// hueco»: una consulta agregada de free/busy por participante (sin N+1), cota
// dura para acotar el trabajo por peticion.
const maxSuggestParticipants = 50

// SuggestSlots calcula los HUECOS libres comunes de un conjunto de participantes
// (`users`, por su sub) en [from, to), reutilizando el free/busy de cada uno: una
// consulta agregada por participante (N consultas, no N+1). La franja laboral wh
// se interpreta en loc. El free/busy respeta el aislamiento por tenant (RLS) y
// solo expone ocupado/libre, nunca detalles de eventos ajenos.
func (s *Service) SuggestSlots(ctx context.Context, users []string, from, to time.Time, dur time.Duration, loc *time.Location, wh domain.WorkingHours, limit int) ([]domain.FreeSlot, error) {
	if !to.After(from) {
		return nil, domain.ErrInvalidRange
	}
	if to.Sub(from) > s.cfg.MaxFreeBusyWindow {
		return nil, fmt.Errorf("%w: ventana demasiado amplia (max %s)", domain.ErrInvalidRange, s.cfg.MaxFreeBusyWindow)
	}
	if dur <= 0 {
		return nil, fmt.Errorf("%w: duracion invalida", domain.ErrInvalidRange)
	}

	seen := make(map[string]struct{}, len(users))
	var busy []domain.FreeBusyPeriod
	for _, u := range users {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		if len(seen) >= maxSuggestParticipants {
			return nil, fmt.Errorf("%w: demasiados participantes (max %d)", domain.ErrInvalidRange, maxSuggestParticipants)
		}
		seen[u] = struct{}{}
		periods, err := s.FreeBusy(ctx, u, from, to)
		if err != nil {
			return nil, err
		}
		busy = append(busy, periods...)
	}
	return domain.SuggestFreeSlots(busy, from, to, loc, wh, dur, limit), nil
}

// --- Invitaciones (iTIP / scheduling, RFC 5546 + RFC 6638) ------------------

// ListInbox lista la scheduling inbox de un destinatario (email): las invitaciones de
// su organización y, detrás, las que le llegan de OTRAS a sus direcciones personales o a
// su `sub` (COM-15), marcadas con la organización que convoca y con el título y la hora
// de su REQUEST, porque su evento no se lee desde aquí.
func (s *Service) ListInbox(ctx context.Context, recipient string, onlyPending bool, limit int) ([]domain.Invitation, error) {
	propias, err := s.store.ListInbox(ctx, recipient, onlyPending, limit)
	if err != nil {
		return nil, err
	}
	t, _ := tenancy.From(ctx)
	sub, direcciones := personales(ctx)
	if sub == "" || t.Slug == "" {
		return propias, nil
	}
	fuera, err := s.store.InvitacionesDeFuera(ctx, t.Slug, direcciones, sub, onlyPending, limit)
	if err != nil {
		return nil, err
	}
	// El nombre visible de la organización que convoca (decisión del dueño, 17/10): solo
	// de las que invitan a ESTA persona, que es la verja de Sytue ID.
	var nombres map[string]string
	if s.nombresOrgs != nil && len(fuera) > 0 {
		slugs := make([]string, 0, len(fuera))
		for _, f := range fuera {
			slugs = append(slugs, f.Tenant)
		}
		nombres = s.nombresOrgs.Nombres(ctx, slugs)
	}
	for i := range fuera {
		fuera[i].Organization = fuera[i].Tenant
		fuera[i].OrganizationName = nombres[fuera[i].Tenant]
		if obj, err := caldav.DecodeCalendarObject(fuera[i].Data, ""); err == nil {
			fuera[i].Summary = obj.Master.Summary
			if !obj.Master.DTStart.IsZero() {
				inicio := obj.Master.DTStart.UTC()
				fuera[i].Start = &inicio
			}
		}
	}
	return append(propias, fuera...), nil
}

// personales son el `sub` y las direcciones de la PERSONA en todas sus organizaciones,
// en minúsculas: las que Sytue ID firma en su token (`mailbox` y `mailboxes`), como la
// campana del Hub (ADR 0053). El correo de entrar no está: nadie ha probado que sea
// suyo. Un token DELEGADO no tiene ninguna: lo que es de la persona fuera de esta
// organización no se abre a quien actúa en su nombre.
func personales(ctx context.Context) (string, []string) {
	c, ok := authn.FromContext(ctx)
	if !ok || c.Delegado || c.Subject == "" {
		return "", nil
	}
	out := []string{}
	for _, d := range append([]string{c.Mailbox}, c.Mailboxes...) {
		if d = domain.NormalizeEmail(d); d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return c.Subject, out
}

// RespondInvitation registra la respuesta de un invitado (accept/decline/
// tentative), actualiza su PARTSTAT y entrega un REPLY al organizador. Solo el
// DESTINATARIO puede responder: el store casa recipient/recipient_sub contra la
// identidad del JWT del llamante; si no casa → ErrNotFound (404).
//
// Una invitación de OTRA organización (COM-15) se responde dentro de la anfitriona: se
// busca allí por sus direcciones personales y su `sub`, y el REPLY y el PARTSTAT van a
// esa organización, sin tocar nada más de ella.
func (s *Service) RespondInvitation(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) (domain.Invitation, error) {
	sub, email := caller(ctx)
	inv, err := s.store.RespondInvitation(ctx, id, status, []string{email}, sub)
	if errors.Is(err, domain.ErrNotFound) {
		ctx, inv, err = s.responderDeFuera(ctx, id, status)
	}
	if err != nil {
		return domain.Invitation{}, err
	}
	if inv.EventID != nil && inv.Organizer != "" {
		if ev, err := s.store.GetEvent(ctx, *inv.EventID); err == nil {
			s.deliverReply(ctx, ev, inv.Recipient)
		}
	}
	s.publish(ctx, "calendar.invitation.responded", map[string]any{
		"invitation_id": inv.ID, "status": inv.Status,
	})
	return inv, nil
}

// responderDeFuera responde una invitación de otra organización, con el contexto de la
// anfitriona para lo que sigue (el REPLY y el evento publicado). ErrNotFound si no es
// una invitación de fuera a esta persona.
func (s *Service) responderDeFuera(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) (context.Context, domain.Invitation, error) {
	t, _ := tenancy.From(ctx)
	sub, direcciones := personales(ctx)
	if sub == "" {
		return ctx, domain.Invitation{}, domain.ErrNotFound
	}
	anfitriona, err := s.store.OrganizacionDeInvitacion(ctx, id, t.Slug, direcciones, sub)
	if err != nil {
		return ctx, domain.Invitation{}, err
	}
	if anfitriona == "" {
		return ctx, domain.Invitation{}, domain.ErrNotFound
	}
	alla, err := tenancy.New(anfitriona)
	if err != nil {
		return ctx, domain.Invitation{}, domain.ErrNotFound
	}
	ctx = tenancy.Into(ctx, alla)
	inv, err := s.store.RespondInvitation(ctx, id, status, direcciones, sub)
	return ctx, inv, err
}

// deliverInvitations entrega un mensaje iTIP (REQUEST/CANCEL) a cada asistente
// con direccion de correo. Es best-effort: una entrega fallida no rompe la
// operacion de calendario (el organizador puede reenviar). La entrega externa
// (iMIP/SMTP) la realiza Sytue Mail al consumir el evento del outbox; aqui
// persistimos el mensaje en la scheduling inbox del tenant.
//
// Solo entrega quien ORGANIZA (COM-07). El calendario de un invitado también guarda el
// evento con su lista de asistentes, y cualquiera con escritura puede editarlo: sin
// esta verja su copia invitaba —o cancelaba— a todos en nombre de quien organiza, por
// la bandeja y por correo. Está aquí y no en cada llamante porque por aquí pasan los
// cuatro: crear, editar, borrar y el guardado por CalDAV.
func (s *Service) deliverInvitations(ctx context.Context, ev domain.Event, method domain.SchedulingMethod) {
	if len(ev.Attendees) == 0 {
		return
	}
	if c, ok := authn.FromContext(ctx); !ok || !esDeLaSesion(c, cleanAddr(ev.Organizer)) {
		return
	}
	if len(ev.Attendees) > MaxInvitados {
		// La API lo rechaza antes de guardar (puedeInvitar); por CalDAV el evento ya
		// está guardado y lo que no se hace es repartirlo.
		log.From(ctx).Warn("calendar: demasiados invitados, no se entrega", "uid", ev.UID, "invitados", len(ev.Attendees))
		return
	}
	data, err := caldav.EncodeEvent(ev, method)
	if err != nil {
		log.From(ctx).Warn("calendar: codificar iTIP", "uid", ev.UID, "error", err)
		return
	}
	for _, a := range ev.Attendees {
		email := strings.TrimSpace(a.Email)
		if email == "" || strings.EqualFold(email, cleanAddr(ev.Organizer)) {
			continue
		}
		evID := ev.ID
		if _, err := s.store.CreateInvitation(ctx, store.NewInvitation{
			EventID:    &evID,
			UID:        ev.UID,
			Method:     method,
			Organizer:  ev.Organizer,
			Recipient:  email,
			SequenceNo: ev.SequenceNo,
			Data:       data,
		}); err != nil {
			log.From(ctx).Warn("calendar: entregar invitacion", "recipient", logsafe.Text(email), "error", err)
		} else if method == domain.MethodRequest {
			s.avisarInvitacion(ctx, ev, email)
		}
	}
	s.mandarIMIP(ctx, ev, method, data)
	s.publish(ctx, "calendar.invitation.sent", map[string]any{
		"uid": ev.UID, "method": method, "attendees": len(ev.Attendees),
	})
}

// MaxInvitados es a cuánta gente reparte un evento sus invitaciones de una vez. Es el
// tope de los calendarios de uso común; una convocatoria mayor es una lista de correo.
const MaxInvitados = 200

// esDeLaSesion dice si una dirección es de quien tiene la sesión.
func esDeLaSesion(c authn.Claims, dir string) bool {
	return dir != "" && (strings.EqualFold(dir, c.Email) || strings.EqualFold(dir, c.Mailbox))
}

// organizador decide quién organiza un evento escrito por la API (COM-07): quien lo
// escribe. Lo que diga el cuerpo solo vale si es una dirección de la sesión —elegir
// entre la de acceso y la del buzón—; antes valía cualquiera, y la invitación salía
// en nombre de otra persona. `organizaba` es quien ya figuraba ("" si el evento es
// nuevo) y se conserva: editar no cambia de organizador.
func organizador(ctx context.Context, pedido, organizaba string) string {
	if organizaba != "" {
		return organizaba
	}
	c, _ := authn.FromContext(ctx)
	if p := cleanAddr(pedido); esDeLaSesion(c, p) {
		return p
	}
	return cmp.Or(c.Email, c.Mailbox)
}

// puedeInvitar dice, ANTES de guardar, si lo que se pide repartir cabe en los topes:
// así la API contesta con un error claro en vez de guardar el evento y callarse las
// invitaciones.
func (s *Service) puedeInvitar(ctx context.Context, in EventInput) error {
	if len(in.Attendees) > MaxInvitados {
		return domain.ErrDemasiadosInvitados
	}
	c, _ := authn.FromContext(ctx)
	fuera := s.deFuera(cleanAddr(in.Organizer), in.Attendees)
	if s.correo != nil && len(fuera) > s.cupoCorreo.libre(c.Subject, time.Now()) {
		return domain.ErrCupoInvitaciones
	}
	return nil
}

// deliverReply entrega un REPLY iTIP al organizador con la respuesta del invitado.
func (s *Service) deliverReply(ctx context.Context, ev domain.Event, replier string) {
	data, err := caldav.EncodeEvent(ev, domain.MethodReply)
	if err != nil {
		log.From(ctx).Warn("calendar: codificar REPLY", "uid", ev.UID, "error", err)
		return
	}
	evID := ev.ID
	if _, err := s.store.CreateInvitation(ctx, store.NewInvitation{
		EventID:   &evID,
		UID:       ev.UID,
		Method:    domain.MethodReply,
		Organizer: replier,
		Recipient: cleanAddr(ev.Organizer),
		Data:      data,
	}); err != nil {
		log.From(ctx).Warn("calendar: entregar REPLY", "organizer", ev.Organizer, "error", err)
	}
}

// --- Conversion -------------------------------------------------------------

// toStoreInput normaliza un EventInput de la API REST a domain.EventInput,
// generando UID si falta, validando rango y recurrencia, sintetizando el
// iCalendar canonico y derivando recurrence_end.
func (s *Service) toStoreInput(in EventInput) (domain.EventInput, error) {
	if in.DTStart.IsZero() || in.DTEnd.IsZero() {
		return domain.EventInput{}, domain.ErrInvalidEvent
	}
	if in.DTEnd.Before(in.DTStart) {
		return domain.EventInput{}, domain.ErrInvalidRange
	}
	uid := in.UID
	if uid == "" {
		uid = uuid.NewString() + "@calendar.sytue.eu"
	}

	si := domain.EventInput{
		CalendarID:     in.CalendarID,
		UID:            uid,
		ResourceName:   uid + ".ics",
		Summary:        in.Summary,
		Description:    in.Description,
		Location:       in.Location,
		Organizer:      cleanAddr(in.Organizer),
		DTStart:        in.DTStart.UTC(),
		DTEnd:          in.DTEnd.UTC(),
		TZID:           orDefault(in.TZID, "UTC"),
		AllDay:         in.AllDay,
		Status:         in.Status,
		Classification: in.Classification,
		Transparency:   in.Transparency,
		Color:          domain.SanitizeColor(in.Color),
		ConferenceURL:  domain.SanitizeConferenceURL(in.ConferenceURL),
		RDate:          in.RDate,
		ExDate:         in.ExDate,
		Attendees:      in.Attendees,
	}
	if in.RRule != "" {
		rule, err := domain.ParseRRule(in.RRule)
		if err != nil {
			return domain.EventInput{}, err
		}
		si.RRule = rule.String()
		// recurrence_end desde el instante en la zona del evento (TZID; UTC si
		// all-day): el último instante UTC de una serie con COUNT depende del DST.
		loc := domain.EventLocation(si.TZID, si.AllDay)
		end := rule.RecurrenceEnd(si.DTStart.In(loc), si.DTEnd.Sub(si.DTStart))
		si.RecurrenceEnd = end
	}

	// Sintetiza el iCalendar canonico para servirlo por CalDAV.
	ev := domain.Event{
		UID: si.UID, Summary: si.Summary, Description: si.Description, Location: si.Location,
		Organizer: si.Organizer, DTStart: si.DTStart, DTEnd: si.DTEnd, TZID: si.TZID,
		AllDay: si.AllDay, Status: orStatus(si.Status), Classification: orClass(si.Classification),
		Transparency: orTransp(si.Transparency), Color: si.Color, ConferenceURL: si.ConferenceURL,
		RRule: si.RRule, Attendees: si.Attendees,
	}
	data, err := caldav.EncodeEvent(ev, "")
	if err != nil {
		return domain.EventInput{}, err
	}
	if in.Reminders != nil {
		if len(*in.Reminders) > maxReminders {
			return domain.EventInput{}, domain.ErrInvalidEvent
		}
		for _, m := range *in.Reminders {
			if m < 0 || m > maxReminderMinutes {
				return domain.EventInput{}, domain.ErrInvalidEvent
			}
		}
		if data, err = caldav.ConRecordatorios(data, *in.Reminders, si.Summary); err != nil {
			return domain.EventInput{}, err
		}
	}
	si.ICalData = data
	return si, nil
}

// PutObject persiste un recurso CalDAV crudo (lo que envia un cliente CalDAV en
// un PUT): decodifica el iCalendar, lo guarda verbatim e indexa el maestro. Usa
// el ETag esperado para condicionales (If-Match). Devuelve el ETag resultante.
func (s *Service) PutObject(ctx context.Context, calendarID uuid.UUID, data string, expectETag uuid.UUID, create bool) (domain.CalendarObject, error) {
	po, err := caldav.DecodeCalendarObject(data, calendarID.String())
	if err != nil {
		return domain.CalendarObject{}, fmt.Errorf("%w: %w", domain.ErrInvalidEvent, err)
	}
	si := po.Master
	si.CalendarID = calendarID
	si.ICalData = data

	if create {
		_, obj, err := s.store.CreateEvent(ctx, si)
		if err != nil {
			return domain.CalendarObject{}, err
		}
		s.publish(ctx, "calendar.event.created", map[string]any{"calendar_id": calendarID, "uid": si.UID})
		return obj, nil
	}
	_, obj, err := s.store.UpdateEvent(ctx, si, expectETag)
	if err != nil {
		return domain.CalendarObject{}, err
	}
	s.publish(ctx, "calendar.event.updated", map[string]any{"calendar_id": calendarID, "uid": si.UID})
	return obj, nil
}

func cleanAddr(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(strings.ToLower(v), "mailto:") {
		return v[len("mailto:"):]
	}
	return v
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orStatus(s domain.EventStatus) domain.EventStatus {
	if s == "" {
		return domain.StatusConfirmed
	}
	return s
}

func orClass(c domain.Classification) domain.Classification {
	if c == "" {
		return domain.ClassPublic
	}
	return c
}

func orTransp(t domain.Transparency) domain.Transparency {
	if t == "" {
		return domain.TranspOpaque
	}
	return t
}

// IsValidationError indica si err es de validacion de entrada (mapea a 4xx).
func IsValidationError(err error) bool {
	return errors.Is(err, domain.ErrInvalidEvent) ||
		errors.Is(err, domain.ErrInvalidRange) ||
		errors.Is(err, domain.ErrInvalidRRULE) ||
		errors.Is(err, domain.ErrInvalidName) ||
		errors.Is(err, domain.ErrInvalidShare) ||
		errors.Is(err, domain.ErrShareSelf) ||
		errors.Is(err, domain.ErrInvalidResource) ||
		errors.Is(err, domain.ErrDemasiadosInvitados)
}
