// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// reunionConInvitados es una serie con dos invitados y un override que repite la
// lista de asistentes: el caso en que la respuesta tiene que llegar a los dos VEVENT.
const reunionConInvitados = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Sytue//Test//ES\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:reunion-1@calendar.sytue.eu\r\n" +
	"DTSTAMP:20260630T120000Z\r\n" +
	"DTSTART:20260701T090000Z\r\n" +
	"DTEND:20260701T100000Z\r\n" +
	"SUMMARY:Seguimiento\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=4\r\n" +
	"ORGANIZER:mailto:grace@sytue.eu\r\n" +
	"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:Ana@Sytue.eu\r\n" +
	"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:bob@sytue.eu\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:reunion-1@calendar.sytue.eu\r\n" +
	"DTSTAMP:20260630T120000Z\r\n" +
	"RECURRENCE-ID:20260708T090000Z\r\n" +
	"DTSTART:20260708T110000Z\r\n" +
	"DTEND:20260708T120000Z\r\n" +
	"SUMMARY:Seguimiento (movida)\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ana@sytue.eu\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:bob@sytue.eu\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// partstatsDe devuelve, por VEVENT, el PARTSTAT de cada asistente decodificado.
func partstatsDe(t *testing.T, data string) []map[string]domain.ParticipationStatus {
	t.Helper()
	po, err := DecodeCalendarObject(data, "cal")
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	out := make([]map[string]domain.ParticipationStatus, 0, 1+len(po.Overrides))
	for _, ev := range append([]domain.EventInput{po.Master}, po.Overrides...) {
		m := map[string]domain.ParticipationStatus{}
		for _, a := range ev.Attendees {
			m[strings.ToLower(a.Email)] = a.PartStat
		}
		out = append(out, m)
	}
	return out
}

func TestConPartStatCambiaSoloLaRespuestaDeQuienContesta(t *testing.T) {
	out, encontrado, err := ConPartStat(reunionConInvitados, "ana@sytue.eu", domain.PartTentative)
	if err != nil || !encontrado {
		t.Fatalf("ConPartStat = (%v, %v), quiero encontrado y sin error", encontrado, err)
	}
	vevents := partstatsDe(t, out)
	if len(vevents) != 2 {
		t.Fatalf("VEVENT = %d, quiero el maestro y el override", len(vevents))
	}
	for i, m := range vevents {
		if m["ana@sytue.eu"] != domain.PartTentative {
			t.Errorf("VEVENT %d: ana = %q, quiero TENTATIVE (maestro y override)", i, m["ana@sytue.eu"])
		}
		if m["bob@sytue.eu"] != domain.PartAccepted {
			t.Errorf("VEVENT %d: bob = %q, la respuesta de otro no se toca", i, m["bob@sytue.eu"])
		}
	}
	// Lo demás del recurso sobrevive: el aviso, la regla, el organizador.
	for _, debe := range []string{"BEGIN:VALARM", "RRULE:FREQ=WEEKLY;COUNT=4", "ORGANIZER:mailto:grace@sytue.eu", "RECURRENCE-ID:20260708T090000Z"} {
		if !strings.Contains(out, debe) {
			t.Errorf("se ha perdido %q:\n%s", debe, out)
		}
	}
}

func TestConPartStatVolverAResponderCambia(t *testing.T) {
	primera, _, err := ConPartStat(reunionConInvitados, "ana@sytue.eu", domain.PartTentative)
	if err != nil {
		t.Fatal(err)
	}
	segunda, encontrado, err := ConPartStat(primera, "ANA@sytue.eu", domain.PartDeclined)
	if err != nil || !encontrado {
		t.Fatalf("segunda respuesta = (%v, %v)", encontrado, err)
	}
	if got := partstatsDe(t, segunda)[0]["ana@sytue.eu"]; got != domain.PartDeclined {
		t.Errorf("ana = %q tras volver a responder, quiero DECLINED", got)
	}
}

func TestConPartStatSinCambiosDevuelveElMismoRecurso(t *testing.T) {
	// bob ya había aceptado: aceptar otra vez no re-serializa (ni gasta una escritura).
	out, encontrado, err := ConPartStat(reunionConInvitados, "bob@sytue.eu", domain.PartAccepted)
	if err != nil || !encontrado {
		t.Fatalf("ConPartStat = (%v, %v)", encontrado, err)
	}
	if out != reunionConInvitados {
		t.Error("una respuesta igual a la que había no debe reescribir el recurso")
	}
}

func TestConPartStatQuienNoEstaInvitadoNoTocaNada(t *testing.T) {
	for _, email := range []string{"eve@sytue.eu", "grace@sytue.eu", ""} {
		out, encontrado, err := ConPartStat(reunionConInvitados, email, domain.PartAccepted)
		if err != nil {
			t.Fatalf("%q: %v", email, err)
		}
		if encontrado || out != reunionConInvitados {
			t.Errorf("%q no es asistente: encontrado=%v y el recurso debe quedar igual", email, encontrado)
		}
	}
}

func TestConPartStatRechazaUnaRespuestaInventada(t *testing.T) {
	if _, _, err := ConPartStat(reunionConInvitados, "ana@sytue.eu", "MAYBE\r\nX-MAL:1"); err == nil {
		t.Error("un PARTSTAT fuera de RFC 5545 no se escribe en el .ics")
	}
}

// El REPLY para quien organiza desde fuera: método REPLY, del maestro, con el
// organizador y SOLO el asistente que contesta, con su respuesta nueva y sin RSVP.
func TestRespuestaEsUnReplyConSoloQuienContesta(t *testing.T) {
	ics, org, resumen, err := Respuesta(reunionConInvitados, "ANA@sytue.eu", domain.PartDeclined)
	if err != nil {
		t.Fatal(err)
	}
	if org != "grace@sytue.eu" || resumen != "Seguimiento" {
		t.Errorf("organizador %q, resumen %q", org, resumen)
	}
	for _, quiero := range []string{
		"METHOD:REPLY",
		"UID:reunion-1@calendar.sytue.eu",
		"ORGANIZER:mailto:grace@sytue.eu",
		"PARTSTAT=DECLINED",
		"mailto:Ana@Sytue.eu",
		"DTSTART:20260701T090000Z",
	} {
		if !strings.Contains(ics, quiero) {
			t.Errorf("falta %q en el REPLY:\n%s", quiero, ics)
		}
	}
	for _, sobra := range []string{"bob@sytue.eu", "RECURRENCE-ID", "VALARM", "RSVP", "NEEDS-ACTION", "RRULE"} {
		if strings.Contains(ics, sobra) {
			t.Errorf("el REPLY no debe llevar %q:\n%s", sobra, ics)
		}
	}
	if n := strings.Count(ics, "BEGIN:VEVENT"); n != 1 {
		t.Errorf("VEVENT = %d; quiero 1", n)
	}
}

// Sin organizador, sin figurar como invitado o con un PARTSTAT inventado no hay
// REPLY que mandar.
func TestRespuestaSinOrganizadorNiInvitado(t *testing.T) {
	if _, _, _, err := Respuesta(reunionConInvitados, "carla@sytue.eu", domain.PartAccepted); err == nil {
		t.Error("quien no está invitado no responde")
	}
	sinOrg := strings.Replace(reunionConInvitados, "ORGANIZER:mailto:grace@sytue.eu\r\n", "", 1)
	if _, _, _, err := Respuesta(sinOrg, "ana@sytue.eu", domain.PartAccepted); err == nil {
		t.Error("sin ORGANIZER no hay a quién responder")
	}
	if _, _, _, err := Respuesta(reunionConInvitados, "ana@sytue.eu", "QUIZAS"); err == nil {
		t.Error("PARTSTAT inventado")
	}
}
