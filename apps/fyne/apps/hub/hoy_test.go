// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestCadaEventoDeHoyAbreSuDiaEnCalendar: pulsar un evento de «Hoy» abre Calendar en SU
// día —el de la instancia, con `all_day=1` si es de día entero—, que es el enlace que
// Calendar atiende; con Calendar sustituido, la aplicación elegida.
//
// Visto en ROJO a mano el 2026-11-05: con la fila `Inerte` como antes, pulsarla no abre
// nada; y sin el `all_day=1` del de día entero, Calendar lo lee a medianoche y no a
// mediodía (el día que no se mueve con la zona), y el test lo dice.
func TestCadaEventoDeHoyAbreSuDiaEnCalendar(t *testing.T) {
	a := func(h int) time.Time { return time.Date(2026, 9, 21, h, 30, 0, 0, time.UTC) }
	l := lanzadorDePrueba(t)
	l.Agenda = Agenda{Estado: Listo, Eventos: []EventoDeHoy{
		{ID: "1", Resumen: "Reunión", Inicio: a(9), Fin: a(10)},
		{ID: "0", Resumen: "Cierre", Inicio: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), TodoElDia: true},
	}}
	var producto, ruta string
	l.AlAbrir = func(p Producto, r string) { producto, ruta = p.Marca.Clave(), r }
	_, r := pinta(t, l, ventana)

	nueva := time.FixedZone("UTC-5", -5*3600)
	for i, quiero := range []time.Time{
		time.Date(2026, 9, 21, 12, 0, 0, 0, nueva), // el de día entero va delante
		a(9).In(nueva),
	} {
		f, ok := r.filasHoy[i].(*FilaEvento)
		if !ok {
			t.Fatalf("la fila %d de «Hoy» no es un evento", i)
		}
		if f.AccessibilityRole() != fyne.AccessibleRoleLink {
			t.Errorf("la fila %d no se anuncia como enlace", i)
		}
		producto, ruta = "", ""
		test.Tap(f)
		if producto != "calendar" || !strings.HasPrefix(ruta, "/?") {
			t.Fatalf("pulsar «%s» abre %q %q y tenía que abrir su día en Calendar", f.Resumen, producto, ruta)
		}
		u, err := url.Parse(ruta)
		if err != nil {
			t.Fatal(err)
		}
		dia, ok := calendario.FechaDelEnlace(u.Query(), nueva)
		if !ok || !dia.Equal(quiero) {
			t.Errorf("Calendar lee %q como %v (%v) y es %v", ruta, dia, ok, quiero)
		}
	}

	var fuera string
	l.Sustitutos = map[string]api.Sustituto{"calendar": {Href: "https://calendar.example.eu/"}}
	l.AlAbrirURL = func(s string) { fuera = s }
	_, r = pinta(t, l, ventana)
	test.Tap(r.filasHoy[0].(*FilaEvento))
	if fuera != "https://calendar.example.eu/" {
		t.Errorf("con Calendar sustituido, el evento abre %q", fuera)
	}
}
