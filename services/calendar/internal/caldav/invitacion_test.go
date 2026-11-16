// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
)

// requestDeFuera es el REQUEST iMIP que manda un calendario de fuera (Google, Outlook)
// a una dirección de la suite.
const requestDeFuera = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Google Inc//Google Calendar 70.9054//EN\r\n" +
	"METHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\nUID:abc123@google.com\r\nDTSTAMP:20261101T120000Z\r\n" +
	"DTSTART:20261112T090000Z\r\nDTEND:20261112T100000Z\r\nSEQUENCE:3\r\nSUMMARY:Revisión\r\n" +
	"ORGANIZER;CN=Luis:mailto:luis@otro.example\r\n" +
	"ATTENDEE;CN=Ana;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ana@sytue.eu\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func TestLeerInvitacionQuitaElMetodoYGuardaLoDemas(t *testing.T) {
	inv, err := LeerInvitacion(requestDeFuera)
	if err != nil {
		t.Fatal(err)
	}
	if inv.UID != "abc123@google.com" || inv.Organizador != "luis@otro.example" || inv.Secuencia != 3 {
		t.Errorf("invitación = %+v", inv)
	}
	if strings.Contains(inv.Objeto, "METHOD") {
		t.Errorf("un recurso de calendario no lleva METHOD:\n%s", inv.Objeto)
	}
	po, err := DecodeCalendarObject(inv.Objeto, "cal-1")
	if err != nil || po.UID != "abc123@google.com" {
		t.Fatalf("el recurso no se puede guardar: %v %+v", err, po)
	}
}

func TestLeerInvitacionSoloAceptaUnRequestConOrganizador(t *testing.T) {
	casos := map[string]string{
		"cancel":          strings.Replace(requestDeFuera, "METHOD:REQUEST", "METHOD:CANCEL", 1),
		"sin método":      strings.Replace(requestDeFuera, "METHOD:REQUEST\r\n", "", 1),
		"sin organizador": strings.Replace(requestDeFuera, "ORGANIZER;CN=Luis:mailto:luis@otro.example\r\n", "", 1),
		"sin UID":         strings.Replace(requestDeFuera, "UID:abc123@google.com\r\n", "", 1),
		"no es iCal":      "hola",
	}
	for nombre, ics := range casos {
		if _, err := LeerInvitacion(ics); err == nil {
			t.Errorf("%s: se aceptó", nombre)
		}
	}
}
