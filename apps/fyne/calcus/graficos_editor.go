// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El editor del gráfico: `ChartEditor`
//
// Ocupa el cuerpo de la tarjeta al pulsar el ajuste: datos (tipo, título, rango y
// series por filas), ejes, leyenda, etiquetas, retícula y apilado, y por serie su
// color, si va como línea (combinado) y si va al eje secundario. Aplicar manda el
// parche; cancelar lo tira. `flex flex-col gap-2 p-2`.
//
// ponytail: los campos no crecen con el zoom de la rejilla (el resto de la tarjeta sí):
// son los widgets de formulario de la suite, que miden lo que dice su talla.

// El orden del selector de tipo es el de `TYPE_KEY` (Object.keys).
var tiposEditor = []string{"bar", "hbar", "line", "area", "combo", "pie", "donut", "scatter", "radar"}

// Las posiciones de la leyenda (`LEGEND_OPTIONS`).
var leyendasEditor = []struct{ Valor, Clave string }{
	{"bottom", "chart.legend.bottom"},
	{"top", "chart.legend.top"},
	{"right", "chart.legend.right"},
	{"none", "chart.legend.none"},
}

// estadoEditor es el `useState` del editor.
type estadoEditor struct {
	tipo, titulo, rango, leyenda, ejeX, ejeY string
	porFilas, apilado, etiquetas, reticula   bool
	colores                                  []string
	lineas, secundario                       []int
}

type editorGrafico struct {
	widget.BaseWidget
	lz       *Lienzo
	series   []string
	e        *estadoEditor
	aplicar  func(map[string]any)
	cancelar func()
	cuerpo   *fyne.Container
	// listo dice que ya está montado: rehacerse al construirse NO debe repintar la
	// rejilla, que es quien lo está construyendo desde su Layout.
	listo bool
}

func nuevoEditorGrafico(lz *Lienzo, g Grafico, series []string, aplicar func(map[string]any), cancelar func()) *editorGrafico {
	leyenda := g.Legend
	if leyenda == "" {
		leyenda = "bottom"
	}
	ed := &editorGrafico{lz: lz, series: series, aplicar: aplicar, cancelar: cancelar, e: &estadoEditor{
		tipo: g.Type, titulo: g.Title, rango: g.Range, leyenda: leyenda, ejeX: g.XTitle, ejeY: g.YTitle,
		porFilas: g.ByRow, apilado: g.Stacked, etiquetas: g.Labels, reticula: g.Grid == nil || *g.Grid,
		colores: slices.Clone(g.Colors), lineas: slices.Clone(g.LineSeries), secundario: slices.Clone(g.SecondaryAxis),
	}}
	ed.cuerpo = container.New(ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(2), Abj: tema.Espacio(2)})
	ed.ExtendBaseWidget(ed)
	ed.rehacer()
	ed.listo = true
	return ed
}

func (ed *editorGrafico) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(ed.cuerpo)
}

// parche es lo que manda `apply`.
func (ed *editorGrafico) parche() (map[string]any, bool) {
	e := ed.e
	if _, ok := formula.ParseRangeRef(e.rango); !ok {
		return nil, false
	}
	p := map[string]any{
		"type": e.tipo, "title": strings.TrimSpace(e.titulo), "range": strings.TrimSpace(e.rango),
		"byRow": e.porFilas, "stacked": Apilable(e.tipo) && e.apilado, "legend": e.leyenda,
		"labels": e.etiquetas, "grid": e.reticula, "xTitle": "", "yTitle": "", "colors": aAny(e.colores),
		"lineSeries": []any{}, "secondaryAxis": []any{},
	}
	if ConEjes(e.tipo) {
		p["xTitle"], p["yTitle"] = strings.TrimSpace(e.ejeX), strings.TrimSpace(e.ejeY)
	}
	if e.tipo == "combo" {
		p["lineSeries"] = aAny(e.lineas)
	}
	if esCartesiano(e.tipo) && !e.apilado {
		p["secondaryAxis"] = aAny(e.secundario)
	}
	return p, true
}

func aAny[T any](s []T) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

// rehacer vuelve a montar los campos: el tipo decide cuáles se ven.
func (ed *editorGrafico) rehacer() {
	e := ed.e
	t := func(clave string) string { return ed.lz.t(clave, nil) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	campo := func(clave string, o fyne.CanvasObject) fyne.CanvasObject {
		return container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
			ui.NuevoParrafo(t(clave), tema.Texto2XS, ui.Normal, "text-subtle", v), o)
	}
	entrada := func(valor, marca string, poner func(string)) *ui.Entrada {
		en := ui.NuevaEntrada(marca).Con(ui.CampoSM)
		en.PonerTexto(valor)
		en.AlCambiar(poner)
		return en
	}
	casilla := func(clave string, valor bool, poner func(bool)) fyne.CanvasObject {
		c := ui.NuevaCasilla(t(clave), poner)
		c.Marcada = valor
		return c
	}

	rotulos := make([]string, len(tiposEditor))
	elegido := 0
	for i, tp := range tiposEditor {
		rotulos[i] = t("chart.type." + tp)
		if tp == e.tipo {
			elegido = i
		}
	}
	tipo := ui.NuevoSelector(rotulos, func(i int) { e.tipo = tiposEditor[i]; ed.rehacer() }).Con(ui.CampoSM)
	tipo.Elegida = elegido
	titulo := entrada(e.titulo, ed.lz.t("chart.defaultTitle", i18n.Vars{"type": t("chart.type." + e.tipo), "range": e.rango}),
		func(s string) { e.titulo = s })
	rango := entrada(e.rango, "", func(s string) {
		_, antes := formula.ParseRangeRef(e.rango)
		e.rango = s
		if _, ahora := formula.ParseRangeRef(s); ahora != antes {
			ed.rehacer() // el rango inválido se marca y el botón se apaga
		}
	})
	_, valido := formula.ParseRangeRef(e.rango)
	rango.Invalida = !valido
	rango.AlTeclear = alIntro(func() {
		if p, ok := ed.parche(); ok {
			ed.aplicar(p)
		}
	})
	filas := []fyne.CanvasObject{
		campo("chart.field.type", tipo), campo("chart.field.title", titulo), campo("chart.field.range", rango),
		casilla("chart.byRow", e.porFilas, func(si bool) { e.porFilas = si }),
	}
	if ConEjes(e.tipo) {
		filas = append(filas, container.New(&ui.Rejilla{Hueco: tema.Espacio(1.5), Fijas: 2},
			campo("chart.xTitle", entrada(e.ejeX, "", func(s string) { e.ejeX = s })),
			campo("chart.yTitle", entrada(e.ejeY, "", func(s string) { e.ejeY = s }))))
	}
	rotLey := make([]string, len(leyendasEditor))
	eleLey := 0
	for i, l := range leyendasEditor {
		rotLey[i] = t(l.Clave)
		if l.Valor == e.leyenda {
			eleLey = i
		}
	}
	leyenda := ui.NuevoSelector(rotLey, func(i int) { e.leyenda = leyendasEditor[i].Valor }).Con(ui.CampoSM)
	leyenda.Elegida = eleLey
	filas = append(filas, campo("chart.legend", leyenda))
	opciones := []fyne.CanvasObject{casilla("chart.labels", e.etiquetas, func(si bool) { e.etiquetas = si })}
	if ConEjes(e.tipo) {
		opciones = append(opciones, casilla("chart.grid", e.reticula, func(si bool) { e.reticula = si }))
	}
	if Apilable(e.tipo) {
		opciones = append(opciones, casilla("chart.stacked", e.apilado, func(si bool) { e.apilado = si; ed.rehacer() }))
	}
	filas = append(filas, container.New(ui.Columna{Hueco: tema.Espacio(1)}, opciones...))
	if len(ed.series) > 0 {
		filas = append(filas, ed.listaSeries(v))
	}
	aplicar := botonPop(t("action.apply"), false, func() {
		if p, ok := ed.parche(); ok {
			ed.aplicar(p)
		}
	})
	aplicar.Inerte = !valido
	cancelar := botonPop(t("action.cancel"), false, ed.cancelar)
	filas = append(filas, container.New(&ui.Rejilla{Hueco: tema.Espacio(1.5), Fijas: 2}, aplicar, cancelar))

	ed.cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)}
	if !ed.listo {
		return
	}
	ed.cuerpo.Refresh()
	ed.Refresh()
	// El editor cambia de alto: la tarjeta se rehace (ver `colocarFlotante`).
	ed.lz.refrescarTarjetas()
}

// listaSeries es la tabla de series: color, nombre y las dos casillas.
func (ed *editorGrafico) listaSeries(v tema.Variante) fyne.CanvasObject {
	e := ed.e
	t := func(clave string) string { return ed.lz.t(clave, nil) }
	conEje := esCartesiano(e.tipo)
	titulo := ui.NuevoParrafo(t("chart.series"), tema.Texto2XS, ui.Normal, "text-subtle", v)
	var der []fyne.CanvasObject
	if conEje && e.tipo == "combo" {
		der = append(der, ui.NuevoParrafo(t("chart.series.line"), tema.Texto2XS, ui.Normal, "text-subtle", v))
	}
	if conEje && !e.apilado {
		der = append(der, ui.NuevoParrafo(t("chart.series.secondaryAxis"), tema.Texto2XS, ui.Normal, "text-subtle", v))
	}
	lista := make([]fyne.CanvasObject, 0, 1+len(ed.series))
	lista = append(lista, entre(titulo, container.New(ui.Fila{Hueco: tema.Espacio(2)}, der...)))
	alternar := func(s *[]int, k int) func(bool) {
		return func(si bool) {
			*s = slices.DeleteFunc(*s, func(x int) bool { return x == k })
			if si {
				*s = append(*s, k)
				slices.Sort(*s)
			}
		}
	}
	for k, nombre := range ed.series {
		col := colorSerie(Grafico{Colors: e.colores}, k)
		muestra := ui.NuevaMuestraColor(colorDe(col), ed.lz.t("chart.series.color", i18n.Vars{"name": nombre}), nil)
		muestra.AlPulsar = func() { ed.elegirColor(k, muestra) }
		izq := container.New(fila{hueco: tema.Espacio(1.5), elastico: 1}, muestra,
			ui.NuevoParrafo(nombre, tema.TextoXS, ui.Normal, "text", v).Recortado(1))
		var marcas []fyne.CanvasObject
		if conEje && e.tipo == "combo" {
			c := ui.NuevaCasilla(ed.lz.t("chart.series.line.label", i18n.Vars{"name": nombre}), alternar(&e.lineas, k))
			c.Marcada, c.SinEtiqueta = slices.Contains(e.lineas, k), true
			marcas = append(marcas, c)
		}
		if conEje && !e.apilado {
			c := ui.NuevaCasilla(ed.lz.t("chart.series.secondaryAxis.label", i18n.Vars{"name": nombre}), alternar(&e.secundario, k))
			c.Marcada, c.SinEtiqueta = slices.Contains(e.secundario, k), true
			marcas = append(marcas, c)
		}
		lista = append(lista, container.New(fila{hueco: tema.Espacio(2), elastico: 0}, izq,
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, marcas...)))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, lista...)
}

// elegirColor abre las muestras de la paleta de series (y «automático») bajo la muestra.
//
// ponytail: la web usa un `<select>` nativo con «■» de cada color; aquí es un emergente
// con las muestras redondas de la suite, que se eligen igual y se ven mejor.
func (ed *editorGrafico) elegirColor(k int, disparador fyne.CanvasObject) {
	var em *ui.Emergente
	elegir := func(hex string) {
		for len(ed.e.colores) <= k {
			ed.e.colores = append(ed.e.colores, "")
		}
		ed.e.colores[k] = hex
		em.Cerrar()
		ed.rehacer()
	}
	objs := make([]fyne.CanvasObject, 0, 1+len(coloresSerie))
	objs = append(objs, ui.NuevaMuestraColor(colorDe(colorSerie(Grafico{}, k)), ed.lz.t("chart.series.auto", nil), func() { elegir("") }))
	for _, hex := range coloresSerie {
		m := ui.NuevaMuestraColor(colorDe(hex), ed.lz.t("common.colorN", i18n.Vars{"c": hex}), func() { elegir(hex) })
		m.Elegida = k < len(ed.e.colores) && ed.e.colores[k] == hex
		objs = append(objs, m)
	}
	em = ui.MostrarEmergente(disparador, container.New(ui.Fila{Hueco: tema.Espacio(1)}, objs...), rellenoPanel)
}
