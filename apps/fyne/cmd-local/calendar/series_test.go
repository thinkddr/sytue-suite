// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// mesaSerie es una serie semanal de Ana en un calendario, como la trae una cuenta
// (iCloud): en Madrid, sin final, sin la quinta (la EXDATE en la hora de su zona), sin
// STATUS y con Luis y Marta sin contestar. Se edita con el formulario de verdad.
type mesaSerie struct {
	t      *testing.T
	repo   *privateCalendar.Repository
	facade *api.Calendars
	cal    string
	p      *calendario.Pantalla
	inicio time.Time
}

const uidSerie = "serie@icloud"

var madrid = func() *time.Location { l, _ := time.LoadLocation("Europe/Madrid"); return l }()

// hora es t en la hora de Madrid, como la escribe iCloud.
func hora(t time.Time) string { return t.In(madrid).Format("20060102T150405") }

func serieDeLaCuenta(inicio time.Time) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\nBEGIN:VEVENT\r\nUID:" + uidSerie + "\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:1\r\n" +
		"DTSTART;TZID=Europe/Madrid:" + hora(inicio) + "\r\nDTEND;TZID=Europe/Madrid:" + hora(inicio.Add(time.Hour)) + "\r\n" +
		"RRULE:FREQ=WEEKLY\r\nEXDATE;TZID=Europe/Madrid:" + hora(inicio.AddDate(0, 0, 28)) + "\r\n" +
		"SUMMARY:Seguimiento\r\nLOCATION:Sala 2\r\nORGANIZER;CN=Ana:mailto:ana@example.org\r\n" +
		"ATTENDEE;CN=Luis;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:luis@example.org\r\n" +
		"ATTENDEE;CN=Marta;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:marta@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// excepcionSemana es la excepción de la ocurrencia de la semana n, como la crea la
// respuesta de un invitado (`agenda.AplicarRespuestas`: la copia del maestro en su hora),
// con lo que contestó cada uno, el título y movida `movida`.
func (m *mesaSerie) excepcionSemana(n int, luis, marta, titulo string, movida time.Duration) string {
	w := m.inicio.AddDate(0, 0, 7*n)
	return "BEGIN:VEVENT\r\nUID:" + uidSerie + "\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:1\r\nRECURRENCE-ID;TZID=Europe/Madrid:" + hora(w) + "\r\n" +
		"DTSTART;TZID=Europe/Madrid:" + hora(w.Add(movida)) + "\r\nDTEND;TZID=Europe/Madrid:" + hora(w.Add(movida+time.Hour)) + "\r\n" +
		"SUMMARY:" + titulo + "\r\nLOCATION:Sala 2\r\nORGANIZER;CN=Ana:mailto:ana@example.org\r\n" +
		"ATTENDEE;CN=Luis;PARTSTAT=" + luis + ";RSVP=TRUE:mailto:luis@example.org\r\n" +
		"ATTENDEE;CN=Marta;PARTSTAT=" + marta + ";RSVP=TRUE:mailto:marta@example.org\r\nEND:VEVENT\r\n"
}

func nuevaMesa(t *testing.T, nuevo func(*almacen.Recursos) (*privateCalendar.Repository, error), datos func(time.Time) string) *mesaSerie {
	t.Helper()
	repo := abrir(t, t.TempDir(), "calendar", nuevo)
	t.Cleanup(repo.Close)
	cal, err := repo.CreateCalendar(t.Context(), api.CalendarInput{Name: "c", DisplayName: "iCloud", Timezone: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	hoy := time.Now().In(madrid).AddDate(0, 0, 2)
	m := &mesaSerie{
		t: t, repo: repo, cal: cal.ID, p: &calendario.Pantalla{Zona: madrid, SinPersonas: true},
		inicio: time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 10, 0, 0, 0, madrid),
	}
	if _, err := repo.PonerObjetoCrudo(t.Context(), cal.ID, "", "", datos(m.inicio)); err != nil {
		t.Fatal(err)
	}
	if m.facade, err = api.NewCalendars("", nil, nil).WithPrivateData(repo); err != nil {
		t.Fatal(err)
	}
	return m
}

// semana son las ocurrencias que se ven la semana n (de la serie o de lo que salió de ella).
func (m *mesaSerie) semana(n int) []api.Instance {
	m.t.Helper()
	w := m.inicio.AddDate(0, 0, 7*n)
	is, err := m.facade.Instances(m.t.Context(), m.cal, w.Add(-2*time.Hour), w.Add(3*time.Hour))
	if err != nil {
		m.t.Fatal(err)
	}
	return is
}

// abrirSemana es la ocurrencia de la serie de la semana n, como la abre el formulario.
func (m *mesaSerie) abrirSemana(n int) (calendario.Ocurrencia, calendario.Edicion) {
	m.t.Helper()
	for _, i := range m.semana(n) {
		if i.Event.UID == uidSerie && i.Event.RecurrenceID == nil {
			o := calendario.Ocurrencia{Instance: i, CalendarioID: m.cal}
			return o, m.p.EdicionDe(o)
		}
	}
	m.t.Fatalf("no está la ocurrencia de la semana %d: %v", n, m.objetos())
	return calendario.Ocurrencia{}, calendario.Edicion{}
}

// objetos son los eventos del calendario por UID, tal como están guardados.
func (m *mesaSerie) objetos() map[string]string {
	m.t.Helper()
	lista, err := m.repo.ObjetosCrudos(m.t.Context(), m.cal)
	if err != nil {
		m.t.Fatal(err)
	}
	out := map[string]string{}
	for _, o := range lista {
		out[o.UID] = o.Datos
	}
	return out
}

// contestar cambia la serie guardada como lo hace una respuesta que llega mientras el
// formulario está abierto.
func (m *mesaSerie) contestar(cambiar func(string) string) {
	m.t.Helper()
	lista, err := m.repo.ObjetosCrudos(m.t.Context(), m.cal)
	if err != nil {
		m.t.Fatal(err)
	}
	for _, o := range lista {
		if o.UID == uidSerie {
			if _, err := m.repo.PonerObjetoCrudo(m.t.Context(), m.cal, o.ID, o.Datos, cambiar(o.Datos)); err != nil {
				m.t.Fatal(err)
			}
			return
		}
	}
	m.t.Fatal("no está la serie")
}

// conExcepcion añade una excepción a los datos de la serie.
func conExcepcion(d, ex string) string {
	return strings.Replace(d, "END:VCALENDAR", strings.TrimSuffix(ex, "\r\n")+"\r\nEND:VCALENDAR", 1)
}

// martaAceptaLaSerie es la respuesta de Marta a la serie entera: en el maestro y en las
// excepciones.
func martaAceptaLaSerie(d string) string {
	return strings.ReplaceAll(d, "PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:marta@", "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:marta@")
}

// «Solo este» y «este y los siguientes» con el formulario de verdad y el repositorio
// como lo abre Sytue Office: lo que los invitados contestan mientras el formulario está
// abierto va al evento nuevo, la ocurrencia no se ve dos veces y la EXDATE nueva va en la
// hora de Madrid, como el resto.
func TestPartirLaSerieLlevaLoContestadoEntreMedias(t *testing.T) {
	m := nuevaMesa(t, nuevoRepositorio, serieDeLaCuenta)
	ctx := t.Context()

	// «Solo este» en la segunda: Ana la abre y le cambia el título; mientras tanto Luis
	// declina ESA ocurrencia (se crea su excepción) y Marta acepta la serie.
	oc, e := m.abrirSemana(1)
	e.Titulo = "Seguimiento (en la sala grande)"
	m.contestar(func(d string) string {
		return martaAceptaLaSerie(conExcepcion(d, m.excepcionSemana(1, "DECLINED", "NEEDS-ACTION", "Seguimiento", 0)))
	})
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceEste, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	obj := m.objetos()
	if len(obj) != 2 {
		t.Fatalf("la serie y la ocurrencia suelta: %v", obj)
	}
	var suelta string
	for uid, d := range obj {
		if uid != uidSerie {
			suelta = d
		}
	}
	// Con el organizador de la serie (el formulario no lo manda al partir): es a quien
	// contestan los invitados.
	if !strings.Contains(suelta, "SUMMARY:Seguimiento (en la sala grande)") || strings.Contains(suelta, "RRULE") ||
		!strings.Contains(suelta, "ORGANIZER;CN=Ana:mailto:ana@example.org") ||
		!strings.Contains(suelta, "PARTSTAT=DECLINED") || !strings.Contains(suelta, "PARTSTAT=ACCEPTED") || strings.Contains(suelta, "NEEDS-ACTION") {
		t.Fatalf("la suelta, con lo que contestaron Luis a ella y Marta a la serie: %s", suelta)
	}
	serie := obj[uidSerie]
	if strings.Contains(serie, "RECURRENCE-ID") || !strings.Contains(serie, "EXDATE;TZID=Europe/Madrid:"+hora(m.inicio.AddDate(0, 0, 7))+"\r\n") ||
		!strings.Contains(serie, "EXDATE;TZID=Europe/Madrid:"+hora(m.inicio.AddDate(0, 0, 28))+"\r\n") {
		t.Fatalf("la serie sin esa ocurrencia (ni su excepción), con la EXDATE en la hora de Madrid: %s", serie)
	}
	if is := m.semana(1); len(is) != 1 || is[0].Event.Summary != "Seguimiento (en la sala grande)" {
		t.Fatalf("la segunda se ve una vez, la suelta: %+v", is)
	}

	// «Este y los siguientes» en la tercera: Ana le cambia el título; mientras tanto Luis
	// declina la cuarta.
	oc, e = m.abrirSemana(2)
	e.Titulo = "Seguimiento (segunda etapa)"
	m.contestar(func(d string) string {
		return conExcepcion(d, m.excepcionSemana(3, "DECLINED", "ACCEPTED", "Seguimiento", 0))
	})
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceSiguientes, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	obj = m.objetos()
	if len(obj) != 3 {
		t.Fatalf("la serie, la suelta y la serie nueva: %v", obj)
	}
	var nueva string
	for _, d := range obj {
		if strings.Contains(d, "Seguimiento (segunda etapa)") {
			nueva = d
		}
	}
	eventos := strings.Split(nueva, "BEGIN:VEVENT")
	if len(eventos) != 3 {
		t.Fatalf("la serie nueva con la excepción de la cuarta: %s", nueva)
	}
	maestro, ex := eventos[1], eventos[2]
	if strings.Contains(maestro, "RECURRENCE-ID") {
		maestro, ex = ex, maestro
	}
	if !strings.Contains(maestro, "RRULE:FREQ=WEEKLY") || !strings.Contains(maestro, "PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:marta@") {
		t.Fatalf("la serie nueva con la respuesta de Marta: %s", maestro)
	}
	if !strings.Contains(ex, "RECURRENCE-ID;TZID=Europe/Madrid:"+hora(m.inicio.AddDate(0, 0, 21))) || strings.Contains(ex, uidSerie) ||
		!strings.Contains(ex, "PARTSTAT=DECLINED;RSVP=TRUE:mailto:luis@") || !strings.Contains(ex, "SUMMARY:Seguimiento (segunda etapa)") {
		t.Fatalf("la cuarta, en la serie nueva, con la respuesta de Luis y el título nuevo: %s", ex)
	}
	if serie := m.objetos()[uidSerie]; strings.Contains(serie, "RECURRENCE-ID") || !strings.Contains(serie, "UNTIL=") {
		t.Fatalf("la serie de antes acaba antes de la tercera y sin excepciones: %s", serie)
	}
	if is := m.semana(3); len(is) != 1 || is[0].Event.Summary != "Seguimiento (segunda etapa)" {
		t.Fatalf("la cuarta se ve una vez, la de la serie nueva: %+v", is)
	}
}

// Sin las opciones de Sytue Office (la edición conectada), partir la serie es como
// siempre: el evento nuevo lleva lo que se leyó al abrir y la excepción se queda.
func TestSinLasOpcionesPartirLaSerieEsComoSiempre(t *testing.T) {
	m := nuevaMesa(t, func(r *almacen.Recursos) (*privateCalendar.Repository, error) { return privateCalendar.New(r.Config) }, serieDeLaCuenta)
	oc, e := m.abrirSemana(1)
	m.contestar(func(d string) string {
		return conExcepcion(d, m.excepcionSemana(1, "DECLINED", "NEEDS-ACTION", "Seguimiento", 0))
	})
	if err := calendario.EditarSerie(t.Context(), m.facade, oc, calendario.AlcanceEste, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	obj := m.objetos()
	for uid, d := range obj {
		if uid != uidSerie && strings.Contains(d, "DECLINED") {
			t.Fatalf("sin la opción no se lleva la respuesta: %s", d)
		}
	}
	if serie := obj[uidSerie]; !strings.Contains(serie, "RECURRENCE-ID") || !strings.Contains(serie, "EXDATE:") {
		t.Fatalf("sin la opción, la excepción se queda y la EXDATE va en UTC: %s", serie)
	}
}

// lineasSin son las líneas de un VCALENDAR (sin plegar), sin las que cambian al
// reescribirlo aunque no cambie nada (DTSTAMP, PRODID).
func lineasSin(d string) map[string]bool {
	out := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(d, "\r\n ", ""), "\r\n") {
		if l != "" && !strings.HasPrefix(l, "DTSTAMP") && !strings.HasPrefix(l, "PRODID") {
			out[l] = true
		}
	}
	return out
}

// Guardar «todos» desde el formulario sin cambiar nada deja el evento de la cuenta
// escrito como vino (la EXDATE con su zona, sin STATUS, la misma SEQUENCE), también sus
// excepciones; cambiar el título lo lleva a la excepción que creó una respuesta (y que lo
// tenía como la serie) y no a la que Ana cambió a propósito, que tampoco se mueve.
func TestGuardarLaSerieSinCambiosNoLaReescribeYLoCambiadoLlegaASusExcepciones(t *testing.T) {
	var original string
	m := nuevaMesa(t, nuevoRepositorio, func(inicio time.Time) string {
		aux := &mesaSerie{inicio: inicio}
		original = conExcepcion(conExcepcion(serieDeLaCuenta(inicio),
			aux.excepcionSemana(1, "DECLINED", "NEEDS-ACTION", "Seguimiento", 0)),
			aux.excepcionSemana(2, "NEEDS-ACTION", "NEEDS-ACTION", "Seguimiento (con la dirección)", time.Hour))
		return original
	})
	ctx := t.Context()
	oc, e := m.abrirSemana(0)
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	guardado := m.objetos()[uidSerie]
	quiero, hay := lineasSin(original), lineasSin(guardado)
	for l := range quiero {
		if !hay[l] {
			t.Errorf("falta, tal cual, %q", l)
		}
	}
	for l := range hay {
		if !quiero[l] {
			t.Errorf("sobra %q", l)
		}
	}
	if t.Failed() {
		t.Fatalf("guardado:\n%s", guardado)
	}

	oc, e = m.abrirSemana(0)
	e.Titulo = "Seguimiento del presupuesto"
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	if is := m.semana(1); len(is) != 1 || is[0].Event.Summary != "Seguimiento del presupuesto" || is[0].Event.RecurrenceID == nil {
		t.Fatalf("la excepción de la respuesta de Luis, con el título nuevo: %+v", is)
	}
	is := m.semana(2)
	if len(is) != 1 || is[0].Event.Summary != "Seguimiento (con la dirección)" || !is[0].Start.Equal(m.inicio.AddDate(0, 0, 14).Add(time.Hour)) {
		t.Fatalf("la cambiada a propósito, con su título y a su hora: %+v", is)
	}
	if d := m.objetos()[uidSerie]; !strings.Contains(d, "SEQUENCE:2") || strings.Contains(d, "STATUS") {
		t.Fatalf("el título sí es un cambio para los invitados: %s", d)
	}
}

// mismasLineas compara dos VCALENDAR por sus líneas (lineasSin).
func mismasLineas(t *testing.T, original, guardado string) {
	t.Helper()
	quiero, hay := lineasSin(original), lineasSin(guardado)
	for l := range quiero {
		if !hay[l] {
			t.Errorf("falta, tal cual, %q", l)
		}
	}
	for l := range hay {
		if !quiero[l] {
			t.Errorf("sobra %q", l)
		}
	}
}

// Guardar «todos» desde la tercera ocurrencia sin tocar nada deja la serie como estaba:
// empieza donde empezaba (antes se anclaba en la tercera y se perdían las dos primeras,
// porque la ocurrencia trae en su DTSTART el suyo y se tomaba por el del maestro) y se
// escribe tal cual. Moverla una hora desde la tercera mueve la serie entera una hora,
// desde su primera.
func TestGuardarTodosDesdeOtraOcurrenciaNoAnclaLaSerie(t *testing.T) {
	m := nuevaMesa(t, nuevoRepositorio, serieDeLaCuenta)
	ctx := t.Context()
	original := m.objetos()[uidSerie]
	oc, e := m.abrirSemana(2)
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	guardado := m.objetos()[uidSerie]
	mismasLineas(t, original, guardado)
	for n := range 4 {
		if is := m.semana(n); len(is) != 1 || !is[0].Start.Equal(m.inicio.AddDate(0, 0, 7*n)) {
			t.Errorf("la semana %d: %+v", n, is)
		}
	}
	if t.Failed() {
		t.Fatalf("guardado:\n%s", guardado)
	}

	oc, e = m.abrirSemana(2)
	e.Inicio, e.Fin = e.Inicio.Add(time.Hour), e.Fin.Add(time.Hour)
	if err := calendario.EditarSerie(ctx, m.facade, oc, calendario.AlcanceTodos, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	if d := m.objetos()[uidSerie]; !strings.Contains(d, "DTSTART;TZID=Europe/Madrid:"+hora(m.inicio.Add(time.Hour))+"\r\n") {
		t.Fatalf("la serie, una hora más tarde desde su primera: %s", d)
	}
	for n := range 4 {
		if is := m.semana(n); len(is) != 1 || !is[0].Start.Equal(m.inicio.AddDate(0, 0, 7*n).Add(time.Hour)) {
			t.Errorf("la semana %d, una hora más tarde: %+v", n, is)
		}
	}
}

// «Este y los siguientes» desde la tercera de una serie de 6 deja a la nueva las 4 que
// quedan (antes salía con COUNT=6 y la serie acababa dos semanas más tarde), y entre las
// dos se ven las mismas semanas que antes (la quinta, quitada, sigue quitada).
func TestPartirLaSerieConCuentaDescuentaLasDeAntes(t *testing.T) {
	m := nuevaMesa(t, nuevoRepositorio, func(inicio time.Time) string {
		return strings.Replace(serieDeLaCuenta(inicio), "RRULE:FREQ=WEEKLY\r\n", "RRULE:FREQ=WEEKLY;COUNT=6\r\n", 1)
	})
	visibles := func() []int {
		var out []int
		for n := range 10 {
			if len(m.semana(n)) > 0 {
				out = append(out, n)
			}
		}
		return out
	}
	antes := visibles()
	if !slices.Equal(antes, []int{0, 1, 2, 3, 5}) {
		t.Fatalf("la serie de 6 sin la quinta se ve %v", antes)
	}
	oc, e := m.abrirSemana(2)
	e.Titulo = "Seguimiento (segunda etapa)"
	if err := calendario.EditarSerie(t.Context(), m.facade, oc, calendario.AlcanceSiguientes, e.EntradaDe(m.p), e.Regla); err != nil {
		t.Fatal(err)
	}
	var nueva string
	for uid, d := range m.objetos() {
		if uid != uidSerie {
			nueva = d
		}
	}
	if !strings.Contains(nueva, "RRULE:FREQ=WEEKLY;COUNT=4\r\n") || !strings.Contains(nueva, "DTSTART;TZID=Europe/Madrid:"+hora(m.inicio.AddDate(0, 0, 14))) {
		t.Fatalf("la serie nueva, desde la tercera y con las 4 que quedan: %s", nueva)
	}
	if despues := visibles(); !slices.Equal(despues, antes) {
		t.Fatalf("entre las dos series se ven las semanas %v y antes %v", despues, antes)
	}
}
