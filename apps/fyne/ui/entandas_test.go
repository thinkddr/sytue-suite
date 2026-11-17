// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// TestEnTandasNoPasaDelCupoNiCortaPorUnFallo: lo que falla se cuenta aparte, el resto se
// hace, y nunca van más llamadas a la vez de las pedidas.
//
// VISTO EN ROJO (2026-11-05): con el cupo del tamaño de la lista, el pico sale 20; y
// apuntando el error como hecho, «b» no aparece entre los fallidos.
func TestEnTandasNoPasaDelCupoNiCortaPorUnFallo(t *testing.T) {
	hechos, fallidos := EnTandas([]string{"a", "b", "c", "d"}, func(x string) error {
		if x == "b" {
			return errors.New("403")
		}
		return nil
	}, 2)
	slices.Sort(hechos)
	if !slices.Equal(hechos, []string{"a", "c", "d"}) || !slices.Equal(fallidos, []string{"b"}) {
		t.Fatalf("hechos %q y fallidos %q", hechos, fallidos)
	}
	var enVuelo, pico atomic.Int32
	n := make([]int, 20)
	hechosN, _ := EnTandas(n, func(int) error {
		v := enVuelo.Add(1)
		for {
			p := pico.Load()
			if v <= p || pico.CompareAndSwap(p, v) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		enVuelo.Add(-1)
		return nil
	}, 3)
	if len(hechosN) != 20 || pico.Load() > 3 {
		t.Fatalf("hechos %d, pico %d (cupo 3)", len(hechosN), pico.Load())
	}
}
