// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Microsoft Graph usa estas claves camelCase y OData fijas.
package agenda

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// Microsoft Graph (Outlook, Microsoft 365): `delta` para la bajada incremental,
// `If-Match` con la ETag del evento o del contacto al cambiarlo. Graph manda él las
// invitaciones al guardar un evento con asistentes (A-11).
//
// El calendario se sincroniza con `calendarView/delta`, que pide una ventana: desde la
// de la primera bajada hasta dos años por delante. Una serie llega como sus
// ocurrencias; aquí se guarda como su maestro con la regla (RRULE, de `recurrence`),
// las excepciones y, como EXDATE, las ocurrencias que se cancelaron (las que la regla
// da y Graph ya no tiene). Las horas van en la zona del evento (`originalStartTimeZone`,
// con su nombre IANA), que es la que hace falta para que una serie no se corra una hora
// al cambiar al horario de verano; y suben en esa zona.
//
// De vuelta, una excepción hecha aquí cambia su ocurrencia en Graph y una fecha quitada
// (EXDATE) borra la suya, que Graph cancela a los invitados. Quien no organiza no puede
// cambiar el evento: su respuesta («¿Vas a ir?») sube con `accept`, `tentativelyAccept`
// o `decline`, y Graph se la manda a quien organiza.

// HorizonteGraph es hasta dónde, hacia delante, se sincroniza el calendario de Graph.
const HorizonteGraph = 2 * 365 * 24 * time.Hour

// GraphCalendario es [Remoto] sobre el calendario de Microsoft Graph.
type GraphCalendario struct {
	api  apiJSON
	Base string // «https://graph.microsoft.com/v1.0»
	// Ahora es el reloj (las pruebas lo cambian).
	Ahora func() time.Time

	mu     sync.Mutex
	correo string // el de la cuenta (/me), al contestar
}

type horaGraph struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func (h horaGraph) instante() time.Time {
	loc := time.UTC
	if h.TimeZone != "" && h.TimeZone != "UTC" {
		if l, err := time.LoadLocation(h.TimeZone); err == nil {
			loc = l
		}
	}
	for _, f := range []string{"2006-01-02T15:04:05.9999999", time.RFC3339Nano, "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(f, h.DateTime, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

type direccionGraph struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type asistenteGraph struct {
	EmailAddress direccionGraph `json:"emailAddress"`
	Status       *struct {
		Response string `json:"response"`
	} `json:"status,omitempty"`
	Type string `json:"type,omitempty"`
}

type patronGraph struct {
	Type           string   `json:"type"`
	Interval       int      `json:"interval"`
	Month          int      `json:"month,omitempty"`
	DayOfMonth     int      `json:"dayOfMonth,omitempty"`
	DaysOfWeek     []string `json:"daysOfWeek,omitempty"`
	FirstDayOfWeek string   `json:"firstDayOfWeek,omitempty"`
	Index          string   `json:"index,omitempty"`
}

type rangoGraph struct {
	Type                string `json:"type"`
	StartDate           string `json:"startDate"`
	EndDate             string `json:"endDate,omitempty"`
	NumberOfOccurrences int    `json:"numberOfOccurrences,omitempty"`
	RecurrenceTimeZone  string `json:"recurrenceTimeZone,omitempty"`
}

type recurrenciaGraph struct {
	Pattern patronGraph `json:"pattern"`
	Range   rangoGraph  `json:"range"`
}

type eventoGraph struct {
	ID            string            `json:"id,omitempty"`
	ETag          string            `json:"@odata.etag,omitempty"`
	Removed       *struct{}         `json:"@removed,omitempty"`
	Type          string            `json:"type,omitempty"`
	SeriesMaster  string            `json:"seriesMasterId,omitempty"`
	ICalUID       string            `json:"iCalUId,omitempty"`
	Subject       string            `json:"subject"`
	Body          *cuerpoGraph      `json:"body,omitempty"`
	Location      *lugarGraph       `json:"location,omitempty"`
	Start         *horaGraph        `json:"start,omitempty"`
	End           *horaGraph        `json:"end,omitempty"`
	OriginalStart string            `json:"originalStart,omitempty"`
	IsAllDay      bool              `json:"isAllDay"`
	IsCancelled   bool              `json:"isCancelled,omitempty"`
	ShowAs        string            `json:"showAs,omitempty"`
	Attendees     []asistenteGraph  `json:"attendees,omitempty"`
	Organizer     *asistenteGraph   `json:"organizer,omitempty"`
	Recurrence    *recurrenciaGraph `json:"recurrence,omitempty"`
	OnlineMeeting *struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting,omitempty"`
	// IsOnlineMeeting y OnlineMeetingProvider solo viajan al subir un evento que pide su
	// reunión de Teams (reuniones.go).
	IsOnlineMeeting       *bool  `json:"isOnlineMeeting,omitempty"`
	OnlineMeetingProvider string `json:"onlineMeetingProvider,omitempty"`
	LastModified          string `json:"lastModifiedDateTime,omitempty"`
	// Solo de lectura: la zona en que se creó y si la persona organiza (y su respuesta).
	OriginalStartTimeZone string `json:"originalStartTimeZone,omitempty"`
	OriginalEndTimeZone   string `json:"originalEndTimeZone,omitempty"`
	IsOrganizer           *bool  `json:"isOrganizer,omitempty"`
	ResponseStatus        *struct {
		Response string `json:"response"`
	} `json:"responseStatus,omitempty"`
}

type cuerpoGraph struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type lugarGraph struct {
	DisplayName string `json:"displayName"`
}

var respuestasGraph = map[string]string{"none": "NEEDS-ACTION", "notResponded": "NEEDS-ACTION", "accepted": "ACCEPTED", "declined": "DECLINED", "tentativelyAccepted": "TENTATIVE", "organizer": "ACCEPTED"}

func (g *GraphCalendario) ahora() time.Time {
	if g.Ahora != nil {
		return g.Ahora()
	}
	return time.Now()
}

// Colecciones son los calendarios de la persona.
func (g *GraphCalendario) Colecciones(ctx context.Context) ([]Coleccion, error) {
	var out []Coleccion
	u := g.Base + "/me/calendars?$top=100"
	for range 50 {
		var r struct {
			Value []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				CanEdit  bool   `json:"canEdit"`
				HexColor string `json:"hexColor"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, nil, &r); err != nil {
			return nil, err
		}
		for _, c := range r.Value {
			out = append(out, Coleccion{ID: c.ID, Nombre: c.Name, Color: strings.ToUpper(c.HexColor), SoloLectura: !c.CanEdit, Planifica: true})
		}
		if r.Next == "" || !mismoHost(g.Base, r.Next) {
			break
		}
		u = r.Next
	}
	return out, nil
}

// Cambios baja con delta. La ficha es el deltaLink.
func (g *GraphCalendario) Cambios(ctx context.Context, col Coleccion, ficha string, _ func(string) (string, bool), desde time.Time) (Cambios, error) {
	var ca Cambios
	u := ficha
	if u == "" || !mismoHost(g.Base, u) {
		ficha = ""
		if desde.IsZero() {
			desde = g.ahora().Add(-VentanaCalendario)
		}
		u = g.Base + "/me/calendars/" + url.PathEscape(col.ID) + "/calendarView/delta?" + url.Values{
			"startDateTime": {desde.UTC().Format(time.RFC3339)},
			"endDateTime":   {g.ahora().Add(HorizonteGraph).UTC().Format(time.RFC3339)},
		}.Encode()
	}
	cab := map[string]string{"Prefer": `odata.maxpagesize=200, outlook.timezone="UTC"`}
	sueltos := map[string]eventoGraph{}
	series := map[string]bool{}
	borrados := map[string]bool{}
	for range 5000 {
		var r struct {
			Value []eventoGraph `json:"value"`
			Next  string        `json:"@odata.nextLink"`
			Delta string        `json:"@odata.deltaLink"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, cab, &r); err != nil {
			return ca, err
		}
		for _, e := range r.Value {
			switch {
			case e.Removed != nil:
				// Una ocurrencia que se quita es la serie que cambia; un evento suelto, que
				// se borra. Por el id no se sabe cuál: si no es una serie conocida, se borra.
				borrados[e.ID] = true
			case e.SeriesMaster != "":
				series[e.SeriesMaster] = true
			case e.Type == "seriesMaster":
				series[e.ID] = true
			default:
				sueltos[e.ID] = e
			}
		}
		if r.Next != "" && mismoHost(g.Base, r.Next) {
			u = r.Next
			continue
		}
		ca.Ficha = r.Delta
		break
	}
	ca.Completo = ficha == ""
	for id, e := range sueltos {
		if e.IsCancelled {
			ca.Borrados = append(ca.Borrados, id)
			continue
		}
		ca.Puestos = append(ca.Puestos, Objeto{ID: id, ETag: e.ETag, Datos: []byte(icsDeGraph(e, nil, nil))})
		ca.Vistos = append(ca.Vistos, id)
	}
	for _, id := range ordenarIDs(series) {
		obj, err := g.Leer(ctx, col, id)
		if errors.Is(err, ErrNoEsta) {
			ca.Borrados = append(ca.Borrados, id)
			continue
		}
		if err != nil {
			return ca, err
		}
		ca.Puestos = append(ca.Puestos, obj)
		ca.Vistos = append(ca.Vistos, id)
	}
	for id := range borrados {
		if !series[id] {
			ca.Borrados = append(ca.Borrados, id)
		}
	}
	return ca, nil
}

// cabUTC pide las horas en UTC: así se leen sin ambigüedad, y la zona del evento sale
// de originalStartTimeZone.
var cabUTC = map[string]string{"Prefer": `outlook.timezone="UTC"`}

// instancias son las ocurrencias de una serie dentro de la ventana (las normales y las
// excepciones; las canceladas ya no están).
func (g *GraphCalendario) instancias(ctx context.Context, id string) ([]eventoGraph, error) {
	u := g.Base + "/me/events/" + url.PathEscape(id) + "/instances?" + url.Values{
		"startDateTime": {g.ahora().Add(-VentanaCalendario).UTC().Format(time.RFC3339)},
		"endDateTime":   {g.ahora().Add(HorizonteGraph).UTC().Format(time.RFC3339)},
		"$top":          {"200"},
	}.Encode()
	var out []eventoGraph
	for range 100 {
		var r struct {
			Value []eventoGraph `json:"value"`
			Next  string        `json:"@odata.nextLink"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, cabUTC, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Value...)
		if r.Next == "" || !mismoHost(g.Base, r.Next) {
			break
		}
		u = r.Next
	}
	return out, nil
}

// originalDe es el inicio original de una ocurrencia (el de la regla), en UTC.
func originalDe(e eventoGraph) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, e.OriginalStart); err == nil {
		return t.UTC()
	}
	if e.Start != nil {
		return e.Start.instante().UTC()
	}
	return time.Time{}
}

// Leer trae un evento; si es una serie, con sus excepciones dentro de la ventana y las
// ocurrencias canceladas como EXDATE.
func (g *GraphCalendario) Leer(ctx context.Context, _ Coleccion, id string) (Objeto, error) {
	var m eventoGraph
	if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/me/events/"+url.PathEscape(id), nil, cabUTC, &m); err != nil {
		return Objeto{}, err
	}
	var exc []eventoGraph
	var quitadas []time.Time
	if m.Type == "seriesMaster" {
		todas, err := g.instancias(ctx, id)
		if err != nil {
			return Objeto{}, err
		}
		vistas := map[time.Time]bool{}
		for _, e := range todas {
			vistas[originalDe(e)] = true
			if e.Type == "exception" {
				exc = append(exc, e)
			}
		}
		// Lo que la regla da en la ventana y Graph no tiene, se canceló.
		for _, t := range g.ocurrenciasDe(m) {
			if !vistas[t] {
				quitadas = append(quitadas, t)
			}
		}
	}
	return Objeto{ID: m.ID, ETag: m.ETag, Datos: []byte(icsDeGraph(m, exc, quitadas))}, nil
}

// ocurrenciasDe son los inicios (UTC) que da la regla de un maestro en la ventana de
// las instancias. Si la regla no se puede expandir, ninguno: no se quita nada.
func (g *GraphCalendario) ocurrenciasDe(m eventoGraph) []time.Time {
	cal, err := ical.NewDecoder(strings.NewReader(icsDeGraph(m, nil, nil))).Decode()
	if err != nil {
		return nil
	}
	ev := principal(cal)
	if ev == nil {
		return nil
	}
	set, err := ev.RecurrenceSet(time.UTC)
	if err != nil || set == nil {
		return nil
	}
	var out []time.Time
	for _, t := range set.Between(g.ahora().Add(-VentanaCalendario), g.ahora().Add(HorizonteGraph), true) {
		out = append(out, t.UTC())
		if len(out) > 5000 {
			break
		}
	}
	return out
}

func momentoGraph(h *horaGraph, dia bool, zona string) momento {
	if h == nil {
		return momento{}
	}
	t := h.instante()
	if dia {
		return momento{Dia: true, T: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}
	}
	return momento{T: t, Zona: zona}
}

// icsDeGraph escribe el VCALENDAR de un evento (y, si es una serie, de sus excepciones
// y sus ocurrencias canceladas).
func icsDeGraph(m eventoGraph, exc []eventoGraph, quitadas []time.Time) string {
	uid := m.ICalUID
	if uid == "" {
		uid = m.ID
	}
	zona := zonaIANA(m.OriginalStartTimeZone)
	var l lineas
	abrirVCALENDAR(&l)
	veventGraph(&l, m, uid, "", zona, quitadas)
	for _, e := range exc {
		veventGraph(&l, e, uid, e.OriginalStart, zona, nil)
	}
	l.poner("END", "", "VCALENDAR")
	return l.b.String()
}

func veventGraph(l *lineas, e eventoGraph, uid, original, zonaSerie string, quitadas []time.Time) {
	zonaIni, zonaFin := zonaIANA(e.OriginalStartTimeZone), zonaIANA(e.OriginalEndTimeZone)
	if e.OriginalStartTimeZone == "" {
		zonaIni = zonaSerie
	}
	if e.OriginalEndTimeZone == "" {
		zonaFin = zonaIni
	}
	l.poner("BEGIN", "", "VEVENT")
	l.poner("UID", "", textoICS(uid))
	sello := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339Nano, e.LastModified); err == nil {
		sello = t.UTC()
	}
	l.poner("DTSTAMP", "", sello.Format("20060102T150405Z"))
	if original != "" {
		if t, err := time.Parse(time.RFC3339Nano, original); err == nil {
			// Como el DTSTART de la serie: en su zona (RFC 5545 §3.8.4.4).
			p, v := momento{Dia: e.IsAllDay, T: t, Zona: zonaSerie}.ics()
			if e.IsAllDay {
				p, v = momento{Dia: true, T: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}.ics()
			}
			l.poner("RECURRENCE-ID", p, v)
		}
	}
	p, v := momentoGraph(e.Start, e.IsAllDay, zonaIni).ics()
	l.poner("DTSTART", p, v)
	p, v = momentoGraph(e.End, e.IsAllDay, zonaFin).ics()
	l.poner("DTEND", p, v)
	for _, t := range quitadas {
		// En UTC: es la forma que lee bien todo calendario (y el de aquí).
		p, v := momento{T: t}.ics()
		if e.IsAllDay {
			p, v = momento{Dia: true, T: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}.ics()
		}
		l.poner("EXDATE", p, v)
	}
	l.texto("SUMMARY", e.Subject)
	if d := descripcionGraph(e.Body); d != "" {
		l.texto("DESCRIPTION", d)
	}
	if e.Location != nil {
		l.texto("LOCATION", e.Location.DisplayName)
	}
	if e.Recurrence != nil && original == "" {
		if r := rruleDeGraph(*e.Recurrence); r != "" {
			l.poner("RRULE", "", r)
		}
	}
	if e.Organizer != nil {
		l.organizador(e.Organizer.EmailAddress.Address, e.Organizer.EmailAddress.Name)
	}
	l.asistentes(asistentesGraph(e.Attendees))
	if e.ShowAs == "free" {
		l.poner("TRANSP", "", "TRANSPARENT")
	}
	if e.OnlineMeeting != nil && e.OnlineMeeting.JoinURL != "" {
		l.poner("CONFERENCE", "VALUE=URI", e.OnlineMeeting.JoinURL)
	}
	l.poner("END", "", "VEVENT")
}

// descripcionGraph es el cuerpo de un evento de Graph como texto ("" si no tiene).
func descripcionGraph(c *cuerpoGraph) string {
	if c == nil || strings.TrimSpace(c.Content) == "" {
		return ""
	}
	texto := c.Content
	if strings.EqualFold(c.ContentType, "html") {
		texto = htmltext.Text(texto)
	}
	return strings.TrimSpace(texto)
}

// asistentesGraph son los asistentes de un evento de Graph, con su respuesta.
func asistentesGraph(gs []asistenteGraph) []asistente {
	as := make([]asistente, 0, len(gs))
	for _, a := range gs {
		x := asistente{Correo: a.EmailAddress.Address, Nombre: a.EmailAddress.Name}
		if a.Status != nil {
			x.Estado = respuestasGraph[a.Status.Response]
		}
		as = append(as, x)
	}
	return as
}

var (
	diasGraph    = map[string]string{"monday": "MO", "tuesday": "TU", "wednesday": "WE", "thursday": "TH", "friday": "FR", "saturday": "SA", "sunday": "SU"}
	indicesGraph = map[string]string{"first": "1", "second": "2", "third": "3", "fourth": "4", "last": "-1"}
)

// rruleDeGraph pasa la recurrencia de Graph a RRULE (RFC 5545).
func rruleDeGraph(r recurrenciaGraph) string {
	var ps []string
	p := r.Pattern
	dias := func() string {
		ds := make([]string, 0, len(p.DaysOfWeek))
		for _, d := range p.DaysOfWeek {
			ds = append(ds, diasGraph[strings.ToLower(d)])
		}
		return strings.Join(ds, ",")
	}
	switch p.Type {
	case "daily":
		ps = append(ps, "FREQ=DAILY")
	case "weekly":
		ps = append(ps, "FREQ=WEEKLY", "BYDAY="+dias())
	case "absoluteMonthly":
		ps = append(ps, "FREQ=MONTHLY", "BYMONTHDAY="+strconv.Itoa(p.DayOfMonth))
	case "relativeMonthly":
		ps = append(ps, "FREQ=MONTHLY", "BYDAY="+dias(), "BYSETPOS="+indicesGraph[p.Index])
	case "absoluteYearly":
		ps = append(ps, "FREQ=YEARLY", "BYMONTH="+strconv.Itoa(p.Month), "BYMONTHDAY="+strconv.Itoa(p.DayOfMonth))
	case "relativeYearly":
		ps = append(ps, "FREQ=YEARLY", "BYMONTH="+strconv.Itoa(p.Month), "BYDAY="+dias(), "BYSETPOS="+indicesGraph[p.Index])
	default:
		return ""
	}
	if p.Interval > 1 {
		ps = append(ps, "INTERVAL="+strconv.Itoa(p.Interval))
	}
	switch r.Range.Type {
	case "endDate":
		// El último día entero de la serie, en su zona, y en UTC como pide UNTIL con un
		// DTSTART con zona (RFC 5545 §3.3.10).
		loc := time.UTC
		if z := zonaIANA(r.Range.RecurrenceTimeZone); z != "" {
			loc, _ = time.LoadLocation(z)
		}
		if t, err := time.ParseInLocation(time.DateOnly, r.Range.EndDate, loc); err == nil {
			ps = append(ps, "UNTIL="+t.AddDate(0, 0, 1).Add(-time.Second).UTC().Format("20060102T150405Z"))
		}
	case "numbered":
		ps = append(ps, "COUNT="+strconv.Itoa(r.Range.NumberOfOccurrences))
	}
	return strings.Join(ps, ";")
}

// recurrenciaDeRRULE es lo contrario, para lo que Graph sabe guardar; si no,
// ErrNoSoportado (el evento se queda aquí y lo dice la cola de pendientes). zona es la
// del DTSTART (vacía, UTC): las fechas de la serie son las de esa zona.
func recurrenciaDeRRULE(rrule string, inicio time.Time, zona string) (*recurrenciaGraph, error) {
	campos := map[string]string{}
	for _, par := range strings.Split(rrule, ";") {
		if k, v, ok := strings.Cut(par, "="); ok {
			campos[strings.ToUpper(k)] = v
		}
	}
	loc := time.UTC
	if zona != "" {
		if l, err := time.LoadLocation(zona); err == nil {
			loc = l
		} else {
			zona = ""
		}
	}
	inicio = inicio.In(loc)
	r := &recurrenciaGraph{Range: rangoGraph{Type: "noEnd", StartDate: inicio.Format(time.DateOnly), RecurrenceTimeZone: cmp.Or(zona, "UTC")}}
	r.Pattern.Interval = 1
	if v := campos["INTERVAL"]; v != "" {
		r.Pattern.Interval, _ = strconv.Atoi(v)
	}
	dias, nombreIndice, err := diasDeRRULE(campos)
	if err != nil {
		return nil, err
	}
	mes, _ := strconv.Atoi(campos["BYMONTH"])
	diaMes, _ := strconv.Atoi(campos["BYMONTHDAY"])
	switch campos["FREQ"] {
	case "DAILY":
		r.Pattern.Type = "daily"
	case "WEEKLY":
		r.Pattern.Type = "weekly"
		if len(dias) == 0 {
			dias = []string{strings.ToLower(inicio.Weekday().String())}
		}
	case "MONTHLY":
		r.Pattern.Type = "absoluteMonthly"
		if nombreIndice != "" {
			r.Pattern.Type, r.Pattern.Index = "relativeMonthly", nombreIndice
		} else if diaMes == 0 {
			diaMes = inicio.Day()
		}
	case "YEARLY":
		r.Pattern.Type = "absoluteYearly"
		if mes == 0 {
			mes = int(inicio.Month())
		}
		if nombreIndice != "" {
			r.Pattern.Type, r.Pattern.Index = "relativeYearly", nombreIndice
		} else if diaMes == 0 {
			diaMes = inicio.Day()
		}
	default:
		return nil, ErrNoSoportado
	}
	r.Pattern.DaysOfWeek, r.Pattern.Month, r.Pattern.DayOfMonth = dias, mes, diaMes
	if err := finDeRRULE(&r.Range, campos, loc); err != nil {
		return nil, err
	}
	return r, nil
}

// finDeRRULE pone en rg el final de la serie: COUNT o UNTIL (el día, en la zona de la
// serie). Sin ninguno, la serie no acaba. ErrNoSoportado si UNTIL no se lee.
func finDeRRULE(rg *rangoGraph, campos map[string]string, loc *time.Location) error {
	if v := campos["COUNT"]; v != "" {
		rg.Type = "numbered"
		rg.NumberOfOccurrences, _ = strconv.Atoi(v)
		return nil
	}
	v := campos["UNTIL"]
	if v == "" {
		return nil
	}
	t, err := time.Parse("20060102T150405Z", v)
	if err != nil {
		t, err = time.Parse("20060102", v[:min(len(v), 8)])
	}
	if err != nil {
		return ErrNoSoportado
	}
	rg.Type, rg.EndDate = "endDate", t.In(loc).Format(time.DateOnly)
	return nil
}

// diasDeRRULE lee BYDAY y BYSETPOS como los días de la semana de Graph y el índice de
// la semana del mes («last» por «-1FR»). ErrNoSoportado si Graph no lo sabe guardar.
func diasDeRRULE(campos map[string]string) (dias []string, nombreIndice string, err error) {
	var indice string
	for _, d := range strings.Split(campos["BYDAY"], ",") {
		if d == "" {
			continue
		}
		if len(d) < 2 {
			return nil, "", ErrNoSoportado // ni siquiera un día («MO», «-1FR»)
		}
		pre, dia := d[:len(d)-2], d[len(d)-2:]
		nombre := ""
		for k, v := range diasGraph {
			if v == dia {
				nombre = k
			}
		}
		if nombre == "" {
			return nil, "", ErrNoSoportado
		}
		if pre != "" {
			indice = pre
		}
		dias = append(dias, nombre)
	}
	if v := campos["BYSETPOS"]; v != "" {
		indice = v
	}
	for k, v := range indicesGraph {
		if v == indice {
			nombreIndice = k
		}
	}
	if indice != "" && nombreIndice == "" {
		return nil, "", ErrNoSoportado
	}
	return dias, nombreIndice, nil
}

// eventoGraphDe es el evento de Graph de un VEVENT de aquí.
func eventoGraphDe(ev ical.Event, maestro bool) (eventoGraph, error) {
	ini, err := momentoDe(ev, ical.PropDateTimeStart)
	if err != nil {
		return eventoGraph{}, err
	}
	fin, err := momentoDe(ev, ical.PropDateTimeEnd)
	if err != nil {
		return eventoGraph{}, err
	}
	hora := func(m momento) *horaGraph {
		// En la zona del evento, como la guardaría Outlook; sin zona, en UTC.
		if !m.Dia && m.Zona != "" {
			if loc, err := time.LoadLocation(m.Zona); err == nil {
				return &horaGraph{DateTime: m.T.In(loc).Format("2006-01-02T15:04:05"), TimeZone: m.Zona}
			}
		}
		return &horaGraph{DateTime: m.T.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"}
	}
	g := eventoGraph{
		Subject: textoDe(ev, ical.PropSummary), Start: hora(ini), End: hora(fin), IsAllDay: ini.Dia,
		Body:     &cuerpoGraph{ContentType: "text", Content: textoDe(ev, ical.PropDescription)},
		Location: &lugarGraph{DisplayName: textoDe(ev, ical.PropLocation)}, ShowAs: "busy",
	}
	if strings.EqualFold(textoDe(ev, ical.PropTransparency), "TRANSPARENT") {
		g.ShowAs = "free"
	}
	for _, a := range asistentesDe(ev) {
		g.Attendees = append(g.Attendees, asistenteGraph{EmailAddress: direccionGraph{Address: a.Correo, Name: a.Nombre}, Type: "required"})
	}
	if maestro && pideReunion(ev, ReunionTeams) {
		si := true
		g.IsOnlineMeeting, g.OnlineMeetingProvider = &si, "teamsForBusiness"
	}
	if maestro {
		reglas := ev.Props.Values(ical.PropRecurrenceRule)
		// Graph no guarda fechas sueltas en la regla. Las quitadas (EXDATE) son allí
		// ocurrencias canceladas: se borran aparte ([GraphCalendario.ocurrencias]).
		if len(reglas) > 1 || ev.Props.Get(ical.PropRecurrenceDates) != nil {
			return g, fmt.Errorf("%w: repetición con fechas sueltas", ErrNoSoportado)
		}
		if len(reglas) == 1 {
			r, err := recurrenciaDeRRULE(reglas[0].Value, ini.T, ini.Zona)
			if err != nil {
				return g, err
			}
			g.Recurrence = r
		}
	}
	return g, nil
}

func (g *GraphCalendario) subir(ctx context.Context, metodo, u, etag string, datos []byte) (Objeto, error) {
	m, exc, err := eventosDe(datos)
	if err != nil {
		return Objeto{}, err
	}
	ev, err := eventoGraphDe(*m, true)
	if err != nil {
		return Objeto{}, err
	}
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = etag
	}
	var r eventoGraph
	if err := g.api.pedir(ctx, metodo, u, ev, cab, &r); err != nil {
		return Objeto{}, err
	}
	if g.ocurrencias(ctx, r.ID, *m, exc) {
		// Tocar una ocurrencia cambia la serie: se lee su ETag de ahora, la que la
		// próxima subida tiene que mandar.
		var x eventoGraph
		if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/me/events/"+url.PathEscape(r.ID)+"?$select=id", nil, nil, &x); err == nil && x.ETag != "" {
			r.ETag = x.ETag
		}
	}
	if ev.IsOnlineMeeting != nil {
		// Se pidió la reunión de Teams: lo que vuelve es el evento como lo tiene ya
		// Outlook, con su enlace (CONFERENCE), y el sincronizador lo guarda aquí.
		if o, err := g.Leer(ctx, Coleccion{}, r.ID); err == nil {
			return o, nil
		}
	}
	return Objeto{ID: r.ID, ETag: r.ETag}, nil
}

// fechasDe son los instantes (UTC) de las EXDATE de un evento, con todos sus valores.
func fechasDe(ev ical.Event) []time.Time {
	var out []time.Time
	for _, p := range ev.Props.Values(ical.PropExceptionDates) {
		for _, v := range strings.Split(p.Value, ",") {
			q := p
			q.Value = strings.TrimSpace(v)
			if q.ValueType() == ical.ValueDate {
				if t, err := time.Parse("20060102", q.Value); err == nil {
					out = append(out, t)
				}
				continue
			}
			if t, err := q.DateTime(time.UTC); err == nil {
				out = append(out, t.UTC())
			}
		}
	}
	return out
}

// ocurrencias lleva a Graph lo que se hizo aquí con ocurrencias sueltas de una serie:
// las quitadas (EXDATE) se borran —Graph lo cancela a los invitados— y las excepciones
// cambian su ocurrencia. Como en Google, lo que falle se ve en la próxima bajada (la
// serie se vuelve a leer entera). Dice si tocó algo.
func (g *GraphCalendario) ocurrencias(ctx context.Context, id string, m ical.Event, exc []ical.Event) bool {
	quitadas := fechasDe(m)
	if m.Props.Get(ical.PropRecurrenceRule) == nil || (len(quitadas) == 0 && len(exc) == 0) {
		return false
	}
	todas, err := g.instancias(ctx, id)
	if err != nil {
		return false
	}
	ini, _ := momentoDe(m, ical.PropDateTimeStart)
	mismo := func(a, b time.Time) bool {
		if ini.Dia {
			return a.Format(time.DateOnly) == b.Format(time.DateOnly)
		}
		return a.Equal(b)
	}
	buscar := func(t time.Time) *eventoGraph {
		for i := range todas {
			if mismo(originalDe(todas[i]), t) {
				return &todas[i]
			}
		}
		return nil
	}
	tocado := false
	borrar := func(e *eventoGraph) {
		if g.api.pedir(ctx, http.MethodDelete, g.Base+"/me/events/"+url.PathEscape(e.ID), nil, nil, nil) == nil {
			tocado = true
		}
	}
	for _, t := range quitadas {
		if e := buscar(t); e != nil {
			borrar(e)
		}
	}
	for _, x := range exc {
		rid, err := momentoDe(x, ical.PropRecurrenceID)
		if err != nil {
			continue
		}
		e := buscar(rid.T.UTC())
		if e == nil {
			continue
		}
		if strings.EqualFold(textoDe(x, ical.PropStatus), "CANCELLED") {
			borrar(e)
			continue
		}
		quiero, err := eventoGraphDe(x, false)
		if err != nil || igualOcurrencia(*e, quiero) {
			continue
		}
		if g.api.pedir(ctx, http.MethodPatch, g.Base+"/me/events/"+url.PathEscape(e.ID), quiero, nil, nil) == nil {
			tocado = true
		}
	}
	return tocado
}

// igualOcurrencia dice si una ocurrencia de Graph ya es como la excepción de aquí.
func igualOcurrencia(e, quiero eventoGraph) bool {
	if e.Subject != quiero.Subject || e.Start == nil || e.End == nil {
		return false
	}
	if !e.Start.instante().Equal(quiero.Start.instante()) || !e.End.instante().Equal(quiero.End.instante()) {
		return false
	}
	lugar := ""
	if e.Location != nil {
		lugar = e.Location.DisplayName
	}
	return lugar == quiero.Location.DisplayName
}

// Crear sube un evento nuevo; Graph manda las invitaciones.
func (g *GraphCalendario) Crear(ctx context.Context, col Coleccion, _ string, datos []byte) (Objeto, error) {
	return g.subir(ctx, http.MethodPost, g.Base+"/me/calendars/"+url.PathEscape(col.ID)+"/events", "", datos)
}

// Cambiar cambia los campos de aquí del evento (PATCH) si sigue en su ETag, con sus
// excepciones y sus ocurrencias quitadas. Si la persona no organiza, Graph no le deja
// cambiarlo: lo que sube es su respuesta (contestar).
func (g *GraphCalendario) Cambiar(ctx context.Context, _ Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	var actual eventoGraph
	u := g.Base + "/me/events/" + url.PathEscape(obj.ID)
	if err := g.api.pedir(ctx, http.MethodGet, u+"?$select=id,isOrganizer,responseStatus,attendees", nil, cabUTC, &actual); err != nil {
		return Objeto{}, err
	}
	if obj.ETag != "" && actual.ETag != "" && actual.ETag != obj.ETag {
		return Objeto{}, ErrConflicto
	}
	if actual.IsOrganizer != nil && !*actual.IsOrganizer {
		return g.contestar(ctx, obj, actual, datos)
	}
	return g.subir(ctx, http.MethodPatch, u, obj.ETag, datos)
}

// accionesGraph son las respuestas a una invitación, por PARTSTAT.
var accionesGraph = map[string]string{"ACCEPTED": "accept", "TENTATIVE": "tentativelyAccept", "DECLINED": "decline"}

// contestar manda la respuesta de la persona a una invitación (`accept`,
// `tentativelyAccept`, `decline`), si cambió. Lo demás que se cambiara aquí es de quien
// organiza y no sube.
func (g *GraphCalendario) contestar(ctx context.Context, obj Objeto, actual eventoGraph, datos []byte) (Objeto, error) {
	m, _, err := eventosDe(datos)
	if err != nil {
		return Objeto{}, err
	}
	antes := ""
	if actual.ResponseStatus != nil {
		antes = respuestasGraph[actual.ResponseStatus.Response]
	}
	ahora := ""
	yo := g.yo(ctx)
	enGraph := map[string]string{}
	for _, a := range actual.Attendees {
		if a.Status != nil {
			enGraph[strings.ToLower(a.EmailAddress.Address)] = respuestasGraph[a.Status.Response]
		}
	}
	for _, a := range asistentesDe(*m) {
		c := strings.ToLower(a.Correo)
		if yo != "" && c == yo {
			ahora = a.Estado
			break
		}
		if yo == "" && a.Estado != enGraph[c] && accionesGraph[a.Estado] != "" {
			// Sin saber quién es la persona, su respuesta es la que cambió aquí.
			ahora = a.Estado
		}
	}
	accion := accionesGraph[ahora]
	if accion == "" || ahora == antes {
		return Objeto{ID: obj.ID, ETag: actual.ETag}, nil
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/me/events/"+url.PathEscape(obj.ID)+"/"+accion, map[string]any{"sendResponse": true}, nil, nil); err != nil {
		return Objeto{}, err
	}
	var x eventoGraph
	if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/me/events/"+url.PathEscape(obj.ID)+"?$select=id", nil, nil, &x); err != nil {
		return Objeto{ID: obj.ID}, nil //nolint:nilerr // la respuesta ya salió; sin ETag, la vuelta siguiente lee el evento.
	}
	return Objeto{ID: obj.ID, ETag: x.ETag}, nil
}

// yo es la dirección de la cuenta de Microsoft (la de /me), en minúsculas, o "" si no
// se sabe.
func (g *GraphCalendario) yo(ctx context.Context) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.correo != "" {
		return g.correo
	}
	var r struct {
		Mail string `json:"mail"`
		UPN  string `json:"userPrincipalName"`
	}
	if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/me?$select=mail,userPrincipalName", nil, nil, &r); err != nil {
		return ""
	}
	g.correo = strings.ToLower(strings.TrimSpace(cmp.Or(r.Mail, r.UPN)))
	return g.correo
}

// CrearColeccion crea un calendario en la cuenta de Microsoft.
func (g *GraphCalendario) CrearColeccion(ctx context.Context, nombre, color string) (Coleccion, error) {
	var r struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/me/calendars", map[string]string{"name": strings.TrimSpace(nombre)}, nil, &r); err != nil {
		return Coleccion{}, err
	}
	return Coleccion{ID: r.ID, Nombre: r.Name, Color: color, Planifica: true}, nil
}

// Borrar quita el evento si sigue en su ETag.
func (g *GraphCalendario) Borrar(ctx context.Context, _ Coleccion, id, etag string) error {
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = etag
	}
	err := g.api.pedir(ctx, http.MethodDelete, g.Base+"/me/events/"+url.PathEscape(id), nil, cab, nil)
	if errors.Is(err, ErrNoEsta) || errors.Is(err, ErrFichaCaducada) {
		return nil
	}
	return err
}

// ----- Contactos de Graph -----

// GraphContactos es [Remoto] sobre los contactos de Outlook: la carpeta principal y
// las demás carpetas de contactos, cada una es una libreta.
type GraphContactos struct {
	api  apiJSON
	Base string

	mu  sync.Mutex
	enl Enlaces // los de la carpeta en la vuelta en curso (etiquetas.go)
}

const carpetaPrincipal = "contacts"

type contactoGraph struct {
	ID               string           `json:"id,omitempty"`
	ETag             string           `json:"@odata.etag,omitempty"`
	Removed          *struct{}        `json:"@removed,omitempty"`
	DisplayName      string           `json:"displayName,omitempty"`
	GivenName        string           `json:"givenName,omitempty"`
	Surname          string           `json:"surname,omitempty"`
	MiddleName       string           `json:"middleName,omitempty"`
	Title            string           `json:"title,omitempty"`
	NickName         string           `json:"nickName,omitempty"`
	CompanyName      string           `json:"companyName,omitempty"`
	Department       string           `json:"department,omitempty"`
	JobTitle         string           `json:"jobTitle,omitempty"`
	EmailAddresses   []direccionGraph `json:"emailAddresses,omitempty"`
	BusinessPhones   []string         `json:"businessPhones,omitempty"`
	HomePhones       []string         `json:"homePhones,omitempty"`
	MobilePhone      string           `json:"mobilePhone,omitempty"`
	Birthday         string           `json:"birthday,omitempty"`
	PersonalNotes    string           `json:"personalNotes,omitempty"`
	BusinessHomePage string           `json:"businessHomePage,omitempty"`
	HomeAddress      *direccionPostal `json:"homeAddress,omitempty"`
	BusinessAddress  *direccionPostal `json:"businessAddress,omitempty"`
}

type direccionPostal struct {
	Street          string `json:"street,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
	CountryOrRegion string `json:"countryOrRegion,omitempty"`
}

func (g *GraphContactos) contactos(col Coleccion) string {
	if col.ID == carpetaPrincipal {
		return g.Base + "/me/contacts"
	}
	return g.Base + "/me/contactFolders/" + url.PathEscape(col.ID) + "/contacts"
}

// Colecciones son la carpeta principal y las demás.
func (g *GraphContactos) Colecciones(ctx context.Context) ([]Coleccion, error) {
	out := []Coleccion{{ID: carpetaPrincipal, Nombre: "Outlook"}}
	var r struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/me/contactFolders?$top=100", nil, nil, &r); err != nil {
		return nil, err
	}
	for _, c := range r.Value {
		out = append(out, Coleccion{ID: c.ID, Nombre: c.DisplayName})
	}
	return out, nil
}

// Cambios baja con delta.
func (g *GraphContactos) Cambios(ctx context.Context, col Coleccion, ficha string, etagDe func(string) (string, bool), _ time.Time) (Cambios, error) {
	var ca Cambios
	vistos := map[string]string{} // id → UID de los contactos que bajan
	u := ficha
	if u == "" || !mismoHost(g.Base, u) {
		ficha, u = "", g.contactos(col)+"/delta"
	}
	cab := map[string]string{"Prefer": "odata.maxpagesize=200"}
	for range 5000 {
		var r struct {
			Value []contactoGraph `json:"value"`
			Next  string          `json:"@odata.nextLink"`
			Delta string          `json:"@odata.deltaLink"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, cab, &r); err != nil {
			return ca, err
		}
		for _, c := range r.Value {
			if c.Removed != nil {
				ca.Borrados = append(ca.Borrados, c.ID)
				continue
			}
			ca.Puestos = append(ca.Puestos, Objeto{ID: c.ID, ETag: c.ETag, Datos: []byte(vcardDeGraph(c))})
			ca.Vistos = append(ca.Vistos, c.ID)
			vistos[c.ID] = "graph-" + c.ID
		}
		if r.Next != "" && mismoHost(g.Base, r.Next) {
			u = r.Next
			continue
		}
		ca.Ficha = r.Delta
		break
	}
	ca.Completo = ficha == ""
	// Las etiquetas (las categorías de sus contactos), detrás de los contactos. (En dos
	// pasos: en un mismo return, Go no garantiza leer ca después de rellenarla.)
	err := g.cambiosCategorias(ctx, col, &ca, etagDe, vistos)
	return ca, err
}

func vcardDeGraph(c contactoGraph) string {
	var l lineas
	l.poner("BEGIN", "", "VCARD")
	l.poner("VERSION", "", "3.0")
	l.poner("UID", "", textoICS("graph-"+c.ID))
	fn := c.DisplayName
	if fn == "" {
		fn = strings.TrimSpace(c.GivenName + " " + c.Surname)
	}
	l.poner("FN", "", textoICS(fn))
	l.poner("N", "", strings.Join([]string{textoICS(c.Surname), textoICS(c.GivenName), textoICS(c.MiddleName), textoICS(c.Title), ""}, ";"))
	l.texto("NICKNAME", c.NickName)
	for _, e := range c.EmailAddresses {
		l.poner("EMAIL", "", textoICS(e.Address))
	}
	if c.MobilePhone != "" {
		l.poner("TEL", "TYPE=cell", textoICS(c.MobilePhone))
	}
	for _, t := range c.BusinessPhones {
		l.poner("TEL", "TYPE=work", textoICS(t))
	}
	for _, t := range c.HomePhones {
		l.poner("TEL", "TYPE=home", textoICS(t))
	}
	if c.CompanyName != "" || c.Department != "" {
		l.poner("ORG", "", textoICS(c.CompanyName)+";"+textoICS(c.Department))
	}
	l.texto("TITLE", c.JobTitle)
	for tipo, d := range map[string]*direccionPostal{"home": c.HomeAddress, "work": c.BusinessAddress} {
		if d == nil || *d == (direccionPostal{}) {
			continue
		}
		l.poner("ADR", "TYPE="+tipo, strings.Join([]string{"", "", textoICS(d.Street), textoICS(d.City), textoICS(d.State), textoICS(d.PostalCode), textoICS(d.CountryOrRegion)}, ";"))
	}
	if t, err := time.Parse(time.RFC3339, c.Birthday); err == nil {
		l.poner("BDAY", "", t.UTC().Format(time.DateOnly))
	}
	l.texto("NOTE", c.PersonalNotes)
	if c.BusinessHomePage != "" {
		l.poner("URL", "", c.BusinessHomePage)
	}
	l.poner("END", "", "VCARD")
	return l.b.String()
}

func contactoDeVCard(datos []byte) (contactoGraph, error) {
	card, err := vcard.NewDecoder(strings.NewReader(string(datos))).Decode()
	if err != nil {
		return contactoGraph{}, err
	}
	c := contactoGraph{
		DisplayName: card.Value(vcard.FieldFormattedName), NickName: card.Value(vcard.FieldNickname),
		JobTitle: card.Value(vcard.FieldTitle), PersonalNotes: card.Value(vcard.FieldNote), BusinessHomePage: card.Value(vcard.FieldURL),
		BusinessPhones: []string{}, HomePhones: []string{}, EmailAddresses: []direccionGraph{},
	}
	if n := card.Name(); n != nil {
		c.GivenName, c.Surname, c.MiddleName, c.Title = n.GivenName, n.FamilyName, n.AdditionalName, n.HonorificPrefix
	}
	if org := card.Value(vcard.FieldOrganization); org != "" {
		c.CompanyName, c.Department, _ = strings.Cut(org, ";")
	}
	for _, f := range card[vcard.FieldEmail] {
		c.EmailAddresses = append(c.EmailAddresses, direccionGraph{Address: f.Value, Name: c.DisplayName})
	}
	for _, f := range card[vcard.FieldTelephone] {
		v := strings.TrimPrefix(f.Value, "tel:")
		switch t := strings.ToLower(f.Params.Get(vcard.ParamType)); {
		case strings.Contains(t, "cell") && c.MobilePhone == "":
			c.MobilePhone = v
		case strings.Contains(t, "home"):
			c.HomePhones = append(c.HomePhones, v)
		default:
			c.BusinessPhones = append(c.BusinessPhones, v)
		}
	}
	for _, a := range card.Addresses() {
		d := &direccionPostal{Street: a.StreetAddress, City: a.Locality, State: a.Region, PostalCode: a.PostalCode, CountryOrRegion: a.Country}
		if strings.Contains(strings.ToLower(a.Params.Get(vcard.ParamType)), "work") {
			c.BusinessAddress = d
		} else if c.HomeAddress == nil {
			c.HomeAddress = d
		}
	}
	if b := card.Value(vcard.FieldBirthday); b != "" {
		if t, err := time.Parse(time.DateOnly, b); err == nil {
			c.Birthday = t.Format(time.RFC3339)
		} else if t, err := time.Parse("20060102", b); err == nil {
			c.Birthday = t.Format(time.RFC3339)
		}
	}
	return c, nil
}

// Leer trae un contacto o una etiqueta.
func (g *GraphContactos) Leer(ctx context.Context, col Coleccion, id string) (Objeto, error) {
	if strings.HasPrefix(id, prefijoCategoria) {
		return g.leerCategoria(ctx, col, id)
	}
	var c contactoGraph
	if err := g.api.pedir(ctx, http.MethodGet, g.contactos(col)+"/"+url.PathEscape(id), nil, nil, &c); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: c.ID, ETag: c.ETag, Datos: []byte(vcardDeGraph(c))}, nil
}

func (g *GraphContactos) subir(ctx context.Context, metodo, u, etag string, datos []byte) (Objeto, error) {
	c, err := contactoDeVCard(datos)
	if err != nil {
		return Objeto{}, err
	}
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = etag
	}
	var r contactoGraph
	if err := g.api.pedir(ctx, metodo, u, c, cab, &r); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: r.ID, ETag: r.ETag}, nil
}

// Crear sube un contacto nuevo, o una etiqueta como categoría de sus contactos.
func (g *GraphContactos) Crear(ctx context.Context, col Coleccion, _ string, datos []byte) (Objeto, error) {
	if esGrupoVCard(string(datos)) {
		return g.crearCategoria(ctx, col, datos)
	}
	return g.subir(ctx, http.MethodPost, g.contactos(col), "", datos)
}

// Cambiar cambia el contacto (o la etiqueta) si sigue en su ETag.
func (g *GraphContactos) Cambiar(ctx context.Context, col Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	if strings.HasPrefix(obj.ID, prefijoCategoria) {
		return g.cambiarCategoria(ctx, col, obj, datos)
	}
	return g.subir(ctx, http.MethodPatch, g.contactos(col)+"/"+url.PathEscape(obj.ID), obj.ETag, datos)
}

// CrearColeccion crea una carpeta de contactos en la cuenta de Microsoft: una libreta.
func (g *GraphContactos) CrearColeccion(ctx context.Context, nombre, _ string) (Coleccion, error) {
	var r struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/me/contactFolders", map[string]string{"displayName": strings.TrimSpace(nombre)}, nil, &r); err != nil {
		return Coleccion{}, err
	}
	return Coleccion{ID: r.ID, Nombre: r.DisplayName}, nil
}

// Borrar quita el contacto (o la etiqueta) si sigue en su ETag.
func (g *GraphContactos) Borrar(ctx context.Context, col Coleccion, id, etag string) error {
	if strings.HasPrefix(id, prefijoCategoria) {
		return g.borrarCategoria(ctx, col, id, etag)
	}
	cab := map[string]string{}
	if etag != "" {
		cab["If-Match"] = etag
	}
	err := g.api.pedir(ctx, http.MethodDelete, g.contactos(col)+"/"+url.PathEscape(id), nil, cab, nil)
	if errors.Is(err, ErrNoEsta) {
		return nil
	}
	return err
}
