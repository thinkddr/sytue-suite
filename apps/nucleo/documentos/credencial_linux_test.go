// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package documentos

import "testing"

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
