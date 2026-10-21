// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La invitación dice la organización que invita por su nombre visible, si Sytue ID lo
// da, y si no por su identificador (decisión del dueño, 17/10).
func TestTextoInvitacionConElNombreDeLaOrganizacion(t *testing.T) {
	tr := traductor(t)
	inv := api.RoomsInvitacion{Organizacion: "acme", Clase: "direct", NombreDeQuienInvita: "Ana"}
	if s := TextoInvitacion(tr, inv); !strings.Contains(s, "acme") {
		t.Fatalf("sin nombre, el identificador: %q", s)
	}
	inv.NombreOrganizacion = "Acme Ingeniería S.L."
	for _, clase := range []string{"direct", "private"} {
		inv.Clase = clase
		if s := TextoInvitacion(tr, inv); !strings.Contains(s, "Acme Ingeniería S.L.") || strings.Contains(s, "acme") {
			t.Fatalf("%s: %q", clase, s)
		}
	}
}
