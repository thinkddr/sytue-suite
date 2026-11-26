// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLosTextosDeMailEstanEnTodosLosIdiomas: los textos propios de Mail tienen las MISMAS
// claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del inglés: los de formato
// los mira Faltan, y los «{n}» que se sustituyen a mano, esta prueba.
func TestLosTextosDeMailEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (local.Propios{Base: textos, Traducciones: traducciones}).Faltan() {
		t.Error(f)
	}
	for loc, tr := range traducciones {
		for clave, v := range tr {
			if quiero := llaves(textos[clave][1]); llaves(v) != quiero {
				t.Errorf("%s: %q lleva %q y no %q", loc, clave, llaves(v), quiero)
			}
		}
		// El aviso nombra la opción tal como se ve en la lista de seguridad.
		if !strings.Contains(tr["sinCifrar"], tr["seguridad.ninguna"]) {
			t.Errorf("%s: el aviso sin cifrar no nombra %q", loc, tr["seguridad.ninguna"])
		}
	}
}

func TestLosTextosDeMailSalenEnSuIdioma(t *testing.T) {
	p := local.Propios{Base: textos, Traducciones: traducciones}
	for loc, quiero := range map[i18n.Locale]string{"es": "Conectar", "en": "Connect", "de": "Verbinden", "pl": "Połącz", "pt-BR": "Ligar", "xx": "Connect"} {
		if got := p.En(loc, "conectar"); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
	}
}

var reLlave = regexp.MustCompile(`\{[a-z]+\}`)

// llaves son los huecos «{nombre}» de un texto, ordenados.
func llaves(s string) string {
	l := reLlave.FindAllString(s, -1)
	slices.Sort(l)
	return strings.Join(l, " ")
}
