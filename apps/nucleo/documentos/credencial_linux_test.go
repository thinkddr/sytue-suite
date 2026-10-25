// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// GVfs recibe el usuario dentro (escapado: un correo lleva @) y nunca la contraseña.
func TestUbicacionGVfs(t *testing.T) {
	u, err := ubicacion(Credencial{URL: "https://drive.ejemplo.eu/dav/", Usuario: "ana@ejemplo.eu", Clave: "secreta"})
	if err != nil || u != "davs://ana%40ejemplo.eu@drive.ejemplo.eu/dav/" {
		t.Fatalf("ubicacion = %q, %v", u, err)
	}
	if _, err := ubicacion(Credencial{URL: "http://drive.ejemplo.eu/dav/"}); err == nil {
		t.Fatal("sin TLS no se monta")
	}
}

func TestMontajeGVfsRespetaLaCancelacionDeLaSesion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gio"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancelar := context.WithCancel(t.Context())
	cancelar()
	err := montar(ctx, Credencial{URL: "https://drive.ejemplo.eu/dav/", Usuario: "ana@ejemplo.eu", Clave: "fixture"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("el montaje sobrevivió al cierre de la sesión: %v", err)
	}
}
