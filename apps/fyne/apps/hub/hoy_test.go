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
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
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

// TestHoyDiceQuePasaAhoraYCuantoQueda: con reloj fijo a las 12:00, el evento en curso
// lleva «Ahora», el primero por venir «Siguiente» y el lugar va en la fila; con nueve
// eventos en el día se ven seis y el pie dice «+3» y lleva a Calendar; y un calendario
// caído pinta una línea de aviso encima. Es lo mismo que `hoy.test.ts` en la web.
//
// Visto en ROJO a mano el 2026-11-12: sin MarcasDeLaAgenda en hacerHoy ninguna fila
// lleva insignia; sin la fila de «+N» el pie solo tiene el correo; y sin el aviso,
// avisoHoy es nil.
func TestHoyDiceQuePasaAhoraYCuantoQueda(t *testing.T) {
	a := func(h, m int) time.Time { return time.Date(2026, 9, 21, h, m, 0, 0, time.UTC) }
	l := lanzadorDePrueba(t) // Ahora = 21/09/2026 12:00 UTC
	l.Agenda = Agenda{Estado: Listo, HayCorreo: true, SinLeer: 2, Total: 9, Parcial: true, Eventos: []EventoDeHoy{
		{ID: "1", Resumen: "Pasado", Inicio: a(9, 0), Fin: a(10, 0)},
		{ID: "2", Resumen: "En curso", Inicio: a(11, 30), Fin: a(12, 30), Lugar: "Sala Norte"},
		{ID: "3", Resumen: "Luego", Inicio: a(15, 0), Fin: a(16, 0)},
		{ID: "4", Resumen: "Más tarde", Inicio: a(16, 0), Fin: a(17, 0)},
		{ID: "5", Resumen: "Tarde", Inicio: a(17, 0), Fin: a(18, 0)},
		{ID: "6", Resumen: "Noche", Inicio: a(19, 0), Fin: a(20, 0)},
	}}
	_, r := pinta(t, l, ventana)

	var marcas []api.EstadoDeEvento
	var pies []*FilaCorreo
	for _, o := range r.filasHoy {
		switch f := o.(type) {
		case *FilaEvento:
			marcas = append(marcas, f.Marca)
			if f.Resumen == "En curso" {
				if f.Lugar != "Sala Norte" || !strings.Contains(f.Nombre, "Sala Norte") {
					t.Errorf("el lugar no llega a la fila: %+v", f)
				}
				if f.insignia == nil || f.insignia.Texto != l.T.T("today.now", nil) {
					t.Errorf("el evento en curso no lleva «Ahora»")
				}
			}
		case *FilaCorreo:
			pies = append(pies, f)
		}
	}
	quiero := []api.EstadoDeEvento{
		api.EventoSinEstado, api.EventoAhora, api.EventoSiguiente,
		api.EventoSinEstado, api.EventoSinEstado, api.EventoSinEstado,
	}
	if len(marcas) != len(quiero) {
		t.Fatalf("salen %d filas de evento", len(marcas))
	}
	for i := range quiero {
		if marcas[i] != quiero[i] {
			t.Errorf("fila %d: marca %d, quiero %d", i, marcas[i], quiero[i])
		}
	}
	if len(pies) != 2 || pies[0].Producto != iconos.Calendar ||
		pies[0].Texto != l.T.T("today.more", i18n.Vars{"count": 3}) || pies[1].Producto != iconos.Mail {
		t.Fatalf("el pie no dice «+3» encima del correo: %+v", pies)
	}
	var producto string
	l.AlAbrir = func(p Producto, _ string) { producto = p.Marca.Clave() }
	test.Tap(pies[0])
	if producto != "calendar" {
		t.Errorf("«+3» abre %q", producto)
	}
	if r.avisoHoy == nil || r.avisoHoy.Text == "" {
		t.Fatal("un calendario caído no pinta el aviso")
	}
	// El aviso y el pie de «+N» cuentan en el alto de la tarjeta. El pie lleva su
	// `border-t` dentro de MedidaCorreo: no suma además el hueco del `divide-y`.
	conTodo := r.altoCuerpoHoy()
	l.Agenda.Parcial, l.Agenda.Total = false, 6
	_, r = pinta(t, l, ventana)
	if r.avisoHoy != nil {
		t.Error("sin calendarios caídos sigue el aviso")
	}
	if dif := conTodo - r.altoCuerpoHoy(); !cerca(dif, MedidaAvisoHoy()+MedidaCorreo()) {
		t.Errorf("el aviso y el «+N» suman %v al alto", dif)
	}
}
