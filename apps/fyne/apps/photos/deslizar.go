// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La pareja en la web es apps/frontend/apps/photos/src/lib/gestos.ts.

// Deslizar entre fotos con el dedo: la regla de `decidir` en gestos.ts, con los mismos
// umbrales (TestDeslizarEsLaReglaDeLaWeb repite sus casos).
const (
	recorridoMin      = 56   // RECORRIDO_MIN
	fraccionAncho     = 0.18 // FRACCION_ANCHO
	velocidadRapida   = 0.5  // VELOCIDAD_RAPIDA, px por milisegundo
	recorridoRapido   = 24   // RECORRIDO_RAPIDO
	dominioHorizontal = 1.4  // DOMINIO_HORIZONTAL
	resistencia       = 0.25 // RESISTENCIA: hacia un lado sin foto
)

// conDedo es `pointer: coarse`. Variable por los guardianes: el driver de pruebas es de
// escritorio y aquí hay que medir también el teléfono.
var conDedo = ui.EsTactil

// decidirDeslizamiento es −1 (la anterior), 1 (la siguiente) o 0. Hacia la IZQUIERDA se
// va a la siguiente, como al pasar una página.
func decidirDeslizamiento(dx, dy, ancho float32, ms float64) int {
	ax := math.Abs(float64(dx))
	if ax < dominioHorizontal*math.Abs(float64(dy)) {
		return 0
	}
	lejos := ax >= math.Max(recorridoMin, fraccionAncho*float64(ancho))
	rapido := ax >= recorridoRapido && ax/math.Max(ms, 1) >= velocidadRapida
	if !lejos && !rapido {
		return 0
	}
	if dx < 0 {
		return 1
	}
	return -1
}

// DeslizaFoto es la capa del escenario que recibe el dedo, encima de la foto y debajo de
// las flechas (el `<div>` de `EscenarioVisor` en la web). Con el dedo, arrastrar de lado
// mueve la foto y al soltar pasa de foto o la deja en su sitio; el doble toque (con el
// ratón, el clic) es el tamaño real en el punto tocado. Con el ratón, arrastrar no hace
// nada: en un visor de escritorio no se pasa de foto así.
//
// PELLIZCAR NO SE PUEDE AQUÍ, y no por falta de ganas: el driver móvil de Fyne 2.8 (y el
// fork) entrega el arrastre sin decir de qué dedo es —`fyne.DragEvent` no lleva el id del
// toque y `mobile.Touchable` no tiene «mover»—, así que con dos dedos no hay distancia
// entre ellos que medir. Hace falta ampliar el fork; mientras, el doble toque amplía.
//
// ponytail: al soltar sin pasar de foto, la foto vuelve de golpe y no en 150 ms como en
// la web; una animación de Fyne aquí pediría un repintado por fotograma del lienzo.
type DeslizaFoto struct {
	widget.BaseWidget

	lienzo                  *LienzoVisor
	alAnterior, alSiguiente func()
	alTocar                 func(fx, fy float32, vista fyne.Position)
	dx, dy                  float32
	desde                   time.Time
	toques                  toques
}

// Doble toque, como en la web: dos a menos de 300 ms y de 24 px (DOBLE_MS, DOBLE_PX).
const (
	dobleToque   = 300 * time.Millisecond
	dobleToquePx = 24
)

// toques recuerda el último toque para reconocer el doble.
type toques struct {
	t time.Time
	p fyne.Position
}

// doble dice si este toque cierra un doble toque, y si no lo apunta como el primero.
func (u *toques) doble(e *fyne.PointEvent) bool {
	var p fyne.Position
	if e != nil {
		p = e.Position
	}
	if !u.t.IsZero() && time.Since(u.t) < dobleToque && math.Hypot(float64(p.X-u.p.X), float64(p.Y-u.p.Y)) < dobleToquePx {
		u.t = time.Time{}
		return true
	}
	u.t, u.p = time.Now(), p
	return false
}

var (
	_ fyne.Widget        = (*DeslizaFoto)(nil)
	_ fyne.Tappable      = (*DeslizaFoto)(nil)
	_ fyne.Draggable     = (*DeslizaFoto)(nil)
	_ desktop.Cursorable = (*DeslizaFoto)(nil)
)

// NuevoDeslizaFoto crea la capa sobre ese lienzo. anterior y siguiente pueden ser nil.
func NuevoDeslizaFoto(l *LienzoVisor, anterior, siguiente func(), alTocar func(fx, fy float32, vista fyne.Position)) *DeslizaFoto {
	d := &DeslizaFoto{lienzo: l, alAnterior: anterior, alSiguiente: siguiente, alTocar: alTocar}
	d.ExtendBaseWidget(d)
	return d
}

// Tapped es el clic en la foto: el tamaño real con el punto pulsado bajo el puntero.
// Con el dedo hace falta el doble toque: un toque suelto no hace nada.
func (d *DeslizaFoto) Tapped(e *fyne.PointEvent) {
	if d.alTocar == nil || (conDedo() && !d.toques.doble(e)) {
		return
	}
	var p fyne.Position
	if e != nil {
		p = e.Position
	}
	fx, fy := d.lienzo.fraccionEn(p)
	d.alTocar(fx, fy, p)
}

// Cursor es el de la foto (`cursor-zoom-in`): Fyne no trae la lupa.
func (d *DeslizaFoto) Cursor() desktop.Cursor { return desktop.PointerCursor }

// Dragged sigue al dedo, a un cuarto hacia un lado sin foto.
func (d *DeslizaFoto) Dragged(e *fyne.DragEvent) {
	if !conDedo() {
		return
	}
	if d.desde.IsZero() {
		d.desde = time.Now()
	}
	d.dx += e.Dragged.DX
	d.dy += e.Dragged.DY
	if math.Abs(float64(d.dx)) < dominioHorizontal*math.Abs(float64(d.dy)) {
		d.lienzo.Desplazar(0)
		return
	}
	sinFoto := (d.dx > 0 && d.alAnterior == nil) || (d.dx < 0 && d.alSiguiente == nil)
	if sinFoto {
		d.lienzo.Desplazar(d.dx * resistencia)
		return
	}
	d.lienzo.Desplazar(d.dx)
}

// DragEnd decide.
func (d *DeslizaFoto) DragEnd() {
	if d.desde.IsZero() {
		return
	}
	paso := decidirDeslizamiento(d.dx, d.dy, d.Size().Width, float64(time.Since(d.desde).Milliseconds()))
	d.dx, d.dy, d.desde = 0, 0, time.Time{}
	d.lienzo.Desplazar(0)
	switch {
	case paso == 1 && d.alSiguiente != nil:
		d.alSiguiente()
	case paso == -1 && d.alAnterior != nil:
		d.alAnterior()
	}
}

// CreateRenderer: no pinta nada, solo recibe.
func (d *DeslizaFoto) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(&fyne.Container{})
}
