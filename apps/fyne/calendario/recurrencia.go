// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"slices"
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
// leer (DAILY, WEEKLY, MONTHLY, YEARLY; INTERVAL; los días de una semanal; COUNT o
// UNTIL).
type Recurrencia struct {
	Freq      string
	Intervalo int
	// Dias es el BYDAY de una semanal («MO», «WE»), de lunes a domingo; nil, el día
	// del inicio.
	Dias   []string
	Cuenta int
	// Hasta es la fecha del UNTIL, «2026-12-31», o "".
	Hasta string
}

// DiasSemana son `WEEKDAYS`: los códigos BYDAY de lunes a domingo, el orden de las
// columnas y de `i18n.WeekdaysNarrow`.
var DiasSemana = []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// Laborables son `WORKWEEK`: «cada día laborable», de lunes a viernes.
var Laborables = []string{"MO", "TU", "WE", "TH", "FR"}

// normalizarDias es `normalizeWeekdays`: los días válidos, sin repetir y de lunes a
// domingo, para que el orden en que se eligen no cambie la regla.
func normalizarDias(dias []string) []string {
	var out []string
	for _, d := range DiasSemana {
		if slices.Contains(dias, d) {
			out = append(out, d)
		}
	}
	return out
}

// diaDe es el código BYDAY del día de la semana de `t`.
func diaDe(t time.Time) string { return DiasSemana[(int(t.Weekday())+6)%7] }

// EsLaborable dice si la regla es «cada día laborable»: semanal, cada semana, de lunes
// a viernes.
func (r Recurrencia) EsLaborable() bool {
	return r.Freq == "WEEKLY" && r.Intervalo <= 1 && slices.Equal(r.Dias, Laborables)
}

// Los tres alcances de ScopeDialog.
const (
	AlcanceEste       = "this"
	AlcanceSiguientes = "following"
	AlcanceTodos      = "all"
)

// LeerRRule es `parseRecurrence` sobre el valor de la RRULE: una FREQ que no sea de
// las cuatro, o un BY* que no sea el BYDAY sin ordinales de una semanal, no se sabe
// editar y el formulario la enseña como «Personalizada».
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
	if r.Freq != "DAILY" && r.Freq != "WEEKLY" && r.Freq != "MONTHLY" && r.Freq != "YEARLY" {
		return Recurrencia{}, false
	}
	for k := range m {
		if strings.HasPrefix(k, "BY") && (k != "BYDAY" || r.Freq != "WEEKLY") {
			return Recurrencia{}, false
		}
	}
	if n, _ := strconv.Atoi(m["INTERVAL"]); n > 1 {
		r.Intervalo = n
	}
	if v, ok := m["BYDAY"]; ok {
		dias, ok := leerDias(v, r.Intervalo, m["WKST"])
		if !ok {
			return Recurrencia{}, false
		}
		r.Dias = dias
	}
	if n, _ := strconv.Atoi(m["COUNT"]); n > 0 {
		r.Cuenta = n
	}
	if u := m["UNTIL"]; len(u) >= 8 {
		r.Hasta = u[:4] + "-" + u[4:6] + "-" + u[6:8]
	}
	return r, true
}

// leerDias es el BYDAY de una semanal que el formulario sabe editar: días sueltos, sin
// ordinales, ordenados de lunes a domingo.
func leerDias(byday string, intervalo int, wkst string) ([]string, bool) {
	dias := strings.Split(strings.ToUpper(byday), ",")
	for _, d := range dias {
		if !slices.Contains(DiasSemana, d) {
			return nil, false
		}
	}
	// Con INTERVAL > 1 la semana de cada salto depende de WKST, que el formulario no
	// guarda: rehacerla sin él movería las repeticiones.
	if w := strings.ToUpper(wkst); intervalo > 1 && w != "" && w != "MO" {
		return nil, false
	}
	return normalizarDias(dias), true
}

// RRule es `buildRRule` sin el «RRULE:» delante, que el servicio no quiere.
func (r Recurrencia) RRule() string {
	partes := []string{"FREQ=" + r.Freq}
	if r.Intervalo > 1 {
		partes = append(partes, "INTERVAL="+strconv.Itoa(r.Intervalo))
	}
	if dias := normalizarDias(r.Dias); r.Freq == "WEEKLY" && len(dias) > 0 {
		partes = append(partes, "BYDAY="+strings.Join(dias, ","))
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
	case "YEARLY":
		return base.AddDate(i*intervalo, 0, 0)
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
	if r.Freq == "WEEKLY" && len(r.Dias) > 0 {
		return contarAntesPorDias(inicio, r.Dias, intervalo, tope, corte)
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

// contarAntesPorDias es `countBeforeByDay`: una semanal con BYDAY como la expande el
// servicio, cada `intervalo` semanas (de lunes a domingo) desde la del inicio, los días
// elegidos en orden y sin contar los anteriores al inicio.
func contarAntesPorDias(inicio time.Time, dias []string, intervalo, tope int, corte time.Time) int {
	var desfases []int
	for i, d := range DiasSemana {
		if slices.Contains(dias, d) {
			desfases = append(desfases, i)
		}
	}
	if len(desfases) == 0 {
		return 0
	}
	lunes := inicio.AddDate(0, 0, -((int(inicio.Weekday()) + 6) % 7))
	n := 0
	for semana := 0; n < tope; semana += intervalo {
		for _, d := range desfases {
			t := lunes.AddDate(0, 0, semana*7+d)
			if t.Before(inicio) {
				continue
			}
			if !t.Before(corte) || n >= tope {
				return n
			}
			n++
		}
	}
	return n
}

// inicioEnSuZona es el DTSTART del maestro en la zona del evento, que es donde el
// servicio decide en qué día de la semana cae cada repetición: un lunes a las 00:30 en
// Madrid es domingo en UTC, y contar los BYDAY en UTC descuadraba la cuenta al partir.
func inicioEnSuZona(ev api.Event) time.Time {
	if ev.TZID != "" {
		if loc, err := time.LoadLocation(ev.TZID); err == nil {
			return ev.DTStart.In(loc)
		}
	}
	return ev.DTStart
}

// arrastrarRegla es `carryRule`: la regla de la serie nueva de «este y los siguientes»,
// con el mismo patrón y el fin ajustado (lo que le quede de COUNT, o el mismo UNTIL).
func arrastrarRegla(maestra Recurrencia, inicioMaestro, corte time.Time) Recurrencia {
	r := Recurrencia{Freq: maestra.Freq, Intervalo: maestra.Intervalo, Dias: slices.Clone(maestra.Dias)}
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
// alcance elegido. `regla` es la del formulario; nil es que el formulario no la
// entiende y la regla es la que ya trae `editado` (la personalizada, intacta, o ninguna
// si se quitó la repetición).
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
		if regla != nil {
			in.RRule = regla.RRule()
		}
		in.RDate, in.ExDate = maestro.RDate, maestro.ExDate
		_, err := c.UpdateEvent(ctx, maestro.ID, in)
		return err
	case AlcanceSiguientes:
		in := copiaContenido(editado)
		if regla != nil {
			in.RRule = arrastrarRegla(*regla, inicioEnSuZona(maestro), o.Start).RRule()
		} else {
			// Una personalizada se arrastra tal cual: su UNTIL vale para la serie nueva.
			// Con COUNT no se sabe cuántas quedan, y por eso alcancesDe no ofrece este
			// alcance.
			in.RRule = editado.RRule
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

// esSerie dice si la ocurrencia es de una serie, sepa o no editar su regla: también
// una «el segundo lunes» pide alcance, porque tratarla como puntual la convertiría en
// un evento suelto al guardar o al moverla.
func esSerie(o Ocurrencia) bool {
	return strings.TrimSpace(o.Event.RRule) != ""
}

// reglaCruda dice si la regla de la serie es una que la interfaz no sabe editar.
func reglaCruda(o Ocurrencia) bool {
	_, ok := LeerRRule(o.Event.RRule)
	return esSerie(o) && !ok
}

// alcancesDe son los alcances que se ofrecen para `o`. Partir una regla personalizada
// con COUNT en «este y los siguientes» exige contar sus ocurrencias según sus BY*, que
// la interfaz no calcula: antes que dejar la serie nueva con más o menos repeticiones,
// no se ofrece al editar. Borrar sí: solo corta el maestro y eso lo hace el servicio.
func alcancesDe(o Ocurrencia, borrar bool) []string {
	todos := []string{AlcanceEste, AlcanceSiguientes, AlcanceTodos}
	if borrar || !reglaCruda(o) {
		return todos
	}
	for _, kv := range strings.Split(strings.TrimPrefix(strings.TrimSpace(o.Event.RRule), "RRULE:"), ";") {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(strings.TrimSpace(k), "COUNT") {
			return []string{AlcanceEste, AlcanceTodos}
		}
	}
	return todos
}
