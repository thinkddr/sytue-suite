// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestEnElTelefonoLaCabeceraVaEnLaBarraDeLaSuite: montado como en cmd/docus, a 390 la
// cabecera del editor no se pinta y su salida, el título y «Compartir» van en la barra de
// la app, que ES la de la suite; la cinta empieza justo debajo. Eran dos barras y la cinta
// (~200 px de cromo). «Renombrar» abre el diálogo, porque el campo no se ve; en ancho
// todo vuelve a la cabecera.
func TestEnElTelefonoLaCabeceraVaEnLaBarraDeLaSuite(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	volver, compartir := 0, 0
	ctx := &Contexto{
		Tipo: Documento, Zoom: 100, Estado: Listo, Guardado: Limpio, Titulo: "Acta",
		AlRenombrar: func(string) {},
		Compartir:   func(fyne.CanvasObject) { compartir++ },
	}
	m := NuevoMarco(ctx, Genericas(), func() { volver++ })
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	barra, restaurar := ui.BarraSuiteDePrueba(tr, iconos.Docs)
	defer restaurar()
	w := test.NewTempWindow(t, barra.Montar(m.Montado()))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})

	donde := fyne.CurrentApp().Driver().AbsolutePositionForObject
	b := m.movil
	if y, alto := donde(b).Y, b.Size().Height; y != 0 || alto != ui.AltoBarraSuite() {
		t.Errorf("la barra de la app está en y=%v con %v de alto; quiero la de la suite", y, alto)
	}
	if m.cabecera.Visible() || !m.enBarra {
		t.Fatal("a 390 la cabecera del editor sigue a la vista")
	}
	if y := donde(m.cinta).Y; y != ui.AltoBarraSuite() {
		t.Errorf("la cinta empieza en y=%v; quiero justo bajo la barra única (%v)", y, ui.AltoBarraSuite())
	}
	if b.Titulo != "Acta" || !b.EnDetalle || len(b.Acciones) != 1 {
		t.Fatalf("la barra dice %q (volver %v) con %d acciones", b.Titulo, b.EnDetalle, len(b.Acciones))
	}
	b.AlVolver()
	test.Tap(b.Acciones[0].(*ui.BotonIcono))
	if volver != 1 || compartir != 1 {
		t.Errorf("la barra: %d vueltas y %d «Compartir»", volver, compartir)
	}
	ctx.EnfocarTitulo()
	if m.renombrar == nil {
		t.Error("«Renombrar» sin cabecera tiene que abrir su diálogo")
	} else {
		m.renombrar.Cerrar()
	}
	// Si falla el guardado, la barra lo dice y reintenta.
	ctx.Guardado = FalloAlGuardar
	ctx.Refrescar()
	if len(b.Acciones) != 2 {
		t.Errorf("con el guardado fallido la barra lleva %d acciones; quiero el aviso y «Compartir»", len(b.Acciones))
	}

	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() {})
	if !m.cabecera.Visible() || m.enBarra {
		t.Error("en ancho la cabecera vuelve")
	}
}
