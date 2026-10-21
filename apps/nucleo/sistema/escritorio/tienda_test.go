// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// La Mac App Store prohíbe que una app se actualice por su cuenta: en la variante de
// tienda no hay plataforma en el catálogo de instaladores, y sin ella
// `sesion.BuscarActualizaciones` no pregunta nada. Corre con y sin `-tags tienda`.
func TestLaVarianteDeTiendaNoSeActualizaSola(t *testing.T) {
	if DeTienda {
		if p := Plataforma(); p != "" {
			t.Fatalf("la variante de tienda dice plataforma %q: buscaría actualizaciones propias", p)
		}
		return
	}
	if p := Plataforma(); p != plataformaDe(runtime.GOOS) {
		t.Fatalf("fuera de la tienda la plataforma es %q y se esperaba %q", p, plataformaDe(runtime.GOOS))
	}
}

// TestElDMGNoTocaLasAppsDeLaTienda: un .app con el recibo de la Mac App Store es de la
// tienda, y uno sin él (el del .dmg) no. Sin esto, el Hub del .dmg cambiaría por su copia
// las apps que la persona instaló desde la tienda en la misma carpeta.
func TestElDMGNoTocaLasAppsDeLaTienda(t *testing.T) {
	raiz := t.TempDir()
	tienda, dmg := filepath.Join(raiz, "Sytue Mail.app"), filepath.Join(raiz, "Sytue Drive.app")
	for _, d := range []string{filepath.Join(tienda, "Contents", "_MASReceipt"), filepath.Join(dmg, "Contents", "MacOS")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if InstaladaPorLaTienda(tienda) {
		t.Fatal("una carpeta _MASReceipt vacía no es un recibo")
	}
	if err := os.WriteFile(filepath.Join(tienda, "Contents", "_MASReceipt", "receipt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !InstaladaPorLaTienda(tienda) {
		t.Fatal("la app con el recibo de la Mac App Store no se reconoce como de la tienda")
	}
	if InstaladaPorLaTienda(dmg) {
		t.Fatal("la app del .dmg se toma por de la tienda: el Hub dejaría de ponerla al día")
	}
}
