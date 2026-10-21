// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestLasCarasDelSistemaSeBuscanPorSuNombre: cada cara se busca con el nombre que le
// da cada sistema —«Georgia Bold Italic.ttf» en el Mac, «georgiaz.ttf» en Windows,
// «JetBrainsMono-BoldItalic.ttf» la instalada a mano— y solo vale un TrueType.
func TestLasCarasDelSistemaSeBuscanPorSuNombre(t *testing.T) {
	c := candidatos("darwin", []string{"F"}, "Georgia", true, true)
	for _, quiero := range []string{"Georgia Bold Italic.ttf", "georgiaz.ttf"} {
		if !slices.Contains(c, filepath.Join("F", quiero)) {
			t.Errorf("falta %s en %v", quiero, c)
		}
	}
	c = candidatos("windows", []string{"F"}, "Courier New", false, true)
	if !slices.Contains(c, filepath.Join("F", "couri.ttf")) {
		t.Errorf("Courier New cursiva en Windows es couri.ttf: %v", c)
	}
	c = candidatos("darwin", []string{"F"}, "JetBrains Mono", true, false)
	if !slices.Contains(c, filepath.Join("F", "JetBrainsMono-Bold.ttf")) {
		t.Errorf("la instalada a mano va con guion: %v", c)
	}
	if !esTTF(Leer("Figtree-BoldItalic.ttf")) || esTTF([]byte("wOF2....")) {
		t.Error("la firma de TrueType no se reconoce")
	}
}

// TestLaFamiliaNoSaleDeLasCarpetasDeLetras (FYE-06): un `font-family` con ruta no
// se busca, y un fichero que no es normal (un FIFO) no se lee.
func TestLaFamiliaNoSaleDeLasCarpetasDeLetras(t *testing.T) {
	for _, f := range []string{"../../../../etc/passwd", `..\..\x`, "a/b", "C:x", "x\x00y"} {
		if _, ok := DelSistema(f, false, false); ok {
			t.Errorf("%q: se encontró una letra", f)
		}
		if familiaValida(f) {
			t.Errorf("%q pasa por nombre de familia", f)
		}
	}
	for _, f := range []string{"Georgia", "Times New Roman", "JetBrains Mono"} {
		if !familiaValida(f) {
			t.Errorf("%q es un nombre de familia", f)
		}
	}
	dir := t.TempDir()
	if _, err := leerLetra(dir); err == nil {
		t.Error("se leyó un directorio como letra")
	}
}
