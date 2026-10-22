// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// TestLaFilaRecicladaSeVuelveAColocar es el guardián del reciclaje.
//
// Dentro de un `widget.List` una fila NO se construye por elemento: se construye una y
// `UpdateItem` la rellena con otro fichero cada vez que pasa por pantalla. Como el
// tamaño no cambia, Fyne no vuelve a llamar a `Layout`, y todo lo que dependa del
// CONTENIDO se queda con la geometría del contenido anterior.
//
// Lo que se rompía, medido el 2026-09-22 sobre el PNG del explorador de Sytue Drive:
//
//   - la columna de tamaño va pegada a la derecha, así que su x se calcula restando el
//     ancho del texto; con el de la fila recién creada (vacío) salía 10,5 px a la
//     derecha y «4,0 KB» se pintaba ENCIMA de «6 sept 2026»;
//   - la tesela decide en `Colocar` si enseña la carpeta o la extensión, así que una
//     carpeta que llegaba por reciclaje salía como un cuadrado vacío.
//
// La comprobación es contra una fila CONSTRUIDA YA con esos datos, que es la referencia
// honesta: reciclar tiene que dar exactamente lo mismo que construir.
func TestLaFilaRecicladaSeVuelveAColocar(t *testing.T) {
	const ancho = 999

	// La de referencia: nace con sus datos y se coloca una vez.
	nueva := NuevaFilaDrive("Notas de la reunión.md", "md")
	nueva.Tamano, nueva.Fecha, nueva.Ventana = "4,0 KB", "6 sept 2026", 1280
	colocar(t, nueva, ancho)

	// La reciclada: nace vacía —como la que hace `CreateItem`— y se rellena después.
	usada := NuevaFilaDrive("", "")
	colocar(t, usada, ancho)
	usada.Nombre, usada.Extension = "Notas de la reunión.md", "md"
	usada.Tamano, usada.Fecha, usada.Ventana = "4,0 KB", "6 sept 2026", 1280
	usada.Refresh()

	a, b := textosDe(nueva), textosDe(usada)
	if len(a) != len(b) {
		t.Fatalf("la reciclada pinta %d textos y la nueva %d", len(b), len(a))
	}
	for texto, x := range a {
		if y, hay := b[texto]; !hay {
			t.Errorf("la reciclada no pinta %q", texto)
		} else if d := x - y; d > 1.0/16 || d < -1.0/16 {
			t.Errorf("%q está en x=%v al reciclar y en x=%v recién construida", texto, y, x)
		}
	}

	// Y la tesela: una carpeta reciclada tiene que ENSEÑAR la carpeta.
	carpeta := NuevaFilaDrive("", "")
	colocar(t, carpeta, ancho)
	carpeta.Nombre, carpeta.Carpeta, carpeta.Ventana = "Contratos", true, 1280
	carpeta.Tamano, carpeta.Fecha = "—", "6 sept 2026"
	carpeta.Refresh()
	if !glifoVisible(carpeta) {
		t.Error("la carpeta reciclada no enseña su glifo: la tesela salió vacía")
	}
	// Y un FICHERO no lo enseña, o el guardián no estaría mirando nada.
	fichero := NuevaFilaDrive("", "")
	colocar(t, fichero, ancho)
	fichero.Nombre, fichero.Extension, fichero.Ventana = "Contrato.pdf", "pdf", 1280
	fichero.Refresh()
	if glifoVisible(fichero) {
		t.Error("un fichero enseña el glifo de carpeta")
	}
}

// colocar monta la fila en un lienzo y le da su ancho, que es lo que dispara el Layout.
func colocar(t *testing.T, f *FilaDrive, ancho float32) {
	t.Helper()
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, MedidaFilaDrive()))
}

// textosDe devuelve la x de cada rótulo de la fila.
func textosDe(f *FilaDrive) map[string]float32 {
	out := map[string]float32{}
	for _, o := range test.WidgetRenderer(f).Objects() {
		if t, ok := o.(*canvas.Text); ok && t.Text != "" {
			out[t.Text] = t.Position().X
		}
	}
	return out
}

// glifoVisible dice si la tesela está enseñando su glifo de carpeta.
//
// Desde el 2026-09-22 el glifo es `iconos.DriveCarpeta` —un `canvas.Raster`— y no dos
// rectángulos de color, así que ya no se cuenta por color: el único ráster de la fila
// es ese. Si algún día hay dos, este ayudante se pone rojo solo (cuenta, no busca).
func glifoVisible(f *FilaDrive) bool {
	visto := false
	for _, o := range test.WidgetRenderer(f).Objects() {
		if r, ok := o.(*canvas.Raster); ok && r.Visible() {
			if visto {
				panic("la fila tiene dos rásteres: este ayudante ya no identifica el glifo")
			}
			visto = true
		}
	}
	return visto
}
