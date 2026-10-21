// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// FondoHastaElBorde extiende el fondo plano de una ventana móvil propia hasta
// los bordes laterales e inferior. Los controles siguen dentro de InteractiveArea.
// El tramo superior incluye la cabecera que Fyne añade a las ventanas secundarias:
// se conserva su fondo para que su título siga siendo legible en el tema claro.
func FondoHastaElBorde(raiz fyne.CanvasObject, fondo *canvas.Rectangle, tam fyne.Size) {
	if c := lienzoDe(raiz); EsTactil() && c != nil {
		segura, _ := c.InteractiveArea()
		colocarFondoDeVentana(fondo, segura, c.Size())
		return
	}
	fondo.Move(fyne.NewPos(0, 0))
	fondo.Resize(tam)
}

func colocarFondoDeVentana(fondo *canvas.Rectangle, segura fyne.Position, total fyne.Size) {
	fondo.Move(fyne.NewPos(-segura.X, 0))
	fondo.Resize(fyne.NewSize(total.Width, max(0, total.Height-segura.Y)))
}

// FondoSuite es el lienzo común de la suite. Claro y oscuro usan `bg`; translúcido
// pinta `--workspace-canvas` de theme.css con sus cuatro roles canónicos. Se coloca
// en la raíz de la pantalla, sin cambiar el fondo propio de filas y tarjetas.
//
// Los colores planos son una primitiva sólida: una página desplazable puede ser
// más alta que la textura máxima de una GPU. Solo el degradado usa un ráster, que
// se conserva al refrescar el contenido. No lleva blur ni animación.
type FondoSuite struct {
	widget.BaseWidget
	Variante tema.Variante
}

// NuevoFondoSuite construye el lienzo para esa apariencia.
func NuevoFondoSuite(v tema.Variante) *FondoSuite {
	f := &FondoSuite{Variante: v}
	f.ExtendBaseWidget(f)
	return f
}

// PonerVariante conserva el ráster y solo lo invalida cuando cambia la apariencia.
func (f *FondoSuite) PonerVariante(v tema.Variante) {
	if f.Variante == v {
		return
	}
	f.Variante = v
	f.Refresh()
}

func (f *FondoSuite) CreateRenderer() fyne.WidgetRenderer {
	r := &fondoSuiteRenderer{f: f, solido: canvas.NewRectangle(tema.Color("bg", f.Variante))}
	r.raster = canvas.NewRaster(r.pintar)
	r.raster.ScaleMode = canvas.ImageScalePixels
	r.Refresh()
	return r
}

type tintasFondoSuite struct {
	azul, medio, menta, calido color.NRGBA
}

type fondoSuiteRenderer struct {
	f      *FondoSuite
	solido *canvas.Rectangle
	raster *canvas.Raster
	v      tema.Variante
	base   color.NRGBA
	tintas tintasFondoSuite
}

func (r *fondoSuiteRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *fondoSuiteRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.solido, r.raster}
}
func (r *fondoSuiteRenderer) Destroy() {}
func (r *fondoSuiteRenderer) Layout(t fyne.Size) {
	r.solido.Resize(t)
	r.raster.Resize(t)
}

func (r *fondoSuiteRenderer) Refresh() {
	r.v = r.f.Variante
	// En la forma Marco el lienzo ambiental del traslúcido no se ve: lo tapa el marco, y
	// lo que hay detrás del contenido es el papel de la HOJA (`sheet-bg`), que pinta
	// quien la lleva. En claro y en oscuro el lienzo y la hoja son el mismo `bg`.
	r.base = tema.Color("bg", r.v)
	r.solido.FillColor = r.base
	r.solido.Hidden = r.v == tema.Translucido
	r.raster.Hidden = r.v != tema.Translucido || tema.EsMarco()
	if r.v == tema.Translucido {
		r.tintas = tintasFondoSuite{
			azul: tema.Color("workspace-blue", r.v), medio: tema.Color("workspace-middle", r.v),
			menta: tema.Color("workspace-mint", r.v), calido: tema.Color("workspace-warm", r.v),
		}
	}
	r.solido.Refresh()
	r.raster.Refresh()
}

// geometriaFondoSuite resuelve una vez por imagen las dos geometrías de CSS:
// linear-gradient(112deg, … 0%, … 44%, … 100%) y radial-gradient(ellipse at 88% 32%,
// …, transparent 38%). La elipse usa el farthest-corner predeterminado de CSS.
type geometriaFondoSuite struct {
	ancho, alto, seno, coseno, longitud float64
	cx, cy, rx, ry                      float64
}

func geometriaDelFondoSuite(ancho, alto float64) geometriaFondoSuite {
	seno, coseno := math.Sincos(112 * math.Pi / 180)
	cx, cy := ancho*0.88, alto*0.32
	return geometriaFondoSuite{
		ancho: ancho, alto: alto, seno: seno, coseno: coseno,
		longitud: math.Abs(ancho*seno) + math.Abs(alto*coseno),
		cx:       cx, cy: cy, rx: math.Sqrt2 * math.Max(cx, ancho-cx), ry: math.Sqrt2 * math.Max(cy, alto-cy),
	}
}

// color interpola los tres stops en sRGB y compone la capa cálida antes de reducir
// el resultado a ocho bits. Redondear primero el degradado lineal acumula error
// en la segunda capa. El alfa de la capa radial sigue la precisión del ráster.
func (g geometriaFondoSuite) color(x, y float64, c tintasFondoSuite) color.NRGBA {
	t := math.Max(0, math.Min(1, 0.5+((x-g.ancho/2)*g.seno-(y-g.alto/2)*g.coseno)/g.longitud))
	desde, hasta, paso := c.azul, c.medio, t/0.44
	if t > 0.44 {
		desde, hasta, paso = c.medio, c.menta, (t-0.44)/0.56
	}
	dx, dy := (x-g.cx)/g.rx, (y-g.cy)/g.ry
	alfa := math.Max(0, 1-math.Sqrt(dx*dx+dy*dy)/0.38) * float64(c.calido.A) / 255
	alfa = math.Floor(alfa*255) / 255
	canal := func(d, h, calido uint8) uint8 {
		lineal := float64(d) + (float64(h)-float64(d))*paso
		return uint8(math.Round(lineal + (float64(calido)-lineal)*alfa))
	}
	return color.NRGBA{
		R: canal(desde.R, hasta.R, c.calido.R),
		G: canal(desde.G, hasta.G, c.calido.G), B: canal(desde.B, hasta.B, c.calido.B), A: 255,
	}
}

func (r *fondoSuiteRenderer) pintar(ancho, alto int) image.Image {
	imagen := image.NewNRGBA(image.Rect(0, 0, max(ancho, 0), max(alto, 0)))
	if ancho <= 0 || alto <= 0 {
		return imagen
	}
	g := geometriaDelFondoSuite(float64(ancho), float64(alto))
	for y := range alto {
		for x := range ancho {
			// Los stops se muestrean en el centro del píxel de dispositivo, igual que
			// Chromium; usar las esquinas desplaza la mezcla al cambiar de escala.
			c := g.color(float64(x)+0.5, float64(y)+0.5, r.tintas)
			i := y*imagen.Stride + x*4
			imagen.Pix[i], imagen.Pix[i+1], imagen.Pix[i+2], imagen.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return imagen
}
