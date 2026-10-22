// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la gestión de organizaciones desde Console contra un
// PostgreSQL real (pgtest, build-tag `integration`): crear (creador⇒owner, slug
// único/​inválido), listar acotado a las orgs gestionadas (owner/admin, no las de
// simple member), editar ajustes (nombre/estado) con autorización por-org
// fail-closed (no-admin→403, cross-org/inexistente→404). Reusa el harness
// newLiveServer.
package http

import (
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func hasSlug(items []any, slug string) bool {
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["slug"] == slug {
			return true
		}
	}
	return false
}

func TestTenant_CreateAndList(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "founder@id.test", "founder-pass-123")
	tok := l.token(t, u.ID, "", []string{"admin"})

	// Crear → 201 con la forma de tenant esperada (namespace derivado, user_count=1).
	rr, out := l.do(t, http.MethodPost, "/api/v1/admin/tenants", tok,
		map[string]string{"slug": "acme", "name": "ACME Europa"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("crear org: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["slug"] != "acme" || out["namespace"] != "tenant-acme" || out["name"] != "ACME Europa" {
		t.Fatalf("forma de tenant inesperada: %v", out)
	}
	if n, _ := out["user_count"].(float64); n != 1 {
		t.Fatalf("user_count del creador = %v, quiero 1", out["user_count"])
	}

	// El creador queda como OWNER de la nueva org.
	org, err := l.st.GetOrgBySlug(l.ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if mem, err := l.st.GetMembership(l.ctx, org.ID, u.ID); err != nil || mem.Role != domain.RoleOwner {
		t.Fatalf("el creador debe ser owner: %+v (%v)", mem, err)
	}

	// Slug duplicado → 409; slug inválido → 400.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/tenants", tok,
		map[string]string{"slug": "acme", "name": "Otra"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("slug duplicado: status=%d, quiero 409", rr.Code)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/tenants", tok,
		map[string]string{"slug": "Acme Mayus!", "name": "X"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("slug inválido: status=%d, quiero 400", rr.Code)
	}
	// Un nombre de página de las apps —los `sinOrg` de la web (ADR 0056)— está
	// reservado también aquí, aunque quien llama tenga rol admin.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/tenants", tok,
		map[string]string{"slug": "edit", "name": "X"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("slug reservado (página de la web): status=%d, quiero 409", rr.Code)
	}

	// Listar → incluye acme (la gestiona como owner).
	rr, out = l.do(t, http.MethodGet, "/api/v1/admin/tenants", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	items, _ := out["items"].([]any)
	if !hasSlug(items, "acme") {
		t.Fatalf("la lista debe incluir acme: %v", out)
	}
}

func TestTenant_ListScopedToManaged(t *testing.T) {
	l := newLiveServer(t)
	orgA, admin := l.seedOrgAdmin(t, "acme") // admin: rol admin de A
	orgB, _ := l.seedOrgAdmin(t, "globex")
	// El mismo usuario es solo MEMBER de B ⇒ B NO debe aparecer en su lista gestionada.
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
		OrgID: orgB.ID, UserID: admin.ID, Role: domain.RoleMember,
	}); err != nil {
		t.Fatal(err)
	}
	tok := l.token(t, admin.ID, orgA.Slug, []string{"admin"})

	rr, out := l.do(t, http.MethodGet, "/api/v1/admin/tenants", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar: status=%d", rr.Code)
	}
	items, _ := out["items"].([]any)
	if !hasSlug(items, "acme") {
		t.Fatal("debe incluir la org que gestiona (acme)")
	}
	if hasSlug(items, "globex") {
		t.Fatal("no debe incluir la org donde es solo member (globex)")
	}
}

func TestTenant_UpdateSettingsAndAuthz(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminA := l.seedOrgAdmin(t, "acme")
	member := l.addMember(t, orgA, "member@acme.test", domain.RoleMember)
	adminTok := l.token(t, adminA.ID, orgA.Slug, []string{"admin"})

	// Editar nombre + estado (owner/admin de ESTA org) → 200 y BD actualizada.
	rr, out := l.do(t, http.MethodPatch, "/api/v1/admin/tenants/acme", adminTok,
		map[string]any{"name": "ACME Renombrada", "status": "suspended"})
	if rr.Code != http.StatusOK {
		t.Fatalf("editar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["name"] != "ACME Renombrada" || out["status"] != "suspended" {
		t.Fatalf("ajustes no aplicados en la respuesta: %v", out)
	}
	got, _ := l.st.GetOrgBySlug(l.ctx, "acme")
	if got.Name != "ACME Renombrada" || got.Status != domain.StatusSuspended {
		t.Fatalf("BD no refleja los ajustes: %+v", got)
	}

	// Estado inválido (deleted no se alcanza por esta vía) → 400.
	rr, _ = l.do(t, http.MethodPatch, "/api/v1/admin/tenants/acme", adminTok,
		map[string]any{"status": "deleted"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("estado inválido: status=%d, quiero 400", rr.Code)
	}

	// No-admin (member, por claim) → 403 (deny-by-default de requireRole).
	memberTok := l.token(t, member.ID, orgA.Slug, []string{"member"})
	rr, _ = l.do(t, http.MethodPatch, "/api/v1/admin/tenants/acme", memberTok,
		map[string]any{"name": "Secuestrada"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin edita: status=%d, quiero 403", rr.Code)
	}
}

// Crear organización desde el selector (POST /orgs): la reserva de los nombres
// de página de las apps (ADR 0056) tampoco se burla por esta vía. Es la misma
// comprobación que el alta guiada aplica en onboarding.
func TestTenant_CreateOrgRejectsReservedPageSlug(t *testing.T) {
	l := newLiveServer(t)
	orgA, admin := l.seedOrgAdmin(t, "acme")
	tok := l.token(t, admin.ID, orgA.Slug, []string{"admin"})

	rr, _ := l.do(t, http.MethodPost, "/api/v1/orgs", tok,
		map[string]string{"slug": "edit", "name": "X"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("slug reservado (página de la web): status=%d, quiero 409", rr.Code)
	}
	if _, err := l.st.GetOrgBySlug(l.ctx, "edit"); err == nil {
		t.Fatal("la org sobre el slug reservado no debe haberse creado")
	}

	// Y es la misma alta que la de Console: normaliza el slug y deja al creador owner
	// en la misma transacción. Antes /orgs tenía su copia y devolvía 400 a esto.
	rr, out := l.do(t, http.MethodPost, "/api/v1/orgs", tok,
		map[string]string{"slug": "  Nueva-Org ", "name": " Nueva SL "})
	if rr.Code != http.StatusCreated || out["slug"] != "nueva-org" || out["name"] != "Nueva SL" {
		t.Fatalf("POST /orgs: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	nueva, err := l.st.GetOrgBySlug(l.ctx, "nueva-org")
	if err != nil {
		t.Fatal(err)
	}
	if mem, err := l.st.GetMembership(l.ctx, nueva.ID, admin.ID); err != nil || mem.Role != domain.RoleOwner {
		t.Fatalf("el creador debe ser owner: %+v (%v)", mem, err)
	}
}

// La cuenta personal de quien administra una empresa es suya, pero no se administra
// desde Sytue Admin (ADR 0038): requireOrgKind solo mira la organización ACTIVA, así
// que las rutas que reciben otra por el slug tienen que comprobarla ellas.
func TestTenant_LaCuentaPersonalNoSeAdministraDesdeLaEmpresa(t *testing.T) {
	l := newLiveServer(t)
	empresa, admin := l.seedOrgAdmin(t, "acme")
	personal, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{
		Slug: domain.PersonalSlug(admin.Email), Name: admin.Email, Kind: domain.OrgKindPersonal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
		OrgID: personal.ID, UserID: admin.ID, Role: domain.RoleOwner,
	}); err != nil {
		t.Fatal(err)
	}
	ajena := l.addMember(t, empresa, "colega@acme.test", domain.RoleMember)
	tok := l.token(t, admin.ID, empresa.Slug, []string{"admin"})

	rr, out := l.do(t, http.MethodGet, "/api/v1/admin/tenants", tok, nil)
	items, _ := out["items"].([]any)
	if rr.Code != http.StatusOK || !hasSlug(items, empresa.Slug) || hasSlug(items, personal.Slug) {
		t.Fatalf("la lista debe traer la empresa y no la cuenta personal: %s", rr.Body)
	}

	rr, _ = l.do(t, http.MethodPatch, "/api/v1/admin/tenants/"+personal.Slug, tok,
		map[string]any{"status": "suspended"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("suspender la cuenta personal: status=%d, quiero 404", rr.Code)
	}
	if got, _ := l.st.GetOrgBySlug(l.ctx, personal.Slug); got.Status == domain.StatusSuspended {
		t.Fatal("la cuenta personal no debe quedar suspendida")
	}

	rr, _ = l.do(t, http.MethodPost, "/api/v1/orgs/"+personal.Slug+"/members", tok,
		map[string]string{"email": ajena.Email, "role": "member"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("meter a otra persona en la cuenta personal: status=%d, quiero 403", rr.Code)
	}
	if _, err := l.st.GetMembership(l.ctx, personal.ID, ajena.ID); err == nil {
		t.Fatal("la otra persona no debe quedar dentro de la cuenta personal")
	}
}

func TestTenant_UpdateCrossOrgIsolation(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminA := l.seedOrgAdmin(t, "acme")
	_, _ = l.seedOrgAdmin(t, "globex")
	adminATok := l.token(t, adminA.ID, orgA.Slug, []string{"admin"})

	// Admin de A intenta editar org B (no es miembro de B) → 404 (no revela existencia).
	rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/tenants/globex", adminATok,
		map[string]any{"name": "Secuestrada"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org editar: status=%d, quiero 404", rr.Code)
	}
	// Org inexistente → 404 (mismo trato).
	rr, _ = l.do(t, http.MethodPatch, "/api/v1/admin/tenants/no-existe", adminATok,
		map[string]any{"name": "X"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org inexistente: status=%d, quiero 404", rr.Code)
	}
	// B queda intacta (seedOrgAdmin fija Name = MAYÚSCULAS del slug).
	if got, _ := l.st.GetOrgBySlug(l.ctx, "globex"); got.Name != strings.ToUpper("globex") {
		t.Fatalf("la org B no debe cambiar: %+v", got)
	}
}
