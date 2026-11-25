// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestEnElTelefonoDePieElFolioCabeALoAncho: con el teléfono de pie el documento abre con
// el papel entero a lo ancho, como la web. Abría al 75 % y el papel no cabía (auditoría
// del 2026-11-24). Tumbado sigue sin bajar del 75 % (ver la rotación con poca altura).
func TestEnElTelefonoDePieElFolioCabeALoAncho(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	f := folioCon(t, "<h1>Propuesta</h1><p>Un párrafo.</p>")
	f.ctx.Estado = editor.Listo
	m := editor.NuevoMarco(f.ctx, registroDocus("es"), nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})
	if f.ctx.Zoom >= 75 {
		t.Fatalf("de pie el documento abre al %d %%", f.ctx.Zoom)
	}
	pag, _ := f.pagina.medidas()
	if ancho := pag * float32(f.ctx.Zoom) / 100; ancho > f.scroll.Size().Width {
		t.Errorf("al %d %% el papel mide %v y el lienzo %v: no cabe a lo ancho", f.ctx.Zoom, ancho, f.scroll.Size().Width)
	}
}
