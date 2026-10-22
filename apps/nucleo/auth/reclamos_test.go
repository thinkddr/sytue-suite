// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"encoding/base64"
	"testing"
)

// jwtDe arma un token sin firma válida con esos claims: `ReclamosDe` no la mira.
func jwtDe(claims string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".firma"
}

// TestReclamosDecidenComoLaWeb: las mismas reglas que `puedeAdministrar`,
// `sinAdminPorPlan` y `esCuentaPersonal`, caso a caso. El caso que importa es el del
// administrador DELEGADO, que no es `admin` y entra.
func TestReclamosDecidenComoLaWeb(t *testing.T) {
	casos := []struct {
		nombre          string
		claims          string
		personal, admin bool
	}{
		{"miembro sin más", `{"sub":"a","tenant":"acme","roles":["member"]}`, false, false},
		{"owner", `{"tenant":"acme","roles":["owner"]}`, false, true},
		{"admin", `{"tenant":"acme","roles":["admin"]}`, false, true},
		{"delegado", `{"tenant":"acme","roles":["member"],"perms":["users.read"]}`, false, true},
		{"personal gratuita", `{"tenant":"p-ada","org_kind":"personal","roles":["owner"],"plan":"personal_free"}`, true, false},
		{"personal de pago", `{"org_kind":"personal","roles":["owner"],"plan":"pro"}`, true, true},
		{"personal sin org_kind", `{"tenant":"personal-ada","roles":["owner"]}`, true, false},
		{"no es un jwt", ``, false, false},
	}
	for _, c := range casos {
		r := ReclamosDe(jwtDe(c.claims))
		if c.claims == "" {
			r = ReclamosDe("no-es-un-jwt")
		}
		if r.EsCuentaPersonal() != c.personal || r.PuedeAdministrar() != c.admin {
			t.Errorf("%s: personal=%v admin=%v, y la web dice %v y %v",
				c.nombre, r.EsCuentaPersonal(), r.PuedeAdministrar(), c.personal, c.admin)
		}
	}
	if s := ReclamosDe(jwtDe(`{"sub":"ada"}`)).Sub; s != "ada" {
		t.Errorf("el sub sale %q", s)
	}
}
