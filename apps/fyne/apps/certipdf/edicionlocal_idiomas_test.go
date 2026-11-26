// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLosTextosLocalesEstanEnTodosLosIdiomas: los textos propios de la edición local de
// CertiPDF tienen las MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos
// «{n}» y «{carpeta}» del inglés, que `propio` sustituye.
func TestLosTextosLocalesEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (i18n.Propios{Base: textosLocales, Traducciones: traduccionesLocales}).Faltan() {
		t.Error(f)
	}
	re := regexp.MustCompile(`\{[a-z]+\}`)
	llaves := func(s string) string {
		l := re.FindAllString(s, -1)
		slices.Sort(l)
		return strings.Join(l, " ")
	}
	for loc, tr := range traduccionesLocales {
		for clave, v := range tr {
			if quiero := llaves(textosLocales[clave][1]); llaves(v) != quiero {
				t.Errorf("%s: %q lleva %q y no %q", loc, clave, llaves(v), quiero)
			}
		}
	}
}

func TestPropioSaleEnElIdiomaDeLaPantalla(t *testing.T) {
	for idioma, quiero := range map[string]string{"": "1 trusted certificate in Docs.", "es": "1 certificado de confianza en Docs.", "de": "1 vertrauenswürdiges Zertifikat in Docs.", "pt-BR": "1 certificado de confiança em Docs."} {
		p := &Pantalla{Idioma: idioma}
		if got := p.propio("anclasUno", "carpeta", "Docs"); got != quiero {
			t.Errorf("%q: %q; quiero %q", idioma, got, quiero)
		}
	}
}
