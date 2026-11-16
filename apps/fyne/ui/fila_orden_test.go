// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// cabeceraOrdenable monta la cabecera de Drive a 1280 con los rótulos que ordenan.
func cabeceraOrdenable(campo string, desc bool, pulsado *string) (*CabeceraDrive, *cabeceraDriveRenderer) {
	cab := NuevaCabeceraDrive("Nombre", "Tamaño", "Modificado")
	cab.ConCasilla, cab.Ventana = true, 1280
	cab.OrdenCampo, cab.OrdenDesc = campo, desc
	cab.AlOrdenar = func(c string) { *pulsado = c }
	r := test.WidgetRenderer(cab).(*cabeceraDriveRenderer)
	cab.Resize(fyne.NewSize(1280, AltoCabeceraDrive))
	r.Layout(cab.Size())
	r.Refresh()
	return cab, r
}

// centroDe es el punto medio de un rótulo, donde se pulsaría.
func centroDe(e *Versal) *fyne.PointEvent {
	x := e.letras[0].Position().X + e.Ancho()/2
	return &fyne.PointEvent{Position: fyne.NewPos(x, AltoCabeceraDrive/2)}
}

// TestLaCabeceraDeDriveOrdenaAlPulsarUnRotulo: los rótulos son los botones de la web
// (`files.sort.by`): cada uno ordena por su columna, y fuera de ellos no pasa nada.
// Sin AlOrdenar (la búsqueda, donde manda la relevancia) son decorativos.
//
// VISTO EN ROJO: sin `Tapped`, la cabecera no era pulsable.
func TestLaCabeceraDeDriveOrdenaAlPulsarUnRotulo(t *testing.T) {
	var pulsado string
	cab, r := cabeceraOrdenable(CampoNombre, false, &pulsado)
	for campo, e := range map[string]*Versal{CampoNombre: r.nombre, CampoTamano: r.tamano, CampoModificado: r.fecha} {
		pulsado = ""
		test.Tap(cab) // en (0, 0): el margen, fuera de todo rótulo
		if pulsado != "" {
			t.Fatalf("pulsar el margen ordena por %q", pulsado)
		}
		cab.Tapped(centroDe(e))
		if pulsado != campo {
			t.Errorf("pulsar «%s» ordena por %q, quiero %q", campo, pulsado, campo)
		}
	}
	pulsado = ""
	cab.AlOrdenar = nil
	cab.Tapped(centroDe(r.nombre))
	if pulsado != "" {
		t.Error("sin AlOrdenar la cabecera no ordena")
	}
}

// TestLaCabeceraDeDriveSeñalaLaColumnaOrdenada: la activa va en `text-text` con su
// flecha detrás (`gap-1`, 12 px; `chevronDown` solo en descendente), las otras en
// `text-text-subtle`; y en «Tamaño», pegado a la derecha, es el botón ENTERO —flecha
// incluida— lo que se pega: el rótulo se corre a la izquierda. El alto no cambia.
func TestLaCabeceraDeDriveSeñalaLaColumnaOrdenada(t *testing.T) {
	var pulsado string
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, sinOrden := cabeceraOrdenable(CampoNombre, false, &pulsado)
	xTamano := sinOrden.tamano.letras[0].Position().X

	cab, r := cabeceraOrdenable(CampoTamano, true, &pulsado)
	if cab.MinSize().Height != AltoCabeceraDrive {
		t.Errorf("la cabecera ordenable mide %v de alto, y mide %v", cab.MinSize().Height, AltoCabeceraDrive)
	}
	if !r.flecha.Objeto().Visible() {
		t.Fatal("la columna ordenada no lleva flecha")
	}
	if got := r.tamano.letras[0].Color; got != tema.Color("text", v) {
		t.Errorf("el rótulo de la columna ordenada pinta %v, quiero text", got)
	}
	if got := r.nombre.letras[0].Color; got != tema.Color("text-subtle", v) {
		t.Errorf("un rótulo sin ordenar pinta %v, quiero text-subtle", got)
	}
	corrido := xTamano - r.tamano.letras[0].Position().X
	if d := corrido - (HuecoOrdenDrive + TamFlechaOrdenDrive); d > 0.01 || d < -0.01 {
		t.Errorf("con la flecha «Tamaño» se corre %v a la izquierda, quiero %v", corrido, HuecoOrdenDrive+TamFlechaOrdenDrive)
	}
	fx := r.flecha.Objeto().Position().X
	if want := r.tamano.letras[0].Position().X + r.tamano.Ancho() + HuecoOrdenDrive; fx-want > 0.01 || want-fx > 0.01 {
		t.Errorf("la flecha está en x=%v, quiero %v (detrás del rótulo)", fx, want)
	}
	if r.flecha.Objeto().Size().Width != TamFlechaOrdenDrive {
		t.Errorf("la flecha mide %v, quiero %v", r.flecha.Objeto().Size().Width, TamFlechaOrdenDrive)
	}

	// Sin ordenar (búsqueda) no hay flecha ni tinta activa.
	cab.AlOrdenar = nil
	cab.Refresh()
	if r.flecha.Objeto().Visible() {
		t.Error("sin AlOrdenar no debería verse la flecha")
	}
	if got := r.tamano.letras[0].Color; got != tema.Color("text-subtle", v) {
		t.Errorf("sin ordenar, el rótulo pinta %v, quiero text-subtle", got)
	}
}

// TestLaCabeceraDeDriveIluminaElRotuloBajoElPuntero: `hover:text-text`, y la mano.
func TestLaCabeceraDeDriveIluminaElRotuloBajoElPuntero(t *testing.T) {
	var pulsado string
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cab, r := cabeceraOrdenable(CampoNombre, false, &pulsado)
	p := centroDe(r.fecha).Position
	cab.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}})
	if got := r.fecha.letras[0].Color; got != tema.Color("text", v) {
		t.Errorf("bajo el puntero «Modificado» pinta %v, quiero text", got)
	}
	if cab.Cursor() != desktop.PointerCursor {
		t.Error("sobre un rótulo que ordena, el puntero es la mano")
	}
	cab.MouseOut()
	if got := r.fecha.letras[0].Color; got != tema.Color("text-subtle", v) {
		t.Errorf("al salir «Modificado» pinta %v, quiero text-subtle", got)
	}
}
