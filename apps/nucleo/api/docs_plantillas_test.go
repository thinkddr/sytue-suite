// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDocsPlantillasHablaElCableDelServicio recorre las rutas con lo que contesta
// services/docs/internal/http (plantillas.go, temas.go, documents.go).
func TestDocsPlantillasHablaElCableDelServicio(t *testing.T) {
	cuerpos := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		clave := r.Method + " " + r.URL.RequestURI()
		cuerpos[clave] = string(b)
		switch clave {
		case "GET /api/v1/templates?locale=es":
			_, _ = io.WriteString(w, `{"templates":[{"key":"carta","kind":"document","name":"Carta","description":"d"}],
"org_templates":[{"key":"org:d1","kind":"document","name":"Acta","description":"","versions":[{"no":2,"id":"v2"},{"no":1,"id":"v1"}]}],"can_manage":true}`)
		case "GET /api/v1/themes":
			_, _ = io.WriteString(w, `{"themes":[{"id":"carbon"}],"brand":{"version":3,"name":"Acme","bg":"#FFFFFF","ink":"#0A0A0A","accent":"#1F2937"},"versions":[{"version":3,"name":"Acme","bg":"#FFFFFF","ink":"#0A0A0A","accent":"#1F2937"}],"can_manage":true}`)
		case "PUT /api/v1/brand":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"brand":{},"version":4}`)
		case "POST /api/v1/documents":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"document":{"id":"n1","title":"Acta","kind":"document"},"role":"owner"}`)
		case "POST /api/v1/documents/d9/theme":
			_, _ = io.WriteString(w, `{"changed":true}`)
		case "POST /api/v1/documents/d8/theme":
			_, _ = io.WriteString(w, `{"changed":true,"changes":[{"scope":"headings","count":3,"from":"","to":"#1F2937"}],"version":4}`)
		default:
			t.Errorf("petición inesperada: %s", clave)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()

	g, err := d.Templates(ctx, "es")
	if err != nil || len(g.Templates) != 1 || !g.CanManage || len(g.OrgTemplates) != 1 ||
		len(g.OrgTemplates[0].Versions) != 2 || g.OrgTemplates[0].Versions[0].ID != "v2" {
		t.Fatalf("galería: %+v, %v", g, err)
	}
	th, err := d.Themes(ctx)
	if err != nil || th.Brand == nil || th.Brand.Version != 3 || len(th.Versions) != 1 || th.Themes[0].ID != "carbon" {
		t.Fatalf("temas: %+v, %v", th, err)
	}
	v, err := d.SaveBrand(ctx, DocBrand{Version: 3, Name: "Acme", Bg: "#FFFFFF", Ink: "#000000", Accent: "#123456", HeadingFont: "Georgia"})
	if err != nil || v != 4 {
		t.Fatalf("marca: %d, %v", v, err)
	}
	if got := cuerpos["PUT /api/v1/brand"]; got != `{"name":"Acme","bg":"#FFFFFF","ink":"#000000","accent":"#123456","heading_font":"Georgia"}` {
		t.Errorf("cuerpo de la marca (sin version: la pone el servicio): %s", got)
	}
	doc, err := d.CreateFromTemplate(ctx, DocNuevo{Template: "org:d1", Locale: "es", TemplateVersion: "v1", Theme: IDTemaMarca, BrandVersion: 2})
	if err != nil || doc.ID != "n1" {
		t.Fatalf("crear: %+v, %v", doc, err)
	}
	if got := cuerpos["POST /api/v1/documents"]; got != `{"template":"org:d1","locale":"es","template_version":"v1","theme":"marca","brand_version":2}` {
		t.Errorf("cuerpo de crear: %s", got)
	}
	if cambio, err := d.ApplyTheme(ctx, "d9", "carbon", 0); err != nil || !cambio {
		t.Errorf("tema: %v, %v", cambio, err)
	}
	pv, err := d.PreviewTheme(ctx, "d8", IDTemaMarca, 0)
	if err != nil || !pv.Changed || pv.Version != 4 || len(pv.Changes) != 1 || pv.Changes[0].Count != 3 {
		t.Errorf("previa: %+v, %v", pv, err)
	}
	if got := cuerpos["POST /api/v1/documents/d8/theme"]; got != `{"theme":"marca","preview":true}` {
		t.Errorf("cuerpo de la previa: %s", got)
	}
	if got := cuerpos["POST /api/v1/documents/d9/theme"]; got != `{"theme":"carbon"}` {
		t.Errorf("cuerpo del tema: %s", got)
	}
}
