// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
)

func conHoras(uid string, ini, fin time.Time, mods ...func(*api.Event)) Ocurrencia {
	ev := api.Event{ID: uid, UID: uid, Summary: uid}
	for _, m := range mods {
		m(&ev)
	}
	return Ocurrencia{Instance: api.Instance{Event: ev, Start: ini, End: fin}}
}

// TestSolapesComoLaWeb: los mismos casos que `lib/solapes.test.ts`. Tocar el borde no es
// pisarse; el propio evento, el día completo, lo disponible y lo rechazado no cuentan; y
// el mismo evento visto desde dos calendarios, una vez.
func TestSolapesComoLaWeb(t *testing.T) {
	h := func(hh, mm int) time.Time { return time.Date(2026, 11, 30, hh, mm, 0, 0, time.UTC) }
	diez, once := h(10, 0), h(11, 0)
	rechazo := func(e *api.Event) {
		e.Attendees = []api.Attendee{{Email: "ana@sytue.eu", PartStat: "DECLINED"}}
	}
	pisa := conHoras("pisa", h(10, 30), h(11, 30))
	eventos := []Ocurrencia{
		conHoras("antes", h(9, 0), diez),
		pisa,
		conHoras("dentro", h(10, 15), h(10, 45)),
		conHoras("despues", once, h(12, 0)),
		conHoras("yo", diez, once),
		conHoras("cumple", h(0, 0), h(24, 0), func(e *api.Event) { e.AllDay = true }),
		conHoras("libre", diez, once, func(e *api.Event) { e.Transparency = "TRANSPARENT" }),
		conHoras("rechazado", diez, once, rechazo),
		pisa, // «pisa» desde otro calendario
	}
	uids := make([]string, 0, 2)
	for _, s := range Solapes(eventos, diez, once, "yo", "Ana@sytue.eu", time.UTC) {
		uids = append(uids, s.O.Event.UID)
	}
	if want := []string{"dentro", "pisa"}; !slices.Equal(uids, want) {
		t.Errorf("solapes = %v, want %v", uids, want)
	}
	if got := Solapes(eventos, once, diez, "", "", time.UTC); got != nil {
		t.Errorf("un tramo al revés se pisa con %v", got)
	}

	// Lo rechazado se esconde solo con el ajuste apagado y solo si es MI respuesta.
	aj := AjustesPorDefecto()
	if OcultoPorRechazo(aj, eventos[7], "ana@sytue.eu") {
		t.Error("con el ajuste por defecto se esconde lo rechazado")
	}
	aj.Rechazados = false
	if !OcultoPorRechazo(aj, eventos[7], "ANA@sytue.eu") || OcultoPorRechazo(aj, eventos[7], "luis@sytue.eu") {
		t.Error("el ajuste apagado no esconde lo que YO he rechazado (o esconde lo de otros)")
	}
	// Un JSON de antes de que existiera el ajuste no esconde nada.
	if !LeerAjustes(`{"duracion":30}`).Rechazados || LeerAjustes(`{"rechazados":false}`).Rechazados {
		t.Error("«rechazados» no se lee como en la web")
	}
}

// TestDisponibilidadComoLaWeb: los mismos huecos y el mismo texto que
// `lib/disponibilidad.test.ts`, agrupados en la zona de quien comparte.
func TestDisponibilidadComoLaWeb(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata")
	}
	u := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	huecos := []api.Hueco{
		{Start: u("2026-11-30T14:00:00Z"), End: u("2026-11-30T17:00:00Z")},
		{Start: u("2026-11-30T08:00:00Z"), End: u("2026-11-30T09:30:00Z")},
		{Start: u("2026-11-29T23:30:00Z"), End: u("2026-11-30T00:00:00Z")},
		{Start: u("2026-12-01T09:00:00Z"), End: u("2026-12-01T10:00:00Z")},
		{Start: u("2026-12-01T12:00:00Z"), End: u("2026-12-01T11:00:00Z")},
	}
	dias := DiasDisponibles(huecos, madrid, i18n.Locale("en"))
	if len(dias) != 2 || !slices.Equal(dias[0].Tramos, []string{"00:30–01:00", "09:00–10:30", "15:00–18:00"}) ||
		!slices.Equal(dias[1].Tramos, []string{"10:00–11:00"}) {
		t.Fatalf("días = %+v", dias)
	}
	tr, err := i18n.New("en", i18n.Common(), catalogos.Calendar())
	if err != nil {
		t.Fatal(err)
	}
	p := &Pantalla{T: tr}
	texto := p.TextoDisponibilidad([]DiaDisponible{
		{Dia: "Mon 30 Nov", Tramos: []string{"09:00–10:30", "15:00–18:00"}},
		{Dia: "Tue 1 Dec", Tramos: []string{"10:00–11:00"}},
	}, "Europe/Madrid")
	want := strings.Join([]string{
		"My availability (Europe/Madrid):",
		"Mon 30 Nov: 09:00–10:30, 15:00–18:00",
		"Tue 1 Dec: 10:00–11:00",
	}, "\n")
	if texto != want {
		t.Errorf("texto =\n%s\nwant\n%s", texto, want)
	}
	if p.TextoDisponibilidad(nil, "Europe/Madrid") != "" {
		t.Error("sin huecos se copia algo")
	}
}
