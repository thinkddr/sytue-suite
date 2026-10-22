// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

// El límite del conector es POR TOKEN: abrir otra conexión TCP (otro puerto de origen)
// no estrena cubo, y lo que no tiene forma de token no crea claves nuevas.
func TestClaveDeConectorNoDependeDelPuerto(t *testing.T) {
	token := store.PrefijoTokenConector + strings.Repeat("a", 43)
	clave := func(auth, remoto string) string {
		r := httptest.NewRequest(http.MethodGet, "/_bases/v1/connector/read", http.NoBody)
		r.RemoteAddr = remoto
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		return claveDeConector(r)
	}
	if a, b := clave(token, "10.0.0.1:40001"), clave(token, "10.0.0.1:40002"); a != b {
		t.Errorf("otro puerto estrena cubo: %q y %q", a, b)
	}
	if a, b := clave(token, "10.0.0.1:1"), clave(store.PrefijoTokenConector+strings.Repeat("b", 43), "10.0.0.1:1"); a == b {
		t.Error("dos tokens distintos comparten cubo")
	}
	for _, malo := range []string{"", "x", "sbc_corto", strings.Repeat("z", 47), token + "z"} {
		if got := clave(malo, "10.0.0.9:5"); got != "conn:mal" {
			t.Errorf("%q crea la clave %q", malo, got)
		}
	}
	if strings.Contains(clave(token, "10.0.0.1:1"), token) {
		t.Error("la clave del límite lleva el token entero")
	}
}
