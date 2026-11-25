// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
)

// # Las animaciones de carga se duermen cuando nadie las pinta
//
// Las ruedas y los esqueletos laten sin fin, y antes solo paraban en el `Destroy` de su
// renderer. Fyne no lo llama al quitar el widget del árbol sino cuando caduca su caché, a
// los 60–70 s, y mientras tanto la animación huérfana seguía repintando un rectángulo
// que ya no está en ninguna parte: el lienzo sigue sucio, el driver no se duerme y se
// pinta la ventana entera sesenta veces por segundo. Medido el 2026-11-24 bajo Xvfb: un
// esqueleto huérfano eran 554 ms de CPU cada 3 s.
//
// Lo que dice si alguien la pinta es el propio pintor: cada fotograma recorre el árbol
// VISIBLE y le pide a cada widget sus `Objects()`, también al de la animación. Ese
// recorrido no pasa por lo que se quitó del árbol, ni por lo escondido (el widget o
// cualquiera de sus padres), ni por una ventana que no se pinta. Así que el renderer
// avisa en `Objects()` (pintada) y cada fotograma de la animación mira si ha habido
// aviso desde el anterior: dos fotogramas seguidos sin él, y se duerme. Al volver a
// pintarse —otra vez en el árbol, enseñada, la ventana de vuelta— se despierta sola, en
// la fase en que se quedó.
//
// Dos y no uno porque un fotograma puede quedarse sin pintar aunque el widget esté (la
// ventana espera al compositor); dormirse ahí no rompe nada —el siguiente pintado la
// despierta—, pero no hace falta.

// fotogramasSinPintar son los que una animación aguanta sin que el pintor pase por su
// widget antes de dormirse.
const fotogramasSinPintar = 2

// animacionSinFin es una animación de carga lineal y sin fin que se duerme sola cuando su
// widget deja de pintarse (ver arriba). El renderer llama a pintada desde `Objects()`.
type animacionSinFin struct {
	mu   sync.Mutex
	anim *fyne.Animation
	dur  time.Duration
	paso func(float32)
	// dormida: la paró nadie la pintaba, y la vuelve a arrancar el próximo pintado.
	// Parada con parar, no la despierta nada.
	dormida bool
	// sinPintar cuenta los fotogramas seguidos sin aviso del pintor; desfase y ultima
	// llevan la fase a través de las siestas.
	sinPintar       int
	desfase, ultima float32
	vista           atomic.Bool
}

// arrancar lanza la animación: `paso` recibe la fase de 0 a 1 en cada fotograma.
func (a *animacionSinFin) arrancar(dur time.Duration, paso func(float32)) {
	a.mu.Lock()
	vieja := a.anim
	a.dur, a.paso = dur, paso
	a.dormida, a.sinPintar, a.desfase, a.ultima = false, 0, 0, 0
	nueva := a.nuevaLocked()
	a.mu.Unlock()
	if vieja != nil {
		vieja.Stop()
	}
	a.vista.Store(true) // la recién nacida tiene un fotograma de gracia
	nueva.Start()
}

// nuevaLocked crea la animación de Fyne. Va con el cerrojo cogido.
func (a *animacionSinFin) nuevaLocked() *fyne.Animation {
	var esta *fyne.Animation
	esta = fyne.NewAnimation(a.dur, func(p float32) { a.fotograma(esta, p) })
	esta.Curve = fyne.AnimationLinear
	esta.RepeatCount = fyne.AnimationRepeatForever
	a.anim = esta
	return esta
}

// fotograma es lo que tica Fyne: duerme la animación si nadie la ha pintado y, si no,
// pasa la fase a quien anima.
func (a *animacionSinFin) fotograma(esta *fyne.Animation, p float32) {
	vista := a.vista.Swap(false)
	a.mu.Lock()
	if a.anim != esta {
		a.mu.Unlock()
		return // una que ya se paró y todavía tica una última vez
	}
	if vista {
		a.sinPintar = 0
	} else {
		a.sinPintar++
	}
	if a.sinPintar >= fotogramasSinPintar {
		a.anim, a.dormida, a.desfase = nil, true, a.ultima
		a.mu.Unlock()
		esta.Stop()
		return
	}
	fase := p + a.desfase
	if fase > 1 {
		fase--
	}
	a.ultima = fase
	paso := a.paso
	a.mu.Unlock()
	paso(fase)
}

// pintada es el aviso del pintor, desde el `Objects()` del renderer: la mantiene
// despierta y despierta a la dormida.
func (a *animacionSinFin) pintada() {
	a.vista.Store(true)
	a.mu.Lock()
	if !a.dormida {
		a.mu.Unlock()
		return
	}
	a.dormida, a.sinPintar = false, 0
	nueva := a.nuevaLocked()
	a.mu.Unlock()
	nueva.Start()
}

// parar la detiene para siempre (hasta otro arrancar): ni el pintor la despierta.
func (a *animacionSinFin) parar() {
	a.mu.Lock()
	vieja := a.anim
	a.anim, a.dormida = nil, false
	a.mu.Unlock()
	if vieja != nil {
		vieja.Stop()
	}
}

// andando dice si está ticando ahora mismo (ni dormida ni parada).
func (a *animacionSinFin) andando() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.anim != nil
}
