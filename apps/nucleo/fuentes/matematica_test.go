// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

// Los SHA son los archivos originales de NotoSansMath-v3.000.zip: el TTF hinted
// y OFL.txt. El TTF coincide además con la cara observada por CDP en Chromium.
func TestMatematicaEsLaReleasePortableConSuLicencia(t *testing.T) {
	datos := Leer(Matematica)
	if got := fmt.Sprintf("%x", sha256.Sum256(datos)); got != "d51afd5739c7ba6c44fcab35a88160e25dfb69a2d4ad0bd99533f8d894af1f96" {
		t.Fatalf("Noto Sans Math hinted v3.000 SHA256 = %s", got)
	}
	licencia := Leer("OFL-NotoSansMath.txt")
	if got := fmt.Sprintf("%x", sha256.Sum256(licencia)); got != "c9c63b8ed6cf76b2ce0c58b241acfc1195c2c928334c989c7047cbdac58773d9" {
		t.Fatalf("la licencia OFL original debe distribuirse sin cambios: %s", got)
	}
	if !strings.Contains(string(licencia), "SIL Open Font License, Version 1.1") {
		t.Fatal("falta la licencia SIL OFL 1.1 en el recurso distribuido")
	}
	f, err := sfnt.Parse(datos)
	if err != nil {
		t.Fatal(err)
	}
	version, err := f.Name(nil, sfnt.NameIDVersion)
	if err != nil || !strings.Contains(version, "3.000") {
		t.Fatalf("versión de Noto Sans Math = %q: %v", version, err)
	}
	for _, r := range []rune{'←', '→'} {
		g, err := f.GlyphIndex(nil, r)
		if err != nil || g == 0 {
			t.Errorf("falta el glifo %U de los botones del moderador: %v", r, err)
		}
	}
	if slices.Contains(Nombres, Matematica) {
		t.Fatal("la reserva matemática no es una cara de la familia Figtree")
	}
}
