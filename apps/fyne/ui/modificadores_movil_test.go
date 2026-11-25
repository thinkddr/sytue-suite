// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"os"
	"regexp"
	"testing"
)

// La guarda se quita sola con el fork que trae el parche 0023, y solo con él.
func TestVersionConModificadores(t *testing.T) {
	for v, quiere := range map[string]bool{
		"v2.8.2-sytue.10": false,
		"v2.8.2-sytue.9":  false,
		"v2.8.2-sytue.11": true,
		"v2.8.2-sytue.12": true,
		"v2.8.3-sytue.1":  true,
		"v2.9.0-sytue.1":  true,
		"v2.8.1-sytue.20": false,
		"v2.8.2":          false, // Fyne sin el fork
		"":                false, // un go.work con un clon local: no se sabe
		"(devel)":         false,
	} {
		if got := VersionConModificadores(v); got != quiere {
			t.Errorf("%q: %v, y debería ser %v", v, got, quiere)
		}
	}
}

// La versión que lee la guarda es la del `replace` de go.mod: si la lectura dejara de
// funcionar, la guarda se quedaría puesta para siempre sin que nadie lo viera.
func TestLaVersionDeFyneSaleDelBinario(t *testing.T) {
	gomod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^replace fyne\.io/fyne/v2 => \S+ (\S+)$`).FindSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod no sustituye Fyne por el fork")
	}
	if got := versionFyne(); got != string(m[1]) {
		t.Errorf("el binario dice %q y go.mod %q", got, m[1])
	}
}

// En el escritorio los modificadores llegan siempre; en el móvil, según el fork.
func TestModificadoresEnElMovil(t *testing.T) {
	antes := esMovil
	t.Cleanup(func() { esMovil = antes })
	esMovil = func() bool { return false }
	if !ModificadoresEnElMovil() {
		t.Error("en el escritorio los modificadores siempre llegan")
	}
	esMovil = func() bool { return true }
	if ModificadoresEnElMovil() != VersionConModificadores(versionFyne()) {
		t.Error("en el móvil tiene que decidir la versión del fork")
	}
}
