// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Calendars es el cliente de Sytue Calendar, escrito contra la API REST del servicio
// (services/calendar/internal/http). La web habla CalDAV; una app nativa no necesita
// componer iCalendar para enseñar un mes.
type Calendars struct{ c *Client }

// NewCalendars crea el cliente sobre la raíz del servicio (ServiceURL(apex, "calendar")).
func NewCalendars(servicio string, s Session, h *http.Client) *Calendars {
	return &Calendars{c: New(servicio+"/api/v1", s, h)}
}

// Calendar es un calendario de la persona.
type Calendar struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
	Timezone    string `json:"timezone"`
	IsDefault   bool   `json:"is_default"`
	// ReadOnly: una suscripción o un calendario compartido solo para ver.
	ReadOnly bool `json:"read_only"`
	// Permission y SharedBy solo vienen en «Compartidos conmigo» (SharedWithMe):
	// «reader» o «writer», y quién lo comparte. En un calendario propio van vacíos.
	Permission string `json:"permission,omitempty"`
	SharedBy   string `json:"shared_by,omitempty"`
	// EditRequested: siendo lector, ya ha pedido editarlo (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// Peticiones es la ruta de «pedir acceso de edición» de un calendario compartido.
func (c *Calendars) Peticiones(id string) Peticiones {
	return NuevasPeticiones(c.c, "/calendars/"+url.PathEscape(id)+"/edit-requests")
}

// Los dos permisos de un compartido (domain.SharePermission).
const (
	PermisoLeer   = "reader"
	PermisoEditar = "writer"
)

// Title es el nombre que se enseña.
func (c Calendar) Title() string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.Name
}

// Attendee es un invitado de un evento. Viaja igual al leer que al escribir.
type Attendee struct {
	Email      string `json:"email"`
	CommonName string `json:"common_name,omitempty"`
	Role       string `json:"role,omitempty"`
	PartStat   string `json:"partstat,omitempty"`
	CUType     string `json:"cutype,omitempty"`
	RSVP       bool   `json:"rsvp"`
}

// Event es un evento tal y como lo devuelve el servicio.
type Event struct {
	ID             string      `json:"id"`
	CalendarID     string      `json:"calendar_id"`
	UID            string      `json:"uid"`
	Summary        string      `json:"summary"`
	Description    string      `json:"description,omitempty"`
	Location       string      `json:"location,omitempty"`
	Organizer      string      `json:"organizer,omitempty"`
	DTStart        time.Time   `json:"dtstart"`
	DTEnd          time.Time   `json:"dtend"`
	TZID           string      `json:"tzid"`
	AllDay         bool        `json:"all_day"`
	Status         string      `json:"status"`
	Classification string      `json:"classification,omitempty"`
	Transparency   string      `json:"transp,omitempty"`
	Color          string      `json:"color,omitempty"`
	ConferenceURL  string      `json:"conference_url,omitempty"`
	RRule          string      `json:"rrule,omitempty"`
	RDate          []time.Time `json:"rdate,omitempty"`
	ExDate         []time.Time `json:"exdate,omitempty"`
	// RecurrenceID: este evento es la excepción de UNA ocurrencia de una serie.
	RecurrenceID *time.Time `json:"recurrence_id,omitempty"`
	Attendees    []Attendee `json:"attendees,omitempty"`
	// Reminders son los recordatorios editables, en minutos antes del inicio. Solo los
	// trae GetEvent (leerlos pide el recurso iCalendar): en un listado van a nil, que
	// NO es «ninguno» sino «no se sabe».
	Reminders *[]int `json:"reminders,omitempty"`
}

// Recurring dice si el evento es una serie o una excepción de una serie.
func (e Event) Recurring() bool {
	return e.RRule != "" || len(e.RDate) > 0 || e.RecurrenceID != nil
}

// Input es el evento ENTERO en forma de petición: el punto de partida de toda edición,
// porque el PUT sustituye y lo que no vaya se pierde (la descripción, los invitados, el
// enlace de la videollamada). El UID no viaja: el servicio lo toma del evento guardado.
func (e Event) Input() EventInput {
	return EventInput{
		Summary: e.Summary, Description: e.Description, Location: e.Location, Organizer: e.Organizer,
		Start: e.DTStart, End: e.DTEnd, TZID: e.TZID, AllDay: e.AllDay,
		Status: e.Status, Classification: e.Classification, Transparency: e.Transparency,
		Color: e.Color, ConferenceURL: e.ConferenceURL,
		RRule: e.RRule, RDate: e.RDate, ExDate: e.ExDate, Attendees: e.Attendees,
		Reminders: e.Reminders,
	}
}

// Instance es UNA ocurrencia de un evento dentro de un rango: un evento que se repite
// cada lunes da cuatro o cinco en un mes, todas con el mismo Event.
type Instance struct {
	Event Event     `json:"event"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Day es el día natural en que cae la ocurrencia, en la zona `loc`. Un evento de día
// completo es FLOTANTE —el servicio lo guarda en UTC— y cae el mismo día en cualquier
// zona: convertirlo a la local lo movería a la víspera al oeste de Greenwich.
func (i Instance) Day(loc *time.Location) time.Time {
	t := i.Start.In(loc)
	if i.Event.AllDay {
		t = i.Start.UTC()
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// EventInput es lo que se manda al crear o al sustituir un evento. OJO: el servicio
// RESPONDE `dtstart` y `dtend` pero PIDE `start` y `end`.
type EventInput struct {
	// UID solo cuenta al CREAR: el servicio lo toma de ahí o genera uno. Hace falta darlo
	// cuando algo ya se ligó al evento antes de guardarlo (la sala de videollamada) y al
	// mover un evento de calendario, que es crearlo en el destino con el mismo UID.
	UID            string      `json:"uid,omitempty"`
	Summary        string      `json:"summary"`
	Description    string      `json:"description,omitempty"`
	Location       string      `json:"location,omitempty"`
	Organizer      string      `json:"organizer,omitempty"`
	Start          time.Time   `json:"start"`
	End            time.Time   `json:"end"`
	TZID           string      `json:"tzid,omitempty"`
	AllDay         bool        `json:"all_day"`
	Status         string      `json:"status,omitempty"`
	Classification string      `json:"classification,omitempty"`
	Transparency   string      `json:"transp,omitempty"`
	Color          string      `json:"color,omitempty"`
	ConferenceURL  string      `json:"conference_url,omitempty"`
	RRule          string      `json:"rrule,omitempty"`
	RDate          []time.Time `json:"rdate,omitempty"`
	ExDate         []time.Time `json:"exdate,omitempty"`
	Attendees      []Attendee  `json:"attendees,omitempty"`
	// Send entrega las invitaciones iTIP a los asistentes al guardar. Por CalDAV —que
	// es por donde guarda la web— la entrega va implícita; por REST hay que pedirla.
	Send bool `json:"send,omitempty"`
	// Reminders son los recordatorios en minutos antes del inicio (0, en el momento).
	// nil no viaja y el servicio conserva los que hubiera; una lista —aunque vacía—
	// sustituye a los editables. Los que la web tampoco edita (correo, hora fija) se
	// conservan siempre.
	Reminders *[]int `json:"reminders,omitempty"`
}

// Invitation es un mensaje de la bandeja de planificación (iTIP, RFC 6638): alguien te
// ha invitado a un evento y espera respuesta.
//
// `Summary` y `Start` solo viajan en la de OTRA organización (COM-15), que trae además
// `Organization`: su evento no se lee desde aquí. En las propias los rellena
// Invitations leyendo el evento, igual que el BFF de la web
// (apps/frontend/apps/calendar/src/pages/api/invitations.ts).
type Invitation struct {
	ID          string     `json:"id"`
	EventID     string     `json:"event_id,omitempty"`
	UID         string     `json:"uid"`
	Method      string     `json:"method"`
	Organizer   string     `json:"organizer,omitempty"`
	Recipient   string     `json:"recipient"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`

	Organization string `json:"organization,omitempty"`
	// OrganizationName es su nombre visible, si Sytue ID lo da.
	OrganizationName string    `json:"organization_name,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	Start            time.Time `json:"start,omitzero"`
}

// Las tres respuestas que admite el servicio (domain.InvitationStatus). «pending» es el
// estado de partida, no una respuesta.
const (
	InvitacionAceptada  = "accepted"
	InvitacionQuiza     = "tentative"
	InvitacionRechazada = "declined"
)

// Invitations lista la bandeja de invitaciones. Con `soloPendientes`, solo las que aún
// esperan respuesta, que es lo que enseña la web.
//
// El título y la hora salen del evento: sin ellos una invitación es una dirección de
// correo y una fecha de entrega, y así no se decide si se va o no. Es best-effort —un
// evento que ya no se puede leer deja la fila con lo que sí se sabe—, como el BFF.
//
// ponytail: una petición por invitación, en serie. La bandeja de pendientes son unas
// pocas; si crece, en paralelo con un tope.
func (c *Calendars) Invitations(ctx context.Context, soloPendientes bool) ([]Invitation, error) {
	q := url.Values{}
	if soloPendientes {
		q.Set("pending", "true")
	}
	r, err := JSON[struct {
		Invitations []Invitation `json:"invitations"`
	}](ctx, c.c, Request{Path: "/invitations", Query: q})
	if err != nil {
		return nil, err
	}
	for i, inv := range r.Invitations {
		// El ORGANIZER de iCalendar es una URI: quien invita se llama «ada@sytue.eu», no
		// «mailto:ada@sytue.eu». El servicio lo limpia al crear el evento (cleanAddr),
		// pero lo que entra por CalDAV llega tal cual.
		r.Invitations[i].Organizer = sinMailto(inv.Organizer)
		if inv.EventID == "" || inv.Organization != "" {
			continue
		}
		ev, err := c.GetEvent(ctx, inv.EventID)
		if err != nil {
			continue
		}
		r.Invitations[i].Summary, r.Invitations[i].Start = ev.Summary, ev.DTStart
	}
	return r.Invitations, nil
}

func sinMailto(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= len("mailto:") && strings.EqualFold(v[:len("mailto:")], "mailto:") {
		return v[len("mailto:"):]
	}
	return v
}

// GetEvent trae UN evento por su id.
func (c *Calendars) GetEvent(ctx context.Context, id string) (Event, error) {
	return JSON[Event](ctx, c.c, Request{Path: "/events/" + url.PathEscape(id)})
}

// RespondInvitation contesta una invitación: InvitacionAceptada, InvitacionQuiza o
// InvitacionRechazada. El servicio pone el PARTSTAT del asistente y manda el REPLY a
// quien organiza; solo el DESTINATARIO puede responder (si no, 404).
func (c *Calendars) RespondInvitation(ctx context.Context, id, estado string) (Invitation, error) {
	return PostJSON[Invitation](ctx, c.c, "/invitations/"+url.PathEscape(id)+"/respond", map[string]string{"status": estado})
}

// Reminder es un recordatorio (VALARM) que ya ha vencido y nadie ha descartado: el aviso
// «tienes esto dentro de diez minutos» que el servicio entrega dentro de la aplicación.
type Reminder struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	UID       string    `json:"uid"`
	Summary   string    `json:"summary"`
	DTStart   time.Time `json:"dtstart"`
	TriggerAt time.Time `json:"trigger_at"`
	Action    string    `json:"action"`
}

// Reminders son los recordatorios vencidos, del más reciente al más antiguo.
//
// ponytail: solo la primera página. El servicio pagina por cursor (`next_cursor`), pero
// esto son los avisos de una persona en su día: si alguien llega a cincuenta sin descartar
// ninguno, el problema no se arregla trayendo la página siguiente.
func (c *Calendars) Reminders(ctx context.Context) ([]Reminder, error) {
	r, err := JSON[struct {
		Reminders []Reminder `json:"reminders"`
	}](ctx, c.c, Request{Path: "/reminders/due"})
	return r.Reminders, err
}

// DismissReminder descarta un recordatorio. Es idempotente en el servicio.
func (c *Calendars) DismissReminder(ctx context.Context, id string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodPost, Path: "/reminders/" + url.PathEscape(id) + "/dismiss"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// List lista los calendarios de la persona.
func (c *Calendars) List(ctx context.Context) ([]Calendar, error) {
	r, err := JSON[struct {
		Calendars []Calendar `json:"calendars"`
	}](ctx, c.c, Request{Path: "/calendars"})
	return r.Calendars, err
}

// SearchEvents busca eventos por texto en los calendarios de la persona (`GET
// /events/search`), el `searchEvents` de la web: el servicio resuelve el índice y nunca se
// trae el calendario entero. Lo usa el selector de la suite (Elegir).
// Es la primera página de SearchEventsPage.
func (c *Calendars) SearchEvents(ctx context.Context, q string, limite int) ([]Event, error) {
	b, err := c.SearchEventsPage(ctx, q, limite, "")
	return b.Events, err
}

// SharedWithMe son los calendarios que OTROS han compartido con la persona.
//
// Un «reader» sale como ReadOnly —igual que en el BFF de la web
// (apps/frontend/apps/calendar/src/lib/restCalendar.ts, `toSharedClient`)—: el
// servicio no lo marca así, porque su ReadOnly es solo de las suscripciones ICS, y una
// pantalla que dejara arrastrar un evento ajeno recibiría un 403 al soltarlo.
func (c *Calendars) SharedWithMe(ctx context.Context) ([]Calendar, error) {
	r, err := JSON[struct {
		Calendars []Calendar `json:"calendars"`
	}](ctx, c.c, Request{Path: "/calendars/shared-with-me"})
	for i := range r.Calendars {
		c := &r.Calendars[i]
		c.ReadOnly = c.ReadOnly || c.Permission == PermisoLeer
		// El «por defecto» es del DUEÑO: aquí nunca es el destino por defecto de nadie.
		c.IsDefault = false
	}
	return r.Calendars, err
}

// CalendarInput es el alta o el cambio de un calendario (`createCalendarReq` y
// `updateCalendarReq` del servicio, que comparten forma salvo el nombre interno). En un
// PATCH van solo los campos con valor: el servicio los lee como punteros y un "" borraría
// el nombre que ya tiene.
type CalendarInput struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
}

// CreateCalendar da de alta un calendario propio.
func (c *Calendars) CreateCalendar(ctx context.Context, in CalendarInput) (Calendar, error) {
	return PostJSON[Calendar](ctx, c.c, "/calendars", in)
}

// UpdateCalendar cambia el nombre y el color de uno que ya existe.
func (c *Calendars) UpdateCalendar(ctx context.Context, id string, in CalendarInput) (Calendar, error) {
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return Calendar{}, err
	}
	return JSON[Calendar](ctx, c.c, Request{
		Method: http.MethodPatch, Path: "/calendars/" + url.PathEscape(id),
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
}

// DeleteCalendar borra un calendario CON SUS EVENTOS, o cancela una suscripción.
func (c *Calendars) DeleteCalendar(ctx context.Context, id string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/calendars/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// SubscriptionInput es el «añadir calendario por URL»: un feed ICS externo, que entra
// como un calendario de SOLO LECTURA.
type SubscriptionInput struct {
	URL         string `json:"url"`
	DisplayName string `json:"display_name,omitempty"`
	Color       string `json:"color,omitempty"`
}

// Subscribe se suscribe a un calendario ICS externo.
func (c *Calendars) Subscribe(ctx context.Context, in SubscriptionInput) (Calendar, error) {
	return PostJSON[Calendar](ctx, c.c, "/subscriptions", in)
}

// Instances devuelve las ocurrencias de un calendario en [desde, hasta), con las
// repeticiones ya expandidas por el servicio. La ventana máxima es de un año.
func (c *Calendars) Instances(ctx context.Context, calendario string, desde, hasta time.Time) ([]Instance, error) {
	r, err := JSON[struct {
		Instances []Instance `json:"instances"`
	}](ctx, c.c, Request{Path: "/calendars/" + url.PathEscape(calendario) + "/instances", Query: url.Values{
		"start": {desde.UTC().Format(time.RFC3339)}, "end": {hasta.UTC().Format(time.RFC3339)},
	}})
	return r.Instances, err
}

// CreateEvent crea un evento en un calendario.
func (c *Calendars) CreateEvent(ctx context.Context, calendario string, in EventInput) (Event, error) {
	return PostJSON[Event](ctx, c.c, "/calendars/"+url.PathEscape(calendario)+"/events", in)
}

// UpdateEvent SUSTITUYE un evento: se parte siempre de Event.Input. El calendario no se
// cambia por aquí —el servicio conserva el del evento guardado—, y el evento que vuelve
// trae OTRO id: el servicio borra la fila y la inserta de nuevo.
func (c *Calendars) UpdateEvent(ctx context.Context, id string, in EventInput) (Event, error) {
	cuerpo, err := json.Marshal(in)
	if err != nil {
		return Event{}, err
	}
	return JSON[Event](ctx, c.c, Request{
		Method: http.MethodPut, Path: "/events/" + url.PathEscape(id),
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
}

// DeleteEvent borra un evento; si se repite, la serie entera.
func (c *Calendars) DeleteEvent(ctx context.Context, id string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/events/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Busqueda es una página de resultados de SearchEvents. Siguiente es el cursor de la
// página que viene; vacío, no hay más.
type Busqueda struct {
	Events    []Event `json:"events"`
	Siguiente string  `json:"next_cursor"`
}

// SearchEventsPage busca en el servidor por texto (título, descripción, lugar e
// invitados), de `limite` en `limite` y a partir del cursor de la página anterior. Es el
// buscador de eventos de la web (SearchPanel.tsx): una serie sale UNA vez, con su RRULE.
func (c *Calendars) SearchEventsPage(ctx context.Context, texto string, limite int, cursor string) (Busqueda, error) {
	q := url.Values{"q": {texto}}
	if limite > 0 {
		q.Set("limit", strconv.Itoa(limite))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return JSON[Busqueda](ctx, c.c, Request{Path: "/events/search", Query: q})
}

// Hueco es un tramo libre que devuelve SuggestSlots.
type Hueco struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SuggestSlots es «buscar hueco» (FindTimePanel.tsx): los tramos libres de la persona
// en [desde, hasta) donde cabe un evento de `minutos`, dentro de la franja laboral
// «HH:MM»–«HH:MM» de la zona `zona` y, si `laborables`, solo de lunes a viernes. El
// servicio incluye SIEMPRE al organizador y calcula él los cruces y el horario de verano.
func (c *Calendars) SuggestSlots(ctx context.Context, desde, hasta time.Time, minutos int, zona, franjaDesde, franjaHasta string, laborables bool) ([]Hueco, error) {
	q := url.Values{
		"start": {desde.UTC().Format(time.RFC3339)}, "end": {hasta.UTC().Format(time.RFC3339)},
		"duration": {strconv.Itoa(minutos)}, "work_start": {franjaDesde}, "work_end": {franjaHasta},
	}
	if zona != "" {
		q.Set("tz", zona)
	}
	if laborables {
		q.Set("weekdays", "true")
	}
	r, err := JSON[struct {
		Slots []Hueco `json:"slots"`
	}](ctx, c.c, Request{Path: "/freebusy/suggest", Query: q})
	return r.Slots, err
}

// Los PARTSTAT que una persona puede dar a una invitación (RFC 5545).
const (
	AsistenciaSi    = "ACCEPTED"
	AsistenciaQuiza = "TENTATIVE"
	AsistenciaNo    = "DECLINED"
)

// SetPartStat contesta DESDE EL EVENTO a una invitación: el PARTSTAT del asistente que
// es la persona (el servicio lo casa con el correo del token). Es el «¿Vas a ir?» de la
// vista rápida de la web.
func (c *Calendars) SetPartStat(ctx context.Context, evento, partstat string) error {
	cuerpo, err := json.Marshal(map[string]string{"partstat": partstat})
	if err != nil {
		return err
	}
	resp, err := c.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/events/" + url.PathEscape(evento) + "/attendees/partstat",
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Los dos recortes de una serie: quitar UNA ocurrencia (EXDATE) o cortarla desde ella
// (UNTIL). Son «solo este» y «este y los siguientes» del diálogo de alcance.
const (
	RecorteExcluir = "exclude"
	RecorteHasta   = "until"
)

// AmendRecurrence recorta el maestro de una serie para editar o borrar una ocurrencia
// sin corromper la serie. `ocurrencia` es el inicio ORIGINAL de la ocurrencia (su
// RECURRENCE-ID).
func (c *Calendars) AmendRecurrence(ctx context.Context, calendario, uid string, ocurrencia time.Time, recorte string) error {
	cuerpo, err := json.Marshal(map[string]any{"uid": uid, "recurrence_id": ocurrencia.UTC(), "mode": recorte})
	if err != nil {
		return err
	}
	resp, err := c.c.Do(ctx, Request{
		Method: http.MethodPost, Path: "/calendars/" + url.PathEscape(calendario) + "/amend-recurrence",
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// RefreshSubscription vuelve a leer ahora el .ics de una suscripción.
func (c *Calendars) RefreshSubscription(ctx context.Context, calendario string) error {
	resp, err := c.c.Do(ctx, Request{Method: http.MethodPost, Path: "/calendars/" + url.PathEscape(calendario) + "/refresh"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// ExportICS descarga el calendario entero como .ics: «Descargar .ics» del menú de un
// calendario. Vale también para una suscripción: es SU copia de esos eventos.
func (c *Calendars) ExportICS(ctx context.Context, calendario string) ([]byte, error) {
	resp, err := c.c.Do(ctx, Request{Path: "/calendars/" + url.PathEscape(calendario) + "/export.ics"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// CalendarShare es a quién se le ha compartido un calendario propio y con qué permiso:
// «reader», «writer» o «manage» (llevar la agenda).
type CalendarShare struct {
	ID         string `json:"id"`
	Correo     string `json:"grantee_email"`
	Permission string `json:"permission"`
}

// PermisoLlevar es el tercer permiso de compartir: todo lo de editar y además organizar
// y contestar en nombre del dueño.
const PermisoLlevar = "manage"

// Shares lista con quién está compartido un calendario (solo su dueño lo ve).
func (c *Calendars) Shares(ctx context.Context, calendario string) ([]CalendarShare, error) {
	r, err := JSON[struct {
		Shares []CalendarShare `json:"shares"`
	}](ctx, c.c, Request{Path: "/calendars/" + url.PathEscape(calendario) + "/shares"})
	return r.Shares, err
}

// Share comparte un calendario propio con un correo.
func (c *Calendars) Share(ctx context.Context, calendario, correo, permiso string) error {
	_, err := PostJSON[json.RawMessage](ctx, c.c, "/calendars/"+url.PathEscape(calendario)+"/shares",
		map[string]string{"email": correo, "permission": permiso})
	return err
}

// RevokeShare deja de compartirlo con alguien.
func (c *Calendars) RevokeShare(ctx context.Context, calendario, compartido string) error {
	resp, err := c.c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   "/calendars/" + url.PathEscape(calendario) + "/shares/" + url.PathEscape(compartido),
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
