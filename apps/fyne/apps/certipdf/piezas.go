// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"math"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

/*
Piezas pequeñas que los paneles de `Paneles.tsx` escriben a mano: el `<details>` con su
triángulo, el enlace-botón subrayado, la casilla con su rótulo, el aviso ámbar, el texto
tachado y la caja de campo. Son de esta app porque son sus clases, no componentes de la
suite (ver [Accion]).
*/

// campoPanel son las medidas de un `<input>` de los paneles: `border-border bg-surface
// rounded-md border px-2 py-1.5 text-xs` (26,5) o, con `py-1`, 23.
func campoPanel(py float32) ui.CampoPropio {
	return ui.CampoPropio{MargenX: tema.Espacio(2), MargenY: py, Texto: tema.TextoXS, Borde: "border"}
}

// nuevoCampoPanel es la entrada de un panel, al ancho que le den.
func nuevoCampoPanel(marca string, py, radio, ancho float32) *ui.Entrada {
	e := ui.NuevaEntrada(marca).ConPropia(campoPanel(py))
	e.Radio, e.Ancho = radio, ancho
	return e
}

// enlaceBoton es un `<button class="text-2xs rounded underline">` con su tinta: los
// «Ir», «Eliminar» y «Responder» de los comentarios y la página de una coincidencia.
func enlaceBoton(texto, tinta string, alPulsar func()) *Accion {
	a := NuevaAccion(texto, alPulsar)
	a.Escalon, a.Peso, a.PX, a.PY = tema.Texto2XS, ui.Normal, 0, 0
	a.Radio, a.Tinta, a.FondoEncima, a.Subrayado = 0.25*tema.Rem, tinta, "", true
	return a
}

// botonPanel es `border-border text-text rounded-md border px-2 py-1.5 text-xs` (o
// `text-2xs py-1`), el botón secundario de los paneles, sin fondo al pasar por encima.
func botonPanel(texto string, esc tema.Escalon, py float32, alPulsar func()) *Accion {
	a := NuevaAccion(texto, alPulsar).ConBorde()
	a.Escalon, a.Peso, a.PY, a.FondoEncima = esc, ui.Normal, py, ""
	return a
}

// botonPrimario es `bg-primary text-[var(--sytue-on-primary)] hover:bg-primary-hover
// rounded-md font-medium`, con el relleno y la letra que diga cada sitio.
func botonPrimario(texto string, esc tema.Escalon, px, py float32, alPulsar func()) *Accion {
	a := NuevaAccion(texto, alPulsar)
	a.Escalon, a.PX, a.PY = esc, px, py
	a.Fondo, a.FondoEncima, a.Tinta = "primary", "primary-hover", "on-primary"
	return a
}

// avisoAmbar es `text-warning-text bg-warning-bg text-2xs rounded p-2`: lo que un panel
// tiene que decir ANTES de que alguien se lleve una sorpresa (un escaneo, texto que va a
// quedar apretado).
func avisoAmbar(texto string, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := tema.Espacio(2)
	fondo := canvas.NewRectangle(tema.Color("warning-bg", v))
	fondo.CornerRadius = 0.25 * tema.Rem
	p := ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "warning-text", v).A(ancho - 2*m)
	return container.NewStack(fondo, container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, p))
}

// ---- El triángulo de un `<details>` ----

// triangulo es el marcador `disclosure-closed`/`disclosure-open` que Chromium pinta en un
// `<summary>`. Medido en la web con `text-xs` (10,5): 5,5 de ancho por 7 de alto
// apuntando a la derecha, centrado en la línea, y el texto empieza 11,11 más allá.
type triangulo struct {
	widget.BaseWidget
	abierto bool
	tinta   string
}

// Las medidas del marcador, en fracciones del cuerpo de letra (10,5).
const (
	altoTriangulo  = 7.0 / 10.5
	anchoTriangulo = 5.5 / 10.5
	textoTrasMarca = 11.11 / 10.5
)

func (t *triangulo) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(tema.TextoXS.Tam*textoTrasMarca, tema.TextoXS.Linea)
}

func (t *triangulo) CreateRenderer() fyne.WidgetRenderer {
	r := &trianguloRenderer{t: t}
	r.raster = canvas.NewRasterWithPixels(r.pixel)
	return r
}

type trianguloRenderer struct {
	t      *triangulo
	raster *canvas.Raster
	tinta  color.NRGBA
}

func (r *trianguloRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *trianguloRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.raster} }
func (r *trianguloRenderer) Destroy()                     {}

func (r *trianguloRenderer) Layout(s fyne.Size) {
	// El triángulo en su caja: el lado largo a lo alto si está cerrado.
	w, h := tema.TextoXS.Tam*anchoTriangulo, tema.TextoXS.Tam*altoTriangulo
	if r.t.abierto {
		w, h = h, w
	}
	r.raster.Move(fyne.NewPos(0, (s.Height-h)/2))
	r.raster.Resize(fyne.NewSize(w, h))
}

func (r *trianguloRenderer) Refresh() {
	r.tinta = tema.Color(r.t.tinta, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.Layout(r.t.Size())
	r.raster.Refresh()
}

// pixel pinta el triángulo lleno con el borde suavizado: cobertura muestreada 4×4.
func (r *trianguloRenderer) pixel(x, y, w, h int) color.Color {
	var dentro int
	for sy := 0; sy < 4; sy++ {
		for sx := 0; sx < 4; sx++ {
			u := (float64(x) + (float64(sx)+0.5)/4) / float64(w)
			v := (float64(y) + (float64(sy)+0.5)/4) / float64(h)
			if r.t.abierto {
				u, v = v, u
			}
			// Cerrado apunta a la derecha: a la altura v, llega hasta 1-|2v-1|.
			if u <= 1-math.Abs(2*v-1) {
				dentro++
			}
		}
	}
	c := r.tinta
	c.A = uint8(uint32(c.A) * uint32(dentro) / 16) //nolint:gosec // G115: dentro va de 0 a 16, así que cabe en un byte
	return c
}

// detalles es `<details class="border-border rounded-md border p-2">` con su
// `<summary class="text-text cursor-pointer text-xs">`: el resumen siempre, y el cuerpo
// debajo con `mt-2` cuando está abierto.
type detalles struct {
	ui.Pulsable
	resumen   string
	abierto   bool
	marca     *triangulo
	AlCambiar func(abierto bool)
	// peso es el del resumen: normal, o `font-medium` en el de la firma.
	peso ui.Peso
}

func nuevoDetalles(resumen string, abierto bool, alCambiar func(bool)) *detalles {
	d := &detalles{
		resumen: resumen, abierto: abierto, AlCambiar: alCambiar,
		marca: &triangulo{abierto: abierto, tinta: "text"},
	}
	d.marca.ExtendBaseWidget(d.marca)
	d.Nombre = resumen
	d.AlPulsar = func() {
		d.abierto = !d.abierto
		d.marca.abierto = d.abierto
		d.marca.Refresh()
		if d.AlCambiar != nil {
			d.AlCambiar(d.abierto)
		}
	}
	d.ExtendBaseWidget(d)
	return d
}

func (d *detalles) MinSize() fyne.Size {
	d.ExtendBaseWidget(d)
	return fyne.NewSize(d.marca.MinSize().Width+ui.Medir(d.resumen, tema.TextoXS, d.peso).Width, tema.TextoXS.Linea)
}

func (d *detalles) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &detallesRenderer{
		d: d, aro: ui.NuevoAro(0.25*tema.Rem, ui.SinHueco, v).Abierto(),
		rotulo: ui.Texto(d.resumen, tema.TextoXS, d.peso, tema.Color("text", v)),
	}
	r.Refresh()
	return r
}

type detallesRenderer struct {
	d      *detalles
	aro    *ui.Aro
	rotulo *canvas.Text
}

func (r *detallesRenderer) MinSize() fyne.Size { return r.d.MinSize() }
func (r *detallesRenderer) Destroy()           {}
func (r *detallesRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.d.marca, r.rotulo)
}

func (r *detallesRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	m := r.d.marca.MinSize()
	r.d.marca.Move(fyne.NewPos(0, 0))
	r.d.marca.Resize(m)
	ui.ColocarTexto(r.rotulo, tema.TextoXS, m.Width, 0)
}

func (r *detallesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Color = tema.Color("text", v)
	r.rotulo.Refresh()
	r.d.marca.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.d.Enfocado())
	r.Layout(r.d.Size())
}

// ---- Texto tachado ----

// tachado es un rótulo con `line-through`: Fyne no tiene ese estilo de letra, así que la
// raya es un rectángulo de 1 px a la altura del centro de la x.
type tachado struct {
	widget.BaseWidget
	texto string
	esc   tema.Escalon
	token string
	ancho float32
}

func nuevoTachado(texto string, esc tema.Escalon, token string, ancho float32) *tachado {
	t := &tachado{texto: texto, esc: esc, token: token, ancho: ancho}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tachado) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(t.ancho, t.esc.Linea)
}

func (t *tachado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tachadoRenderer{
		t: t, texto: ui.Texto(t.texto, t.esc, ui.Normal, tema.Color(t.token, v)),
		raya: canvas.NewRectangle(tema.Color(t.token, v)),
	}
	r.Layout(t.MinSize())
	return r
}

type tachadoRenderer struct {
	t     *tachado
	texto *canvas.Text
	raya  *canvas.Rectangle
}

func (r *tachadoRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *tachadoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.texto, r.raya} }
func (r *tachadoRenderer) Destroy()                     {}
func (r *tachadoRenderer) Refresh()                     { r.Layout(r.t.Size()) }

func (r *tachadoRenderer) Layout(s fyne.Size) {
	w := ui.Truncar(r.texto, r.t.texto, r.t.esc, ui.Normal, s.Width)
	ui.ColocarTexto(r.texto, r.t.esc, 0, 0)
	// La raya de `line-through` de Figtree va a media altura de la x: en CSS, la
	// `strikeout position` de la fuente, ≈0,3 em sobre la línea base.
	base := (r.t.esc.Linea+r.t.esc.Tam*0.72)/2 - r.t.esc.Tam*0.3
	r.raya.Move(fyne.NewPos(0, base))
	r.raya.Resize(fyne.NewSize(w, max(r.t.esc.Tam*0.05, 1)))
}

// filaCentrada es `flex items-center gap-*`: los hijos de izquierda a derecha, cada uno
// a su tamaño y centrado en el alto del más alto. `ui.Fila` los apoya arriba.
type filaCentrada struct{ hueco float32 }

func (f filaCentrada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Fila{Hueco: f.hueco}.MinSize(objs)
}

func (f filaCentrada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	alto := f.MinSize(objs).Height
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (alto-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.hueco
	}
}

// ---- Cifras tabulares ----

// avanceTabular es el ancho de una cifra con `tabular-nums` en Figtree, en em: 0,623,
// medido en Chromium e igual para las diez. Las cifras de serie de Figtree son
// PROPORCIONALES (el «1» mide 0,413 y el «0», 0,641), así que un rango como «1-2» con
// `tabular-nums` mide 2,6 px más a `text-2xs` que sin él.
const avanceTabular = 0.623

// medirTabular es lo que ocupa un texto con `tabular-nums`: cada cifra en su celda y lo
// demás a su ancho.
func medirTabular(s string, e tema.Escalon, p ui.Peso) float32 {
	var w float32
	for _, r := range s {
		if unicode.IsDigit(r) {
			w += avanceTabular * e.Tam
		} else {
			w += ui.Medir(string(r), e, p).Width
		}
	}
	return w
}

// nuevoRotuloTabular es un rótulo con `tabular-nums` montado en su caja, para ir en una
// fila como cualquier otro rótulo.
func nuevoRotuloTabular(s string, e tema.Escalon, p ui.Peso, token string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := nuevoTabular(s, e, p, tema.Color(token, v))
	t.Colocar(0, 0)
	return container.New(fijo{fyne.NewSize(t.Ancho(), e.Linea)}, t.Objetos()...)
}

// tabular es un texto con `tabular-nums`: las cifras, centradas cada una en su celda de
// ancho fijo, como las dibuja la variante tabular de la fuente. Se tiñe y se coloca
// desde el renderer que lo lleva (el número bajo la hoja, el de la miniatura).
// ponytail: el «1» sale con la forma proporcional, centrado; la tabular de Figtree es
// el mismo dibujo, así que solo cambia el hueco.
type tabular struct {
	s      string
	e      tema.Escalon
	p      ui.Peso
	textos []*canvas.Text
}

func nuevoTabular(s string, e tema.Escalon, p ui.Peso, c color.Color) *tabular {
	t := &tabular{s: s, e: e, p: p}
	for _, r := range s {
		t.textos = append(t.textos, ui.Texto(string(r), e, p, c))
	}
	return t
}

// Ancho es [medirTabular].
func (t *tabular) Ancho() float32 { return medirTabular(t.s, t.e, t.p) }

// Objetos son los caracteres, uno por texto.
func (t *tabular) Objetos() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(t.textos))
	for i, x := range t.textos {
		objs[i] = x
	}
	return objs
}

// Colocar pone el texto con su esquina en (x, y).
func (t *tabular) Colocar(x, y float32) {
	for i, r := range []rune(t.s) {
		w := ui.Medir(string(r), t.e, t.p).Width
		celda := w
		if unicode.IsDigit(r) {
			celda = avanceTabular * t.e.Tam
		}
		ui.ColocarTexto(t.textos[i], t.e, x+(celda-w)/2, y)
		x += celda
	}
}

// Tenir cambia el color (el tema ha cambiado).
func (t *tabular) Tenir(c color.Color) {
	for _, x := range t.textos {
		x.Color = c
		x.Refresh()
	}
}
