// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El token SCIM de una organización solo sirve para ESA organización, contra
// PostgreSQL real (mig 0055).
//
// Es la verja que hace posible que la misma instalación sirva a varias empresas:
// antes había UN token para todo el despliegue, ligado a una sola organización
// por una variable de entorno, así que este aislamiento no existía porque no
// hacía falta. Ahora sí, y es exactamente la clase de fallo que no se nota: un
// token que funciona de más aprovisiona igual, solo que en el directorio
// equivocado.
package http

import (
	"encoding/json"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// crearTokenSCIM pide el token desde la consola de una organización y devuelve
// el secreto, que solo se ve esta vez.
func (l *liveServer) crearTokenSCIM(t *testing.T, adminTok string) string {
	t.Helper()
	rr, out := l.do(t, http.MethodPost, "/api/v1/admin/scim/tokens", adminTok,
		map[string]any{"label": "Entra ID"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("crear token SCIM: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	secreto, _ := out["token"].(string)
	if secreto == "" {
		t.Fatalf("respuesta sin token: %v", out)
	}
	return secreto
}

func TestSCIMTokenDeUnaOrganizacionNoVeLaOtra(t *testing.T) {
	l := newLiveServer(t)

	orgA, tokA := l.orgAdmin(t, "empresa-a", "admin@empresa-a.example")
	orgB, tokB := l.orgAdmin(t, "empresa-b", "admin@empresa-b.example")

	// Una persona en cada organización, para poder distinguir los directorios.
	for _, caso := range []struct {
		org   domain.Organization
		email string
	}{{orgA, "gente@empresa-a.example"}, {orgB, "gente@empresa-b.example"}} {
		u := l.mkUser(t, caso.email, "una-clave-larga-123")
		if err := l.st.UpsertMembership(l.ctx, nil,
			domain.Membership{OrgID: caso.org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", caso.email, err)
		}
	}

	secretoA := l.crearTokenSCIM(t, tokA)
	secretoB := l.crearTokenSCIM(t, tokB)
	if secretoA == secretoB {
		t.Fatal("dos organizaciones recibieron el mismo token")
	}

	// El token de A ve el directorio de A y solo el de A.
	emails := func(tokenSCIM string) []string {
		t.Helper()
		rr, out := l.do(t, http.MethodGet, "/scim/v2/Users", tokenSCIM, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("SCIM /Users: código=%d cuerpo=%s", rr.Code, rr.Body)
		}
		crudo, _ := json.Marshal(out["Resources"])
		var recursos []struct {
			UserName string `json:"userName"`
		}
		_ = json.Unmarshal(crudo, &recursos)
		got := make([]string, 0, len(recursos))
		for _, r := range recursos {
			got = append(got, r.UserName)
		}
		return got
	}

	for _, caso := range []struct {
		token, suyo, ajeno string
	}{
		{secretoA, "gente@empresa-a.example", "gente@empresa-b.example"},
		{secretoB, "gente@empresa-b.example", "gente@empresa-a.example"},
	} {
		vistos := emails(caso.token)
		var tieneSuyo, tieneAjeno bool
		for _, e := range vistos {
			if e == caso.suyo {
				tieneSuyo = true
			}
			if e == caso.ajeno {
				tieneAjeno = true
			}
		}
		if !tieneSuyo {
			t.Errorf("el token no ve a los suyos (%s): %v", caso.suyo, vistos)
		}
		if tieneAjeno {
			t.Fatalf("FUGA ENTRE ORGANIZACIONES: el token ve a %s — %v", caso.ajeno, vistos)
		}
	}

	// Revocado, deja de valer en el acto: eso es lo que no se podía hacer cuando
	// el token vivía en una variable de entorno del despliegue.
	rr, out := l.do(t, http.MethodGet, "/api/v1/admin/scim/status", tokA, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("estado SCIM: código=%d", rr.Code)
	}
	lista, _ := out["tokens"].([]any)
	if len(lista) != 1 {
		t.Fatalf("tokens vivos=%d; quería 1", len(lista))
	}
	primero, _ := lista[0].(map[string]any)
	id, _ := primero["id"].(string)
	if rr, _ := l.do(t, http.MethodDelete, "/api/v1/admin/scim/tokens/"+id, tokA, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("revocar: código=%d", rr.Code)
	}
	if rr, _ := l.do(t, http.MethodGet, "/scim/v2/Users", secretoA, nil); rr.Code == http.StatusOK {
		t.Fatal("el token revocado sigue aprovisionando")
	}
}
