// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// people:resolve contra PostgreSQL real: encuentra a alguien de OTRA organización
// por su @usuario o por cualquiera de sus direcciones, y no cuenta nada más
// (ADR 0040).
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestPeopleResolve_EncuentraAGenteDeOtraOrganizacion(t *testing.T) {
	l := newLiveServer(t)
	acme, _ := l.seedOrgAdmin(t, "acme")
	yo := l.addMember(t, acme, "yo@acme.test", domain.RoleMember)
	otra, _ := l.seedOrgAdmin(t, "otra")
	ajena := l.addMember(t, otra, "privado@gmail.test", domain.RoleMember)
	buzon, err := l.st.EnsureOrgMailbox(l.ctx, nil, otra.ID, ajena.ID, "")
	if err != nil || buzon == "" {
		t.Fatalf("buzón de la persona de fuera: %q %v", buzon, err)
	}
	tok := l.token(t, yo.ID, acme.Slug, []string{"member"})

	for _, q := range []string{"@" + ajena.Username, ajena.Username, buzon, "privado@gmail.test"} {
		rr, body := l.do(t, http.MethodPost, "/api/v1/people:resolve", tok, map[string]string{"query": q})
		if rr.Code != http.StatusOK || body["user_id"] != ajena.ID || body["username"] != ajena.Username {
			t.Fatalf("resolver %q: status=%d body=%v", q, rr.Code, body)
		}
		// Ni el correo de acceso —puede ser un Gmail— ni la organización salen.
		for _, campo := range []string{"email", "org", "tenant", "role"} {
			if _, hay := body[campo]; hay {
				t.Fatalf("resolver %q no debe devolver %q: %v", q, campo, body)
			}
		}
	}
	// Camino de vuelta: del identificador al nombre.
	if rr, body := l.do(t, http.MethodPost, "/api/v1/people:resolve", tok, map[string]string{"user_id": ajena.ID}); rr.Code != http.StatusOK || body["name"] != ajena.Name {
		t.Fatalf("por user_id: status=%d body=%v", rr.Code, body)
	}

	// Ni prefijos, ni quien no existe, ni una cuenta suspendida.
	for _, q := range []string{"nadie", "privado@", ajena.Username[:3]} {
		if rr, _ := l.do(t, http.MethodPost, "/api/v1/people:resolve", tok, map[string]string{"query": q}); rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
			t.Fatalf("%q: status=%d, quiero 404 o 400", q, rr.Code)
		}
	}
	if _, err := l.st.SetUserStatus(l.ctx, nil, ajena.ID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/people:resolve", tok, map[string]string{"query": "@" + ajena.Username}); rr.Code != http.StatusNotFound {
		t.Fatalf("suspendida: status=%d, quiero 404", rr.Code)
	}
}
