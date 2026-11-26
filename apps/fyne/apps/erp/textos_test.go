// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLosTextosLocalesDelERPEstanEnTodosLosIdiomas: los textos propios de la edición
// local del ERP (los errores que solo contesta Sytue Office y los del envío por correo)
// tienen las MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del
// inglés: los de formato los mira Faltan, y «{ruta}», que se sustituye a mano, esta
// prueba.
func TestLosTextosLocalesDelERPEstanEnTodosLosIdiomas(t *testing.T) {
	for nombre, p := range map[string]i18n.Propios{
		"ErroresLocales": {Base: ErroresLocales, Traducciones: traduccionesErrores},
		"TextosEnvio":    {Base: TextosEnvio, Traducciones: traduccionesEnvio},
		"TextosVIES":     {Base: TextosVIES, Traducciones: traduccionesVIES},
	} {
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
	if !strings.Contains(TextosEnvio["adjuntaPDF"][1], "{ruta}") {
		t.Fatal("«adjuntaPDF» ya no lleva «{ruta}»: la pantalla la sustituye a mano")
	}
}

// Los textos locales salen en el idioma de quien mira, el del traductor de la pantalla:
// español e inglés de la tabla y los otros veinticuatro de las traducciones.
func TestLosTextosLocalesDelERPSalenEnSuIdioma(t *testing.T) {
	e := &api.Error{Status: http.StatusBadGateway, Body: `{"error":{"code":"correo_acceso","message":"x"}}`}
	for loc, quiero := range map[i18n.Locale]string{
		"de": traduccionesErrores["de"]["correo_acceso"],
		"pl": traduccionesErrores["pl"]["correo_acceso"],
		"pt": traduccionesErrores["pt"]["correo_acceso"],
		"es": ErroresLocales["correo_acceso"][0],
		"en": ErroresLocales["correo_acceso"][1],
	} {
		tr, err := Traductor(loc)
		if err != nil {
			t.Fatal(err)
		}
		if got := MensajeEnvio(e, tr); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
		if got, quiero := textoEnvio("desde", tr), (i18n.Propios{Base: TextosEnvio, Traducciones: traduccionesEnvio}).En(loc, "desde"); got != quiero || got == "" {
			t.Errorf("%s: «desde» es %q", loc, got)
		}
	}
	tr, err := Traductor("de")
	if err != nil {
		t.Fatal(err)
	}
	if got := textoEnvio("desde", tr); got != "Von" {
		t.Errorf("de: «desde» es %q", got)
	}
}

var reLlave = regexp.MustCompile(`\{[a-z]+\}`)

// llaves son los huecos «{nombre}» de un texto, ordenados.
func llaves(s string) string {
	l := reLlave.FindAllString(s, -1)
	slices.Sort(l)
	return strings.Join(l, " ")
}
