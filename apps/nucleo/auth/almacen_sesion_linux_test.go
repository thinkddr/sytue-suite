// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestAlmacenSesionSeparaEmisoresYClientes(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	uno := (&OIDC{Issuer: "https://id.example"}).Almacen()
	mismo := (&OIDC{Issuer: "https://id.example/"}).Almacen()
	otro := (&OIDC{Issuer: "https://id.otro.example"}).Almacen()
	movil := (&OIDC{Issuer: "https://id.example", Cliente: ClientIDMovil}).Almacen()
	if uno.cuenta != mismo.cuenta || uno.cuenta == otro.cuenta || uno.cuenta == movil.cuenta {
		t.Fatal("el nombre estable no separa emisor y cliente")
	}
	if err := uno.Guardar(Tokens{Refresh: "privado", Subject: "persona"}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []*AlmacenSesion{otro, movil} {
		if tokens, err := a.Cargar(); err != nil || tokens.Refresh != "" {
			t.Fatalf("compartió con otro ámbito: %+v %v", tokens, err)
		}
	}
	leidos, err := mismo.Cargar()
	if err != nil || leidos.Refresh != "privado" || leidos.Issuer != uno.emisor || leidos.Cliente != ClientID {
		t.Fatalf("otra app del mismo ámbito no conserva la sesión: %+v %v", leidos, err)
	}
	if err := otro.Guardar(leidos); !errors.Is(err, ErrSesionAjena) {
		t.Fatalf("aceptó guardar un refresh ajeno: %v", err)
	}
}

func TestMigracionNoResucitaSesionTrasCerrarEnOtraApp(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := Save(Tokens{Refresh: "anterior", Subject: "persona"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrar(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if tks, _ := a.Cargar(); tks.Refresh != "" {
		t.Fatal("importó un token sin emisor a un entorno desconocido")
	}
	if err := a.Migrar(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if tks, _ := a.Cargar(); tks.Refresh != "anterior" {
		t.Fatal("la actualización perdió la sesión anterior")
	}
	if tks, _ := Load(); tks.Refresh != "" {
		t.Fatal("no retiró la copia antigua después de migrar")
	}
	if err := a.Guardar(Tokens{}); err != nil {
		t.Fatal(err)
	}
	// Simula la copia privada de otra app que se actualiza más tarde.
	if err := Save(Tokens{Refresh: "copia-vieja", Subject: "persona"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrar(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if tks, _ := a.Cargar(); tks.Refresh != "" {
		t.Fatal("otra actualización resucitó la sesión cerrada")
	}
	ruta, _ := a.marcaMigracion()
	if crudo, err := os.ReadFile(ruta); err != nil || string(crudo) != "1\n" {
		t.Fatalf("el testigo de migración contiene algo distinto de una marca: %q %v", crudo, err)
	}
}

func TestRenewNoPresentaRefreshAOtroEmisorOCliente(t *testing.T) {
	llamadas := 0
	idp := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { llamadas++ }))
	defer idp.Close()
	o := &OIDC{Issuer: idp.URL}
	for _, tokens := range []Tokens{
		{Refresh: "privado", Issuer: "https://otro.example", Cliente: ClientID},
		{Refresh: "privado", Issuer: idp.URL, Cliente: ClientIDMovil},
	} {
		if _, err := o.Renew(context.Background(), tokens); !errors.Is(err, ErrSesionAjena) {
			t.Fatalf("no rechazó ámbito ajeno: %v", err)
		}
	}
	if llamadas != 0 {
		t.Fatal("se presentó un refresh a un emisor/cliente distinto")
	}
}

func TestCierreImpideReutilizarUnaCopiaQueElLlaveroNoBorro(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := a.Guardar(Tokens{Refresh: "anterior", Subject: "persona"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Guardar(Tokens{}); err != nil {
		t.Fatal(err)
	}
	// Simula un borrado fallido o una copia vieja que reaparece del almacén.
	if err := guardarCuenta(a.cuenta, Tokens{Refresh: "vieja", Subject: "persona", Issuer: a.emisor, Cliente: a.cliente}); err != nil {
		t.Fatal(err)
	}
	if tokens, err := a.Cargar(); err != nil || tokens.Refresh != "" {
		t.Fatal("se abrió una copia antigua después del cierre compartido")
	}
	if err := a.Guardar(Tokens{Refresh: "login-explicito", Subject: "nueva"}); err != nil {
		t.Fatal(err)
	}
	if tokens, err := a.Cargar(); err != nil || tokens.Refresh != "login-explicito" {
		t.Fatal("un login explícito no retiró la marca de cierre")
	}
}
