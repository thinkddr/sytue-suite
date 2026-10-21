// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// tabla es `<Table>` de la suite con las columnas del informe: las cifras a la derecha,
// el reparto de `table-layout: auto` —el mismo que el de la lista del ERP— y las celdas
// que no caben, en varias líneas. Medido en el arnés: cabecera 31,5 con su filete, filas
// de 36,75 separadas por un filete y la última sin él (`last:border-b-0`).
//
// NO es `ui.Tabla`, y no por estilo: su cuerpo es un `widget.List`, que se queda la
// rueda del ratón aunque no tenga nada que desplazar, y aquí la tabla va dentro de la
// página, que es lo que desplaza en la web. Por eso se pinta con primitivas, como la
// tabla de la lista.
//
// ponytail: todas las filas se pintan (la web tampoco virtualiza aquí). Con el tope de
// mil filas por parte del servicio son mil filas de texto; si pesa, cortar en pantalla
// a unas cien y dejar el resto para el Excel.
func (x *Informe) tabla(v tema.Variante, t api.ERPTablaInforme, ancho float32) (fyne.CanvasObject, float32) {
	n := len(t.Columnas)
	dentro := max(ancho-2*tema.Borde, 0)
	anchos, celdas, derecha := x.medirTabla(t, dentro)
	xs := make([]float32, n+1)
	for i, a := range anchos {
		xs[i+1] = xs[i] + a
	}
	// Los altos primero: la caja va debajo de todo y tiene que saber lo que mide.
	lineas := make([][][]string, len(celdas))
	altos := make([]float32, len(celdas))
	for f := range celdas {
		lineas[f] = make([][]string, n)
		altos[f] = tema.TextoBase.Linea
		for i, cel := range celdas[f] {
			lineas[f][i] = lineasCelda(cel, anchos[i]-2*ui.MargenCelda)
			altos[f] = max(altos[f], float32(len(lineas[f][i]))*tema.TextoBase.Linea)
		}
		altos[f] += 2 * ui.AltoCelda
	}
	alto := AltoTablaInforme(altos)
	// El contenido, en coordenadas de DENTRO de la caja: sin el borde.
	in := &lienzo{}
	for i, c := range t.Columnas {
		vs := ui.NuevoVersal(c.Etiqueta, tema.TextoXS, ui.Negrita, tema.Color("text", v))
		xx := xs[i] + ui.MargenCelda
		if derecha[i] {
			xx = xs[i+1] - ui.MargenCelda - vs.Ancho()
		}
		in.poner(vs.Objeto(), xx, ui.AltoCelda, vs.Ancho(), tema.TextoXS.Linea)
	}
	total := max(xs[n], dentro)
	y := CabeceraTablaInforme
	in.poner(canvas.NewRectangle(tema.Color("border", v)), 0, y, total, tema.Borde)
	y += tema.Borde
	if len(celdas) == 0 {
		// `colSpan` con `px-3 py-10 text-center text-text-muted`.
		e := ui.Texto(x.t("ai.empty"), tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
		in.texto(e, (total-ui.Medir(e.Text, tema.TextoBase, ui.Normal).Width)/2, y+(ui.AltoVacio-tema.TextoBase.Linea)/2)
	}
	for f := range celdas {
		in.poner(NuevaFilaInforme(), 0, y, total, altos[f])
		pintarFilaTablaInforme(in, v, y, lineas[f], celdas[f], xs, derecha)
		y += altos[f]
		if f < len(celdas)-1 {
			in.poner(canvas.NewRectangle(tema.Color("border-subtle", v)), 0, y, total, tema.Borde)
			y += tema.Borde
		}
	}
	contenido, altoDentro := in.objeto(alto - 2*tema.Borde)
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG), ancho, alto)
	if total > dentro {
		// `overflow-x-auto`: lo que no cabe se desplaza de lado dentro de la caja, que
		// se queda quieta con su borde. Lo vertical —la rueda, el dedo— sigue siendo de
		// la página, como en la web.
		contenido = nuevoDesplazaLado(contenido, total, x.Afuera)
	}
	l.poner(contenido, tema.Borde, tema.Borde, dentro, altoDentro)
	return l.objeto(alto)
}

// pintarFilaTablaInforme mantiene la medida tabular y la alineación de cada
// columna al pintar las líneas que ya calculó la tabla.
func pintarFilaTablaInforme(in *lienzo, v tema.Variante, y float32, lineas [][]string, celdas []celdaERP, xs []float32, derecha []bool) {
	for i := range lineas {
		for k, s := range lineas[i] {
			xx := xs[i] + ui.MargenCelda
			yy := y + ui.AltoCelda + float32(k)*tema.TextoBase.Linea
			if strings.ContainsAny(s, "0123456789") {
				// .erp-workspace table hereda tabular-nums también en fechas.
				// Medida y pintura usan la misma pieza compartida de la suite.
				tabular := ui.NuevoTextoTabular(s, tema.TextoBase, ui.Normal, tema.Color("text", v))
				w := tabular.Ancho()
				if derecha[i] {
					xx = xs[i+1] - ui.MargenCelda - w
				}
				in.poner(tabular.Objeto(), xx, yy, w, tema.TextoBase.Linea)
			} else {
				if derecha[i] {
					xx = xs[i+1] - ui.MargenCelda - ui.Medir(s, tema.TextoBase, ui.Normal).Width
				}
				in.texto(textoCelda(s, celdas[i], v), xx, yy)
			}
		}
	}
}

// Desplazador es lo que se lleva lo vertical de una tabla que se desplaza de lado: el
// desplazador de la página (`container.Scroll` lo es).
type Desplazador = fyne.Scrollable

// desplazaLado es `overflow-x-auto`: la pieza nació aquí y subió a `ui.DesplazaLado`
// cuando Sytue Bases necesitó lo mismo (`docs/CONVENTIONS.md` §13.2).
type desplazaLado = ui.DesplazaLado

func nuevoDesplazaLado(o fyne.CanvasObject, ancho float32, afuera func() Desplazador) *desplazaLado {
	return ui.NuevoDesplazaLado(o, ancho, afuera)
}

// medirTabla resuelve la tabla para `dentro` (la caja sin sus bordes): el ancho de cada
// columna, las celdas ya escritas y qué columnas van a la derecha.
func (x *Informe) medirTabla(t api.ERPTablaInforme, dentro float32) ([]float32, [][]celdaERP, []bool) {
	n := len(t.Columnas)
	maxs, mins, fijas := make([]float32, n), make([]float32, n), make([]float32, n)
	derecha := make([]bool, n)
	for i, c := range t.Columnas {
		derecha[i] = c.Tipo == api.ERPColDinero || c.Tipo == api.ERPColNumero
		cab := ui.MedirVersal(c.Etiqueta, tema.TextoXS, ui.Negrita) + 2*ui.MargenCelda
		maxs[i], mins[i] = cab, cab
	}
	celdas := make([][]celdaERP, len(t.Filas))
	for f, fila := range t.Filas {
		celdas[f] = make([]celdaERP, n)
		for i := range n {
			cel := celdaERP{texto: ValorInforme(columnaDe(t, i), celdaDeFila(fila, i), x.loc), escalon: tema.TextoBase, tinta: "text"}
			celdas[f][i] = cel
			maxs[i] = max(maxs[i], anchoCelda(cel))
			mins[i] = max(mins[i], minCelda(cel))
		}
	}
	return RepartoERP(dentro, maxs, mins, fijas), celdas, derecha
}

// CabeceraTablaInforme es el `th` de Table.tsx: `py-2.5` y la caja de `text-xs`.
var CabeceraTablaInforme = 2*ui.AltoCelda + tema.TextoXS.Linea

// AltoTablaInforme es lo que ocupa la tabla con esas filas: los bordes, la cabecera con
// su filete, las filas con el de entre ellas, o el `py-10` del vacío.
func AltoTablaInforme(altos []float32) float32 {
	alto := 2*tema.Borde + CabeceraTablaInforme + tema.Borde
	if len(altos) == 0 {
		return alto + ui.AltoVacio
	}
	for i, a := range altos {
		alto += a
		if i < len(altos)-1 {
			alto += tema.Borde
		}
	}
	return alto
}

// FilaInforme es el fondo de una fila de la tabla de un informe: `hover:bg-bg-subtle`.
// Es un widget solo para enterarse del ratón; los textos van encima.
type FilaInforme struct {
	widget.BaseWidget
	encima bool
	fondo  *canvas.Rectangle
}

var _ desktop.Hoverable = (*FilaInforme)(nil)

// NuevaFilaInforme crea el fondo.
func NuevaFilaInforme() *FilaInforme {
	f := &FilaInforme{fondo: canvas.NewRectangle(color.NRGBA{})}
	f.ExtendBaseWidget(f)
	return f
}

// MouseIn enciende el fondo.
func (f *FilaInforme) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no cambia nada.
func (f *FilaInforme) MouseMoved(*desktop.MouseEvent) {}

// MouseOut lo apaga.
func (f *FilaInforme) MouseOut() { f.encima = false; f.Refresh() }

// CreateRenderer es el rectángulo del fondo.
func (f *FilaInforme) CreateRenderer() fyne.WidgetRenderer {
	r := &filaInformeRenderer{f: f}
	r.Refresh()
	return r
}

type filaInformeRenderer struct{ f *FilaInforme }

func (r *filaInformeRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *filaInformeRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.f.fondo} }
func (r *filaInformeRenderer) Destroy()                     {}
func (r *filaInformeRenderer) Layout(t fyne.Size)           { r.f.fondo.Resize(t) }

func (r *filaInformeRenderer) Refresh() {
	c := color.NRGBA{}
	if r.f.encima {
		c = tema.Color("bg-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	r.f.fondo.FillColor = c
	r.f.fondo.Refresh()
}
