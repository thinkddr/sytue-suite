// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogos

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// TestDeProductoEsLoQueCargaCadaCmd: lo que traduce el widget sin el `main` de la app
// (Android, en segundo plano) son los mismos catálogos que carga su `cmd`. Sin esto, un
// «Sin título» saldría como la clave en el widget de una app que cambió de catálogo.
func TestDeProductoEsLoQueCargaCadaCmd(t *testing.T) {
	tomls, err := filepath.Glob("../cmd/*/FyneApp.toml")
	if err != nil || len(tomls) == 0 {
		t.Fatalf("sin los FyneApp.toml: %v", err)
	}
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([a-z-]+)"`)
	vistos := 0
	for _, ruta := range tomls {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		m := esquema.FindSubmatch(datos)
		if len(m) < 2 || string(m[1]) == "fyne-demo" {
			continue
		}
		producto, cmd := string(m[1]), "cmd/"+filepath.Base(filepath.Dir(ruta))
		quiere, hay := catalogo[cmd]
		if !hay {
			t.Errorf("%s no está en la tabla `catalogo`", cmd)
			continue
		}
		if got := deProducto[producto]; !slices.Equal(got, quiere) {
			t.Errorf("DeProducto(%q) = %v; %s carga %v", producto, got, cmd, quiere)
		}
		if len(DeProducto(producto)) != len(quiere) {
			t.Errorf("DeProducto(%q) no abre sus catálogos", producto)
		}
		vistos++
	}
	if vistos != 15 || len(deProducto) != 15 {
		t.Fatalf("vistas %d apps y %d productos; son quince", vistos, len(deProducto))
	}
}
