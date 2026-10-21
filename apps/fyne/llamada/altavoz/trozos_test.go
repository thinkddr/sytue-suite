// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package altavoz

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// De tramos de 512 (CoreAudio) a los de 960 que come la llamada, sin perder ni repetir
// una muestra, en los dos sentidos.
//
// VISTO EN ROJO (2026-09-26): sin guardar el resto de la captura, se pierden muestras.
func TestLosTrozosDeLaTarjeta(t *testing.T) {
	var capturado []int16
	sig := int16(0)
	tr := &trozos{
		capturado: func(p []int16) {
			if len(p) != llamada.Tramo {
				t.Fatalf("tramo de %d", len(p))
			}
			capturado = append(capturado, p...)
		},
		reproducir: func(p []int16) {
			for i := range p {
				p[i] = sig
				sig++
			}
		},
	}
	in := make([]int16, 512)
	for k := range 13 {
		for i := range in {
			in[i] = int16(k*512 + i)
		}
		tr.capturar(in)
	}
	if len(capturado) != 6*llamada.Tramo {
		t.Fatalf("de 13×512 (6 tramos y 896 de resto) salen %d muestras", len(capturado))
	}
	for i, v := range capturado {
		if int(v) != i {
			t.Fatalf("la muestra %d es %d", i, v)
		}
	}
	out := make([]int16, 512)
	for k := range 4 {
		tr.rellenar(out)
		if int(out[0]) != k*512 || int(out[511]) != k*512+511 {
			t.Fatalf("salida %d: %d…%d", k, out[0], out[511])
		}
	}
}
