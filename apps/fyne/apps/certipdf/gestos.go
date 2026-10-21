// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

/*
Lo que el puntero HACE sobre una hoja con una herramienta en la mano: el `onDown`,
`onMove` y `onUp` de la `Hoja` de `PdfApp.tsx`.

La hoja solo recoge el gesto y lo enseña mientras dura —el trazo del lápiz, la caja,
la línea o la elipse que se está arrastrando, o la selección de texto—; qué se hace con
él al soltar lo decide la pantalla ([Pantalla.alGesto]), que es la que sabe de colores,
de permisos y del motor.

Lo que la web hace y aquí NO:

  - ponytail: sin rechazo de la palma ni presión del lápiz. Fyne no dice con qué se
    toca (ratón, dedo o lápiz: no hay `pointerType`) ni con cuánta fuerza, así que el
    grosor es el de la cinta, que es lo que la web hace con un ratón o un dedo. Se
    releva si Fyne expone el tipo de puntero.
*/

// Gesto es un arrastre sobre la hoja en PUNTOS del SVG de la página: la posición del
// puntero entre el zoom, que es `enSVG` de la web.
type Gesto struct {
	// Desde y Hasta son las dos esquinas del arrastre (la línea va de una a otra).
	Desde, Hasta [2]float64
	// Trazo son todos los puntos por los que pasó, en orden: lo que dibuja el lápiz.
	Trazo [][2]float64
	// Clics es 2 en un doble clic y 3 en un triple, que seleccionan la palabra y la
	// línea en Desde; 0 es un arrastre o un clic.
	Clics int
}

// esTrazo, esLinea y marcaTexto son los tres grupos de herramientas de `Hoja`: las que
// siguen la mano, las de dos puntos y las que marcan texto seleccionado.
func esTrazo(h Herramienta) bool { return h == Lapiz }

func esLinea(h Herramienta) bool { return h == Linea || h == Flecha || h == Medida }

func marcaTexto(h Herramienta) bool { return h == Resaltar || h == Subrayar || h == Tachar }

// seleccionaTexto son las herramientas con las que arrastrar sobre la hoja selecciona su
// texto: las de marcar y, en el escritorio, la mano, que en la web deja seleccionar y
// copiar el texto del SVG. En el móvil la mano desplaza, como el dedo en el navegador.
func seleccionaTexto(h Herramienta) bool {
	return marcaTexto(h) || (h == Mano && !ui.EsTactil())
}

// capaInteractiva es la de la web: con la mano no se dibuja, y con las de texto tampoco
// —marcan lo seleccionado—, pero aquí también las recoge la tapa: el texto de la página
// es un dibujo y no hay selección del sistema que pedir (ver `seleccion.go`).
func capaInteractiva(h Herramienta) bool { return h != Mano && !marcaTexto(h) }

// PonerHerramienta dice con qué se está trabajando y con qué tinta y grosor se enseña
// lo que se arrastra. Con la mano no hay tapa y los enlaces del documento vuelven.
func (h *Hoja) PonerHerramienta(hm Herramienta, tinta color.NRGBA, grosor float64) {
	h.mu.Lock()
	cambia := h.herramienta != hm
	h.herramienta, h.tinta, h.grosor = hm, tinta, grosor
	if cambia {
		h.gesto = nil
	}
	h.mu.Unlock()
	if cambia {
		h.seleccion = nil
		h.Refresh()
	}
}

// Herramienta es con la que se está trabajando sobre la hoja.
func (h *Hoja) Herramienta() Herramienta {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.herramienta
}

// PonerSeleccion enseña, sobre el texto, lo que se está seleccionando: los recuadros del
// `::selection` de la suite (`focus-ring` al 26 %), en cajas del SVG.
func (h *Hoja) PonerSeleccion(cajas []CajaSVG) {
	h.seleccion = cajas
	if h.r != nil {
		h.r.rehacerSeleccion()
		h.r.Layout(h.Size())
		canvas.Refresh(h)
	}
}

// Arrastrar es un movimiento con el botón abajo, en píxeles de la hoja. La primera vez
// empieza el gesto donde se pulsó (la posición menos lo ya arrastrado).
func (h *Hoja) Arrastrar(pos fyne.Position, delta fyne.Delta) {
	z := h.zoom
	if z <= 0 {
		return
	}
	x, y := float64(pos.X)/z, float64(pos.Y)/z
	h.mu.Lock()
	if h.gesto == nil {
		x0, y0 := float64(pos.X-delta.DX)/z, float64(pos.Y-delta.DY)/z
		h.gesto = &Gesto{Desde: [2]float64{x0, y0}, Hasta: [2]float64{x0, y0}}
		if esTrazo(h.herramienta) {
			h.gesto.Trazo = [][2]float64{{x0, y0}}
		}
	}
	h.gesto.Hasta = [2]float64{x, y}
	if esTrazo(h.herramienta) {
		h.gesto.Trazo = append(h.gesto.Trazo, [2]float64{x, y})
	}
	g := copiaGesto(h.gesto)
	h.mu.Unlock()
	h.refrescarPrevia()
	if h.AlGesto != nil {
		h.AlGesto(h, g, false)
	}
}

// Soltar acaba el gesto y se lo entrega a quien decide qué hacer con él.
func (h *Hoja) Soltar() {
	h.mu.Lock()
	g := h.gesto
	h.gesto = nil
	h.mu.Unlock()
	h.refrescarPrevia()
	if g != nil && h.AlGesto != nil {
		h.AlGesto(h, copiaGesto(g), true)
	}
}

func copiaGesto(g *Gesto) Gesto {
	c := *g
	c.Trazo = append([][2]float64(nil), g.Trazo...)
	return c
}

// cajaPrevia es lo que ocupa lo que se enseña del gesto, en puntos, con el grosor de
// margen: el raster de la previa mide eso y no la página entera, que a ×2 son siete
// megapíxeles que subir a la tarjeta en cada movimiento del ratón.
func cajaPrevia(g Gesto, hm Herramienta, grosor float64) (x, y, w, h float64) {
	minX, minY := math.Min(g.Desde[0], g.Hasta[0]), math.Min(g.Desde[1], g.Hasta[1])
	maxX, maxY := math.Max(g.Desde[0], g.Hasta[0]), math.Max(g.Desde[1], g.Hasta[1])
	if esTrazo(hm) {
		for _, p := range g.Trazo {
			minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
			maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
		}
	}
	m := grosor/2 + 1
	return minX - m, minY - m, maxX - minX + 2*m, maxY - minY + 2*m
}

// pintarPrevia es `PrevisualizacionCaja` (y la `<polyline>` del lápiz): lo que se está
// arrastrando, a la tinta y el grosor de la cinta, en píxeles de dispositivo.
func pintarPrevia(g Gesto, hm Herramienta, tinta color.NRGBA, grosor float64, w, h int) image.Image {
	ox, oy, bw, _ := cajaPrevia(g, hm, grosor)
	p := nuevoPincel(w, h, float64(w)/bw, ox, oy)
	x, y, cw, ch := cajaDe(g.Desde[0], g.Desde[1], g.Hasta[0], g.Hasta[1])
	switch {
	case esTrazo(hm):
		p.linea(g.Trazo, tinta, grosor)
	case esLinea(hm):
		p.linea([][2]float64{g.Desde, g.Hasta}, tinta, grosor)
	case hm == Elipse:
		p.elipse(x, y, cw, ch, tinta, grosor)
	case hm == Censura:
		// `fill="#111111"` y el contorno igual: se ve tal y como quedará.
		p.caja(x, y, cw, ch, colorCensura, colorCensura, grosor, nil)
	default:
		p.caja(x, y, cw, ch, nil, tinta, grosor, nil)
	}
	return p.img
}

// colorCensura es el `#111111` literal de la web: el color que tendrá la barra.
var colorCensura = color.NRGBA{R: 17, G: 17, B: 17, A: 255}

// refrescarPrevia vuelve a pintar lo que se arrastra sin tocar el dibujo de la página.
func (h *Hoja) refrescarPrevia() {
	if h.r == nil {
		return
	}
	h.r.colocarPrevia()
	h.r.previa.Refresh()
	canvas.Refresh(h)
}

// generarPrevia es el generador del raster de la previa.
func (h *Hoja) generarPrevia(w, alto int) image.Image {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gesto == nil || seleccionaTexto(h.herramienta) || w <= 0 || alto <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return pintarPrevia(*h.gesto, h.herramienta, h.tinta, h.grosor, w, alto)
}

// tapaHoja es la capa que recoge el puntero cuando hay una herramienta: el `<svg>` con
// `cursor-crosshair` y `touch-action: none` de la web. Sin ella, arrastrar sobre la hoja
// desplazaría el documento en vez de dibujar.
type tapaHoja struct {
	widget.BaseWidget
	h *Hoja
	// doble es cuándo y dónde fue el último doble clic: un clic justo después, en el
	// mismo sitio, es el triple.
	doble      time.Time
	dondeDoble fyne.Position
}

var (
	_ fyne.Draggable      = (*tapaHoja)(nil)
	_ fyne.Tappable       = (*tapaHoja)(nil)
	_ fyne.DoubleTappable = (*tapaHoja)(nil)
	_ desktop.Cursorable  = (*tapaHoja)(nil)
)

func nuevaTapa(h *Hoja) *tapaHoja {
	t := &tapaHoja{h: h}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapaHoja) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// Cursor es la cruz de dibujar (`cursor-crosshair`); con las de texto y la mano, el del
// texto.
//
// ponytail: con la mano la web solo lo pone encima del texto; aquí, en toda la hoja.
func (t *tapaHoja) Cursor() desktop.Cursor {
	if seleccionaTexto(t.h.Herramienta()) {
		return desktop.TextCursor
	}
	return desktop.CrosshairCursor
}

func (t *tapaHoja) Dragged(e *fyne.DragEvent) { t.h.Arrastrar(e.Position, e.Dragged) }

func (t *tapaHoja) DragEnd() { t.h.Soltar() }

// Tapped es un clic sin arrastrar: un gesto de un punto, que con la mano quita la
// selección, como un clic en el texto del navegador. Las demás no hacen nada con él.
func (t *tapaHoja) Tapped(e *fyne.PointEvent) {
	clics := 0
	if d := e.Position.Subtract(t.dondeDoble); time.Since(t.doble) < plazoTriple && d.X*d.X+d.Y*d.Y < 16 {
		clics = 3
	}
	t.doble = time.Time{}
	t.clic(e.Position, clics)
}

// DoubleTapped es el doble clic: selecciona la palabra, como el del navegador.
//
// ponytail: con el doble clic atendido, Fyne espera su plazo antes de dar el clic suelto,
// así que quitar la selección con un clic tarda ese rato en verse.
func (t *tapaHoja) DoubleTapped(e *fyne.PointEvent) {
	t.doble, t.dondeDoble = time.Now(), e.Position
	t.clic(e.Position, 2)
}

// plazoTriple es lo que puede tardar el tercer clic: el suelto llega ya con el retraso
// del doble.
const plazoTriple = 800 * time.Millisecond

func (t *tapaHoja) clic(pos fyne.Position, clics int) {
	if z := t.h.zoom; z > 0 && t.h.AlGesto != nil {
		q := [2]float64{float64(pos.X) / z, float64(pos.Y) / z}
		t.h.AlGesto(t.h, Gesto{Desde: q, Hasta: q, Clics: clics}, true)
	}
}
