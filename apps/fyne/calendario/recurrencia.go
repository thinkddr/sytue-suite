// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Las series: `buildRRule`/`parseRecurrence` de lib/ical.ts y la edición y el borrado
// POR ALCANCE de lib/recurrence.ts —«solo este», «este y los siguientes», «todos»—,
// sobre la API REST del servicio en vez de sobre CalDAV. El orden es el de la web:
// crear SIEMPRE antes de recortar el maestro, para que un fallo a mitad deje como mucho
// un duplicado, nunca una pérdida.

// Recurrencia es `Recurrence`: el subconjunto de RRULE que la interfaz crea y sabe
// leer (DAILY, WEEKLY, MONTHLY; INTERVAL; COUNT o UNTIL).
type Recurrencia struct {
	Freq      string
	Intervalo int
	Cuenta    int
	// Hasta es la fecha del UNTIL, «2026-12-31», o "".
	Hasta string
}

// Los tres alcances de ScopeDialog.
const (
	AlcanceEste       = "this"
	AlcanceSiguientes = "following"
	AlcanceTodos      = "all"
)

// LeerRRule es `parseRecurrence` sobre el valor de la RRULE: una FREQ que no sea de
// las tres, o cualquier BY*, no se sabe editar y el evento se trata como puntual.
func LeerRRule(s string) (Recurrencia, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "RRULE:")
	if s == "" {
		return Recurrencia{}, false
	}
	m := map[string]string{}
	for _, kv := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k != "" && v != "" {
			m[strings.ToUpper(k)] = strings.TrimSpace(v)
		}
	}
	r := Recurrencia{Freq: m["FREQ"]}
	if r.Freq != "DAILY" && r.Freq != "WEEKLY" && r.Freq != "MONTHLY" {
		return Recurrencia{}, false
	}
	for k := range m {
		if strings.HasPrefix(k, "BY") {
			return Recurrencia{}, false
		}
	}
	if n, _ := strconv.Atoi(m["INTERVAL"]); n > 1 {
		r.Intervalo = n
	}
	if n, _ := strconv.Atoi(m["COUNT"]); n > 0 {
		r.Cuenta = n
	}
	if u := m["UNTIL"]; len(u) >= 8 {
		r.Hasta = u[:4] + "-" + u[4:6] + "-" + u[6:8]
	}
	return r, true
}

// RRule es `buildRRule` sin el «RRULE:» delante, que el servicio no quiere.
func (r Recurrencia) RRule() string {
	partes := []string{"FREQ=" + r.Freq}
	if r.Intervalo > 1 {
		partes = append(partes, "INTERVAL="+strconv.Itoa(r.Intervalo))
	}
	switch {
	case r.Cuenta > 0:
		partes = append(partes, "COUNT="+strconv.Itoa(r.Cuenta))
	case r.Hasta != "":
		partes = append(partes, "UNTIL="+strings.ReplaceAll(r.Hasta, "-", "")+"T235959Z")
	}
	return strings.Join(partes, ";")
}

// inicioOcurrencia es `occurrenceStart`: la i-ésima desde el maestro, sin acumular.
func inicioOcurrencia(base time.Time, i int, freq string, intervalo int) time.Time {
	switch freq {
	case "DAILY":
		return base.AddDate(0, 0, i*intervalo)
	case "WEEKLY":
		return base.AddDate(0, 0, 7*i*intervalo)
	}
	return base.AddDate(0, i*intervalo, 0)
}

// contarAntes es `countBefore`: cuántas ocurrencias del maestro empiezan antes del
// corte, acotado por su COUNT.
func contarAntes(inicio time.Time, r Recurrencia, corte time.Time) int {
	intervalo := max(r.Intervalo, 1)
	tope := 1000
	if r.Cuenta > 0 {
		tope = r.Cuenta
	}
	n := 0
	for i := 0; i < tope; i++ {
		if !inicioOcurrencia(inicio, i, r.Freq, intervalo).Before(corte) {
			break
		}
		n++
	}
	return n
}

// arrastrarRegla es `carryRule`: la regla de la serie nueva de «este y los siguientes»,
// con el mismo patrón y el fin ajustado (lo que le quede de COUNT, o el mismo UNTIL).
func arrastrarRegla(maestra Recurrencia, inicioMaestro, corte time.Time) Recurrencia {
	r := Recurrencia{Freq: maestra.Freq, Intervalo: maestra.Intervalo}
	switch {
	case maestra.Cuenta > 0:
		r.Cuenta = max(1, maestra.Cuenta-contarAntes(inicioMaestro, maestra, corte))
	case maestra.Hasta != "":
		r.Hasta = maestra.Hasta
	}
	return r
}

// idOcurrencia es `recurrenceIso`: el RECURRENCE-ID de la ocurrencia. En un día
// completo, la medianoche UTC de su fecha, para que el servicio saque el día sin sesgo.
func idOcurrencia(o Ocurrencia) time.Time {
	if o.Event.AllDay {
		u := o.Start.UTC()
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	}
	return o.Start
}

// copiaContenido es `contentCopy`: los campos de CONTENIDO para un evento suelto, sin
// su serie.
func copiaContenido(in api.EventInput) api.EventInput {
	return api.EventInput{
		Summary: in.Summary, Description: in.Description, Location: in.Location,
		Start: in.Start, End: in.End, TZID: in.TZID, AllDay: in.AllDay,
		Color: in.Color, ConferenceURL: in.ConferenceURL, Attendees: in.Attendees, Send: in.Send,
	}
}

// EditarSerie es `applyRecurringEdit`: aplica lo editado de UNA ocurrencia `o` con el
// alcance elegido. `regla` es la del formulario (nil: se quitó la repetición).
func EditarSerie(ctx context.Context, c *api.Calendars, o Ocurrencia, alcance string, editado api.EventInput, regla *Recurrencia) error {
	maestro := o.Event
	switch alcance {
	case AlcanceTodos:
		// Un cambio de hora en «todos» DESPLAZA la serie: el mismo desfase se aplica al
		// inicio del maestro, no se ancla la serie a esta ocurrencia.
		in := editado
		delta := editado.Start.Sub(o.Start)
		duracion := editado.End.Sub(editado.Start)
		in.Start = maestro.DTStart.Add(delta)
		in.End = in.Start.Add(duracion)
		in.RRule = ""
		if regla != nil {
			in.RRule = regla.RRule()
		}
		in.RDate, in.ExDate = maestro.RDate, maestro.ExDate
		_, err := c.UpdateEvent(ctx, maestro.ID, in)
		return err
	case AlcanceSiguientes:
		in := copiaContenido(editado)
		if regla != nil {
			in.RRule = arrastrarRegla(*regla, maestro.DTStart, o.Start).RRule()
		}
		for _, ex := range maestro.ExDate {
			if !ex.Before(o.Start) {
				in.ExDate = append(in.ExDate, ex)
			}
		}
		if _, err := c.CreateEvent(ctx, o.CalendarioID, in); err != nil {
			return err
		}
		return c.AmendRecurrence(ctx, o.CalendarioID, maestro.UID, idOcurrencia(o), api.RecorteHasta)
	}
	if _, err := c.CreateEvent(ctx, o.CalendarioID, copiaContenido(editado)); err != nil {
		return err
	}
	return c.AmendRecurrence(ctx, o.CalendarioID, maestro.UID, idOcurrencia(o), api.RecorteExcluir)
}

// BorrarSerie es `applyRecurringDelete`.
func BorrarSerie(ctx context.Context, c *api.Calendars, o Ocurrencia, alcance string) error {
	switch alcance {
	case AlcanceTodos:
		return c.DeleteEvent(ctx, o.Event.ID)
	case AlcanceSiguientes:
		return c.AmendRecurrence(ctx, o.CalendarioID, o.Event.UID, idOcurrencia(o), api.RecorteHasta)
	}
	return c.AmendRecurrence(ctx, o.CalendarioID, o.Event.UID, idOcurrencia(o), api.RecorteExcluir)
}

// esSerie dice si la ocurrencia es de una serie que la interfaz sabe partir. Una regla
// que no sabe leer (BYDAY…) se edita como un evento puntual, como en la web.
func esSerie(o Ocurrencia) bool {
	_, ok := LeerRRule(o.Event.RRule)
	return ok
}
