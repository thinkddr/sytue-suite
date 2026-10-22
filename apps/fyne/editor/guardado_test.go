// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestElAutoguardadoAgrupaLosCambios: diez cambios seguidos son UN guardado, y Parar
// desarma el que queda pendiente.
func TestElAutoguardadoAgrupaLosCambios(t *testing.T) {
	var guardados atomic.Int32
	hecho := make(chan struct{}, 4)
	a := &Autoguardado{
		Espera:     20 * time.Millisecond,
		Guardar:    func() error { guardados.Add(1); return nil },
		AlTerminar: func(error) { hecho <- struct{}{} },
	}
	for range 10 {
		a.Cambio()
	}
	<-hecho
	a.Cambio()
	a.Parar()
	time.Sleep(60 * time.Millisecond)
	if n := guardados.Load(); n != 1 {
		t.Errorf("diez cambios seguidos y uno parado han dado %d guardados y tiene que ser uno", n)
	}
}

// TestAhoraGuardaYDesarmaElReloj: «Ahora» guarda en el acto lo pendiente y el reloj ya
// no vuelve a guardar lo mismo al vencer.
func TestAhoraGuardaYDesarmaElReloj(t *testing.T) {
	var guardados atomic.Int32
	a := &Autoguardado{Espera: 20 * time.Millisecond, Guardar: func() error { guardados.Add(1); return nil }}
	a.Cambio()
	a.Ahora()
	if n := guardados.Load(); n != 1 {
		t.Fatalf("«Ahora» ha dado %d guardados y tiene que ser uno, ya", n)
	}
	time.Sleep(60 * time.Millisecond)
	if n := guardados.Load(); n != 1 {
		t.Errorf("tras «Ahora» el reloj ha vuelto a guardar: %d guardados", n)
	}
}
