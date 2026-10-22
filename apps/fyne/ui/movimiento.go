// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"time"

	"fyne.io/fyne/v2"
)

// Las duraciones de `styles.css`. Van aquí y no en cada widget porque son de la casa:
// `TestLasDuracionesSonLasDeLaHoja` las compara con los `@keyframes` del fichero.
//
// NO hay animación de SALIDA. La web desmonta el componente y ya; un port que anime el
// cierre no está copiando la web, la está mejorando por su cuenta.
const (
	// DuracionFundido es `sytue-fade-in`: el velo de un diálogo, la pista.
	DuracionFundido = 150 * time.Millisecond
	// DuracionPop es `sytue-pop-in`: el menú emergente.
	DuracionPop = 150 * time.Millisecond
	// DuracionDialogo es `sytue-modal-in`: el panel de un diálogo.
	DuracionDialogo = 180 * time.Millisecond
	// DuracionHoja es `sytue-hoja-in`: el diálogo del teléfono, que sube desde abajo.
	DuracionHoja = 220 * time.Millisecond
	// DuracionAviso es `sytue-toast-in`.
	DuracionAviso = 180 * time.Millisecond
)

// DesplazamientoDialogo es el `translateY(10px)` del que entra `sytue-modal-in`.
const DesplazamientoDialogo float32 = 10

// DesplazamientoAviso es el `translateY(8px)` del que entra `sytue-toast-in`.
const DesplazamientoAviso float32 = 8

// DesplazamientoPop es el `translateY(-4px)` de `sytue-pop-in`, hacia ARRIBA.
const DesplazamientoPop float32 = 4

// EscalaEntrada es el `scale(0.98)` del que parten `sytue-pop-in` y `sytue-modal-in`. La
// opacidad y la escala de las entradas se hacen con una imagen del panel (fundido.go).
const EscalaEntrada float32 = 0.98

// CurvaEstandar es `--ease-standard`, que resuelto es `cubic-bezier(0.2, 0, 0, 1)`: la
// curva de TODAS las animaciones de la casa.
//
// No es ninguna de las de Fyne —`AnimationEaseInOut` es una cúbica simétrica— y la
// diferencia se ve: esta arranca casi recta y frena largo, que es lo que hace que un
// diálogo parezca que se posa. Con `Linear` o con la de Fyne, entra y se planta.
//
// Ojo, porque conviven dos curvas en la suite: los controles (color, fondo, borde)
// transicionan en 0,15 s con la de Tailwind, `cubic-bezier(0.4, 0, 0.2, 1)`, que NO es
// esta. Aquí solo está la de las entradas.
var CurvaEstandar fyne.AnimationCurve = func(t float32) float32 {
	return bezier(t, 0.2, 0, 0, 1)
}

// bezier evalúa una `cubic-bezier(x1, y1, x2, y2)` de CSS en `t`.
//
// Una Bézier de CSS da y en función de x, y los puntos de control solo parametrizan la
// curva: hay que invertir x(s) = t antes de poder leer y. Se hace por bisección y no
// por Newton porque son ocho pasadas sobre un polinomio de tercer grado —nada, y esto
// corre una vez por fotograma— y la bisección no se va a ningún sitio con tangente
// cero, que es justo lo que tiene esta curva en los dos extremos (x2 = 0).
func bezier(t, x1, y1, x2, y2 float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	eje := func(s, a, b float32) float32 {
		u := 1 - s
		return 3*u*u*s*a + 3*u*s*s*b + s*s*s
	}
	lo, hi := float32(0), float32(1)
	for range 24 {
		medio := (lo + hi) / 2
		if eje(medio, x1, x2) < t {
			lo = medio
		} else {
			hi = medio
		}
	}
	return eje((lo+hi)/2, y1, y2)
}
