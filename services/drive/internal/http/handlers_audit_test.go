// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del handler de lectura de la traza de compliance (GET /admin/audit-log)
// con el fake del store: gating por rol, listado, filtros (acción/actor/fecha) y
// exportación CSV. El aislamiento por org lo garantiza ListAudit del store (RLS +
// predicado tenant, probado en el test de integración) — aquí el handler delega.
package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

func auditFixture() []domain.AuditEntry {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	// El store devuelve orden (occurred_at, id) DESC; se replica aquí.
	return []domain.AuditEntry{
		{ID: 3, OccurredAt: now, Actor: "admin-1", Action: "ediscovery.search", Target: "alice", Detail: json.RawMessage(`{"case":"C1"}`)},
		{ID: 2, OccurredAt: now.Add(-time.Hour), Actor: "admin-2", Action: "legalhold.create", Target: "bob", Detail: json.RawMessage(`{}`)},
		{ID: 1, OccurredAt: now.Add(-48 * time.Hour), Actor: "admin-1", Action: "legalhold.release", Target: ""},
	}
}

func auditItems(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var m struct {
		Items []map[string]any `json:"items"`
		//nolint:tagliatelle // el endpoint de auditoría responde camelCase (contrato ya publicado a la Console).
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON objeto: %v (%s)", err, rec.Body.String())
	}
	return m.Items
}

func TestAuditLogForbiddenNonAdmin(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{auditLog: auditFixture()}}
	r := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "u", Tenant: "acme", Roles: []string{"member"}}))
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin rol admin => %d, quiero 403", rec.Code)
	}
}

func TestAuditLogListsAll(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{auditLog: auditFixture()}}
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	items := auditItems(t, rec)
	if len(items) != 3 {
		t.Fatalf("items = %d, quiero 3", len(items))
	}
	if items[0]["action"] != "ediscovery.search" {
		t.Fatalf("primera fila = %v, quiero ediscovery.search (orden DESC)", items[0]["action"])
	}
}

func TestAuditLogFilters(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"prefijo de acción", "/x?action=legalhold.", 2},
		{"actor subcadena", "/x?actor=admin-2", 1},
		{"desde (incluye el día)", "/x?since=2026-07-20", 2},
		{"hasta (inclusive del día indicado)", "/x?until=2026-07-19", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &complianceHandlers{store: &fakeCompliance{auditLog: auditFixture()}}
			rec := httptest.NewRecorder()
			a.listAuditLog(rec, adminReq(http.MethodGet, c.query, "", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
			}
			if got := len(auditItems(t, rec)); got != c.want {
				t.Fatalf("items = %d, quiero %d", got, c.want)
			}
		})
	}
}

func TestAuditLogRejectsBadRange(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{auditLog: auditFixture()}}
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x?since=2026-07-21&until=2026-07-20", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rango invertido => %d, quiero 400", rec.Code)
	}
}

func TestAuditLogCSV(t *testing.T) {
	a := &complianceHandlers{store: &fakeCompliance{auditLog: auditFixture()}}
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x?format=csv", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, quiero text/csv", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "audit-drive-") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "ts,action,actor,target,detail\n") {
		t.Fatalf("falta la cabecera CSV normalizada: %q", body[:min(40, len(body))])
	}
	if !strings.Contains(body, "ediscovery.search") || !strings.Contains(body, "alice") {
		t.Fatalf("faltan filas en el CSV: %q", body)
	}
}
