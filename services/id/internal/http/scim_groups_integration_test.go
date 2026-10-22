// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del recurso Group de SCIM 2.0 (aprovisionamiento entrante
// de grupos, RFC 7643 §4.2 / 7644) contra un PostgreSQL real (pgtest, build-tag
// `integration`). Ejercita el ciclo de Entra/Okta de extremo a extremo por HTTP
// con el bearer estático del provisioner: aprovisionar usuarios (Users ⇒ membresía
// de la org), crear grupo con members, filtrar por displayName, paginar, GET con
// members, PATCH add/remove (formatos de Entra) reflejando el claim `groups`,
// aislamiento por org (ajeno ⇒ 400; grupo de otro tenant ⇒ 404 con error SCIM) y
// DELETE con cascada del claim.
package http

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const schemaSCIMError = "urn:ietf:params:scim:api:messages:2.0:Error"

// scimCreateUser aprovisiona un usuario por SCIM Users y devuelve su id.
func (l *liveServer) scimCreateUser(t *testing.T, email string) string {
	t.Helper()
	rr, out := l.do(t, http.MethodPost, "/scim/v2/Users", itSCIMToken, map[string]any{
		"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"userName": email,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("SCIM POST /Users(%s): status=%d cuerpo=%s", email, rr.Code, rr.Body)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("SCIM user sin id: %v", out)
	}
	return id
}

func TestSCIMGroups_ProvisioningLifecycle(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: itSCIMOrg, Name: "SCIM Org"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// --- Users por SCIM ⇒ miembros de la org (si no, el claim de grupo sería inerte).
	u1 := l.scimCreateUser(t, "ada@scim.test")
	u2 := l.scimCreateUser(t, "bob@scim.test")
	for _, id := range []string{u1, u2} {
		if _, err := l.st.GetMembership(l.ctx, org.ID, id); err != nil {
			t.Fatalf("SCIM user %s no quedó como miembro de la org: %v", id, err)
		}
	}

	// --- POST /Groups con member inicial ⇒ 201 + Location + claim reflejado.
	rr, out := l.do(t, http.MethodPost, "/scim/v2/Groups", itSCIMToken, map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"displayName": "engineering",
		"externalId":  "grp-ext-1",
		"members":     []any{map[string]any{"value": u1}},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /Groups: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	gid, _ := out["id"].(string)
	if gid == "" {
		t.Fatalf("grupo creado sin id: %v", out)
	}
	if rr.Header().Get("Location") == "" {
		t.Error("falta la cabecera Location en la creación")
	}
	if out["externalId"] != "grp-ext-1" {
		t.Errorf("externalId no persistido: %v", out["externalId"])
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u1); len(names) != 1 || names[0] != "engineering" {
		t.Fatalf("claim `groups` de u1 = %v; quería [engineering]", names)
	}

	// Duplicado por displayName ⇒ 409.
	rr, _ = l.do(t, http.MethodPost, "/scim/v2/Groups", itSCIMToken, map[string]any{"displayName": "engineering"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("POST /Groups duplicado: status=%d; quería 409", rr.Code)
	}

	// --- Filtro displayName eq.
	rr, out = l.do(t, http.MethodGet, "/scim/v2/Groups?filter="+url.QueryEscape(`displayName eq "engineering"`), itSCIMToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /Groups filtrado: status=%d", rr.Code)
	}
	if tr, _ := out["totalResults"].(float64); tr != 1 {
		t.Fatalf("filtro que matchea: totalResults=%v; quería 1", out["totalResults"])
	}
	_, out = l.do(t, http.MethodGet, "/scim/v2/Groups?filter="+url.QueryEscape(`displayName eq "nope"`), itSCIMToken, nil)
	if tr, _ := out["totalResults"].(float64); tr != 0 {
		t.Fatalf("filtro sin match: totalResults=%v; quería 0", out["totalResults"])
	}

	// --- GET /{id} trae members.
	rr, out = l.do(t, http.MethodGet, "/scim/v2/Groups/"+gid, itSCIMToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /Groups/{id}: status=%d", rr.Code)
	}
	if ms, _ := out["members"].([]any); len(ms) != 1 {
		t.Fatalf("members en GET/{id} = %v; quería 1", out["members"])
	}

	// --- PATCH add u2 (formato Entra: op add, path members, value [{value}]).
	rr, _ = l.do(t, http.MethodPatch, "/scim/v2/Groups/"+gid, itSCIMToken, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []any{map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": u2}}}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH add member: status=%d", rr.Code)
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u2); len(names) != 1 {
		t.Fatalf("claim de u2 tras add = %v; quería [engineering]", names)
	}

	// --- PATCH remove u1 (formato Entra: path members[value eq "id"]).
	rr, _ = l.do(t, http.MethodPatch, "/scim/v2/Groups/"+gid, itSCIMToken, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []any{map[string]any{"op": "remove", "path": `members[value eq "` + u1 + `"]`}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH remove member: status=%d", rr.Code)
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u1); len(names) != 0 {
		t.Fatalf("claim de u1 tras remove = %v; quería vacío", names)
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u2); len(names) != 1 {
		t.Fatalf("u2 debía seguir en el grupo; claim = %v", names)
	}

	// --- Aislamiento: un usuario ajeno a la org NO puede añadirse (400).
	outsider := l.mkUser(t, "outsider@other.test", "")
	rr, _ = l.do(t, http.MethodPatch, "/scim/v2/Groups/"+gid, itSCIMToken, map[string]any{
		"Operations": []any{map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": outsider.ID}}}},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("añadir a un usuario ajeno a la org: status=%d; quería 400", rr.Code)
	}

	// --- Aislamiento cross-tenant: un grupo de OTRA org ⇒ 404 con error SCIM.
	other, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatal(err)
	}
	og, err := l.st.CreateGroup(l.ctx, nil, other.ID, "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	rr, _ = l.do(t, http.MethodGet, "/scim/v2/Groups/"+og.ID, itSCIMToken, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET grupo de otra org: status=%d; quería 404", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), schemaSCIMError) {
		t.Fatalf("404 sin formato de error SCIM: %s", rr.Body)
	}

	// --- PUT reemplaza displayName + members.
	rr, out = l.do(t, http.MethodPut, "/scim/v2/Groups/"+gid, itSCIMToken, map[string]any{
		"displayName": "platform",
		"members":     []any{map[string]any{"value": u1}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /Groups/{id}: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["displayName"] != "platform" {
		t.Fatalf("PUT no renombró: %v", out["displayName"])
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u1); len(names) != 1 || names[0] != "platform" {
		t.Fatalf("claim de u1 tras PUT = %v; quería [platform]", names)
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u2); len(names) != 0 {
		t.Fatalf("u2 debía salir del grupo tras el PUT (replace); claim = %v", names)
	}

	// --- Paginación (startIndex/count).
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if _, err := l.st.CreateGroup(l.ctx, nil, org.ID, n, ""); err != nil {
			t.Fatalf("CreateGroup(%s): %v", n, err)
		}
	}
	rr, out = l.do(t, http.MethodGet, "/scim/v2/Groups?startIndex=1&count=2", itSCIMToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("paginación: status=%d", rr.Code)
	}
	if ipp, _ := out["itemsPerPage"].(float64); ipp != 2 {
		t.Fatalf("itemsPerPage=%v; quería 2", out["itemsPerPage"])
	}
	if tr, _ := out["totalResults"].(float64); tr != 4 {
		t.Fatalf("totalResults=%v; quería 4 (platform+alpha+beta+gamma)", out["totalResults"])
	}

	// --- DELETE ⇒ 204, luego 404, y el claim de u1 cae en cascada.
	rr, _ = l.do(t, http.MethodDelete, "/scim/v2/Groups/"+gid, itSCIMToken, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE /Groups/{id}: status=%d; quería 204", rr.Code)
	}
	rr, _ = l.do(t, http.MethodGet, "/scim/v2/Groups/"+gid, itSCIMToken, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET tras DELETE: status=%d; quería 404", rr.Code)
	}
	if names, _ := l.st.GroupsForUser(l.ctx, org.ID, u1); len(names) != 0 {
		t.Fatalf("claim de u1 tras borrar el grupo = %v; quería vacío", names)
	}
}

// TestSCIMGroups_AuthRequired comprueba que el recurso Group exige el bearer del
// provisioner (mismo guard que Users): sin token ⇒ 401.
func TestSCIMGroups_AuthRequired(t *testing.T) {
	l := newLiveServer(t)
	rr, _ := l.do(t, http.MethodGet, "/scim/v2/Groups", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("GET /Groups sin token: status=%d; quería 401", rr.Code)
	}
	rr, _ = l.do(t, http.MethodGet, "/scim/v2/Groups", "token-incorrecto", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("GET /Groups con token inválido: status=%d; quería 401", rr.Code)
	}
}
