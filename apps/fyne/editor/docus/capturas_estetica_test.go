// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS un documento abierto en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`), con los
// rótulos de verdad.
func TestCapturasEsteticas(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<h1>Propuesta de reforma de la oficina</h1><p>Esta propuesta resume el alcance, " +
		"el calendario y el presupuesto de la reforma de la <strong>segunda planta</strong>.</p>" +
		"<h2>Alcance</h2><ul><li>Sala de reuniones con videollamada</li><li>Doce puestos nuevos</li></ul>"}
	previa := appDePrueba(t, s)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	a := NuevaApp(tr.T, "es", previa.Svc)
	a.lanzar, a.programar, a.EnVivo = previa.lanzar, previa.programar, previa.EnVivo
	a.portada.Lanzar = a.lanzar
	w := test.NewTempWindow(t, a.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	a.AbrirID("d1", "")
	montaje.Capturar(t, w, "docus-documento", iconos.Docs)
}
