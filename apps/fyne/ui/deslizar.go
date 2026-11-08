// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "math"

// La pareja en la web es apps/frontend/packages/ui/src/lib/deslizar.ts.
//
// DESLIZAR CON EL DEDO: cuándo un arrastre horizontal es «pasa a lo siguiente». Nació en
// el visor de Sytue Photos (pasar de foto) y Calendar la necesita igual para cambiar de
// mes en el teléfono: sería la tercera copia, así que la regla vive aquí. Mismos umbrales
// y mismos casos de test que la web (TestDeslizarEsLaReglaDeLaWeb): si se cambia un
// umbral allí, se cambia aquí.
const (
	recorridoMinDeslizar    = 56   // RECORRIDO_MIN: lo menos que recorre el dedo, en px
	fraccionAnchoDeslizar   = 0.18 // FRACCION_ANCHO: o esta fracción del ancho, si es mayor
	recorridoRapidoDeslizar = 24   // RECORRIDO_RAPIDO: y aun rápido, al menos esto
)

// VelocidadRapida es `VELOCIDAD_RAPIDA`: un golpe rápido, en px por milisegundo. Es
// pública porque el asa del mes del teléfono (Calendar) llama «lanzamiento» a lo mismo.
const VelocidadRapida = 0.5

// DominioDeslizamiento es `DOMINIO_HORIZONTAL`: horizontal de verdad es |dx| > 1,4·|dy|.
// Es pública porque quien sigue al dedo mientras arrastra (el visor de Photos) decide con
// ella si mueve la foto o la deja quieta.
const DominioDeslizamiento = 1.4

// DecidirDeslizamiento es `decidirDeslizamiento`: −1 lo anterior, 1 lo siguiente, 0 nada.
// Hacia la IZQUIERDA (dx < 0) se va a lo siguiente, como al pasar una página. `ancho` es
// el del área que se desliza y `ms` lo que duró el gesto.
func DecidirDeslizamiento(dx, dy, ancho float32, ms float64) int {
	ax := math.Abs(float64(dx))
	if ax < DominioDeslizamiento*math.Abs(float64(dy)) {
		return 0
	}
	lejos := ax >= math.Max(recorridoMinDeslizar, fraccionAnchoDeslizar*float64(ancho))
	rapido := ax >= recorridoRapidoDeslizar && ax/math.Max(ms, 1) >= VelocidadRapida
	if !lejos && !rapido {
		return 0
	}
	if dx < 0 {
		return 1
	}
	return -1
}
