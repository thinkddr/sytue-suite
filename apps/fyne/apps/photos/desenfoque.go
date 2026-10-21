// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// SigmaBackdropMD es `backdrop-blur-md`: `blur(12px)` (el `--blur-md` de Tailwind v4, en
// px y no en rem).
const SigmaBackdropMD = 12

// FondoDesenfocado es lo que una caja con `backdrop-blur-*` enseña detrás de su fondo
// semitransparente: lo que queda debajo de ella, desenfocado y recortado a sus esquinas.
// Va de primer hijo de la caja, debajo de su relleno.
//
// Fyne no lo trae (ver ui.DesenfocarImagen, que es quien desenfoca), así que se pinta a
// mano: quien pinta lo de debajo sabe volver a pintar un trozo (`Debajo`), y el layout
// que coloca la caja le dice dónde está (Colocar). Solo se desenfoca la caja, con el
// borde extendido, que es lo que muestrea un `backdrop-filter`.
type FondoDesenfocado struct {
	widget.BaseWidget
	// Sigma es el del `blur()` y Radio el de las esquinas de la caja, en px de CSS.
	Sigma, Radio float32
	// Debajo pinta en dst, a `escala` píxeles por px de CSS, lo que hay debajo de la
	// caja con su esquina en `origen` (px de CSS, en las coordenadas de quien lo pinta).
	// Corre en el hilo que pinta.
	Debajo func(dst *image.NRGBA, origen fyne.Position, escala float32)

	mu     sync.Mutex
	origen fyne.Position
	ancho  float32
}

var _ fyne.Widget = (*FondoDesenfocado)(nil)

// NuevoFondoDesenfocado crea el fondo.
func NuevoFondoDesenfocado(sigma, radio float32, debajo func(*image.NRGBA, fyne.Position, float32)) *FondoDesenfocado {
	f := &FondoDesenfocado{Sigma: sigma, Radio: radio, Debajo: debajo}
	f.ExtendBaseWidget(f)
	return f
}

// Colocar dice dónde está la caja en las coordenadas de quien pinta lo de debajo, y
// cuánto mide de ancho. Lo llama el layout que la coloca: el hilo que pinta no puede
// leer `Position()` sin carrera.
func (f *FondoDesenfocado) Colocar(origen fyne.Position, ancho float32) {
	f.mu.Lock()
	f.origen, f.ancho = origen, ancho
	f.mu.Unlock()
}

// CreateRenderer es un ráster que se genera con cada pintado.
func (f *FondoDesenfocado) CreateRenderer() fyne.WidgetRenderer {
	r := canvas.NewRaster(f.generar)
	r.ScaleMode = canvas.ImageScalePixels
	return &fondoDesenfocadoRenderer{r: r}
}

func (f *FondoDesenfocado) generar(w, h int) image.Image {
	f.mu.Lock()
	origen, ancho := f.origen, f.ancho
	f.mu.Unlock()
	if w <= 0 || h <= 0 || ancho <= 0 || f.Debajo == nil {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	}
	escala := float32(w) / ancho
	debajo := image.NewNRGBA(image.Rect(0, 0, w, h))
	f.Debajo(debajo, origen, escala)
	out := ui.DesenfocarImagen(debajo, float64(f.Sigma*escala))
	// Opaca, así que sus bytes valen igual como RGBA premultiplicado: es lo que
	// `redondear` recorta con antialias.
	m := &image.RGBA{Pix: out.Pix, Stride: out.Stride, Rect: out.Rect}
	redondear(m, f.Radio*escala)
	return m
}

type fondoDesenfocadoRenderer struct{ r *canvas.Raster }

func (r *fondoDesenfocadoRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *fondoDesenfocadoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.r} }
func (r *fondoDesenfocadoRenderer) Destroy()                     {}
func (r *fondoDesenfocadoRenderer) Layout(t fyne.Size)           { r.r.Resize(t) }
func (r *fondoDesenfocadoRenderer) Refresh()                     { r.r.Refresh() }

// pintarFoto pinta en dst lo que se ve de una foto colocada en la caja (pos, tam) —en
// px de CSS, con el origen de dst en `origen`— tomando los píxeles de `m`, que es la
// foto entera a la resolución que sea. El vecino más cercano basta: lo que sale de aquí
// se desenfoca.
func pintarFoto(dst *image.NRGBA, origen fyne.Position, escala float32, pos fyne.Position, tam fyne.Size, m image.Image, fondo color.NRGBA) {
	if tam.Width <= 0 || tam.Height <= 0 {
		return
	}
	var b image.Rectangle
	if m != nil {
		b = m.Bounds()
	}
	for y := range dst.Rect.Dy() {
		cy := origen.Y + (float32(y)+0.5)/escala - pos.Y
		if cy < 0 || cy >= tam.Height {
			continue
		}
		for x := range dst.Rect.Dx() {
			cx := origen.X + (float32(x)+0.5)/escala - pos.X
			if cx < 0 || cx >= tam.Width {
				continue
			}
			c := fondo
			if m != nil && !b.Empty() {
				sx, sy := b.Min.X+int(cx/tam.Width*float32(b.Dx())), b.Min.Y+int(cy/tam.Height*float32(b.Dy()))
				if px, ok := color.NRGBAModel.Convert(m.At(sx, sy)).(color.NRGBA); ok {
					c = sobre(px, fondo)
				}
			}
			dst.SetNRGBA(x, y, c)
		}
	}
}

// rellenar pinta todo dst de un color.
func rellenar(dst *image.NRGBA, c color.NRGBA) {
	for i := 0; i+3 < len(dst.Pix); i += 4 {
		dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
	}
}
