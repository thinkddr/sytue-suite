// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El grupo «Esquema» de la pestaña Datos: agrupar y desagrupar filas y columnas, y el
// asistente de Subtotales (`SubtotalMenu` de Datos.tsx).

var (
	anchoPanelSubtotal = tema.Espacio(64) // w-64
	altoListaSubtotal  = tema.Espacio(28) // max-h-28
)

// anchoSelectorCampo es el `width={116}` del RibbonSelect de `PivotField`.
const anchoSelectorCampo float32 = 116

// funcionesSubtotal son SUBTOTAL_FNS: las de la fila de totales sin «Ninguno».
var funcionesSubtotal = []struct{ valor, clave string }{
	{"sum", "query.agg.sum"},
	{"average", "query.agg.avg"},
	{"count", "table.total.count"},
	{"counta", "table.total.counta"},
	{"min", "query.agg.min"},
	{"max", "query.agg.max"},
}

// grupoEsquema son los cinco botones del grupo, como los de la web.
func (lz *Lienzo) grupoEsquema(c *editor.Contexto) []fyne.CanvasObject {
	boton := func(i iconos.Ed, clave, eje, op string) *editor.BotonCinta {
		valor := `{"axis":"` + eje + `","op":"` + op + `"}`
		b := editor.NuevoBotonCinta(i, c.Traducir(clave), func() { c.Mandar(CmdEsquema, valor) })
		b.Inerte = c.SoloLectura
		return b
	}
	var sub *editor.BotonCinta
	sub = editor.NuevoBotonCinta(iconos.EdSubtotal, c.Traducir("data.subtotals"), func() {
		lz.abrirSubtotales(c, sub, &formSubtotal{rango: rangoTexto(lz.Rejilla), grupo: "0", fn: "sum"})
	})
	sub.Inerte = c.SoloLectura
	return []fyne.CanvasObject{
		boton(iconos.EdAgruparFilas, "data.groupRows", "row", "group"),
		boton(iconos.EdDesagruparFilas, "data.ungroupRows", "row", "ungroup"),
		boton(iconos.EdAgruparCols, "data.groupCols", "col", "group"),
		boton(iconos.EdDesagruparCols, "data.ungroupCols", "col", "ungroup"),
		sub,
	}
}

// formSubtotal es el estado del asistente: el rango, la columna de grupo (su índice en el
// rango, como el `<select>`), la función y «Añadir subtotal a» (nil: automático).
type formSubtotal struct {
	rango, grupo, fn string
	anadir           []int
}

// TextosRango es `readSheetGrid`: el texto calculado (sin formato de número) de cada
// celda del rango, por filas; false si el rango no se lee.
func (l *Libro) TextosRango(texto string) ([][]string, bool) {
	r, ok := formula.ParseRangeRef(texto)
	if !ok {
		return nil, false
	}
	var out [][]string
	for f := r.R1; f <= min(r.R2, Filas-1); f++ {
		fila := []string{}
		for c := r.C1; c <= min(r.C2, Columnas-1); c++ {
			fila = append(fila, l.textoFiltro(c, f))
		}
		out = append(out, fila)
	}
	return out, true
}

// numeroEs dice si lo escrito se lee como número es-ES (`parseEsNumber` no nulo): para
// marcar a priori las columnas con números.
func numeroEs(texto string) bool {
	_, ok := numeroEsValor(texto)
	return ok
}

// abrirSubtotales es `SubtotalMenu`. Lo que depende del rango —los campos, la lista de
// columnas y si se puede aplicar— se rehace al teclearlo, sin cerrar el panel.
func (lz *Lienzo) abrirSubtotales(c *editor.Contexto, disparador *editor.BotonCinta, f *formSubtotal) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var em *ui.Emergente
	dinamico := container.New(ui.Columna{})
	rango := ui.NuevaEntrada("").ConPropia(campoPanel)
	rango.PonerTexto(f.rango)
	aplicar := func() {
		rejilla, _ := lz.Libro.TextosRango(f.rango)
		cab := cabecerasDe(rejilla)
		gc, _ := strconv.Atoi(f.grupo)
		if _, ok := formula.ParseRangeRef(f.rango); !ok || len(cab) == 0 || gc < 0 || gc >= len(cab) {
			return
		}
		req := map[string]any{"range": strings.ToUpper(strings.TrimSpace(f.rango)), "groupCol": gc, "fn": f.fn}
		if f.anadir != nil {
			req["cols"] = slices.DeleteFunc(slices.Clone(f.anadir), func(i int) bool { return i == gc })
		}
		b, _ := json.Marshal(req)
		c.Mandar(CmdSubtotal, string(b))
		em.Cerrar()
	}
	var rehacer func()
	rehacer = func() {
		dinamico.Objects = lz.cuerpoSubtotales(c, f, v, rehacer, aplicar, func() {
			if _, ok := formula.ParseRangeRef(f.rango); ok {
				b, _ := json.Marshal(map[string]any{"range": strings.ToUpper(strings.TrimSpace(f.rango)), "remove": true})
				c.Mandar(CmdSubtotal, string(b))
				em.Cerrar()
			}
		})
		dinamico.Refresh()
		if em != nil {
			em.Recolocar()
		}
	}
	rango.AlCambiar(func(s string) { f.rango = s; rehacer() })
	rango.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		aplicar()
		return true
	}
	dinamico.Objects = nil
	em = abrirEmergente(disparador, panel{ancho: anchoPanelSubtotal, filas: []fyne.CanvasObject{
		conPie(ui.NuevoParrafo(c.Traducir("data.subtotals"), tema.TextoXS, ui.Medio, "text", v), tema.Espacio(1)),
		conPie(conEtiqueta(c.Traducir("data.rangeWithHeaders"), rango), tema.Espacio(1.5)),
		dinamico,
	}}.objeto())
	rehacer()
}

// cuerpoSubtotales es lo que depende del rango: los dos campos (o el aviso), «Añadir
// subtotal a», la explicación y los dos botones.
func (lz *Lienzo) cuerpoSubtotales(c *editor.Contexto, f *formSubtotal, v tema.Variante, rehacer, aplicar, quitar func()) []fyne.CanvasObject {
	rejilla, _ := lz.Libro.TextosRango(f.rango)
	cab := cabecerasDe(rejilla)
	gc, _ := strconv.Atoi(f.grupo)
	_, rangoOK := formula.ParseRangeRef(f.rango)
	var out []fyne.CanvasObject
	if len(cab) == 0 {
		out = append(out, conPie(meta(c.Traducir("data.subtotals.pickGroup"), v), tema.Espacio(2)))
	} else {
		campos := make([]editor.Opcion, len(cab))
		for i, h := range cab {
			campos[i] = editor.Opcion{Valor: strconv.Itoa(i), Rotulo: h}
			if strings.TrimSpace(h) == "" {
				campos[i].Rotulo = c.Traducir("common.columnN", i18n.Vars{"n": i + 1})
			}
		}
		out = append(out, conPie(container.New(&ui.Rejilla{Hueco: tema.Espacio(1.5), Fijas: 2},
			conEtiqueta(c.Traducir("common.groupBy"), editor.NuevoSelectorCinta(c.Traducir("common.groupBy"),
				anchoSelectorCampo, campos, f.grupo, func(s string) { f.grupo = s; rehacer() })),
			conEtiqueta(c.Traducir("common.function"), editor.NuevoSelectorCinta(c.Traducir("common.function"),
				anchoSelectorCampo, opciones(c, funcionesSubtotal), f.fn, func(s string) { f.fn = s; rehacer() })),
		), tema.Espacio(2)))
		// Sin tocar las casillas se marcan las columnas con algún número: lo que haría la
		// hoja sola. Tocar una fija la lista.
		marcadas := f.anadir
		if marcadas == nil {
			for i := range cab {
				if i != gc && slices.ContainsFunc(rejilla[1:], func(fila []string) bool { return i < len(fila) && numeroEs(fila[i]) }) {
					marcadas = append(marcadas, i)
				}
			}
		}
		var casillas []fyne.CanvasObject
		for i, cf := range campos {
			if i == gc {
				continue
			}
			ca := ui.NuevaCasilla(cf.Rotulo, func(si bool) {
				base := slices.Clone(marcadas)
				if si {
					base = append(base, i)
					slices.Sort(base)
				} else {
					base = slices.DeleteFunc(base, func(x int) bool { return x == i })
				}
				if base == nil {
					base = []int{}
				}
				f.anadir = base
				rehacer()
			})
			ca.Marcada = slices.Contains(marcadas, i)
			casillas = append(casillas, ca)
		}
		// ponytail: `recuadro` lleva `p-1.5` y la lista de la web `px-1.5 py-1`: 1,75 px más
		// de alto por arriba y por abajo. El tope sí se descuenta entero (borde y relleno).
		lista := topeAlto(altoListaSubtotal-2*(tema.Borde+tema.Espacio(1.5)), container.New(ui.Columna{}, casillas...))
		out = append(out, conPie(container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, meta(c.Traducir("data.subtotals.addTo"), v),
			recuadro(lista)), tema.Espacio(1.5)))
	}
	out = append(out, conPie(meta(c.Traducir("data.subtotals.explain"), v), tema.Espacio(2)))
	quitarB := botonPop(c.Traducir("data.subtotals.removeAll"), false, quitar)
	quitarB.Inerte = !rangoOK || c.SoloLectura
	aplicarB := botonPop(c.Traducir("action.apply"), false, aplicar)
	aplicarB.Inerte = !rangoOK || len(cab) == 0 || gc < 0 || gc >= len(cab) || c.SoloLectura
	return append(out, rejillaBotones(2, quitarB, aplicarB))
}

// cabecerasDe es la primera fila de lo leído (sin nada si no hay filas).
func cabecerasDe(rejilla [][]string) []string {
	if len(rejilla) == 0 {
		return nil
	}
	return rejilla[0]
}

// meta es `sytue-meta-dense`: `text-2xs` en `text-subtle`.
func meta(texto string, v tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "text-subtle", v)
}

// conPie añade el `pb-*` de un bloque del panel.
func conPie(o fyne.CanvasObject, pie float32) fyne.CanvasObject {
	return container.New(margenAbajo{pie}, o)
}
