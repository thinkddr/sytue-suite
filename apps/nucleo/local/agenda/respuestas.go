// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	calical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Las RESPUESTAS DE LOS INVITADOS que llegan por correo (iMIP REPLY, RFC 5546 §3.2.3; A-11)
// a lo que organiza la persona en un calendario de una cuenta CalDAV que NO planifica: ahí
// la invitación sale por correo ([Cartero]) y cada invitado contesta por correo a Mail.
// Nadie más las aplica (el servidor no planifica), así que Mail las deja en su cola
// (`correo.RespuestasPorAplicar`) y Calendar, al abrirse y en cada vuelta, las pone aquí:
// el PARTSTAT del invitado en el evento de este equipo, que la sincronización sube con
// él. No manda nada a nadie: la firma de la invitación ([firma]) solo cambia en las
// respuestas, y el Cartero no avisa por eso.
//
// Se aplica una respuesta si el evento con su UID está en una de esas colecciones, lo
// organiza la persona (la dirección que dio el proveedor, el usuario de la cuenta o la de
// una cuenta de Mail) y es el ORGANIZER del REPLY, el invitado está en la lista y la
// respuesta no es de una versión anterior (SEQUENCE) ni más vieja que la última aplicada
// de ese invitado (DTSTAMP, en `respuestas.json` junto al estado de las cuentas). Una
// respuesta a UNA ocurrencia (RECURRENCE-ID) se pone en su excepción si ya la hay; si no,
// se CREA la excepción de esa ocurrencia (una copia del maestro en su hora, sin la
// repetición) con el PARTSTAT del invitado. Crearla cambia lo que ven los invitados (la
// [firma]), así que el cambio se marca como venido de una respuesta
// ([Sincronizador.ponerDeRespuesta]): la vuelta lo sube y no manda nada a nadie, ni el
// [Cartero] ni su cola. Si la persona cambia después el evento, lo que mande será por su
// cambio, y llevará también la excepción.
//
// En Google, Microsoft o un CalDAV que planifica, el proveedor aplica él la respuesta y la
// trae la sincronización: aquí se descarta (aplicarla también chocaría con la suya).

// EstadoRespuesta es lo que pasó con una respuesta de la cola.
type EstadoRespuesta int

const (
	// RespuestaAplicada: puesta en su evento (o ya lo estaba). Se quita de la cola.
	RespuestaAplicada EstadoRespuesta = iota
	// RespuestaDescartada: no es de nada que se aplique aquí. Se quita de la cola.
	RespuestaDescartada
	// RespuestaEspera: su evento aún no está en este equipo, o cambió mientras se
	// aplicaba. Se vuelve a intentar.
	RespuestaEspera
)

// respuestaITIP es la de un invitado leída de un VEVENT de un REPLY.
type respuestaITIP struct {
	uid         string
	recurrencia *time.Time // nil: la serie (o el evento suelto)
	organizador string
	asistente   string
	estado      string
	secuencia   int
	sello       time.Time
}

func (r respuestaITIP) clave() string {
	rid := ""
	if r.recurrencia != nil {
		rid = r.recurrencia.UTC().Format(time.RFC3339)
	}
	return r.uid + "|" + rid + "|" + r.asistente
}

// leerRespuestas lee un VCALENDAR METHOD:REPLY: una respuesta por VEVENT con su ATTENDEE.
func leerRespuestas(ics []byte) ([]respuestaITIP, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(ics)).Decode()
	if err != nil {
		return nil, err
	}
	if p := cal.Props.Get(ical.PropMethod); p == nil || !strings.EqualFold(strings.TrimSpace(p.Value), "REPLY") {
		return nil, errors.New("agenda: no es un REPLY")
	}
	var out []respuestaITIP
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		texto := func(n string) string { t, _ := ev.Props.Text(n); return strings.TrimSpace(t) }
		r := respuestaITIP{uid: texto(ical.PropUID)}
		if p := ev.Props.Get(ical.PropOrganizer); p != nil {
			r.organizador = direccionICS(p.Value)
		}
		// SEQUENCE es un entero: `Props.Text` no lo lee (daba siempre 0).
		r.secuencia, _ = strconv.Atoi(strings.TrimSpace(valorDe(ev, ical.PropSequence)))
		if p := ev.Props.Get(ical.PropDateTimeStamp); p != nil {
			r.sello, _ = p.DateTime(time.UTC)
		}
		if p := ev.Props.Get(ical.PropRecurrenceID); p != nil {
			t, err := p.DateTime(time.UTC)
			if err != nil {
				continue
			}
			r.recurrencia = &t
		}
		for _, a := range ev.Props.Values(ical.PropAttendee) {
			x := r
			x.asistente = direccionICS(a.Value)
			x.estado = strings.ToUpper(strings.TrimSpace(a.Params.Get(ical.ParamParticipationStatus)))
			if x.uid != "" && x.organizador != "" && x.asistente != "" {
				out = append(out, x)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("agenda: el REPLY no trae ninguna respuesta")
	}
	return out, nil
}

// resultadoAplicar es lo que pasó al poner una respuesta en los datos de un evento.
type resultadoAplicar int

const (
	aplicadaCambio resultadoAplicar = iota // los datos cambian
	aplicadaIgual                          // ya estaba así
	noAplica                               // otro organizador, invitado que no está, versión vieja, una ocurrencia que no hay
)

// aplicarRespuesta pone la respuesta r en los datos de un evento organizado por la
// persona (propia dice si una dirección es suya). RSVP se queda: es lo que quien organiza
// pidió. La de una ocurrencia sin excepción la crea ([excepcionDe]), si de verdad es una
// ocurrencia de la serie y la respuesta cambia algo en ella.
func aplicarRespuesta(datos string, r respuestaITIP, propia func(string) bool) (string, resultadoAplicar) {
	if !respuestaValida(r.estado) {
		return datos, noAplica
	}
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return datos, noAplica
	}
	maestro := principal(cal)
	if maestro == nil {
		return datos, noAplica
	}
	org := ""
	if p := maestro.Props.Get(ical.PropOrganizer); p != nil {
		org = direccionICS(p.Value)
	}
	if org == "" || org != r.organizador || !propia(org) {
		return datos, noAplica
	}
	var destinos []*ical.Component
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		rid := ev.Props.Get(ical.PropRecurrenceID)
		switch {
		case r.recurrencia == nil:
			destinos = append(destinos, ev) // la serie entera: el maestro y sus excepciones
		case rid != nil:
			if t, err := rid.DateTime(time.UTC); err == nil && t.Equal(*r.recurrencia) {
				destinos = append(destinos, ev)
			}
		}
	}
	if r.recurrencia != nil && len(destinos) == 0 {
		// Sin excepción para esa ocurrencia: se crea, como la haría quien organiza al
		// cambiar solo esa (la copia del maestro en su hora). Si no es una ocurrencia de
		// la serie (quitada con EXDATE, fuera de la regla), no hay nada que contestar.
		ex := excepcionDe(maestro, *r.recurrencia)
		if ex == nil {
			return datos, noAplica
		}
		cal.Children = append(cal.Children, ex)
		destinos = []*ical.Component{ex}
	}
	cambio, esta := ponerPartstat(destinos, r)
	switch {
	case !esta:
		return datos, noAplica
	case !cambio:
		return datos, aplicadaIgual
	}
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return datos, noAplica
	}
	return b.String(), aplicadaCambio
}

// ponerPartstat pone el PARTSTAT de r al asistente en los destinos que no son de una
// versión posterior a la que contesta. esta dice si el asistente estaba en alguno, y
// cambio, si cambió algo.
func ponerPartstat(destinos []*ical.Component, r respuestaITIP) (cambio, esta bool) {
	for _, ev := range destinos {
		sec, _ := strconv.Atoi(strings.TrimSpace(cmp.Or(valorDe(ev, ical.PropSequence), "0")))
		if r.secuencia < sec {
			continue // contesta a una versión anterior: la de ahora espera otra respuesta
		}
		ats := ev.Props[ical.PropAttendee]
		for i := range ats {
			if direccionICS(ats[i].Value) != r.asistente {
				continue
			}
			esta = true
			if !strings.EqualFold(ats[i].Params.Get(ical.ParamParticipationStatus), r.estado) {
				ats[i].Params.Set(ical.ParamParticipationStatus, r.estado)
				cambio = true
			}
		}
	}
	return cambio, esta
}

func valorDe(c *ical.Component, n string) string {
	if p := c.Props.Get(n); p != nil {
		return p.Value
	}
	return ""
}

// estadoRespuestas es lo que se recuerda de las respuestas aplicadas: el DTSTAMP de la
// última de cada invitado en cada evento (y ocurrencia).
type estadoRespuestas struct {
	Sellos map[string]time.Time `json:"sellos"`
}

// guardaSellos es cuánto se recuerda la última respuesta aplicada de un invitado.
const guardaSellos = 400 * 24 * time.Hour

// AplicarRespuestas pone en sus eventos las respuestas de los invitados que llegaron por
// correo (cada una, un VCALENDAR METHOD:REPLY tal como llegó) y dice qué pasó con cada
// una, en su orden. Las de varias se aplican por su DTSTAMP, de la más vieja a la más
// nueva. Lo que cambia aquí sube en la siguiente sincronización. Sin red: solo lee y
// escribe en local y en dir (`<Datos>/calendar/agenda`).
func (r *Registro) AplicarRespuestas(ctx context.Context, local Local, dir string, replies [][]byte) []EstadoRespuesta {
	p := &paseRespuestas{local: local, cache: map[string][]ObjetoLocal{}, estados: make([]EstadoRespuesta, len(replies)), hecho: make([]bool, len(replies))}
	for i := range p.estados {
		p.estados[i] = RespuestaDescartada
	}
	// sincDe es el sincronizador de cada colección de aquí: el que sube lo que cambie.
	sincDe := r.sincronizadoresPorColeccion(local, dir)
	// Las colecciones que mandan sus invitaciones por correo, y las demás de las cuentas.
	propias, porCorreo, otras := r.coleccionesRespuesta(local, dir)
	if len(porCorreo) == 0 {
		return p.estados // ningún calendario donde la respuesta vuelva por correo
	}
	propia := func(d string) bool { return propias[d] }

	entradas := leerTodas(replies)
	ruta := filepath.Join(dir, "respuestas.json")
	var st estadoRespuestas
	_ = espacio.LeerJSON(ruta, &st)
	if st.Sellos == nil {
		st.Sellos = map[string]time.Time{}
	}
	cambiado := false
	for _, e := range entradas {
		if ctx.Err() != nil {
			p.peor(e.i, RespuestaEspera)
			continue
		}
		col, j, ok := p.buscar(ctx, porCorreo, e.r.uid)
		if !ok {
			if _, _, enOtra := p.buscar(ctx, otras, e.r.uid); enOtra {
				p.peor(e.i, RespuestaDescartada) // la aplica su proveedor
			} else {
				p.peor(e.i, RespuestaEspera) // aún no ha bajado a este equipo
			}
			continue
		}
		if ultima, ok := st.Sellos[e.r.clave()]; ok && !e.r.sello.IsZero() && !e.r.sello.After(ultima) {
			p.peor(e.i, RespuestaDescartada) // ya se aplicó una igual o más nueva
			continue
		}
		o := p.cache[col][j]
		nuevo, res := aplicarRespuesta(o.Datos, e.r, propia)
		switch res {
		case noAplica:
			p.peor(e.i, RespuestaDescartada)
			continue
		case aplicadaIgual:
			// Ya estaba así: cuenta como aplicada.
		case aplicadaCambio:
			puesto, err := ponerRespuesta(ctx, sincDe[col], local, col, o, nuevo)
			if err != nil {
				p.peor(e.i, RespuestaEspera) // cambió aquí mientras tanto: la próxima vez
				continue
			}
			p.cache[col][j] = puesto
		}
		if !e.r.sello.IsZero() {
			st.Sellos[e.r.clave()] = e.r.sello
			cambiado = true
		}
		p.peor(e.i, RespuestaAplicada)
	}
	if cambiado {
		st.podar(time.Now().Add(-guardaSellos))
		_ = espacio.EscribirJSON(ruta, st)
	}
	return p.estados
}

// sincronizadoresPorColeccion es el sincronizador de cada colección de calendario.
func (r *Registro) sincronizadoresPorColeccion(local Local, dir string) map[string]*Sincronizador {
	sincDe := map[string]*Sincronizador{}
	for _, sc := range r.Sincronizadores(Calendario, local, dir, "") {
		for _, c := range sc.Colecciones() {
			sincDe[c.ID] = sc
		}
	}
	return sincDe
}

// coleccionesRespuesta reparte las colecciones de calendario de las cuentas entre las
// que mandan sus invitaciones por correo y las demás, y junta las direcciones propias:
// las de las cuentas y, si hay alguna por correo, las de los buzones de Mail.
func (r *Registro) coleccionesRespuesta(local Local, dir string) (propias map[string]bool, porCorreo, otras []string) {
	propias = map[string]bool{}
	for _, g := range r.Grupos(Calendario, local, dir) {
		for _, d := range []string{r.Direccion(g.Cuenta.ID), strings.ToLower(strings.TrimSpace(g.Cuenta.Usuario))} {
			if strings.Contains(d, "@") {
				propias[d] = true
			}
		}
		for _, c := range g.Colecciones {
			if !c.Planifica && !c.SoloLectura {
				porCorreo = append(porCorreo, c.ID)
			} else {
				otras = append(otras, c.ID)
			}
		}
	}
	if len(porCorreo) == 0 || r.Correo == nil {
		return propias, porCorreo, otras
	}
	if bs, err := r.Correo.Buzones(); err == nil {
		for _, b := range bs {
			propias[strings.ToLower(strings.TrimSpace(b.Direccion))] = true
		}
	}
	return propias, porCorreo, otras
}

// respuestaLeida es una de las respuestas que trae el REPLY i.
type respuestaLeida struct {
	i int
	r respuestaITIP
}

// leerTodas lee las respuestas de todos los REPLY, de la más vieja a la más nueva por su
// DTSTAMP. Un REPLY que no se lee no da ninguna.
func leerTodas(replies [][]byte) []respuestaLeida {
	var entradas []respuestaLeida
	for i, ics := range replies {
		rs, err := leerRespuestas(ics)
		if err != nil {
			continue
		}
		for _, x := range rs {
			entradas = append(entradas, respuestaLeida{i, x})
		}
	}
	slices.SortStableFunc(entradas, func(a, b respuestaLeida) int { return a.r.sello.Compare(b.r.sello) })
	return entradas
}

// paseRespuestas es lo que comparte una pasada de AplicarRespuestas: los objetos de cada
// colección (leídos una sola vez) y lo que va pasando con cada REPLY.
type paseRespuestas struct {
	local   Local
	cache   map[string][]ObjetoLocal
	estados []EstadoRespuesta
	hecho   []bool
}

// objetos son los de una colección, leídos la primera vez que se piden.
func (p *paseRespuestas) objetos(ctx context.Context, col string) []ObjetoLocal {
	if lista, ok := p.cache[col]; ok {
		return lista
	}
	lista, _ := p.local.Objetos(ctx, col)
	p.cache[col] = lista
	return lista
}

// buscar es la colección (de cols) y la posición del evento con ese UID.
func (p *paseRespuestas) buscar(ctx context.Context, cols []string, uid string) (string, int, bool) {
	for _, col := range cols {
		for j, o := range p.objetos(ctx, col) {
			if limpiarUID(o.UID) == limpiarUID(uid) {
				return col, j, true
			}
		}
	}
	return "", 0, false
}

// peor junta lo de varias respuestas de un mismo REPLY: si una espera, espera; si no,
// si alguna se aplicó, aplicado.
func (p *paseRespuestas) peor(i int, e EstadoRespuesta) {
	switch {
	case !p.hecho[i], e == RespuestaEspera, e == RespuestaAplicada && p.estados[i] == RespuestaDescartada:
		p.estados[i] = e
	}
	p.hecho[i] = true
}

// ponerRespuesta guarda el evento con la respuesta puesta. Por su sincronizador, si lo
// tiene, marcado como venido de una respuesta: la vuelta lo sube sin mandar nada.
func ponerRespuesta(ctx context.Context, sc *Sincronizador, local Local, col string, o ObjetoLocal, nuevo string) (ObjetoLocal, error) {
	if sc != nil {
		return sc.ponerDeRespuesta(ctx, col, o, nuevo)
	}
	return local.Poner(ctx, col, o.ID, o.Datos, nuevo)
}

// podar olvida los sellos de antes de limite.
func (st *estadoRespuestas) podar(limite time.Time) {
	for k, t := range st.Sellos {
		if t.Before(limite) {
			delete(st.Sellos, k)
		}
	}
}

// excepcionDe es la excepción de la ocurrencia rid de la serie del maestro, como la
// escribiría quien organiza al cambiar solo esa: una copia del maestro (con sus
// invitados, su SEQUENCE y sus avisos) en la hora de la ocurrencia, con RECURRENCE-ID y
// sin la repetición. nil si el maestro no se repite o rid no es una de sus ocurrencias
// (quitada con EXDATE, fuera de la regla o de otra hora).
func excepcionDe(maestro *ical.Component, rid time.Time) *ical.Component {
	if !esOcurrencia(maestro, rid) {
		return nil
	}
	ini := maestro.Props.Get(ical.PropDateTimeStart)
	inicio, err := ini.DateTime(time.UTC)
	if err != nil {
		return nil
	}
	ex := ical.NewComponent(ical.CompEvent)
	for nombre, ps := range maestro.Props {
		switch nombre {
		case ical.PropRecurrenceRule, ical.PropRecurrenceDates, ical.PropExceptionDates, ical.PropRecurrenceID,
			ical.PropDateTimeStart, ical.PropDateTimeEnd:
			continue
		}
		for _, p := range ps {
			ex.Props.Add(copiaProp(&p))
		}
	}
	for _, hijo := range maestro.Children {
		ex.Children = append(ex.Children, copiaComponente(hijo))
	}
	enSuHora := func(p *ical.Prop, nombre string, t time.Time) *ical.Prop {
		q := copiaProp(p)
		q.Name = nombre
		q.Value = formatoComo(p, t)
		return q
	}
	ex.Props.Set(enSuHora(ini, ical.PropRecurrenceID, rid))
	ex.Props.Set(enSuHora(ini, ical.PropDateTimeStart, rid))
	if fin := maestro.Props.Get(ical.PropDateTimeEnd); fin != nil {
		if f, err := fin.DateTime(time.UTC); err == nil {
			ex.Props.Set(enSuHora(fin, ical.PropDateTimeEnd, rid.Add(f.Sub(inicio))))
		}
	}
	return ex
}

// esOcurrencia dice si rid es el inicio de una ocurrencia de la serie del maestro, con
// la expansión de Calendar (`event.ExpandEvents`: RRULE, RDATE y EXDATE, en la zona del
// evento).
func esOcurrencia(maestro *ical.Component, rid time.Time) bool {
	solo := ical.NewCalendar()
	solo.Props.SetText(ical.PropVersion, "2.0")
	solo.Props.SetText(ical.PropProductID, "-//Sytue//Sytue Office Calendar//ES")
	solo.Children = []*ical.Component{maestro}
	var b bytes.Buffer
	if err := ical.NewEncoder(&b).Encode(solo); err != nil {
		return false
	}
	po, err := calical.DecodeCalendarObject(b.String(), "")
	if err != nil {
		return false
	}
	m := po.Master
	ev := event.Event{
		UID: m.UID, DTStart: m.DTStart, DTEnd: m.DTEnd, TZID: m.TZID, AllDay: m.AllDay,
		RRule: m.RRule, RDate: m.RDate, ExDate: m.ExDate,
	}
	if !ev.IsRecurring() {
		return false
	}
	for _, i := range event.ExpandEvents([]event.Event{ev}, rid.Add(-time.Second), rid.Add(time.Second)) {
		if i.Start.Equal(rid) {
			return true
		}
	}
	return false
}

// formatoComo escribe t como está escrita p: una fecha (VALUE=DATE), una hora en su
// TZID, en UTC («Z») o flotante.
func formatoComo(p *ical.Prop, t time.Time) string {
	switch {
	case p.ValueType() == ical.ValueDate:
		return t.UTC().Format("20060102")
	case p.Params.Get(ical.ParamTimezoneID) != "":
		if loc, err := time.LoadLocation(p.Params.Get(ical.ParamTimezoneID)); err == nil {
			return t.In(loc).Format("20060102T150405")
		}
		return t.UTC().Format("20060102T150405")
	case strings.HasSuffix(strings.TrimSpace(p.Value), "Z"):
		return t.UTC().Format("20060102T150405Z")
	default:
		// Flotante: go-ical la lee en la zona que se le pide (UTC, aquí).
		return t.UTC().Format("20060102T150405")
	}
}

func copiaProp(p *ical.Prop) *ical.Prop {
	q := ical.NewProp(p.Name)
	q.Value = p.Value
	for k, vs := range p.Params {
		q.Params[k] = slices.Clone(vs)
	}
	return q
}

func copiaComponente(c *ical.Component) *ical.Component {
	out := ical.NewComponent(c.Name)
	for _, ps := range c.Props {
		for _, p := range ps {
			out.Props.Add(copiaProp(&p))
		}
	}
	for _, h := range c.Children {
		out.Children = append(out.Children, copiaComponente(h))
	}
	return out
}
