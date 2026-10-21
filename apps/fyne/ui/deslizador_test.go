// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestDeslizadorBipolar: a 0 el recorrido lleno mide 0 px; con valores positivos va del
// centro a la derecha; con valores negativos va de la izquierda al centro.
func TestDeslizadorBipolar(t *testing.T) {
	d := NuevoDeslizador(-1, 1, 0, nil)
	d.Bipolar = true
	w := test.NewTempWindow(t, d)
	w.Resize(fyne.NewSize(200, AltoDeslizador()))
	d.Resize(fyne.NewSize(200, AltoDeslizador()))

	r := test.WidgetRenderer(d).(*deslizadorRenderer)
	r.Layout(d.Size())

	// En 0: el ancho del recorrido debe ser 0.
	if r.recorrido.dentro.Size().Width != 0 {
		t.Errorf("a 0 el recorrido mide %v, se esperaba 0", r.recorrido.dentro.Size().Width)
	}

	// En +0.5: el recorrido va del centro (100) hacia la derecha.
	d.Poner(0.5)
	r.Layout(d.Size())
	anchoPositivo := r.recorrido.dentro.Size().Width
	posPositiva := r.recorrido.dentro.Position().X
	if anchoPositivo <= 0 {
		t.Errorf("en +0.5 el recorrido no creció: %v", anchoPositivo)
	}
	if posPositiva != 100 {
		t.Errorf("en +0.5 el recorrido empieza en %v, se esperaba en 100", posPositiva)
	}

	// En -0.5: el recorrido termina en el centro (100) y empieza antes.
	d.Poner(-0.5)
	r.Layout(d.Size())
	anchoNegativo := r.recorrido.dentro.Size().Width
	posNegativa := r.recorrido.dentro.Position().X
	if anchoNegativo <= 0 {
		t.Errorf("en -0.5 el recorrido no creció: %v", anchoNegativo)
	}
	if posNegativa+anchoNegativo != 100 {
		t.Errorf("en -0.5 el recorrido termina en %v, se esperaba en 100", posNegativa+anchoNegativo)
	}
}
