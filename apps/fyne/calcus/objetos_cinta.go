// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los paneles de la cinta de los objetos de la hoja: `TableMenu` y `SparklineMenu` de
// Datos.tsx, con sus medidas.

var (
	anchoPanelTabla = tema.Espacio(64) // w-64
	anchoPanelMini  = tema.Espacio(56) // w-56
	ladoMuestra     = tema.Espacio(6)  // h-6 w-6
	altoFranjaCab   = tema.Espacio(2)  // la franja de cabecera de la muestra (h-2)
)

// Los selectores de estos paneles, en px como los escribe la web.
const (
	anchoSelectorTotal float32 = 128
	anchoSelectorMini  float32 = 208
)

// bloque es un hijo con su `pb-*` debajo.
func bloque(pb float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(margenAbajo{pb}, o)
}

// ---- tabla con formato ----

// formTabla es el formulario de creación, que sobrevive a rehacer el emergente.
type formTabla struct {
	rango, estilo string
	cabeceras     bool
}

func (lz *Lienzo) botonTabla(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdTabla, c.Traducir("data.table.format"), func() {
		f := &formTabla{rango: "A1:D10", estilo: EstilosTabla[0].ID, cabeceras: true}
		if lz.Estado().Tabla == nil {
			f.rango = lz.Estado().Seleccion // prellena con la selección
		}
		lz.abrirTabla(c, b, f)
	})
	b.Activo = lz.Estado().Tabla != nil // `aria-pressed={table !== null}`
	b.Inerte = c.SoloLectura
	return b
}

// abrirTabla es `TableMenu`: el gestor de la tabla que cubre el ancla o, si no hay, el
// formulario para crear una.
func (lz *Lienzo) abrirTabla(c *editor.Contexto, disparador *editor.BotonCinta, f *formTabla) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirTabla(c, disparador, f)
	}
	var filas []fyne.CanvasObject
	if t := lz.Estado().Tabla; t != nil {
		filas = lz.gestorTabla(c, *t, rehacer)
	} else {
		filas = lz.formularioTabla(c, f, func() { em.Cerrar() }, rehacer)
	}
	em = abrirEmergente(disparador, panel{ancho: anchoPanelTabla, filas: filas}.objeto())
}

// formularioTabla es la rama de creación.
func (lz *Lienzo) formularioTabla(c *editor.Contexto, f *formTabla, cerrar, rehacer func()) []fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, valido := formula.ParseRangeRef(f.rango)
	crear := func() {
		if _, ok := formula.ParseRangeRef(f.rango); !ok {
			return
		}
		b, _ := json.Marshal(map[string]any{"range": strings.ToUpper(strings.TrimSpace(f.rango)), "headers": f.cabeceras, "style": f.estilo})
		c.Mandar(CmdTablaAnadir, string(b))
		cerrar()
	}
	rango := ui.NuevaEntrada("").ConPropia(campoPanel)
	rango.PonerTexto(f.rango)
	rango.AlCambiar(func(s string) {
		_, antes := formula.ParseRangeRef(f.rango)
		f.rango = s
		if _, ahora := formula.ParseRangeRef(s); ahora != antes {
			rehacer() // el botón se enciende o se apaga
		}
	})
	rango.AlTeclear = alIntro(crear)
	cab := ui.NuevaCasilla(c.Traducir("data.table.hasHeaders"), func(si bool) { f.cabeceras = si })
	cab.Marcada = f.cabeceras
	boton := botonPop(c.Traducir("data.table.create"), false, crear)
	boton.Inerte = !valido
	return []fyne.CanvasObject{
		bloque(tema.Espacio(1), ui.NuevoParrafo(c.Traducir("data.table.format"), tema.TextoXS, ui.Medio, "text", v)),
		bloque(tema.Espacio(1.5), conEtiqueta(c.Traducir("data.range"), rango)),
		selectorEstiloTabla(c, f.estilo, func(id string) { f.estilo = id; rehacer() }),
		bloque(tema.Espacio(2), cab),
		anchoEntero(boton),
	}
}

// gestorTabla es `TableManage`: nombre y rango, estilo, fila de totales con la función
// de cada columna, y quitar.
func (lz *Lienzo) gestorTabla(c *editor.Contexto, t TablaFmt, rehacer func()) []fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r, _ := formula.ParseRangeRef(t.Range)
	ancho := r.C2 - r.C1 + 1
	nombreCol := func(i int) string {
		if t.Headers == nil || *t.Headers {
			if n := strings.TrimSpace(lz.Libro.Texto(r.C1+i, r.R1)); n != "" {
				return n
			}
		}
		return c.Traducir("common.columnN", i18n.Vars{"n": i + 1})
	}
	mandar := func(cmd editor.Comando, valor string) {
		c.Mandar(cmd, valor)
		rehacer()
	}
	cab := container.New(fila{hueco: tema.Espacio(2), elastico: 0},
		ui.NuevoParrafo(t.Name, tema.TextoXS, ui.Medio, "text", v).Recortado(1),
		ui.NuevoParrafo(t.Range, tema.Texto2XS, ui.Normal, "text-subtle", v))
	totales := ui.NuevaCasilla(c.Traducir("data.table.totalsRow"), func(bool) { mandar(CmdTablaTotales, "") })
	totales.Marcada = t.TotalsRow
	filas := []fyne.CanvasObject{
		bloque(tema.Espacio(1.5), cab),
		selectorEstiloTabla(c, t.Style, func(id string) { mandar(CmdTablaEstilo, id) }),
		bloque(tema.Espacio(1.5), totales),
	}
	if t.TotalsRow && ancho > 0 {
		opts := make([]editor.Opcion, len(FuncionesTotal))
		for i, fn := range FuncionesTotal {
			opts[i] = editor.Opcion{Valor: fn.Valor, Rotulo: c.Traducir(fn.Clave)}
		}
		lista := make([]fyne.CanvasObject, 0, 2)
		lista = append(lista, bloque(tema.Espacio(1), ui.NuevoParrafo(c.Traducir("data.columnRole"), tema.Texto2XS, ui.Normal, "text-subtle", v)))
		cols := make([]fyne.CanvasObject, 0, ancho)
		for i := range ancho {
			actual := t.Totals[strconv.Itoa(i)]
			if actual == "" {
				actual = "none"
			}
			sel := editor.NuevoSelectorCinta(c.Traducir("data.totalOf", i18n.Vars{"col": nombreCol(i)}), anchoSelectorTotal, opts, actual,
				func(fn string) {
					b, _ := json.Marshal(map[string]any{"name": t.Name, "col": i, "fn": fn})
					mandar(CmdTablaFuncion, string(b))
				})
			sel.Inerte = c.SoloLectura
			cols = append(cols, container.New(fila{hueco: tema.Espacio(1.5), elastico: 0},
				ui.NuevoParrafo(nombreCol(i), tema.TextoXS, ui.Normal, "text", v).Recortado(1), sel))
		}
		lista = append(lista, container.New(ui.Columna{Hueco: tema.Espacio(1)}, cols...))
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		linea := canvas.NewRectangle(tema.Color("border", v))
		filas = append(filas, bloque(tema.Espacio(2), container.New(bordeArribaDe{tema.Espacio(1.5)}, linea,
			topeAlto(tema.Espacio(44), container.New(ui.Columna{}, lista...)))))
	}
	quitar := botonPop(c.Traducir("data.table.remove"), false, func() { mandar(CmdTablaQuitar, "") })
	quitar.Inerte = c.SoloLectura
	return append(filas, anchoEntero(quitar))
}

// bordeArribaDe es `border-border border-t pt-*`: la línea y lo de debajo a `pt`.
type bordeArribaDe struct{ pt float32 }

func (b bordeArribaDe) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde+b.pt)
}

func (b bordeArribaDe) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	d := tema.Borde + b.pt
	objs[1].Move(fyne.NewPos(0, d))
	objs[1].Resize(fyne.NewSize(t.Width, t.Height-d))
}

// selectorEstiloTabla es `StylePicker`: el rótulo y las cinco muestras.
func selectorEstiloTabla(c *editor.Contexto, actual string, elegir func(string)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	muestras := make([]fyne.CanvasObject, len(EstilosTabla))
	for i, e := range EstilosTabla {
		m := nuevaMuestraTabla(e, c.Traducir("common.styleN", i18n.Vars{"label": c.Traducir(e.Clave)}), func() { elegir(e.ID) })
		m.Elegida = e.ID == actual
		muestras[i] = m
	}
	return bloque(tema.Espacio(2), container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoParrafo(c.Traducir("data.table.style"), tema.Texto2XS, ui.Normal, "text-subtle", v),
		container.New(ui.Fila{Hueco: tema.Espacio(1)}, muestras...)))
}

// muestraTabla es un botón del `StylePicker`:
//
//	<button class="relative h-6 w-6 overflow-hidden rounded-[3px] border {elegida ?
//	        'border-text' : 'border-border'}">
//	  <span class="absolute inset-x-0 top-0 h-2" style="background: headerBg">
//	  <span class="absolute inset-x-0 top-2 bottom-0" style="background: band">
type muestraTabla struct {
	ui.Pulsable
	estilo  EstiloTabla
	Elegida bool
}

func nuevaMuestraTabla(e EstiloTabla, nombre string, alPulsar func()) *muestraTabla {
	m := &muestraTabla{estilo: e}
	m.Nombre, m.AlPulsar = nombre, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

func (m *muestraTabla) MinSize() fyne.Size { return fyne.NewSize(ladoMuestra, ladoMuestra) }

func (m *muestraTabla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cab, _ := tokens.ParseColor(m.estilo.Cabecera)
	banda, _ := tokens.ParseColor(m.estilo.Banda)
	r := &muestraTablaRenderer{
		m: m, aro: ui.NuevoAro(ladoMuestra, ui.SinHueco, v),
		fondo: canvas.NewRectangle(banda), franja: canvas.NewRectangle(cab),
		borde: canvas.NewRectangle(color.NRGBA{}),
	}
	r.fondo.CornerRadius, r.borde.CornerRadius = RadioMuestraTabla, RadioMuestraTabla
	r.borde.StrokeWidth = 2 * tema.Borde // el trazo va centrado: el doble para 1 px por dentro
	r.objetos = r.aro.Antes(r.fondo, r.franja, r.borde)
	return r
}

// RadioMuestraTabla es el `rounded-[3px]`.
const RadioMuestraTabla float32 = 3

type muestraTablaRenderer struct {
	m                    *muestraTabla
	aro                  *ui.Aro
	fondo, franja, borde *canvas.Rectangle
	objetos              []fyne.CanvasObject
}

func (r *muestraTablaRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraTablaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *muestraTablaRenderer) Destroy()                     {}

func (r *muestraTablaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	// La franja va dentro del borde y bajo el radio: `overflow-hidden` recorta sus
	// esquinas de arriba, que el radio de 3 px apenas toca.
	r.franja.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.franja.Resize(fyne.NewSize(t.Width-2*tema.Borde, altoFranjaCab-tema.Borde))
	r.borde.Resize(t)
	r.aro.ColocarEn(fyne.NewPos(0, 0), t)
}

func (r *muestraTablaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.borde.StrokeColor = tema.Color("border", v)
	if r.m.Elegida {
		r.borde.StrokeColor = tema.Color("text", v)
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.m.Enfocado())
	r.Layout(r.m.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- minigráficos ----

// formMini es el formulario de `SparklineMenu`.
type formMini struct{ rango, tipo, color string }

func (lz *Lienzo) botonMinigrafico(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdGrafico, c.Traducir("data.sparkline.insert"), func() {
		lz.abrirMinigrafico(c, b, &formMini{rango: lz.Estado().Seleccion, tipo: "line", color: ColoresMinigrafico[0]})
	})
	b.Inerte = c.SoloLectura
	return b
}

// anfitriona es `anchorCell`: la primera celda de la selección.
func anfitriona(seleccion string) string {
	r, ok := formula.ParseRangeRef(seleccion)
	if !ok {
		return ""
	}
	return Ref(min(r.C1, Columnas-1), r.R1)
}

// abrirMinigrafico es `SparklineMenu`: los que hay (con quitar), el rango, el tipo y
// el color, y dónde se va a insertar.
func (lz *Lienzo) abrirMinigrafico(c *editor.Contexto, disparador *editor.BotonCinta, f *formMini) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirMinigrafico(c, disparador, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	host := anfitriona(lz.Estado().Seleccion)
	insertar := func() {
		if _, ok := formula.ParseRangeRef(f.rango); !ok || host == "" {
			return
		}
		b, _ := json.Marshal(map[string]any{"range": strings.ToUpper(strings.TrimSpace(f.rango)), "type": f.tipo, "color": f.color, "cell": host})
		c.Mandar(CmdMiniAnadir, string(b))
		em.Cerrar()
	}
	filas := []fyne.CanvasObject{bloque(tema.Espacio(1), ui.NuevoParrafo(c.Traducir("data.sparkline.title"), tema.TextoXS, ui.Medio, "text", v))}
	if hay := lz.Libro.Minigraficos(); len(hay) > 0 {
		lista := make([]fyne.CanvasObject, 0, len(hay))
		for _, m := range hay {
			quitar := editor.NuevoBotonCinta(iconos.EdPapelera, c.Traducir("sheet.sparkline.remove", i18n.Vars{"cell": m.Cell}),
				func() { c.Mandar(CmdMiniQuitar, m.Cell); rehacer() })
			quitar.Inerte = c.SoloLectura
			lista = append(lista, container.New(fila{hueco: tema.Espacio(1), elastico: 0},
				ui.NuevoParrafo(m.Cell+" · "+m.Range, tema.TextoXS, ui.Normal, "text", v).Recortado(1),
				container.New(ladoFijo{ladoBotonRegla}, quitar)))
		}
		linea := canvas.NewRectangle(tema.Color("border", v))
		filas = append(filas, bloque(tema.Espacio(2), container.New(bordeAbajoDe{tema.Espacio(2)},
			topeAlto(tema.Espacio(24), container.New(ui.Columna{}, lista...)), linea)))
	}
	rango := ui.NuevaEntrada("").ConPropia(campoPanel)
	rango.PonerTexto(f.rango)
	rango.AlCambiar(func(s string) { f.rango = s })
	rango.AlTeclear = alIntro(insertar)
	tipos := make([]editor.Opcion, len(TiposMinigrafico))
	for i, t := range TiposMinigrafico {
		tipos[i] = editor.Opcion{Valor: t.Valor, Rotulo: c.Traducir(t.Clave)}
	}
	tipo := editor.NuevoSelectorCinta(c.Traducir("chart.field.type"), anchoSelectorMini, tipos, f.tipo, func(s string) { f.tipo = s })
	aviso := c.Traducir("data.sparkline.pickHost")
	if host != "" {
		aviso = c.Traducir("data.sparkline.willInsert", i18n.Vars{"cell": host})
	}
	_, valido := formula.ParseRangeRef(f.rango)
	boton := botonPop(c.Traducir("action.insert"), false, insertar)
	boton.Inerte = !valido || host == "" || c.SoloLectura
	filas = append(filas,
		bloque(tema.Espacio(1.5), conEtiqueta(c.Traducir("data.sparkline.range"), rango)),
		bloque(tema.Espacio(1.5), tipo),
		bloque(tema.Espacio(2), muestras(ColoresMinigrafico, f.color, c.Traducir("palette.colorLabel"), func(hex string) { f.color = hex; rehacer() })),
		bloque(tema.Espacio(2), ui.NuevoParrafo(aviso, tema.Texto2XS, ui.Normal, "text-subtle", v)),
		anchoEntero(boton),
	)
	em = abrirEmergente(disparador, panel{ancho: anchoPanelMini, filas: filas}.objeto())
}

// bordeAbajoDe es `border-border border-b pb-*`: lo de encima y la línea a `pb`.
type bordeAbajoDe struct{ pb float32 }

func (b bordeAbajoDe) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height+b.pb+tema.Borde)
}

func (b bordeAbajoDe) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, t.Height-b.pb-tema.Borde))
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
}

// alIntro es el `onKeyDown` de Intro de los campos de los paneles.
func alIntro(f func()) func(*fyne.KeyEvent) bool {
	return func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		f()
		return true
	}
}

// ---- gráficos ----

// formGrafico es el de `ChartMenu`: rango y tipo.
type formGrafico struct{ rango, tipo string }

func (lz *Lienzo) botonGrafico(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdGrafico, c.Traducir("data.chart.insert"), func() {
		lz.abrirGrafico(c, b, &formGrafico{rango: lz.Estado().Seleccion, tipo: "bar"})
	})
	b.Inerte = c.SoloLectura
	return b
}

// abrirGrafico es `ChartMenu`: el rango (prellenado con la selección) y el tipo.
func (lz *Lienzo) abrirGrafico(c *editor.Contexto, disparador *editor.BotonCinta, f *formGrafico) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirGrafico(c, disparador, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	insertar := func() {
		if _, ok := formula.ParseRangeRef(f.rango); !ok {
			return
		}
		b, _ := json.Marshal(map[string]any{"type": f.tipo, "range": strings.TrimSpace(f.rango)})
		c.Mandar(CmdGrafico, string(b))
		em.Cerrar()
	}
	rango := ui.NuevaEntrada("").ConPropia(campoPanel)
	rango.PonerTexto(f.rango)
	rango.AlCambiar(func(s string) {
		_, antes := formula.ParseRangeRef(f.rango)
		f.rango = s
		if _, ahora := formula.ParseRangeRef(s); ahora != antes {
			rehacer()
		}
	})
	rango.AlTeclear = alIntro(insertar)
	tipos := make([]editor.Opcion, len(TiposGrafico))
	for i, t := range TiposGrafico {
		tipos[i] = editor.Opcion{Valor: t.Valor, Rotulo: c.Traducir(t.Clave)}
	}
	_, valido := formula.ParseRangeRef(f.rango)
	boton := botonPop(c.Traducir("action.insert"), false, insertar)
	boton.Inerte = !valido || c.SoloLectura
	em = abrirEmergente(disparador, panel{ancho: anchoPanelMini, filas: []fyne.CanvasObject{
		bloque(tema.Espacio(1), ui.NuevoParrafo(c.Traducir("data.chart.insert"), tema.TextoXS, ui.Medio, "text", v)),
		bloque(tema.Espacio(1.5), conEtiqueta(c.Traducir("data.chart.range"), rango)),
		bloque(tema.Espacio(2), editor.NuevoSelectorCinta(c.Traducir("chart.type"), anchoSelectorMini, tipos, f.tipo,
			func(s string) { f.tipo = s })),
		anchoEntero(boton),
	}}.objeto())
}

// ---- segmentación ----

// anchoSelectorSegmentacion es el `width={240}` del RibbonSelect del campo en `SlicerMenu`.
const anchoSelectorSegmentacion float32 = 240

// formSegmentacion es el de `SlicerMenu`: el origen (un rango con cabecera o, si no
// va vacía, esa tabla dinámica de la hoja) y el campo.
type formSegmentacion struct {
	rango    string
	dinamica string
	campo    int
}

func (lz *Lienzo) botonSegmentacion(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdSegmentacion, c.Traducir("data.slicer.insertFull"), func() {
		lz.abrirSegmentacion(c, b, &formSegmentacion{rango: lz.Estado().Seleccion})
	})
	b.Inerte = c.SoloLectura
	return b
}

// cabecerasDe son los rótulos de la primera fila de un rango (`readSheetGrid(range)[0]`),
// con «Columna N» para los vacíos. nil si el rango no se lee.
func (lz *Lienzo) cabecerasDe(c *editor.Contexto, rango string) []string {
	r, ok := formula.ParseRangeRef(rango)
	if !ok {
		return nil
	}
	var out []string
	for col := r.C1; col <= min(r.C2, Columnas-1); col++ {
		t := strings.TrimSpace(lz.Libro.Texto(col, r.R1))
		if t == "" {
			t = c.Traducir("common.columnN", i18n.Vars{"n": col - r.C1 + 1})
		}
		out = append(out, t)
	}
	return out
}

// camposDinamica son los encabezados del origen de esa tabla dinámica, con «Columna N»
// para los vacíos: todos, estén o no en la tabla (como Excel).
func (lz *Lienzo) camposDinamica(c *editor.Contexto, d analisis.Dinamica) []string {
	rejilla, _, ok := lz.Libro.origenDinamica(d)
	if !ok || len(rejilla) == 0 {
		return nil
	}
	out := make([]string, len(rejilla[0]))
	for i, v := range rejilla[0] {
		t, _ := textoConFormato(v, nil)
		if out[i] = strings.TrimSpace(t); out[i] == "" {
			out[i] = c.Traducir("common.columnN", i18n.Vars{"n": i + 1})
		}
	}
	return out
}

// abrirSegmentacion es `SlicerMenu`: el origen (un rango con cabeceras o una tabla
// dinámica, si la hoja tiene) y el campo a segmentar.
func (lz *Lienzo) abrirSegmentacion(c *editor.Contexto, disparador *editor.BotonCinta, f *formSegmentacion) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirSegmentacion(c, disparador, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	dinamicas := lz.Libro.Dinamicas()
	d, conDinamica := lz.Libro.dinamicaDe(f.dinamica)
	campos := lz.cabecerasDe(c, f.rango)
	r, valido := formula.ParseRangeRef(f.rango)
	if conDinamica {
		campos, valido = lz.camposDinamica(c, d), true
	}
	puede := valido && len(campos) > 0 && f.campo >= 0 && f.campo < len(campos)
	crear := func() {
		if !puede {
			return
		}
		pedido := map[string]any{"range": strings.ToUpper(strings.TrimSpace(f.rango)), "col": r.C1 + f.campo}
		if conDinamica {
			pedido = map[string]any{"pivot": d.Name, "col": f.campo}
		}
		b, _ := json.Marshal(pedido)
		c.Mandar(CmdSegAnadir, string(b))
		em.Cerrar()
	}
	rango := ui.NuevaEntrada("").ConPropia(campoPanel)
	rango.PonerTexto(f.rango)
	rango.AlCambiar(func(s string) {
		antes := len(lz.cabecerasDe(c, f.rango))
		f.rango = s
		if len(lz.cabecerasDe(c, s)) != antes {
			rehacer() // cambian los campos que se ofrecen
		}
	})
	rango.AlTeclear = alIntro(crear)
	filas := []fyne.CanvasObject{
		bloque(tema.Espacio(1), ui.NuevoParrafo(c.Traducir("data.slicer"), tema.TextoXS, ui.Medio, "text", v)),
	}
	if len(dinamicas) > 0 {
		opts := make([]editor.Opcion, 0, 1+len(dinamicas))
		opts = append(opts, editor.Opcion{Valor: "", Rotulo: c.Traducir("data.rangeWithHeaders")})
		for _, x := range dinamicas {
			opts = append(opts, editor.Opcion{Valor: x.Name, Rotulo: x.Name})
		}
		origen := editor.NuevoSelectorCinta(c.Traducir("query.source"), anchoSelectorSegmentacion, opts, f.dinamica,
			func(s string) {
				f.dinamica, f.campo = s, 0
				if x, ok := lz.Libro.dinamicaDe(s); ok {
					if cs := slices.Concat(x.Filters, x.Rows, x.Cols); len(cs) > 0 {
						f.campo = cs[0].Field
					}
				}
				rehacer()
			})
		filas = append(filas, bloque(tema.Espacio(1.5), conEtiqueta(c.Traducir("query.source"), origen)))
	}
	if !conDinamica {
		filas = append(filas, bloque(tema.Espacio(1.5), conEtiqueta(c.Traducir("data.rangeWithHeaders"), rango)))
	}
	if len(campos) == 0 {
		filas = append(filas, bloque(tema.Espacio(2), ui.NuevoParrafo(c.Traducir("data.slicer.pickField"), tema.Texto2XS, ui.Normal, "text-subtle", v)))
	} else {
		opts := make([]editor.Opcion, len(campos))
		for i, n := range campos {
			opts[i] = editor.Opcion{Valor: strconv.Itoa(i), Rotulo: n}
		}
		sel := editor.NuevoSelectorCinta(c.Traducir("common.field"), anchoSelectorSegmentacion, opts, strconv.Itoa(f.campo),
			func(s string) { f.campo, _ = strconv.Atoi(s); rehacer() })
		filas = append(filas, bloque(tema.Espacio(2), conEtiqueta(c.Traducir("common.field"), sel)))
	}
	boton := botonPop(c.Traducir("action.insert"), false, crear)
	boton.Inerte = !puede || c.SoloLectura
	filas = append(filas,
		bloque(tema.Espacio(2), ui.NuevoParrafo(c.Traducir("data.slicer.explain"), tema.Texto2XS, ui.Normal, "text-subtle", v)),
		anchoEntero(boton))
	em = abrirEmergente(disparador, panel{ancho: anchoPanelTabla, filas: filas}.objeto())
}
