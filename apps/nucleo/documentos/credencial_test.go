// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// cuentaDoble es Sytue ID: cuenta las contraseñas que crea y las que revoca.
type cuentaDoble struct {
	creadas, revocadas []string
	revocarFalla       bool
}

func (c *cuentaDoble) Me(context.Context) (api.Profile, error) {
	return api.Profile{Email: "ana@ejemplo.eu"}, nil
}

func (c *cuentaDoble) CreateAppPassword(_ context.Context, etiqueta string) (api.AppPasswordNueva, error) {
	id := "ap" + string(rune('1'+len(c.creadas)))
	c.creadas = append(c.creadas, id)
	return api.AppPasswordNueva{AppPassword: api.AppPassword{ID: id, Label: etiqueta}, Password: "clave-" + id}, nil
}

func (c *cuentaDoble) RevokeAppPassword(_ context.Context, id string) error {
	if c.revocarFalla {
		return errors.New("sin red")
	}
	c.revocadas = append(c.revocadas, id)
	return nil
}

// enMemoria pone un almacén y un sistema de mentira, y devuelve qué se ha conectado.
func enMemoria(t *testing.T) (map[string][]byte, *[]string) {
	t.Helper()
	almacen := map[string][]byte{}
	var sistema []string
	l, g, b, h, c, d := leer, guardar, borrar, hayProveedor, conectar, desconectar
	t.Cleanup(func() { leer, guardar, borrar, hayProveedor, conectar, desconectar = l, g, b, h, c, d })
	leer = func(k string) ([]byte, error) { return almacen[k], nil }
	guardar = func(k string, v []byte) error { almacen[k] = v; return nil }
	borrar = func(k string) error { delete(almacen, k); return nil }
	hayProveedor = true
	conectar = func(c Credencial) error { sistema = append(sistema, "+"+c.Usuario+":"+c.Clave); return nil }
	desconectar = func(c Credencial) { sistema = append(sistema, "-"+c.ID) }
	return almacen, &sistema
}

// Una contraseña por equipo: la segunda vez se reutiliza; al salir se revoca y se borra;
// la que no se pudo revocar se revoca al volver, ANTES de crear la siguiente.
func TestCredencialDelSistema(t *testing.T) {
	ctx := context.Background()
	almacen, sistema := enMemoria(t)
	c := &cuentaDoble{}
	const url = "https://drive.ejemplo.eu/dav/"

	for range 2 {
		if err := Asegurar(ctx, c, url, "Sytue Drive · prueba"); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(c.creadas, []string{"ap1"}) {
		t.Fatalf("creadas = %v, quería una sola", c.creadas)
	}
	if !slices.Equal(*sistema, []string{"+ana@ejemplo.eu:clave-ap1", "+ana@ejemplo.eu:clave-ap1"}) {
		t.Fatalf("sistema = %v", *sistema)
	}

	// Sin token (la sesión ya no está): fuera la contraseña, queda el id.
	if err := Olvidar(); err != nil {
		t.Fatal(err)
	}
	if cred, _ := cargarCredencial(); cred.Clave != "" || cred.ID != "ap1" {
		t.Fatalf("tras Olvidar = %+v", cred)
	}
	if err := Asegurar(ctx, c, url, "x"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.revocadas, []string{"ap1"}) || !slices.Equal(c.creadas, []string{"ap1", "ap2"}) {
		t.Fatalf("revocadas = %v, creadas = %v", c.revocadas, c.creadas)
	}

	// Al salir sin red, se queda pendiente; con red, se revoca y no queda nada.
	c.revocarFalla = true
	if err := Retirar(ctx, c); err == nil {
		t.Fatal("Retirar sin red tenía que fallar")
	}
	if cred, _ := cargarCredencial(); cred.Clave != "" || cred.ID != "ap2" {
		t.Fatalf("tras Retirar sin red = %+v", cred)
	}
	c.revocarFalla = false
	if err := Retirar(ctx, c); err != nil {
		t.Fatal(err)
	}
	if len(almacen) != 0 || !slices.Equal(c.revocadas, []string{"ap1", "ap2"}) {
		t.Fatalf("almacén = %v, revocadas = %v", almacen, c.revocadas)
	}

	// Sin proveedor no se crea nada.
	hayProveedor = false
	if err := Asegurar(ctx, c, url, "x"); !errors.Is(err, ErrNoDisponible) || len(c.creadas) != 2 {
		t.Fatalf("sin proveedor: %v, creadas = %v", err, c.creadas)
	}
}
