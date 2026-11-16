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
// El pulso y su «menos movimiento» los ponen `ui.Esqueleto` y `ui.FilasEsqueleto`; aquí
// solo hay medidas.

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

// esqueleto es el lienzo mientras carga, con la forma del cuaderno. La hoja y las
// tarjetas son UN `ui.BloquesEsqueleto`: laten a una y se anuncian una vez, como el
// `role="status"` de la web; las filas, un `ui.FilasEsqueleto`.
func (l *Lienzo) esqueleto() fyne.CanvasObject {
	switch l.forma() {
	case EsqueletoHoja:
		// `notes-sheet grid content-start gap-6`: el título y, debajo, el párrafo.
		b := ui.NuevosBloquesEsqueleto()
		lineas := make([]fyne.CanvasObject, 0, len(anchosParrafoEsqueleto))
		for _, f := range anchosParrafoEsqueleto {
			lineas = append(lineas, b.BloqueFraccion(f, 0))
		}
		return b.Con(container.New(ui.Margen{Izq: tema.Espacio(6), Der: tema.Espacio(6), Arr: tema.Espacio(8), Abj: tema.Espacio(8)},
			ui.EnColumna(tema.Espacio(6),
				b.BloqueFraccion(0.45, 1.75*tema.Rem),
				ui.EnColumna(tema.Espacio(3), lineas...))))
	case EsqueletoFilas:
		// La misma columna y el mismo sangrado que `reparto` en filas: la marca cae en
		// el borde de la columna y la línea empieza donde empezará el título.
		filas := filasEsqueleto(len(anchosFilaEsqueleto), fyne.NewSize(1.25*tema.Rem, 0.375*tema.Rem), anchosFilaEsqueleto, nil, 0)
		margen := max(l.margenDiario()-RellenoFilaX, 0)
		return container.New(&acotado{ancho: AnchoLectura + 2*tema.Espacio(6)},
			container.New(ui.Margen{Izq: margen, Der: margen, Abj: tema.Espacio(10)}, filas))
	case EsqueletoTarjetas:
	}
	// La rejilla de columnas tal cual: en el teléfono es una, y en escritorio, tres.
	b := ui.NuevosBloquesEsqueleto()
	cols := make([]fyne.CanvasObject, len(altosTarjetaEsqueleto))
	for i, altos := range altosTarjetaEsqueleto {
		tarjetas := make([]fyne.CanvasObject, 0, len(altos))
		for _, alto := range altos {
			e := b.Bloque(0, alto*tema.Rem)
			e.CornerRadius = tema.RadioLG
			tarjetas = append(tarjetas, e)
		}
		cols[i] = ui.EnColumna(HuecoLista, tarjetas...)
	}
	return b.Con(conMargen(container.New(&rejillaVentana{hueco: HuecoColumnas, cortes: ui.Cortes{1, 0, 2, 3, 0, 0}}, cols...)))
}

// esqueletoRamas son las filas del panel de notas del árbol mientras carga: a la altura
// de una rama, con alguna sangrada para que se lea como árbol y no como lista.
func esqueletoRamas() fyne.CanvasObject {
	anchos := make([]float32, len(ramasEsqueleto))
	sangrias := make([]float32, len(ramasEsqueleto))
	for i, r := range ramasEsqueleto {
		anchos[i], sangrias[i] = r.ancho, float32(r.nivel)*1.25*tema.Rem
	}
	return container.New(ui.Margen{Izq: RellenoListaArbol, Der: RellenoListaArbol, Arr: RellenoListaArbol, Abj: RellenoListaArbol},
		filasEsqueleto(len(ramasEsqueleto), fyne.Size{}, anchos, sangrias, 0.5*0.25*tema.Rem))
}

// esqueletoCuadernos son los cuadernos del menú mientras se cargan: filas con la altura
// de una fila del árbol y la forma de un destino, tintadas para el cromo oscuro (un
// `bg-subtle` ahí sería un destello claro), como el lateral de Calendar.
//
// Es `ui.FilasEsqueleto` con una raya a toda la fila: un solo latido para las tres.
func esqueletoCuadernos() fyne.CanvasObject {
	return ui.NuevasFilasEsqueleto(3, ui.SinDelantero, 1).ConReceta(ui.RecetaEsqueleto{
		Alto: 2 * tema.Rem, Hueco: 0.5 * 0.25 * tema.Rem,
		Rayas: []ui.RayaEsqueleto{{Alto: 2 * tema.Rem, Radio: tema.Forma("radius-nav")}},
	}).SobreCromo()
}

// filasEsqueleto son las filas de carga (`flex h-8 items-center gap-2.5 px-2`): una
// marca opcional (con medida cero no la hay) y la línea, que se queda con el resto del
// ancho y pinta su fracción. Las sangrías hacen que se lea como árbol; hueco es el aire
// entre filas. Es `ui.FilasEsqueleto` con esa receta: un solo latido para todas.
func filasEsqueleto(n int, marca fyne.Size, anchos, sangrias []float32, hueco float32) *ui.FilasEsqueleto {
	delantero := ui.SinDelantero
	if marca.Width > 0 {
		delantero = ui.DelanteroTesela
	}
	return ui.NuevasFilasEsqueleto(n, delantero, 1).ConReceta(ui.RecetaEsqueleto{
		Alto:           AltoFilaEsqueleto,
		Hueco:          hueco,
		Izq:            RellenoFilaX,
		Der:            RellenoFilaX,
		Delantero:      marca,
		HuecoDelantero: 2.5 * 0.25 * tema.Rem,
		Rayas:          []ui.RayaEsqueleto{{Alto: 0.875 * tema.Rem, Partes: anchos}},
		Sangrias:       sangrias,
	})
}
