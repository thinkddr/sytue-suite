// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La tesela de la portada comparte con el MISMO panel que la cabecera del editor, y
// descarga en la carpeta de la app. Sin las direcciones de la web no hay qué copiar, y
// la tesela no lleva «Compartir».
func TestLaPortadaCompartirYDescargaComoElEditor(t *testing.T) {
	a := NuevaApp(nil, "es", nil)
	dir := t.TempDir()
	a.carpeta = func() (string, error) { return dir, nil }
	w := test.NewTempWindow(t, a.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	if a.Portada().Compartir != nil {
		t.Fatal("sin EnlaceDoc, la tesela no puede compartir")
	}
	if got, err := a.Portada().Carpeta(); err != nil || got != dir {
		t.Errorf("la portada descarga en %q (%v) y la app en %q", got, err, dir)
	}
	a.EnlaceDoc = func(id string) string { return "https://docs.test/edit?id=" + id }
	a.Arrancar()
	if a.Portada().Compartir == nil {
		t.Fatal("con EnlaceDoc, la tesela comparte")
	}
	boton := ui.NuevoBoton("x", nil)
	w.SetContent(boton)
	a.Portada().Compartir(api.Document{ID: "d1", Title: "Plan"}, boton)
	if len(w.Canvas().Overlays().List()) == 0 {
		t.Error("«Compartir» de la tesela no abre el panel")
	}
}
