// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Las organizaciones de una persona (ADR 0059): solo las activas, de una persona
// activa, con el secreto del directorio y solo desde dentro.
func TestOrganizacionesDeUnaPersona(t *testing.T) {
	l := newLiveServer(t)
	nueva := func(slug string) domain.Organization {
		o, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	acme, globex, parada := nueva("acme-orgs"), nueva("globex-orgs"), nueva("parada-orgs")
	propia, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "leire-orgs-test", Name: "Leire", Kind: domain.OrgKindPersonal})
	if err != nil {
		t.Fatal(err)
	}
	u, err := l.st.CreateUser(l.ctx, nil, domain.User{Email: "leire@orgs.test", EmailVerified: true, Name: "Leire"})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []domain.Organization{acme, globex, parada, propia} {
		if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: o.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, parada.ID); err != nil {
		t.Fatal(err)
	}
	puerta := httpx.SoloDesdeDentro("s")(http.HandlerFunc(l.srv.handleInternalPeopleOrgs))
	var personal bool
	pedir := func(secreto, cuerpo string, cabeceras ...string) (int, []string) {
		r := httptest.NewRequest(http.MethodPost, "/internal/people/orgs", strings.NewReader(cuerpo))
		r.Header.Set("Authorization", "Bearer "+secreto)
		for i := 0; i+1 < len(cabeceras); i += 2 {
			r.Header.Set(cabeceras[i], cabeceras[i+1])
		}
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		var v struct {
			Orgs     []string `json:"orgs"`
			Personal bool     `json:"personal"`
		}
		_ = json.NewDecoder(w.Body).Decode(&v)
		personal = v.Personal
		return w.Code, v.Orgs
	}
	// La personal no sale por su nombre (se deriva de su dirección): solo el aviso.
	if code, orgs := pedir("s", `{"user_id":"`+u.ID+`"}`); code != http.StatusOK || len(orgs) != 2 ||
		!slices.Contains(orgs, "acme-orgs") || !slices.Contains(orgs, "globex-orgs") || !personal {
		t.Fatalf("organizaciones: %d %v %v", code, orgs, personal)
	}
	if code, _ := pedir("otro", `{"user_id":"`+u.ID+`"}`); code != http.StatusUnauthorized {
		t.Errorf("con otro secreto: %d", code)
	}
	if code, _ := pedir("s", `{"user_id":"`+u.ID+`"}`, "X-Forwarded-For", "203.0.113.9"); code != http.StatusNotFound {
		t.Errorf("desde internet: %d", code)
	}
	if code, _ := pedir("s", `{"user_id":"no"}`); code != http.StatusBadRequest {
		t.Errorf("identificador inválido: %d", code)
	}
	if code, orgs := pedir("s", `{"user_id":"00000000-0000-4000-8000-000000000000"}`); code != http.StatusOK || len(orgs) != 0 {
		t.Errorf("nadie: %d %v", code, orgs)
	}
	if _, err := l.st.SetUserStatus(l.ctx, nil, u.ID, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if code, orgs := pedir("s", `{"user_id":"`+u.ID+`"}`); code != http.StatusOK || len(orgs) != 0 {
		t.Errorf("persona suspendida: %d %v", code, orgs)
	}
}
