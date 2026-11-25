// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"sort"
	"strings"
	"testing"
)

// La lista blanca de la actividad es de la CUENTA: ninguna acción de administración
// (de una organización, de la plataforma, de clients o SCIM) puede colarse en lo que
// la persona ve como «su» actividad. El día que alguien añada una acción de esas al
// mapa, la pantalla enseñaría lo que la persona hizo sobre OTRAS cuentas.
func TestLaActividadSoloTraeAccionesDeLaCuenta(t *testing.T) {
	prohibidos := []string{"org.", "platform.", "admin", "scim.", "apps.", "sso.", "privacy.", "access_policy."}
	for accion, clase := range clasesDeActividad {
		for _, p := range prohibidos {
			if strings.HasPrefix(accion, p) || strings.Contains(accion, ".admin_") {
				t.Errorf("%q (%s) es de administración, no de la cuenta", accion, clase)
			}
		}
		if clase == "" {
			t.Errorf("%q sin clase pública", accion)
		}
	}
	acciones := accionesDeActividad()
	if len(acciones) != len(clasesDeActividad) || !sort.StringsAreSorted(acciones) {
		t.Errorf("accionesDeActividad no es la lista blanca entera y ordenada: %v", acciones)
	}
}

func TestNombrePasskeyValido(t *testing.T) {
	casos := []struct {
		entra, sale string
		vale        bool
	}{
		{"  Llave USB ", "Llave USB", true},
		{"", "", false},
		{"   ", "", false},
		{strings.Repeat("ñ", 60), strings.Repeat("ñ", 60), true}, // 60 runas, 120 bytes
		{strings.Repeat("a", 61), strings.Repeat("a", 61), false},
	}
	for _, c := range casos {
		sale, vale := nombrePasskeyValido(c.entra)
		if sale != c.sale || vale != c.vale {
			t.Errorf("nombrePasskeyValido(%q) = %q,%v; quiero %q,%v", c.entra, sale, vale, c.sale, c.vale)
		}
	}
}
