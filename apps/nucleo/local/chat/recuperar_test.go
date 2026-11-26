// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// cuentaSinRed es una cuenta cuya escucha cae por la red la primera vez (el resto, unas
// notas).
type cuentaSinRed struct {
	*Notas
	mu    sync.Mutex
	veces int
}

func (c *cuentaSinRed) Escuchar(ctx context.Context, _ func(api.RoomsEvent)) error {
	c.mu.Lock()
	c.veces++
	primera := c.veces == 1
	c.mu.Unlock()
	if primera {
		return &url.Error{Op: "Get", URL: "https://chat.ejemplo.eu", Err: errors.New("sin red")}
	}
	<-ctx.Done()
	return ctx.Err()
}

// Una cuenta que se cayó por la red y vuelve a contestar se avisa (la pantalla vuelve a
// pedir los canales); la que no se cae, no.
func TestAgregadorAvisaCuandoUnaCuentaVuelve(t *testing.T) {
	notas, err := AbrirNotas(t.TempDir(), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	a := NuevoAgregador("")
	a.Poner(CuentaNotas, notas)
	a.Poner("caida", &cuentaSinRed{Notas: notas})
	vueltas := make(chan string, 4)
	a.AlRecuperar = func(c string) { vueltas <- c }
	ctx, parar := context.WithTimeout(t.Context(), 5*time.Second)
	defer parar()
	go func() { _ = a.Listen(ctx, nil, func(api.RoomsEvent) {}) }()
	select {
	case c := <-vueltas:
		if c != "caida" {
			t.Fatal("avisó de", c)
		}
	case <-ctx.Done():
		t.Fatal("no avisó de que la cuenta volvía")
	}
	select {
	case c := <-vueltas:
		t.Fatal("avisó de más:", c)
	case <-time.After(200 * time.Millisecond):
	}
}
