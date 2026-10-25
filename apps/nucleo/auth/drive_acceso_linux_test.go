// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func TestAlmacenDriveAccesoRenovacionCambioYSalidaSinRed(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	token := Tokens{Refresh: "refresh-sintetico", Subject: "ana", Organizacion: "acme"}
	soltar, err := ajustes.CerrojoSesion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	if err := a.Guardar(token); err != nil {
		t.Fatal(err)
	}
	antes, err := ajustes.LeerAccesoDrive()
	if err != nil || !antes.Activo {
		t.Fatalf("sin acceso tras entrar: %+v %v", antes, err)
	}
	token.Refresh = "refresh-rotado"
	if err := a.Guardar(token); err != nil {
		t.Fatal(err)
	}
	despues, _ := ajustes.LeerAccesoDrive()
	if despues != antes {
		t.Fatal("renovar OIDC cambió la autoridad del proveedor")
	}
	token.Organizacion = "otra"
	if err := a.Guardar(token); err != nil {
		t.Fatal(err)
	}
	otra, _ := ajustes.LeerAccesoDrive()
	if otra.Identidad == antes.Identidad || otra.Revision == antes.Revision {
		t.Fatal("cambiar de organización reutiliza autorización")
	}
	// Sin servidor ni token de red: el cierre local basta para bloquear File Provider.
	if err := a.Guardar(Tokens{}); err != nil {
		t.Fatal(err)
	}
	cerrado, _ := ajustes.LeerAccesoDrive()
	if cerrado.Activo {
		t.Fatal("salir sin red dejó el proveedor autorizado")
	}
}

func TestAlmacenDriveCargarVacioRevocaMarcaAnterior(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := a.publicarAccesoDrive(Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Cargar(); err != nil {
		t.Fatal(err)
	}
	estado, _ := ajustes.LeerAccesoDrive()
	if estado.Activo {
		t.Fatal("sin sesión guardada quedó acceso a Drive")
	}
}

// El cerrojo de la extensión puede sobrevivir al plazo de las dos publicaciones
// del cierre. Su contención no puede conservar ni el permiso ni el refresh.
func TestAlmacenDriveCierreConMarcaBloqueada(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := a.Guardar(Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}); err != nil {
		t.Fatal(err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "drive-acceso.cerrojo"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	soltar, err := ajustes.CerrojoFichero(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	inicio := time.Now()
	if err := a.Guardar(Tokens{}); err != nil {
		t.Errorf("no cerró con el cerrojo ocupado: %v", err)
	}
	if time.Since(inicio) < 250*time.Millisecond {
		t.Fatal("la prueba no ha ejercitado la contención real")
	}
	acceso, _ := ajustes.LeerAccesoDrive()
	if acceso.Activo {
		t.Error("el cierre conservó la autorización anterior")
	}
	// Leer el llavero, además de Cargar, acredita que sí se intentó retirar el
	// secreto y no sólo esconderlo detrás de una marca de cierre.
	guardado, err := cargarCuenta(a.cuenta)
	if err != nil || guardado.Refresh != "" {
		t.Errorf("no retiró el refresh: %v", err)
	}
}

func TestAlmacenDriveErrorDeMarcaNoImpideRetirarRefresh(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	tokens := Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}
	if err := a.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, ajustes.FicheroAccesoDrive)
	if err := os.Remove(ruta); err != nil {
		t.Fatal(err)
	}
	// Una carpeta no vacía impide tanto rename como remove incluso con UID 0.
	if err := os.Mkdir(ruta, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruta, "ocupado"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSession(tokens, nil, a.Guardar)
	if err := s.Logout(); !errors.Is(err, ErrSesionNoDisponible) {
		t.Fatalf("ocultó el fallo persistente: %v", err)
	}
	if s.Subject() != "" {
		t.Fatal("un fallo de disco mantuvo la sesión en memoria")
	}
	guardado, err := cargarCuenta(a.cuenta)
	if err != nil || guardado.Refresh != "" {
		t.Fatalf("no intentó borrar el refresh: %v", err)
	}
	marca, err := a.marcaCierre()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marca); err != nil {
		t.Fatalf("no dejó testigo de cierre: %v", err)
	}
}

func TestAlmacenDriveActualizarUsaSesionGuardadaYRespetaDesactivacion(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := a.Guardar(Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}); err != nil {
		t.Fatal(err)
	}
	if err := ajustes.Cambiar(func(v *ajustes.Ajustes) { v.DriveSistemaDesactivado = true }); err != nil {
		t.Fatal(err)
	}
	if err := a.ActualizarAccesoDrive(context.Background()); err != nil {
		t.Fatal(err)
	}
	acceso, _ := ajustes.LeerAccesoDrive()
	if acceso.Activo {
		t.Fatal("ignoró la desactivación canónica")
	}
	if err := a.Guardar(Tokens{}); err != nil {
		t.Fatal(err)
	}
	if err := ajustes.Cambiar(func(v *ajustes.Ajustes) { v.DriveSistemaDesactivado = false }); err != nil {
		t.Fatal(err)
	}
	if err := a.ActualizarAccesoDrive(context.Background()); err != nil {
		t.Fatal(err)
	}
	acceso, _ = ajustes.LeerAccesoDrive()
	if acceso.Activo {
		t.Fatal("el ajuste reabrió una sesión cerrada")
	}
}

func TestAlmacenDriveCarpetaInaccesibleNoImpideBorrarLlavero(t *testing.T) {
	aparte(t)
	keyring.MockInit()
	a := (&OIDC{Issuer: "https://id.example"}).Almacen()
	if err := a.Guardar(Tokens{Refresh: "sintetico", Subject: "ana", Organizacion: "acme"}); err != nil {
		t.Fatal(err)
	}
	// El cierre ya tiene su cerrojo cuando el volumen de ajustes deja de estar
	// disponible. El llavero sigue accesible y hay que intentar retirarlo también.
	soltar, err := ajustes.CerrojoSesion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+"-apartada"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("no es una carpeta"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Guardar(Tokens{}); err == nil {
		t.Fatal("ocultó el fallo del volumen")
	}
	if _, err := keyring.Get(llaveroServicio, a.cuenta); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("no intentó borrar el llavero: %v", err)
	}
}
