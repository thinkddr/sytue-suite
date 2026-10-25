// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"os"
	"strings"
	"testing"
)

func TestDominioArchivosAislaCuentaYCierre(t *testing.T) {
	a, revision := strings.Repeat("a", 64), strings.Repeat("b", 32)
	primero, err := identificadorDominioArchivos(a, revision)
	if err != nil {
		t.Fatal(err)
	}
	otraCuenta, _ := identificadorDominioArchivos(strings.Repeat("c", 64), revision)
	nuevaSesion, _ := identificadorDominioArchivos(a, strings.Repeat("c", 32))
	renovacion, _ := identificadorDominioArchivos(a, revision)
	if primero == otraCuenta || primero == nuevaSesion || primero != renovacion {
		t.Fatal("el SO debe aislar cuenta/cierre y conservar una renovación normal")
	}
	for _, par := range [][2]string{{"", revision}, {a, ""}, {strings.Repeat("z", 64), revision}, {a, strings.Repeat("z", 32)}, {"../" + a, revision}, {a, revision + "/"}} {
		if _, err := identificadorDominioArchivos(par[0], par[1]); err == nil {
			t.Fatalf("dominio inseguro: %q", par)
		}
	}
	swift, err := os.ReadFile("../../fyne/xcode/proveedor-archivos/Proveedor.swift")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(swift), `static let prefijo = "sytue-drive."`) {
		t.Fatal("host y extensión deben identificar el mismo dominio")
	}
}
