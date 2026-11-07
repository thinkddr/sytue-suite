// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestInterrogacionAbreLaHojaDeAtajos es `lib/atajos.test.ts`: «?» abre la hoja con
// los tres grupos de la web, y la hoja no promete una tecla que Atajo no atienda.
func TestInterrogacionAbreLaHojaDeAtajos(t *testing.T) {
	a, w := appConVentana(t, &buscador{})
	if !a.Atajo('?') {
		t.Fatal("«?» no es un atajo")
	}
	capa := w.Canvas().Overlays().Top()
	if capa == nil {
		t.Fatal("«?» no abrió la hoja de atajos")
	}
	for _, texto := range []string{"Ir a hoy", "Ver esta lista", "Periodo siguiente"} {
		if buscarObjeto(capa, func(o fyne.CanvasObject) bool {
			x, ok := o.(*canvas.Text)
			return ok && x.Text == texto
		}) == nil {
			t.Errorf("la hoja no dice %q", texto)
		}
	}
	// n y j son la misma fila: dos teclas, una descripción.
	if buscarObjeto(capa, func(o fyne.CanvasObject) bool { x, ok := o.(*ui.Tecla); return ok && x.Texto == "j" }) == nil {
		t.Error("la hoja no enseña «j» junto a «n»")
	}
	hoja := a.P.hojaDeAtajos()
	if len(hoja) != 3 || hoja[0].Titulo != "Moverse" || hoja[1].Titulo != "Vistas" || hoja[2].Titulo != "Acciones" {
		t.Errorf("los grupos de la hoja son %+v", hoja)
	}
	for _, g := range hoja {
		for _, f := range g.Atajos {
			for _, k := range f.Teclas {
				if r, _ := utf8.DecodeRuneInString(k); accionDeTecla(r) == "" {
					t.Errorf("la hoja enseña %q y Atajo no la atiende", k)
				}
			}
		}
	}
	if accionDeTecla('M') != atajoMes || accionDeTecla('x') != "" {
		t.Error("las teclas de siempre ya no hacen lo suyo")
	}
}
