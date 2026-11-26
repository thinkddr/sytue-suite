// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-ical"
)

// Guardar desde el formulario sin reescribir lo que no cambió (Sytue Office).
//
// El formulario manda el evento ENTERO y EncodeEvent lo vuelve a escribir a su manera:
// la EXDATE en UTC aunque viniera en la hora de su zona, `STATUS:CONFIRMED`,
// `CLASS:PUBLIC` y `TRANSP:OPAQUE` aunque no los trajera, el fin con DTEND aunque
// viniera con DURATION, la regla en su orden, cada invitado con `RSVP=FALSE` y
// `CUTYPE=INDIVIDUAL`, los recordatorios con su texto… Lo que vino de la cuenta escrito
// de otra forma se subía cambiado sin que la persona hubiera tocado nada. ConservarForma
// deja cada cosa que NO cambió como estaba; lo que cambió, como lo escribe el
// formulario.

// ConservarForma devuelve el recurso `nuevo` —lo que regenera el formulario
// (EncodeEvent y ConRecordatorios)— con lo que significa lo mismo que en `anterior` (lo
// guardado) escrito como en `anterior`, en el VEVENT maestro: el inicio (el mismo
// instante en la misma zona), el fin, la regla, RDATE y EXDATE, el título, el lugar, la
// descripción, el organizador, el estado, la clase, la transparencia, el color, la
// videollamada, cada invitado (con su papel, su respuesta, su tipo, su RSVP y su
// nombre) y los recordatorios editables (los mismos minutos). Va ANTES de CarryOver:
// así una serie cuya regla escribe el formulario en otro orden sigue siendo la misma y
// conserva sus excepciones. Sin `anterior` (un evento nuevo), o si no se deja leer,
// vuelve `nuevo`.
func ConservarForma(nuevo, anterior string) (string, error) {
	if strings.TrimSpace(anterior) == "" {
		return nuevo, nil
	}
	calNuevo, err := ical.NewDecoder(strings.NewReader(nuevo)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso nuevo: %w", err)
	}
	calAnterior, err := ical.NewDecoder(strings.NewReader(anterior)).Decode()
	if err != nil {
		return nuevo, nil //nolint:nilerr // lo guardado ilegible no puede bloquear la edición
	}
	mN, mA := masterComponent(calNuevo), masterComponent(calAnterior)
	if mN == nil || mA == nil {
		return nuevo, nil
	}
	conservarGrupos(mN, mA)
	conservarAsistentes(mN, mA)
	conservarAvisos(mN, mA)
	conservarSecuencia(mN, mA)
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(calNuevo); err != nil {
		return "", fmt.Errorf("ical: re-serializar el recurso: %w", err)
	}
	return buf.String(), nil
}

// conservarGrupos deja en el maestro nuevo mN, escrito como en el guardado mA, cada
// grupo de gruposDeLaForma que significa lo mismo en los dos.
func conservarGrupos(mN, mA *ical.Component) {
	inicio, fin := gruposDeLaForma[0], gruposDeLaForma[1]
	mismoInicio, mismoFin := mismoGrupo(mN, mA, inicio), mismoGrupo(mN, mA, fin)
	for _, grupo := range gruposDeLaForma {
		switch {
		case grupo[0] == inicio[0] && mismoInicio:
			// Un inicio flotante con un fin que ya no lo es mezclaría las dos formas.
			if !flotante(mA.Props.Get(ical.PropDateTimeStart)) || mismoFin {
				copiarGrupo(mN, mA, grupo)
			}
		case grupo[0] == fin[0] && mismoFin:
			// Una DURATION cuenta desde el inicio: con otro inicio, ya no es el mismo fin.
			if mA.Props.Get(ical.PropDuration) == nil && !flotante(mA.Props.Get(ical.PropDateTimeEnd)) || mismoInicio {
				copiarGrupo(mN, mA, grupo)
			}
		case grupo[0] == inicio[0] || grupo[0] == fin[0]:
			// Cambiado: como lo escribe el formulario.
		case mismoGrupo(mN, mA, grupo):
			copiarGrupo(mN, mA, grupo)
		case grupo[0] == ical.PropExceptionDates:
			// Con la serie movida, la pantalla manda sus EXDATE movidas con ella.
			conservarFechas(mN, conLasDesplazadas(mN, mA), grupo[0])
		case grupo[0] == ical.PropRecurrenceDates:
			conservarFechas(mN, mA, grupo[0])
		}
	}
}

// gruposDeLaForma son las propiedades del maestro que se comparan por lo que significan,
// en grupos que van juntos (el fin es DTEND o DURATION; el color, COLOR o el de Apple).
var gruposDeLaForma = [][]string{
	{ical.PropDateTimeStart},
	{ical.PropDateTimeEnd, ical.PropDuration},
	{ical.PropRecurrenceRule},
	{ical.PropRecurrenceDates},
	{ical.PropExceptionDates},
	{ical.PropSummary},
	{ical.PropLocation},
	{ical.PropDescription},
	{ical.PropOrganizer},
	{ical.PropStatus},
	{ical.PropClass},
	{ical.PropTransparency},
	{"COLOR", "X-APPLE-CALENDAR-COLOR"},
	{"CONFERENCE"},
}

// claveDe es lo que significa el grupo de propiedades en el VEVENT ev. El inicio lleva
// siempre su zona: aunque para los invitados sea la misma hora, la persona la eligió.
func claveDe(ev *ical.Component, grupo []string) (string, bool) {
	n := grupo[0]
	if n == ical.PropDateTimeStart {
		p := ev.Props.Get(n)
		if p == nil {
			return "", false
		}
		if p.ValueType() == ical.ValueDate {
			return instanteSemantico(p), true
		}
		return instanteSemantico(p) + "@" + zonaSemantica(p), true
	}
	return valorSemantico(ev, n)
}

// flotante: p es una hora sin zona (ni TZID ni «Z»).
func flotante(p *ical.Prop) bool {
	if p == nil || p.ValueType() == ical.ValueDate {
		return false
	}
	v := strings.TrimSpace(p.Value)
	return len(v) == 15 && p.Params.Get(ical.ParamTimezoneID) == ""
}

// mismoGrupo: el grupo significa lo mismo en los dos VEVENT (o ninguno lo tiene).
func mismoGrupo(a, b *ical.Component, grupo []string) bool {
	va, okA := claveDe(a, grupo)
	vb, okB := claveDe(b, grupo)
	return okA == okB && va == vb
}

// conservarFechas: si `a` trae otras RDATE o EXDATE que `de`, las que siguen se quedan
// en sus líneas de `de` (en su zona) y solo las nuevas van como las escribe `a`.
func conservarFechas(a, de *ical.Component, nombre string) {
	nuevas := map[string]bool{}
	for i := range a.Props[nombre] {
		for _, v := range strings.Split(fechasSemanticas(a.Props[nombre][i:i+1]), ",") {
			nuevas[v] = true
		}
	}
	var quedan []ical.Prop
	for _, p := range de.Props[nombre] {
		var valores []string
		for _, crudo := range strings.Split(p.Value, ",") {
			q := copiaPropiedad(p)
			q.Value = strings.TrimSpace(crudo)
			if v := fechasSemanticas([]ical.Prop{q}); v != "" && nuevas[v] {
				valores = append(valores, q.Value)
				delete(nuevas, v)
			}
		}
		if len(valores) > 0 {
			q := copiaPropiedad(p)
			q.Value = strings.Join(valores, ",")
			quedan = append(quedan, q)
		}
	}
	for _, p := range a.Props[nombre] {
		var valores []string
		for _, crudo := range strings.Split(p.Value, ",") {
			q := copiaPropiedad(p)
			q.Value = strings.TrimSpace(crudo)
			if v := fechasSemanticas([]ical.Prop{q}); nuevas[v] {
				valores = append(valores, q.Value)
				delete(nuevas, v)
			}
		}
		if len(valores) > 0 {
			q := copiaPropiedad(p)
			q.Value = strings.Join(valores, ",")
			quedan = append(quedan, q)
		}
	}
	if len(quedan) > 0 {
		a.Props[nombre] = quedan
	} else {
		delete(a.Props, nombre)
	}
}

// copiarGrupo deja en `a` las propiedades del grupo tal como están en `de`.
func copiarGrupo(a, de *ical.Component, grupo []string) {
	for _, n := range grupo {
		if ps := de.Props[n]; len(ps) > 0 {
			a.Props[n] = copiaPropiedades(ps)
		} else {
			delete(a.Props, n)
		}
	}
}

func copiaPropiedades(ps []ical.Prop) []ical.Prop {
	out := make([]ical.Prop, 0, len(ps))
	for _, p := range ps {
		out = append(out, copiaPropiedad(p))
	}
	return out
}

// asistenteSemantico es lo que significa un ATTENDEE, salvo su respuesta, con los
// valores de RFC 5545 de lo que no trae.
func asistenteSemantico(p ical.Prop) string {
	tipo := strings.ToUpper(strings.TrimSpace(p.Params.Get(ical.ParamCalendarUserType)))
	if tipo == "" {
		tipo = "INDIVIDUAL"
	}
	return strings.Join([]string{
		cleanCalAddress(strings.ToLower(strings.TrimSpace(p.Value))), rolDe(p), tipo,
		strconv.FormatBool(strings.EqualFold(strings.TrimSpace(p.Params.Get(ical.ParamRSVP)), "TRUE")),
		strings.TrimSpace(p.Params.Get(ical.ParamCommonName)),
	}, "\x01")
}

// conservarAsistentes deja como estaba en `de` cada invitado de `a` que significa lo
// mismo (con la respuesta que trae `a`, si es otra); el orden de la lista es el de `a`.
func conservarAsistentes(a, de *ical.Component) {
	antes := asistentesDe(de)
	for i, p := range a.Props[ical.PropAttendee] {
		g, ok := antes[cleanCalAddress(strings.ToLower(p.Value))]
		if !ok || asistenteSemantico(g) != asistenteSemantico(p) {
			continue
		}
		q := copiaPropiedad(g)
		if estadoDe(q) != estadoDe(p) {
			q.Params.Set(ical.ParamParticipationStatus, estadoDe(p))
		}
		a.Props[ical.PropAttendee][i] = q
	}
}

// conservarSecuencia: la SEQUENCE es un entero (EncodeEvent la escribe como texto,
// «SEQUENCE;VALUE=TEXT:2»). La misma que `de` va como en `de`; otra, sin el VALUE.
func conservarSecuencia(a, de *ical.Component) {
	p := a.Props.Get(ical.PropSequence)
	if p == nil {
		return
	}
	if n, ok := secuenciaDe(a); ok {
		if v, okV := secuenciaDe(de); okV && v == n {
			a.Props[ical.PropSequence] = copiaPropiedades(de.Props[ical.PropSequence])
			return
		}
	}
	p.Params.Del(ical.ParamValue)
}

// avisosEditables son los VALARM editables del VEVENT (los que regenera el formulario) y
// sus minutos, ordenados.
func avisosEditables(ev *ical.Component) ([]*ical.Component, []int) {
	var avisos []*ical.Component
	var minutos []int
	for _, h := range ev.Children {
		if h.Name != ical.CompAlarm {
			continue
		}
		if m, ok := minutosEditables(h); ok {
			avisos = append(avisos, h)
			minutos = append(minutos, m)
		}
	}
	slices.Sort(minutos)
	return avisos, minutos
}

// conservarAvisos: si `a` trae los mismos recordatorios editables que `de` (los mismos
// minutos), van los de `de` tal cual (con su texto y su UID).
func conservarAvisos(a, de *ical.Component) {
	nuevos, mn := avisosEditables(a)
	viejos, mv := avisosEditables(de)
	if len(nuevos) == 0 || !slices.Equal(mn, mv) {
		return
	}
	sustituirAvisos(a, viejos)
}

// sustituirAvisos cambia los VALARM editables de ev por copias de `por`; los demás se
// quedan.
func sustituirAvisos(ev *ical.Component, por []*ical.Component) {
	quedan := ev.Children[:0]
	for _, h := range ev.Children {
		if h.Name == ical.CompAlarm {
			if _, ok := minutosEditables(h); ok {
				continue
			}
		}
		quedan = append(quedan, h)
	}
	ev.Children = quedan
	for _, h := range por {
		ev.Children = append(ev.Children, copiaComponente(h))
	}
}

func copiaComponente(c *ical.Component) *ical.Component {
	out := ical.NewComponent(c.Name)
	for n, ps := range c.Props {
		out.Props[n] = copiaPropiedades(ps)
	}
	for _, h := range c.Children {
		out.Children = append(out.Children, copiaComponente(h))
	}
	return out
}

// Lo que la persona cambia en la serie, en sus excepciones (Sytue Office).
//
// Una excepción (RECURRENCE-ID) es la copia de una ocurrencia con algo cambiado. Al
// cambiar la serie, la práctica habitual (la de Google y Outlook) es que cada excepción
// que tenía en un campo lo mismo que la serie reciba lo nuevo, y que la que lo tenía
// cambiado a propósito lo conserve. Sin esto, cambiar el título de una serie dejaba con el
// de antes las ocurrencias con excepción, también la que creó la respuesta de un invitado
// a esa sola ocurrencia (una copia del maestro: lo tiene todo igual). La hora de las
// excepciones no se mueve por esto.

// gruposDeLaSerie son los campos de la serie que llegan a sus excepciones.
var gruposDeLaSerie = [][]string{
	{ical.PropSummary},
	{ical.PropLocation},
	{ical.PropDescription},
	{ical.PropStatus},
	{ical.PropOrganizer},
	{ical.PropClass},
	{ical.PropTransparency},
	{"COLOR", "X-APPLE-CALENDAR-COLOR"},
	{"CONFERENCE"},
}

// LlevarALasExcepciones devuelve `nuevo` —la serie que se guarda, con las excepciones
// que CarryOver trajo de `anterior`— con lo que la persona cambió en el maestro (lo que
// difiere entre el maestro de `anterior` y el de `nuevo`) puesto en cada excepción que lo
// tenía como el maestro de `anterior`: el título, el lugar, la descripción, el estado, el
// organizador, la clase, la transparencia, el color, la videollamada y los recordatorios
// editables. La excepción que lo tenía cambiado lo conserva, y la hora no se toca. A la
// que recibe algo que ven los invitados se le sube la SEQUENCE a la del maestro. Sin nada
// que llevar, vuelve `nuevo` tal cual.
func LlevarALasExcepciones(nuevo, anterior string) (string, error) {
	if strings.TrimSpace(anterior) == "" {
		return nuevo, nil
	}
	calNuevo, err := ical.NewDecoder(strings.NewReader(nuevo)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso nuevo: %w", err)
	}
	calAnterior, err := ical.NewDecoder(strings.NewReader(anterior)).Decode()
	if err != nil {
		return nuevo, nil //nolint:nilerr // lo guardado ilegible no puede bloquear la edición
	}
	mN, mA := masterComponent(calNuevo), masterComponent(calAnterior)
	if mN == nil || mA == nil {
		return nuevo, nil
	}
	tocado := false
	for _, ex := range calNuevo.Children {
		if ex.Name == ical.CompEvent && ex.Props.Get(ical.PropRecurrenceID) != nil && llevarCambios(mA, mN, ex) {
			tocado = true
		}
	}
	if !tocado {
		return nuevo, nil
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(calNuevo); err != nil {
		return "", fmt.Errorf("ical: re-serializar el recurso: %w", err)
	}
	return buf.String(), nil
}

// llevarCambios pone en la excepción ex lo que cambió del maestro `antes` al maestro
// `ahora` y ex tenía como `antes`. Dice si ha cambiado algo.
func llevarCambios(antes, ahora, ex *ical.Component) bool {
	tocado, visible := false, false
	for _, grupo := range gruposDeLaSerie {
		if mismoGrupo(antes, ahora, grupo) {
			continue // la persona no lo cambió
		}
		if !mismoGrupo(ex, antes, grupo) {
			continue // la excepción lo tenía cambiado a propósito
		}
		copiarGrupo(ex, ahora, grupo)
		tocado = true
		visible = visible || slices.Contains(PropiedadesDeLaInvitacion, grupo[0])
	}
	_, ma := avisosEditables(antes)
	nuevos, mn := avisosEditables(ahora)
	if _, me := avisosEditables(ex); !slices.Equal(ma, mn) && slices.Equal(me, ma) {
		sustituirAvisos(ex, nuevos)
		tocado = true
	}
	if visible {
		if s, ok := secuenciaDe(ahora); ok {
			if e, _ := secuenciaDe(ex); e < s {
				ex.Props[ical.PropSequence] = []ical.Prop{copiaPropiedad(*ahora.Props.Get(ical.PropSequence))}
			}
		}
	}
	return tocado
}

// secuenciaDe es la SEQUENCE del VEVENT; ok=false si no la trae.
func secuenciaDe(ev *ical.Component) (int, bool) {
	p := ev.Props.Get(ical.PropSequence)
	if p == nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(p.Value))
	return n, err == nil
}
