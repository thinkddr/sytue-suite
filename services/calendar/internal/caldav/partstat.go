// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// ConPartStat devuelve el recurso iCalendar `data` con el PARTSTAT del ATTENDEE
// `email` cambiado a `partstat`, en el maestro Y en cada override (RECURRENCE-ID)
// del recurso: la respuesta a una serie vale para toda la serie, y un override que
// conservara la respuesta vieja pintaría esa ocurrencia como sin contestar.
//
// Solo se toca la línea de ESA dirección (sin distinguir mayúsculas ni «mailto:»):
// los demás asistentes, el organizador, los VALARM y lo que Sytue no interpreta
// salen tal cual. `encontrado` dice si la dirección figura como asistente en algún
// VEVENT; si no figura, o si ya tenía esa respuesta, `data` vuelve sin cambios
// (sin re-serializar), y así quien llama puede ahorrarse una escritura en balde.
func ConPartStat(data, email string, partstat domain.ParticipationStatus) (out string, encontrado bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return data, false, nil
	}
	if !domain.ValidPartStat(partstat) {
		return "", false, fmt.Errorf("ical: PARTSTAT no válido: %q", partstat)
	}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", false, fmt.Errorf("ical: decodificar evento: %w", err)
	}
	cambiado := false
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		props := child.Props[ical.PropAttendee]
		for i := range props {
			if !strings.EqualFold(cleanCalAddress(props[i].Value), email) {
				continue
			}
			encontrado = true
			if strings.EqualFold(props[i].Params.Get(ical.ParamParticipationStatus), string(partstat)) {
				continue
			}
			if props[i].Params == nil {
				props[i].Params = make(ical.Params)
			}
			props[i].Params.Set(ical.ParamParticipationStatus, string(partstat))
			cambiado = true
		}
	}
	if !cambiado {
		return data, encontrado, nil
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", false, fmt.Errorf("ical: re-serializar evento: %w", err)
	}
	return buf.String(), true, nil
}

// Respuesta es el REPLY iTIP (RFC 5546 §3.2.3) con el que el asistente `email` le
// dice a quien organiza el evento de `data` que su respuesta es `partstat`. Sirve
// para quien organiza desde FUERA de la suite, que no tiene bandeja de Sytue: le
// llega por correo y su cliente lo casa por UID y ATTENDEE.
//
// Lleva solo lo que el RFC pide al REPLY de un VEVENT —ORGANIZER, UID, SEQUENCE,
// DTSTAMP, el horario para quien lo lea y UN ATTENDEE, el que contesta—, y del
// maestro: la respuesta vale para toda la serie, como en ConPartStat. Los demás
// invitados no van: su respuesta no es cosa de quien contesta, ni sus direcciones
// tienen por qué salir de aquí. Los VTIMEZONE sí, porque el DTSTART puede citarlos.
//
// `organizador` es la dirección de ORGANIZER sin «mailto:» y `resumen` el SUMMARY.
// Error si el recurso no se lee, si no hay organizador o si `email` no figura como
// asistente: un REPLY de quien no está invitado no se manda.
func Respuesta(data, email string, partstat domain.ParticipationStatus) (ics, organizador, resumen string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !domain.ValidPartStat(partstat) {
		return "", "", "", fmt.Errorf("ical: PARTSTAT no válido: %q", partstat)
	}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", "", "", fmt.Errorf("ical: decodificar evento: %w", err)
	}
	maestro, zonas := maestroYZonas(cal)
	if maestro == nil {
		return "", "", "", fmt.Errorf("ical: el recurso no tiene evento")
	}
	org := maestro.Props.Get(ical.PropOrganizer)
	if org == nil || cleanCalAddress(org.Value) == "" {
		return "", "", "", fmt.Errorf("ical: el evento no tiene organizador")
	}
	asistente := asistenteDe(maestro, email)
	if asistente == nil {
		return "", "", "", fmt.Errorf("ical: %q no figura como asistente", email)
	}

	evt := ical.NewEvent()
	copiadas := []string{
		ical.PropUID, ical.PropSequence, ical.PropSummary,
		ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDuration,
	}
	for _, nombre := range copiadas {
		if p := maestro.Props.Get(nombre); p != nil {
			evt.Props.Set(p)
		}
	}
	evt.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	evt.Props.Set(org)
	// Del ATTENDEE se guarda su nombre y su papel; RSVP no tiene sentido en una
	// respuesta, que ya es el RSVP.
	resp := ical.NewProp(ical.PropAttendee)
	resp.Value = asistente.Value
	for _, param := range []string{"CN", "ROLE", "CUTYPE"} {
		if v := asistente.Params.Get(param); v != "" {
			resp.Params.Set(param, v)
		}
	}
	resp.Params.Set(ical.ParamParticipationStatus, string(partstat))
	evt.Props.Add(resp)

	out := ical.NewCalendar()
	out.Props.SetText(ical.PropProductID, ProductID)
	out.Props.SetText(ical.PropVersion, "2.0")
	out.Props.SetText(ical.PropMethod, string(domain.MethodReply))
	out.Children = append(out.Children, zonas...)
	out.Children = append(out.Children, evt.Component)
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(out); err != nil {
		return "", "", "", fmt.Errorf("ical: codificar REPLY: %w", err)
	}
	if p := maestro.Props.Get(ical.PropSummary); p != nil {
		resumen = p.Value
	}
	return buf.String(), cleanCalAddress(org.Value), resumen, nil
}

// maestroYZonas son el VEVENT maestro (el primero sin RECURRENCE-ID) y los VTIMEZONE
// de un VCALENDAR.
func maestroYZonas(cal *ical.Calendar) (maestro *ical.Component, zonas []*ical.Component) {
	for _, child := range cal.Children {
		switch {
		case child.Name == ical.CompTimezone:
			zonas = append(zonas, child)
		case child.Name == ical.CompEvent && maestro == nil && child.Props.Get(ical.PropRecurrenceID) == nil:
			maestro = child
		}
	}
	return maestro, zonas
}

// asistenteDe es el ATTENDEE de `email` (en minúsculas) en el VEVENT, o nil.
func asistenteDe(evt *ical.Component, email string) *ical.Prop {
	asistentes := evt.Props[ical.PropAttendee]
	for i := range asistentes {
		if email != "" && strings.EqualFold(cleanCalAddress(asistentes[i].Value), email) {
			return &asistentes[i]
		}
	}
	return nil
}
