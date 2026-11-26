// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package sieve

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Contra un ManageSieve de verdad (Dovecot con Pigeonhole), si se dice dónde:
//
//	SIEVE_SERVIDOR=127.0.0.1:4190 SIEVE_USUARIO=… SIEVE_CLAVE=… SIEVE_CA=cert.pem \
//	  go test -tags integration -run Dovecot ./cuentas/sieve/
//
// Sube unos scripts de prueba a la cuenta y la deja con la respuesta automática
// activa delante de «reglas-prueba», para mirarla con `sieve-test`.
func TestDovecotDeVerdad(t *testing.T) {
	dir := os.Getenv("SIEVE_SERVIDOR")
	if dir == "" {
		t.Skip("sin SIEVE_SERVIDOR")
	}
	raices := x509.NewCertPool()
	if ca := os.Getenv("SIEVE_CA"); ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			t.Fatal(err)
		}
		raices.AppendCertsFromPEM(pem)
	}
	host, _, _ := net.SplitHostPort(dir)
	abrirReal := func() *Cliente {
		conn, err := net.Dial("tcp", dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(parar)
		c, err := Conectar(ctx, conn, Opciones{Host: host, Raices: raices})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Autenticar(os.Getenv("SIEVE_USUARIO"), os.Getenv("SIEVE_CLAVE")); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Salir() })
		return c
	}
	c := abrirReal()
	t.Logf("servidor: %+v", c.Capacidades())
	// Se empieza de cero: nada activo y sin los scripts de pruebas anteriores.
	if err := c.Activar(""); err != nil {
		t.Fatal(err)
	}
	ss, err := c.Scripts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ss {
		if _, err := c.orden("DELETESCRIPT", s.Nombre); err != nil {
			t.Fatal(err)
		}
	}
	const reglasPrueba = "require [\"fileinto\"];\r\nif header :contains \"subject\" \"factura\" { fileinto \"Facturas\"; }\r\n"
	if err := c.Subir("reglas-prueba", reglasPrueba); err != nil {
		t.Fatal(err)
	}
	if err := c.Activar("reglas-prueba"); err != nil {
		t.Fatal(err)
	}
	// Un script que el servidor rechaza: llega su NO con el motivo.
	var es *ErrorServidor
	if err := c.Comprobar("require [\"no-existe\"];\r\nkeep;\r\n"); !errors.As(err, &es) || es.Respuesta != "NO" {
		t.Fatalf("CHECKSCRIPT de algo malo: %v", err)
	} else {
		t.Logf("rechazo del servidor: %v", err)
	}

	r := Respuesta{
		Asunto: `Fuera "de la oficina"`, Mensaje: "Hola:\n.\n..Vuelvo el lunes.\nUn saludo", Dias: 3,
		De: "ana@ejemplo.eu", Direcciones: []string{"ana@ejemplo.eu"},
		Desde: time.Now().AddDate(0, 0, -1), Hasta: time.Now().AddDate(0, 0, 10),
	}
	if err := Guardar(abrirReal(), propio, r, true); err != nil {
		t.Fatal(err)
	}
	st, err := LeerEstado(abrirReal(), propio)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Activa || st.Anterior != "reglas-prueba" || st.Asunto != r.Asunto || st.Mensaje != r.Mensaje || st.Dias != 3 || st.Desde.IsZero() || st.Hasta.IsZero() {
		t.Fatalf("estado leído de Dovecot: %+v", st)
	}
	texto, _ := abrirReal().Leer(propio)
	t.Logf("script en Dovecot:\n%s", texto)

	// Apagar devuelve «reglas-prueba».
	if err := Guardar(abrirReal(), propio, r, false); err != nil {
		t.Fatal(err)
	}
	c2 := abrirReal()
	ss, _ = c2.Scripts()
	for _, s := range ss {
		if s.Activo && s.Nombre != "reglas-prueba" {
			t.Fatalf("activo tras apagar: %q", s.Nombre)
		}
	}
	if got, _ := c2.Leer("reglas-prueba"); got != reglasPrueba {
		t.Fatalf("«reglas-prueba» cambió:\n%s", got)
	}
	// Un activo que ya responde no se combina.
	if err := c2.Subir("responde-prueba", "require [\"vacation\"];\r\nvacation \"fuera\";\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := c2.Activar("responde-prueba"); err != nil {
		t.Fatal(err)
	}
	var eb *ErrBloqueada
	if err := Guardar(abrirReal(), propio, r, true); !errors.As(err, &eb) {
		t.Fatalf("con otro que responde: %v", err)
	}
	// Se deja la respuesta activa delante de «reglas-prueba» para sieve-test.
	if err := abrirReal().Activar("reglas-prueba"); err != nil {
		t.Fatal(err)
	}
	if err := Guardar(abrirReal(), propio, r, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(texto, "include :personal :optional \"reglas-prueba\"") {
		t.Fatalf("sin include:\n%s", texto)
	}
}
