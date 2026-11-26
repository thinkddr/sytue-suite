// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google Calendar y People usan estas claves camelCase fijas.
package agenda

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
)

// Google Calendar (API v3): `syncToken` para la bajada incremental, `If-Match` con la
// ETag del evento al cambiarlo y `sendUpdates=all` para que Google mande él las
// invitaciones y sus cambios (A-11). Cada objeto de aquí es un evento con su `iCalUID`:
// el maestro y sus excepciones, que Google guarda como eventos aparte
// (`recurringEventId`); las excepciones canceladas son EXDATE del maestro.

// GoogleCalendar es [Remoto] sobre Google Calendar.
type GoogleCalendar struct {
	api  apiJSON
	Base string // «https://www.googleapis.com/calendar/v3»

	mu       sync.Mutex
	primario string // el id del calendario principal: el correo de la persona
}

type horaGoogle struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

func (h horaGoogle) momento() momento {
	if h.Date != "" {
		t, _ := time.Parse(time.DateOnly, h.Date)
		return momento{Dia: true, T: t}
	}
	t, _ := time.Parse(time.RFC3339Nano, h.DateTime)
	return momento{T: t, Zona: h.TimeZone}
}

func horaDe(m momento) horaGoogle {
	if m.Dia {
		return horaGoogle{Date: m.T.Format(time.DateOnly)}
	}
	h := horaGoogle{DateTime: m.T.UTC().Format(time.RFC3339), TimeZone: "UTC"}
	if m.Zona != "" {
		if loc, err := time.LoadLocation(m.Zona); err == nil {
			h = horaGoogle{DateTime: m.T.In(loc).Format(time.RFC3339), TimeZone: m.Zona}
		}
	}
	return h
}

type personaGoogle struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
}

type eventoGoogle struct {
	ID               string          `json:"id,omitempty"`
	ETag             string          `json:"etag,omitempty"`
	Status           string          `json:"status,omitempty"`
	ICalUID          string          `json:"iCalUID,omitempty"`
	Summary          string          `json:"summary"`
	Description      string          `json:"description"`
	Location         string          `json:"location"`
	Start            *horaGoogle     `json:"start,omitempty"`
	End              *horaGoogle     `json:"end,omitempty"`
	Recurrence       []string        `json:"recurrence,omitempty"`
	RecurringEventID string          `json:"recurringEventId,omitempty"`
	OriginalStart    *horaGoogle     `json:"originalStartTime,omitempty"`
	Attendees        []personaGoogle `json:"attendees,omitempty"`
	Organizer        *personaGoogle  `json:"organizer,omitempty"`
	Transparency     string          `json:"transparency,omitempty"`
	Sequence         int             `json:"sequence,omitempty"`
	HangoutLink      string          `json:"hangoutLink,omitempty"`
	Updated          string          `json:"updated,omitempty"`
	// ConferenceData solo viaja al subir un evento que pide su sala de Meet (reuniones.go).
	ConferenceData *conferenciaGoogle `json:"conferenceData,omitempty"`
}

var estadosGoogle = map[string]string{"needsAction": "NEEDS-ACTION", "accepted": "ACCEPTED", "declined": "DECLINED", "tentative": "TENTATIVE"}

func estadoGoogle(partstat string) string {
	for k, v := range estadosGoogle {
		if v == partstat {
			return k
		}
	}
	return "needsAction"
}

// Colecciones son los calendarios de la lista de la persona.
func (g *GoogleCalendar) Colecciones(ctx context.Context) ([]Coleccion, error) {
	var out []Coleccion
	pagina := ""
	for range 50 {
		var r struct {
			Items []struct {
				ID              string `json:"id"`
				Summary         string `json:"summary"`
				SummaryOverride string `json:"summaryOverride"`
				BackgroundColor string `json:"backgroundColor"`
				AccessRole      string `json:"accessRole"`
				Deleted         bool   `json:"deleted"`
				Primary         bool   `json:"primary"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		u := g.Base + "/users/me/calendarList?" + url.Values{"pageToken": {pagina}, "maxResults": {"250"}}.Encode()
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, nil, &r); err != nil {
			return nil, err
		}
		for _, c := range r.Items {
			if c.Primary && strings.Contains(c.ID, "@") {
				g.mu.Lock()
				g.primario = strings.ToLower(c.ID)
				g.mu.Unlock()
			}
			if c.Deleted || c.AccessRole == "freeBusyReader" {
				continue
			}
			nombre := c.SummaryOverride
			if nombre == "" {
				nombre = c.Summary
			}
			out = append(out, Coleccion{
				ID: c.ID, Nombre: nombre, Color: strings.ToUpper(c.BackgroundColor),
				SoloLectura: c.AccessRole != "owner" && c.AccessRole != "writer", Planifica: true,
			})
		}
		if pagina = r.NextPageToken; pagina == "" {
			break
		}
	}
	return out, nil
}

func (g *GoogleCalendar) eventos(col Coleccion) string {
	return g.Base + "/calendars/" + url.PathEscape(col.ID) + "/events"
}

// Cambios baja con syncToken (la primera vez, desde la ventana).
func (g *GoogleCalendar) Cambios(ctx context.Context, col Coleccion, ficha string, _ func(string) (string, bool), desde time.Time) (Cambios, error) {
	var ca Cambios
	q := url.Values{"showDeleted": {"true"}, "maxResults": {"250"}, "singleEvents": {"false"}}
	if ficha != "" {
		q.Set("syncToken", ficha)
	} else if !desde.IsZero() {
		q.Set("timeMin", desde.UTC().Format(time.RFC3339))
	}
	var items []eventoGoogle
	for range 1000 {
		var r struct {
			Items         []eventoGoogle `json:"items"`
			NextPageToken string         `json:"nextPageToken"`
			NextSyncToken string         `json:"nextSyncToken"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, g.eventos(col)+"?"+q.Encode(), nil, nil, &r); err != nil {
			return ca, err
		}
		items = append(items, r.Items...)
		if r.NextPageToken == "" {
			ca.Ficha = r.NextSyncToken
			break
		}
		q.Set("pageToken", r.NextPageToken)
	}
	// Por maestro: sus datos y las excepciones que llegaron.
	maestros := map[string]*eventoGoogle{}
	excepciones := map[string][]eventoGoogle{}
	tocados := map[string]bool{}
	for i := range items {
		ev := items[i]
		if ev.RecurringEventID != "" {
			excepciones[ev.RecurringEventID] = append(excepciones[ev.RecurringEventID], ev)
			tocados[ev.RecurringEventID] = true
			continue
		}
		maestros[ev.ID] = &items[i]
		tocados[ev.ID] = true
	}
	ca.Completo = ficha == ""
	for _, id := range ordenarIDs(tocados) {
		m := maestros[id]
		if m != nil && m.Status == "cancelled" {
			ca.Borrados = append(ca.Borrados, id)
			continue
		}
		var obj Objeto
		var err error
		if m != nil && (ficha == "" || (len(m.Recurrence) == 0 && len(excepciones[id]) == 0)) {
			obj, err = objetoGoogle(*m, excepciones[id])
		} else {
			// Cambió una excepción (o una serie): se trae la serie entera.
			obj, err = g.Leer(ctx, col, id)
			if errors.Is(err, ErrNoEsta) {
				ca.Borrados = append(ca.Borrados, id)
				continue
			}
		}
		if err != nil {
			return ca, err
		}
		ca.Puestos = append(ca.Puestos, obj)
		ca.Vistos = append(ca.Vistos, id)
	}
	return ca, nil
}

// Leer trae un evento con sus excepciones.
func (g *GoogleCalendar) Leer(ctx context.Context, col Coleccion, id string) (Objeto, error) {
	var m eventoGoogle
	if err := g.api.pedir(ctx, http.MethodGet, g.eventos(col)+"/"+url.PathEscape(id), nil, nil, &m); err != nil {
		return Objeto{}, err
	}
	if m.Status == "cancelled" {
		return Objeto{}, ErrNoEsta
	}
	var exc []eventoGoogle
	if len(m.Recurrence) > 0 && m.ICalUID != "" {
		q := url.Values{"iCalUID": {m.ICalUID}, "showDeleted": {"true"}, "maxResults": {"2500"}}
		var r struct {
			Items []eventoGoogle `json:"items"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, g.eventos(col)+"?"+q.Encode(), nil, nil, &r); err != nil {
			return Objeto{}, err
		}
		for _, e := range r.Items {
			if e.RecurringEventID == m.ID {
				exc = append(exc, e)
			}
		}
	}
	return objetoGoogle(m, exc)
}

// objetoGoogle escribe el VCALENDAR de un maestro y sus excepciones.
func objetoGoogle(m eventoGoogle, exc []eventoGoogle) (Objeto, error) {
	uid := m.ICalUID
	if uid == "" {
		uid = m.ID + "@google.com"
	}
	var l lineas
	abrirVCALENDAR(&l)
	var exdates []string
	for _, e := range exc {
		if e.Status == "cancelled" && e.OriginalStart != nil {
			p, v := e.OriginalStart.momento().ics()
			exdates = append(exdates, propLinea(ical.Prop{Name: ical.PropExceptionDates, Params: paramsDe(p), Value: v}))
		}
	}
	veventGoogle(&l, m, uid, nil, exdates)
	for _, e := range exc {
		if e.Status != "cancelled" && e.OriginalStart != nil {
			o := e.OriginalStart.momento()
			veventGoogle(&l, e, uid, &o, nil)
		}
	}
	l.poner("END", "", "VCALENDAR")
	return Objeto{ID: m.ID, ETag: m.ETag, Datos: []byte(l.b.String())}, nil
}

func paramsDe(p string) ical.Params {
	ps := ical.Params{}
	if k, v, ok := strings.Cut(p, "="); ok {
		ps.Set(k, v)
	}
	return ps
}

func veventGoogle(l *lineas, e eventoGoogle, uid string, recurrencia *momento, extra []string) {
	l.poner("BEGIN", "", "VEVENT")
	l.poner("UID", "", uid)
	sello := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339Nano, e.Updated); err == nil {
		sello = t.UTC()
	}
	l.poner("DTSTAMP", "", sello.Format("20060102T150405Z"))
	if recurrencia != nil {
		p, v := recurrencia.ics()
		l.poner("RECURRENCE-ID", p, v)
	}
	if e.Start != nil {
		p, v := e.Start.momento().ics()
		l.poner("DTSTART", p, v)
	}
	if e.End != nil {
		p, v := e.End.momento().ics()
		l.poner("DTEND", p, v)
	}
	l.texto("SUMMARY", e.Summary)
	l.texto("DESCRIPTION", e.Description)
	l.texto("LOCATION", e.Location)
	for _, r := range append(e.Recurrence, extra...) {
		l.b.WriteString(r + "\r\n")
	}
	if e.Organizer != nil {
		l.organizador(e.Organizer.Email, e.Organizer.DisplayName)
	}
	as := make([]asistente, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		as = append(as, asistente{Correo: a.Email, Nombre: a.DisplayName, Estado: estadosGoogle[a.ResponseStatus]})
	}
	l.asistentes(as)
	if e.Transparency == "transparent" {
		l.poner("TRANSP", "", "TRANSPARENT")
	}
	if e.Status == "tentative" {
		l.poner("STATUS", "", "TENTATIVE")
	}
	if e.Sequence > 0 {
		l.poner("SEQUENCE", "", strconv.Itoa(e.Sequence))
	}
	if e.HangoutLink != "" {
		l.poner("CONFERENCE", "VALUE=URI", e.HangoutLink)
	}
	l.poner("X-GOOGLE-ID", "", textoICS(e.ID))
	l.poner("END", "", "VEVENT")
}

// eventoDeICS es el evento de Google de un VEVENT de aquí (lo que se sube).
func eventoDeICS(ev ical.Event) (eventoGoogle, error) {
	ini, err := momentoDe(ev, ical.PropDateTimeStart)
	if err != nil {
		return eventoGoogle{}, err
	}
	fin, err := momentoDe(ev, ical.PropDateTimeEnd)
	if err != nil {
		return eventoGoogle{}, err
	}
	hi, hf := horaDe(ini), horaDe(fin)
	g := eventoGoogle{
		Summary: textoDe(ev, ical.PropSummary), Description: textoDe(ev, ical.PropDescription), Location: textoDe(ev, ical.PropLocation),
		Start: &hi, End: &hf, Recurrence: recurrenciaDe(ev), Transparency: "opaque",
	}
	if strings.EqualFold(textoDe(ev, ical.PropTransparency), "TRANSPARENT") {
		g.Transparency = "transparent"
	}
	for _, a := range asistentesDe(ev) {
		g.Attendees = append(g.Attendees, personaGoogle{Email: a.Correo, DisplayName: a.Nombre, ResponseStatus: estadoGoogle(a.Estado)})
	}
	if pideReunion(ev, ReunionMeet) {
		g.ConferenceData = peticionMeet(textoDe(ev, ical.PropUID))
	}
	return g, nil
}

// conSala es la consulta de una subida: con `conferenceDataVersion=1` si pide la sala de
// Meet (sin él, Google no mira `conferenceData`).
func conSala(ev eventoGoogle) string {
	if ev.ConferenceData != nil {
		return "?sendUpdates=all&conferenceDataVersion=1"
	}
	return "?sendUpdates=all"
}

// conLoDeGoogle es lo que se devuelve tras subir: si se pidió la sala, el evento como lo
// tiene ya Google (con su enlace en CONFERENCE), que el sincronizador guarda aquí. Si no se
// puede leer, solo el id y la ETag: el enlace llegará con la próxima bajada.
func (g *GoogleCalendar) conLoDeGoogle(ctx context.Context, col Coleccion, ev, r eventoGoogle) Objeto {
	if ev.ConferenceData != nil {
		if o, err := g.Leer(ctx, col, r.ID); err == nil {
			return o
		}
	}
	return Objeto{ID: r.ID, ETag: r.ETag}
}

// Crear sube un evento nuevo (con su UID) y avisa a los invitados.
func (g *GoogleCalendar) Crear(ctx context.Context, col Coleccion, uid string, datos []byte) (Objeto, error) {
	m, exc, err := eventosDe(datos)
	if err != nil {
		return Objeto{}, err
	}
	ev, err := eventoDeICS(*m)
	if err != nil {
		return Objeto{}, err
	}
	ev.ICalUID = uid
	if ev.ConferenceData != nil {
		ev.ConferenceData = peticionMeet(uid)
	}
	var r eventoGoogle
	if err := g.api.pedir(ctx, http.MethodPost, g.eventos(col)+conSala(ev), ev, nil, &r); err != nil {
		return Objeto{}, err
	}
	g.excepciones(ctx, col, r.ID, exc)
	return g.conLoDeGoogle(ctx, col, ev, r), nil
}

// Cambiar cambia los campos de aquí del evento (PATCH: lo que Google tiene y aquí no se
// ve —recordatorios, adjuntos, la sala de Meet— se queda) si sigue en su ETag.
func (g *GoogleCalendar) Cambiar(ctx context.Context, col Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	m, exc, err := eventosDe(datos)
	if err != nil {
		return Objeto{}, err
	}
	ev, err := eventoDeICS(*m)
	if err != nil {
		return Objeto{}, err
	}
	if ev.Recurrence == nil {
		ev.Recurrence = []string{} // quitarla si ya no la hay
	}
	cab := map[string]string{}
	if obj.ETag != "" {
		cab["If-Match"] = obj.ETag
	}
	var r eventoGoogle
	if err := g.api.pedir(ctx, http.MethodPatch, g.eventos(col)+"/"+url.PathEscape(obj.ID)+conSala(ev), ev, cab, &r); err != nil {
		return Objeto{}, err
	}
	g.excepciones(ctx, col, r.ID, exc)
	return g.conLoDeGoogle(ctx, col, ev, r), nil
}

// excepciones sube cada ocurrencia cambiada a su instancia de Google. Lo que falle se
// verá en la próxima bajada (la serie se vuelve a leer entera).
func (g *GoogleCalendar) excepciones(ctx context.Context, col Coleccion, maestro string, exc []ical.Event) {
	for _, e := range exc {
		id := textoDe(e, "X-GOOGLE-ID")
		if id == "" {
			o, err := momentoDe(e, ical.PropRecurrenceID)
			if err != nil {
				continue
			}
			if o.Dia {
				id = maestro + "_" + o.T.Format("20060102")
			} else {
				id = maestro + "_" + o.T.UTC().Format("20060102T150405Z")
			}
		}
		ev, err := eventoDeICS(e)
		if err != nil {
			continue
		}
		ev.Recurrence = nil
		_ = g.api.pedir(ctx, http.MethodPatch, g.eventos(col)+"/"+url.PathEscape(id)+"?sendUpdates=all", ev, nil, nil)
	}
}

// CrearColeccion crea un calendario en la cuenta de Google y le pone el color en la
// lista de la persona (si eso falla, el calendario queda con el de Google).
func (g *GoogleCalendar) CrearColeccion(ctx context.Context, nombre, color string) (Coleccion, error) {
	var r struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/calendars", map[string]string{"summary": strings.TrimSpace(nombre)}, nil, &r); err != nil {
		return Coleccion{}, err
	}
	if color != "" {
		_ = g.api.pedir(ctx, http.MethodPatch, g.Base+"/users/me/calendarList/"+url.PathEscape(r.ID)+"?colorRgbFormat=true",
			map[string]string{"backgroundColor": strings.ToLower(color), "foregroundColor": "#000000"}, nil, nil)
	}
	return Coleccion{ID: r.ID, Nombre: r.Summary, Color: color, Planifica: true}, nil
}

// Borrar quita el evento (y avisa a los invitados) si sigue en su ETag.
func (g *GoogleCalendar) Borrar(ctx context.Context, col Coleccion, id, etag string) error {
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = etag
	}
	err := g.api.pedir(ctx, http.MethodDelete, g.eventos(col)+"/"+url.PathEscape(id)+"?sendUpdates=all", nil, cab, nil)
	if errors.Is(err, ErrNoEsta) || errors.Is(err, ErrFichaCaducada) { // 404 o 410: ya no estaba
		return nil
	}
	return err
}

// ----- Google Contacts (People API) -----

// GooglePeople es [Remoto] sobre los contactos de Google: una libreta, «Contactos»,
// con `syncToken`. El UID de aquí se guarda en el contacto (`clientData`), así el de
// Google y el de aquí siguen siendo el mismo.
type GooglePeople struct {
	api  apiJSON
	Base string // «https://people.googleapis.com/v1»

	mu  sync.Mutex
	enl Enlaces // los de la libreta en la vuelta en curso (etiquetas.go)
}

const camposPeople = "names,emailAddresses,phoneNumbers,organizations,addresses,birthdays,biographies,urls,nicknames,clientData,metadata"

type valorGoogle struct {
	Value string `json:"value,omitempty"`
	Type  string `json:"type,omitempty"`
}

type personaPeople struct {
	ResourceName string `json:"resourceName,omitempty"`
	ETag         string `json:"etag,omitempty"`
	Metadata     *struct {
		Deleted bool `json:"deleted"`
	} `json:"metadata,omitempty"`
	Names          []nombrePeople `json:"names,omitempty"`
	EmailAddresses []valorGoogle  `json:"emailAddresses,omitempty"`
	PhoneNumbers   []valorGoogle  `json:"phoneNumbers,omitempty"`
	URLs           []valorGoogle  `json:"urls,omitempty"`
	Nicknames      []valorGoogle  `json:"nicknames,omitempty"`
	Biographies    []valorGoogle  `json:"biographies,omitempty"`
	Organizations  []orgPeople    `json:"organizations,omitempty"`
	Addresses      []dirPeople    `json:"addresses,omitempty"`
	Birthdays      []cumplePeople `json:"birthdays,omitempty"`
	ClientData     []datoCliente  `json:"clientData,omitempty"`
}

type nombrePeople struct {
	DisplayName     string `json:"displayName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

type orgPeople struct {
	Name       string `json:"name,omitempty"`
	Department string `json:"department,omitempty"`
	Title      string `json:"title,omitempty"`
}

type dirPeople struct {
	StreetAddress string `json:"streetAddress,omitempty"`
	City          string `json:"city,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postalCode,omitempty"`
	Country       string `json:"country,omitempty"`
	Type          string `json:"type,omitempty"`
}

type cumplePeople struct {
	Date struct {
		Year  int `json:"year,omitempty"`
		Month int `json:"month,omitempty"`
		Day   int `json:"day,omitempty"`
	} `json:"date"`
}

type datoCliente struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Colecciones: Google tiene una sola libreta de contactos.
func (g *GooglePeople) Colecciones(context.Context) ([]Coleccion, error) {
	return []Coleccion{{ID: "people/me", Nombre: "Google"}}, nil
}

// Cambios baja con syncToken.
func (g *GooglePeople) Cambios(ctx context.Context, _ Coleccion, ficha string, etagDe func(string) (string, bool), _ time.Time) (Cambios, error) {
	var ca Cambios
	vistos := map[string]string{} // resourceName → UID de los contactos que bajan
	q := url.Values{"personFields": {camposPeople}, "pageSize": {"1000"}, "requestSyncToken": {"true"}}
	if ficha != "" {
		q.Set("syncToken", ficha)
	}
	for range 1000 {
		var r struct {
			Connections   []personaPeople `json:"connections"`
			NextPageToken string          `json:"nextPageToken"`
			NextSyncToken string          `json:"nextSyncToken"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/people/me/connections?"+q.Encode(), nil, nil, &r); err != nil {
			return ca, err
		}
		for _, p := range r.Connections {
			if p.Metadata != nil && p.Metadata.Deleted {
				ca.Borrados = append(ca.Borrados, p.ResourceName)
				continue
			}
			ca.Puestos = append(ca.Puestos, Objeto{ID: p.ResourceName, ETag: p.ETag, Datos: []byte(vcardDePersona(p))})
			ca.Vistos = append(ca.Vistos, p.ResourceName)
			vistos[p.ResourceName] = uidDePersona(p)
		}
		if r.NextPageToken == "" {
			ca.Ficha = r.NextSyncToken
			break
		}
		q.Set("pageToken", r.NextPageToken)
	}
	ca.Completo = ficha == ""
	// Las etiquetas, detrás de los contactos: sus miembros ya están aquí. (En dos
	// pasos: en un mismo return, Go no garantiza leer ca después de rellenarla.)
	err := g.cambiosGrupos(ctx, &ca, etagDe, vistos)
	return ca, err
}

// uidDePersona es el UID de aquí de un contacto de Google: el de clientData o, si no lo
// lleva (se creó allí), uno hecho con su resourceName.
func uidDePersona(p personaPeople) string {
	uid := "google-" + strings.TrimPrefix(p.ResourceName, "people/")
	for _, c := range p.ClientData {
		if c.Key == "sytue-uid" && c.Value != "" {
			uid = c.Value
		}
	}
	return uid
}

func vcardDePersona(p personaPeople) string {
	var l lineas
	l.poner("BEGIN", "", "VCARD")
	l.poner("VERSION", "", "3.0")
	l.poner("UID", "", textoICS(uidDePersona(p)))
	fn := ""
	if len(p.Names) > 0 {
		n := p.Names[0]
		fn = n.DisplayName
		if fn == "" {
			fn = strings.TrimSpace(n.GivenName + " " + n.FamilyName)
		}
		l.poner("N", "", strings.Join([]string{textoICS(n.FamilyName), textoICS(n.GivenName), textoICS(n.MiddleName), textoICS(n.HonorificPrefix), textoICS(n.HonorificSuffix)}, ";"))
	}
	if fn == "" && len(p.EmailAddresses) > 0 {
		fn = p.EmailAddresses[0].Value
	}
	l.poner("FN", "", textoICS(fn))
	for _, n := range p.Nicknames {
		l.texto("NICKNAME", n.Value)
	}
	for _, e := range p.EmailAddresses {
		l.poner("EMAIL", tipoVCard(e.Type), textoICS(e.Value))
	}
	for _, t := range p.PhoneNumbers {
		l.poner("TEL", tipoVCard(t.Type), textoICS(t.Value))
	}
	for _, o := range p.Organizations {
		if o.Name != "" || o.Department != "" {
			l.poner("ORG", "", textoICS(o.Name)+";"+textoICS(o.Department))
		}
		l.texto("TITLE", o.Title)
	}
	for _, a := range p.Addresses {
		l.poner("ADR", tipoVCard(a.Type), strings.Join([]string{"", "", textoICS(a.StreetAddress), textoICS(a.City), textoICS(a.Region), textoICS(a.PostalCode), textoICS(a.Country)}, ";"))
	}
	for _, b := range p.Birthdays {
		d := b.Date
		if d.Month == 0 || d.Day == 0 {
			continue
		}
		if d.Year > 0 {
			l.poner("BDAY", "", time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly))
		} else {
			l.poner("BDAY", "", "--"+strconv.Itoa(100 + d.Month)[1:]+"-"+strconv.Itoa(100 + d.Day)[1:])
		}
		break
	}
	for _, b := range p.Biographies {
		l.texto("NOTE", b.Value)
	}
	for _, u := range p.URLs {
		l.poner("URL", "", u.Value)
	}
	l.poner("END", "", "VCARD")
	return l.b.String()
}

func tipoVCard(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "":
		return ""
	case "mobile":
		t = "cell"
	}
	if strings.ContainsAny(t, ";:,\"") {
		return ""
	}
	return "TYPE=" + t
}

func tipoDeVCard(f *vcard.Field) string {
	t := strings.ToLower(f.Params.Get(vcard.ParamType))
	if t == "cell" {
		return "mobile"
	}
	return t
}

// personaDeVCard es el contacto de Google de una vCard de aquí.
func personaDeVCard(datos []byte, uid string) (personaPeople, error) {
	card, err := vcard.NewDecoder(strings.NewReader(string(datos))).Decode()
	if err != nil {
		return personaPeople{}, err
	}
	var p personaPeople
	if n := card.Name(); n != nil {
		p.Names = append(p.Names, nombrePeople{GivenName: n.GivenName, FamilyName: n.FamilyName, MiddleName: n.AdditionalName, HonorificPrefix: n.HonorificPrefix, HonorificSuffix: n.HonorificSuffix})
	}
	for _, f := range card[vcard.FieldEmail] {
		p.EmailAddresses = append(p.EmailAddresses, valorGoogle{Value: f.Value, Type: tipoDeVCard(f)})
	}
	for _, f := range card[vcard.FieldTelephone] {
		p.PhoneNumbers = append(p.PhoneNumbers, valorGoogle{Value: strings.TrimPrefix(f.Value, "tel:"), Type: tipoDeVCard(f)})
	}
	for _, f := range card[vcard.FieldURL] {
		p.URLs = append(p.URLs, valorGoogle{Value: f.Value})
	}
	for _, f := range card[vcard.FieldNickname] {
		p.Nicknames = append(p.Nicknames, valorGoogle{Value: f.Value})
	}
	if v := card.Value(vcard.FieldNote); v != "" {
		p.Biographies = append(p.Biographies, valorGoogle{Value: v, Type: ""})
	}
	org, titulo := card.Value(vcard.FieldOrganization), card.Value(vcard.FieldTitle)
	if org != "" || titulo != "" {
		nombre, depto, _ := strings.Cut(org, ";")
		p.Organizations = append(p.Organizations, orgPeople{Name: nombre, Department: depto, Title: titulo})
	}
	for _, a := range card.Addresses() {
		p.Addresses = append(p.Addresses, dirPeople{StreetAddress: a.StreetAddress, City: a.Locality, Region: a.Region, PostalCode: a.PostalCode, Country: a.Country, Type: strings.ToLower(a.Params.Get(vcard.ParamType))})
	}
	if b := card.Value(vcard.FieldBirthday); b != "" {
		var y, m, d int
		if t, err := time.Parse(time.DateOnly, b); err == nil {
			y, m, d = t.Year(), int(t.Month()), t.Day()
		} else if t, err := time.Parse("20060102", b); err == nil {
			y, m, d = t.Year(), int(t.Month()), t.Day()
		} else if strings.HasPrefix(b, "--") && len(b) >= 6 {
			m, _ = strconv.Atoi(strings.ReplaceAll(b[2:4], "-", ""))
			d, _ = strconv.Atoi(b[len(b)-2:])
		}
		if m > 0 && d > 0 {
			var bd cumplePeople
			bd.Date.Year, bd.Date.Month, bd.Date.Day = y, m, d
			p.Birthdays = append(p.Birthdays, bd)
		}
	}
	p.ClientData = append(p.ClientData, datoCliente{Key: "sytue-uid", Value: uid})
	return p, nil
}

// Leer trae un contacto o una etiqueta.
func (g *GooglePeople) Leer(ctx context.Context, _ Coleccion, id string) (Objeto, error) {
	if strings.HasPrefix(id, prefijoGrupoGoogle) {
		return g.leerGrupoGoogle(ctx, id)
	}
	var p personaPeople
	if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/"+id+"?personFields="+camposPeople, nil, nil, &p); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: p.ResourceName, ETag: p.ETag, Datos: []byte(vcardDePersona(p))}, nil
}

// Crear sube un contacto nuevo, o una etiqueta como grupo de contactos.
func (g *GooglePeople) Crear(ctx context.Context, _ Coleccion, uid string, datos []byte) (Objeto, error) {
	if esGrupoVCard(string(datos)) {
		return g.crearGrupo(ctx, datos)
	}
	p, err := personaDeVCard(datos, uid)
	if err != nil {
		return Objeto{}, err
	}
	var r personaPeople
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/people:createContact?personFields=metadata", p, nil, &r); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: r.ResourceName, ETag: r.ETag}, nil
}

// Cambiar reemplaza los campos de aquí del contacto si sigue en su ETag (Google la
// lleva en el cuerpo y contesta FAILED_PRECONDITION si cambió).
func (g *GooglePeople) Cambiar(ctx context.Context, _ Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	if strings.HasPrefix(obj.ID, prefijoGrupoGoogle) {
		return g.cambiarGrupo(ctx, obj, datos)
	}
	p, err := personaDeVCard(datos, uidDe(string(datos)))
	if err != nil {
		return Objeto{}, err
	}
	p.ETag = obj.ETag
	campos := "names,emailAddresses,phoneNumbers,organizations,addresses,birthdays,biographies,urls,nicknames,clientData"
	var r personaPeople
	if err := g.api.pedir(ctx, http.MethodPatch, g.Base+"/"+obj.ID+":updateContact?updatePersonFields="+campos+"&personFields=metadata", p, nil, &r); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: r.ResourceName, ETag: r.ETag}, nil
}

// Borrar quita el contacto. People no admite If-Match al borrar: un cambio hecho allí
// en el mismo instante se pierde con el borrado que la persona pidió aquí.
func (g *GooglePeople) Borrar(ctx context.Context, _ Coleccion, id, etag string) error {
	if strings.HasPrefix(id, prefijoGrupoGoogle) {
		return g.borrarGrupo(ctx, id, etag)
	}
	err := g.api.pedir(ctx, http.MethodDelete, g.Base+"/"+id+":deleteContact", nil, nil, nil)
	if errors.Is(err, ErrNoEsta) {
		return nil
	}
	return err
}
