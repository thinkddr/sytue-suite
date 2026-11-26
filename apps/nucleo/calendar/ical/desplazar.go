// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// Mover la hora (o el día) de una serie entera.
//
// Guardar «todos» con otra hora o en otro día desplaza el DTSTART del maestro
// (`calendario.desplazarMaestro`): los mismos días de calendario y la misma diferencia de
// hora de pared, en la zona del evento. Sus EXDATE y los RECURRENCE-ID de sus excepciones
// nombran ocurrencias de la serie: si se quedaban en la hora de antes, la ocurrencia que
// se había quitado volvía a salir a la hora nueva y cada excepción dejaba de casar con la
// suya. Se desplazan igual que el inicio, cada valor escrito como venía (en su TZID, en
// UTC, flotante o como fecha). La pantalla desplaza las EXDATE, que manda en el evento
// (la edición conectada las guarda de ahí); aquí se desplazan las excepciones (CarryOver)
// y las EXDATE desplazadas conservan su forma (ConservarForma).

// desplazamiento es lo que se movió el inicio del maestro de una serie: días de
// calendario y hora de pared, leídos en la zona de antes y puestos en la de ahora.
type desplazamiento struct {
	dias         int
	pared        time.Duration
	antes, ahora *time.Location
}

// desplazamientoDe es lo que se movió el DTSTART del maestro `antes` al del maestro
// `ahora`. ok=false si a alguno le falta o no se deja leer, o si uno es una fecha y el
// otro una hora.
func desplazamientoDe(antes, ahora *ical.Component) (desplazamiento, bool) {
	pa, pn := antes.Props.Get(ical.PropDateTimeStart), ahora.Props.Get(ical.PropDateTimeStart)
	if pa == nil || pn == nil || esFecha(pa) != esFecha(pn) {
		return desplazamiento{}, false
	}
	ta, za, okA := primeraFecha(pa)
	tn, zn, okN := primeraFecha(pn)
	if !okA || !okN {
		return desplazamiento{}, false
	}
	return desplazamientoEntre(ta.In(za), tn.In(zn)), true
}

// desplazamientoEntre es lo que se movió un inicio de `a` a `n`, cada uno en su zona.
func desplazamientoEntre(a, n time.Time) desplazamiento {
	return desplazamiento{dias: diasEntre(a, n), pared: reloj(n) - reloj(a), antes: a.Location(), ahora: n.Location()}
}

// MoverConLaSerie son `fechas` (las EXDATE de una serie) movidas como su inicio, que pasa
// de `antes` a `ahora`, cada uno en la zona del evento (domain.Location): los mismos días
// de calendario y la misma diferencia de hora de pared, como las mueve la pantalla al
// guardar «todos» con otra hora. Es para quien guarda la serie sin ver sus EXDATE (la
// emulación de Google Calendar del servicio): en la hora de antes ya no nombraban ninguna
// ocurrencia, y la quitada volvía a la hora nueva. Van en UTC, como en las columnas.
func MoverConLaSerie(fechas []time.Time, antes, ahora time.Time) []time.Time {
	if len(fechas) == 0 {
		return fechas
	}
	d := desplazamientoEntre(antes, ahora)
	out := make([]time.Time, 0, len(fechas))
	for _, f := range fechas {
		out = append(out, d.aplicar(f).UTC())
	}
	return out
}

// nulo: no mueve nada (el mismo día, la misma hora de pared y la misma zona).
func (d desplazamiento) nulo() bool {
	return d.dias == 0 && d.pared == 0 && d.antes.String() == d.ahora.String()
}

// aplicar es t desplazado: su hora de pared en la zona de antes, con los días y la
// diferencia de hora de pared, en la zona de ahora (como `calendario.desplazarMaestro`).
func (d desplazamiento) aplicar(t time.Time) time.Time {
	x := t.In(d.antes)
	return time.Date(x.Year(), x.Month(), x.Day()+d.dias, x.Hour(), x.Minute(), x.Second(), x.Nanosecond()+int(d.pared), d.ahora)
}

// propiedad es una copia de p (una EXDATE, un RECURRENCE-ID, un DTSTART…) con cada una
// de sus fechas desplazada y escrita como venía. Lo que no se deja leer se queda.
func (d desplazamiento) propiedad(p ical.Prop) ical.Prop {
	q := copiaPropiedad(p)
	valores := strings.Split(p.Value, ",")
	for i, crudo := range valores {
		crudo = strings.TrimSpace(crudo)
		valores[i] = crudo
		t, _, ok := leerFecha(crudo, &p)
		if !ok {
			continue
		}
		if esValorDeFecha(crudo, &p) {
			// Una fecha solo cambia de día.
			valores[i] = enSuForma(t.AddDate(0, 0, d.dias), crudo, &p)
		} else {
			valores[i] = enSuForma(d.aplicar(t), crudo, &p)
		}
	}
	q.Value = strings.Join(valores, ",")
	return q
}

// excepcion es una copia de la excepción ex de la serie, desplazada con ella: su
// RECURRENCE-ID, siempre; su hora, si era la de su ocurrencia (nadie la cambió a
// propósito): pasa a la de la ocurrencia en la serie movida, con la duración nueva de la
// serie si tenía la de antes. La que estaba a otra hora a propósito se queda a su hora.
// La SEQUENCE sube a la del maestro `ahora`, si la tenía menor: cambia lo que ven los
// invitados. `antes` y `ahora` son el maestro de antes y el de después.
func (d desplazamiento) excepcion(ex, antes, ahora *ical.Component) *ical.Component {
	c := copiaComponente(ex)
	rid := c.Props.Get(ical.PropRecurrenceID)
	ini := c.Props.Get(ical.PropDateTimeStart)
	enSuHora := rid != nil && ini != nil && instanteSemantico(ini) == instanteSemantico(rid)
	if rid != nil {
		c.Props[ical.PropRecurrenceID] = []ical.Prop{d.propiedad(*rid)}
	}
	if enSuHora {
		d.moverHora(c, antes, ahora)
	}
	if s, ok := secuenciaDe(ahora); ok {
		if e, _ := secuenciaDe(c); e < s {
			c.Props[ical.PropSequence] = []ical.Prop{copiaPropiedad(*ahora.Props.Get(ical.PropSequence))}
		}
	}
	return c
}

// moverHora pone la excepción ex a la hora de su ocurrencia en la serie movida.
func (d desplazamiento) moverHora(ex, antes, ahora *ical.Component) {
	ini := *ex.Props.Get(ical.PropDateTimeStart)
	dur, okDur := duracionDe(ex)
	nuevoIni := d.propiedad(ini)
	ex.Props[ical.PropDateTimeStart] = []ical.Prop{nuevoIni}
	if !okDur {
		return
	}
	// La que duraba lo que la serie dura lo que la serie dura ahora.
	if da, ok := duracionDe(antes); ok && da == dur {
		if dn, ok := duracionDe(ahora); ok {
			dur = dn
		}
	}
	t, _, ok := primeraFecha(&nuevoIni)
	if !ok {
		return
	}
	if fin := ex.Props.Get(ical.PropDateTimeEnd); fin != nil {
		crudo := strings.TrimSpace(fin.Value)
		q := copiaPropiedad(*fin)
		q.Value = enSuForma(t.Add(dur), crudo, fin)
		ex.Props[ical.PropDateTimeEnd] = []ical.Prop{q}
		return
	}
	if dd, ok := propDuration(ex.Props); ok && dd != dur {
		// Con DURATION, el fin se escribe como el inicio.
		delete(ex.Props, ical.PropDuration)
		q := copiaPropiedad(nuevoIni)
		q.Name = ical.PropDateTimeEnd
		q.Value = enSuForma(t.Add(dur), strings.TrimSpace(nuevoIni.Value), &nuevoIni)
		ex.Props[ical.PropDateTimeEnd] = []ical.Prop{q}
	}
}

// duracionDe es lo que dura el VEVENT ev: de su DTSTART a su DTEND, o su DURATION.
// ok=false si no se sabe.
func duracionDe(ev *ical.Component) (time.Duration, bool) {
	pi := ev.Props.Get(ical.PropDateTimeStart)
	if pi == nil {
		return 0, false
	}
	ti, _, ok := primeraFecha(pi)
	if !ok {
		return 0, false
	}
	if pf := ev.Props.Get(ical.PropDateTimeEnd); pf != nil {
		tf, _, ok := primeraFecha(pf)
		return tf.Sub(ti), ok
	}
	return propDuration(ev.Props)
}

// esFecha: p es una fecha (VALUE=DATE o de 8 cifras), no una hora.
func esFecha(p *ical.Prop) bool {
	return esValorDeFecha(strings.TrimSpace(p.Value), p)
}

func esValorDeFecha(crudo string, p *ical.Prop) bool {
	return strings.EqualFold(p.Params.Get(ical.ParamValue), "DATE") || len(crudo) == 8
}

// primeraFecha es la primera fecha de p, con la zona en la que se lee su hora de pared.
func primeraFecha(p *ical.Prop) (time.Time, *time.Location, bool) {
	crudo, _, _ := strings.Cut(p.Value, ",")
	return leerFecha(strings.TrimSpace(crudo), p)
}

// leerFecha es el instante de crudo, una de las fechas de p, y la zona en la que se lee
// su hora de pared: la de su TZID (como la resuelve la expansión); UTC en UTC, flotante
// (que Calendar lee en UTC) o como fecha (a medianoche en UTC).
func leerFecha(crudo string, p *ical.Prop) (time.Time, *time.Location, bool) {
	var t time.Time
	var err error
	zona := time.UTC
	switch {
	case esValorDeFecha(crudo, p):
		t, err = time.Parse("20060102", crudo)
	case len(crudo) == 16 && strings.HasSuffix(crudo, "Z"):
		t, err = time.Parse("20060102T150405Z", crudo)
	case len(crudo) == 15:
		if tz := strings.TrimSpace(p.Params.Get(ical.ParamTimezoneID)); tz != "" {
			zona = domain.LoadLocationCached(tz)
		}
		t, err = time.ParseInLocation("20060102T150405", crudo, zona)
	default:
		return time.Time{}, nil, false
	}
	return t, zona, err == nil
}

// enSuForma es t escrito como crudo, una de las fechas de p: una fecha, en UTC, en la
// hora de su TZID o flotante.
func enSuForma(t time.Time, crudo string, p *ical.Prop) string {
	switch {
	case esValorDeFecha(crudo, p):
		return t.UTC().Format("20060102")
	case strings.HasSuffix(crudo, "Z"):
		return t.UTC().Format("20060102T150405Z")
	}
	zona := time.UTC
	if tz := strings.TrimSpace(p.Params.Get(ical.ParamTimezoneID)); tz != "" {
		zona = domain.LoadLocationCached(tz)
	}
	return t.In(zona).Format("20060102T150405")
}

// diasEntre son los días de calendario desde la fecha de a hasta la de b, cada una en
// su zona.
func diasEntre(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da) / (24 * time.Hour))
}

// reloj es la hora de pared de t desde su medianoche.
func reloj(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
}

// excepcionesQueSiguen son las excepciones de calAnterior (con el maestro `anterior`)
// que valen para la serie del maestro `nuevo`: todas tal cual si es la misma serie (la
// misma regla desde el mismo inicio); desplazadas como el inicio si se repite igual
// desde otro (la serie movida: `desplazamiento.excepcion`); ninguna con otra regla, o
// si deja de ser de fecha o de hora, porque apuntarían a ocurrencias que no existen.
func excepcionesQueSiguen(nuevo, anterior *ical.Component, calAnterior *ical.Calendar) []*ical.Component {
	regla := valorDe(nuevo, ical.PropRecurrenceRule)
	if regla == "" || regla != valorDe(anterior, ical.PropRecurrenceRule) {
		return nil
	}
	var d desplazamiento
	movida := valorDe(nuevo, ical.PropDateTimeStart) != valorDe(anterior, ical.PropDateTimeStart)
	if movida {
		var ok bool
		if d, ok = desplazamientoDe(anterior, nuevo); !ok {
			return nil
		}
		movida = !d.nulo()
	}
	var out []*ical.Component
	for _, hijo := range calAnterior.Children {
		if hijo.Name != ical.CompEvent || hijo.Props.Get(ical.PropRecurrenceID) == nil {
			continue
		}
		if movida {
			hijo = d.excepcion(hijo, anterior, nuevo)
		}
		out = append(out, hijo)
	}
	return out
}

// conLasDesplazadas es `de` para conservarFechas con las EXDATE: con la serie movida,
// sus EXDATE y además las mismas desplazadas como el inicio, para que las que la
// pantalla movió con la serie sigan escritas como estaban (en la hora de su zona).
func conLasDesplazadas(mN, mA *ical.Component) *ical.Component {
	d, ok := desplazamientoDe(mA, mN)
	if !ok || d.nulo() {
		return mA
	}
	c := ical.NewComponent(mA.Name)
	ps := copiaPropiedades(mA.Props[ical.PropExceptionDates])
	for _, p := range mA.Props[ical.PropExceptionDates] {
		ps = append(ps, d.propiedad(p))
	}
	c.Props[ical.PropExceptionDates] = ps
	return c
}
