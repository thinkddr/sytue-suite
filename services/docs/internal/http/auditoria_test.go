// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// auditoriaFalsa guarda con qué parámetros se la llamó.
type auditoriaFalsa struct {
	llamadas  int
	antesDe   int64
	ventana   int
	respuesta domain.PaginaAuditoria
}

func (a *auditoriaFalsa) AuditoriaOps(_ context.Context, _ string, antesDe int64, ventana int) (domain.PaginaAuditoria, error) {
	a.llamadas++
	a.antesDe, a.ventana = antesDe, ventana
	return a.respuesta, nil
}

// Solo el propietario ve la auditoría: un editor no, un lector no, y ninguno llega al store.
func TestAuditoriaSoloPropietario(t *testing.T) {
	for _, role := range []domain.Role{"", domain.RoleViewer, domain.RoleCommenter, domain.RoleEditor, domain.RoleOwner} {
		t.Run(string(role), func(t *testing.T) {
			fs := &fakeStore{doc: &domain.Document{ID: "d1", Kind: domain.KindSpreadsheet, OwnerSub: "otra-persona"}}
			fs.perms = []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner", Role: role}}
			aud := &auditoriaFalsa{respuesta: domain.PaginaAuditoria{Entradas: []domain.EntradaAuditoria{
				{SeqDesde: 1, SeqHasta: 3, AuthorSub: "ada", Kind: domain.OpSet, Key: "doc:sheet", Cuantas: 3},
			}}}
			h := newHandler(fs)
			h.auditoria = aud
			rec := httptest.NewRecorder()
			h.auditoriaOps(rec, req(t, "GET", "/v1/documents/d1/audit", ownerClaims(), map[string]string{"id": "d1"}, ""))
			want, llamadas := http.StatusForbidden, 0
			if role == domain.RoleOwner {
				want, llamadas = http.StatusOK, 1
			}
			if rec.Code != want || aud.llamadas != llamadas {
				t.Fatalf("rol %q: HTTP=%d, consultas=%d; quiero %d/%d", role, rec.Code, aud.llamadas, want, llamadas)
			}
		})
	}
}

func TestAuditoriaParametrosYRespuesta(t *testing.T) {
	aud := &auditoriaFalsa{respuesta: domain.PaginaAuditoria{
		Entradas:         []domain.EntradaAuditoria{{SeqDesde: 5, SeqHasta: 9, AuthorSub: "ada", Kind: domain.OpSet, Key: "doc:sheet", Cuantas: 2}},
		SiguienteAntesDe: 5,
	}}
	h := newHandler(&fakeStore{doc: ownedDoc()})
	h.auditoria = aud
	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.auditoriaOps(rec, req(t, "GET", "/v1/documents/d1/audit"+query, ownerClaims(), map[string]string{"id": "d1"}, ""))
		return rec
	}

	rec := get("")
	if rec.Code != http.StatusOK || aud.ventana != ventanaAuditoriaPorDefecto || aud.antesDe != 0 {
		t.Fatalf("por defecto: HTTP=%d ventana=%d antesDe=%d", rec.Code, aud.ventana, aud.antesDe)
	}
	for _, campo := range []string{`"seq_desde":5`, `"seq_hasta":9`, `"author_sub":"ada"`, `"kind":"set"`, `"key":"doc:sheet"`, `"count":2`, `"next_before_seq":5`} {
		if !strings.Contains(rec.Body.String(), campo) {
			t.Fatalf("falta %s en %s", campo, rec.Body.String())
		}
	}
	if rec := get("?before_seq=40&limit=10"); rec.Code != http.StatusOK || aud.antesDe != 40 || aud.ventana != 10 {
		t.Fatalf("con parámetros: HTTP=%d antesDe=%d ventana=%d", rec.Code, aud.antesDe, aud.ventana)
	}
	antes := aud.llamadas
	for _, malo := range []string{"?before_seq=-1", "?before_seq=x", "?limit=0", "?limit=99999999", "?limit=x"} {
		if rec := get(malo); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: HTTP=%d, quiero 400", malo, rec.Code)
		}
	}
	if aud.llamadas != antes {
		t.Fatal("una petición inválida llegó al store")
	}
}
