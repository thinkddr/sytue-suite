// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local"
)

// TestLosTextosDeDriveEstanEnTodosLosIdiomas: los textos propios de Drive tienen las
// MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del inglés: los de
// formato los mira Faltan, y los «{name}» que se sustituyen a mano, esta prueba.
func TestLosTextosDeDriveEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (local.Propios{Base: tabla, Traducciones: traducciones}).Faltan() {
		t.Error(f)
	}
	re := regexp.MustCompile(`\{[a-z]+\}`)
	llaves := func(s string) string {
		l := re.FindAllString(s, -1)
		slices.Sort(l)
		return strings.Join(l, " ")
	}
	for loc, tr := range traducciones {
		for clave, v := range tr {
			if quiero := llaves(tabla[clave][1]); llaves(v) != quiero {
				t.Errorf("%s: %q lleva %q y no %q", loc, clave, llaves(v), quiero)
			}
		}
	}
	// Los nombres de los documentos nuevos acaban en un nombre de fichero.
	for loc, tr := range traducciones {
		for _, clave := range tituloDe {
			if strings.ContainsAny(tr[clave], `/\:*?"<>|`) {
				t.Errorf("%s: %q no vale como nombre de fichero", loc, tr[clave])
			}
		}
	}
}
