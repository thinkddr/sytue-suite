// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package altavoz

import (
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// trozos adapta una tarjeta que pide y da tramos de cualquier tamaño (CoreAudio da 512 o
// 470 muestras, según el equipo) al contrato de llamada.Dispositivo, que son tramos de
// 20 ms exactos: la captura se junta hasta completar uno y la reproducción se sirve de
// una cola que se rellena de tramo en tramo.
type trozos struct {
	capturado, reproducir func([]int16)

	mu      sync.Mutex
	entrada []int16
	salida  []int16
}

func (t *trozos) capturar(pcm []int16) {
	t.mu.Lock()
	t.entrada = append(t.entrada, pcm...)
	var listos [][]int16
	for len(t.entrada) >= llamada.Tramo {
		listos = append(listos, append([]int16(nil), t.entrada[:llamada.Tramo]...))
		t.entrada = t.entrada[llamada.Tramo:]
	}
	t.entrada = append(t.entrada[:0:0], t.entrada...)
	t.mu.Unlock()
	for _, tr := range listos {
		t.capturado(tr)
	}
}

func (t *trozos) rellenar(out []int16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for len(t.salida) < len(out) {
		tr := make([]int16, llamada.Tramo)
		t.reproducir(tr)
		t.salida = append(t.salida, tr...)
	}
	n := copy(out, t.salida)
	t.salida = append(t.salida[:0], t.salida[n:]...)
}
