// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS la presentación de muestra en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`), montada como en
// la app: el marco en su armazón (`editor.Marco.Montado`), con «Volver».
func TestCapturasEsteticas(t *testing.T) {
	ctx := &editor.Contexto{Tipo: editor.Presentacion, Estado: editor.Listo, Zoom: 100, T: traductor(t), Titulo: "Resultados del trimestre"}
	e := NuevoEditor(ctx)
	e.Programar = func(time.Duration, func()) {}
	ctx.Lienzo = e
	ctx.SoloLectura = !e.Cargar(string(muestra(t)))
	marco := editor.NuevoMarco(ctx, Registro(e), func() {})
	cromo := ctx.Refrescar
	ctx.Refrescar = func() { cromo(); e.Refrescar() }
	ctx.Refrescar()
	w := test.NewTempWindow(t, container.NewStack(marco.Montado(), e.Avisos()))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	montaje.Capturar(t, w, "diapos-presentacion", iconos.Slides)
}
