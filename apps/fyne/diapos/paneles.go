// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Los paneles de debajo del escenario
//
// SlidesCanvas.tsx tiene un único hueco bajo el escenario (`bottom`) que enseña, uno
// cada vez, las notas del ponente, la secuencia de animaciones, los datos del gráfico
// elegido o los elementos del diagrama elegido. Las notas viven en `editor.go`; aquí
// van los otros tres, que comparten armazón: `border-t bg-surface`, una cabecera con
// el título en `text-xs font-semibold` y la pista en `sytue-meta-dense` (`px-3
// pt-1.5`), un cuerpo que se desplaza y, en dos de ellos, un pie con botones.
//
// Los paneles con campos de texto se rehacen solo cuando cambia su FORMA (otra caja,
// otra fila, otro elemento): rehacerlos con cada letra quitaría el foco al campo.

// Medidas comunes de los paneles (con el rem de 14).
var (
	// Los topes de alto: `max-h-44` el del gráfico, `max-h-52` los otros dos.
	altoMaxGrafico = tema.Espacio(44)
	altoMaxPanel   = tema.Espacio(52)
	// La celda de la tabla: `h-6 min-h-[24px] w-20 px-1 text-xs`, con `p-0.5` alrededor.
	anchoCelda    = tema.Espacio(20)
	altoCelda     = float32(24)
	margenCelda   = tema.Espacio(0.5)
	botonPanelXS  = float32(24)     // `h-6 min-h-[24px]` de los botones de pie
	ladoBotonFila = tema.Espacio(6) // `h-6 w-6` de los botones de una fila (21)
)

// campoCelda son las medidas del `<input>` de una celda o de un elemento, escrito a
// mano en la web con `border-border rounded-sm border text-xs`.
func campoCelda(alto, margenX float32) ui.CampoPropio {
	return ui.CampoPropio{
		MargenX: margenX, MargenY: (alto - 2*tema.Borde - tema.TextoXS.Linea) / 2,
		Texto: tema.TextoXS, Borde: "border",
	}
}

// cambiarLigero es `cambiar` para lo que se TECLEA en un panel: el escenario y las
// miniaturas se ponen al día, pero el panel no se rehace y el campo no pierde el foco.
func (e *Editor) cambiarLigero(f func()) {
	e.cambiar(f, false)
	if e.ctx.SoloLectura {
		return
	}
	e.escenario.Refresh()
	e.capa.Refresh()
	e.carril.rehacer()
}

// panelAbajo devuelve el panel que toca, reutilizando el anterior si su forma no ha
// cambiado.
func (e *Editor) panelAbajo() fyne.CanvasObject {
	var clave string
	var hacer func() fyne.CanvasObject
	switch e.abajo {
	case abajoNotas:
		return e.notas
	case abajoGrafico:
		clave, hacer = e.claveGrafico(), e.panelGrafico
	case abajoSmart:
		clave, hacer = e.claveSmart(), e.panelSmart
	case abajoAnim:
		return e.panelAnim() // sin campos de texto: se rehace siempre
	default:
		return nil
	}
	if clave != e.clavePanel || e.panel == nil {
		e.clavePanel, e.panel = clave, hacer()
	}
	return e.panel
}

// ---- el armazón ----

// armazonPanel apila la línea de arriba, la cabecera, el cuerpo y el pie opcional.
type armazonPanel struct {
	tope float32
	pie  bool
}

func (a armazonPanel) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	h := float32(0)
	for _, o := range objs[2:] {
		if d, ok := o.(*container.Scroll); ok {
			h += d.Content.MinSize().Height
			continue
		}
		if ancho > 0 {
			h += ui.AltoA(o, ancho)
		} else {
			h += o.MinSize().Height
		}
	}
	return min(h+tema.Borde, a.tope)
}

func (a armazonPanel) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, a.AltoCon(objs, 0))
}

// Layout: objs = fondo, línea, cabecera, cuerpo[, pie]. El cuerpo se queda lo que sobre.
func (a armazonPanel) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	y := tema.Borde
	cab := ui.AltoA(objs[2], t.Width)
	objs[2].Move(fyne.NewPos(0, y))
	objs[2].Resize(fyne.NewSize(t.Width, cab))
	y += cab
	pie := float32(0)
	if a.pie {
		pie = objs[4].MinSize().Height
		objs[4].Move(fyne.NewPos(0, t.Height-pie))
		objs[4].Resize(fyne.NewSize(t.Width, pie))
	}
	objs[3].Move(fyne.NewPos(0, y))
	objs[3].Resize(fyne.NewSize(t.Width, max(t.Height-y-pie, 0)))
}

func (e *Editor) armazon(tope float32, titulo, pista string, cuerpo, pie fyne.CanvasObject) fyne.CanvasObject {
	v := e.variante()
	objs := []fyne.CanvasObject{
		canvas.NewRectangle(tema.Color("surface", v)), canvas.NewRectangle(tema.Color("border", v)),
		nuevaCabeceraPanel(titulo, pista, tema.Espacio(1.5), v), cuerpo,
	}
	if pie != nil {
		objs = append(objs, pie)
	}
	return container.New(armazonPanel{tope: tope, pie: pie != nil}, objs...)
}

// vacioPanel es `border-t px-3 py-3` con la frase `sytue-meta` de «elige uno».
func (e *Editor) vacioPanel(frase string) fyne.CanvasObject {
	v := e.variante()
	return container.New(&rellenoCaja{arriba: tema.Borde + tema.Espacio(3), abajo: tema.Espacio(3), lados: tema.Espacio(3)},
		canvas.NewRectangle(tema.Color("surface", v)), canvas.NewRectangle(tema.Color("border", v)),
		ui.NuevoParrafo(frase, tema.TextoXS, ui.Normal, "text-subtle", v))
}

// rellenoCaja pone un contenido con su relleno encima de un fondo y una línea arriba.
type rellenoCaja struct{ arriba, abajo, lados float32 }

func (r *rellenoCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, r.arriba+objs[2].MinSize().Height+r.abajo)
}

func (r *rellenoCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	objs[2].Move(fyne.NewPos(r.lados, r.arriba))
	objs[2].Resize(fyne.NewSize(t.Width-2*r.lados, objs[2].MinSize().Height))
}

// La pista queda a la derecha mientras cabe junto al título. En un teléfono baja
// de fila y se reparte: alinearla solo por su borde derecho la pintaba encima del
// título. El alto al ancho actual lo propagan Margen y FilaAuto al armazón.
func nuevaCabeceraPanel(titulo, pista string, arriba float32, v tema.Variante) *fyne.Container {
	t := ui.NuevoParrafo(titulo, tema.TextoXS, ui.SemiNegrita, "text", v).
		A(ui.Medir(titulo, tema.TextoXS, ui.SemiNegrita).Width)
	p := ui.NuevoParrafo(pista, tema.Texto2XS, ui.Normal, "text-subtle", v).
		A(ui.Medir(pista, tema.Texto2XS, ui.Normal).Width)
	fila := container.New(ui.FilaAuto{
		Hueco: tema.Espacio(3), Derecha: 1, Partir: true, SaltoFila: tema.Espacio(1),
	}, t, p)
	return container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: arriba}, fila)
}

// filaPie es `flex items-center gap-1 border-t px-3 py-1`, con el divisor `mx-1 h-4
// w-px` donde vaya.
type filaPie struct{ divisor fyne.CanvasObject }

func (filaPie) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, tema.Borde+2*tema.Espacio(1)+botonPanelXS)
}

func (f filaPie) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	x := tema.Espacio(3)
	for _, o := range objs[2:] {
		m := o.MinSize()
		if o == f.divisor {
			x += tema.Espacio(1)
			o.Move(fyne.NewPos(x, (t.Height+tema.Borde-tema.Espacio(4))/2))
			o.Resize(fyne.NewSize(tema.Borde, tema.Espacio(4)))
			x += tema.Borde + tema.Espacio(1) + tema.Espacio(1)
			continue
		}
		o.Move(fyne.NewPos(x, (t.Height+tema.Borde-m.Height)/2))
		o.Resize(m)
		x += m.Width + tema.Espacio(1)
	}
}

func (e *Editor) pie(botones ...fyne.CanvasObject) fyne.CanvasObject {
	v := e.variante()
	objs := make([]fyne.CanvasObject, 0, 2+len(botones))
	objs = append(objs, canvas.NewRectangle(tema.Color("surface", v)), canvas.NewRectangle(tema.Color("border", v)))
	var divisor fyne.CanvasObject
	for _, b := range botones {
		if b == nil { // el divisor: `mx-1 h-4 w-px bg-border`
			divisor = canvas.NewRectangle(tema.Color("border", v))
			b = divisor
		}
		objs = append(objs, b)
	}
	return container.New(filaPie{divisor: divisor}, objs...)
}

// ---- el gráfico ----

func (e *Editor) graficoElegido() *Caja {
	if p := e.Primaria(); p != nil && p.Tipo == "chart" && p.Grafico != nil {
		return p
	}
	return nil
}

func (e *Editor) claveGrafico() string {
	p := e.graficoElegido()
	if p == nil {
		return "chart:"
	}
	return "chart:" + p.ID + ":" + strconv.Itoa(len(p.Grafico.Rejilla)) + "x" +
		strconv.Itoa(columnasRejilla(p.Grafico)) + ":" + strconv.FormatBool(e.ctx.SoloLectura)
}

// panelGrafico es el panel «Datos del gráfico»: la rejilla editable, celda a celda, y
// el pie para añadir y quitar filas y columnas.
func (e *Editor) panelGrafico() fyne.CanvasObject {
	p := e.graficoElegido()
	if p == nil {
		return e.vacioPanel(e.t("slides.chart.pick"))
	}
	id, ro := p.ID, e.ctx.SoloLectura
	cols := columnasRejilla(p.Grafico)
	var celdas []fyne.CanvasObject
	for f, fila := range p.Grafico.Rejilla {
		for c := range cols {
			valor := ""
			if c < len(fila) {
				valor = fila[c]
			}
			campo := ui.NuevaEntrada("").ConPropia(campoCelda(altoCelda, tema.Espacio(1)))
			campo.Ancho, campo.Radio = anchoCelda, tema.RadioSM
			campo.PonerTexto(valor)
			campo.SoloLectura(ro)
			// ponytail: la primera fila y la primera columna van en `font-medium` en la
			// web; el editor de la entrada no cambia de peso.
			ff, cc := f, c
			campo.AlCambiar(func(s string) { e.cambiarLigero(func() { e.m.PonerCelda(e.objetivo(), id, ff, cc, s) }) })
			celdas = append(celdas, campo)
		}
	}
	tabla := container.New(&rejillaCeldas{cols: cols}, celdas...)
	cuerpo := container.NewScroll(container.New(&rellenoCuerpo{
		arriba: tema.Espacio(1), abajo: tema.Espacio(2), lados: tema.Espacio(3),
	}, tabla))
	filas := len(p.Grafico.Rejilla)
	boton := func(i iconos.Ed, texto, rotulo string, inerte bool, f func()) *botonPanel {
		b := nuevoBotonPanel(i, texto, e.t(rotulo), func() { e.cambiar(f, true) })
		b.Inerte = inerte || ro
		return b
	}
	pie := e.pie(
		boton(iconos.EdMas, e.t("common.row"), "slides.chart.addRow", false, func() { e.m.AñadirFila(e.objetivo(), id) }),
		boton(iconos.EdMas, e.t("common.column"), "slides.chart.addCol", false, func() { e.m.AñadirColumna(e.objetivo(), id) }),
		nil,
		boton(iconos.EdMenos, e.t("common.row"), "slides.chart.removeRow", filas <= 2, func() { e.m.QuitarFila(e.objetivo(), id) }),
		boton(iconos.EdMenos, e.t("common.column"), "slides.chart.removeCol", cols <= 2, func() { e.m.QuitarColumna(e.objetivo(), id) }),
	)
	return e.armazon(altoMaxGrafico, e.t("slides.chartData"), e.t("slides.chart.hint"), cuerpo, pie)
}

// rejillaCeldas es la `<table class="border-collapse">` con `p-0.5` en cada celda.
type rejillaCeldas struct{ cols int }

func (r *rejillaCeldas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	filas := (len(objs) + r.cols - 1) / max(r.cols, 1)
	return fyne.NewSize(float32(r.cols)*(anchoCelda+2*margenCelda), float32(filas)*(altoCelda+2*margenCelda))
}

func (r *rejillaCeldas) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	for i, o := range objs {
		f, c := i/r.cols, i%r.cols
		o.Move(fyne.NewPos(float32(c)*(anchoCelda+2*margenCelda)+margenCelda, float32(f)*(altoCelda+2*margenCelda)+margenCelda))
		o.Resize(fyne.NewSize(anchoCelda, altoCelda))
	}
}

// rellenoCuerpo es el `px-* pt-* pb-*` alrededor de un único hijo, a su medida.
type rellenoCuerpo struct{ arriba, abajo, lados float32 }

func (r *rellenoCuerpo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*r.lados, m.Height+r.arriba+r.abajo)
}

func (r *rellenoCuerpo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(r.lados, r.arriba))
	objs[0].Resize(fyne.NewSize(max(t.Width-2*r.lados, objs[0].MinSize().Width), objs[0].MinSize().Height))
}

// ---- el diagrama ----

func (e *Editor) smartElegido() *Caja {
	if p := e.Primaria(); p != nil && p.Tipo == "smartart" && p.Diagrama != nil {
		return p
	}
	return nil
}

func (e *Editor) claveSmart() string {
	p := e.smartElegido()
	if p == nil {
		return "smart:"
	}
	var b strings.Builder
	b.WriteString("smart:" + p.ID + ":" + p.Diagrama.Disposicion + ":" + strconv.FormatBool(e.ctx.SoloLectura))
	for _, el := range p.Diagrama.Elementos {
		b.WriteString(":" + strconv.FormatFloat(nivelDe(el), 'f', -1, 64))
	}
	return b.String()
}

// panelSmart es el panel «Elementos del diagrama»: un campo por elemento con sus
// botones de sangrar (solo en la jerarquía), subir, bajar y quitar, y «+ Elemento».
func (e *Editor) panelSmart() fyne.CanvasObject {
	p := e.smartElegido()
	if p == nil {
		return e.vacioPanel(e.t("slides.smartart.pick"))
	}
	id, ro := p.ID, e.ctx.SoloLectura
	d := p.Diagrama
	jerarquia := d.Disposicion == "hierarchy"
	n := len(d.Elementos)
	var filas []fyne.CanvasObject
	for i, el := range d.Elementos {
		boton := func(ic iconos.Ed, rotulo string, inerte bool, f func()) *botonPanel {
			b := nuevoBotonPanel(ic, "", e.ctx.Traducir(rotulo, i18n.Vars{"n": i + 1}), func() { e.cambiar(f, true) })
			b.Inerte = inerte || ro
			return b
		}
		antes := make([]fyne.CanvasObject, 0, 2)
		sangria := float32(0)
		if jerarquia {
			nivel := nivelDe(el)
			maximo := 0.0
			if i > 0 {
				maximo = nivelDe(d.Elementos[i-1]) + 1
			}
			antes = append(antes,
				boton(iconos.EdChevronIzq, "item.outdent", i == 0 || nivel == 0, func() { e.m.SangrarElemento(e.objetivo(), id, i, -1) }),
				boton(iconos.EdChevronDer, "item.indent", i == 0 || nivel >= maximo, func() { e.m.SangrarElemento(e.objetivo(), id, i, 1) }),
			)
			sangria = float32(nivel) * 14 // `marginLeft: level * 14px`
		}
		campo := ui.NuevaEntrada(e.t("slides.smartart.itemText")).ConPropia(campoCelda(tema.Espacio(7), tema.Espacio(1.5)))
		campo.Radio = tema.RadioSM
		campo.PonerTexto(el.Texto)
		campo.SoloLectura(ro)
		k := i
		campo.AlCambiar(func(s string) { e.cambiarLigero(func() { e.m.PonerTextoElemento(e.objetivo(), id, k, s) }) })
		despues := []fyne.CanvasObject{
			boton(iconos.EdChevronArriba, "item.moveUp", i == 0, func() { e.m.MoverElemento(e.objetivo(), id, i, -1) }),
			boton(iconos.EdChevronAbajo, "item.moveDown", i == n-1, func() { e.m.MoverElemento(e.objetivo(), id, i, 1) }),
			boton(iconos.EdCerrar, "item.remove", n <= 1, func() { e.m.QuitarElemento(e.objetivo(), id, i) }),
		}
		filas = append(filas, container.New(&filaElemento{antes: len(antes), sangria: sangria},
			append(append(antes, campo), despues...)...))
	}
	lista := container.NewScroll(container.New(&rellenoCuerpo{
		arriba: tema.Espacio(1), abajo: tema.Espacio(1), lados: tema.Espacio(2),
	}, container.New(&columnaFilas{}, filas...)))
	mas := nuevoBotonPanel(iconos.EdMas, e.t("slides.smartart.item"), e.t("slides.smartart.addItem"),
		func() { e.cambiar(func() { e.m.AñadirElemento(e.objetivo(), id) }, true) })
	mas.Inerte = ro
	pista := "smartart.hint.items"
	if jerarquia {
		pista = "smartart.hint.hierarchy"
	}
	return e.armazon(altoMaxPanel, e.t("slides.smartart.items"), e.t(pista), lista, e.pie(mas))
}

// filaElemento es `flex items-center gap-1 py-0.5`: los botones de antes, el campo
// (`flex-1`, con su sangría) y los de después.
type filaElemento struct {
	antes   int
	sangria float32
}

func (f *filaElemento) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, tema.Espacio(7)+2*tema.Espacio(0.5))
}

func (f *filaElemento) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	gap := tema.Espacio(1)
	fijos := float32(0)
	for i, o := range objs {
		if i != f.antes {
			fijos += o.MinSize().Width + gap
		}
	}
	x := float32(0)
	for i, o := range objs {
		m := o.MinSize()
		if i == f.antes {
			x += f.sangria
			m = fyne.NewSize(max(t.Width-fijos-f.sangria, 0), m.Height)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + gap
	}
}

// columnaFilas apila filas a todo el ancho.
type columnaFilas struct{}

func (columnaFilas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h, w := float32(0), float32(0)
	for _, o := range objs {
		m := o.MinSize()
		h += m.Height
		w = max(w, m.Width)
	}
	return fyne.NewSize(w, h)
}

func (columnaFilas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := float32(0)
	for _, o := range objs {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h
	}
}

// ---- las animaciones ----

// rotuloCaja es `boxLabel`: el nombre de una caja en el panel de animaciones y en el
// `aria-label` de su marco.
func (e *Editor) rotuloCaja(c Caja) string {
	if c.Tipo == "shape" {
		for _, f := range formasUI {
			if f.ID == c.Forma {
				return e.t(f.Clave)
			}
		}
		return e.t("slides.box.shape")
	}
	if !esTexto(c) {
		return e.t(claveDe(c))
	}
	t := strings.Join(strings.Fields(htmltext.Text(c.HTML)), " ")
	switch r := []rune(t); {
	case len(r) == 0:
		return e.t("slides.textBox")
	case len(r) > 30:
		return string(r[:29]) + "…"
	}
	return t
}

// clavesEfecto son ANIM_EFFECT_LABEL en el orden de ANIM_CATEGORY.
var clavesEfecto = []struct{ Tipo, Clave string }{
	{"appear", "slides.effect.appear"},
	{"fade", "slides.effect.fade"},
	{"fly", "slides.effect.fly"},
	{"zoom", "slides.effect.zoom"},
	{"pulse", "slides.effect.pulse"},
	{"spin", "slides.effect.spin"},
	{"grow-shrink", "slides.effect.growShrink"},
	{"disappear", "slides.effect.disappear"},
	{"fade-out", "slides.effect.fadeOut"},
	{"fly-out", "slides.effect.flyOut"},
	{"zoom-out", "slides.effect.zoomOut"},
	{"motion-right", "slides.effect.motionRight"},
	{"motion-left", "slides.effect.motionLeft"},
	{"motion-up", "slides.effect.motionUp"},
	{"motion-down", "slides.effect.motionDown"},
	{"motion-up-right", "slides.effect.motionUpRight"},
	{"motion-up-left", "slides.effect.motionUpLeft"},
	{"motion-down-right", "slides.effect.motionDownRight"},
	{"motion-down-left", "slides.effect.motionDownLeft"},
	{"motion-loop", "slides.effect.motionLoop"},
	{"motion-arc", "slides.effect.motionArc"},
	{"motion-custom", "slides.effect.motionCustom"},
}

// clavesCategoria son ANIM_CATEGORY_LABEL, en su orden.
var clavesCategoria = []string{
	"slides.category.entrance", "slides.category.emphasis", "slides.category.exit", "slides.category.motion",
}

func (e *Editor) efectosDe(cat Categoria) []editor.Opcion {
	var ops []editor.Opcion
	for _, ef := range clavesEfecto {
		if categoriaAnim[ef.Tipo] == cat {
			ops = append(ops, editor.Opcion{Valor: ef.Tipo, Rotulo: e.t(ef.Clave)})
		}
	}
	return ops
}

// Medidas de los `<select>` del panel: `h-6 rounded-sm border px-1 text-xs`, el del
// efecto `w-28` y el del disparador `w-32`.
var (
	altoSelectPanel    = tema.Espacio(6)
	anchoSelectEfecto  = tema.Espacio(28)
	anchoSelectDisparo = tema.Espacio(32)
)

// selectorPanel es un `<select>` del panel con el aspecto de RibbonSelect a 21 de alto.
//
// ponytail: la web los pinta como `<select>` del sistema, con `px-1` y la flecha del
// navegador; aquí van con la caja y el chevrón del selector de la cinta.
func (e *Editor) selectorPanel(rotulo string, ancho float32, ops []editor.Opcion, valor string, inerte bool, f func(string)) *editor.SelectorCinta {
	s := editor.NuevoSelectorCinta(rotulo, ancho, ops, valor, f)
	s.Alto, s.Inerte = altoSelectPanel, inerte || e.ctx.SoloLectura
	return s
}

// panelAnim es el panel «Animaciones»: añadir un efecto a la caja elegida (categoría y
// efecto) y la secuencia de la diapositiva, paso a paso, con su efecto, su disparador
// y los botones de subir, bajar y quitar.
func (e *Editor) panelAnim() fyne.CanvasObject {
	v := e.variante()
	if e.activa < 0 || e.activa >= len(e.m.Diapos) {
		return nil
	}
	d := e.m.Diapos[e.activa]
	var partes []fyne.CanvasObject
	if p := e.Primaria(); p != nil {
		cats := make([]editor.Opcion, len(clavesCategoria))
		for i, k := range clavesCategoria {
			cats[i] = editor.Opcion{Valor: strconv.Itoa(i), Rotulo: e.t(k)}
		}
		id := p.ID
		añadir := append([]editor.Opcion{{Valor: "", Rotulo: e.t("slides.effect.add")}}, e.efectosDe(e.catAnim)...)
		partes = append(partes, container.New(&filaAñadir{},
			ui.NuevoRotulo(e.ctx.Traducir("slides.anim.addTo", i18n.Vars{"label": e.rotuloCaja(*p)}), tema.Texto2XS, ui.Normal, "text-muted"),
			e.selectorPanel(e.t("slides.effect.categoryAdd"), anchoNativo(cats), cats, strconv.Itoa(int(e.catAnim)), false,
				func(s string) {
					if n, err := strconv.Atoi(s); err == nil && n >= 0 && n < len(clavesCategoria) {
						e.catAnim = Categoria(n) //nolint:gosec // G115: acotado a las cuatro categorías
						e.recomponer()
					}
				}),
			e.selectorPanel(e.t("slides.effect.toAdd"), anchoNativo(añadir), añadir, "", false,
				func(s string) {
					if s != "" {
						e.cambiar(func() { e.m.AñadirAnim(e.activa, id, s) }, true)
					}
				}),
		))
	} else {
		partes = append(partes, container.New(&rellenoCuerpo{arriba: tema.Espacio(1), abajo: tema.Espacio(1), lados: tema.Espacio(3)},
			ui.NuevoRotulo(e.t("slides.anim.pickBox"), tema.Texto2XS, ui.Normal, "text-subtle")))
	}
	plan := Plan(d)
	if len(plan) == 0 {
		partes = append(partes, container.New(&rellenoCuerpo{arriba: tema.Espacio(2), abajo: tema.Espacio(2), lados: tema.Espacio(3)},
			ui.NuevoRotulo(e.t("slides.anim.empty"), tema.TextoXS, ui.Normal, "text-subtle")))
	} else {
		cajas := map[string]Caja{}
		for _, c := range d.Cajas {
			cajas[c.ID] = c
		}
		var filas []fyne.CanvasObject
		for i, pl := range plan {
			filas = append(filas, e.filaAnim(i, len(plan), pl, cajas[pl.Caja], v))
		}
		partes = append(partes, container.NewScroll(container.New(&rellenoCuerpo{
			arriba: tema.Espacio(1), abajo: tema.Espacio(1), lados: tema.Espacio(2),
		}, container.New(&columnaFilas{}, filas...))))
	}
	cuerpo := partes[len(partes)-1]
	if len(partes) == 2 {
		cuerpo = container.New(&cuerpoAnim{}, partes...)
	}
	return e.armazon(altoMaxPanel, e.t("slides.animations"), e.t("slides.anim.legend"), cuerpo, nil)
}

// anchoNativo es el ancho de un `<select>` sin `w-*`: el de su opción más larga más la
// flecha, los `px-1` y el borde. Medido en el arnés con el español: 82 el de categoría
// («Movimiento») y 100 el de añadir («Añadir efecto…»); los dos son su rótulo + 32.
func anchoNativo(ops []editor.Opcion) float32 {
	var m float32
	for _, o := range ops {
		m = max(m, ui.Medir(o.Rotulo, tema.TextoXS, ui.Normal).Width)
	}
	return float32(math.Round(float64(m))) + 32
}

// cuerpoAnim pone la fila de añadir arriba, a su alto, y la secuencia con el resto.
type cuerpoAnim struct{}

func (cuerpoAnim) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := objs[0].MinSize().Height
	if d, ok := objs[1].(*container.Scroll); ok {
		h += d.Content.MinSize().Height
	} else {
		h += objs[1].MinSize().Height
	}
	return fyne.NewSize(0, h)
}

func (cuerpoAnim) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	h := objs[0].MinSize().Height
	objs[0].Resize(fyne.NewSize(t.Width, h))
	objs[1].Move(fyne.NewPos(0, h))
	objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-h, 0)))
}

// filaAñadir es `flex flex-wrap items-center gap-1.5 px-3 py-1`.
type filaAñadir struct{}

func (filaAñadir) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, altoSelectPanel+2*tema.Espacio(1))
}

func (filaAñadir) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := tema.Espacio(3)
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + tema.Espacio(1.5)
	}
}

// filaAnim es un `<li>` de la secuencia: `flex items-center gap-1.5 rounded-sm px-1
// py-0.5`, con el paso, el nombre (que elige la caja), la categoría, el efecto, el
// disparador y los tres botones.
func (e *Editor) filaAnim(i, n int, pl Planeada, c Caja, v tema.Variante) fyne.CanvasObject {
	id, k := pl.Caja, pl.Indice
	rotulo := e.rotuloCaja(c)
	if c.ID == "" {
		rotulo = "Caja"
	}
	nombre := ui.NuevoEnlace(rotulo, tema.TextoXS, ui.Normal, "text", func() { e.SeleccionarSolo(id) })
	if e.sel == id {
		nombre = ui.NuevoEnlace(rotulo, tema.TextoXS, ui.Medio, "text", func() { e.SeleccionarSolo(id) })
	}
	disparo := pl.Anim.Disparo
	if disparo == "" {
		disparo = "click"
	}
	disparos := []editor.Opcion{
		{Valor: "click", Rotulo: e.t("slides.anim.onClickTrigger")},
		{Valor: "with", Rotulo: e.t("slides.anim.withPrev")},
		{Valor: "after", Rotulo: e.t("slides.anim.afterPrev")},
	}
	boton := func(ic iconos.Ed, clave string, inerte bool, f func()) *botonPanel {
		b := nuevoBotonPanel(ic, "", e.ctx.Traducir(clave, i18n.Vars{"label": rotulo}), func() { e.cambiar(f, true) })
		b.Inerte = inerte || e.ctx.SoloLectura
		return b
	}
	orden := pl.Anim.Orden
	objs := []fyne.CanvasObject{
		nuevoPasoAnim(pl.Paso, tema.Color("primary", v)),
		nombre,
		nuevaEtiquetaCategoria(e.t(clavesCategoria[pl.Categoria]), v),
		e.selectorPanel(e.ctx.Traducir("slides.anim.effectOf", i18n.Vars{"label": rotulo}), anchoSelectEfecto,
			e.efectosDe(pl.Categoria), pl.Anim.Tipo, false,
			func(s string) { e.cambiar(func() { e.m.PonerEfecto(e.activa, id, k, s) }, true) }),
		e.selectorPanel(e.ctx.Traducir("slides.anim.triggerOf", i18n.Vars{"label": rotulo}), anchoSelectDisparo,
			disparos, disparo, i == 0,
			func(s string) { e.cambiar(func() { e.m.PonerDisparo(e.activa, id, k, s) }, true) }),
		boton(iconos.EdChevronArriba, "slides.anim.moveUp", i == 0, func() { e.m.MoverAnim(e.activa, orden, -1) }),
		boton(iconos.EdChevronAbajo, "slides.anim.moveDown", i == n-1, func() { e.m.MoverAnim(e.activa, orden, 1) }),
		boton(iconos.EdCerrar, "slides.anim.remove", false, func() { e.m.QuitarAnim(e.activa, id, k) }),
	}
	return container.New(&filaSecuencia{}, objs...)
}

// filaSecuencia coloca la fila: el nombre es el `flex-1` (el segundo hijo).
type filaSecuencia struct{}

func (filaSecuencia) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, altoSelectPanel+2*tema.Espacio(0.5))
}

func (filaSecuencia) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	gap, x := tema.Espacio(1.5), tema.Espacio(1)
	fijos := x * 2
	for i, o := range objs {
		if i != 1 {
			fijos += o.MinSize().Width + gap
		}
	}
	for i, o := range objs {
		m := o.MinSize()
		if i == 1 {
			m.Width = max(t.Width-fijos, 0)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + gap
	}
}

// pasoAnim es el círculo del paso: `grid h-5 w-5 rounded-full text-2xs font-semibold
// text-white` sobre el acento.
func nuevoPasoAnim(paso int, acento color.NRGBA) fyne.CanvasObject {
	lado := tema.Espacio(5)
	fondo := canvas.NewCircle(acento)
	t := ui.Texto(strconv.Itoa(paso), tema.Texto2XS, ui.SemiNegrita, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	return container.New(&centrado{lado: lado}, fondo, t)
}

// centrado es un cuadrado de `lado` con el primer hijo a todo el tamaño y el texto en
// el centro.
type centrado struct{ lado float32 }

func (c *centrado) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(c.lado, c.lado) }

func (c *centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	if tx, ok := objs[1].(*canvas.Text); ok {
		m := ui.Medir(tx.Text, tema.Texto2XS, ui.SemiNegrita)
		ui.ColocarTexto(tx, tema.Texto2XS, (t.Width-m.Width)/2, (t.Height-tema.Texto2XS.Linea)/2)
	}
}

// nuevaEtiquetaCategoria es `border-border text-text-muted text-2xs rounded-sm border
// px-1 leading-4`.
func nuevaEtiquetaCategoria(texto string, v tema.Variante) fyne.CanvasObject {
	e := tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: 14}
	// El relleno es el del panel: `ui.Caja` pinta el borde como una caja entera debajo,
	// y un relleno transparente la dejaría ver.
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM)
	t := ui.Texto(texto, e, ui.Normal, tema.Color("text-muted", v))
	ancho := ui.Medir(texto, e, ui.Normal).Width + 2*tema.Espacio(1) + 2*tema.Borde
	return container.New(&etiquetaLayout{ancho: ancho, e: e}, append(caja.Objetos(), t)...)
}

type etiquetaLayout struct {
	ancho float32
	e     tema.Escalon
}

func (l *etiquetaLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.ancho, l.e.Linea+2*tema.Borde)
}

func (l *etiquetaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	n := len(objs) - 1
	objs[0].Resize(t)
	if n == 2 {
		objs[1].Move(fyne.NewPos(tema.Borde, tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
	}
	if tx, ok := objs[n].(*canvas.Text); ok {
		ui.ColocarTexto(tx, l.e, tema.Borde+tema.Espacio(1), tema.Borde)
	}
}

// ---- el botón de los paneles ----

// botonPanel es el botón de un panel: `text-text-muted hover:bg-bg-subtle rounded-sm`,
// con texto (`h-6 min-h-[24px] gap-1 px-2 text-xs`, icono `sm`) o solo icono (`h-6 w-6`,
// icono `md`), y `disabled:opacity-30`.
//
// Contorno es la variante de «Cerrar patrón»: `h-6 border border-border bg-surface
// text-text hover:bg-bg-subtle`, sin el `min-h-[24px]` (21 de alto).
type botonPanel struct {
	widget.BaseWidget
	Icono    iconos.Ed
	Texto    string
	Rotulo   string
	AlPulsar func()
	Inerte   bool
	Contorno bool
	encima   bool
	enfocado bool
}

var (
	_ fyne.Tappable     = (*botonPanel)(nil)
	_ fyne.Focusable    = (*botonPanel)(nil)
	_ desktop.Hoverable = (*botonPanel)(nil)
)

func nuevoBotonPanel(i iconos.Ed, texto, rotulo string, alPulsar func()) *botonPanel {
	b := &botonPanel{Icono: i, Texto: texto, Rotulo: rotulo, AlPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonPanel) lado() float32 {
	if b.Texto == "" {
		return 16 // `size="md"`
	}
	return 14 // `size="sm"`
}

// MinSize: con texto, `px-2`, el icono, `gap-1` y el rótulo a 24 de alto; solo icono,
// el `h-6 w-6` (21).
func (b *botonPanel) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	if b.Texto == "" {
		return fyne.NewSize(ladoBotonFila, ladoBotonFila)
	}
	ancho := 2*tema.Espacio(2) + b.lado() + tema.Espacio(1) + ui.Medir(b.Texto, tema.TextoXS, ui.Normal).Width
	if b.Contorno {
		return fyne.NewSize(ancho+2*tema.Borde, tema.Espacio(6))
	}
	return fyne.NewSize(ancho, botonPanelXS)
}

func (b *botonPanel) Tapped(*fyne.PointEvent) {
	if !b.Inerte && b.AlPulsar != nil {
		b.AlPulsar()
	}
}

func (b *botonPanel) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonPanel) MouseMoved(*desktop.MouseEvent) {}
func (b *botonPanel) MouseOut()                      { b.encima = false; b.Refresh() }
func (b *botonPanel) FocusGained()                   { b.enfocado = true; b.Refresh() }
func (b *botonPanel) FocusLost()                     { b.enfocado = false; b.Refresh() }
func (b *botonPanel) TypedRune(r rune) {
	if r == ' ' {
		b.Tapped(nil)
	}
}

func (b *botonPanel) TypedKey(ev *fyne.KeyEvent) {
	if ui.SubirEscape(b, ev) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if ev.Name == fyne.KeyReturn || ev.Name == fyne.KeyEnter {
		b.Tapped(nil)
	}
}

func (b *botonPanel) AccessibilityLabel() string             { return b.Rotulo }
func (b *botonPanel) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (b *botonPanel) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonPanelRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		caja:  ui.NuevaCaja(color.NRGBA{}, bordeContorno(b.Contorno, v), tema.RadioSM),
		glifo: iconos.TrazoEd(b.Icono, b.lado(), "text-muted", v),
		texto: ui.Texto(b.Texto, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.Refresh()
	return r
}

type botonPanelRenderer struct {
	b     *botonPanel
	aro   *ui.Aro
	caja  *ui.Caja
	glifo *iconos.Dibujo
	texto *canvas.Text
}

func (r *botonPanelRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *botonPanelRenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.glifo.Objeto(), r.texto)
}
func (r *botonPanelRenderer) Destroy() {}

func (r *botonPanelRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	l := r.b.lado()
	if r.b.Texto == "" {
		r.glifo.Colocar((t.Width-l)/2, (t.Height-l)/2, l)
		return
	}
	x := tema.Espacio(2)
	if r.b.Contorno {
		x += tema.Borde
	}
	r.glifo.Colocar(x, (t.Height-l)/2, l)
	ui.ColocarTexto(r.texto, tema.TextoXS, x+l+tema.Espacio(1), (t.Height-tema.TextoXS.Linea)/2)
}

func (r *botonPanelRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.b.encima && !r.b.Inerte {
		fondo = tema.Color("bg-subtle", v)
		if r.b.Texto == "" {
			fondo = tema.Color("selected", v) // `hover:bg-selected` en los de fila
		}
	}
	tinta, borde := tema.Color("text-muted", v), color.NRGBA{}
	if r.b.Contorno {
		tinta, borde = tema.Color("text", v), tema.Color("border", v)
		if !r.b.encima {
			fondo = tema.Color("surface", v)
		}
	}
	if r.b.Inerte {
		tinta.A = uint8(float32(tinta.A) * 0.3)
	}
	r.caja.Pintar(fondo, borde)
	r.glifo.TenirCon(tinta)
	r.texto.Color = tinta
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.b.enfocado)
	r.Layout(r.b.Size())
}

// bordeContorno es el borde con el que nace la caja del botón: `ui.Caja` decide al nacer
// si lo pinta, así que el de «Contorno» tiene que llevarlo desde el principio.
func bordeContorno(contorno bool, v tema.Variante) color.NRGBA {
	if contorno {
		return tema.Color("border", v)
	}
	return color.NRGBA{}
}
