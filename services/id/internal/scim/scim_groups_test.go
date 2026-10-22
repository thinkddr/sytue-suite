// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios (sin BD) del recurso Group de SCIM: parseo del filtro
// displayName, del path de members de Entra/Okta y del value de PATCH; el mapeo
// dominio→SCIM; el formato de error SCIM; y el guard de «SCIM sin organización».
package scim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestParseDisplayNameFilter(t *testing.T) {
	cases := map[string]string{
		`displayName eq "engineering"`: "engineering",
		`DISPLAYNAME EQ "Ventas"`:      "Ventas", // case-insensitive
		`displayName eq "con espacio"`: "con espacio",
		`userName eq "x"`:              "", // otro atributo
		``:                             "",
		`displayName co "eng"`:         "", // operador no soportado
	}
	for in, want := range cases {
		if got := parseDisplayNameFilter(in); got != want {
			t.Errorf("parseDisplayNameFilter(%q) = %q; quería %q", in, got, want)
		}
	}
}

func TestMemberIDFromPath(t *testing.T) {
	id := "11111111-1111-1111-1111-111111111111"
	if got, ok := memberIDFromPath(`members[value eq "` + id + `"]`); !ok || got != id {
		t.Errorf("path con filtro Entra = (%q,%v); quería (%q,true)", got, ok, id)
	}
	if _, ok := memberIDFromPath("members"); ok {
		t.Error("path sin filtro debía dar ok=false")
	}
	if _, ok := memberIDFromPath(`members[value eq ]`); ok {
		t.Error("path sin comillas debía dar ok=false")
	}
}

func TestMembersFromValue(t *testing.T) {
	// Array de objetos (formato Entra add/replace de members).
	arr := []any{
		map[string]any{"value": "a"},
		map[string]any{"value": "b"},
		map[string]any{"noValue": "x"}, // se ignora
	}
	got := membersFromValue(arr)
	if len(got) != 2 || got[0].Value != "a" || got[1].Value != "b" {
		t.Fatalf("membersFromValue(array) = %+v", got)
	}
	// Objeto único.
	if got := membersFromValue(map[string]any{"value": "solo"}); len(got) != 1 || got[0].Value != "solo" {
		t.Fatalf("membersFromValue(objeto) = %+v", got)
	}
	// Tipos no soportados ⇒ nil.
	if got := membersFromValue("cadena"); got != nil {
		t.Fatalf("membersFromValue(string) = %+v; quería nil", got)
	}
}

func TestToSCIMGroupMapping(t *testing.T) {
	h := &Handler{issuer: "https://id.test"}
	g := domain.Group{ID: "gid-1", Name: "engineering", ExternalID: "ext-9"}
	members := []store.GroupMember{
		{UserID: "uid-1", Email: "ada@id.test", Name: "Ada"},
		{UserID: "uid-2", Email: "bob@id.test", Name: ""}, // sin nombre ⇒ display=email
	}
	sg := h.toSCIMGroup(g, members)
	if sg.DisplayName != "engineering" || sg.ExternalID != "ext-9" || sg.ID != "gid-1" {
		t.Fatalf("mapeo base incorrecto: %+v", sg)
	}
	if len(sg.Schemas) != 1 || sg.Schemas[0] != schemaGroup {
		t.Fatalf("schemas = %v", sg.Schemas)
	}
	if sg.Meta == nil || sg.Meta.Location != "https://id.test/scim/v2/Groups/gid-1" || sg.Meta.ResourceType != "Group" {
		t.Fatalf("meta = %+v", sg.Meta)
	}
	if len(sg.Members) != 2 {
		t.Fatalf("members = %+v", sg.Members)
	}
	if sg.Members[0].Value != "uid-1" || sg.Members[0].Display != "Ada" ||
		sg.Members[0].Ref != "https://id.test/scim/v2/Users/uid-1" {
		t.Fatalf("member[0] = %+v", sg.Members[0])
	}
	if sg.Members[1].Display != "bob@id.test" { // fallback a email
		t.Fatalf("member[1].Display = %q; quería el email", sg.Members[1].Display)
	}
	// El listado omite members (sin N+1): nil ⇒ members ausente en JSON.
	if list := h.toSCIMGroup(g, nil); list.Members != nil {
		t.Fatalf("listado no debía expandir members: %+v", list.Members)
	}
}

func TestSCIMErrorFormat(t *testing.T) {
	rr := httptest.NewRecorder()
	scimErr(rr, http.StatusNotFound, "grupo no encontrado")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != contentType {
		t.Fatalf("content-type = %q; quería %q", ct, contentType)
	}
	var body struct {
		Schemas []string `json:"schemas"`
		Status  string   `json:"status"`
		Detail  string   `json:"detail"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	if len(body.Schemas) != 1 || body.Schemas[0] != schemaError {
		t.Fatalf("schemas de error = %v; quería [%s]", body.Schemas, schemaError)
	}
	if body.Status != "404" { // status es STRING en el esquema de error SCIM
		t.Fatalf("status = %q; quería \"404\"", body.Status)
	}
	if body.Detail != "grupo no encontrado" {
		t.Fatalf("detail = %q", body.Detail)
	}
}

// TestGroupsRequireOrg comprueba que, sin org ligada al token, el recurso Group
// responde 500 con error SCIM (no toca el store: resolveOrg corta antes).
func TestGroupsRequireOrg(t *testing.T) {
	h := &Handler{issuer: "https://id.test"} // orgSlug vacío
	if _, err := h.resolveOrg(context.Background()); !errors.Is(err, errSCIMOrgUnset) {
		t.Fatalf("resolveOrg sin slug = %v; quería errSCIMOrgUnset", err)
	}
	rr := httptest.NewRecorder()
	h.ListGroups(rr, httptest.NewRequest(http.MethodGet, "/scim/v2/Groups", http.NoBody))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("ListGroups sin org: status = %d; quería 500", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), schemaError) {
		t.Fatalf("cuerpo sin esquema de error SCIM: %s", rr.Body.String())
	}
}

// TestUsersRequireOrg: /Users exige la misma org ligada que /Groups. Sin ella
// existía un «modo global» que listaba a TODOS los usuarios del despliegue —de
// todas las organizaciones— y daba por bueno cualquier {id} en las rutas de
// detalle. El store es nil a propósito: si algún camino volviera a consultarlo
// sin acotar por organización, el test entra en pánico en vez de pasar.
func TestUsersRequireOrg(t *testing.T) {
	h := &Handler{issuer: "https://id.test"} // orgSlug vacío
	rr := httptest.NewRecorder()
	h.ListUsers(rr, httptest.NewRequest(http.MethodGet, "/scim/v2/Users", http.NoBody))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("ListUsers sin org: status = %d; quería 500 (jamás un listado)", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), schemaError) {
		t.Fatalf("cuerpo sin esquema de error SCIM: %s", rr.Body.String())
	}
	if ok, err := h.userInScope(context.Background(), "00000000-0000-0000-0000-000000000001"); ok || err == nil {
		t.Fatalf("userInScope sin org = (%v, %v); quería (false, err): sin ámbito no hay verja anti-IDOR", ok, err)
	}
}
