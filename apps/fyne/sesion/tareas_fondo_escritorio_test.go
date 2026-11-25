// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// El Hub en la bandeja lanza la tarea de cada app hermana tras la primera espera y
// luego cada periodo, solo con alguien dentro, y deja de hacerlo al cerrarse.
//
// VISTO EN ROJO (2026-11-25): sin el bucle, la copia de Photos con la app cerrada no
// corría nunca en el escritorio.
func TestElHubDespiertaLasTareasDeLasAppsCerradas(t *testing.T) {
	var mu sync.Mutex
	var lanzadas []string
	antes := lanzarTareaHermana
	t.Cleanup(func() { lanzarTareaHermana = antes })
	lanzarTareaHermana = func(_ context.Context, producto string) error {
		mu.Lock()
		defer mu.Unlock()
		lanzadas = append(lanzadas, producto)
		return errors.New("salió con 1") // un fallo no para el bucle
	}
	var dentro atomic.Bool
	ctx, parar := context.WithCancel(t.Context())
	hecho := make(chan struct{})
	go func() {
		despertarCada(ctx, 5*time.Millisecond, 5*time.Millisecond, dentro.Load, []string{"photos", "otra"})
		close(hecho)
	}()
	cuantas := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(lanzadas)
	}
	time.Sleep(40 * time.Millisecond)
	if n := cuantas(); n != 0 {
		t.Fatalf("sin nadie dentro se lanzaron %d tareas", n)
	}
	dentro.Store(true)
	limite := time.Now().Add(2 * time.Second)
	for cuantas() < 4 && time.Now().Before(limite) {
		time.Sleep(5 * time.Millisecond)
	}
	parar()
	<-hecho
	mu.Lock()
	defer mu.Unlock()
	if len(lanzadas) < 4 || lanzadas[0] != "photos" || lanzadas[1] != "otra" || lanzadas[2] != "photos" {
		t.Errorf("se lanzaron %v", lanzadas)
	}
}

// Lanzada por el Hub, la app corre su tarea con la sesión guardada y sale con 0; si la
// tarea falla, con 1.
func TestTareaEnFondoEscritorio(t *testing.T) {
	nuevoBancoSinVentana(t)
	guardarMovil(t, "ana")
	resultado := Nuevos
	conTarea(t, func(context.Context, EnFondo) Resultado { return resultado })
	if c := TareaEnFondoEscritorio("eu.sytue.photos", "fotos"); c != 0 {
		t.Errorf("una ronda que sube sale con %d", c)
	}
	resultado = Fallo
	if c := TareaEnFondoEscritorio("eu.sytue.photos", "fotos"); c != 1 {
		t.Errorf("una ronda que falla sale con %d", c)
	}
}
