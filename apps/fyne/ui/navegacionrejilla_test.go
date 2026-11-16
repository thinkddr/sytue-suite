// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// Los casos son los de `@sytue/ui` `lib/navegacionRejilla.test.ts`: si cambia uno, cambia
// el otro.

// rejillaRegular son `n` cajas de 100 × 80 con 10 de hueco en filas de `cols`.
func rejillaRegular(n, cols int) []CajaRejilla {
	cajas := make([]CajaRejilla, n)
	for i := range cajas {
		cajas[i] = CajaRejilla{X: float32(i%cols) * 110, Y: float32(i/cols) * 90, Ancho: 100, Alto: 80}
	}
	return cajas
}

// justificadas son dos filas como las de Photos (tres y cuatro fotos), la cabecera de un
// mes y una fila de dos.
var justificadas = []CajaRejilla{
	{X: 0, Y: 0, Ancho: 200, Alto: 100},
	{X: 204, Y: 0, Ancho: 60, Alto: 100},
	{X: 268, Y: 0, Ancho: 232, Alto: 100},
	{X: 0, Y: 104, Ancho: 120, Alto: 120},
	{X: 124, Y: 104, Ancho: 120, Alto: 120},
	{X: 248, Y: 104, Ancho: 120, Alto: 120},
	{X: 372, Y: 104, Ancho: 128, Alto: 120},
	{X: 0, Y: 264, Ancho: 300, Alto: 150},
	{X: 304, Y: 264, Ancho: 196, Alto: 150},
}

func TestVecinoEnLista(t *testing.T) {
	casos := []struct {
		actual int
		tecla  fyne.KeyName
		quiero int
	}{
		{3, fyne.KeyDown, 4},
		{3, fyne.KeyUp, 2},
		{3, fyne.KeyHome, 0},
		{3, fyne.KeyEnd, 9},
		{9, fyne.KeyDown, -1},
		{0, fyne.KeyUp, -1},
		{0, fyne.KeyHome, -1},
		{3, fyne.KeyRight, -1},
	}
	for _, c := range casos {
		if got := VecinoEnLista(c.actual, 10, c.tecla); got != c.quiero {
			t.Errorf("%s desde %d en una lista de 10 lleva a %d; quería %d", c.tecla, c.actual, got, c.quiero)
		}
	}
}

func TestVecinoEnRejilla(t *testing.T) {
	cuatro := rejillaRegular(10, 4) // filas de 4, 4 y 2
	casos := []struct {
		nombre string
		cajas  []CajaRejilla
		actual int
		tecla  fyne.KeyName
		quiero int
	}{
		{"→ pasa de fila", cuatro, 3, fyne.KeyRight, 4},
		{"← vuelve de fila", cuatro, 4, fyne.KeyLeft, 3},
		{"→ en la última", cuatro, 9, fyne.KeyRight, -1},
		{"← en la primera", cuatro, 0, fyne.KeyLeft, -1},
		{"↓ misma columna", cuatro, 1, fyne.KeyDown, 5},
		{"↑ misma columna", cuatro, 5, fyne.KeyUp, 1},
		{"↑ en la primera fila", cuatro, 2, fyne.KeyUp, -1},
		{"↓ a la columna más cercana de la última fila", cuatro, 7, fyne.KeyDown, 9},
		{"↓ en la última fila", cuatro, 9, fyne.KeyDown, -1},
		{"↓ con dos columnas suma dos", rejillaRegular(6, 2), 1, fyne.KeyDown, 3},
		{"↓ desde la estrecha", justificadas, 1, fyne.KeyDown, 4},
		{"↓ desde la ancha, a la más cercana", justificadas, 2, fyne.KeyDown, 6},
		{"↑ a la estrecha", justificadas, 4, fyne.KeyUp, 1},
		{"↓ cruza el mes", justificadas, 6, fyne.KeyDown, 8},
		{"↑ cruza el mes", justificadas, 7, fyne.KeyUp, 4},
		{"Inicio", cuatro, 5, fyne.KeyHome, 0},
		{"Fin", cuatro, 5, fyne.KeyEnd, 9},
		{"Intro no es de moverse", cuatro, 5, fyne.KeyReturn, -1},
		{"sin cajas", nil, 0, fyne.KeyDown, -1},
	}
	for _, c := range casos {
		if got := VecinoEnRejilla(c.cajas, c.actual, c.tecla); got != c.quiero {
			t.Errorf("%s: %s desde %d lleva a %d; quería %d", c.nombre, c.tecla, c.actual, got, c.quiero)
		}
	}
}

// TestMoverFocoEnRejillaEnfocaYTraeALaVista: en una rejilla de 2 columnas dentro de un
// desplazable bajo, ↓ desde la última fila visible enfoca la de debajo y desplaza hasta
// ella.
func TestMoverFocoEnRejillaEnfocaYTraeALaVista(t *testing.T) {
	objs := make([]fyne.CanvasObject, 8)
	for i := range objs {
		objs[i] = widget.NewButton("", nil)
	}
	rej := container.NewGridWrap(fyne.NewSize(100, 80), objs...)
	s := container.NewVScroll(rej)
	w := test.NewWindow(s)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 180))
	if c := rej.Size(); c.Width < 200 {
		t.Fatalf("la rejilla mide %v: no caben dos columnas", c)
	}
	w.Canvas().Focus(objs[3].(fyne.Focusable))
	if !MoverFocoEnRejilla(objs, 3, fyne.KeyDown, s) {
		t.Fatal("↓ desde la segunda fila no movió el foco")
	}
	if w.Canvas().Focused() != objs[5].(fyne.Focusable) {
		t.Fatalf("el foco está en %v; tenía que pasar a la tesela de debajo", w.Canvas().Focused())
	}
	if s.Offset.Y <= 0 {
		t.Error("la tesela enfocada está fuera de la vista y el desplazable no se movió")
	}
	if MoverFocoEnRejilla(objs, 0, fyne.KeyUp, s) {
		t.Error("↑ desde la primera fila no tiene a dónde ir")
	}
}
