// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// botonFlotanteDe busca el «Subir» flotante en lo que pinta la ventana.
func botonFlotanteDe(o fyne.CanvasObject) *ui.BotonFlotante {
	var hallado *ui.BotonFlotante
	recorrer(o, func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.BotonFlotante); ok {
			hallado = b
		}
	})
	return hallado
}

// TestEnElTelefonoSubirFlota: a 390, en «Mi unidad» y con red, «Subir» flota abajo a la
// derecha (`MobileFab` de FileBrowser.tsx) y abre el selector del sistema; sin red, en
// otra sección o a 1280 no se ve.
//
// VISTO EN ROJO (2026-11-05): sin la capa en `Objeto` el test no encuentra el botón, y
// sin el `Hidden` de `Repintar` sigue visible al quedarse sin red.
func TestEnElTelefonoSubirFlota(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), nil, nil, w)
	p.explor.Fase = Exito
	elegido := 0
	p.ElegirFicheros = func(func([]Entrada)) { elegido++ }
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(390, 760))
	_ = w.Canvas().Capture()

	fab := botonFlotanteDe(w.Content())
	if fab == nil {
		t.Fatal("a 390 en «Mi unidad» no hay «Subir» flotante")
	}
	visible := func() bool { return fab.Visible() && p.capaSubir.Visible() }
	if !visible() || fab.Nombre != p.tr.T("files.upload", nil) {
		t.Fatalf("el botón flotante no se ve o no se llama «Subir» (%q)", fab.Nombre)
	}
	pos, lado, raiz := fab.Position(), fab.Size(), p.raiz.Size()
	if pos.X+lado.Width != raiz.Width-ui.MargenBotonFlotante || pos.Y+lado.Height != raiz.Height-ui.MargenBotonFlotante {
		t.Errorf("el botón no está en la esquina de abajo a la derecha: %v %v en %v", pos, lado, raiz)
	}
	test.Tap(fab)
	if elegido != 1 {
		t.Errorf("pulsar «Subir» abrió el selector %d veces", elegido)
	}

	p.conexion(false)
	if visible() {
		t.Error("sin red el «Subir» flotante sigue a la vista")
	}
	p.conexion(true)
	p.irA(Papelera)
	if visible() {
		t.Error("en la papelera el «Subir» flotante sigue a la vista")
	}
	p.irA(MiUnidad)
	w.Resize(fyne.NewSize(1280, 800))
	_ = w.Canvas().Capture()
	if visible() {
		t.Error("a 1280 el «Subir» flotante sigue a la vista")
	}
}
