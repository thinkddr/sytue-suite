// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Spinner.tsx

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # La rueda de carga, copiada de Spinner.tsx
//
// La web pinta un SVG de 24×24 y lo hace girar con `animate-spin
// [animation-duration:700ms]`:
//
//	<circle cx=12 cy=12 r=9 stroke=currentColor stroke-width=2 opacity=0.25/>
//	<path d="M12 3a9 9 0 1 1-9 9" stroke=currentColor stroke-width=2 stroke-linecap=round/>
//
// O sea: un anillo completo al 25 % y encima un arco de 270° —de arriba, en el sentido
// del reloj, hasta la izquierda— opaco y con los extremos redondeados.
//
// # Por qué se rasteriza a mano y no se gira un icono
//
// Girar un `canvas.Raster` no se puede: Fyne no tiene transformación de objeto, y
// rehacer el SVG en cada fotograma pasaría por `oksvg` sesenta veces por segundo (0,15
// ms por pasada a 16 px, medido en `iconos`). Aquí el dibujo es un anillo y un arco:
// dos distancias con signo y una interpolación, sin parsear nada.
//
// Tampoco es un icono del catálogo: `iconos` es un conjunto CERRADO que sale de
// `Icon.tsx`, y la rueda no está ahí — está en `Spinner.tsx`, que es otro componente.
//
// # prefers-reduced-motion: Fyne no lo tiene
//
// El navegador consulta el ajuste del sistema y la hoja de la suite lo respeta; Fyne
// v2.8.1 no expone nada equivalente en ninguna plataforma. Lo que se hace aquí es
// `SinMovimiento`: una variable que pone quien arranca la app leyendo el ajuste del
// sistema, y que NO apaga el indicador —lo deja QUIETO en su posición de salida—. Una
// rueda parada sigue diciendo «esto está cargando» con la forma y con su nombre
// accesible; una rueda que desaparece no dice nada.

// SinMovimiento apaga las animaciones de repetición infinita de la suite. Es lo que la
// web recibe de `prefers-reduced-motion: reduce`, y aquí lo pone quien monta la app
// —antes de la primera pantalla— porque Fyne no lee ese ajuste del sistema.
//
// No afecta a las animaciones de ENTRADA (diálogo, velo, pista): esas duran 150-180 ms
// y se acaban solas. La que marea es la que no para.
var SinMovimiento bool

// DuracionRueda es el `[animation-duration:700ms]` de Spinner.tsx.
const DuracionRueda = 700 * time.Millisecond

// LadoRueda es el `size = 18` por defecto de Spinner.tsx.
const LadoRueda float32 = 18

// Geometría de Spinner.tsx, en unidades de su `viewBox="0 0 24 24"`.
const (
	rejillaRueda   = 24
	radioRueda     = 9
	grosorRueda    = 2
	barridoRueda   = 3 * math.Pi / 2 // el arco de 270° del `path`
	opacidadAnillo = 0.25            // el `opacity="0.25"` del `circle`
)

// Rueda es Spinner.tsx: el indicador de carga que gira.
//
// El color es el `currentColor` de la web, así que lo pone quien la coloca: `NuevaRueda`
// con un token, o `Tenir` con un color ya calculado (el rótulo atenuado de un botón
// cargando, por ejemplo).
type Rueda struct {
	widget.BaseWidget
	// Lado es el `size` en px de CSS. Con 0, LadoRueda.
	Lado float32
	// Rotulo es el nombre accesible. Vacío: `ui.loading` del catálogo común, que es el
	// `label ?? t('ui.loading')` de Spinner.tsx.
	Rotulo string

	mu     sync.Mutex
	tinta  color.NRGBA
	angulo float64
	anim   *fyne.Animation
}

var (
	_ fyne.Widget     = (*Rueda)(nil)
	_ fyne.Accessible = (*Rueda)(nil)
)

// NuevaRueda crea la rueda del tamaño pedido teñida con un token de la paleta.
func NuevaRueda(lado float32, token string, v tema.Variante) *Rueda {
	return NuevaRuedaCon(lado, tema.Color(token, v))
}

// NuevaRuedaCon es NuevaRueda con un color ya calculado; ver `iconos.Dibujo.TenirCon`
// para cuándo vale usarlo.
func NuevaRuedaCon(lado float32, c color.NRGBA) *Rueda {
	if lado <= 0 {
		lado = LadoRueda
	}
	r := &Rueda{Lado: lado, tinta: c}
	r.ExtendBaseWidget(r)
	return r
}

// Tenir cambia el color sin reconstruir nada.
func (r *Rueda) Tenir(c color.NRGBA) {
	r.mu.Lock()
	cambia := c != r.tinta
	r.tinta = c
	r.mu.Unlock()
	if cambia {
		r.Refresh()
	}
}

// AccessibilityLabel es el `aria-label` del `role="status"` de la web.
func (r *Rueda) AccessibilityLabel() string {
	if r.Rotulo == "" {
		return textoComun("ui.loading")
	}
	return r.Rotulo
}

// AccessibilityRole: Fyne 2.8 no tiene rol «status», así que se anuncia como texto —que
// es lo que de verdad llega al lector— con el rótulo por contenido.
func (r *Rueda) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MedidaRueda es lo que ocupa: cuadrada, como el `<svg width={size} height={size}>`.
func MedidaRueda(lado float32) fyne.Size {
	if lado <= 0 {
		lado = LadoRueda
	}
	return fyne.NewSize(lado, lado)
}

// CreateRenderer construye el raster y arranca el giro.
func (r *Rueda) CreateRenderer() fyne.WidgetRenderer {
	raster := canvas.NewRaster(r.pintar)
	raster.ScaleMode = canvas.ImageScalePixels
	rr := &ruedaRenderer{r: r, raster: raster}
	r.arrancar(raster)
	return rr
}

// arrancar lanza el giro: 700 ms por vuelta, lineal y sin fin.
//
// Lineal a propósito: `animate-spin` de Tailwind es `linear infinite`, y una rueda con
// aceleración se lee como un tirón.
func (r *Rueda) arrancar(raster *canvas.Raster) {
	if SinMovimiento {
		return
	}
	a := fyne.NewAnimation(DuracionRueda, func(p float32) {
		r.mu.Lock()
		r.angulo = 2 * math.Pi * float64(p)
		r.mu.Unlock()
		raster.Refresh()
	})
	a.Curve = fyne.AnimationLinear
	a.RepeatCount = fyne.AnimationRepeatForever
	r.mu.Lock()
	r.anim = a
	r.mu.Unlock()
	a.Start()
}

// Parar detiene el giro. Lo llama el renderer al destruirse; una pantalla que esconde
// la rueda sin quitarla del árbol puede llamarlo también.
func (r *Rueda) Parar() {
	r.mu.Lock()
	a := r.anim
	r.anim = nil
	r.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// pintar rasteriza el anillo y el arco en w×h píxeles de DISPOSITIVO.
//
// ponytail: recorre todos los píxeles de la caja con una raíz y dos arcotangentes por
// píxel. A la talla de una rueda —18 px de CSS, 36×36 a ×2: 1 296 píxeles— es nada, y
// solo corre mientras algo está cargando. Si alguna vez hace falta una rueda grande y
// permanente, lo que toca es acotar el bucle al anillo (dos tercios de los píxeles caen
// fuera), no cambiar de técnica.
func (r *Rueda) pintar(w, h int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return out
	}
	r.mu.Lock()
	tinta, giro := r.tinta, r.angulo
	r.mu.Unlock()
	if tinta.A == 0 {
		return out
	}

	esc := float64(w) / rejillaRueda
	cx, cy := float64(w)/2, float64(h)/2
	rad, hs := radioRueda*esc, grosorRueda*esc/2
	// Los dos extremos del arco, para los `stroke-linecap="round"`. El ángulo se mide
	// desde ARRIBA y en el sentido del reloj, que es donde empieza el `path` (12,3).
	punta := func(a float64) (float64, float64) {
		return cx + rad*math.Sin(a), cy - rad*math.Cos(a)
	}
	ax, ay := punta(giro)
	bx, by := punta(giro + barridoRueda)

	base := float64(tinta.A) / 255
	for y := range h {
		for x := range w {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			rho := math.Hypot(dx, dy)
			anillo := math.Abs(rho-rad) - hs // distancia con signo al anillo completo
			if anillo > 0.5 {
				continue // ni el anillo ni el arco llegan hasta aquí
			}

			// El arco es el anillo recortado por una cuña de 270°, más los dos
			// casquetes redondos. La distancia angular se mide en longitud de arco
			// para que el antialias del corte tenga el mismo grosor que el del borde.
			ang := math.Mod(math.Atan2(dx, -dy)-giro, 2*math.Pi)
			if ang < 0 {
				ang += 2 * math.Pi
			}
			var cuna float64
			if ang <= barridoRueda {
				cuna = -min(ang, barridoRueda-ang) * rho
			} else {
				cuna = min(ang-barridoRueda, 2*math.Pi-ang) * rho
			}
			arco := max(anillo, cuna)
			arco = min(arco, math.Hypot(float64(x)+0.5-ax, float64(y)+0.5-ay)-hs)
			arco = min(arco, math.Hypot(float64(x)+0.5-bx, float64(y)+0.5-by)-hs)

			// `circle` primero y `path` encima, con la regla de siempre: el arco se
			// compone sobre el anillo, los dos del mismo color.
			a1 := cobertura(anillo) * opacidadAnillo
			a2 := cobertura(arco)
			a := (a1 + a2*(1-a1)) * base
			if a <= 0 {
				continue
			}
			p := (y*w + x) * 4
			out.Pix[p], out.Pix[p+1], out.Pix[p+2] = tinta.R, tinta.G, tinta.B
			out.Pix[p+3] = uint8(a*255 + 0.5)
		}
	}
	return out
}

// cobertura convierte una distancia con signo en cuánto del píxel queda dentro, con la
// misma franja de un píxel que usa `planoRedondeado` en `sombra.go`.
func cobertura(d float64) float64 { return min(max(0.5-d, 0), 1) }

type ruedaRenderer struct {
	r      *Rueda
	raster *canvas.Raster
}

func (rr *ruedaRenderer) MinSize() fyne.Size           { return MedidaRueda(rr.r.Lado) }
func (rr *ruedaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{rr.raster} }
func (rr *ruedaRenderer) Destroy()                     { rr.r.Parar() }
func (rr *ruedaRenderer) Layout(t fyne.Size)           { rr.raster.Resize(t) }
func (rr *ruedaRenderer) Refresh()                     { rr.raster.Refresh() }
