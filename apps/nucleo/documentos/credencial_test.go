// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// cuentaDoble es Sytue ID: cuenta las contraseñas que crea y las que revoca.
type cuentaDoble struct {
	creadas, revocadas []string
	revocarFalla       bool
	sujeto, org        string
	prefijo            string
	alCrear            func()
}

func (c *cuentaDoble) Me(context.Context) (api.Profile, error) {
	return api.Profile{ID: c.sujeto, Email: "ana@ejemplo.eu"}, nil
}

func (c *cuentaDoble) CreateAppPassword(_ context.Context, etiqueta string) (api.AppPasswordNueva, error) {
	if c.alCrear != nil {
		c.alCrear()
	}
	id := c.prefijo + "ap" + string(rune('1'+len(c.creadas)))
	c.creadas = append(c.creadas, id)
	return api.AppPasswordNueva{AppPassword: api.AppPassword{ID: id, Label: etiqueta, OrgID: "uuid-" + c.org, Tenant: c.org}, Password: "clave-" + id}, nil
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
	l, g, b, h, c, d, x := leer, guardar, borrar, hayProveedor, conectar, desconectar, excluir
	t.Cleanup(func() { leer, guardar, borrar, hayProveedor, conectar, desconectar, excluir = l, g, b, h, c, d, x })
	excluir = func(context.Context) (func(), error) { return func() {}, nil }
	leer = func(k string) ([]byte, error) { return almacen[k], nil }
	guardar = func(k string, v []byte) error { almacen[k] = v; return nil }
	borrar = func(k string) error { delete(almacen, k); return nil }
	hayProveedor = true
	conectar = func(_ context.Context, c Credencial) error {
		sistema = append(sistema, "+"+c.Usuario+":"+c.Clave)
		return nil
	}
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
	if cred, _ := cargarCredencial(); cred.Clave != "" || len(cred.Pendientes) != 1 || cred.Pendientes[0].ID != "ap2" {
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

func TestCredencialNoAdoptaOtraCuentaNiOrganizacion(t *testing.T) {
	_, sistema := enMemoria(t)
	ctx := context.Background()
	a := Identidad{Emisor: "https://id.ejemplo.eu", Cliente: "mobile", Sujeto: "ana", Organizacion: "acme"}
	b := a
	b.Organizacion = "beta"
	c := &cuentaDoble{sujeto: "ana", org: "acme"}
	const url = "https://drive.ejemplo.eu/dav/"
	if err := AsegurarIdentidad(ctx, c, url, "x", a); err != nil {
		t.Fatal(err)
	}
	if err := AsegurarIdentidad(ctx, c, url, "x", a); err != nil {
		t.Fatal(err)
	}
	if len(c.creadas) != 1 {
		t.Fatal("duplicó la credencial en otro aviso de sesión")
	}
	c.org = "beta"
	if err := AsegurarIdentidad(ctx, c, url, "x", b); err != nil {
		t.Fatal(err)
	}
	cred, _ := cargarCredencial()
	if cred.Dueno != b || cred.ID != "ap2" || len(cred.Pendientes) != 1 || cred.Pendientes[0].Dueno != a {
		t.Fatalf("cambio de organización: %+v", cred)
	}
	if len(c.revocadas) != 0 {
		t.Fatalf("intentó revocar con la cuenta nueva: %v", c.revocadas)
	}
	c.org = "acme"
	if err := AsegurarIdentidad(ctx, c, url, "x", a); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.revocadas, []string{"ap1"}) {
		t.Fatalf("no revocó su pendiente: %v", c.revocadas)
	}
	if len(*sistema) != 6 {
		t.Fatalf("no retiró la conexión anterior antes de cambiar: %v", *sistema)
	}
}

func TestCredencialExigeOrganizacionFijadaYNoPublicaUnAltaCancelada(t *testing.T) {
	for _, cancelar := range []bool{false, true} {
		t.Run(map[bool]string{false: "tenant distinto", true: "cancelada"}[cancelar], func(t *testing.T) {
			_, sistema := enMemoria(t)
			i := Identidad{Emisor: "https://id.ejemplo.eu", Cliente: "mobile", Sujeto: "ana", Organizacion: "acme"}
			ctx, parar := context.WithCancel(context.Background())
			defer parar()
			c := &cuentaDoble{sujeto: "ana", org: "otra"}
			if cancelar {
				c.org = "acme"
				c.alCrear = parar
			}
			if err := AsegurarIdentidad(ctx, c, "https://drive.ejemplo.eu/dav/", "x", i); err == nil {
				t.Fatal("aceptó el alta inválida")
			}
			cred, _ := cargarCredencial()
			if cred.Clave != "" || len(cred.Pendientes) != 1 || len(*sistema) != 0 {
				t.Fatalf("publicó contraseña: %+v, conexiones %v", cred, *sistema)
			}
		})
	}
}

func TestRetiradaTardiaNoDesconectaLaCuentaNueva(t *testing.T) {
	_, sistema := enMemoria(t)
	a := Identidad{Emisor: "https://id.ejemplo.eu", Cliente: "desktop", Sujeto: "ana", Organizacion: "acme"}
	b := a
	b.Sujeto = "bea"
	ca := &cuentaDoble{sujeto: a.Sujeto, org: a.Organizacion, prefijo: "a-"}
	cb := &cuentaDoble{sujeto: b.Sujeto, org: b.Organizacion, prefijo: "b-"}
	const url = "https://drive.ejemplo.eu/dav/"
	if err := AsegurarIdentidad(t.Context(), ca, url, "x", a); err != nil {
		t.Fatal(err)
	}
	// La retirada de A queda esperando el cerrojo mientras otra app conecta B.
	// Los canales fijan el orden sin leer/escribir el almacén en paralelo.
	ctxA, cancelar := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelar()
	esperando, continuar := make(chan struct{}), make(chan struct{})
	excluir = func(ctx context.Context) (func(), error) {
		if ctx == ctxA {
			close(esperando)
			select {
			case <-continuar:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return func() {}, nil
	}
	fin := make(chan error, 1)
	go func() { fin <- RetirarIdentidad(ctxA, ca, a) }()
	select {
	case <-esperando:
	case <-ctxA.Done():
		t.Fatal("la retirada de A no alcanzó el cerrojo")
	}
	err := AsegurarIdentidad(t.Context(), cb, url, "x", b)
	antes := len(*sistema)
	close(continuar)
	retirada := <-fin
	if err != nil || retirada != nil {
		t.Fatalf("alta B: %v; retirada A: %v", err, retirada)
	}
	cred, err := cargarCredencial()
	if err != nil || cred.Dueno != b || cred.ID != "b-ap1" || cred.Clave != "clave-b-ap1" || cred.URL != url {
		t.Fatalf("la retirada antigua perdió la conexión B: credencial=%+v, error=%v", cred, err)
	}
	if len(*sistema) != antes || !slices.Equal(ca.revocadas, []string{"a-ap1"}) || len(cb.revocadas) != 0 || len(cred.Pendientes) != 0 {
		t.Fatalf("retirada fuera de su dueño: sistema=%v, revocadas A=%v B=%v, pendientes=%v", *sistema, ca.revocadas, cb.revocadas, cred.Pendientes)
	}
}
