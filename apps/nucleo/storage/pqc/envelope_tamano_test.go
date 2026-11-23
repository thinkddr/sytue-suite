// SPDX-License-Identifier: Apache-2.0
package pqc

import (
	"math"
	"testing"
)

// Tamaños hostiles: una suma ingenua 4+cabecera+carga da la vuelta a int y queda
// por debajo del tope (o negativa), y make reservaría o entraría en pánico.
// tamanoRegistro debe rechazarlos todos sin sumar antes de comparar.
func TestTamanoRegistroRechazaDesbordamiento(t *testing.T) {
	hostiles := []struct{ cabecera, carga int }{
		{math.MaxInt, math.MaxInt},
		{math.MaxInt - 3, 0},
		{1, math.MaxInt - 4},
		{math.MaxInt / 2, math.MaxInt/2 + 10},
		{-8, 4},
		{4, -8},
		{MaxEnvelopePayload, 0},
		{0, MaxEnvelopePayload - 3},
		{MaxEnvelopePayload / 2, MaxEnvelopePayload / 2},
	}
	for _, h := range hostiles {
		if total, ok := tamanoRegistro(h.cabecera, h.carga); ok {
			t.Errorf("tamanoRegistro(%d, %d) = %d aceptado; debía rechazarse", h.cabecera, h.carga, total)
		}
	}
	if total, ok := tamanoRegistro(10, MaxEnvelopePayload-14); !ok || total != MaxEnvelopePayload {
		t.Fatalf("el máximo exacto debía caber: %d %v", total, ok)
	}
	if total, ok := tamanoRegistro(0, 0); !ok || total != 4 {
		t.Fatalf("el mínimo debía caber: %d %v", total, ok)
	}
}
