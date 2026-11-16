// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package llamada

import (
	"math"
	"sync"
	"time"
)

// El timbre de una llamada entrante: el de `lib/timbre.ts` de la web, sin fichero de
// audio. Allí son dos osciladores de la Web Audio API; aquí es el mismo bitono escrito
// muestra a muestra a 48 kHz, que es lo que la tarjeta del sistema ya sabe reproducir
// (`altavoz.Sistema.Abrir` sin micrófono: no enciende el punto de grabación).
//
// Se para solo a los TopeTimbre, como en la web: un timbre que suena para siempre
// porque nadie contestó es peor que no tenerlo.

const (
	// CadenciaTimbre es cada cuánto se repite el bitono (`CADENCIA_MS`).
	CadenciaTimbre = 3 * time.Second
	// TopeTimbre es lo que dura como mucho, conteste alguien o no (`MAXIMO_MS`).
	TopeTimbre = 30 * time.Second
	// volumenTimbre es la ganancia de cada pitido en la web (0.14).
	volumenTimbre = 0.14
	// ataqueTimbre y caidaTimbre son las rampas de la ganancia: un seno a pelo chasquea
	// al empezar y al parar.
	ataqueTimbre = 0.02
	caidaTimbre  = 0.04
)

// notaTimbre es un pitido: su frecuencia, cuándo empieza dentro de la tanda y cuánto
// dura, en segundos.
type notaTimbre struct{ hz, empieza, dura float64 }

// notasTimbre son el re5 y el la5 de la web: un bitono ascendente, como el de un
// teléfono.
var notasTimbre = [...]notaTimbre{{587.33, 0, 0.18}, {880, 0.22, 0.22}}

var (
	muestrasCadencia = int(CadenciaTimbre.Seconds() * Muestreo)
	muestrasTope     = int(TopeTimbre.Seconds() * Muestreo)
)

// MuestraTimbre es la muestra `i` del timbre a Muestreo: el bitono al principio de cada
// CadenciaTimbre, silencio entre medias y silencio para siempre pasado el TopeTimbre.
//
// Es una función pura del índice a propósito: la cadencia y el tope se comprueban
// contando muestras, sin reloj y sin tarjeta.
func MuestraTimbre(i int) int16 {
	if i < 0 || i >= muestrasTope {
		return 0
	}
	t := float64(i%muestrasCadencia) / Muestreo
	for _, n := range notasTimbre {
		x := t - n.empieza
		if x < 0 || x >= n.dura {
			continue
		}
		g := volumenTimbre
		switch {
		case x < ataqueTimbre:
			g *= x / ataqueTimbre
		case x > n.dura-caidaTimbre:
			g *= (n.dura - x) / caidaTimbre
		}
		return int16(math.Round(g * math.Sin(2*math.Pi*n.hz*x) * math.MaxInt16))
	}
	return 0
}

// Timbre va dando el bitono a trozos, en el orden en que la tarjeta los pide.
type Timbre struct {
	mu  sync.Mutex
	pos int
}

// Rellenar escribe en `pcm` las muestras siguientes y dice si queda timbre. Pasado el
// tope rellena con silencio: la tarjeta sigue pidiendo hasta que se cierra.
func (t *Timbre) Rellenar(pcm []int16) (sigue bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range pcm {
		pcm[k] = MuestraTimbre(t.pos + k)
	}
	t.pos += len(pcm)
	return t.pos < muestrasTope
}

// SonarTimbre abre `d` solo para reproducir y hace sonar el timbre. Devuelve la función
// que lo para; llamarla más de una vez no hace daño. Al llegar al tope se cierra solo.
//
// La tarjeta se abre en otra rutina y SonarTimbre vuelve en el acto: se llama desde el
// hilo de la interfaz al llegar la trama, y arrancar el audio del sistema puede tardar
// (PulseAudio, WASAPI) lo bastante para que la ventana se note atascada. Si se para
// antes de que abra, se cierra nada más abrir.
//
// Si la tarjeta no abre, no pasa nada: el aviso en pantalla ya está puesto, y una
// llamada entrante no es el momento de enseñar un fallo de audio.
func SonarTimbre(d Dispositivo) (parar func()) {
	var (
		mu     sync.Mutex
		cerrar func()
		parado bool
		agotar sync.Once
	)
	parar = func() {
		mu.Lock()
		c := cerrar
		cerrar, parado = nil, true
		mu.Unlock()
		if c != nil {
			c()
		}
	}
	go func() {
		var tb Timbre
		c, err := d.Abrir(nil, func(pcm []int16) {
			if !tb.Rellenar(pcm) {
				// En otra rutina: cerrar la tarjeta desde su propia llamada de vuelta la
				// esperaría a ella misma.
				agotar.Do(func() { go parar() })
			}
		})
		if err != nil {
			return
		}
		mu.Lock()
		if parado {
			// Se agotó (o se paró) antes de que Abrir volviera.
			mu.Unlock()
			c()
			return
		}
		cerrar = c
		mu.Unlock()
	}()
	return parar
}
