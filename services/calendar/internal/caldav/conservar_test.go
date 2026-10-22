// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
)

// Lo que regenera el PUT REST: los campos del evento, sin recordatorios.
const sinRecordatorios = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Calendar//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:series-1@sytue.eu\r\nDTSTAMP:20260901T080000Z\r\nDTSTART:20260701T090000Z\r\nDTEND:20260701T100000Z\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=10;BYDAY=WE\r\nSUMMARY:Standup (con otro título)\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// El PUT REST regenera el iCalendar desde los campos, y ahí no hay recordatorios: editar un
// evento por REST le quitaba a la persona los que había puesto en la web.
func TestCarryOverConservaLosRecordatorios(t *testing.T) {
	out, err := CarryOver(sinRecordatorios, seriesWithOverride, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "BEGIN:VALARM") != 1 || !strings.Contains(out, "TRIGGER:-PT15M") {
		t.Fatalf("el recordatorio no ha sobrevivido a la edición:\n%s", out)
	}
	if !strings.Contains(out, "Standup (con otro título)") {
		t.Fatalf("conservar lo viejo ha pisado lo nuevo:\n%s", out)
	}
	// Misma regla y mismo inicio: la serie es la misma y sus excepciones siguen valiendo.
	if strings.Count(out, "RECURRENCE-ID") != strings.Count(seriesWithOverride, "RECURRENCE-ID") {
		t.Fatalf("se han perdido las excepciones de la serie:\n%s", out)
	}
	// Al reparsear tiene que seguir siendo UN evento con su recordatorio: `replaceReminders`
	// saca de aquí las filas de recordatorios.
	if _, err := DecodeCalendarObject(out, "cal"); err != nil {
		t.Fatalf("lo conservado no se deja leer: %v", err)
	}
}

func TestCarryOverNoInventaNiDuplica(t *testing.T) {
	// Sin nada que conservar, el recurso nuevo vuelve TAL CUAL (ni reserializado).
	if out, err := CarryOver(sinRecordatorios, sinRecordatorios, false); err != nil || out != sinRecordatorios {
		t.Fatalf("sin recordatorios que conservar ha cambiado el recurso: %v\n%s", err, out)
	}
	if out, _ := CarryOver(sinRecordatorios, "", false); out != sinRecordatorios {
		t.Fatal("un evento nuevo (sin recurso anterior) no tiene nada que conservar")
	}
	// Si el nuevo ya trae sus recordatorios, mandan los suyos: no se suman los viejos.
	conSuyo := strings.Replace(sinRecordatorios, "END:VEVENT", "BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT5M\r\nEND:VALARM\r\nEND:VEVENT", 1)
	out, err := CarryOver(conSuyo, seriesWithOverride, true)
	if err != nil || strings.Count(out, "BEGIN:VALARM") != 1 || !strings.Contains(out, "TRIGGER:-PT5M") {
		t.Fatalf("se han mezclado recordatorios nuevos y viejos: %v\n%s", err, out)
	}
	// Otra regla u otra hora: las excepciones apuntarían a ocurrencias que ya no existen.
	otraHora := strings.Replace(sinRecordatorios, "DTSTART:20260701T090000Z", "DTSTART:20260701T110000Z", 1)
	out, err = CarryOver(otraHora, seriesWithOverride, false)
	if err != nil || strings.Contains(out, "RECURRENCE-ID") || strings.Count(out, "BEGIN:VALARM") != 1 {
		t.Fatalf("con la serie movida: %v\n%s", err, out)
	}
	// Un recurso guardado ilegible no bloquea la edición.
	if out, err := CarryOver(sinRecordatorios, "esto no es iCalendar", false); err != nil || out != sinRecordatorios {
		t.Fatalf("un recurso viejo ilegible ha bloqueado la edición: %v", err)
	}
}
