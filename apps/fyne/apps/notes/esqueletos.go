// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los esqueletos de carga de Notes son `Esqueletos.tsx`: lo que se ve mientras llegan
// las notas de un cuaderno, con la GEOMETRÍA de lo que va a llegar. Era una rueda
// centrada, y al llegar los datos la columna de lectura aparecía de golpe donde antes
// había un hueco. Tres formas, una por manera de leer un cuaderno —filas (lista y
// diario), tarjetas (columnas y lienzo libre) y la hoja (la nota del árbol)—, más las
// ramas del panel del árbol y los cuadernos del menú.
//
// El pulso y su «menos movimiento» los pone `ui.Esqueleto`; aquí solo hay medidas.

// FormaEsqueleto es cuál de las tres formas pinta el lienzo mientras carga.
type FormaEsqueleto uint8

// Las tres formas, como el `FormaEsqueleto` de la web.
const (
	EsqueletoFilas FormaEsqueleto = iota
	EsqueletoTarjetas
	EsqueletoHoja
)

// Anchos y altos de la web: con todas las líneas iguales parece una tabla vacía, y las
// notas de verdad no miden todas lo mismo.
var (
	anchosFilaEsqueleto    = []float32{0.62, 0.46, 0.74, 0.38, 0.56, 0.68}
	altosTarjetaEsqueleto  = [][]float32{{7, 4.5}, {5, 8}, {6}}
	anchosParrafoEsqueleto = []float32{1, 0.94, 0.97, 0.88, 0.62}
	ramasEsqueleto         = []struct {
		nivel int
		ancho float32
	}{{0, 0.70}, {1, 0.55}, {1, 0.62}, {0, 0.48}, {0, 0.66}}
)

// AltoFilaEsqueleto es el `h-8` de una fila de carga: lo que mide una línea de la lista.
const AltoFilaEsqueleto = 8 * 0.25 * tema.Rem

// forma es la que toca al cuaderno que se está abriendo.
func (l *Lienzo) forma() FormaEsqueleto {
	switch {
	case l.Llenar:
		return EsqueletoHoja
	case l.Cuaderno.Vista == Lista:
		return EsqueletoFilas
	default:
		return EsqueletoTarjetas
	}
}

// esqueleto es el lienzo mientras carga, con la forma del cuaderno.
func (l *Lienzo) esqueleto() fyne.CanvasObject {
	switch l.forma() {
	case EsqueletoHoja:
		// `notes-sheet grid content-start gap-6`: el título y, debajo, el párrafo.
		lineas := make([]fyne.CanvasObject, 0, len(anchosParrafoEsqueleto))
		for _, f := range anchosParrafoEsqueleto {
			lineas = append(lineas, ui.NuevoEsqueletoFraccion(f, 0))
		}
		return container.New(ui.Margen{Izq: tema.Espacio(6), Der: tema.Espacio(6), Arr: tema.Espacio(8), Abj: tema.Espacio(8)},
			ui.EnColumna(tema.Espacio(6),
				ui.NuevoEsqueletoFraccion(0.45, 1.75*tema.Rem),
				ui.EnColumna(tema.Espacio(3), lineas...)))
	case EsqueletoFilas:
		// La misma columna y el mismo sangrado que `reparto` en filas: la marca cae en
		// el borde de la columna y la línea empieza donde empezará el título.
		filas := make([]fyne.CanvasObject, 0, len(anchosFilaEsqueleto))
		for _, f := range anchosFilaEsqueleto {
			filas = append(filas, filaEsqueleto(0, ui.NuevoEsqueleto(1.25*tema.Rem, 0.375*tema.Rem),
				ui.NuevoEsqueletoFraccion(f, 0.875*tema.Rem)))
		}
		margen := max(l.margenDiario()-RellenoFilaX, 0)
		return container.New(&acotado{ancho: AnchoLectura + 2*tema.Espacio(6)},
			container.New(ui.Margen{Izq: margen, Der: margen, Abj: tema.Espacio(10)}, ui.EnColumna(0, filas...)))
	case EsqueletoTarjetas:
	}
	// La rejilla de columnas tal cual: en el teléfono es una, y en escritorio, tres.
	cols := make([]fyne.CanvasObject, len(altosTarjetaEsqueleto))
	for i, altos := range altosTarjetaEsqueleto {
		tarjetas := make([]fyne.CanvasObject, 0, len(altos))
		for _, alto := range altos {
			e := ui.NuevoEsqueleto(0, alto*tema.Rem)
			e.Radio = tema.RadioLG
			tarjetas = append(tarjetas, e)
		}
		cols[i] = ui.EnColumna(HuecoLista, tarjetas...)
	}
	return conMargen(container.New(&rejillaVentana{hueco: HuecoColumnas, cortes: ui.Cortes{1, 0, 2, 3, 0, 0}}, cols...))
}

// esqueletoRamas son las filas del panel de notas del árbol mientras carga: a la altura
// de una rama, con alguna sangrada para que se lea como árbol y no como lista.
func esqueletoRamas() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(ramasEsqueleto))
	for _, r := range ramasEsqueleto {
		filas = append(filas, filaEsqueleto(float32(r.nivel)*1.25*tema.Rem, ui.NuevoEsqueletoFraccion(r.ancho, 0.875*tema.Rem)))
	}
	return container.New(ui.Margen{Izq: RellenoListaArbol, Der: RellenoListaArbol, Arr: RellenoListaArbol, Abj: RellenoListaArbol},
		ui.EnColumna(0.5*0.25*tema.Rem, filas...))
}

// esqueletoCuadernos son los cuadernos del menú mientras se cargan: filas con la altura
// de una fila del árbol y la forma de un destino, tintadas para el cromo oscuro (un
// `bg-subtle` ahí sería un destello claro), como el lateral de Calendar.
func esqueletoCuadernos() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 3)
	for i := range filas {
		e := ui.NuevoEsqueleto(0, 2*tema.Rem).SobreCromo()
		e.Radio = tema.Forma("radius-nav")
		filas[i] = e
	}
	return ui.EnColumna(0.5*0.25*tema.Rem, filas...)
}

// filaEsqueleto es una fila de carga (`flex h-8 items-center gap-2.5 px-2`): una marca
// opcional y la línea, que se queda con el resto del ancho y pinta su fracción.
func filaEsqueleto(sangria float32, partes ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&filaCarga{sangria: sangria}, partes...)
}

type filaCarga struct{ sangria float32 }

var _ fyne.Layout = (*filaCarga)(nil)

func (f *filaCarga) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := 2*RellenoFilaX + f.sangria
	for i, o := range objs {
		if i > 0 {
			ancho += 2.5 * 0.25 * tema.Rem
		}
		ancho += o.MinSize().Width
	}
	return fyne.NewSize(ancho, AltoFilaEsqueleto)
}

func (f *filaCarga) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := RellenoFilaX + f.sangria
	for i, o := range objs {
		m := o.MinSize()
		ancho := m.Width
		if i == len(objs)-1 {
			// La última es la línea: llega hasta el relleno de la derecha.
			ancho = max(t.Width-x-RellenoFilaX, m.Width)
		}
		o.Move(fyne.NewPos(x, float32(int((t.Height-m.Height)/2))))
		o.Resize(fyne.NewSize(ancho, m.Height))
		x += ancho + 2.5*0.25*tema.Rem
	}
}
