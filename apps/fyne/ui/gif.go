// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/draw"
	"image/gif"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// Gif pinta un GIF ANIMADO, que `canvas.Image` no sabe hacer (se queda en el primer
// fotograma; animarlo pide fyne-x, que no está en el módulo). Con `image/gif` de la
// stdlib basta: los fotogramas se componen UNA vez al crearlo —con su disposición— y
// luego solo se cambia cuál se enseña.
//
// Lo que cuesta, y cuándo no:
//
//   - Fuera de la pantalla no avanza: si el widget no está en un lienzo o está oculto,
//     el reloj se salta el fotograma sin tocar la textura. Al destruirse el renderer, el
//     reloj se para del todo.
//   - Con «menos movimiento» (`SinMovimiento` o el ajuste del sistema) enseña el primer
//     fotograma y no arranca reloj ninguno: es el `prefers-reduced-motion` de la web, que
//     el navegador aplica a los GIF igual que a las animaciones CSS en Safari y Firefox.
//
// ponytail: fotogramas compuestos en memoria (ancho×alto×4 por fotograma). Para los GIF
// del catálogo —unos cientos de KiB— es nada; uno de 500 fotogramas a 1080p pediría
// descomponer al vuelo.
type Gif struct {
	widget.BaseWidget

	FillMode canvas.ImageFill

	fotogramas []image.Image
	pausas     []time.Duration
	vueltas    int // LoopCount de image/gif: 0 sin fin, -1 una pasada, n → n+1

	mu     sync.Mutex
	actual int
	parar  chan struct{}
}

var _ fyne.Widget = (*Gif)(nil)

// NuevoGif crea el widget a partir de un GIF ya descodificado con `gif.DecodeAll`.
func NuevoGif(g *gif.GIF) *Gif {
	w := &Gif{FillMode: canvas.ImageFillContain, vueltas: g.LoopCount}
	w.fotogramas, w.pausas = ComponerGif(g)
	w.ExtendBaseWidget(w)
	return w
}

// Fotograma es el que se enseña ahora, para los guardianes.
func (w *Gif) Fotograma() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.actual
}

// PausaMinimaGif es lo que los navegadores aplican a un retardo de 0 o 1 centésimas:
// 100 ms. Sin ella, un GIF con `delay 0` giraría a lo que dé la CPU.
const PausaMinimaGif = 100 * time.Millisecond

// ComponerGif devuelve cada fotograma ya compuesto sobre los anteriores, según su
// disposición, y cuánto dura cada uno.
func ComponerGif(g *gif.GIF) ([]image.Image, []time.Duration) {
	lienzo := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if lienzo.Empty() && len(g.Image) > 0 {
		lienzo = g.Image[0].Bounds()
	}
	actual := image.NewNRGBA(lienzo)
	fotogramas := make([]image.Image, 0, len(g.Image))
	pausas := make([]time.Duration, 0, len(g.Image))
	for i, f := range g.Image {
		var previo *image.NRGBA
		disp := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disp = g.Disposal[i]
		}
		if disp == gif.DisposalPrevious {
			previo = image.NewNRGBA(lienzo)
			copy(previo.Pix, actual.Pix)
		}
		draw.Draw(actual, f.Bounds(), f, f.Bounds().Min, draw.Over)
		foto := image.NewNRGBA(lienzo)
		copy(foto.Pix, actual.Pix)
		fotogramas = append(fotogramas, foto)

		pausa := PausaMinimaGif
		if i < len(g.Delay) && g.Delay[i] > 1 {
			pausa = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}
		pausas = append(pausas, pausa)

		switch disp {
		case gif.DisposalBackground:
			draw.Draw(actual, f.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			actual = previo
		}
	}
	return fotogramas, pausas
}

// animado dice si hay algo que mover.
func (w *Gif) animado() bool {
	return len(w.fotogramas) > 1 && !SinMovimiento && !sistema.MenosMovimiento()
}

// CreateRenderer pinta el primer fotograma y arranca el reloj si toca.
func (w *Gif) CreateRenderer() fyne.WidgetRenderer {
	img := canvas.NewImageFromImage(nil)
	if len(w.fotogramas) > 0 {
		img.Image = w.fotogramas[0]
	}
	img.FillMode = w.FillMode
	r := &gifRenderer{w: w, img: img}
	if w.animado() {
		w.mu.Lock()
		w.parar = make(chan struct{})
		parar := w.parar
		w.mu.Unlock()
		go w.reloj(img, parar)
	}
	return r
}

// reloj avanza los fotogramas con la pausa de cada uno.
func (w *Gif) reloj(img *canvas.Image, parar chan struct{}) {
	pasadas := 0
	for i := 0; ; {
		t := time.NewTimer(w.pausas[i])
		select {
		case <-parar:
			t.Stop()
			return
		case <-t.C:
		}
		siguiente := (i + 1) % len(w.fotogramas)
		if siguiente == 0 {
			pasadas++
			if w.vueltas < 0 || (w.vueltas > 0 && pasadas > w.vueltas) {
				return // se queda en el último, como el navegador
			}
		}
		fyne.Do(func() {
			if !w.Visible() || fyne.CurrentApp().Driver().CanvasForObject(w) == nil {
				return // fuera de la pantalla: ni textura ni repintado
			}
			w.mu.Lock()
			w.actual = siguiente
			w.mu.Unlock()
			img.Image = w.fotogramas[siguiente]
			img.Refresh()
		})
		i = siguiente
	}
}

type gifRenderer struct {
	w   *Gif
	img *canvas.Image
}

func (r *gifRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *gifRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.img} }
func (r *gifRenderer) Layout(t fyne.Size)           { r.img.Resize(t) }

func (r *gifRenderer) Refresh() {
	r.img.FillMode = r.w.FillMode
	r.img.Refresh()
}

// Destroy para el reloj: un GIF que ya no se pinta no despierta a nadie.
func (r *gifRenderer) Destroy() {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if r.w.parar != nil {
		close(r.w.parar)
		r.w.parar = nil
	}
}
