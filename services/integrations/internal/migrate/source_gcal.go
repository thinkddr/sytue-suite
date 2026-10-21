// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ConfigGCalBase apunta la API de Google Calendar a otra raíz (nubes soberanas y
// los servidores simulados de los tests).
const ConfigGCalBase = "gcal_base"

const (
	defaultGCalBase = "https://www.googleapis.com"
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	scopeCalendarRead = "https://www.googleapis.com/auth/calendar.readonly"
	// gcalPageSize es el tamaño de página del listado de eventos.
	gcalPageSize = 250
)

// gcal lee los calendarios y los eventos de la cuenta que autorizó la conexión.
//
// Lo que hace difícil migrar un calendario no son los eventos sueltos: son las
// RECURRENCIAS («todos los martes hasta junio», con dos excepciones movidas) y
// las ZONAS HORARIAS. Los dos viajan aquí tal cual los define iCalendar, porque
// Sytue Calendar habla ese mismo lenguaje (RRULE/RDATE/EXDATE + TZID) y no hay
// que traducir nada por el camino.
type gcal struct {
	*google
}

// ListCalendars devuelve los calendarios de la cuenta.
func (g *gcal) ListCalendars(ctx context.Context) ([]Calendar, error) {
	base, err := g.gcalBase()
	if err != nil {
		return nil, err
	}
	//nolint:tagliatelle // los tags son el CONTRATO de la API de Google Calendar.
	var resp struct {
		Items []struct {
			ID              string `json:"id"`
			Summary         string `json:"summary"`
			Description     string `json:"description"`
			TimeZone        string `json:"timeZone"`
			BackgroundColor string `json:"backgroundColor"`
			AccessRole      string `json:"accessRole"`
			Primary         bool   `json:"primary"`
		} `json:"items"`
	}
	if err := getJSON(ctx, g.rt, base+"/calendar/v3/users/me/calendarList", g.token, &resp); err != nil {
		return nil, err
	}

	out := make([]Calendar, 0, len(resp.Items))
	for _, c := range resp.Items {
		cal := Calendar{
			ID: c.ID, Name: c.Summary, Description: c.Description,
			TimeZone: c.TimeZone, Color: c.BackgroundColor, Primary: c.Primary,
		}
		// Un calendario de solo lectura ajeno (el de festivos, el de otra persona)
		// no es contenido de quien migra: copiarlo crearía un duplicado que ya no
		// se actualiza nunca.
		if c.AccessRole == "reader" || c.AccessRole == "freeBusyReader" {
			cal.Skip = "es un calendario ajeno al que solo tienes acceso de lectura: " +
				"copiarlo dejaría una copia congelada; vuelve a suscribirte a él desde Sytue Calendar"
		}
		out = append(out, cal)
	}
	return out, nil
}

// ListEvents devuelve una página de eventos de un calendario.
//
// singleEvents=false a propósito: se quiere la SERIE, no sus mil repeticiones
// expandidas. Expandirlas convertiría «todos los martes» en 500 eventos sueltos
// que ya no se pueden editar como serie.
func (g *gcal) ListEvents(ctx context.Context, calendarID, cursor string) ([]Event, string, error) {
	base, err := g.gcalBase()
	if err != nil {
		return nil, "", err
	}
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(gcalPageSize))
	q.Set("singleEvents", "false")
	q.Set("showDeleted", "false")
	if cursor != "" {
		q.Set("pageToken", cursor)
	}
	u := base + "/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events?" + q.Encode()

	//nolint:tagliatelle // contrato de la API de Google Calendar.
	var resp struct {
		Items []struct {
			ID             string   `json:"id"`
			Status         string   `json:"status"`
			Summary        string   `json:"summary"`
			Description    string   `json:"description"`
			Location       string   `json:"location"`
			ICalUID        string   `json:"iCalUID"`
			Recurrence     []string `json:"recurrence"`
			RecurringEvent string   `json:"recurringEventId"`
			Transparency   string   `json:"transparency"`
			Visibility     string   `json:"visibility"`
			HangoutLink    string   `json:"hangoutLink"`
			Organizer      struct {
				Email string `json:"email"`
			} `json:"organizer"`
			Start     gcalTime `json:"start"`
			End       gcalTime `json:"end"`
			Attendees []struct {
				Email          string `json:"email"`
				DisplayName    string `json:"displayName"`
				Optional       bool   `json:"optional"`
				ResponseStatus string `json:"responseStatus"`
				Organizer      bool   `json:"organizer"`
				Resource       bool   `json:"resource"`
			} `json:"attendees"`
		} `json:"items"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
		return nil, "", err
	}

	out := make([]Event, 0, len(resp.Items))
	for _, e := range resp.Items {
		ev := Event{
			ID: e.ID, UID: e.ICalUID, Summary: e.Summary, Description: e.Description,
			Location: e.Location, Organizer: e.Organizer.Email,
			ConferenceURL: e.HangoutLink, Status: gcalStatus(e.Status),
			Transparency: e.Transparency, Classification: gcalVisibility(e.Visibility),
		}
		if e.RecurringEvent != "" {
			// Una excepción de una serie («el martes 12 se mueve a las 17:00»). Sin la
			// serie delante no significa nada, y esta ola migra la serie completa: se
			// omite con su motivo en vez de crear un evento suelto duplicado.
			ev.Skip = "es una modificación puntual de un evento que se repite: la serie sí se migra, " +
				"pero esa excepción concreta hay que rehacerla a mano"
			out = append(out, ev)
			continue
		}
		ev.Start, ev.AllDay, ev.TZID = e.Start.parse()
		ev.End, _, _ = e.End.parse()
		if ev.Start.IsZero() || ev.End.IsZero() {
			ev.Skip = "el evento de origen no tiene fecha de inicio o de fin utilizable"
			out = append(out, ev)
			continue
		}
		ev.RRule, ev.RDate, ev.ExDate = splitRecurrence(e.Recurrence)
		for _, a := range e.Attendees {
			ev.Attendees = append(ev.Attendees, Attendee{
				Email: a.Email, Name: a.DisplayName,
				Role:     gcalRole(a.Optional, a.Resource),
				PartStat: gcalPartStat(a.ResponseStatus),
				CUType:   gcalCUType(a.Resource),
			})
		}
		out = append(out, ev)
	}
	return out, resp.NextPageToken, nil
}

// gcalTime es la fecha de un evento: `date` para los de todo el día y `dateTime`
// con su zona para el resto.
//
//nolint:tagliatelle // contrato de la API de Google Calendar.
type gcalTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// parse devuelve el instante, si es de todo el día y su zona horaria.
func (t gcalTime) parse() (time.Time, bool, string) {
	if t.Date != "" {
		ts, err := time.Parse("2006-01-02", t.Date)
		if err != nil {
			return time.Time{}, true, t.TimeZone
		}
		return ts.UTC(), true, t.TimeZone
	}
	ts, err := time.Parse(time.RFC3339, t.DateTime)
	if err != nil {
		return time.Time{}, false, t.TimeZone
	}
	return ts, false, t.TimeZone
}

// splitRecurrence separa las líneas iCalendar que Google entrega juntas. Los
// nombres son los del RFC 5545, así que viajan tal cual: reescribirlos sería
// inventar una traducción entre dos cosas que ya son la misma.
func splitRecurrence(lines []string) (rrule string, rdate, exdate []time.Time) {
	for _, raw := range lines {
		name, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		// El nombre puede traer parámetros (EXDATE;TZID=Europe/Madrid).
		if i := strings.Index(name, ";"); i >= 0 {
			name = name[:i]
		}
		switch strings.ToUpper(name) {
		case "RRULE":
			rrule = value
		case "RDATE":
			rdate = append(rdate, parseICalDates(value)...)
		case "EXDATE":
			exdate = append(exdate, parseICalDates(value)...)
		}
	}
	return rrule, rdate, exdate
}

// parseICalDates lee una lista de fechas iCalendar separadas por comas.
func parseICalDates(value string) []time.Time {
	out := make([]time.Time, 0, 2)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
			if ts, err := time.Parse(layout, part); err == nil {
				out = append(out, ts.UTC())
				break
			}
		}
	}
	return out
}

// gcalBase resuelve la raíz de la API (configurable para tests y soberanas).
func (g *gcal) gcalBase() (string, error) {
	return baseURLOr(g.gcalAPIBase, defaultGCalBase)
}

// --- Traducciones de vocabulario a iCalendar --------------------------------

func gcalStatus(s string) string {
	switch s {
	case "cancelled":
		return "CANCELLED"
	case "tentative":
		return "TENTATIVE"
	default:
		return "CONFIRMED"
	}
}

func gcalVisibility(v string) string {
	switch v {
	case "private", "confidential":
		return "PRIVATE"
	default:
		return "PUBLIC"
	}
}

func gcalPartStat(s string) string {
	switch s {
	case "accepted":
		return "ACCEPTED"
	case "declined":
		return "DECLINED"
	case "tentative":
		return "TENTATIVE"
	default:
		return "NEEDS-ACTION"
	}
}

func gcalRole(optional, resource bool) string {
	switch {
	case resource:
		return "NON-PARTICIPANT"
	case optional:
		return "OPT-PARTICIPANT"
	default:
		return "REQ-PARTICIPANT"
	}
}

func gcalCUType(resource bool) string {
	if resource {
		return "RESOURCE"
	}
	return "INDIVIDUAL"
}

var _ CalendarSource = (*gcal)(nil)

// compilación: el origen de calendario necesita la raíz configurable.
var _ = fmt.Sprintf
