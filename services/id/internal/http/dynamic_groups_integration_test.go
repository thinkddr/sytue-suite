// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del CICLO COMPLETO de un grupo dinámico alimentado por el
// directorio corporativo, contra un PostgreSQL real (pgtest, build-tag `integration`):
// un alta por SCIM con la extensión enterprise entra SOLA en el grupo que le toca; un
// cambio de department por SCIM lo saca del anterior y lo mete en el nuevo; el recálculo
// (endpoint admin y barrido) repara una membresía obsoleta escrita a mano en la BD; los
// grupos MANUALES no se tocan nunca; y nada de esto cruza de una organización a otra.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"

// scimProvision aprovisiona un usuario por SCIM con los atributos del directorio
// (department en la extensión enterprise, title en el núcleo, ciudad de trabajo).
func (l *liveServer) scimProvision(t *testing.T, email, dept, title, city string) string {
	t.Helper()
	rr, out := l.do(t, http.MethodPost, "/scim/v2/Users", itSCIMToken, map[string]any{
		"schemas":     []string{scimUserSchema, enterpriseUserSchema},
		"userName":    email,
		"displayName": email,
		"title":       title,
		"addresses":   []any{map[string]any{"type": "work", "locality": city}},
		enterpriseUserSchema: map[string]any{
			"employeeNumber": "1", "costCenter": "CC", "department": dept,
		},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("SCIM POST /Users(%s): status=%d cuerpo=%s", email, rr.Code, rr.Body)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("usuario SCIM sin id: %v", out)
	}
	return id
}

const enterpriseUserSchema = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"

// mkDynamicGroup crea un grupo dinámico con la regla «department eq dept».
func (l *liveServer) mkDynamicGroup(t *testing.T, orgID, name, dept string) string {
	t.Helper()
	g, err := l.st.CreateGroup(l.ctx, nil, orgID, name, "")
	if err != nil {
		t.Fatalf("CreateGroup(%s): %v", name, err)
	}
	rule := domain.Rule{Match: "all", Conditions: []domain.Condition{
		{Attribute: domain.AttrDepartment, Op: domain.OpEquals, Value: dept},
	}}
	if _, err := l.st.SetGroupRule(l.ctx, nil, orgID, g.ID, rule); err != nil {
		t.Fatalf("SetGroupRule(%s): %v", name, err)
	}
	return g.ID
}

// memberEmails devuelve los emails materializados en un grupo (orden estable).
func (l *liveServer) memberEmails(t *testing.T, orgID, groupID string) []string {
	t.Helper()
	ms, err := l.st.ListGroupMembers(l.ctx, orgID, groupID)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Email)
	}
	return out
}

func assertEmails(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v; quería %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v; quería %v", what, got, want)
		}
	}
}

func TestDynamicGroups_SCIMEnterpriseYRecalculo(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: itSCIMOrg, Name: "SCIM Org"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	// Otra org con la MISMA regla: nada de lo que sigue puede tocarla (aislamiento).
	otra, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otraorg", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}
	otraSales := l.mkDynamicGroup(t, otra.ID, "sales-team", "Sales")

	sales := l.mkDynamicGroup(t, org.ID, "sales-team", "Sales")
	eng := l.mkDynamicGroup(t, org.ID, "eng-team", "Engineering")

	// Grupo MANUAL con un miembro asignado a mano: ningún recálculo debe tocarlo.
	manualG, err := l.st.CreateGroup(l.ctx, nil, org.ID, "guardia", "")
	if err != nil {
		t.Fatalf("CreateGroup(manual): %v", err)
	}
	pivot := l.scimProvision(t, "pivot@scim.test", "Legal", "Counsel", "Lisboa")
	if err := l.st.AddGroupMember(l.ctx, nil, org.ID, manualG.ID, pivot); err != nil {
		t.Fatalf("AddGroupMember(manual): %v", err)
	}

	// --- 1. Un alta del directorio con la extensión enterprise entra SOLA en su grupo.
	ada := l.scimProvision(t, "ada@scim.test", "Sales", "Head of Sales", "Madrid")
	got, err := l.st.GetUserByID(l.ctx, ada)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Department != "Sales" || got.Title != "Head of Sales" || got.Location != "Madrid" {
		t.Fatalf("atributos no persistidos: dept=%q title=%q loc=%q", got.Department, got.Title, got.Location)
	}
	assertEmails(t, "sales tras el alta", l.memberEmails(t, org.ID, sales), []string{"ada@scim.test"})
	assertEmails(t, "eng tras el alta", l.memberEmails(t, org.ID, eng), nil)
	// El claim `groups` (lo que consume Drive) ya lo refleja sin más intervención.
	names, err := l.st.GroupsForUser(l.ctx, org.ID, ada)
	if err != nil || len(names) != 1 || names[0] != "sales-team" {
		t.Fatalf("GroupsForUser = %v, err=%v; quería [sales-team]", names, err)
	}
	// Aislamiento: el grupo homónimo de la otra org sigue vacío.
	assertEmails(t, "sales de otra org", l.memberEmails(t, otra.ID, otraSales), nil)

	// --- 2. Un cambio de department por SCIM lo saca del anterior y lo mete en el nuevo.
	rr, _ := l.do(t, http.MethodPatch, "/scim/v2/Users/"+ada, itSCIMToken, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []any{map[string]any{
			"op":    "replace",
			"path":  enterpriseUserSchema + ":department",
			"value": "Engineering",
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("SCIM PATCH department: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	assertEmails(t, "sales tras el PATCH", l.memberEmails(t, org.ID, sales), nil)
	assertEmails(t, "eng tras el PATCH", l.memberEmails(t, org.ID, eng), []string{"ada@scim.test"})

	// El PUT (forma de Okta) también mueve la membresía: vuelve a Sales.
	rr, _ = l.do(t, http.MethodPut, "/scim/v2/Users/"+ada, itSCIMToken, map[string]any{
		"schemas":            []string{scimUserSchema, enterpriseUserSchema},
		"userName":           "ada@scim.test",
		"displayName":        "Ada",
		enterpriseUserSchema: map[string]any{"department": "Sales"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("SCIM PUT: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	assertEmails(t, "sales tras el PUT", l.memberEmails(t, org.ID, sales), []string{"ada@scim.test"})
	assertEmails(t, "eng tras el PUT", l.memberEmails(t, org.ID, eng), nil)

	// --- 3. DERIVA simulada: se cambia el atributo POR DEBAJO (escritura directa en la
	// BD, sin pasar por ningún disparador). La membresía queda obsoleta...
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET department = 'Engineering' WHERE id = $1`, ada); err != nil {
		t.Fatalf("deriva simulada: %v", err)
	}
	assertEmails(t, "sales con deriva", l.memberEmails(t, org.ID, sales), []string{"ada@scim.test"})

	// ...y el recálculo del grupo la repara, informando del efecto REAL.
	admin := l.mkUser(t, "admin@scim.test", "")
	tok := l.token(t, admin.ID, itSCIMOrg, []string{"admin"})
	rr, out := l.do(t, http.MethodPost, "/api/v1/admin/groups/"+sales+"/recompute", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST recompute: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["removed"] != float64(1) || out["added"] != float64(0) || out["member_count"] != float64(0) {
		t.Fatalf("recompute devolvió %v; quería added=0 removed=1 member_count=0", out)
	}
	assertEmails(t, "sales tras recalcular", l.memberEmails(t, org.ID, sales), nil)
	// El grupo dinámico que SÍ le toca aún no lo tiene: lo arregla el recálculo de la org.
	assertEmails(t, "eng antes del recálculo de org", l.memberEmails(t, org.ID, eng), nil)

	// --- 4. Recálculo de TODA la org (un clic) y barrido cross-org (el tick periódico).
	rr, out = l.do(t, http.MethodPost, "/api/v1/admin/groups/recompute", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST recompute org: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["groups"] != float64(2) || out["added"] != float64(1) || out["removed"] != float64(0) {
		t.Fatalf("recompute de org devolvió %v; quería groups=2 added=1 removed=0", out)
	}
	assertEmails(t, "eng tras el recálculo de org", l.memberEmails(t, org.ID, eng), []string{"ada@scim.test"})

	// Idempotente: en estado estable no mueve nada (el barrido no escribe).
	sweep, err := l.st.RecomputeAllDynamicGroups(l.ctx)
	if err != nil {
		t.Fatalf("RecomputeAllDynamicGroups: %v", err)
	}
	if sweep.Added != 0 || sweep.Removed != 0 {
		t.Fatalf("barrido en estado estable = %+v; no debía escribir nada", sweep)
	}
	if sweep.Orgs != 2 || sweep.Groups != 3 {
		t.Fatalf("barrido = %+v; quería 2 orgs y 3 grupos dinámicos", sweep)
	}

	// --- 5. El grupo MANUAL sigue intacto tras todo lo anterior, y no admite recálculo.
	assertEmails(t, "grupo manual", l.memberEmails(t, org.ID, manualG.ID), []string{"pivot@scim.test"})
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/groups/"+manualG.ID+"/recompute", tok, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("recompute de grupo manual: status=%d, quiero 400", rr.Code)
	}

	// --- 6. Aislamiento: la otra org nunca vio a nadie, ni por SCIM ni por recálculo.
	assertEmails(t, "sales de otra org al final", l.memberEmails(t, otra.ID, otraSales), nil)
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/groups/"+otraSales+"/recompute", tok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("recompute de un grupo de OTRA org: status=%d, quiero 404", rr.Code)
	}
}
