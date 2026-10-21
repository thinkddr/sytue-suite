// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// LA CLAVE DE LA AGENDA SEPARA ORGANIZACIONES, Y ESO NO PUEDE ROMPERSE.
//
// Lo que se cachea es la plantilla de una organización: sus nombres, sus
// departamentos y sus direcciones internas. Si dos organizaciones compartieran
// clave, la segunda en preguntar recibiría la agenda de la primera — y no habría
// ni un error en ningún log, porque la respuesta es válida, solo que de otra
// gente.
func TestClaveAgendaSeparaOrganizaciones(t *testing.T) {
	t.Parallel()
	a := claveAgenda("org-a", "ana", "", 200)
	b := claveAgenda("org-b", "ana", "", 200)
	if a == b {
		t.Fatalf("dos organizaciones comparten clave: %s", a)
	}
}

// Y separa consultas: si no, la primera búsqueda se serviría para todas.
func TestClaveAgendaSeparaLoQueCambiaLaRespuesta(t *testing.T) {
	t.Parallel()
	base := claveAgenda("org", "ana", "cur", 200)
	casos := map[string]string{
		"otra consulta": claveAgenda("org", "luis", "cur", 200),
		"otro cursor":   claveAgenda("org", "ana", "otro", 200),
		"otro límite":   claveAgenda("org", "ana", "cur", 50),
	}
	for nombre, clave := range casos {
		if clave == base {
			t.Errorf("%s produce la misma clave que la base", nombre)
		}
	}
	if claveAgenda("org", "ana", "cur", 200) != base {
		t.Error("la misma consulta debería dar la misma clave")
	}
}

// La consulta es texto libre de hasta 128 caracteres y va HASHEADA: una clave de
// Dragonfly no es sitio para meter lo que alguien teclee, ni para que crezca sin
// tope.
func TestClaveAgendaNoLlevaLaConsultaCruda(t *testing.T) {
	t.Parallel()
	clave := claveAgenda("org", "ana:pérez\nsalto", "", 200)
	for _, prohibido := range []string{"pérez", "\n", " "} {
		if contiene(clave, prohibido) {
			t.Errorf("la clave lleva %q crudo: %s", prohibido, clave)
		}
	}
	if len(clave) > 80 {
		t.Errorf("clave de %d caracteres; debería estar acotada: %s", len(clave), clave)
	}
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
