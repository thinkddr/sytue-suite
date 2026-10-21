// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElBordeDiscontinuoEsElDeChromium: el patrón medido en Chromium, desde donde acaba la
// esquina. Con 2 px (la zona de abrir de CertiPDF, radio 12), trazos de 6 y huecos de 4;
// con 1 px (la zona de soltar de Photos, radio 6), trazos de 3 y huecos de 2. Las
// esquinas, enteras, y el centro, vacío.
func TestElBordeDiscontinuoEsElDeChromium(t *testing.T) {
	for _, c := range []struct {
		grosor, radio, x, y float64
		si                  bool
	}{
		{2, 12, 12 + 3, 1, true},
		{2, 12, 12 + 7, 1, false},
		{2, 12, 12 + 13, 1, true},
		{2, 12, 2, 6, true},
		{2, 12, 400, 100, false},
		{2, 12, 1, 12 + 3, true},
		{1, 6, 6 + 1, 0.5, true},
		{1, 6, 6 + 3.5, 0.5, false},
		{1, 6, 6 + 5.5, 0.5, true},
		{1, 6, 6 + 1, 1.5, false},
		{1, 6, 1, 3, true},
	} {
		if got := EnBordeDiscontinuo(c.x, c.y, 800, 200, c.grosor, c.radio); got != c.si {
			t.Errorf("grosor %g: (%g,%g) en el borde = %v, quiero %v", c.grosor, c.x, c.y, got, c.si)
		}
	}
}

// TestElBordeDiscontinuoSePintaConSuToken: a ×2, en el lado de arriba de un borde de 1 px,
// el píxel de un trazo sale del color del token y el de un hueco, del fondo.
func TestElBordeDiscontinuoSePintaConSuToken(t *testing.T) {
	b := NuevoBordeDiscontinuo(1, tema.RadioMD, "primary")
	b.Resize(fyne.NewSize(200, 32))
	img := pinta(t, 200, 32, b)
	primario := tema.Color("primary", tema.Claro)
	if got := px(img, int((tema.RadioMD+1)*escala), 0); !cerca(got, primario) {
		t.Errorf("en un trazo hay %v y el token da %v", got, primario)
	}
	if got := px(img, int((tema.RadioMD+3.5)*escala), 0); cerca(got, primario) {
		t.Errorf("en un hueco hay %v: el borde no es discontinuo", got)
	}
}
