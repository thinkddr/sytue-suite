// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"
	"time"

	event "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// El CN del ORGANIZER llega a la proyección (lo que lee la API REST y nombra al
// organizador en las apps nativas) y vuelve al iCalendar al escribir.
//
// VISTO EN ROJO (2026-11-25): sin leer el CN, OrganizerName sale vacío y Fyne nombra
// al organizador por su dirección mientras la web lo hace por su nombre.
func TestElCNDelOrganizadorVaYVuelve(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\n" +
		"UID:cn-organizador\r\nDTSTAMP:20261125T090000Z\r\nDTSTART:20261126T090000Z\r\nDTEND:20261126T100000Z\r\n" +
		"SUMMARY:Repaso\r\nORGANIZER;CN=\"Ada Lovelace\":mailto:ada@sytue.eu\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	po, err := DecodeCalendarObject(ics, "cal")
	if err != nil {
		t.Fatal(err)
	}
	if po.Master.Organizer != "ada@sytue.eu" || po.Master.OrganizerName != "Ada Lovelace" {
		t.Fatalf("organizador %q, CN %q", po.Master.Organizer, po.Master.OrganizerName)
	}

	inicio := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	ev := event.Event{
		UID: "cn", Summary: "Repaso", Organizer: "ada@sytue.eu", OrganizerName: "Ada Lovelace",
		DTStart: inicio, DTEnd: inicio.Add(time.Hour), TZID: "UTC",
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatal(err)
	}
	vuelta, err := DecodeCalendarObject(data, "cal")
	if err != nil {
		t.Fatal(err)
	}
	if vuelta.Master.OrganizerName != "Ada Lovelace" || !strings.Contains(data, "mailto:ada@sytue.eu") {
		t.Errorf("al escribir se pierde el CN: %q\n%s", vuelta.Master.OrganizerName, data)
	}

	// Sin CN no se escribe uno vacío.
	ev.OrganizerName = ""
	data, err = EncodeEvent(ev, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, "CN=") {
		t.Errorf("sin nombre se escribe un CN:\n%s", data)
	}
}

// Un CN es un parámetro: ni saltos de línea (inyectarían propiedades) ni comillas
// dobles (RFC 5545 no las admite en un valor de parámetro), y con tope de largo.
func TestElCNDelOrganizadorSeSanea(t *testing.T) {
	inicio := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	ev := event.Event{
		UID: "cn-sucio", Summary: "Repaso", Organizer: "ada@sytue.eu",
		OrganizerName: "Ada \"la\"\r\nATTENDEE:mailto:intrusa@example.test",
		DTStart:       inicio, DTEnd: inicio.Add(time.Hour), TZID: "UTC",
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatal(err)
	}
	po, err := DecodeCalendarObject(data, "cal")
	if err != nil {
		t.Fatal(err)
	}
	if len(po.Master.Attendees) != 0 {
		t.Fatalf("el CN inyectó un invitado: %+v", po.Master.Attendees)
	}
	if po.Master.OrganizerName != "Ada laATTENDEE:mailto:intrusa@example.test" {
		t.Errorf("CN saneado: %q", po.Master.OrganizerName)
	}
	if got := event.SanitizeCommonName(strings.Repeat("a", 400)); len([]rune(got)) != event.MaxCommonName {
		t.Errorf("un CN de 400 se queda en %d", len([]rune(got)))
	}
}
