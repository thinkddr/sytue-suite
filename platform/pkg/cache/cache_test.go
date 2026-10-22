// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	c, err := New(context.Background(), "redis://"+s.Addr())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, s
}

func TestSetTTLGetDel(t *testing.T) {
	c, s := newTestClient(t)
	ctx := context.Background()

	if err := c.SetTTL(ctx, "presence:u1", "online", time.Minute); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}
	val, found, err := c.Get(ctx, "presence:u1")
	if err != nil || !found || val != "online" {
		t.Fatalf("Get = (%q, %v, %v); esperaba (online, true, nil)", val, found, err)
	}

	// La clave expira al avanzar el reloj de miniredis más allá del TTL.
	s.FastForward(2 * time.Minute)
	if _, found, err = c.Get(ctx, "presence:u1"); err != nil || found {
		t.Fatalf("tras TTL: found=%v err=%v; esperaba not-found", found, err)
	}

	if err := c.SetTTL(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}
	if err := c.Del(ctx, "k"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if _, found, _ = c.Get(ctx, "k"); found {
		t.Fatal("Del no borró la clave")
	}
}

func TestPublishSubscribe(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()

	ch, cancel, err := c.Subscribe(ctx, "fanout:room1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if err := c.Publish(ctx, "fanout:room1", []byte("hola")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case msg := <-ch:
		if string(msg) != "hola" {
			t.Fatalf("mensaje = %q; esperaba hola", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout esperando el mensaje publicado")
	}

	// cancel es idempotente y cierra el canal (drenando lo pendiente).
	cancel()
	cancel()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // cerrado: OK
			}
		case <-deadline:
			t.Fatal("el canal no se cerró tras cancel")
		}
	}
}

func TestDisabledNoop(t *testing.T) {
	c, err := New(context.Background(), "")
	if err != nil {
		t.Fatalf("New(\"\"): %v", err)
	}
	ctx := context.Background()

	if err := c.SetTTL(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("SetTTL deshabilitado: %v", err)
	}
	if _, found, err := c.Get(ctx, "k"); err != nil || found {
		t.Fatalf("Get deshabilitado = (found=%v, err=%v); esperaba not-found sin error", found, err)
	}
	if err := c.Del(ctx, "k"); err != nil {
		t.Fatalf("Del deshabilitado: %v", err)
	}
	if err := c.Publish(ctx, "ch", []byte("x")); err != nil {
		t.Fatalf("Publish deshabilitado: %v", err)
	}
	ch, cancel, err := c.Subscribe(ctx, "ch")
	if err != nil {
		t.Fatalf("Subscribe deshabilitado: %v", err)
	}
	select {
	case <-ch:
		t.Fatal("el canal deshabilitado no debe entregar nada")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("el canal deshabilitado debe cerrarse tras cancel")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close deshabilitado: %v", err)
	}
}

func TestNewErrores(t *testing.T) {
	if _, err := New(context.Background(), "://mal"); err == nil {
		t.Fatal("URL inválida: esperaba error")
	}
	// Puerto cerrado => el Ping de arranque debe fallar (no arranque silencioso
	// creyéndose conectado).
	if _, err := New(context.Background(), "redis://127.0.0.1:1"); err == nil {
		t.Fatal("sin servidor: esperaba error de conexión")
	}
}

// Un Client nil NO puede panicar al preguntarle si está disponible.
//
// «Sin cache» es un estado normal —cualquier servicio sin DRAGONFLY_URL arranca
// así— y el patrón con el que se consulta es `if s.cache.Enabled()` sobre un campo
// que puede ser nil. Antes de esta guarda, ese `if` panicaba exactamente en el caso
// que la cache existe para tolerar. Se descubrió cableando el derecho de suscripción
// de Sytue ID, que es la puerta de entrada de toda la suite: habría muerto en la
// primera petición de cualquier producción sin Dragonfly.
func TestUnClienteNilNoPanica(t *testing.T) {
	var nulo *Client
	if nulo.Enabled() {
		t.Error("un Client nil se declara disponible")
	}
}
