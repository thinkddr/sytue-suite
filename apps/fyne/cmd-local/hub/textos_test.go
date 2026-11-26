// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaTablaDelHubCuadra: los textos propios del Hub están en los 26 idiomas (C-12), sin
// claves de más ni de menos, y cada traducción lleva las mismas `{variables}` que el
// inglés (en el orden que pida el idioma: se sustituyen por nombre).
func TestLaTablaDelHubCuadra(t *testing.T) {
	for _, f := range tabla.Faltan() {
		t.Error(f)
	}
	llaves := regexp.MustCompile(`\{[a-zA-Z]+\}`)
	conjunto := func(s string) []string {
		v := llaves.FindAllString(s, -1)
		slices.Sort(v)
		return v
	}
	for clave, v := range textos {
		if a, b := conjunto(v[0]), conjunto(v[1]); !slices.Equal(a, b) {
			t.Errorf("%q: el español lleva %v y el inglés %v", clave, a, b)
		}
	}
	for loc, tr := range traducciones {
		for clave, v := range tr {
			if a, b := conjunto(v), conjunto(textos[clave][1]); !slices.Equal(a, b) {
				t.Errorf("%s: %q lleva %v y el inglés %v", loc, clave, a, b)
			}
		}
	}
}

// texto sustituye las variables en cualquier idioma y cae al inglés en uno que no conoce.
func TestTextoDelHubPorIdioma(t *testing.T) {
	for loc, quiero := range map[i18n.Locale]string{
		"es": "Nada coincide con «x»", "en": "Nothing matches “x”", "de": "Keine Treffer für „x“",
		"fr": "Aucun résultat pour « x »", "pt-BR": "Nada corresponde a «x»", "xx": "Nothing matches “x”",
	} {
		if got := texto(loc, "sinResultados", "q", "x"); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
	}
	if got := texto("fr", "no.existe"); got != "no.existe" {
		t.Errorf("clave desconocida: %q", got)
	}
}
