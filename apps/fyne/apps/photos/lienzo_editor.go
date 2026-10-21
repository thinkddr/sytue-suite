// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LienzoEditor es el `<canvas>` del editor con la capa de recorte encima: enseña la
// fuente (la previa del ORIGINAL con la geometría de la receta ya aplicada) con el color
// de la receta, encajada sin deformarse y sin crecer más que su tamaño natural
// (`max-h-full max-w-full object-contain`), y recibe el arrastre del recorte.
//
// El color se calcula EN CADA PINTADO sobre la fuente ya reducida a los píxeles de
// pantalla del hueco: en la web lo hace WebGL sobre la previa entera; aquí la CPU, pero
// sobre los píxeles que se ven, que es lo que deja arrastrar un deslizador sin tirones.
// La reducción se guarda por tamaño y solo se rehace al cambiar la geometría o el hueco.
type LienzoEditor struct {
	widget.BaseWidget

	mu      sync.Mutex
	fuente  *image.NRGBA
	receta  api.Receta
	fondo   FondoLienzo
	cacheW  int
	cacheH  int
	cacheDe *image.NRGBA
	cache   *image.NRGBA
	// pintada es la última foto que salió coloreada, para el desenfoque de la barra.
	pintada *image.NRGBA

	// Recortando enciende la capa de recorte: el cursor en cruz y el arrastre.
	Recortando bool
	// Marco es el recorte en curso, en fracciones de la foto; nil, ninguno.
	Marco *Marco
	// Proporcion fija la del marco (0, libre).
	Proporcion float64
	// AlMarco avisa de que el marco ha cambiado, para encender «Aplicar».
	AlMarco func()

	ancla  [2]float64
	caja   fyne.Position
	tamPx  fyne.Size
	activo bool
}

var (
	_ fyne.Widget        = (*LienzoEditor)(nil)
	_ fyne.Draggable     = (*LienzoEditor)(nil)
	_ desktop.Cursorable = (*LienzoEditor)(nil)
)

// NuevoLienzoEditor crea el lienzo vacío: la fuente llega con Poner.
func NuevoLienzoEditor() *LienzoEditor {
	l := &LienzoEditor{fondo: FondoCrema}
	l.ExtendBaseWidget(l)
	return l
}

// Poner cambia la fuente, la receta de color y el fondo, y repinta.
func (l *LienzoEditor) Poner(fuente *image.NRGBA, r api.Receta, fondo FondoLienzo) {
	l.mu.Lock()
	l.fuente, l.receta, l.fondo = fuente, r, fondo
	l.mu.Unlock()
	l.Refresh()
}

// Proporcionada es la proporción de la fuente (ancho, alto), o ceros sin fuente.
func (l *LienzoEditor) Proporcionada() (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fuente == nil {
		return 0, 0
	}
	return l.fuente.Rect.Dx(), l.fuente.Rect.Dy()
}

// Cursor es `cursor-crosshair` mientras se recorta.
func (l *LienzoEditor) Cursor() desktop.Cursor {
	if l.Recortando {
		return desktop.CrosshairCursor
	}
	return desktop.DefaultCursor
}

// fraccion es el punto del puntero en fracciones de la FOTO, acotado a 0..1: es el
// `fraccion(e)` de la web, que mide contra el rectángulo del `<canvas>`.
func (l *LienzoEditor) fraccion(p fyne.Position) (float64, float64) {
	if l.tamPx.Width <= 0 || l.tamPx.Height <= 0 {
		return 0, 0
	}
	x := float64((p.X - l.caja.X) / l.tamPx.Width)
	y := float64((p.Y - l.caja.Y) / l.tamPx.Height)
	return min(1, max(0, x)), min(1, max(0, y))
}

// Dragged es el `pointerdown` + `pointermove` del recorte: el primero fija el ancla.
func (l *LienzoEditor) Dragged(e *fyne.DragEvent) {
	if !l.Recortando {
		return
	}
	px, py := l.fraccion(e.Position)
	if !l.activo {
		l.activo = true
		l.ancla[0], l.ancla[1] = l.fraccion(e.Position.Subtract(e.Dragged))
	}
	ax, ay := l.ancla[0], l.ancla[1]
	m := Marco{X: min(ax, px), Y: min(ay, py), W: math.Abs(px - ax), H: math.Abs(py - ay)}
	if l.Proporcion > 0 {
		m = conProporcion(m, l.Proporcion, float64(l.tamPx.Width), float64(l.tamPx.Height), ax, ay)
	}
	l.Marco = &m
	l.Refresh()
	if l.AlMarco != nil {
		l.AlMarco()
	}
}

// DragEnd suelta el ancla; el marco se queda hasta que se aplique o se cancele.
func (l *LienzoEditor) DragEnd() { l.activo = false }

// AjustarProporcion es lo que hace pulsar una proporción con un marco ya dibujado: se
// rehace desde su esquina de arriba a la izquierda.
func (l *LienzoEditor) AjustarProporcion(p float64) {
	l.Proporcion = p
	if p > 0 && l.Marco != nil && !l.activo {
		m := conProporcion(*l.Marco, p, float64(l.tamPx.Width), float64(l.tamPx.Height), l.Marco.X, l.Marco.Y)
		l.Marco = &m
		l.Refresh()
	}
}

// CreateRenderer arma el fondo del lienzo, la foto, la sombra y la capa de recorte.
func (l *LienzoEditor) CreateRenderer() fyne.WidgetRenderer {
	r := &lienzoEditorRenderer{l: l, sombra: ui.NuevaSombra(tema.SombraSM, 0)}
	r.fondo = canvas.NewRectangle(color.NRGBA{})
	r.damero = canvas.NewRaster(func(w, h int) image.Image {
		css := math.Float32frombits(r.anchoCSS.Load())
		if css <= 0 {
			return image.NewNRGBA(image.Rect(0, 0, w, h))
		}
		return pintarDameroCon(w, h, 8*float32(w)/css)
	})
	r.foto = canvas.NewRaster(r.generar)
	negro := color.NRGBA{A: 140} // el negro al 55 % de la web
	for i := range r.velo {
		r.velo[i] = canvas.NewRectangle(negro)
	}
	r.borde = canvas.NewRectangle(color.NRGBA{})
	r.borde.StrokeColor, r.borde.StrokeWidth = color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 2
	r.Refresh()
	return r
}

type lienzoEditorRenderer struct {
	l        *LienzoEditor
	sombra   *ui.Sombra
	fondo    *canvas.Rectangle
	damero   *canvas.Raster
	foto     *canvas.Raster
	velo     [4]*canvas.Rectangle
	borde    *canvas.Rectangle
	anchoCSS atomic.Uint32 // lo escribe quien coloca y lo lee quien pinta
}

func (r *lienzoEditorRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *lienzoEditorRenderer) Destroy()           {}

func (r *lienzoEditorRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.sombra.Objeto(), r.fondo, r.damero, r.foto, r.velo[0], r.velo[1], r.velo[2], r.velo[3], r.borde}
}

// generar es la foto a los píxeles de pantalla que se le piden, con su color.
func (r *lienzoEditorRenderer) generar(w, h int) image.Image {
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fuente == nil || w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	}
	if l.cache == nil || l.cacheW != w || l.cacheH != h || l.cacheDe != l.fuente {
		red := image.NewNRGBA(image.Rect(0, 0, w, h))
		if l.fuente.Rect.Dx() == w && l.fuente.Rect.Dy() == h {
			draw.Draw(red, red.Rect, l.fuente, image.Point{}, draw.Src)
		} else {
			escalador(l.fuente.Rect, w, h).Scale(red, red.Rect, l.fuente, l.fuente.Rect, draw.Src, nil)
		}
		l.cache, l.cacheW, l.cacheH, l.cacheDe = red, w, h, l.fuente
	}
	l.pintada = colorear(l.cache, l.receta)
	return l.pintada
}

// debajo pinta lo que el lienzo enseña bajo un trozo de él —el `bg-subtle` del hueco y,
// en la caja de la foto, su fondo y la foto coloreada—: lo que desenfoca la barra
// flotante de fondos.
func (l *LienzoEditor) debajo(bg color.NRGBA) func(*image.NRGBA, fyne.Position, float32) {
	return func(dst *image.NRGBA, origen fyne.Position, escala float32) {
		rellenar(dst, bg)
		l.mu.Lock()
		pos, tam, m, fondo := l.caja, l.tamPx, l.pintada, l.fondo
		l.mu.Unlock()
		var img image.Image
		if m != nil {
			img = m
		}
		pintarFoto(dst, origen, escala, pos, tam, img, fondo.colorFondo())
	}
}

// Layout encaja la foto: `object-contain` y nunca más grande que su tamaño natural, en
// píxeles de CSS, que es lo que mide un `<canvas>` de N píxeles sin estilo de tamaño.
func (r *lienzoEditorRenderer) Layout(t fyne.Size) {
	l := r.l
	fw, fh := l.Proporcionada()
	var pos fyne.Position
	var tam fyne.Size
	if fw > 0 && fh > 0 {
		k := min(1, t.Width/float32(fw), t.Height/float32(fh))
		tam = fyne.NewSize(float32(fw)*k, float32(fh)*k)
		pos = fyne.NewPos((t.Width-tam.Width)/2, (t.Height-tam.Height)/2)
	}
	l.mu.Lock()
	l.caja, l.tamPx = pos, tam
	l.mu.Unlock()
	r.anchoCSS.Store(math.Float32bits(tam.Width))
	for _, o := range []fyne.CanvasObject{r.fondo, r.damero, r.foto} {
		o.Move(pos)
		o.Resize(tam)
	}
	r.sombra.Ajustar(pos, tam, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))

	// La capa de recorte: el `shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]` es un velo sobre
	// todo lo que NO es el marco, y el marco lleva `border-2 border-white`. Como en la
	// web, se sale 9999 px del lienzo: ningún antepasado recorta y oscurece también la
	// cabecera y el panel, porque el hueco del lienzo se pinta el último (editorLayout).
	// Fyne no recorta lo que un renderer pinta fuera de su caja, y un rectángulo no
	// recibe pulsaciones, así que el panel sigue funcionando debajo, como en la web.
	ver := l.Recortando && l.Marco != nil
	for _, v := range r.velo {
		v.Hidden = !ver
	}
	r.borde.Hidden = !ver
	if !ver {
		return
	}
	m := *l.Marco
	x0, y0 := pos.X+float32(m.X)*tam.Width, pos.Y+float32(m.Y)*tam.Height
	x1, y1 := x0+float32(m.W)*tam.Width, y0+float32(m.H)*tam.Height
	colocar := func(o fyne.CanvasObject, x, y, w, h float32) {
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(max(w, 0), max(h, 0)))
	}
	const fuera = 9999
	colocar(r.velo[0], -fuera, -fuera, t.Width+2*fuera, y0+fuera)
	colocar(r.velo[1], -fuera, y1, t.Width+2*fuera, t.Height-y1+fuera)
	colocar(r.velo[2], -fuera, y0, x0+fuera, y1-y0)
	colocar(r.velo[3], x1, y0, t.Width-x1+fuera, y1-y0)
	// El trazo de Fyne va CENTRADO en el borde (regla 4 de ui.go); el `border-2` de CSS,
	// por dentro: se mete un píxel para que los dos caigan en la caja.
	colocar(r.borde, x0+1, y0+1, x1-x0-2, y1-y0-2)
}

// Refresh repinta con el fondo elegido y el color de la receta.
func (r *lienzoEditorRenderer) Refresh() {
	l := r.l
	l.mu.Lock()
	fondo := l.fondo
	l.mu.Unlock()
	r.fondo.FillColor = fondo.colorFondo()
	r.damero.Hidden = fondo != FondoDamero
	r.Layout(l.Size())
	for _, o := range r.Objects() {
		o.Refresh()
	}
}
