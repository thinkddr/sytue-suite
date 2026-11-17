// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"testing"
	"time"
)

func TestRespuestaDeCuentaNoCruzaGeneraciones(t *testing.T) {
	encoladas := make(chan func(), 4)
	despachar := func(f func()) { encoladas <- f }
	cambioCorreo, cambioGestor := "", false
	// Same account A returns after B, but with a distinct adopted revision.
	actual := "A:revision-1"
	anterior, cancelarAnterior := context.WithCancel(context.Background())
	t.Cleanup(cancelarAnterior)
	rolEnCurso := make(chan struct{})
	rolTerminado := make(chan struct{})
	pedirDatosCuenta(anterior, func() bool { return actual == "A:revision-1" }, func(context.Context) (string, error) {
		return "anterior@example.invalid", nil
	}, func(ctx context.Context) (bool, error) {
		close(rolEnCurso)
		<-ctx.Done()
		close(rolTerminado)
		return false, ctx.Err()
	}, despachar, func(s string) { cambioCorreo = s }, func(v bool) { cambioGestor = v })
	var anteriorEncolada func()
	select {
	case anteriorEncolada = <-encoladas:
	case <-time.After(5 * time.Second):
		t.Fatal("el correo anterior no se encoló")
	}
	select {
	case <-rolEnCurso:
	case <-time.After(5 * time.Second):
		t.Fatal("la lectura de rol no arrancó")
	}
	// La sesión cambió dos veces antes de que UI procese AlCambiar; el contexto
	// anterior sigue vivo y comparar solo el sujeto A no detectaría el retorno.
	actual = "B:revision-2"
	actual = "A:revision-3"
	anteriorEncolada()
	cancelarAnterior()
	select {
	case <-rolTerminado:
	case <-time.After(5 * time.Second):
		t.Fatal("el rol no recibió la cancelación")
	}
	if cambioCorreo != "" || cambioGestor {
		t.Fatal("un resultado de la sesión anterior cambió la cuenta")
	}
	nueva, cancelarNueva := context.WithCancel(context.Background())
	t.Cleanup(cancelarNueva)
	pedirDatosCuenta(nueva, func() bool { return actual == "A:revision-3" }, func(context.Context) (string, error) {
		return "nueva@example.invalid", nil
	}, func(context.Context) (bool, error) { return true, nil }, despachar,
		func(s string) { cambioCorreo = s }, func(v bool) { cambioGestor = v })
	for range 2 {
		select {
		case aplicar := <-encoladas:
			aplicar()
		case <-time.After(5 * time.Second):
			t.Fatal("la nueva sesión no pudo aplicar sus datos")
		}
	}
	if cambioCorreo != "nueva@example.invalid" || !cambioGestor {
		t.Fatalf("datos de la sesión nueva: %q, gestor=%v", cambioCorreo, cambioGestor)
	}
}
