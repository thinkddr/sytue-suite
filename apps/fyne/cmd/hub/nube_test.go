// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

// TestLaCarpetaSigueALaCuentaDeLaSesion (FYS-04, decidido por el dueño): al cambiar de
// organización, al salir o al entrar con otra cuenta, la carpeta de antes se para, y al
// volver se reanuda la suya; «Conectar» arranca la de quien está dentro.
func TestLaCarpetaSigueALaCuentaDeLaSesion(t *testing.T) {
	var (
		mu     sync.Mutex
		hechos []string
		ahora  nube.Cuenta
	)
	apuntar := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		hechos = append(hechos, s)
	}
	a, b := nube.Cuenta{Sub: "ana", Org: "acme"}, nube.Cuenta{Sub: "ana", Org: "beta"}
	c := &conexionNube{
		cuenta: func() nube.Cuenta { return ahora },
		fuente: func(nube.Cuenta) nube.Fuente { return nil },
		conectar: func(ctx context.Context, dir string, _ nube.Fuente) error {
			apuntar("arranca " + dir)
			<-ctx.Done()
			apuntar("para " + dir)
			return ctx.Err()
		},
		// Solo la de acme estaba conectada en este equipo.
		conectada: func(cu nube.Cuenta) (string, bool) { return cu.Org, cu == a },
		carpeta:   func(cu nube.Cuenta) (string, error) { return cu.Org, nil },
	}
	esperar := func(n int) {
		t.Helper()
		for fin := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
			mu.Lock()
			hay := slices.Clone(hechos)
			mu.Unlock()
			if len(hay) >= n {
				return
			}
			if time.Now().After(fin) {
				t.Fatalf("esperaba %d hechos y hay %q", n, hay)
			}
		}
	}
	ahora = a
	c.seguir() // arranca con la sesión de acme
	esperar(1)
	c.seguir() // otro aviso de la misma sesión: nada
	ahora = b
	c.seguir() // cambia a beta, que nunca se conectó: la de acme se para
	ahora = nube.Cuenta{}
	c.seguir() // sale
	ahora = a
	c.seguir() // vuelve a acme: se reanuda la suya
	esperar(3)
	ahora = b
	if dir, err := c.abrir(); err != nil || dir != "beta" { // «Conectar» con beta
		t.Fatalf("abrir = %q, %v", dir, err)
	}
	esperar(5)
	c.seguir() // ya es la de beta
	c.mu.Lock()
	c.pararLa()
	c.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	quiero := []string{"arranca acme", "para acme", "arranca acme", "para acme", "arranca beta", "para beta"}
	if !slices.Equal(hechos, quiero) {
		t.Errorf("hechos:\n%q\nquiero:\n%q", hechos, quiero)
	}
}
