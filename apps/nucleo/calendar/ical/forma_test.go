// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"
	"time"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// deICloud es una serie como la escribe un CalDAV de verdad: la EXDATE en la hora de su
// zona y en dos líneas, sin STATUS, el fin con DURATION, la regla con INTERVAL=1 y el
// UNTIL al final del día en Madrid, el organizador con su nombre, los invitados sin
// CUTYPE ni ROLE y un recordatorio con su UID.
const deICloud = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@icloud\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:3\r\n" +
	"DTSTART;TZID=Europe/Madrid:20261202T100000\r\nDURATION:PT1H30M\r\n" +
	"RRULE:UNTIL=20261230T225959Z;INTERVAL=1;FREQ=WEEKLY\r\n" +
	"EXDATE;TZID=Europe/Madrid:20261209T100000\r\nEXDATE;TZID=Europe/Madrid:20261216T100000\r\n" +
	"SUMMARY:Seguimiento\\, semanal\r\nLOCATION:Sala 2\r\n" +
	"ORGANIZER;CN=Ana:mailto:ana@example.org\r\n" +
	"ATTENDEE;CN=Luis;PARTSTAT=ACCEPTED:mailto:luis@example.org\r\n" +
	"ATTENDEE;CN=Marta;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;X-NUM-GUESTS=0:mailto:marta@example.org\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nUID:aviso-1\r\nX-WR-ALARMUID:aviso-1\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// comoElFormulario es lo que hace el formulario con `deICloud` al guardarlo: el evento
// entero desde sus campos (EncodeEvent), con su recordatorio (ConRecordatorios).
func comoElFormulario(t *testing.T, cambiar func(*domain.Event)) string {
	t.Helper()
	po, err := DecodeCalendarObject(deICloud, "")
	if err != nil {
		t.Fatal(err)
	}
	m := po.Master
	madrid, _ := time.LoadLocation("Europe/Madrid")
	ev := domain.Event{
		UID: m.UID, Summary: m.Summary, Description: m.Description, Location: m.Location, Organizer: m.Organizer,
		DTStart: m.DTStart, DTEnd: m.DTEnd, TZID: m.TZID, Status: domain.StatusConfirmed, Classification: domain.ClassPublic,
		Transparency: domain.TranspOpaque, RRule: "FREQ=WEEKLY;UNTIL=" + time.Date(2026, 12, 30, 23, 59, 59, 0, time.UTC).Format("20060102T150405Z"),
		ExDate: m.ExDate, SequenceNo: m.SequenceNo, Attendees: m.Attendees,
	}
	if !ev.DTStart.Equal(time.Date(2026, 12, 2, 10, 0, 0, 0, madrid)) {
		t.Fatalf("inicio: %v", ev.DTStart)
	}
	if cambiar != nil {
		cambiar(&ev)
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err = ConRecordatorios(data, []int{15}, ev.Summary)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// lineas son las líneas del VEVENT maestro de un VCALENDAR (sin plegar), sin DTSTAMP.
func lineas(d string) map[string]bool {
	out := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(d, "\r\n ", ""), "\r\n") {
		if l != "" && !strings.HasPrefix(l, "DTSTAMP") {
			out[l] = true
		}
	}
	return out
}

// Lo mismo escrito de otra forma no es un cambio para los invitados; lo que cambia lo
// que ven, sí.
func TestLaInvitacionSeComparaPorLoQueSignifica(t *testing.T) {
	formulario := comoElFormulario(t, nil)
	if CambiaLaInvitacion(formulario, deICloud) {
		t.Fatalf("lo mismo escrito por el formulario cuenta como cambio:\n%s", formulario)
	}
	cambios := map[string]func(*domain.Event){
		"el título":      func(e *domain.Event) { e.Summary = "Otro" },
		"el fin":         func(e *domain.Event) { e.DTEnd = e.DTEnd.Add(time.Hour) },
		"una EXDATE":     func(e *domain.Event) { e.ExDate = e.ExDate[:1] },
		"la zona":        func(e *domain.Event) { e.TZID = "UTC" },
		"cancelado":      func(e *domain.Event) { e.Status = domain.StatusCancelled },
		"el final":       func(e *domain.Event) { e.RRule = "FREQ=WEEKLY;COUNT=2" },
		"cada dos":       func(e *domain.Event) { e.RRule = "FREQ=WEEKLY;INTERVAL=2;UNTIL=20261230T235959Z" },
		"un invitado":    func(e *domain.Event) { e.Attendees = e.Attendees[:1] },
		"el organizador": func(e *domain.Event) { e.Organizer = "otra@example.org" },
	}
	for que, cambiar := range cambios {
		if !CambiaLaInvitacion(comoElFormulario(t, cambiar), deICloud) {
			t.Errorf("cambiar %s no cuenta como cambio", que)
		}
	}
	// Una respuesta, un recordatorio o un color no los ven los invitados.
	sinCambio := strings.Replace(strings.Replace(deICloud, "PARTSTAT=NEEDS-ACTION", "PARTSTAT=ACCEPTED", 1), "TRIGGER:-PT15M", "TRIGGER:-PT5M", 1)
	if CambiaLaInvitacion(sinCambio, deICloud) {
		t.Fatal("una respuesta o un recordatorio no cambian la invitación")
	}
	// El UNTIL a otra hora del mismo día que no deja fuera ninguna repetición es el mismo
	// final; uno que deja fuera la última, no.
	if CambiaLaInvitacion(strings.Replace(deICloud, "UNTIL=20261230T225959Z", "UNTIL=20261231T120000Z", 1), deICloud) {
		t.Fatal("un UNTIL que deja las mismas repeticiones")
	}
	if !CambiaLaInvitacion(strings.Replace(deICloud, "UNTIL=20261230T225959Z", "UNTIL=20261230T080000Z", 1), deICloud) {
		t.Fatal("un UNTIL que quita la última repetición")
	}
}

// Guardar desde el formulario sin cambiar nada deja cada línea como vino de la cuenta;
// lo que cambia la persona, como lo escribe el formulario.
func TestConservarFormaDejaComoEstabaLoQueNoCambio(t *testing.T) {
	out, err := ConservarForma(comoElFormulario(t, nil), deICloud)
	if err != nil {
		t.Fatal(err)
	}
	// CarryOver y PreserveExtensions, como en Calendar.
	if out, err = CarryOver(out, deICloud, true); err == nil {
		out, err = PreserveExtensions(out, deICloud)
	}
	if err != nil {
		t.Fatal(err)
	}
	quiero, hay := lineas(deICloud), lineas(out)
	for l := range quiero {
		if !hay[l] && !strings.HasPrefix(l, "PRODID") {
			t.Errorf("falta, tal cual, %q", l)
		}
	}
	for l := range hay {
		if !quiero[l] && !strings.HasPrefix(l, "PRODID") {
			t.Errorf("sobra %q", l)
		}
	}
	if CambiaLaInvitacion(out, deICloud) {
		t.Fatal("no cambia nada para los invitados")
	}

	// La persona cambia el título y la hora del fin: eso va como lo escribe el
	// formulario; las EXDATE, el STATUS que no había y los invitados, como estaban.
	out, err = ConservarForma(comoElFormulario(t, func(e *domain.Event) {
		e.Summary = "Seguimiento del presupuesto"
		e.DTEnd = e.DTEnd.Add(30 * time.Minute)
	}), deICloud)
	if err != nil {
		t.Fatal(err)
	}
	hay = lineas(out)
	for _, l := range []string{
		"SUMMARY:Seguimiento del presupuesto", "DTEND;TZID=Europe/Madrid:20261202T120000",
		"EXDATE;TZID=Europe/Madrid:20261209T100000", "RRULE:UNTIL=20261230T225959Z;INTERVAL=1;FREQ=WEEKLY",
		"ATTENDEE;CN=Luis;PARTSTAT=ACCEPTED:mailto:luis@example.org", "SEQUENCE:3",
	} {
		if !hay[l] {
			t.Errorf("falta %q en:\n%s", l, out)
		}
	}
	for _, l := range []string{"STATUS:CONFIRMED", "CLASS:PUBLIC", "TRANSP:OPAQUE", "DURATION:PT1H30M"} {
		if hay[l] {
			t.Errorf("sobra %q en:\n%s", l, out)
		}
	}
	// Ana retrasa el inicio media hora y deja el fin: la DURATION de antes ya no da ese
	// fin, así que va el DTEND del formulario.
	out, err = ConservarForma(comoElFormulario(t, func(e *domain.Event) { e.DTStart = e.DTStart.Add(30 * time.Minute) }), deICloud)
	if err != nil {
		t.Fatal(err)
	}
	if po, err := DecodeCalendarObject(out, ""); err != nil || !po.Master.DTEnd.Equal(time.Date(2026, 12, 2, 10, 30, 0, 0, time.UTC)) || strings.Contains(out, "DURATION") {
		t.Fatalf("el fin de siempre con el inicio nuevo: %v\n%s", err, out)
	}
	// Sin lo guardado (un evento nuevo), nada que conservar.
	if nuevo := comoElFormulario(t, nil); func() string { o, _ := ConservarForma(nuevo, ""); return o }() != nuevo {
		t.Fatal("un evento nuevo no tiene forma que conservar")
	}
}

// serieConExcepciones es una serie semanal con la excepción que creó la respuesta de Luis
// a la segunda (la copia del maestro: lo tiene todo igual) y una tercera a la que Ana le
// cambió el título a propósito y la movió una hora.
const serieConExcepciones = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:2\r\n" +
	"DTSTART:20261202T090000Z\r\nDTEND:20261202T100000Z\r\nRRULE:FREQ=WEEKLY;COUNT=5\r\n" +
	"SUMMARY:Seguimiento\r\nLOCATION:Sala 2\r\nORGANIZER:mailto:ana@example.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:2\r\nRECURRENCE-ID:20261209T090000Z\r\n" +
	"DTSTART:20261209T090000Z\r\nDTEND:20261209T100000Z\r\n" +
	"SUMMARY:Seguimiento\r\nLOCATION:Sala 2\r\nORGANIZER:mailto:ana@example.org\r\n" +
	"ATTENDEE;PARTSTAT=DECLINED:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:2\r\nRECURRENCE-ID:20261216T090000Z\r\n" +
	"DTSTART:20261216T100000Z\r\nDTEND:20261216T110000Z\r\n" +
	"SUMMARY:Seguimiento (con la dirección)\r\nLOCATION:Sala 2\r\nORGANIZER:mailto:ana@example.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// vevents son los VEVENT de un VCALENDAR por su RECURRENCE-ID ("" el maestro).
func vevents(d string) map[string]string {
	out := map[string]string{}
	for _, ev := range strings.Split(d, "BEGIN:VEVENT")[1:] {
		rid := ""
		if _, v, ok := strings.Cut(ev, "RECURRENCE-ID"); ok {
			v, _, _ = strings.Cut(v, "\r\n")
			_, rid, _ = strings.Cut(v, ":")
		}
		out[rid] = ev
	}
	return out
}

// Lo que la persona cambia en la serie llega a cada excepción que lo tenía como la serie
// —también a la que creó una respuesta—; la que lo tenía cambiado a propósito lo
// conserva, y la hora de las excepciones no se mueve.
func TestLoQueCambiaEnLaSerieLlegaASusExcepciones(t *testing.T) {
	nuevo := strings.NewReplacer("SUMMARY:Seguimiento\r\nLOCATION:Sala 2\r\nORGANIZER:mailto:ana@example.org\r\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:2\r\nRECURRENCE-ID:20261209",
		"SUMMARY:Seguimiento del presupuesto\r\nLOCATION:Sala 4\r\nORGANIZER:mailto:ana@example.org\r\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@example.org\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Aviso\r\nTRIGGER:-PT30M\r\nEND:VALARM\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:2\r\nRECURRENCE-ID:20261209").Replace(serieConExcepciones)
	nuevo = strings.Replace(nuevo, "SEQUENCE:2\r\nDTSTART:20261202", "SEQUENCE:3\r\nDTSTART:20261202", 1)
	if nuevo == serieConExcepciones {
		t.Fatal("la prueba no cambia la serie")
	}
	out, err := LlevarALasExcepciones(nuevo, serieConExcepciones)
	if err != nil {
		t.Fatal(err)
	}
	ev := vevents(out)
	de := ev["20261209T090000Z"]
	for _, l := range []string{
		"SUMMARY:Seguimiento del presupuesto", "LOCATION:Sala 4", "TRIGGER:-PT30M", "SEQUENCE:3",
		"DTSTART:20261209T090000Z", "PARTSTAT=DECLINED:mailto:luis@",
	} {
		if !strings.Contains(de, l) {
			t.Errorf("la excepción de la respuesta de Luis sin %q:\n%s", l, de)
		}
	}
	cambiada := ev["20261216T090000Z"]
	for _, l := range []string{"SUMMARY:Seguimiento (con la dirección)", "LOCATION:Sala 4", "DTSTART:20261216T100000Z", "SEQUENCE:3"} {
		if !strings.Contains(cambiada, l) {
			t.Errorf("la excepción cambiada a propósito sin %q:\n%s", l, cambiada)
		}
	}
	// Sin cambios en la serie, nada que llevar: vuelve tal cual.
	if out, _ := LlevarALasExcepciones(serieConExcepciones, serieConExcepciones); out != serieConExcepciones {
		t.Fatal("sin cambios en la serie, las excepciones no se tocan")
	}
}

// Recortar la serie para «solo este» o «este y los siguientes» quita las excepciones de
// lo recortado (con SinExcepciones) y escribe la EXDATE como el inicio (ComoElInicio).
func TestRecortarSerieQuitaLasExcepcionesDeLoRecortado(t *testing.T) {
	segunda := time.Date(2026, 12, 9, 9, 0, 0, 0, time.UTC)
	out, err := RecortarSerie(serieConExcepciones, segunda, AmendExclude, OpcionesRecorte{SinExcepciones: true})
	if err != nil {
		t.Fatal(err)
	}
	if ev := vevents(out); len(ev) != 2 || ev["20261216T090000Z"] == "" || !strings.Contains(out, "EXDATE:20261209T090000Z") {
		t.Fatalf("solo este: sin la excepción de la segunda, con la tercera:\n%s", out)
	}
	out, err = RecortarSerie(serieConExcepciones, segunda, AmendUntil, OpcionesRecorte{SinExcepciones: true})
	if err != nil {
		t.Fatal(err)
	}
	if ev := vevents(out); len(ev) != 1 || !strings.Contains(out, "UNTIL=20261209T085959Z") {
		t.Fatalf("este y los siguientes: sin las dos excepciones:\n%s", out)
	}
	// Sin opciones (la edición conectada y el servicio), como siempre.
	out, err = AmendMasterRecurrence(serieConExcepciones, segunda, AmendUntil)
	if err != nil || len(vevents(out)) != 3 {
		t.Fatalf("sin opciones: %v\n%s", err, out)
	}
	madrid, _ := time.LoadLocation("Europe/Madrid")
	out, err = RecortarSerie(deICloud, time.Date(2026, 12, 23, 10, 0, 0, 0, madrid), AmendExclude, OpcionesRecorte{ComoElInicio: true})
	if err != nil || !strings.Contains(out, "EXDATE;TZID=Europe/Madrid:20261223T100000\r\n") {
		t.Fatalf("la EXDATE como el inicio: %v\n%s", err, out)
	}
}

// «Solo este»: el evento suelto lleva lo que contestó cada invitado a ESA ocurrencia
// según lo guardado al dividir: el de su excepción, si una respuesta la creó mientras el
// formulario estaba abierto, o el del maestro. A quien la persona quitó no se le añade.
func TestDividirSoloEsteLlevaLasRespuestasDeEsaOcurrencia(t *testing.T) {
	// Lo que leyó el formulario al abrir la segunda (sin excepción entonces): Luis a la
	// espera y Marta, que la persona quitó.
	suelto := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\nBEGIN:VEVENT\r\nUID:nuevo@sytue\r\nDTSTAMP:20261120T080000Z\r\n" +
		"DTSTART:20261209T090000Z\r\nDTEND:20261209T100000Z\r\nSUMMARY:Seguimiento (solo esta)\r\nORGANIZER:mailto:ana@example.org\r\n" +
		"ATTENDEE;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT:mailto:luis@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	out, err := DividirConRespuestas(suelto, serieConExcepciones, time.Date(2026, 12, 9, 9, 0, 0, 0, time.UTC), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "PARTSTAT=DECLINED;ROLE=REQ-PARTICIPANT:mailto:luis@") || strings.Contains(out, "marta@") ||
		!strings.Contains(out, "SUMMARY:Seguimiento (solo esta)") || strings.Contains(out, "RECURRENCE-ID") {
		t.Fatalf("la respuesta de Luis a esa ocurrencia, sin Marta:\n%s", out)
	}
	// La cuarta no tiene excepción: lo del maestro (Marta, que aceptó la serie).
	cuarta := strings.NewReplacer("20261209T09", "20261223T09", "20261209T10", "20261223T10",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT:mailto:luis@example.org\r\n",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:marta@example.org\r\n").Replace(suelto)
	out, err = DividirConRespuestas(cuarta, serieConExcepciones, time.Date(2026, 12, 23, 9, 0, 0, 0, time.UTC), false, true)
	if err != nil || !strings.Contains(out, "PARTSTAT=ACCEPTED:mailto:marta@") || !strings.Contains(out, "PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT:mailto:luis@") {
		t.Fatalf("lo del maestro: %v\n%s", err, out)
	}
}

// «Este y los siguientes»: la serie nueva lleva lo contestado a la serie y las
// excepciones desde la ocurrencia, con su UID, lo que se contestó en cada una y lo que la
// persona cambió en la serie; las de antes se quedan en la vieja. Si la serie nueva
// cambia de hora, sus excepciones apuntarían a ocurrencias que no existen: no van.
func TestDividirEsteYLosSiguientesLlevaLasExcepciones(t *testing.T) {
	segunda := time.Date(2026, 12, 9, 9, 0, 0, 0, time.UTC)
	desde := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\nBEGIN:VEVENT\r\nUID:nuevo@sytue\r\nDTSTAMP:20261120T080000Z\r\n" +
		"DTSTART:20261209T090000Z\r\nDTEND:20261209T100000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:Seguimiento del presupuesto\r\nLOCATION:Sala 2\r\nORGANIZER:mailto:ana@example.org\r\n" +
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:luis@example.org\r\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:marta@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	out, err := DividirConRespuestas(desde, serieConExcepciones, segunda, true, true)
	if err != nil {
		t.Fatal(err)
	}
	ev := vevents(out)
	if len(ev) != 3 || strings.Contains(out, "UID:serie@prueba") || strings.Count(out, "UID:nuevo@sytue") != 3 {
		t.Fatalf("el maestro nuevo y las dos excepciones desde la segunda, con el UID nuevo:\n%s", out)
	}
	if !strings.Contains(ev[""], "PARTSTAT=ACCEPTED:mailto:marta@") {
		t.Fatalf("Marta aceptó la serie:\n%s", ev[""])
	}
	if de := ev["20261209T090000Z"]; !strings.Contains(de, "PARTSTAT=DECLINED:mailto:luis@") || !strings.Contains(de, "SUMMARY:Seguimiento del presupuesto") || strings.Contains(de, "SEQUENCE") {
		t.Fatalf("la de Luis, con su respuesta y el título nuevo:\n%s", de)
	}
	if c := ev["20261216T090000Z"]; !strings.Contains(c, "SUMMARY:Seguimiento (con la dirección)") || !strings.Contains(c, "DTSTART:20261216T100000Z") {
		t.Fatalf("la cambiada a propósito, como estaba:\n%s", c)
	}
	// A otra hora: solo lo contestado a la serie.
	out, err = DividirConRespuestas(strings.ReplaceAll(desde, "T09", "T11"), serieConExcepciones, segunda, true, true)
	if err != nil || len(vevents(out)) != 1 || !strings.Contains(out, "PARTSTAT=ACCEPTED:mailto:marta@") {
		t.Fatalf("a otra hora: %v\n%s", err, out)
	}
}

// Un texto con escapes («\,», «\n») se lee como lo lee una persona: antes el título de
// iCloud se enseñaba con la barra y cada guardado la volvía a escapar.
func TestLosTextosSeLeenSinEscapes(t *testing.T) {
	d := strings.Replace(deICloud, "LOCATION:Sala 2", "LOCATION:Sala 2\\; planta 1\r\nDESCRIPTION:Orden del día:\\n1. Cuentas\\, gastos", 1)
	po, err := DecodeCalendarObject(d, "")
	if err != nil {
		t.Fatal(err)
	}
	if m := po.Master; m.Summary != "Seguimiento, semanal" || m.Location != "Sala 2; planta 1" || m.Description != "Orden del día:\n1. Cuentas, gastos" {
		t.Fatalf("%q %q %q", m.Summary, m.Location, m.Description)
	}
	ev := domain.Event{UID: "x", Summary: po.Master.Summary, Description: po.Master.Description, DTStart: po.Master.DTStart, DTEnd: po.Master.DTEnd}
	out, err := EncodeEvent(ev, "")
	if err != nil || !strings.Contains(out, "SUMMARY:Seguimiento\\, semanal\r\n") || !strings.Contains(out, "DESCRIPTION:Orden del día:\\n1. Cuentas\\, gastos\r\n") {
		t.Fatalf("se escapa una sola vez: %v\n%s", err, out)
	}
}
