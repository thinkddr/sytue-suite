// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Los esqueletos de carga del ERP
//
// Son `apps/frontend/apps/erp/src/components/Esqueletos.tsx`.
//
// La lista y la portada pintan la FORMA de lo que llega y no una rueda en un hueco: con
// la rueda la tabla saltaba al cargar, de un bloque de 150 px a seis filas. Las medidas
// son las de la web y `TestLosEsqueletosMidenLoDeLaWeb` las vigila.

// Medidas del esqueleto de la lista.
const (
	// FilasEsqueletoLista es `FILAS_ESQUELETO_LISTA`.
	FilasEsqueletoLista = 6
	// ColumnasEsqueletoMovil es `COLUMNAS_ESQUELETO_MOVIL`: bajo `md` la fila enseña dos.
	ColumnasEsqueletoMovil = 2
	// EsqueletoPX y EsqueletoPY son el `px-3 py-3` de cada fila; EsqueletoHueco, su
	// `gap-4` entre columnas.
	EsqueletoPX    float32 = 3 * 0.25 * tema.Rem
	EsqueletoPY    float32 = 3 * 0.25 * tema.Rem
	EsqueletoHueco float32 = 4 * 0.25 * tema.Rem
	// AltoFilaEsqueleto es una fila sin su filete: `py-3` y la barra de `1rem`.
	AltoFilaEsqueleto float32 = 2*EsqueletoPY + ui.AltoEsqueleto
)

// AnchosEsqueleto son los `ANCHOS_ESQUELETO` de la web: la parte de su celda que ocupa
// cada barra, desplazada una columna en cada fila.
var AnchosEsqueleto = [...]float32{0.70, 0.45, 0.85, 0.55}

// AltoEsqueletoLista es la caja entera: sus dos bordes, las seis filas y los cinco
// filetes de `divide-y` entre ellas.
const AltoEsqueletoLista float32 = 2*tema.Borde + FilasEsqueletoLista*AltoFilaEsqueleto + (FilasEsqueletoLista-1)*tema.Borde

// columnasEsqueleto son las barras de cada fila: una por columna de `modelo.lista`, y
// en el teléfono solo las dos primeras (`max-md:hidden` en las demás).
func (l *Lista) columnasEsqueleto() int {
	n := max(len(l.Modelo.Lista), 1)
	if l.esMovil() {
		n = min(n, ColumnasEsqueletoMovil)
	}
	return n
}

// esqueletoLista es `EsqueletoLista`: la caja de la tabla con seis filas de barras.
func (l *Lista) esqueletoLista(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	return esqueletoDeLista(v, ancho, l.columnasEsqueleto())
}

func esqueletoDeLista(v tema.Variante, ancho float32, cols int) (fyne.CanvasObject, float32) {
	lz := &lienzo{}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radioTarjeta())
	sombra := ui.NuevaSombra(tema.SombraXS, radioTarjeta())
	lz.poner(container.New(&conSombra{caja: caja, sombra: sombra, v: v},
		append([]fyne.CanvasObject{sombra.Objeto()}, caja.Objetos()...)...), 0, 0, ancho, AltoEsqueletoLista)
	dentro := max(ancho-2*tema.Borde, 0)
	celda := max((dentro-2*EsqueletoPX-float32(cols-1)*EsqueletoHueco)/float32(cols), 0)
	y := tema.Borde
	for f := range FilasEsqueletoLista {
		if f > 0 {
			lz.poner(canvas.NewRectangle(tema.Color("border-subtle", v)), tema.Borde, y, dentro, tema.Borde)
			y += tema.Borde
		}
		x := tema.Borde + EsqueletoPX
		for c := range cols {
			barra := ui.NuevoEsqueletoFraccion(AnchosEsqueleto[(f+c)%len(AnchosEsqueleto)], 0)
			lz.poner(barra, x, y+EsqueletoPY, celda, ui.AltoEsqueleto)
			x += celda + EsqueletoHueco
		}
		y += AltoFilaEsqueleto
	}
	return lz.objeto(AltoEsqueletoLista)
}

// Medidas de las líneas de una tarjeta del resumen sin datos (`EsqueletoKPI`): el
// rótulo (`0.75rem`) en la fila de la insignia, la cifra (`1.25rem`) en la caja de
// línea de `text-xl` y la pista (`0.75rem`) en la de `text-xs`.
const (
	AltoBarraRotulo float32 = 0.75 * tema.Rem
	AltoBarraCifra  float32 = 1.25 * tema.Rem
)

// cifraCargando es `CifraCargando`: la barra centrada en la caja de línea de `text-xl`.
func cifraCargando(l *lienzo, x, y, dentro float32) {
	l.poner(ui.NuevoEsqueletoFraccion(0.5, AltoBarraCifra), x, y+(tema.TextoXL.Linea-AltoBarraCifra)/2, dentro, AltoBarraCifra)
}

// tarjetaKPIEsqueleto es `EsqueletoKPI`: la caja de `tarjetaKPI`, a su mismo alto, con
// tres barras donde irán el rótulo, la cifra y la pista.
func tarjetaKPIEsqueleto(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	dentro := max(ancho-2*tema.Borde-2*TarjetaPX, 0)
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	fondo, _ := fondoConCaja(v, tema.Color("surface-raised", v), tema.Color("border", v), tema.SombraSM)
	l.poner(fondo, 0, 0, ancho, AltoKPI)
	fila := ui.MedidaInsignia("").Height
	l.poner(ui.NuevoEsqueletoFraccion(0.4, AltoBarraRotulo), x0, x0+(fila-AltoBarraRotulo)/2, dentro, AltoBarraRotulo)
	y := x0 + fila + HuecoUno
	l.poner(ui.NuevoEsqueletoFraccion(0.6, AltoBarraCifra), x0, y+(tema.TextoXL.Linea-AltoBarraCifra)/2, dentro, AltoBarraCifra)
	y += tema.TextoXL.Linea + HuecoUno
	l.poner(ui.NuevoEsqueletoFraccion(0.8, AltoBarraRotulo), x0, y+(tema.TextoXS.Linea-AltoBarraRotulo)/2, dentro, AltoBarraRotulo)
	return l.objeto(AltoKPI)
}

// esqueleto es lo que pinta la portada mientras llega el catálogo (`EsqueletoKPIs`):
// los tres indicadores en su rejilla, dentro de la página de siempre.
func (b *Bienvenida) esqueleto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(b.Ventana, AnchoContenidoLista)
	rej := ui.RejillaTarjetas(b.Ventana, HuecoTarjetas, CortesResumen)
	cols := max(rej.Columnas(ancho), 1)
	w := rej.AnchoColumna(ancho)
	celdas := make([]celdaRejilla, 0, 3)
	for range 3 {
		o, alto := tarjetaKPIEsqueleto(v, w)
		celdas = append(celdas, celdaRejilla{o, alto})
	}
	o, alto := enRejilla(rej, cols, ancho, celdas)
	return enPagina(o, alto, AnchoContenidoLista)
}
