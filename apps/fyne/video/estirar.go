// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package video

import "math"

// # Velocidad sin cambiar el tono
//
// Reproducir a 1,5× no puede ser leer el PCM más deprisa: la voz sube de tono y suena
// a ardilla. Lo que hace el navegador con `playbackRate` (y lo que se hace aquí) es
// WSOLA: se trocea el sonido en tramos que se solapan, se avanza por la ENTRADA a
// saltos de `paso × velocidad` y por la SALIDA a saltos de `paso`, y cada tramo se
// busca unas muestras antes o después de donde tocaría para que case con lo que venía
// sonando (la correlación más alta). Así la duración escala y las ondas siguen siendo
// las mismas, con su periodo: el tono no cambia.
//
// Va en mono a 48 kHz, que es lo que lleva `Reproductor.pcm`.

// Velocidades que ofrece el menú, de menor a mayor, como `lib/velocidad.ts` de la web:
// por debajo de 0,5× la voz ya no se entiende y por encima de 2× no hace falta.
var Velocidades = []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2}

const (
	// pasoWSOLA es lo que avanza la salida en cada tramo y lo que dura el fundido: 10 ms.
	pasoWSOLA = 480
	// holguraWSOLA es cuánto se puede mover cada tramo para casar: 5 ms, más que el
	// periodo de una voz grave (≈ 100 Hz), así que siempre hay un punto en fase.
	holguraWSOLA = 240
)

// estirador cambia la duración del PCM que le pasa por delante sin tocar el tono. El
// cero es «sin empezar»; a 1× no se usa (el PCM pasa tal cual).
type estirador struct {
	vel float64
	ent []int16 // entrada pendiente; ent[0] es la muestra `origen` de la entrada
	// origen es la posición absoluta (en la entrada) de ent[0].
	origen int
	// pos es dónde tocaría empezar el siguiente tramo (posición absoluta, fraccionaria).
	pos float64
	// elegido es dónde empezó el último tramo; su continuación natural, la que se funde
	// con el siguiente, es ent[elegido+paso : elegido+2·paso].
	elegido  int
	empezado bool
	// salida es lo calculado del tramo en curso que aún no se ha escrito; buf, su
	// memoria, que se reutiliza.
	salida, buf []int16
}

// necesita es hasta qué posición absoluta tiene que haber entrada para el siguiente
// tramo.
func (e *estirador) necesita() int {
	if !e.empezado {
		return 2 * pasoWSOLA
	}
	// La continuación del último tramo también: a menos de 1× va por delante de pos.
	return max(int(e.pos)+holguraWSOLA+pasoWSOLA, e.elegido+2*pasoWSOLA)
}

// leer rellena `out` estirando lo que va sacando de `fuente` (que se va comiendo).
// Devuelve cuántas muestras ha escrito: menos que len(out) si se acaba la fuente.
func (e *estirador) leer(out []int16, fuente *[]int16) int {
	escrito := 0
	for escrito < len(out) {
		// Lo que se puede escribir del tramo en curso. Se trabaja tramo a tramo y se
		// guarda en `ent` lo que sobra: la tarjeta pide de 20 en 20 ms.
		if falta := e.necesita() - (e.origen + len(e.ent)); falta > 0 {
			n := min(falta, len(*fuente))
			e.ent = append(e.ent, (*fuente)[:n]...)
			*fuente = (*fuente)[n:]
			if n < falta {
				return escrito
			}
		}
		if len(e.salida) == 0 {
			e.tramo()
		}
		n := copy(out[escrito:], e.salida)
		e.salida = e.salida[n:]
		escrito += n
	}
	return escrito
}

// tramo calcula los `paso` siguientes de la salida. Hay entrada de sobra (necesita).
func (e *estirador) tramo() {
	if !e.empezado {
		// El primero va tal cual: no hay nada con lo que fundirlo.
		e.empezado = true
		e.elegido = e.origen
		e.buf = append(e.buf[:0], e.ent[:pasoWSOLA]...)
		e.salida = e.buf
		e.pos = float64(e.origen) + pasoWSOLA*e.vel
		return
	}
	cont := e.ent[e.elegido+pasoWSOLA-e.origen:][:pasoWSOLA]
	// El candidato que más se parece a la continuación natural, entre pos ± holgura (sin
	// salirse de lo que hay ni volver a antes de la entrada guardada).
	nominal := int(e.pos)
	mejor, mejorC := nominal, -1e300
	for d := -holguraWSOLA; d <= holguraWSOLA; d++ {
		c := nominal + d
		if c < e.origen {
			continue
		}
		cand := e.ent[c-e.origen:][:pasoWSOLA]
		var s, n2 float64
		// Cada cuarta muestra basta para encontrar la fase y cuesta la cuarta parte.
		for i := 0; i < pasoWSOLA; i += 4 {
			x := float64(cand[i])
			s += x * float64(cont[i])
			n2 += x * x
		}
		// Correlación normalizada por la energía del candidato: si no, gana el tramo
		// más fuerte y no el que está en fase.
		if n2 > 0 {
			s /= math.Sqrt(n2)
		}
		if s > mejorC {
			mejor, mejorC = c, s
		}
	}
	cand := e.ent[mejor-e.origen:][:pasoWSOLA]
	salida := e.buf[:0]
	for i := range pasoWSOLA {
		w := float64(i) / pasoWSOLA
		salida = append(salida, int16(float64(cont[i])*(1-w)+float64(cand[i])*w)) //nolint:gosec // G115: la mezcla de dos int16 cabe en int16
	}
	e.salida, e.buf = salida, salida
	e.elegido = mejor
	e.pos += pasoWSOLA * e.vel
	// Lo que ya no puede volver a hacer falta se suelta.
	if corte := min(int(e.pos)-holguraWSOLA, e.elegido+pasoWSOLA) - e.origen; corte > 0 {
		e.ent = append(e.ent[:0], e.ent[corte:]...)
		e.origen += corte
	}
}

// soltar devuelve, sin estirar, lo que queda por sonar desde donde iba (lo que está
// por escribir del tramo en curso y la continuación natural de la entrada), y deja el
// estirador vacío. Es lo que se hace al volver a 1× o al cambiar de velocidad: el
// sonido sigue justo donde estaba, sin saltos.
func (e *estirador) soltar() []int16 {
	var resto []int16
	if e.empezado {
		resto = append(resto, e.salida...)
		if desde := e.elegido + pasoWSOLA - e.origen; desde < len(e.ent) {
			resto = append(resto, e.ent[desde:]...)
		}
	} else {
		resto = append(resto, e.ent...)
	}
	*e = estirador{vel: e.vel}
	return resto
}
