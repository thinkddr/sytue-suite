// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/emersion/go-ical"
)

// Las respuestas de los invitados al guardar un evento.
//
// El formulario de Calendar guarda el evento ENTERO (el PUT sustituye) hecho con lo que
// leyó al abrirse. Si mientras estaba abierto cambió en el almacén lo que contestó un
// invitado —su respuesta llegó por correo y se aplicó (iMIP REPLY, `local/agenda`), o la
// trajo la sincronización de la cuenta—, guardar la devolvía a lo de antes, aquí y en el
// servidor. ConservarRespuestas funde lo que se guarda con lo que hay; Sytue Office lo
// pide con `calendar.Repository.ConservarRespuestas` (la edición conectada no cambia).

// PropiedadesDeLaInvitacion son las de cada VEVENT que, si cambian, cambian lo que ven
// los invitados (junto con quiénes son): las fechas, la repetición, el título, el lugar,
// la descripción, el estado y quién organiza. Un recordatorio, un color o lo que
// contestó cada uno no cuentan. Es también la firma de las invitaciones por correo
// (`local/agenda`, iMIP).
var PropiedadesDeLaInvitacion = []string{
	ical.PropRecurrenceID, ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDuration, ical.PropRecurrenceRule,
	ical.PropRecurrenceDates, ical.PropExceptionDates, ical.PropSummary, ical.PropLocation, ical.PropDescription, ical.PropStatus, ical.PropOrganizer,
}

// ConservarRespuestas devuelve el recurso `nuevo` —lo que se guarda, que el formulario
// hizo con lo que leyó al abrirse— con lo que contestaron los invitados según `guardado`
// (lo que hay ahora en el almacén):
//
//   - en el maestro, un invitado que sigue en `nuevo` con el mismo ROLE lleva el PARTSTAT
//     de `guardado`. El formulario no cambia lo que contestó nadie: lleva lo que leyó. Si
//     no coincide con lo guardado, es que cambió en el almacén mientras tanto;
//   - un invitado que la persona TOCÓ queda como ella lo dejó: si lo quitó, no está (ni
//     vuelve porque llegara su respuesta); si le cambió el ROLE, lleva el suyo y lo que
//     ella tenía de él;
//   - las excepciones (RECURRENCE-ID) que `nuevo` trae de `guardado` (CarryOver: la misma
//     serie) ya llevan lo que se contestó a cada ocurrencia, también la que creó una
//     respuesta mientras tanto. Lo que la persona cambió en la lista del maestro vale
//     para toda la serie: a quien quitó se le quita de ellas, a quien añadió se le añade
//     y a quien cambió el ROLE, se le cambia;
//   - todo lo demás (el título, las horas, la repetición…) es de `nuevo`.
//
// Un `guardado` que no se deja leer no tiene nada que conservar: vuelve `nuevo`.
func ConservarRespuestas(nuevo, guardado string) (string, error) {
	if strings.TrimSpace(guardado) == "" {
		return nuevo, nil
	}
	calNuevo, err := ical.NewDecoder(strings.NewReader(nuevo)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso nuevo: %w", err)
	}
	calGuardado, err := ical.NewDecoder(strings.NewReader(guardado)).Decode()
	if err != nil {
		return nuevo, nil //nolint:nilerr // lo guardado ilegible no puede bloquear la edición
	}
	mNuevo, mGuardado := masterComponent(calNuevo), masterComponent(calGuardado)
	if mNuevo == nil || mGuardado == nil {
		return nuevo, nil
	}
	antes := asistentesDe(mGuardado)
	ahora := asistentesDe(mNuevo)
	// El maestro: lo que se contestó mientras tanto, de quien la persona no tocó.
	tocado := ponerRespuestas(mNuevo, antes)

	// Las excepciones: lo que la persona cambió en la lista de la serie.
	for _, ev := range calNuevo.Children {
		if ev.Name == ical.CompEvent && ev.Props.Get(ical.PropRecurrenceID) != nil && llevarListaDeLaSerie(ev, mNuevo, antes, ahora) {
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

// llevarListaDeLaSerie pone en la excepción ev lo que la persona cambió en la lista de
// invitados de la serie: `antes` es la del maestro guardado y `ahora` la del maestro
// nuevo mNuevo. Quien quitó deja de estar, quien añadió se añade y a quien cambió el ROLE
// se le cambia (si la excepción no lo tenía ya cambiado). Dice si ha cambiado algo.
func llevarListaDeLaSerie(ev, mNuevo *ical.Component, antes, ahora map[string]ical.Prop) bool {
	tocado := false
	var quedan []ical.Prop
	estan := map[string]bool{}
	for _, p := range ev.Props[ical.PropAttendee] {
		d := cleanCalAddress(strings.ToLower(p.Value))
		n, sigue := ahora[d]
		if _, estaba := antes[d]; estaba && !sigue {
			tocado = true // quitado de la serie
			continue
		}
		if g, estaba := antes[d]; estaba && sigue && rolDe(g) != rolDe(n) && rolDe(p) != rolDe(n) {
			p = copiaPropiedad(p)
			p.Params.Set(ical.ParamRole, rolDe(n))
			tocado = true
		}
		estan[d] = true
		quedan = append(quedan, p)
	}
	for _, p := range mNuevo.Props[ical.PropAttendee] {
		d := cleanCalAddress(strings.ToLower(p.Value))
		if _, estaba := antes[d]; !estaba && !estan[d] {
			quedan = append(quedan, copiaPropiedad(p)) // añadido a la serie
			estan[d] = true
			tocado = true
		}
	}
	if len(quedan) == 0 {
		delete(ev.Props, ical.PropAttendee)
	} else {
		ev.Props[ical.PropAttendee] = quedan
	}
	return tocado
}

// CambiaLaInvitacion dice si `nuevo` cambia, respecto de `anterior`, lo que ven los
// invitados: alguna de [PropiedadesDeLaInvitacion] o quiénes son, en el maestro o en una
// excepción (lo que contestó cada uno no). Compara lo que SIGNIFICA cada propiedad
// ([ContenidoDeLaInvitacion]), no cómo está escrita: la misma hora en otra zona, las
// mismas EXDATE en UTC o el STATUS que faltaba escrito como CONFIRMED no son un cambio.
// Es lo que pide subir la SEQUENCE (RFC 5545 §3.8.7.4: «una revisión significativa») y
// lo que manda la invitación por correo. Ante la duda (uno de los dos no se lee), sí.
func CambiaLaInvitacion(nuevo, anterior string) bool {
	a, errA := contenidoDeLaInvitacion(nuevo)
	b, errB := contenidoDeLaInvitacion(anterior)
	return errA != nil || errB != nil || a != b
}

// contenidoDeLaInvitacion es [ContenidoDeLaInvitacion] de un VCALENDAR en texto.
func contenidoDeLaInvitacion(datos string) (string, error) {
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return "", err
	}
	return ContenidoDeLaInvitacion(cal), nil
}

// valorDe es el valor tal cual de la primera propiedad `nombre` de c, o "".
func valorDe(c *ical.Component, nombre string) string {
	if p := c.Props.Get(nombre); p != nil {
		return p.Value
	}
	return ""
}

// lineaDe es la propiedad con sus parámetros en orden.
func lineaDe(p ical.Prop) string {
	claves := make([]string, 0, len(p.Params))
	for k := range p.Params {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	var b strings.Builder
	b.WriteString(p.Name)
	for _, k := range claves {
		b.WriteString(";" + k + "=" + strings.Join(p.Params[k], ","))
	}
	b.WriteString(":" + p.Value)
	return b.String()
}

// ponerRespuestas pone en los invitados del VEVENT ev el PARTSTAT que tienen en
// `guardados` (lo que hay en el almacén, por dirección), salvo a quien la persona cambió
// el ROLE: el formulario no cambia lo que contestó nadie, así que lo distinto es lo que
// se contestó mientras tanto. Dice si ha cambiado alguno.
func ponerRespuestas(ev *ical.Component, guardados map[string]ical.Prop) bool {
	tocado := false
	for i := range ev.Props[ical.PropAttendee] {
		p := &ev.Props[ical.PropAttendee][i]
		g, ok := guardados[cleanCalAddress(strings.ToLower(p.Value))]
		if !ok || rolDe(g) != rolDe(*p) || estadoDe(g) == estadoDe(*p) {
			continue
		}
		p.Params.Set(ical.ParamParticipationStatus, estadoDe(g))
		tocado = true
	}
	return tocado
}

// asistentesDe son los ATTENDEE de un VEVENT por su dirección (el primero, si se repite).
func asistentesDe(ev *ical.Component) map[string]ical.Prop {
	out := map[string]ical.Prop{}
	for _, p := range ev.Props[ical.PropAttendee] {
		d := cleanCalAddress(strings.ToLower(p.Value))
		if _, ya := out[d]; d != "" && !ya {
			out[d] = p
		}
	}
	return out
}

// estadoDe es el PARTSTAT de un ATTENDEE; sin él, NEEDS-ACTION (RFC 5545 §3.2.12).
func estadoDe(p ical.Prop) string {
	if v := strings.ToUpper(strings.TrimSpace(p.Params.Get(ical.ParamParticipationStatus))); v != "" {
		return v
	}
	return "NEEDS-ACTION"
}

// rolDe es el ROLE de un ATTENDEE; sin él, REQ-PARTICIPANT (RFC 5545 §3.2.16).
func rolDe(p ical.Prop) string {
	if v := strings.ToUpper(strings.TrimSpace(p.Params.Get(ical.ParamRole))); v != "" {
		return v
	}
	return "REQ-PARTICIPANT"
}

func copiaPropiedad(p ical.Prop) ical.Prop {
	q := ical.Prop{Name: p.Name, Value: p.Value, Params: ical.Params{}}
	for k, vs := range p.Params {
		q.Params[k] = append([]string(nil), vs...)
	}
	return q
}
