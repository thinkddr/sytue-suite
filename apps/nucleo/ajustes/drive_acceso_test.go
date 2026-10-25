// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccesoDriveRefreshNoInvalidaYLogoutEvitaABA(t *testing.T) {
	aparte(t)
	identidad := strings.Repeat("a", 64)
	if err := PonerAccesoDrive(identidad, true); err != nil {
		t.Fatal(err)
	}
	inicial, err := LeerAccesoDrive()
	if err != nil {
		t.Fatal(err)
	}
	if err := PonerAccesoDrive(identidad, true); err != nil {
		t.Fatal(err)
	}
	renovado, _ := LeerAccesoDrive()
	if renovado != inicial {
		t.Fatal("renovar la misma identidad cancelaría transferencias")
	}
	if err := PonerAccesoDrive("", false); err != nil {
		t.Fatal(err)
	}
	cerrado, _ := LeerAccesoDrive()
	if cerrado.Activo || cerrado.Revision == inicial.Revision {
		t.Fatal("el cierre conservó la autorización")
	}
	if err := PonerAccesoDrive(identidad, true); err != nil {
		t.Fatal(err)
	}
	reabierto, _ := LeerAccesoDrive()
	if reabierto.Revision == inicial.Revision || !reabierto.Activo {
		t.Fatal("A→salir→A reutiliza una transferencia anterior")
	}
}

func TestAccesoDriveNoReentraCerrojoSesion(t *testing.T) {
	aparte(t)
	soltar, err := CerrojoSesion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	ctx, cancelar := context.WithTimeout(context.Background(), time.Second)
	defer cancelar()
	if err := PonerAccesoDriveCon(ctx, strings.Repeat("a", 64), true); err != nil {
		t.Fatalf("se bloqueó bajo el cerrojo de sesión: %v", err)
	}
}

func TestAccesoDriveCerrojoRespetaCancelacion(t *testing.T) {
	dir := aparte(t)
	f, err := os.OpenFile(filepath.Join(dir, "drive-acceso.cerrojo"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	soltar, err := CerrojoFichero(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelar()
	if err := PonerAccesoDriveCon(ctx, strings.Repeat("a", 64), true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("no respetó cancelación: %v", err)
	}
}

func TestAccesoDriveErrorEscrituraRetiraPermisoAnterior(t *testing.T) {
	dir := aparte(t)
	ruta := filepath.Join(dir, FicheroAccesoDrive)
	if err := PonerAccesoDrive(strings.Repeat("a", 64), true); err != nil {
		t.Fatal(err)
	}
	denegado := errors.New("disco no disponible")
	if err := escribirAccesoDrive(ruta, "", false, func(string, []byte) error { return denegado }, os.Remove); !errors.Is(err, denegado) {
		t.Fatal(err)
	}
	if _, err := LeerAccesoDrive(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se conservó autorización anterior: %v", err)
	}
}

func TestAccesoDriveAusenteCorruptoYBooleanoAmbiguoFallanCerrado(t *testing.T) {
	dir := aparte(t)
	for _, dato := range []string{"", `{}`, `{"version":1,"active":1}`, `{"version":1,"active":true,"identity_hash":"ana@sytue.eu","revision":"00000000000000000000000000000000"}`} {
		if err := os.WriteFile(filepath.Join(dir, FicheroAccesoDrive), []byte(dato), 0o600); err != nil {
			t.Fatal(err)
		}
		if a, err := LeerAccesoDrive(); err == nil || a.Activo {
			t.Fatalf("aceptó marca inválida: %q", dato)
		}
	}
}

func TestAccesoDriveRetiradaTrasErrorNoAfirmaCerrarSiBorradoFalla(t *testing.T) {
	dir := aparte(t)
	ruta := filepath.Join(dir, FicheroAccesoDrive)
	if err := PonerAccesoDrive(strings.Repeat("a", 64), true); err != nil {
		t.Fatal(err)
	}
	bloqueado, denegado := errors.New("lock no disponible"), errors.New("borrado denegado")
	if err := retirarAccesoDriveTrasError(ruta, false, bloqueado, func(string) error { return denegado }); !errors.Is(err, bloqueado) || !errors.Is(err, denegado) {
		t.Fatalf("ocultó un cierre que no pudo persistir: %v", err)
	}
	estado, err := LeerAccesoDrive()
	if err != nil || !estado.Activo {
		t.Fatal("la prueba no conservaba el permiso inaccesible")
	}
	if err := retirarAccesoDriveTrasError(ruta, false, bloqueado, os.Remove); err != nil {
		t.Fatal(err)
	}
	if _, err := LeerAccesoDrive(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("la retirada no quitó el permiso")
	}
	// Un alta nunca se da por completada porque se haya retirado la anterior.
	if err := retirarAccesoDriveTrasError(ruta, true, bloqueado, os.Remove); !errors.Is(err, bloqueado) {
		t.Fatal(err)
	}
}
