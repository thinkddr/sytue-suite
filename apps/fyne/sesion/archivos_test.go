// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La contraseña del proveedor de archivos se revoca con el token del momento y solo
// entonces se olvida su id; una que Sytue ID ya no tiene (404) también se olvida, y un
// fallo la deja apuntada para el siguiente intento. Visto en rojo olvidando el id antes
// de revocar.
func TestRevocarArchivos(t *testing.T) {
	aparte(t)
	var estado int
	var vista, token string
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vista, token = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(estado)
	}))
	defer id.Close()
	cuenta := api.NewAccount(id.URL, tokenFijo("t-1"), nil)

	for _, c := range []struct {
		estado int
		queda  bool
	}{{http.StatusInternalServerError, true}, {http.StatusNoContent, false}, {http.StatusNotFound, false}} {
		if err := ApuntarArchivos("p1", "u1"); err != nil {
			t.Fatal(err)
		}
		estado = c.estado
		err := RevocarArchivos(context.Background(), cuenta)
		if (err != nil) != c.queda {
			t.Fatalf("con %d: %v", c.estado, err)
		}
		if vista != "DELETE /api/v1/me/app-passwords/p1" || token != "Bearer t-1" {
			t.Fatalf("con %d se ha pedido %q con %q", c.estado, vista, token)
		}
		if p, de, _ := ArchivosApuntada(); (p == "p1" && de == "u1") != c.queda {
			t.Fatalf("con %d queda apuntada %q de %q", c.estado, p, de)
		}
	}
	// Sin ninguna apuntada no se pregunta nada.
	vista = ""
	if err := RevocarArchivos(context.Background(), cuenta); err != nil || vista != "" {
		t.Fatalf("sin apuntada: %v, %q", err, vista)
	}
}
