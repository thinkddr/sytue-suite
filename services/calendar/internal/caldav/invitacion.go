// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Invitacion es lo que se saca de un REQUEST iMIP (RFC 6047) que llega por correo de
// un organizador de FUERA de la suite (Google, Outlook, Apple…): el recurso listo para
// guardar en el calendario de quien lo recibe y lo que hace falta para casarlo.
type Invitacion struct {
	// Objeto es el VCALENDAR sin METHOD: un recurso de calendario no lo lleva (RFC
	// 4791 §4.1); METHOD es del mensaje iTIP, no del evento.
	Objeto string
	UID    string
	// Organizador es la dirección de ORGANIZER sin «mailto:».
	Organizador string
	// Secuencia es el SEQUENCE del maestro (0 si no viene).
	Secuencia int
}

// LeerInvitacion valida el iCalendar de un correo de invitación y lo convierte en
// recurso guardable. Error si no es un REQUEST (un CANCEL, un REPLY o un PUBLISH no
// se contestan con Sí/Quizá/No), si no hay VEVENT con UID o si no hay organizador: sin
// él no hay a quién mandar la respuesta.
func LeerInvitacion(ics string) (Invitacion, error) {
	cal, err := ical.NewDecoder(strings.NewReader(ics)).Decode()
	if err != nil {
		return Invitacion{}, fmt.Errorf("ical: decodificar invitación: %w", err)
	}
	metodo := ""
	if p := cal.Props.Get(ical.PropMethod); p != nil {
		metodo = strings.ToUpper(strings.TrimSpace(p.Value))
	}
	if metodo != string(domain.MethodRequest) {
		return Invitacion{}, fmt.Errorf("ical: la invitación no es un REQUEST (%q)", metodo)
	}
	maestro, _ := maestroYZonas(cal)
	if maestro == nil {
		return Invitacion{}, fmt.Errorf("ical: la invitación no tiene evento")
	}
	uid := ""
	if p := maestro.Props.Get(ical.PropUID); p != nil {
		uid = strings.TrimSpace(p.Value)
	}
	if uid == "" {
		return Invitacion{}, fmt.Errorf("ical: la invitación no tiene UID")
	}
	org := ""
	if p := maestro.Props.Get(ical.PropOrganizer); p != nil {
		org = cleanCalAddress(p.Value)
	}
	if org == "" {
		return Invitacion{}, fmt.Errorf("ical: la invitación no tiene organizador")
	}
	secuencia := 0
	if p := maestro.Props.Get(ical.PropSequence); p != nil {
		secuencia, _ = strconv.Atoi(strings.TrimSpace(p.Value))
	}
	delete(cal.Props, ical.PropMethod)
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return Invitacion{}, fmt.Errorf("ical: re-serializar invitación: %w", err)
	}
	return Invitacion{Objeto: buf.String(), UID: uid, Organizador: org, Secuencia: secuencia}, nil
}
