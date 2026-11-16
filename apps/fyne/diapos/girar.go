// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Cajas giradas
//
// `SlideBox.rot` gira la caja alrededor de su centro (slides/girar.ts de la web, la
// propiedad `rotate` de CSS). Fyne no gira objetos, así que una caja girada se pinta como
// el navegador la compone: su contenido, pintado una vez con el pintor de software a la
// resolución de la pantalla (ui.Instantanea), y ese mapa girado (iconos.GirarImagen) en un
// `canvas.Raster` del tamaño de su envolvente. El raster lo genera a los píxeles que le
// toquen, así que no se pinta a una escala y se estira a otra (ver
// TestElEscenarioEscalaSinRasterizar). Aquí solo se PINTA girada: el editor de Fyne aún
// no la gira ni la redimensiona sobre sus ejes.

// GiroDe es el giro de una caja tal y como se pinta, en [0, 360): 0 si no lleva o no es
// un número (`giroDe` de la web).
func GiroDe(c Caja) float64 {
	g := c.Giro
	if math.IsNaN(g) || math.IsInf(g, 0) {
		return 0
	}
	g = math.Round(math.Mod(math.Mod(g, 360)+360, 360)*1000) / 1000
	if g >= 360 {
		return 0
	}
	return g
}

// envolventeGirada es el rectángulo que ocupa en pantalla una caja de `tam` girada `grados`
// alrededor de su centro.
func envolventeGirada(tam fyne.Size, grados float64) fyne.Size {
	s, c := math.Sincos(grados * math.Pi / 180)
	w, h := float64(tam.Width), float64(tam.Height)
	return fyne.NewSize(float32(math.Abs(w*c)+math.Abs(h*s)), float32(math.Abs(w*s)+math.Abs(h*c)))
}

// girada pinta las primitivas de una caja (colocadas en `pos`, de tamaño `tam`) giradas
// `grados` alrededor de su centro: un solo raster centrado donde estaba la caja.
func girada(objs []fyne.CanvasObject, pos fyne.Position, tam fyne.Size, grados float64) []fyne.CanvasObject {
	if len(objs) == 0 || tam.Width <= 0 || tam.Height <= 0 {
		return objs
	}
	// Las primitivas vienen en coordenadas del escenario: se llevan a las de la caja.
	for _, o := range objs {
		o.Move(o.Position().Subtract(pos))
	}
	cont := container.NewWithoutLayout(objs...)
	cont.Resize(tam)
	env := envolventeGirada(tam, grados)
	g := &capaGirada{cont: cont, tam: tam, env: env, grados: grados}
	r := canvas.NewRaster(g.generar)
	r.ScaleMode = canvas.ImageScaleSmooth
	r.Resize(env)
	r.Move(pos.AddXY((tam.Width-env.Width)/2, (tam.Height-env.Height)/2))
	return []fyne.CanvasObject{r}
}

// capaGirada es el contenido de una caja girada, que se repinta solo si cambian los
// píxeles que pide el raster.
type capaGirada struct {
	cont   *fyne.Container
	tam    fyne.Size
	env    fyne.Size
	grados float64

	mu     sync.Mutex
	w, h   int
	ultimo image.Image
}

func (g *capaGirada) generar(w, h int) image.Image {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ultimo != nil && g.w == w && g.h == h {
		return g.ultimo
	}
	esc := float32(1)
	if g.env.Width > 0 {
		esc = float32(w) / g.env.Width
	}
	g.w, g.h = w, h
	g.ultimo = iconos.GirarImagen(ui.Instantanea(g.cont, esc), g.grados)
	return g.ultimo
}
