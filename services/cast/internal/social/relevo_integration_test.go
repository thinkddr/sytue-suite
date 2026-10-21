// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// esperarTrama espera a que la conexión reciba una trama que cumpla `vale`.
func esperarTrama(c *Cliente, espera time.Duration, vale func(Trama) bool) bool {
	fin := time.After(espera)
	for {
		select {
		case t := <-c.salida:
			if vale(t) {
				return true
			}
		case <-fin:
			return false
		}
	}
}

// Dos réplicas sobre la misma base: lo que una difunde en el chat llega a las
// conexiones de la otra, el bloqueo recién puesto en una vale en la otra, y el
// recuento de espectadores no se mezcla.
func TestRelevoEntreReplicas(t *testing.T) {
	h := montar(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	acc := nuevoAcceso()
	a, b := New(h.Pool, acc.resolver), New(h.Pool, acc.resolver)
	a.Relevar(ctx)
	b.Relevar(ctx)

	video := uuid.New()
	enA, enB := clienteDePrueba(), clienteDePrueba()
	enB.quien = Quien{Sub: "bea"}
	a.hub.alta("acme", video, enA)
	b.hub.alta("acme", video, enB)
	zoe := Autor{Sub: "zoe", Name: "Zoe"}

	// El LISTEN de la otra réplica tarda en estar puesto: se reintenta hasta que llega.
	llego := false
	for i := 0; i < 50 && !llego; i++ {
		a.hub.Difundir("acme", video, Trama{Type: TramaMensaje, Author: &zoe, Body: "hola"})
		llego = esperarTrama(enB, 100*time.Millisecond, func(t Trama) bool { return t.Body == "hola" })
	}
	if !llego {
		t.Fatal("el mensaje difundido en una réplica no llega a la otra")
	}
	drenar(enA)
	drenar(enB)

	// Sin eco: la réplica que difunde lo entrega una sola vez a los suyos.
	a.hub.Difundir("acme", video, Trama{Type: TramaMensaje, Author: &zoe, Body: "una vez"})
	if !esperarTrama(enB, 3*time.Second, func(t Trama) bool { return t.Body == "una vez" }) {
		t.Fatal("el segundo mensaje no llega a la otra réplica")
	}
	time.Sleep(300 * time.Millisecond)
	if got := drenar(enA); len(got) != 1 {
		t.Fatalf("la réplica que difunde entrega %d veces, quiero 1: %+v", len(got), got)
	}

	// Bea bloquea a Zoe en la réplica A; su conexión está en B.
	a.hub.bloqueo("bea", "zoe", true)
	deadline := time.Now().Add(3 * time.Second)
	for !enB.bloquea("zoe") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !enB.bloquea("zoe") {
		t.Fatal("el bloqueo puesto en una réplica no llega a la conexión abierta en la otra")
	}
	a.hub.Difundir("acme", video, Trama{Type: TramaMensaje, Author: &zoe, Body: "bloqueada"})
	if esperarTrama(enB, 500*time.Millisecond, func(t Trama) bool { return t.Body == "bloqueada" }) {
		t.Fatal("a quien bloqueó le llega el mensaje desde otra réplica")
	}

	// El recuento de espectadores es de cada réplica: no viaja.
	drenar(enB)
	a.hub.Difundir("acme", video, tramaEspectadores(7))
	if esperarTrama(enB, 500*time.Millisecond, func(t Trama) bool { return t.Type == TramaEspectadores }) {
		t.Fatal("el recuento de una réplica se mezcla en la otra")
	}
}
