// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del handler de lectura de la traza de compliance (GET /admin/audit-log)
// con el fake del store: gating por rol, listado, filtros (acción/actor/fecha) y
// exportación CSV. El aislamiento por org lo garantiza ListAudit del store (RLS,
// probado en el test de integración) — aquí el handler solo delega.
package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func auditFixture() []domain.AuditEntry {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	// El store devuelve orden (occurred_at, id) DESC; se replica aquí.
	return []domain.AuditEntry{
		{ID: 3, OccurredAt: now, Actor: "admin-1", Action: "ediscovery.search", Target: "sue@acme.eu", Detail: json.RawMessage(`{"case":"C1"}`)},
		{ID: 2, OccurredAt: now.Add(-time.Hour), Actor: "admin-2", Action: "legalhold.create", Target: "bob@acme.eu", Detail: json.RawMessage(`{}`)},
		{ID: 1, OccurredAt: now.Add(-48 * time.Hour), Actor: "admin-1", Action: "legalhold.release", Target: ""},
	}
}

func auditItems(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	m := decodeBody(t, rec)
	raw, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items no es un array: %T", m["items"])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestAuditLogForbiddenNonAdmin(t *testing.T) {
	a := complianceAPI(&fakeCompliance{auditLog: auditFixture()}, nil)
	rec := httptest.NewRecorder()
	// req() inyecta claims SIN roles => requireAdmin debe rechazar (403).
	a.listAuditLog(rec, req(http.MethodGet, "/api/v1/admin/audit-log", "", true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin rol admin => %d, quiero 403", rec.Code)
	}
}

func TestAuditLogListsAll(t *testing.T) {
	a := complianceAPI(&fakeCompliance{auditLog: auditFixture()}, nil)
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	items := auditItems(t, rec)
	if len(items) != 3 {
		t.Fatalf("items = %d, quiero 3", len(items))
	}
	// Orden DESC preservado: la primera fila es la más reciente.
	if items[0]["action"] != "ediscovery.search" {
		t.Fatalf("primera fila = %v, quiero ediscovery.search", items[0]["action"])
	}
	if m := decodeBody(t, rec); m["nextCursor"] != "" {
		t.Fatalf("nextCursor = %v, quiero vacío (cabe todo)", m["nextCursor"])
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
			a := complianceAPI(&fakeCompliance{auditLog: auditFixture()}, nil)
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
	a := complianceAPI(&fakeCompliance{auditLog: auditFixture()}, nil)
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x?since=2026-07-21&until=2026-07-20", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rango invertido => %d, quiero 400", rec.Code)
	}
}

func TestAuditLogCSV(t *testing.T) {
	a := complianceAPI(&fakeCompliance{auditLog: auditFixture()}, nil)
	rec := httptest.NewRecorder()
	a.listAuditLog(rec, adminReq(http.MethodGet, "/x?format=csv", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, quiero text/csv", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "audit-mail-") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "ts,action,actor,target,detail\n") {
		t.Fatalf("falta la cabecera CSV normalizada: %q", body[:min(40, len(body))])
	}
	if !strings.Contains(body, "ediscovery.search") || !strings.Contains(body, "sue@acme.eu") {
		t.Fatalf("faltan filas en el CSV: %q", body)
	}
}
