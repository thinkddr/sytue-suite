// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"

	"github.com/emersion/go-ical"
)

// serieConRespuestas es una serie semanal que organiza Ana con Luis y Marta invitados, y
// la excepción de la segunda semana (la que crea la respuesta de Luis a esa ocurrencia).
func serieConRespuestas(luisMaestro, martaMaestro, luisExcepcion, resumen string) string {
	at := func(cn, partstat, correo string) string {
		return "ATTENDEE;CN=" + cn + ";PARTSTAT=" + partstat + ";ROLE=REQ-PARTICIPANT;RSVP=TRUE:mailto:" + correo + "\r\n"
	}
	maestro := "BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nDTSTART:20261202T090000Z\r\nDTEND:20261202T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:" + resumen + "\r\nORGANIZER:mailto:ana@example.org\r\n" +
		at("Luis", luisMaestro, "luis@example.org") + at("Marta", martaMaestro, "marta@example.org") + "END:VEVENT\r\n"
	excepcion := ""
	if luisExcepcion != "" {
		excepcion = "BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nRECURRENCE-ID:20261209T090000Z\r\n" +
			"DTSTART:20261209T090000Z\r\nDTEND:20261209T100000Z\r\nSUMMARY:" + resumen + "\r\nORGANIZER:mailto:ana@example.org\r\n" +
			at("Luis", luisExcepcion, "luis@example.org") + at("Marta", martaMaestro, "marta@example.org") + "END:VEVENT\r\n"
	}
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" + maestro + excepcion + "END:VCALENDAR\r\n"
}

// asistente es el ATTENDEE de correo en el VEVENT con ese RECURRENCE-ID ("" el maestro).
func asistente(t *testing.T, datos, rid, correo string) (ical.Prop, bool) {
	t.Helper()
	cal, err := ical.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range cal.Children {
		if ev.Name != ical.CompEvent {
			continue
		}
		r := ""
		if p := ev.Props.Get(ical.PropRecurrenceID); p != nil {
			r = p.Value
		}
		if r != rid {
			continue
		}
		for _, p := range ev.Props[ical.PropAttendee] {
			if strings.EqualFold(p.Value, "mailto:"+correo) {
				return p, true
			}
		}
		return ical.Prop{}, false
	}
	t.Fatalf("no hay VEVENT %q en %s", rid, datos)
	return ical.Prop{}, false
}

// Lo que contestaron mientras el formulario estaba abierto se queda: en el maestro, de
// quien sigue con el mismo ROLE; las excepciones vienen de lo guardado. Lo demás es de lo
// que se guarda.
func TestConservarRespuestasDeQuienNoSeToco(t *testing.T) {
	// Al abrir: Luis y Marta sin contestar. Mientras tanto: Marta acepta la serie y Luis
	// declina la segunda semana (su excepción). Lo que se guarda: otro título, con lo leído.
	guardado := serieConRespuestas("NEEDS-ACTION", "ACCEPTED", "DECLINED", "Seguimiento")
	nuevo, err := CarryOver(serieConRespuestas("NEEDS-ACTION", "NEEDS-ACTION", "", "Seguimiento Q4"), guardado, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ConservarRespuestas(nuevo, guardado)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := asistente(t, out, "", "marta@example.org"); estadoDe(m) != "ACCEPTED" {
		t.Fatalf("la respuesta de Marta en el maestro: %s", out)
	}
	if l, _ := asistente(t, out, "", "luis@example.org"); estadoDe(l) != "NEEDS-ACTION" {
		t.Fatalf("Luis no contestó a la serie: %s", out)
	}
	if l, _ := asistente(t, out, "20261209T090000Z", "luis@example.org"); estadoDe(l) != "DECLINED" {
		t.Fatalf("la de Luis a su ocurrencia: %s", out)
	}
	if !strings.Contains(out, "SUMMARY:Seguimiento Q4") {
		t.Fatalf("el título es lo que se guarda: %s", out)
	}
	// Sin nada que conservar, vuelve tal cual.
	if igual, err := ConservarRespuestas(nuevo, nuevo); err != nil || igual != nuevo {
		t.Fatal("sin cambios", err)
	}
	if igual, err := ConservarRespuestas(nuevo, "no es un calendario"); err != nil || igual != nuevo {
		t.Fatal("con lo guardado ilegible", err)
	}
}

// A quien la persona tocó, gana ella: si lo quitó, no está (tampoco en las excepciones,
// aunque su respuesta llegara entre medias); si le cambió el ROLE, lleva el suyo y lo
// que ella tenía de él; y quien añadió a la serie entra también en sus excepciones.
func TestConservarRespuestasGanaLaPersonaEnQuienToco(t *testing.T) {
	guardado := serieConRespuestas("ACCEPTED", "DECLINED", "DECLINED", "Seguimiento")
	// Quita a Luis.
	sinLuis := strings.Replace(serieConRespuestas("NEEDS-ACTION", "NEEDS-ACTION", "", "Seguimiento"),
		"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT;RSVP=TRUE:mailto:luis@example.org\r\n", "", 1)
	nuevo, err := CarryOver(sinLuis, guardado, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ConservarRespuestas(nuevo, guardado)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "luis@") {
		t.Fatalf("quitado de la serie, Luis no vuelve: %s", out)
	}
	if m, _ := asistente(t, out, "", "marta@example.org"); estadoDe(m) != "DECLINED" {
		t.Fatalf("Marta conserva lo suyo: %s", out)
	}

	// Cambia el ROLE de Marta y añade a Pedro.
	otra := strings.Replace(serieConRespuestas("NEEDS-ACTION", "NEEDS-ACTION", "", "Seguimiento"),
		"PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT;RSVP=TRUE:mailto:marta@example.org\r\n",
		"PARTSTAT=NEEDS-ACTION;ROLE=OPT-PARTICIPANT;RSVP=TRUE:mailto:marta@example.org\r\nATTENDEE;CN=Pedro;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:pedro@example.org\r\n", 1)
	nuevo, err = CarryOver(otra, guardado, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err = ConservarRespuestas(nuevo, guardado)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := asistente(t, out, "", "marta@example.org"); rolDe(m) != "OPT-PARTICIPANT" || estadoDe(m) != "NEEDS-ACTION" {
		t.Fatalf("Marta como la dejó la persona: %s", out)
	}
	if m, _ := asistente(t, out, "20261209T090000Z", "marta@example.org"); rolDe(m) != "OPT-PARTICIPANT" {
		t.Fatalf("su ROLE también en la excepción: %s", out)
	}
	if l, _ := asistente(t, out, "", "luis@example.org"); estadoDe(l) != "ACCEPTED" {
		t.Fatalf("Luis, sin tocar, con lo suyo: %s", out)
	}
	if _, ok := asistente(t, out, "20261209T090000Z", "pedro@example.org"); !ok {
		t.Fatalf("Pedro, añadido a la serie, también en la excepción: %s", out)
	}
}

// Lo que ven los invitados: las fechas, el título, quiénes son… pero no lo que contestó
// cada uno, ni un recordatorio.
func TestCambiaLaInvitacion(t *testing.T) {
	base := serieConRespuestas("NEEDS-ACTION", "ACCEPTED", "DECLINED", "Seguimiento")
	conAviso := strings.Replace(base, "END:VEVENT\r\n", "BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nEND:VALARM\r\nEND:VEVENT\r\n", 1)
	for nombre, caso := range map[string]struct {
		otro   string
		cambia bool
	}{
		"igual":               {base, false},
		"otras respuestas":    {serieConRespuestas("ACCEPTED", "TENTATIVE", "ACCEPTED", "Seguimiento"), false},
		"un recordatorio":     {conAviso, false},
		"otro título":         {serieConRespuestas("NEEDS-ACTION", "ACCEPTED", "DECLINED", "Otro"), true},
		"otra hora":           {strings.Replace(base, "DTEND:20261202T100000Z", "DTEND:20261202T103000Z", 1), true},
		"sin la excepción":    {serieConRespuestas("NEEDS-ACTION", "ACCEPTED", "", "Seguimiento"), true},
		"un invitado más":     {strings.Replace(base, "END:VEVENT", "ATTENDEE:mailto:pedro@example.org\r\nEND:VEVENT", 1), true},
		"no es un calendario": {"BEGIN:VCALENDAR", true},
	} {
		if got := CambiaLaInvitacion(caso.otro, base); got != caso.cambia {
			t.Errorf("%s: %v, quiero %v", nombre, got, caso.cambia)
		}
	}
}
