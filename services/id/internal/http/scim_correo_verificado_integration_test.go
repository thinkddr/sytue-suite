// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Un correo cambiado por SCIM deja de constar como verificado. `email_verified`
// dice que alguien demostró control de esa dirección; al cambiar `userName` por
// SCIM (PUT o PATCH) la dirección nueva heredaba el «verificado» de la antigua, y
// así salía en el claim `email` de los tokens, que Calendar, Hub, Mail y Cloud usan
// como identidad.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestSCIMCambiarElCorreoLoDejaSinVerificar(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: itSCIMOrg, Name: "SCIM Org"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	// Una persona de la organización con su correo ya verificado.
	u := l.mkUser(t, "ana@scimorg.example", "")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	if antes, _ := l.st.GetUserByID(l.ctx, u.ID); !antes.EmailVerified {
		t.Fatal("precondición: el correo de partida debe estar verificado")
	}

	patch := func(valor string) {
		t.Helper()
		rr, _ := l.do(t, http.MethodPatch, "/scim/v2/Users/"+u.ID, itSCIMToken, map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{{"op": "replace", "path": "userName", "value": valor}},
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("SCIM PATCH userName=%s: status=%d cuerpo=%s", valor, rr.Code, rr.Body)
		}
	}

	// Un PATCH que NO cambia la dirección (el IdP reenvía la misma, o con otras
	// mayúsculas) no toca la verificación.
	patch("Ana@SCIMORG.example")
	if igual, _ := l.st.GetUserByID(l.ctx, u.ID); !igual.EmailVerified {
		t.Fatal("reenviar la misma dirección no debe quitar la verificación")
	}

	// Cambiar la dirección sí: la nueva no la ha verificado nadie.
	patch("otra@fuera.example")
	despues, _ := l.st.GetUserByID(l.ctx, u.ID)
	if despues.Email != "otra@fuera.example" {
		t.Fatalf("el correo no cambió: %q", despues.Email)
	}
	if despues.EmailVerified {
		t.Fatal("un correo cambiado por SCIM sigue constando como verificado")
	}
}
