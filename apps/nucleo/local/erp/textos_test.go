// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLosTextosDeErpEstanEnTodosLosIdiomas: la franja de VERI*FACTU (D-29) y las
// plantillas del correo de una factura tienen las mismas claves en los 26 idiomas de la
// suite (C-12), con los huecos del inglés: los de formato los mira Faltan, y los
// «{numero}», «{importe}», «{vence}» y «{emisor}», que se sustituyen a mano, esta prueba.
func TestLosTextosDeErpEstanEnTodosLosIdiomas(t *testing.T) {
	for nombre, p := range map[string]i18n.Propios{"aviso": aviso, "envio": envio} {
		for _, f := range p.Faltan() {
			t.Errorf("%s: %s", nombre, f)
		}
		for clave, v := range p.Base {
			if llaves(v[0]) != llaves(v[1]) {
				t.Errorf("%s: %q no lleva los mismos huecos en español y en inglés", nombre, clave)
			}
		}
		for loc, tr := range p.Traducciones {
			for clave, v := range tr {
				if quiero := llaves(p.Base[clave][1]); llaves(v) != quiero {
					t.Errorf("%s: %s: %q lleva %q y no %q", nombre, loc, clave, llaves(v), quiero)
				}
			}
		}
	}
}

// TestElAvisoDeVerifactuEstaEnTodosLosIdiomas: la franja nombra VERI*FACTU y la AEAT tal
// cual en cada idioma, no es la inglesa en ninguno más, y una variante regional usa la de
// su idioma.
func TestElAvisoDeVerifactuEstaEnTodosLosIdiomas(t *testing.T) {
	en := AvisoVerifactu("en")
	for _, loc := range i18n.Locales {
		s := AvisoVerifactu(string(loc))
		switch {
		case !strings.Contains(s, "VERI*FACTU") || !strings.Contains(s, "AEAT") || !strings.Contains(s, "Sytue ERP"):
			t.Errorf("%s: el aviso no nombra Sytue ERP, VERI*FACTU o la AEAT: %q", loc, s)
		case loc != "en" && s == en:
			t.Errorf("%s: el aviso está en inglés", loc)
		}
	}
	if AvisoVerifactu("pt-BR") != traduccionesAviso["pt"]["verifactu"] || AvisoVerifactu("xx") != en {
		t.Error("una variante no cae en su idioma, o uno desconocido no cae al inglés")
	}
}

var reLlave = regexp.MustCompile(`\{[a-z]+\}`)

// llaves son los huecos «{nombre}» de un texto, ordenados.
func llaves(s string) string {
	l := reLlave.FindAllString(s, -1)
	slices.Sort(l)
	return strings.Join(l, " ")
}
