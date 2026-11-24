// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"net/url"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// TestUnaFilaDelWidgetLlevaASuDia: cada evento del widget de Calendar lleva a
// `sytue-calendar:/?date=AAAA-MM-DD`, con `&all_day=1` en los de día completo: la ruta
// de su fila (formato 3), que escribe la Fuente con fuentes.RutaDia y el lector nativo
// abre con el esquema de la app. Solo la fecha: ni el título ni el id del evento salen en
// la URL. La app la lee con FechaDelEnlace y abre la vista de día en esa fecha, sin pedir
// nada al servicio.
func TestUnaFilaDelWidgetLlevaASuDia(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata")
	}
	test.NewTempApp(t)
	for enlace, dia := range map[string]int{
		"sytue-calendar:" + fuentes.RutaDia(time.Date(2026, 11, 30, 18, 30, 0, 0, madrid), false): 30,
		"sytue-calendar:" + fuentes.RutaDia(time.Date(2026, 12, 1, 0, 0, 0, 0, madrid), true):     1,
	} {
		p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 21, 12, 0, 0, 0, madrid), Zona: madrid, Vista: VistaMes}
		a := NuevaApp(p, api.NewCalendars("http://calendar.invalid", nil, nil))
		a.lanzar = func(f func(context.Context)) bool { f(a.vida); return true }
		a.enUI = func(f func()) { f() }
		u, err := url.Parse(enlace)
		if err != nil {
			t.Fatal(err)
		}
		a.AtenderEnlace(u)
		if p.Vista != VistaDia || p.Cursor.Day() != dia || p.Cursor.Location() != madrid {
			t.Errorf("%s abre la vista %v en %v; quiero el día %d", enlace, p.Vista, p.Cursor, dia)
		}
	}
}
