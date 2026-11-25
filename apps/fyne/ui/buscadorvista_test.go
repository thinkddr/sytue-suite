// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestElBuscadorDeVistaEsUnoSolo: el preajuste común de búsqueda —el `<Input>` de
// Form.tsx con la lupa delante y `type="search"`— en la talla `md` (36, y 44 con el
// dedo), y a todo el ancho en el teléfono. Mail lo tenía a media fila, el ERP sin lupa,
// Cast de 24 con letra de 12 (auditoría del 2026-11-24).
func TestElBuscadorDeVistaEsUnoSolo(t *testing.T) {
	e := NuevoBuscador("Buscar en el correo")
	if e.Icono != iconos.Buscar || !e.Busqueda || e.Talla != CampoMD {
		t.Fatalf("el buscador es talla %v, lupa %v, búsqueda %v", e.Talla, e.Icono, e.Busqueda)
	}
	if got := e.MinSize().Height; got != 36 {
		t.Errorf("con puntero mide %v de alto y el `h-9` son 36", got)
	}
	restaurar := TactilDePrueba(true)
	if got := e.MinSize().Height; got != SueloTactilBarra {
		t.Errorf("con el dedo mide %v de alto y el suelo son 44", got)
	}
	restaurar()

	w := test.NewTempWindow(t, e)
	w.Resize(fyne.NewSize(390, 200))
	if got := AnchoBuscador(e, 184, 300); got != 300 {
		t.Errorf("en el teléfono el buscador mide %v y le caben 300: se queda a media fila", got)
	}
	w.Resize(fyne.NewSize(1280, 800))
	if got := AnchoBuscador(e, 184, 300); got != 184 {
		t.Errorf("en escritorio el buscador mide %v y el suyo son 184", got)
	}
	if got := AnchoBuscador(e, 184, 120); got != 120 {
		t.Errorf("sin sitio el buscador mide %v y le caben 120", got)
	}
}
