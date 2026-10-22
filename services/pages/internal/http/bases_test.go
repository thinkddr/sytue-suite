// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/basesconn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func pedirTablaDeBases(h *Handlers, roles []string, cuerpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites/x/bases/table", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "00000000-0000-4000-8000-000000000001")
	ctx := tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"})
	ctx = authn.Into(ctx, authn.Claims{Subject: "ana", Roles: roles, Tenant: "acme"})
	w := httptest.NewRecorder()
	h.tablaDesdeBases(w, req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx)))
	return w
}

// Solo quien puede publicar trae datos de Bases a un sitio, y lo que no es un token de
// conexión no sale hacia Bases. Las dos puertas van ANTES de tocar el store o la red: el
// manejador se prueba sin ninguno de los dos y un fallo aquí sería un pánico.
func TestTablaDesdeBasesExigeAdminYTokenDeConexion(t *testing.T) {
	llamadas := 0
	bases := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { llamadas++ }))
	defer bases.Close()
	h := &Handlers{Deps: Deps{Cfg: config.Config{BasesURL: bases.URL}}}
	const bueno = `{"token":"sbc_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`
	if w := pedirTablaDeBases(h, []string{"member"}, bueno); w.Code != http.StatusForbidden {
		t.Errorf("un miembro trae datos de Bases: %d %s", w.Code, w.Body)
	}
	for _, malo := range []string{`{}`, `{"token":"eyJ.un.jwt"}`, `{"token":"sbc_corto"}`} {
		if w := pedirTablaDeBases(h, []string{"admin"}, malo); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "bases_connection_invalid") {
			t.Errorf("token %s: %d %s", malo, w.Code, w.Body)
		}
	}
	if llamadas != 0 {
		t.Errorf("salieron %d peticiones hacia Sytue Bases", llamadas)
	}
}

func TestATablaDeSitio(t *testing.T) {
	l := &basesconn.Lectura{
		Columns:    []basesconn.Columna{{Name: "<b>Cliente</b>", Type: "text"}, {Name: "Importe", Type: "number"}, {Name: "Activo", Type: "boolean"}},
		Rows:       [][]any{{"<script>alert(1)</script>", 1234567.5, true}, {nil, float64(-3), false}},
		Provenance: basesconn.Procedencia{ViewName: "Grandes", BaseName: "Tienda", GrantedBy: "ana"},
	}
	out, err := aTablaDeSitio(l)
	if err != nil {
		t.Fatal(err)
	}
	// Texto plano tal cual (lo escapa el renderizador, como en cualquier tabla), números sin
	// notación científica y vacío para lo que no tiene valor.
	if out.Caption != "Grandes" || out.Header[0] != "<b>Cliente</b>" || out.Rows[0][0] != "<script>alert(1)</script>" ||
		out.Rows[0][1] != "1234567.5" || out.Rows[0][2] != "true" || out.Rows[1][0] != "" || out.Rows[1][1] != "-3" {
		t.Errorf("tabla: %+v", out)
	}
	if out.Provenance.GrantedBy != "ana" {
		t.Errorf("procedencia: %+v", out.Provenance)
	}

	// Lo que no cabe se rechaza entero, con su motivo: no se publica media vista.
	l.Rows = make([][]any, ir.Limites.FilasTabla+1)
	for i := range l.Rows {
		l.Rows[i] = []any{"a", 1.0, true}
	}
	if _, err := aTablaDeSitio(l); err == nil || !strings.Contains(err.Error(), "bases_view_too_large") && !strings.Contains(err.Error(), "Narrow the view") {
		t.Errorf("vista con demasiadas filas: %v", err)
	}
	l.Rows = l.Rows[:1]
	l.Columns = make([]basesconn.Columna, ir.Limites.ColumnasTabla+1)
	if _, err := aTablaDeSitio(l); err == nil {
		t.Error("vista con demasiadas columnas: aceptada")
	}
}
