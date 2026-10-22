// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && cgo

package sistema

import (
	"strings"
	"testing"
)

// TestEnElMacElIdiomaLoDiceElSistema: con el entorno vacío —como arranca una app desde el
// Finder— el idioma sale de los ajustes del Mac, y ya en BCP 47. VISTO EN ROJO con
// `delSistema` devolviendo "" (el Mac en español daba el idioma por defecto).
func TestEnElMacElIdiomaLoDiceElSistema(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	got := Idioma()
	if got == "" || strings.ContainsAny(got, "_.") {
		t.Fatalf("Idioma() = %q en un Mac sin LANG", got)
	}
	t.Logf("el Mac dice %q", got)
}
