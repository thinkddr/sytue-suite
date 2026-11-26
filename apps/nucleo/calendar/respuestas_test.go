// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const reunionConInvitados = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:reunion@prueba\r\nDTSTAMP:20260101T000000Z\r\nSEQUENCE:2\r\nDTSTART:20261210T090000Z\r\nDTEND:20261210T100000Z\r\n" +
	"SUMMARY:Revisión\r\nSTATUS:CONFIRMED\r\nORGANIZER:mailto:ana@example.org\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;ROLE=REQ-PARTICIPANT;RSVP=TRUE:mailto:luis@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// guardarConRespuestaEntreMedias abre el evento (lo que lee el formulario), apunta en el
// almacén la respuesta de Luis (lo que hace `local/agenda` con la que llega por correo) y
// guarda lo leído con un recordatorio: devuelve cómo queda el objeto.
func guardarConRespuestaEntreMedias(t *testing.T, conservar bool) string {
	t.Helper()
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	repo.ConservarRespuestas = conservar
	ctx := t.Context()
	cal, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	o, err := repo.PonerObjetoCrudo(ctx, cal.ID, "", "", reunionConInvitados)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := repo.SearchEvents(ctx, "revision", 10)
	if err != nil || len(hits) != 1 {
		t.Fatal(hits, err)
	}
	ev, err := repo.GetEvent(ctx, hits[0].ID) // abrir
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, o.ID, o.Datos, strings.Replace(o.Datos, "PARTSTAT=NEEDS-ACTION", "PARTSTAT=ACCEPTED", 1)); err != nil {
		t.Fatal(err)
	}
	in := ev.Input() // guardar
	in.Reminders = &[]int{15}
	if _, err := repo.UpdateEvent(ctx, ev.ID, in); err != nil {
		t.Fatal(err)
	}
	lista, err := repo.ObjetosCrudos(ctx, cal.ID)
	if err != nil || len(lista) != 1 {
		t.Fatal(lista, err)
	}
	return lista[0].Datos
}

// Con ConservarRespuestas (Sytue Office), guardar lo que se leyó al abrir no pisa la
// respuesta que se apuntó entre medias, y un recordatorio no sube la SEQUENCE. Sin la
// opción (la edición conectada), todo sigue como siempre: manda lo que llega.
func TestConservarRespuestasAlGuardar(t *testing.T) {
	secuencia := func(d string) string {
		_, v, _ := strings.Cut(d, "\r\nSEQUENCE")
		v, _, _ = strings.Cut(v, "\r\n")
		_, v, _ = strings.Cut(v, ":")
		return v
	}
	d := guardarConRespuestaEntreMedias(t, true)
	if !strings.Contains(d, "PARTSTAT=ACCEPTED") || !strings.Contains(d, "TRIGGER:-PT15M") {
		t.Fatalf("con la opción: %s", d)
	}
	if secuencia(d) != "2" {
		t.Fatalf("un recordatorio no es una revisión de la invitación: %s", d)
	}
	d = guardarConRespuestaEntreMedias(t, false)
	if !strings.Contains(d, "PARTSTAT=NEEDS-ACTION") || secuencia(d) != "3" {
		t.Fatalf("sin la opción, como siempre: %s", d)
	}
}
