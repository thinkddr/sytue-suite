// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// Moverse con las flechas por una cuadrícula de elementos enfocables: las tarjetas de
// Drive (la rejilla y la franja de carpetas) y las filas justificadas de Photos. Es el
// espejo de `@sytue/ui` `lib/navegacionRejilla.ts`, con los mismos casos en las pruebas.
//
// LA CUADRÍCULA SE MIDE, NO SE CUENTA. Las columnas de Drive cambian con el ancho y las
// filas de Photos tienen cada una las suyas, partidas además por meses. ↑ y ↓ buscan en
// la fila de encima o de debajo la caja cuyo centro está más cerca en horizontal; ← y →
// siguen el orden de lectura y pasan de fila, como Google Photos y el Finder.

// CajaRejilla es la caja de un elemento en el lienzo.
type CajaRejilla struct{ X, Y, Ancho, Alto float32 }

// holguraRejilla: con fracciones de píxel, dos cajas de la misma fila no casan al céntimo.
const holguraRejilla float32 = 1

// VecinoEnLista es el índice al que lleva `tecla` desde `actual` en una lista de una
// columna, o -1 si la tecla no es de moverse o ya está en el borde.
func VecinoEnLista(actual, total int, tecla fyne.KeyName) int {
	var destino int
	switch tecla {
	case fyne.KeyDown:
		destino = actual + 1
	case fyne.KeyUp:
		destino = actual - 1
	case fyne.KeyHome:
		destino = 0
	case fyne.KeyEnd:
		destino = total - 1
	default:
		return -1
	}
	if destino < 0 || destino >= total || destino == actual {
		return -1
	}
	return destino
}

// VecinoEnRejilla es el índice al que lleva `tecla` desde `actual` en una cuadrícula cuyas
// cajas, en orden de lectura, son `cajas`; -1 si la tecla no es de moverse o no hay a
// dónde ir.
func VecinoEnRejilla(cajas []CajaRejilla, actual int, tecla fyne.KeyName) int {
	total := len(cajas)
	if actual < 0 || actual >= total {
		return -1
	}
	switch tecla {
	case fyne.KeyRight:
		if actual+1 < total {
			return actual + 1
		}
		return -1
	case fyne.KeyLeft:
		return actual - 1 // -1 en la primera, que es «ninguno»
	case fyne.KeyHome, fyne.KeyEnd:
		return VecinoEnLista(actual, total, tecla)
	case fyne.KeyDown, fyne.KeyUp:
	default:
		return -1
	}
	return vecinoVertical(cajas, actual, tecla == fyne.KeyDown)
}

// vecinoVertical es ↓ (abajo) o ↑ en VecinoEnRejilla: la caja de la fila de al lado cuyo
// centro está más cerca en horizontal.
func vecinoVertical(cajas []CajaRejilla, actual int, abajo bool) int {
	yo := cajas[actual]
	borde := func(c CajaRejilla) float32 {
		if abajo {
			return c.Y
		}
		return c.Y + c.Alto
	}
	// La fila más cercana en esa dirección: la de arriba del todo de las que empiezan bajo
	// mi borde inferior, o la de abajo del todo de las que acaban sobre mi borde superior.
	hay, fila := false, float32(0)
	for _, c := range cajas {
		dentro := c.Y+c.Alto <= yo.Y+holguraRejilla
		if abajo {
			dentro = c.Y >= yo.Y+yo.Alto-holguraRejilla
		}
		if !dentro {
			continue
		}
		if b := borde(c); !hay || (abajo && b < fila) || (!abajo && b > fila) {
			hay, fila = true, b
		}
	}
	if !hay {
		return -1
	}
	mio := yo.X + yo.Ancho/2
	mejor, distancia := -1, float32(math.Inf(1))
	for i, c := range cajas {
		if d := borde(c) - fila; d > holguraRejilla || d < -holguraRejilla {
			continue
		}
		// Con un empate gana la primera en el orden de lectura.
		if d := float32(math.Abs(float64(c.X + c.Ancho/2 - mio))); d < distancia {
			mejor, distancia = i, d
		}
	}
	return mejor
}

// MoverFocoEnRejilla es el TypedKey de una cuadrícula: si `tecla` lleva desde `objs[actual]`
// a otro, le da el foco y, con `s`, lo trae a la vista (MostrarFoco). Las cajas se miden en
// el lienzo, así que valen las de varias rejillas seguidas (los meses de Photos). Devuelve
// si se movió; si no, la tecla sigue su camino.
func MoverFocoEnRejilla(objs []fyne.CanvasObject, actual int, tecla fyne.KeyName, s *container.Scroll) bool {
	d := fyne.CurrentApp().Driver()
	cajas := make([]CajaRejilla, len(objs))
	for i, o := range objs {
		p, t := d.AbsolutePositionForObject(o), o.Size()
		cajas[i] = CajaRejilla{X: p.X, Y: p.Y, Ancho: t.Width, Alto: t.Height}
	}
	destino := VecinoEnRejilla(cajas, actual, tecla)
	if destino < 0 {
		return false
	}
	return enfocarEn(objs[destino], s)
}

// enfocarEn da el foco a `o` y lo trae a la vista dentro de `s`, si lo hay.
func enfocarEn(o fyne.CanvasObject, s *container.Scroll) bool {
	f, ok := o.(fyne.Focusable)
	c := lienzoDe(o)
	if !ok || c == nil {
		return false
	}
	c.Focus(f)
	if s != nil {
		MostrarFoco(s, c, 0)
	}
	return true
}
