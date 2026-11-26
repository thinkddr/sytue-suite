// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// serieDeAna es una reunión semanal que organiza Ana, con Luis y Marta sin contestar.
func serieDeAna(inicio time.Time) string {
	h := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20260101T000000Z\r\nSEQUENCE:1\r\n" +
		"DTSTART:" + h(inicio) + "\r\nDTEND:" + h(inicio.Add(time.Hour)) + "\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:Seguimiento\r\nSTATUS:CONFIRMED\r\n" +
		"ORGANIZER;CN=Ana:mailto:ana@example.org\r\nATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@example.org\r\n" +
		"ATTENDEE;CN=Marta;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:marta@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// Con el formulario de verdad (`calendario.Pantalla.EdicionDe` al abrir, `EntradaDe` y
// `EditarSerie` al guardar «todos») sobre el repositorio como lo abre Sytue Office: lo que
// contestan los invitados mientras el formulario está abierto —Marta a la serie, Luis a
// UNA ocurrencia— se queda al guardar el título nuevo, y quitar después a Luis lo quita de
// toda la serie.
func TestGuardarElFormularioConservaLasRespuestasDeEntreMedias(t *testing.T) {
	repo := abrir(t, t.TempDir(), "calendar", nuevoRepositorio)
	t.Cleanup(repo.Close)
	ctx := t.Context()
	cal, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "c", DisplayName: "Nextcloud", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	inicio := time.Now().AddDate(0, 0, 2).UTC().Truncate(time.Hour)
	o, err := repo.PonerObjetoCrudo(ctx, cal.ID, "", "", serieDeAna(inicio))
	if err != nil {
		t.Fatal(err)
	}
	facade, err := api.NewCalendars("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	p := &calendario.Pantalla{Zona: time.UTC, SinPersonas: true}
	abrirPrimera := func() calendario.Ocurrencia {
		t.Helper()
		is, err := facade.Instances(ctx, cal.ID, inicio.Add(-time.Hour), inicio.Add(time.Hour))
		if err != nil || len(is) != 1 {
			t.Fatal(is, err)
		}
		return calendario.Ocurrencia{Instance: is[0], CalendarioID: cal.ID}
	}

	// Ana abre la primera ocurrencia y cambia el título.
	oc := abrirPrimera()
	e := p.EdicionDe(oc)
	e.Titulo = "Seguimiento del presupuesto"
	// Mientras tanto (lo que deja `agenda.AplicarRespuestas`): Marta acepta la serie y Luis
	// declina la segunda semana, con su excepción.
	rid := inicio.AddDate(0, 0, 7).Format("20060102T150405Z")
	excepcion := "BEGIN:VEVENT\r\nUID:serie@prueba\r\nDTSTAMP:20260101T000000Z\r\nSEQUENCE:1\r\nRECURRENCE-ID:" + rid + "\r\nDTSTART:" + rid +
		"\r\nDTEND:" + inicio.AddDate(0, 0, 7).Add(time.Hour).Format("20060102T150405Z") + "\r\nSUMMARY:Seguimiento\r\nSTATUS:CONFIRMED\r\n" +
		"ORGANIZER;CN=Ana:mailto:ana@example.org\r\nATTENDEE;CN=Luis;PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@example.org\r\n" +
		"ATTENDEE;CN=Marta;PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:marta@example.org\r\nEND:VEVENT\r\n"
	contestada := strings.Replace(o.Datos, "PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:marta@", "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:marta@", 1)
	contestada = strings.Replace(contestada, "END:VCALENDAR", strings.TrimSuffix(excepcion, "\r\n")+"\r\nEND:VCALENDAR", 1)
	if _, err := repo.PonerObjetoCrudo(ctx, cal.ID, o.ID, o.Datos, contestada); err != nil {
		t.Fatal(err)
	}
	// Ana guarda «todos».
	if err := calendario.EditarSerie(ctx, facade, oc, calendario.AlcanceTodos, e.EntradaDe(p), e.Regla); err != nil {
		t.Fatal(err)
	}
	datos := func() string {
		lista, err := repo.ObjetosCrudos(ctx, cal.ID)
		if err != nil || len(lista) != 1 {
			t.Fatal(lista, err)
		}
		return lista[0].Datos
	}
	d := datos()
	eventos := strings.Split(d, "BEGIN:VEVENT")
	if len(eventos) != 3 {
		t.Fatalf("el maestro y la excepción de Luis: %s", d)
	}
	m, ex := eventos[1], eventos[2]
	if strings.Contains(m, "RECURRENCE-ID") {
		m, ex = ex, m
	}
	// Marta, escrita como venía (sin el ROLE ni el CUTYPE que pone el formulario).
	if !strings.Contains(m, "SUMMARY:Seguimiento del presupuesto") || !strings.Contains(m, "ATTENDEE;CN=Marta;PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:marta@") {
		t.Fatalf("el título de Ana con la respuesta de Marta: %s", m)
	}
	// La excepción de Luis tenía el título de la serie: le llega el nuevo.
	if !strings.Contains(ex, "PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@") || !strings.Contains(ex, "SUMMARY:Seguimiento del presupuesto") {
		t.Fatalf("la respuesta de Luis a su ocurrencia, con el título nuevo: %s", ex)
	}

	// Ana vuelve a abrir y quita a Luis: no queda en ninguna ocurrencia.
	oc = abrirPrimera()
	e = p.EdicionDe(oc)
	e.Invitados = []string{"marta@example.org"}
	if err := calendario.EditarSerie(ctx, facade, oc, calendario.AlcanceTodos, e.EntradaDe(p), e.Regla); err != nil {
		t.Fatal(err)
	}
	if d := datos(); strings.Contains(d, "luis@") || strings.Count(d, "PARTSTAT=ACCEPTED") != 2 {
		t.Fatalf("Luis fuera de toda la serie, Marta igual: %s", d)
	}
}
