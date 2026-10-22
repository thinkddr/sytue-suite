// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image"
	"sync"

	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// rasterSVG es un SVG propio (un minigráfico, los trazos de un gráfico) pintado con
// `canvas.Raster` y el rasterizador de los iconos, que es el que pinta a píxeles de
// DISPOSITIVO y escala el trazo con el viewBox (ver la nota de `iconos/pintar.go`:
// `canvas.Image` con un recurso SVG hace las dos cosas mal).
//
// El mapa de bits se rehace solo si cambian el SVG o el tamaño: los pintados de la
// rejilla lo piden en cada vuelta.
type rasterSVG struct {
	raster *canvas.Raster

	mu     sync.Mutex
	svg    string
	w, h   int
	hecho  string
	ultimo image.Image
}

func nuevoRasterSVG() *rasterSVG {
	r := &rasterSVG{}
	r.raster = canvas.NewRaster(r.generar)
	r.raster.ScaleMode = canvas.ImageScalePixels
	return r
}

// poner cambia el SVG; si es el mismo no hace nada.
func (r *rasterSVG) poner(svg string) {
	r.mu.Lock()
	cambia := svg != r.svg
	r.svg = svg
	r.mu.Unlock()
	if cambia {
		r.raster.Refresh()
	}
}

func (r *rasterSVG) generar(w, h int) image.Image {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ultimo != nil && r.hecho == r.svg && r.w == w && r.h == h {
		return r.ultimo
	}
	if r.svg == "" || w <= 0 || h <= 0 {
		r.ultimo = image.NewNRGBA(image.Rect(0, 0, max(w, 0), max(h, 0)))
	} else {
		r.ultimo = iconos.RasterizarSVG([]byte(r.svg), w, h)
	}
	r.hecho, r.w, r.h = r.svg, w, h
	return r.ultimo
}
