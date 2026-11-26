// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/asistente"
)

// Lo que falla de la IA del sistema en el escritorio sale en el idioma de la app, no en
// el español del núcleo.
func TestElErrorDeLaIADelEscritorioSaleEnSuIdioma(t *testing.T) {
	for _, c := range []string{
		asistente.EscritorioFaltaAtajo, asistente.EscritorioFalloAtajo, asistente.EscritorioSinTexto,
		asistente.EscritorioFaltaFoundry, asistente.EscritorioFoundryParado,
	} {
		e := &asistente.ErrorEscritorio{Codigo: c, Detalle: "salida del atajo"}
		es := textoIAEscritorio("es", e)
		for _, loc := range []i18n.Locale{"de", "fr", "pl"} {
			got := textoIAEscritorio(loc, e)
			if got == es || strings.Contains(got, "ia.escritorio") || strings.Contains(got, "asistente:") || strings.Contains(got, "%!") {
				t.Errorf("%s %s: %q", loc, c, got)
			}
		}
	}
	if got := textoIAEscritorio("de", &asistente.ErrorEscritorio{Codigo: asistente.EscritorioFaltaAtajo}); !strings.Contains(got, asistente.AtajoIA) {
		t.Errorf("el nombre del atajo no sale: %q", got)
	}
}
