// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElMenuDeLaFilaSeAbreEnSuBotonYNoAbreLaFila es el «⋯» de `FileRow`: un `h-8 w-8`
// en la columna `w-8`, pegado al `px-3` de la derecha, que abre el menú SIN abrir la
// fila (`stopPropagation`) y que solo se ve al pasar por encima.
//
// VISTO EN ROJO (2026-09-22, en el frontal): quitando la comprobación de `enMas` en
// `Tapped`, pulsar el «⋯» abre la fila.
func TestElMenuDeLaFilaSeAbreEnSuBotonYNoAbreLaFila(t *testing.T) {
	const ancho = 999
	f := NuevaFilaDrive("Contrato.pdf", "pdf")
	f.GlifoMas = iconos.TrazoDeCuerpo(`<circle cx="12" cy="5" r="1" />`, iconos.TallaSM, tema.Color("text-muted", tema.Claro))
	var abierta, menus int
	var colgado fyne.CanvasObject
	f.AlAbrir = func() { abierta++ }
	f.AlMenu = func(d fyne.CanvasObject) { menus++; colgado = d }
	colocar(t, f, ancho)

	if f.GlifoMas.Objeto().Visible() {
		t.Error("el «⋯» se ve sin pasar por encima: es `opacity-0 group-hover:opacity-100`")
	}
	f.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 20)}})
	if !f.GlifoMas.Objeto().Visible() {
		t.Error("al pasar por encima de la fila el «⋯» no aparece")
	}

	o, m := cajaMas(f.Size())
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(o.X+m.Width/2, o.Y+m.Height/2)})
	if menus != 1 || abierta != 0 {
		t.Fatalf("pulsar el «⋯» abre %d menús y %d filas; quiero 1 y 0", menus, abierta)
	}
	if colgado == nil || colgado.Size() != fyne.NewSize(32, 32) {
		t.Errorf("el menú cuelga de una caja de %v; el botón es `h-8 w-8` (28)", colgado)
	} else if x := colgado.Position().X; x != ancho-MargenFila-32 {
		t.Errorf("el «⋯» empieza en x=%v y va pegado al `px-3`: %v", x, ancho-MargenFila-32)
	}

	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(100, 20)})
	if abierta != 1 || menus != 1 {
		t.Errorf("pulsar fuera del «⋯» tiene que abrir la fila: %d filas y %d menús", abierta, menus)
	}
}

// TestLaCasillaDeLaFilaSoloSeVeAlPasarOConSeleccion: `opacity-0 group-hover:opacity-100`
// mientras no haya nada marcado; con algo marcado en la lista, se ven TODAS; y la de una
// fila marcada, siempre. Ocupa su sitio igual, así que el nombre no se mueve.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `Hidden` de la casilla en `Refresh`,
// se ve sin pasar por encima.
func TestLaCasillaDeLaFilaSoloSeVeAlPasarOConSeleccion(t *testing.T) {
	f := NuevaFilaDrive("Contrato.pdf", "pdf")
	f.ConCasilla = true
	colocar(t, f, 999)
	r := test.WidgetRenderer(f).(*filaDriveRenderer)
	if r.casilla.Visible() {
		t.Error("la casilla se ve sin pasar por encima y sin nada marcado")
	}
	f.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 20)}})
	if !r.casilla.Visible() {
		t.Error("al pasar por encima la casilla no aparece")
	}
	f.MouseOut()
	f.SeleccionActiva = true
	f.Refresh()
	if !r.casilla.Visible() {
		t.Error("con algo marcado en la lista, las casillas tienen que verse todas")
	}
}

// TestLaCabeceraDeDriveEscondeLasColumnasQueNoCaben: a 360 no caben «Tamaño» ni
// «Modificado» (`hidden sm:block`, `hidden md:block`). Sin colocar, sus letras se
// quedaban en (0, 0) y se pintaban amontonadas encima de la casilla (visto en una
// ventana real el 2026-09-27). VISTO EN ROJO sin los `Ver` del Layout.
func TestLaCabeceraDeDriveEscondeLasColumnasQueNoCaben(t *testing.T) {
	for _, c := range []struct {
		ventana       float32
		tamano, fecha bool
	}{{360, false, false}, {700, true, false}, {1280, true, true}} {
		cab := NuevaCabeceraDrive("Nombre", "Tamaño", "Modificado")
		cab.ConCasilla, cab.Ventana = true, c.ventana
		r := test.WidgetRenderer(cab).(*cabeceraDriveRenderer)
		cab.Resize(fyne.NewSize(c.ventana, AltoCabeceraDrive))
		r.Layout(cab.Size())
		for _, v := range []struct {
			que    string
			versal *Versal
			quiero bool
		}{{"el tamaño", r.tamano, c.tamano}, {"la fecha", r.fecha, c.fecha}} {
			for _, l := range v.versal.letras {
				if l.Visible() != v.quiero {
					t.Errorf("a %g, %s se ve = %v", c.ventana, v.que, l.Visible())
					break
				}
			}
		}
	}
}
