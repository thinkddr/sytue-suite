// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # Cómo se pinta la rejilla
//
// Cada celda visible es SU caja, como el `<div>` de la web: su fondo, sus dos líneas
// (`border-r border-b`), su texto y, si su estilo los lleva, los bordes de arriba y de
// la izquierda. No hay un papel común debajo con la cuadrícula encima: con rellenos,
// rangos teñidos y paneles inmovilizados, cada celda tiene que tapar lo que pase por
// debajo de ella, y eso solo lo hace una caja por celda.
//
// Las líneas van con su ALFA (tinta marina al 17 %) y no aplanadas sobre el papel:
// así una línea sobre un relleno amarillo sale mezclada con el amarillo, igual que en
// el navegador. Cuantizar 0,17 a un byte cambia el color en menos de una décima de
// nivel, que no llega a moverlo.
//
// # El orden es el de apilado de la web
//
// Primero las celdas que se desplazan en los dos ejes, luego las de columnas
// inmovilizadas (`sticky z-10`), luego las filas inmovilizadas (`sticky z-20`) con sus
// celdas y, encima de todo, las cabeceras y la esquina. Lo que se desplaza pasa POR
// DEBAJO de lo inmovilizado sin recortar nada: lo tapa.

// LineaRejillaAlfa es `--editor-grid-line` sin aplanar: lo que se pinta.
var LineaRejillaAlfa = func() color.NRGBA {
	c := tema.Color("accent-800", tema.Claro)
	c.A = tokens.Alfa(0.17)
	return c
}()

// CreateRenderer monta los charcos de primitivas.
func (r *Rejilla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rr := &rejillaRenderer{
		r: r, fondo: canvas.NewRectangle(FondoLienzo(v)), esquina: nuevaCabecera(v),
		asa: canvas.NewRectangle(tema.Color("primary-500", v)), asaBorde: canvas.NewRectangle(Papel), botonVal: nuevaCabecera(v),
	}
	for i := range rr.marcoSel {
		rr.marcoSel[i] = canvas.NewRectangle(tema.Color("primary-500", v))
		rr.marcoFoco[i] = canvas.NewRectangle(tema.Color("focus-ring", v))
	}
	r.r = rr
	rr.recomponer()
	return rr
}

// celdaPintada son las primitivas de UNA celda visible.
type celdaPintada struct {
	fondo       *canvas.Rectangle
	barra       *canvas.Rectangle   // la barra de datos del formato condicional
	der, abajo  *canvas.Rectangle   // `border-r border-b`: la cuadrícula o el borde del estilo
	arriba, izq *canvas.Rectangle   // los bordes de arriba e izquierda del estilo
	trazos      []*canvas.Rectangle // los segmentos de los bordes discontinuos, de puntos o dobles
	icono       *canvas.Image       // el icono de un juego del formato condicional
	mini        *rasterSVG          // el minigráfico, si la celda es su anfitriona
	texto       *canvas.Text
	// otra es la segunda línea de un texto ajustado que, centrado en alto, enseña la
	// mitad de dos líneas; las dos van dentro de `caja`, que recorta a la celda.
	otra *canvas.Text
	caja *recorte
}

// nuevaCelda monta las primitivas de una celda; `rueda` es la de la rejilla, que el
// recorte del texto le devuelve.
func nuevaCelda(rueda func(*fyne.ScrollEvent)) *celdaPintada {
	cp := &celdaPintada{
		fondo: canvas.NewRectangle(Papel), barra: canvas.NewRectangle(color.NRGBA{}),
		der: canvas.NewRectangle(LineaRejillaAlfa), abajo: canvas.NewRectangle(LineaRejillaAlfa),
		arriba: canvas.NewRectangle(color.NRGBA{}), izq: canvas.NewRectangle(color.NRGBA{}),
		icono: &canvas.Image{FillMode: canvas.ImageFillStretch}, mini: nuevoRasterSVG(),
		texto: ui.Texto("", EscalonCelda, ui.Normal, Tinta), otra: ui.Texto("", EscalonCelda, ui.Normal, Tinta),
	}
	cp.caja = nuevoRecorte(rueda, cp.texto, cp.otra)
	return cp
}

func (cp *celdaPintada) objetos() []fyne.CanvasObject {
	o := make([]fyne.CanvasObject, 0, 9+len(cp.trazos))
	o = append(o, cp.fondo, cp.barra, cp.der, cp.abajo, cp.arriba, cp.izq)
	for _, t := range cp.trazos {
		o = append(o, t)
	}
	return append(o, cp.icono, cp.mini.raster, cp.caja)
}

// recorte es `overflow-hidden`: lo que cuelga de él se ve solo dentro de su caja, y se
// corta a mitad de glifo como en el navegador. Fyne recorta lo que cuelga de un widget
// solo si el widget se desplaza (`fyne.Scrollable`, que es lo que mira
// `isClipWithRenderer`), así que este dice que se desplaza y le pasa la rueda a la
// rejilla. No es pulsable ni sigue el puntero: lo demás le llega a la rejilla de debajo.
type recorte struct {
	widget.BaseWidget
	objs  []fyne.CanvasObject
	rueda func(*fyne.ScrollEvent)
}

var _ fyne.Scrollable = (*recorte)(nil)

func nuevoRecorte(rueda func(*fyne.ScrollEvent), objs ...fyne.CanvasObject) *recorte {
	r := &recorte{objs: objs, rueda: rueda}
	r.ExtendBaseWidget(r)
	return r
}

// Scrolled devuelve la rueda a quien la atiende, si hay alguien.
func (r *recorte) Scrolled(e *fyne.ScrollEvent) {
	if r.rueda != nil {
		r.rueda(e)
	}
}

func (r *recorte) CreateRenderer() fyne.WidgetRenderer { return recorteRenderer{r} }

// recorteRenderer no coloca nada: lo hace el pintor de la rejilla, en coordenadas de la caja.
type recorteRenderer struct{ r *recorte }

func (recorteRenderer) Layout(fyne.Size)                { /* coloca el pintor */ }
func (recorteRenderer) MinSize() fyne.Size              { return fyne.Size{} }
func (rr recorteRenderer) Objects() []fyne.CanvasObject { return rr.r.objs }
func (recorteRenderer) Destroy()                        {}
func (rr recorteRenderer) Refresh() {
	for _, o := range rr.r.objs {
		o.Refresh()
	}
}

// iconosCond guarda el SVG de cada icono de juego ya hecho recurso: son nueve y se
// piden en cada pintado.
var iconosCond = map[string]fyne.Resource{}

func recursoIcono(icono string) fyne.Resource {
	r, ok := iconosCond[icono]
	if !ok {
		r = fyne.NewStaticResource("cond-"+strings.ReplaceAll(icono, ":", "-")+".svg", []byte(svgIcono(icono)))
		iconosCond[icono] = r
	}
	return r
}

// LadoIconoCond y HuecoIconoCond son el `width="12"` del SVG y su `margin-right:3px`.
const (
	LadoIconoCond  float32 = 12
	HuecoIconoCond float32 = 3
)

// cabeceraPintada es una cabecera de fila o de columna (o la esquina): `headCls`, con
// su fondo, sus dos líneas y su rótulo.
type cabeceraPintada struct {
	fondo, der, abajo *canvas.Rectangle
	rotulo            *canvas.Text
}

func nuevaCabecera(v tema.Variante) *cabeceraPintada {
	return &cabeceraPintada{
		fondo: canvas.NewRectangle(tema.Color("bg-subtle", v)), der: canvas.NewRectangle(tema.Color("border", v)),
		abajo: canvas.NewRectangle(tema.Color("border", v)), rotulo: ui.Texto("", EscalonCabecera, ui.Normal, tema.Color("text-muted", v)),
	}
}

func (cb *cabeceraPintada) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{cb.fondo, cb.der, cb.abajo, cb.rotulo}
}

func (cb *cabeceraPintada) esconder() {
	for _, o := range cb.objetos() {
		o.Hide()
	}
}

// rejillaRenderer recicla primitivas: los charcos crecen hasta lo que cabe en la
// ventana y ahí se quedan. Es la virtualización, y es lo que hace que una rejilla de
// 26×100 y una de 26×5.000 cuesten lo mismo.
type rejillaRenderer struct {
	r *Rejilla

	fondo             *canvas.Rectangle
	celdas            []*celdaPintada
	cabCols, cabFilas []*cabeceraPintada
	esquina           *cabeceraPintada
	marcoSel          [4]*canvas.Rectangle
	marcoFoco         [4]*canvas.Rectangle
	marcosFx          []*marcoFx
	tinteCelda        tinte
	asa, asaBorde     *canvas.Rectangle
	invalidas         []*canvas.Rectangle // el contorno rojo de lo que no cumple su validación
	botonVal          *cabeceraPintada    // el «▾» de la validación de lista (misma receta: fondo, línea y rótulo)
	botonesFiltro     []*cabeceraPintada  // los «▾» del autofiltro, uno por columna
	previa            []*canvas.Rectangle // el contorno del destino del autorrelleno
	marcosTabla       []*canvas.Rectangle // el contorno de cada tabla y el filo bajo su cabecera
	flotantes         []fyne.CanvasObject // los gráficos y las segmentaciones (`z-10`)
	canal             canalPintado        // el canalón del esquema: ver esquema_pintor.go
	objetos           []fyne.CanvasObject
	ultimo            fyne.Size

	// Lo de la hoja que se mira en cada celda, leído una vez por pintado.
	tablas []tablaCompilada
	minis  map[Celda]Minigrafico
}

// marcoFx es el marco de color de una referencia de la fórmula en edición
// (`rangeOverlays`): `1.5px solid` del color de la referencia y el fondo al 8 %.
type marcoFx struct {
	fondo  *canvas.Rectangle
	bordes [4]*canvas.Rectangle
}

func nuevoMarcoFx() *marcoFx {
	m := &marcoFx{fondo: canvas.NewRectangle(color.NRGBA{})}
	for i := range m.bordes {
		m.bordes[i] = canvas.NewRectangle(color.NRGBA{})
	}
	return m
}

// GrosorMarcoFx es el `1.5px` del borde del marco de una referencia.
const GrosorMarcoFx float32 = 1.5

func (rr *rejillaRenderer) MinSize() fyne.Size           { return rr.r.MinSize() }
func (rr *rejillaRenderer) Destroy()                     {}
func (rr *rejillaRenderer) Objects() []fyne.CanvasObject { return rr.objetos }

// recomponer ordena los objetos en el orden en que se pintan (ver el comentario del
// fichero). Solo hace falta cuando un charco crece.
func (rr *rejillaRenderer) recomponer() {
	o := make([]fyne.CanvasObject, 0, 16+6*len(rr.celdas)+4*(len(rr.cabCols)+len(rr.cabFilas)))
	o = append(o, rr.fondo)
	for _, cp := range rr.celdas {
		o = append(o, cp.objetos()...)
	}
	for _, m := range rr.marcoSel {
		o = append(o, m)
	}
	// Lo que no cumple su validación, la previa del autorrelleno, el asa y los marcos de
	// las referencias van sobre las celdas y bajo las cabeceras (`z-10`).
	for _, p := range rr.invalidas {
		o = append(o, p)
	}
	for _, p := range rr.previa {
		o = append(o, p)
	}
	for _, m := range rr.marcosTabla {
		o = append(o, m)
	}
	o = append(o, rr.asaBorde, rr.asa)
	o = append(o, rr.botonVal.objetos()...)
	for _, b := range rr.botonesFiltro {
		o = append(o, b.objetos()...)
	}
	for _, m := range rr.marcosFx {
		o = append(o, m.fondo)
		for _, b := range m.bordes {
			o = append(o, b)
		}
	}
	o = append(o, rr.canal.cuerpo()...)
	o = append(o, rr.flotantes...)
	for _, cb := range rr.cabFilas {
		o = append(o, cb.objetos()...)
	}
	for _, cb := range rr.cabCols {
		o = append(o, cb.objetos()...)
	}
	o = append(o, rr.esquina.objetos()...)
	o = append(o, rr.canal.encima()...)
	for _, m := range rr.marcoFoco {
		o = append(o, m)
	}
	o = append(o, rr.tinteCelda.objetos()...)
	rr.objetos = append(o, rr.r.ed.Objeto()) //nolint:gocritic // `o` es local y su único destino es este campo
}

// Refresh recoloca: en esta rejilla el contenido de cada primitiva depende de dónde
// cae, así que pintar y colocar son lo mismo.
func (rr *rejillaRenderer) Refresh() {
	if rr.ultimo.IsZero() {
		return
	}
	rr.Layout(rr.ultimo)
	for _, o := range rr.objetos {
		o.Refresh()
	}
}

// vista es la geometría de un pintado: el zoom, lo inmovilizado y el desplazamiento.
// Todo en píxeles de la PANTALLA salvo los índices.
type vista struct {
	z          float32
	fc, ff     int     // columnas y filas inmovilizadas
	desX, desY float32 // el desplazamiento, ya con el zoom
	filas      mapaFilas
	// cols son las columnas que se ven (el esquema puede esconder alguna), con el mismo
	// mapa que las filas; cx y cy, el canalón del esquema (`RGUT` y `CGUT`) en px de la
	// hoja: corren el origen de las celdas sin cambiar su tamaño, como en la web.
	cols   mapaFilas
	cx, cy float32
}

// mapaFilas es el `visRows` de la web: con el autofiltro escondiendo filas, `antes[f]` es
// cuántas se ven por encima de f —su puesto en pantalla— y `orden[i]` es la fila que se
// ve en el puesto i. El cero es la rejilla entera, sin escondidas.
type mapaFilas struct {
	antes, orden []int
}

// nuevoMapaFilas lo monta con las filas que esconde el autofiltro (nil: ninguna).
func nuevoMapaFilas(ocultas []bool) mapaFilas {
	if ocultas == nil {
		return mapaFilas{}
	}
	m := mapaFilas{antes: make([]int, len(ocultas)+1)}
	for f, oculta := range ocultas {
		m.antes[f] = len(m.orden)
		if !oculta {
			m.orden = append(m.orden, f)
		}
	}
	m.antes[len(ocultas)] = len(m.orden)
	return m
}

// puesto es dónde se pinta la fila f contando solo las que se ven. Una escondida da el
// de la siguiente que se ve: mide cero.
func (m mapaFilas) puesto(f int) int {
	if m.antes == nil || f < 0 {
		return f
	}
	if f >= len(m.antes) {
		return m.antes[len(m.antes)-1] + f - (len(m.antes) - 1)
	}
	return m.antes[f]
}

// fila es la inversa: la fila que se ve en el puesto i. Más allá de la última, una fila
// fuera de la rejilla.
func (m mapaFilas) fila(i int) int {
	if m.antes == nil || i < 0 {
		return i
	}
	if i >= len(m.orden) {
		return len(m.antes) - 1 + i - len(m.orden)
	}
	return m.orden[i]
}

// oculta dice si el autofiltro esconde la fila f.
func (m mapaFilas) oculta(f int) bool {
	return m.antes != nil && f >= 0 && f < len(m.antes)-1 && m.antes[f+1] == m.antes[f]
}

func (v vista) ancho() float32 { return AnchoCelda * v.z }
func (v vista) alto() float32  { return AltoCelda * v.z }
func (v vista) ox() float32    { return (AnchoCabecera + v.cx) * v.z }
func (v vista) oy() float32    { return (AltoCelda + v.cy) * v.z }

// x es dónde empieza la columna c en la pantalla: las inmovilizadas no se mueven.
func (v vista) x(c int) float32 {
	x := v.ox() + float32(v.cols.puesto(c))*v.ancho()
	if c >= v.fc {
		x -= v.desX
	}
	return x
}

func (v vista) y(f int) float32 {
	y := v.oy() + float32(v.filas.puesto(f))*v.alto()
	if f >= v.ff {
		y -= v.desY
	}
	return y
}

// finFijas es dónde acaban las columnas y filas inmovilizadas: lo que se desplaza y cae
// antes de ahí está tapado.
func (v vista) finFijas() (x, y float32) {
	return v.ox() + float32(v.cols.puesto(v.fc))*v.ancho(), v.oy() + float32(v.filas.puesto(v.ff))*v.alto()
}

// visibles son las columnas (o filas) que se ven: primero las que se desplazan —las de
// debajo— y luego las inmovilizadas, que se pintan encima. `desde` es la primera que
// se desplaza y no está tapada del todo.
//
// `oculta` son las que esconde el autofiltro (nil en las columnas), que no se pintan.
func visibles(fijas, total, desde int, pos func(int) float32, limite float32, oculta func(int) bool) []int {
	out := make([]int, 0, 32)
	for i := max(fijas, desde); i < total && pos(i) < limite; i++ {
		if oculta == nil || !oculta(i) {
			out = append(out, i)
		}
	}
	for i := range min(fijas, total) {
		if pos(i) < limite && (oculta == nil || !oculta(i)) {
			out = append(out, i)
		}
	}
	return out
}

// Layout coloca la ventana visible.
func (rr *rejillaRenderer) Layout(t fyne.Size) {
	rr.ultimo = t
	r := rr.r
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	vw := r.vista()

	rr.fondo.FillColor = FondoLienzo(v)
	rr.fondo.Move(fyne.NewPos(0, 0))
	rr.fondo.Resize(t)
	rr.tablas, rr.minis = r.L.tablasActivas(), r.L.minigraficosPorCelda()

	cols := visibles(vw.fc, r.Cols, vw.cols.fila(vw.cols.puesto(vw.fc)+int(r.desX/AnchoCelda)), vw.x, t.Width, vw.cols.oculta)
	fils := visibles(vw.ff, r.Fils, vw.filas.fila(vw.filas.puesto(vw.ff)+int(r.desY/AltoCelda)), vw.y, t.Height, vw.filas.oculta)
	rr.crecer(len(cols), len(fils), v)

	rr.pintarCeldas(v, vw, cols, fils)
	rr.pintarInvalidas(vw, cols, fils)
	rr.pintarMarcosTabla(vw, t)
	rr.pintarCabeceras(v, vw, cols, fils)
	rr.pintarCanal(v, vw, cols, fils)
	rr.pintarSeleccion(v, vw, t)
	rr.pintarMarcosFx(vw)
	rr.pintarAsa(v, vw)
	rr.pintarBotonValores(v)
	rr.pintarBotonesFiltro(v)
	rr.colocarEditor(vw)
	rr.pintarFlotantes(vw)
}

// pintarFlotantes coloca los gráficos y las segmentaciones, que pone el lienzo, y
// recompone si han cambiado: van encima de las celdas y bajo las cabeceras pegajosas,
// que es el `z-10` de la web.
func (rr *rejillaRenderer) pintarFlotantes(vw vista) {
	if rr.r.Flotantes == nil {
		return
	}
	nuevos := rr.r.Flotantes(vw)
	if !slices.Equal(nuevos, rr.flotantes) {
		rr.flotantes = nuevos
		rr.recomponer()
	}
}

// pintarBotonValores pone el «▾» de la celda con una validación de lista:
//
//	border-border bg-bg-subtle text-text-muted hover:bg-selected hover:text-text text-2xs
//	flex h-[24px] w-[18px] items-center justify-center border-l leading-none
func (rr *rejillaRenderer) pintarBotonValores(v tema.Variante) {
	pos, tam, ok := rr.r.botonValores()
	if !ok {
		rr.botonVal.esconder()
		return
	}
	rr.pintarDesplegable(rr.botonVal, pos, tam, rr.r.boton == -1, v)
}

// pintarBotonesFiltro pone los «▾» del autofiltro en su fila de cabecera:
//
//	border-border text-2xs flex h-[24px] w-4 items-center justify-center border-l
//	leading-none, y `bg-selected text-text` en la columna que filtra y bajo el puntero
//	(si no, `bg-bg-subtle text-text-muted`)
func (rr *rejillaRenderer) pintarBotonesFiltro(v tema.Variante) {
	r := rr.r
	af := r.L.Autofiltro()
	n := 0
	if af != nil {
		n = af.C2 - af.C1 + 1
	}
	for len(rr.botonesFiltro) < n {
		rr.botonesFiltro = append(rr.botonesFiltro, nuevaCabecera(v))
		rr.recomponer()
	}
	for i, b := range rr.botonesFiltro {
		if i >= n {
			b.esconder()
			continue
		}
		pos, tam, ok := r.botonFiltro(i + af.C1)
		if !ok {
			b.esconder()
			continue
		}
		col := i + af.C1
		rr.pintarDesplegable(b, pos, tam, af.filtrada(col) || r.boton == col+1, v)
	}
}

// pintarDesplegable es la receta de los dos «▾»: fondo, `border-l` y la flecha. `activo`
// es la columna que filtra o el puntero encima: `bg-selected text-text`.
func (rr *rejillaRenderer) pintarDesplegable(b *cabeceraPintada, pos fyne.Position, tam fyne.Size, activo bool, v tema.Variante) {
	fondo, tinta := tema.Color("bg-subtle", v), tema.Color("text-muted", v)
	if activo {
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v) // translúcido, sobre la celda
	}
	caja(b.fondo, fondo, pos.X, pos.Y, tam.Width, tam.Height)
	caja(b.der, tema.Color("border", v), pos.X, pos.Y, tema.Borde*rr.r.z(), tam.Height) // `border-l`
	b.abajo.Hide()
	esc := tema.Escalon{Tam: tema.Texto2XS.Tam * rr.r.z(), Linea: tema.Texto2XS.Tam * rr.r.z()}
	b.rotulo.Text, b.rotulo.TextSize, b.rotulo.FontSource = "▾", esc.Tam, tema.Regular
	b.rotulo.Color = tinta
	w := medirCon(b.rotulo, "▾")
	ui.ColocarTexto(b.rotulo, esc, pos.X+(tam.Width-w)/2, pos.Y+(tam.Height-esc.Linea)/2)
	b.rotulo.Show()
}

// Peligro es `DANGER`, el `CONTENT_PALETTE[5]` con el que la web marca un valor que no
// cumple la validación de su celda.
var Peligro = color.NRGBA{R: 0xDC, G: 0x26, B: 0x26, A: 0xff}

// pintarInvalidas marca las celdas visibles cuyo literal no cumple una validación NO
// estricta: `outline: 1px solid DANGER` con `outline-offset: -1px`, el píxel de fuera
// de la caja de la celda, encima de sus líneas.
func (rr *rejillaRenderer) pintarInvalidas(vw vista, cols, fils []int) {
	r := rr.r
	n := 0
	if len(r.L.Validaciones()) > 0 {
		for _, f := range fils {
			for _, c := range cols {
				if !r.L.marcaInvalida(c, f) {
					continue
				}
				for len(rr.invalidas) < n+4 {
					rr.invalidas = append(rr.invalidas, canvas.NewRectangle(Peligro))
					rr.recomponer()
				}
				var cuatro [4]*canvas.Rectangle
				copy(cuatro[:], rr.invalidas[n:n+4])
				marco(cuatro, Peligro, fyne.NewPos(vw.x(c), vw.y(f)), fyne.NewSize(vw.ancho(), vw.alto()), tema.Borde*vw.z, 0, 0, true)
				n += 4
			}
		}
	}
	for ; n < len(rr.invalidas); n++ {
		rr.invalidas[n].Hide()
	}
}

// pintarAsa pone el asa de autorrelleno (`bg ACCENT` con borde de papel) y, mientras se
// arrastra, la previa del destino: en la web es un `outline: 1px dashed ACCENT` con
// `outline-offset: -2px` en cada celda. Fyne no tiene trazos discontinuos: cada trazo
// es un rectángulo, repartido como lo reparte Chromium (ver `discontinuo`).
func (rr *rejillaRenderer) pintarAsa(v tema.Variante, vw vista) {
	r := rr.r
	primario := tema.Color("primary-500", v)
	if r.editando {
		rr.asa.Hide()
		rr.asaBorde.Hide()
	} else {
		pos, lado := r.asa()
		b := tema.Borde * vw.z
		caja(rr.asaBorde, Papel, pos.X, pos.Y, lado, lado)
		caja(rr.asa, primario, pos.X+b, pos.Y+b, lado-2*b, lado-2*b)
	}
	n := 0
	if d := r.destRell; d != nil {
		for f := d[1]; f <= d[3]; f++ {
			for c := d[0]; c <= d[2]; c++ {
				// El trazo de 1 px, 2 px hacia dentro del borde de la celda.
				dentro, g := 2*vw.z, tema.Borde*vw.z
				pos := fyne.NewPos(vw.x(c)+dentro-g, vw.y(f)+dentro-g)
				tam := fyne.NewSize(vw.ancho()-2*dentro+2*g, vw.alto()-2*dentro+2*g)
				for _, t := range discontinuo(pos, tam, g) {
					if n == len(rr.previa) {
						rr.previa = append(rr.previa, canvas.NewRectangle(primario))
						rr.recomponer()
					}
					caja(rr.previa[n], primario, t.x, t.y, t.w, t.h)
					n++
				}
			}
		}
	}
	for ; n < len(rr.previa); n++ {
		rr.previa[n].Hide()
	}
}

// tramo es un rectángulo suelto, en píxeles de la pantalla.
type tramo struct{ x, y, w, h float32 }

// discontinuo son los trazos de `outline: <g> dashed` alrededor de la caja: en cada lado,
// trazos de 3·g con huecos de unos 2·g, repartidos para que el lado empiece y acabe en
// trazo. Es `SelectBestDashGap` de Chromium, MEDIDO: en la celda del autorrelleno
// (91×24, el contorno de 89×22) pinta 18 trazos arriba con huecos de 2,06 y 5 a cada
// lado con huecos de 1,75, igual a ×1 que a ×2.
func discontinuo(pos fyne.Position, tam fyne.Size, g float32) []tramo {
	var out []tramo
	lado := func(largo float32, poner func(desde, hasta float32)) {
		t := 3 * g
		h := huecoDiscontinuo(largo, t, 2*g)
		for x := float32(0); x < largo-0.001; x += t + h {
			poner(x, min(x+t, largo))
		}
	}
	lado(tam.Width, func(a, b float32) {
		out = append(out, tramo{pos.X + a, pos.Y, b - a, g}, tramo{pos.X + a, pos.Y + tam.Height - g, b - a, g})
	})
	lado(tam.Height, func(a, b float32) {
		out = append(out, tramo{pos.X, pos.Y + a, g, b - a}, tramo{pos.X + tam.Width - g, pos.Y + a, g, b - a})
	})
	return out
}

// huecoDiscontinuo es `SelectBestDashGap` de Chromium para un trazo abierto: de los dos
// números de trazos que caben, el que deja el hueco más parecido al pedido.
func huecoDiscontinuo(largo, trazo, hueco float32) float32 {
	n := float32(math.Floor(float64((largo + hueco) / (trazo + hueco))))
	menos, mas := float32(math.Inf(1)), (largo-(n+1)*trazo)/n
	if n > 1 {
		menos = (largo - n*trazo) / (n - 1)
	}
	if mas <= 0 || math.Abs(float64(menos-hueco)) < math.Abs(float64(mas-hueco)) {
		return menos
	}
	return mas
}

// pintarMarcosFx pone un marco de su color sobre cada zona a la que apunta la fórmula
// en edición. Van en las coordenadas de la HOJA, como en la web (`colX` y el índice de
// fila visible): no saben de paneles inmovilizados. Y como allí las filas y columnas
// inmovilizadas (`z-20`) tapan los marcos (`z-10`), se recortan donde empieza lo que se
// desplaza: lo que cae bajo lo inmovilizado no se ve.
func (rr *rejillaRenderer) pintarMarcosFx(vw vista) {
	r := rr.r
	for len(rr.marcosFx) < len(r.refsFx) {
		rr.marcosFx = append(rr.marcosFx, nuevoMarcoFx())
		rr.recomponer()
	}
	n := 0
	for _, ref := range r.refsFx {
		if ref.Rect.C1 >= r.Cols || ref.Rect.R1 >= r.Fils {
			continue
		}
		c2, f2 := min(ref.Rect.C2, r.Cols-1), min(ref.Rect.R2, r.Fils-1)
		x := vw.ox() + float32(vw.cols.puesto(ref.Rect.C1))*vw.ancho() - vw.desX
		y := vw.oy() + float32(vw.filas.puesto(ref.Rect.R1))*vw.alto() - vw.desY
		w := float32(vw.cols.puesto(c2+1)-vw.cols.puesto(ref.Rect.C1)) * vw.ancho()
		h := float32(vw.filas.puesto(f2+1)-vw.filas.puesto(ref.Rect.R1)) * vw.alto()
		m := rr.marcosFx[n]
		n++
		c := ColorRef(ref.Color)
		fondo := c
		fondo.A = tokens.Alfa(0.08)
		finX, finY := vw.finFijas()
		if x0, y0 := max(x, finX), max(y, finY); x+w > x0 && y+h > y0 {
			caja(m.fondo, fondo, x0, y0, x+w-x0, y+h-y0)
		} else {
			m.fondo.Hide()
		}
		marco(m.bordes, c, fyne.NewPos(x, y), fyne.NewSize(w, h), GrosorMarcoFx*vw.z, finX, finY, true)
	}
	for ; n < len(rr.marcosFx); n++ {
		rr.marcosFx[n].fondo.Hide()
		for _, b := range rr.marcosFx[n].bordes {
			b.Hide()
		}
	}
}

// crecer agranda los charcos hasta lo que pide la ventana. Nunca encoge: una ventana
// que se estira y se vuelve a encoger no debe reasignar nada.
func (rr *rejillaRenderer) crecer(nc, nf int, v tema.Variante) {
	cambio := false
	for len(rr.celdas) < nc*nf {
		rr.celdas = append(rr.celdas, nuevaCelda(rr.r.Scrolled))
		cambio = true
	}
	for len(rr.cabCols) < nc {
		rr.cabCols = append(rr.cabCols, nuevaCabecera(v))
		cambio = true
	}
	for len(rr.cabFilas) < nf {
		rr.cabFilas = append(rr.cabFilas, nuevaCabecera(v))
		cambio = true
	}
	if cambio {
		rr.recomponer()
	}
}

// pintarCeldas coloca cada celda visible en el orden de apilado: fila a fila, primero
// las que se desplazan y luego las inmovilizadas (así salen ya de `visibles`), y dentro
// de cada fila igual con las columnas. Como las filas inmovilizadas van al final, sus
// celdas tapan a todas las demás, y dentro de las demás las columnas inmovilizadas
// tapan a las que se desplazan.
func (rr *rejillaRenderer) pintarCeldas(v tema.Variante, vw vista, cols, fils []int) {
	r := rr.r
	c1, f1, c2, f2 := r.Rango()
	primario := tema.Color("primary-500", v)
	fuerte := tema.Color("border-strong", v)
	i := 0
	// Primero todas las filas que se desplazan con sus columnas que se desplazan; luego
	// esas mismas filas con sus columnas inmovilizadas (`sticky z-10`, encima); y al
	// final las filas inmovilizadas enteras (`sticky z-20`).
	pasada := func(filaFija bool, colFija *bool) {
		for _, f := range fils {
			if (f < vw.ff) != filaFija {
				continue
			}
			for _, c := range cols {
				if colFija != nil && (c < vw.fc) != *colFija {
					continue
				}
				dentro := r.ext != nil && c >= c1 && c <= c2 && f >= f1 && f <= f2 && (c != r.sel.C || f != r.sel.F)
				rr.pintarCelda(rr.celdas[i], c, f, vw, dentro, primario, fuerte)
				i++
			}
		}
	}
	no, si := false, true
	pasada(false, &no)
	pasada(false, &si)
	pasada(true, nil)
	for ; i < len(rr.celdas); i++ {
		for _, o := range rr.celdas[i].objetos() {
			o.Hide()
		}
	}
}

// pintarCelda es `cellStyle` + `cellStyleCss` + `cellHtml` de la web para UNA celda.
func (rr *rejillaRenderer) pintarCelda(cp *celdaPintada, c, f int, vw vista, enRango bool, primario, fuerte color.NRGBA) {
	r := rr.r
	z := vw.z
	x, y := vw.x(c), vw.y(f)
	w, h := vw.ancho(), vw.alto()
	e := r.L.Estilo(c, f)
	d := r.L.decoracionDe(c, f)

	fondo, info, enTabla := rr.fondoCelda(c, f, e, d, enRango, primario)
	if r.resaltes[Celda{C: c, F: f}] {
		e.Tinta = "" // «Buscar»: la tinta del contenido sobre el amarillo, como en la web
	}
	caja(cp.fondo, fondo, x, y, w, h)
	pintarBarra(cp.barra, d, x, y, w-tema.Borde*z, h-tema.Borde*z)

	anchoDer, anchoAbajo, bt, bl := rr.pintarBordesCelda(cp, c, f, vw, e.Bordes, x, y, w, h, fuerte)

	icono := ""
	if d != nil {
		icono = d.icono
	}
	var papel string
	if enTabla {
		papel = info.papel
		// La tinta de la cabecera es la del estilo de la tabla, si la celda no fija otra.
		if papel == papelCabecera && e.Tinta == "" {
			e.Tinta = estiloTablaDe(info.ct.t.Style).TintaCab
		}
	}
	if d != nil && d.tinta != "" && !r.resaltes[Celda{C: c, F: f}] {
		// El formato condicional gana al estilo y a la tabla, como su relleno; no a «Buscar».
		e.Tinta = d.tinta
	}
	if m, ok := rr.minis[Celda{C: c, F: f}]; ok {
		cp.caja.Hide() // el texto va dentro de su recorte
		cp.texto.Hide()
		cp.icono.Hide()
		rr.pintarMinigrafico(cp.mini, m, x+bl, y+bt, w-anchoDer-bl, z)
		return
	}
	cp.mini.raster.Hide()
	rr.pintarTexto(cp, c, f, e, icono, papel, x+bl, y+bt, w-anchoDer-bl, h-anchoAbajo-bt, z)
}

// pintarBordesCelda coloca las líneas de la cuadrícula (`border-strong` en el filo de lo
// inmovilizado) o los bordes del estilo (`borderSideCss`), por segmentos en `dashed`,
// `dotted` y `double`.
func (rr *rejillaRenderer) pintarBordesCelda(cp *celdaPintada, c, f int, vw vista, bo Bordes, x, y, w, h float32, fuerte color.NRGBA) (anchoDer, anchoAbajo, bt, bl float32) {
	b := tema.Borde * vw.z
	lado := func(trazo, hex string) (color.NRGBA, float32) {
		col := fuerte
		if parsed, ok := tokens.ParseColor(hex); ok {
			col = parsed
		}
		switch trazo {
		case "medium":
			return col, 2 * b
		case "thick", "double":
			return col, 3 * b
		}
		return col, b
	}
	n := 0
	crecio := false
	poner := func(rect *canvas.Rectangle, col color.NRGBA, trazo string, bx, by, bw, bh float32, horiz bool) {
		ts := tramosBorde(trazo, b, bx, by, bw, bh, horiz)
		if len(ts) == 0 {
			caja(rect, col, bx, by, bw, bh)
			return
		}
		rect.Hide()
		for _, t := range ts {
			if n == len(cp.trazos) {
				cp.trazos = append(cp.trazos, canvas.NewRectangle(col))
				crecio = true
			}
			caja(cp.trazos[n], col, t.x, t.y, t.w, t.h)
			n++
		}
	}
	der, anchoDer := LineaRejillaAlfa, b
	trazoDer := ""
	if bo.R {
		der, anchoDer = lado(bo.RS, bo.RC)
		trazoDer = bo.RS
	} else if c == vw.fc-1 {
		der = fuerte
	}
	abajo, anchoAbajo := LineaRejillaAlfa, b
	trazoAbajo := ""
	if bo.B {
		abajo, anchoAbajo = lado(bo.BS, bo.BC)
		trazoAbajo = bo.BS
	} else if f == vw.ff-1 {
		abajo = fuerte
	}
	poner(cp.der, der, trazoDer, x+w-anchoDer, y, anchoDer, h, false)
	poner(cp.abajo, abajo, trazoAbajo, x, y+h-anchoAbajo, w, anchoAbajo, true)
	if bo.T {
		col, ancho := lado(bo.TS, bo.TC)
		bt = ancho
		poner(cp.arriba, col, bo.TS, x, y, w, ancho, true)
	} else {
		cp.arriba.Hide()
	}
	if bo.L {
		col, ancho := lado(bo.LS, bo.LC)
		bl = ancho
		poner(cp.izq, col, bo.LS, x, y, ancho, h, false)
	} else {
		cp.izq.Hide()
	}
	for ; n < len(cp.trazos); n++ {
		cp.trazos[n].Hide()
	}
	if crecio {
		rr.recomponer()
	}
	return anchoDer, anchoAbajo, bt, bl
}

// tramosBorde descompone un lado discontinuo (`dashed`), de puntos (`dotted`) o doble
// (`double`) en sus rectángulos; devuelve nil cuando el trazo es una línea continua.
func tramosBorde(trazo string, b, x, y, w, h float32, horiz bool) []tramo {
	if trazo == "double" {
		if horiz {
			return []tramo{{x, y, w, b}, {x, y + 2*b, w, b}}
		}
		return []tramo{{x, y, b, h}, {x + 2*b, y, b, h}}
	}
	var t, hueco float32
	switch trazo {
	case "dashed":
		t, hueco = 3*b, 2*b
	case "dotted":
		t, hueco = b, b
	default:
		return nil
	}
	largo := h
	if horiz {
		largo = w
	}
	paso := huecoDiscontinuo(largo, t, hueco)
	var out []tramo
	for d := float32(0); d < largo-0.001; d += t + paso {
		fin := min(d+t, largo)
		if horiz {
			out = append(out, tramo{x + d, y, fin - d, b})
		} else {
			out = append(out, tramo{x, y + d, b, fin - d})
		}
	}
	return out
}

// pintarBarra es `dataBarBackground`: una barra de bordes duros (sin degradado) sobre
// el fondo de la caja de relleno, desde el eje del cero hacia la derecha o, si el valor
// es negativo, hacia la izquierda. Las posiciones van al décimo de porcentaje, como en
// la web.
func pintarBarra(rc *canvas.Rectangle, d *decoracion, x, y, w, h float32) {
	if d == nil || d.barra == nil {
		rc.Hide()
		return
	}
	b := d.barra
	col, ok := tokens.ParseColor(b.color)
	if !ok {
		rc.Hide()
		return
	}
	pct := func(n float64) float32 { return float32(redondearJS(min(max(n, 0), 100)*10)/10) / 100 }
	eje := b.eje * 100
	ini, fin := pct(eje), pct(eje+b.frac*(100-eje))
	if b.neg {
		ini, fin = pct(max(0, eje-b.frac*b.eje*100)), pct(eje)
	}
	if fin <= ini {
		rc.Hide()
		return
	}
	caja(rc, col, x+ini*w, y, (fin-ini)*w, h)
}

// pintarTexto coloca el texto de la celda dentro de su caja de contenido: la de la celda
// menos sus bordes. `anchoRelleno` y `altoRelleno` son esa caja con el `px-1` todavía
// dentro.
func (rr *rejillaRenderer) pintarTexto(cp *celdaPintada, c, f int, e Estilo, icono, papel string, x, y, anchoRelleno, altoRelleno, z float32) {
	r := rr.r
	t := cp.texto
	texto, tintaMascara := r.L.Pintura(c, f)
	cp.otra.Hide()
	if texto == "" && icono == "" {
		cp.caja.Hide()
		t.Hide() // también el texto: quien lo mire suelto no debe ver el de antes
		cp.icono.Hide()
		return
	}
	negrita, cursiva, alinear := r.L.Formato(c, f)
	if papel == papelTotal {
		// `tableTotalHtml` pinta `<b>total</b>` sin el envoltorio de la celda.
		negrita, cursiva, alinear = true, false, ""
	}
	esc := escalonCelda(e, z)
	t.TextSize = esc.Tam
	t.FontSource, t.TextStyle = caraCelda(e.Fuente, negrita, cursiva)
	// La cabecera y los totales de una tabla van a 600 (`fontWeight: '600'`), salvo que
	// la celda ya sea negrita (el `<b>` de dentro manda) o cursiva.
	if (papel == papelCabecera || papel == papelTotal) && !negrita && !cursiva && t.FontSource == tema.Regular {
		t.FontSource = tema.SemiNegra
	}
	t.TextStyle.Underline, t.TextStyle.Strikethrough = e.Subraya, e.Tacha
	cp.otra.TextSize, cp.otra.FontSource, cp.otra.TextStyle = t.TextSize, t.FontSource, t.TextStyle
	// La tinta: la de la máscara (`[Rojo]`, un `<span>` dentro), si no la del estilo, y
	// si no la del CONTENIDO —no la del tema: la celda es papel blanco en los dos—.
	t.Color = Tinta
	if tinta, ok := tokens.ParseColor(e.Tinta); ok {
		t.Color = tinta
	}
	if tinta, ok := tokens.ParseColor(tintaMascara); ok {
		t.Color = tinta
	}
	cp.otra.Color = t.Color

	margen := MargenCelda * z
	contenido := anchoRelleno - 2*margen
	// El icono va en línea delante del texto (`withIcon`), con su hueco a la derecha.
	delante := float32(0)
	if icono != "" {
		delante = (LadoIconoCond + HuecoIconoCond) * z
	}
	// El texto va ENTERO dentro de la caja de relleno, que lo recorta (`overflow-hidden`
	// recorta ahí, no en la de contenido): lo que desborda se corta a mitad de glifo.
	cp.caja.Move(fyne.NewPos(x, y))
	cp.caja.Resize(fyne.NewSize(anchoRelleno, altoRelleno))
	cp.caja.Show()
	lineas := []string{texto}
	if e.Ajustar {
		lineas = partirLineas(cp.texto, texto, contenido-delante)
	}
	lx, ly := cp.colocarLineas(lineas, esc, e.Vertical, alinear, margen, contenido, delante, altoRelleno)
	px, py := x+lx, y+ly
	if icono == "" {
		cp.icono.Hide()
		return
	}
	// `vertical-align: -1px`: el pie del icono, un píxel por debajo de la línea base.
	// La línea base sale de la fuente, centrada en la caja de línea como el texto
	// (`ColocarTexto`), y así vale también para una celda sin texto, solo con icono.
	medida, base := fyne.CurrentApp().Driver().RenderedTextSize("x", t.TextSize, t.TextStyle, t.FontSource)
	lado := LadoIconoCond * z
	cp.icono.Resource = recursoIcono(icono)
	cp.icono.Move(fyne.NewPos(px, py+(esc.Linea-medida.Height)/2+base+z-lado))
	cp.icono.Resize(fyne.NewSize(lado, lado))
	cp.icono.Show()
}

// colocarLineas pone las líneas del texto en la caja que las recorta (`texto` y, si se
// ven dos, `otra`) y devuelve dónde empieza la primera que se ve, relativa a la caja.
//
// La caja de línea son 24 px (`leading-[24px]`) en una caja de contenido de 23 —la
// línea de abajo va dentro—. Sin `valign` es un bloque y el texto empieza arriba; con
// él es un `flex` columna y el bloque se reparte según `justify-content`, que con un
// hijo MÁS alto que la caja lo saca por arriba o por los dos lados. Con dos líneas
// centradas se ve la mitad de cada una.
func (cp *celdaPintada) colocarLineas(lineas []string, esc tema.Escalon, vertical, alinear string,
	margen, contenido, delante, alto float32,
) (px, py float32) {
	bloque := float32(len(lineas)) * esc.Linea
	arriba := float32(0)
	switch vertical {
	case "middle":
		arriba = (alto - bloque) / 2
	case "bottom":
		arriba = alto - bloque
	}
	pintadas := 0
	for i, linea := range lineas {
		ly := arriba + float32(i)*esc.Linea
		if ly+esc.Linea <= 0 || ly >= alto || pintadas == 2 {
			continue // fuera de la caja: no se ve
		}
		lt := []*canvas.Text{cp.texto, cp.otra}[pintadas]
		lt.Text = linea
		lx := margen
		// `text-align` solo mueve lo que cabe: una línea que desborda se alinea al
		// principio, que es lo que hace el navegador.
		if ancho := medirCon(lt, linea) + delante; ancho <= contenido {
			switch alinear {
			case "center":
				lx += (contenido - ancho) / 2
			case "right":
				lx += contenido - ancho
			}
		}
		ui.ColocarTexto(lt, esc, lx+delante, ly)
		lt.Show()
		if pintadas == 0 {
			px, py = lx, ly
		}
		pintadas++
	}
	if pintadas == 0 {
		cp.texto.Hide()
	}
	return px, py
}

// escalonCelda es el escalón de la letra de una celda: `text-sm` (o el tamaño del estilo,
// que va en pt) con la caja de línea fija de 24 px, todo por el zoom.
func escalonCelda(e Estilo, z float32) tema.Escalon {
	tam := EscalonCelda.Tam
	if e.Tam > 0 {
		tam = float32(e.Tam) * 4 / 3 // 1 pt = 4/3 px en CSS
	}
	return tema.Escalon{Tam: tam * z, Linea: AltoCelda * z}
}

// partirLineas es `wrap`: el texto repartido en líneas por palabras al ancho del
// contenido. La celda no crece: enseña lo que cae en su caja (ver pintarTexto).
func partirLineas(t *canvas.Text, texto string, ancho float32) []string {
	var lineas []string
	actual := ""
	for _, palabra := range strings.Fields(texto) {
		prueba := palabra
		if actual != "" {
			prueba = actual + " " + palabra
		}
		if actual != "" && medirCon(t, prueba) > ancho {
			lineas = append(lineas, actual)
			actual = palabra
			continue
		}
		actual = prueba
	}
	return append(lineas, actual)
}

// pintarCabeceras coloca las bandas, con el tinte opaco y la arista azul de superficie-hoja.css.
func (rr *rejillaRenderer) pintarCabeceras(v tema.Variante, vw vista, cols, fils []int) {
	r := rr.r
	c1, f1, c2, f2 := r.Rango()
	// Con esquema, las cabeceras van tras su canalón: la de columnas debajo del de
	// columnas y la de filas a la derecha del de filas. La esquina ocupa los dos.
	for i, c := range cols {
		rr.cabecera(rr.cabCols[i], v, c >= c1 && c <= c2, c == vw.fc-1, false, formula.NumToCol(c),
			vw.x(c), vw.cy*vw.z, vw.ancho(), vw.alto(), vw.z)
		if c >= c1 && c <= c2 {
			caja(rr.cabCols[i].abajo, tema.Color("primary-500", v), vw.x(c), vw.cy*vw.z+vw.alto()-2*vw.z, vw.ancho(), 2*vw.z)
		}
	}
	for i, f := range fils {
		rr.cabecera(rr.cabFilas[i], v, f >= f1 && f <= f2, false, f == vw.ff-1, strconv.Itoa(f+1),
			vw.cx*vw.z, vw.y(f), AnchoCabecera*vw.z, vw.alto(), vw.z)
		if f >= f1 && f <= f2 {
			caja(rr.cabFilas[i].der, tema.Color("primary-500", v), vw.cx*vw.z+AnchoCabecera*vw.z-2*vw.z, vw.y(f), 2*vw.z, vw.alto())
		}
	}
	for i := len(cols); i < len(rr.cabCols); i++ {
		rr.cabCols[i].esconder()
	}
	for i := len(fils); i < len(rr.cabFilas); i++ {
		rr.cabFilas[i].esconder()
	}
	rr.cabecera(rr.esquina, v, false, false, false, "", 0, vw.cy*vw.z, vw.ox(), vw.alto(), vw.z)
}

// cabecera pinta UNA cabecera: son todas el mismo `headCls`, con el rótulo centrado
// en la caja de CONTENIDO (la cabecera menos su `border-r`) y su caja de línea de 24.
// `filoDer` y `filoAbajo` son el `border-*-border-strong` del filo de lo inmovilizado.
func (rr *rejillaRenderer) cabecera(cb *cabeceraPintada, v tema.Variante, dentro, filoDer, filoAbajo bool,
	texto string, x, y, w, h, z float32,
) {
	fondo, peso, tinta, cara := tema.Color("bg-subtle", v), ui.Normal, tema.Color("text-muted", v), tema.Regular
	if dentro {
		// `color-mix(editor-select selected-strength, bg-subtle)` de superficie-hoja.css: el
		// acento es el del editor (`primary-500`) y la fuerza, la de la selección del tema,
		// que es el alfa del token `selected`. Mezclar el propio `selected` daba lo mismo
		// mientras ese token fue el 500; ahora es el 600.
		fuerza := float64(tokens.Crudo("selected", v).A) / 255
		fondo, peso, tinta, cara = tokens.Aplanar(tema.Color("primary-500", v), fuerza, fondo), ui.Medio, tema.Color("text", v), tema.Media
	}
	b := tema.Borde * z
	der, abajo := tema.Color("border", v), tema.Color("border", v)
	if filoDer {
		der = tema.Color("border-strong", v)
	}
	if filoAbajo {
		abajo = tema.Color("border-strong", v)
	}
	caja(cb.fondo, fondo, x, y, w, h)
	caja(cb.der, der, x+w-b, y, b, h)
	caja(cb.abajo, abajo, x, y+h-b, w, b)
	if texto == "" {
		cb.rotulo.Hide()
		return
	}
	esc := tema.Escalon{Tam: EscalonCabecera.Tam * z, Linea: EscalonCabecera.Linea * z}
	cb.rotulo.Text, cb.rotulo.Color, cb.rotulo.FontSource, cb.rotulo.TextSize = texto, tinta, cara, esc.Tam
	m := ui.Medir(texto, EscalonCabecera, peso).Width * z
	ui.ColocarTexto(cb.rotulo, esc, x+(w-b-m)/2, y)
	cb.rotulo.Show()
}

// pintarSeleccion pone el marco de la celda ancla —la sombra `inset` de 2 px— y el aro
// de foco de la rejilla, que en la web también es `inset`.
//
// El marco es de la CELDA en la web, así que se esconde con ella cuando pasa por debajo
// de lo inmovilizado; aquí se pinta después de todas las celdas y se recorta contra lo
// que la taparía.
func (rr *rejillaRenderer) pintarSeleccion(v tema.Variante, vw vista, t fyne.Size) {
	r := rr.r
	pos := fyne.NewPos(vw.x(r.sel.C), vw.y(r.sel.F))
	tam := fyne.NewSize(vw.ancho(), vw.alto())
	finX, finY := vw.finFijas()
	var minX, minY float32
	if r.sel.C >= vw.fc {
		minX = finX
	}
	if r.sel.F >= vw.ff {
		minY = finY
	}
	// Una fila que esconde el filtro no se ve, y su marco tampoco (`selVi` −1 en la web).
	marco(rr.marcoSel, tema.Color("primary-500", v), pos, tam, GrosorSeleccion*vw.z, minX, minY, !vw.filas.oculta(r.sel.F))
	marco(rr.marcoFoco, tema.Color("focus-ring", v), fyne.NewPos(0, 0), t, GrosorSeleccion, 0, 0, r.enfocado)
}

// colocarEditor pone la celda en edición encima de su casilla. El editor va por dentro
// del marco de selección y con el mismo `px-1` que la celda, para que la letra no
// salte al abrirlo.
func (rr *rejillaRenderer) colocarEditor(vw vista) {
	r := rr.r
	if !r.editando {
		r.ed.Objeto().Hide()
		rr.tinteCelda.pintar("", nil, nil, color.NRGBA{}, EscalonCelda, 0, 0, 0)
		return
	}
	x, y := vw.x(r.sel.C), vw.y(r.sel.F)
	alto := r.ed.AltoFuente()
	m := MargenCelda * vw.z
	r.ed.Colocar(fyne.NewPos(x+m, y+(vw.alto()-alto)/2), fyne.NewSize(vw.ancho()-2*m, alto))
	r.ed.Objeto().Show()

	// Y con la letra de la celda, como la web, que edita la celda misma: su familia, y la
	// negrita y la cursiva de su envoltorio. Una fórmula no las lleva: la web la abre con
	// su texto crudo, sin el `<b>` (`showEditSource`). El tinte de la fórmula va con la
	// misma letra, o sus trozos no caerían bajo el cursor.
	est := r.L.Estilo(r.sel.C, r.sel.F)
	negrita, cursiva, _ := r.L.Formato(r.sel.C, r.sel.F)
	if r.celdaFx {
		negrita, cursiva = false, false
	}
	cara, estilo := caraCelda(est.Fuente, negrita, cursiva)
	r.ed.Cara(cara, estilo)
	rr.tinteCelda.cara, rr.tinteCelda.estilo = cara, estilo

	// La celda abierta escribe con la tinta del CONTENIDO —la celda es papel blanco en
	// los dos temas—, y una fórmula, por trozos de colores con el texto del editor
	// transparente encima.
	pintado, crecio := false, false
	if r.celdaFx {
		esc := escalonCelda(r.L.Estilo(r.sel.C, r.sel.F), vw.z)
		pintado, crecio = rr.tinteCelda.pintar(r.ed.Texto(), formula.ExtractRefsCon(r.ed.Texto(), r.L.ResolverParaResaltar(r.sel)), ColorRef, Tinta,
			esc, x+m, y, vw.ancho()-2*m)
	} else {
		rr.tinteCelda.pintar("", nil, nil, color.NRGBA{}, EscalonCelda, 0, 0, 0)
	}
	if pintado {
		r.ed.Tinta(color.NRGBA{}, Tinta)
	} else {
		r.ed.Tinta(Tinta, Tinta)
	}
	if crecio {
		rr.recomponer()
	}
}

// ---- utilidades ----

// caja coloca y pinta un rectángulo y lo enseña.
func caja(rc *canvas.Rectangle, c color.NRGBA, x, y, w, h float32) {
	rc.FillColor = c
	rc.Move(fyne.NewPos(x, y))
	rc.Resize(fyne.NewSize(w, h))
	rc.Show()
}

// marco son los cuatro listones de una sombra `inset` de grosor g: un rectángulo hueco,
// que es lo que `ui.Caja` no puede dar —la suya es una caja RELLENA y taparía el fondo
// de la celda, que en una hoja de cálculo lleva el formato condicional—. Lo que cae a
// la izquierda de minX o por encima de minY no se pinta: está tapado.
func marco(bordes [4]*canvas.Rectangle, c color.NRGBA, pos fyne.Position, tam fyne.Size, g, minX, minY float32, ver bool) {
	cajas := [4]struct{ x, y, w, h float32 }{
		{pos.X, pos.Y, tam.Width, g},
		{pos.X, pos.Y + tam.Height - g, tam.Width, g},
		{pos.X, pos.Y, g, tam.Height},
		{pos.X + tam.Width - g, pos.Y, g, tam.Height},
	}
	for i, b := range bordes {
		x0, y0 := max(cajas[i].x, minX), max(cajas[i].y, minY)
		w, h := cajas[i].x+cajas[i].w-x0, cajas[i].y+cajas[i].h-y0
		if !ver || w <= 0 || h <= 0 {
			b.Hide()
			continue
		}
		caja(b, c, x0, y0, w, h)
	}
}

// medirCon mide con la cara, el tamaño y el estilo con los que se va a pintar: una
// negrita es más ancha, y medir con una y pintar con otra es cómo se come la última
// letra.
func medirCon(t *canvas.Text, s string) float32 {
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, t.TextSize, t.TextStyle, t.FontSource)
	return tam.Width
}

// caraCelda es la fuente de la celda: la familia del estilo (`FONT_STACKS`) con la
// negrita y la cursiva del envoltorio (`<b>` es `font-weight: bold` y `<i>` cursiva).
//
// Como el navegador, una familia que la suite no trae se pinta con la que haya en el
// equipo (Arial, Georgia, Times, Courier…); si no está, con la reserva del final de la
// pila: la monoespaciada para Courier y JetBrains Mono, Figtree para el resto.
func caraCelda(fuente string, negrita, cursiva bool) (fyne.Resource, fyne.TextStyle) {
	if fuente != "" {
		if r := caraDelSistema(fuente, negrita, cursiva); r != nil {
			return r, fyne.TextStyle{}
		}
	}
	if fuente == "Courier New" || fuente == "JetBrains Mono" {
		return nil, fyne.TextStyle{Monospace: true, Bold: negrita, Italic: cursiva}
	}
	switch {
	case negrita && cursiva:
		return tema.NegraCursiva, fyne.TextStyle{}
	case cursiva:
		return tema.Cursiva, fyne.TextStyle{}
	case negrita:
		return tema.Negra, fyne.TextStyle{}
	}
	return tema.Regular, fyne.TextStyle{}
}

// caraDelSistema es la cara del equipo para esa familia (ver tema.CaraDelSistema).
func caraDelSistema(familia string, negrita, cursiva bool) fyne.Resource {
	return tema.CaraDelSistema(familia, negrita, cursiva)
}

// fondoCelda es el fondo de la celda y su papel en la tabla que la cubra.
func (rr *rejillaRenderer) fondoCelda(c, f int, e Estilo, d *decoracion, enRango bool, primario color.NRGBA) (color.NRGBA, infoTabla, bool) {
	r := rr.r
	// El fondo: el relleno condicional, si no el del estilo, si no el de la tabla, si no
	// el papel; teñido al 12 % si está en el rango (`decor?.fill ?? cs?.fill ?? tableBg`).
	fondo := Papel
	info, enTabla := infoTablaEn(rr.tablas, c, f)
	if enTabla {
		et := estiloTablaDe(info.ct.t.Style)
		switch {
		case info.papel == papelCabecera:
			fondo, _ = tokens.ParseColor(et.Cabecera)
		case info.papel == papelTotal || info.banda:
			fondo, _ = tokens.ParseColor(et.Banda)
		}
	}
	if rell, ok := tokens.ParseColor(e.Relleno); ok {
		fondo = rell
	}
	if d != nil {
		if rell, ok := tokens.ParseColor(d.relleno); ok {
			fondo = rell
		}
	}
	if enRango {
		fondo = tokens.Aplanar(primario, MezclaRango, fondo)
	}
	// «Buscar»: amarillo en las celdas que casan y melocotón en la actual, que es la
	// seleccionada. Gana a todo, como en la web.
	if r.resaltes[Celda{C: c, F: f}] {
		fondo = editor.ResalteBuscar
		if c == r.sel.C && f == r.sel.F {
			fondo = editor.ResalteActivo
		}
	}
	return fondo, info, enTabla
}
