// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLosPasosHechosSeVuelvenYLosDemasNo: solo un paso ya hecho es un botón —se pulsa y
// se enfoca—; el actual y los pendientes son texto y se saltan al tabular. Y los tramos
// llenan la barra: el último a su medida y los otros a partes iguales, con `gap-2`.
func TestLosPasosHechosSeVuelvenYLosDemasNo(t *testing.T) {
	p := NuevosPasos("Uno", "Dos", "Tres")
	p.Actual = 1
	elegido := -1
	p.AlElegir = func(i int) { elegido = i }
	w := test.NewWindow(p)
	defer w.Close()
	w.Resize(fyne.NewSize(600, 40))

	if p.marcas[0].Disabled() || !p.marcas[1].Disabled() || !p.marcas[2].Disabled() {
		t.Error("solo el paso hecho se puede pulsar")
	}
	p.marcas[0].Tapped(nil)
	p.marcas[2].Tapped(nil)
	if elegido != 0 {
		t.Errorf("pulsar devuelve al paso %d y solo vale volver al primero", elegido)
	}
	p.AlElegir = nil
	if !p.marcas[0].Disabled() {
		t.Error("sin AlElegir el progreso no es interactivo y el paso hecho sigue pulsable")
	}

	xs, ws := p.Tramos(600)
	if fin := xs[2] + ws[2]; fin-600 > 0.01 || 600-fin > 0.01 || ws[0] != ws[1] || ws[2] != p.AnchoMarca(2) {
		t.Errorf("los tramos son %v × %v y tienen que llenar 600", xs, ws)
	}
}

// TestElRotuloQueNoCabeSeRecorta: con seis pasos en 630 —el alta guiada— «Tu
// organización» no cabe en su tramo con el filete; en la web la marca cede (`min-w-0`),
// el filete se queda en su mínimo (`min-w-4`) y el rótulo sale con elipsis
// (`truncate`): «Tu orga…». Sin esto el rótulo se montaba encima del paso siguiente.
//
// Visto en ROJO a mano (en el frontal): sin el `min` de la marca en `Layout`, el rótulo
// sale entero y la marca invade el tramo siguiente.
func TestElRotuloQueNoCabeSeRecorta(t *testing.T) {
	p := NuevosPasos("Empezar", "Tu organización", "Tu equipo", "Facturación", "Permisos", "Listo")
	w := test.NewWindow(p)
	defer w.Close()
	w.Resize(fyne.NewSize(630, 40))
	p.Resize(fyne.NewSize(630, AltoPasos))
	xs, ws := p.Tramos(630)
	for i, m := range p.marcas[:len(p.marcas)-1] {
		if fin := m.Position().X + m.Size().Width; fin > xs[i]+ws[i]-HuecoPasos-MinFilete+0.01 {
			t.Errorf("la marca %d acaba en %v y su tramo deja sitio hasta %v", i+1, fin, xs[i]+ws[i]-HuecoPasos-MinFilete)
		}
	}
	r := test.WidgetRenderer(p.marcas[1]).(*marcaRenderer)
	if r.rotulo.Text == "Tu organización" || !strings.HasSuffix(r.rotulo.Text, "…") {
		t.Errorf("el rótulo del paso 2 dice %q; tenía que recortarse con elipsis", r.rotulo.Text)
	}
}
