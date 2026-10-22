// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "testing"

// Con `leading-none` (medio interlineado negativo) la línea base sale de la cuenta de
// Blink a ×2 con el préstamo de Linux. Valores medidos el 2026-09-25 con la insignia de
// `text-xs` semibold (subida 9,97, bajada 2,63, caja de 10,5): la web la pinta 8,5 por
// debajo del borde de la caja; la cuenta de antes daba 8, un píxel de dispositivo más
// arriba (VISTO EN ROJO en la sonda de pulido: filas 29-42 contra 30-43 de la web).
func TestLaLineaBaseConInterlineadoNegativoEsLaDeBlink(t *testing.T) {
	if got := lineaBaseApretada(10.5, 9.96875, 2.625); got != 8.5 {
		t.Errorf("línea base %v, la web 8,5", got)
	}
}
