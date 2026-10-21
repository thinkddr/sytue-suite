// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// La marca de la organización y los temas contra PostgreSQL real: solo la cambia quien
// administra y cada cambio es una versión; se aplica al crear y a un documento que ya
// existe SIN tocar su texto, con un punto de restauración antes, y la marca de una
// organización no existe para otra.
func TestMarcaYTemasDeOrganizacion(t *testing.T) {
	pg := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(pg.Pool)
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024, marcaOrg: st,
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	admin := []string{"admin"}

	pedir := func(tenant, metodo, sub string, roles []string, params map[string]string, cuerpo string, f http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		r := req(t, metodo, "/x", &authn.Claims{Subject: sub, Tenant: tenant, Roles: roles}, params, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: tenant}))
		rec := httptest.NewRecorder()
		f(rec, r)
		return rec
	}
	marca := func(acento string) string {
		return `{"name":"Acme","bg":"#FFF7ED","ink":"#1C1917","accent":"` + acento + `","heading_font":"Georgia","body_font":"Trebuchet MS"}`
	}

	// --- La marca: solo administradores, y cada guardado es una versión ---
	if rec := pedir("acme", "PUT", "edu", nil, nil, marca("#B45309"), hd.guardarMarcaOrg); rec.Code != http.StatusForbidden {
		t.Errorf("guardar la marca sin administrar: %d", rec.Code)
	}
	if rec := pedir("acme", "PUT", "ada", admin, nil, `{"name":"Acme","bg":"rojo","ink":"#000","accent":"#111"}`, hd.guardarMarcaOrg); rec.Code != http.StatusBadRequest {
		t.Errorf("marca con un color inválido: %d", rec.Code)
	}
	for i, ac := range []string{"#B45309", "#0F766E"} {
		rec := pedir("acme", "PUT", "ada", admin, nil, marca(ac), hd.guardarMarcaOrg)
		var r struct{ Version int }
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &r) != nil || r.Version != i+1 {
			t.Fatalf("guardar la marca %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	listar := func(tenant string) (brand map[string]any, versiones int, puede bool) {
		rec := pedir(tenant, "GET", "cualquiera", nil, nil, "", hd.listarTemas)
		var b struct {
			Brand     map[string]any `json:"brand"`
			Versions  []any          `json:"versions"`
			CanManage bool           `json:"can_manage"`
			Themes    []any          `json:"themes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || len(b.Themes) < 6 {
			t.Fatalf("listar temas: %v %s", err, rec.Body.String())
		}
		return b.Brand, len(b.Versions), b.CanManage
	}
	if b, n, puede := listar("acme"); b["accent"] != "#0F766E" || b["version"].(float64) != 2 || n != 2 || puede {
		t.Errorf("la marca vigente es la 2 y quedan las dos versiones: %v %d %v", b, n, puede)
	}
	if b, n, _ := listar("otra"); b != nil || n != 0 {
		t.Errorf("otra organización ve la marca de acme: %v %d", b, n)
	}

	// --- Al crear: la plantilla de Sytue con la marca ---
	crear := func(tenant, cuerpo string) (int, *domain.Document) {
		rec := pedir(tenant, "POST", "marta", nil, nil, cuerpo, hd.createDocument)
		var r struct{ Document domain.Document }
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		return rec.Code, &r.Document
	}
	contenido := func(d *domain.Document) string {
		t.Helper()
		est, _, err := hd.svc.LoadFull(ctx, domain.Subject{Sub: d.OwnerSub}, d.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var s string
		_ = json.Unmarshal(est.Materialize().Registers[domain.ContentKey(d.Kind)], &s)
		return s
	}
	code, sin := crear("acme", `{"template":"carta","locale":"es"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear sin tema: %d", code)
	}
	code, con := crear("acme", `{"template":"informe","locale":"es","theme":"marca"}`)
	if code != http.StatusCreated {
		t.Fatalf("crear con la marca: %d", code)
	}
	if c := contenido(con); !strings.Contains(c, `data-tema="marca:`) || !strings.Contains(c, "#0F766E") || strings.Contains(contenido(sin), "data-tema") {
		t.Errorf("la marca vigente debe estar en el documento nuevo (y solo en él): %.200s", c)
	}
	// Una versión anterior de la marca, a elección.
	if _, v1 := crear("acme", `{"template":"informe","locale":"es","theme":"marca","brand_version":1}`); !strings.Contains(contenido(v1), "#B45309") {
		t.Error("brand_version 1 debe aplicar el acento de la versión 1")
	}
	if code, _ := crear("acme", `{"template":"carta","theme":"no-existe"}`); code != http.StatusBadRequest {
		t.Errorf("tema desconocido: %d", code)
	}
	if code, _ := crear("otra", `{"template":"carta","theme":"marca"}`); code != http.StatusNotFound {
		t.Errorf("la marca de una organización sin marca: %d", code)
	}
	// Una hoja y una presentación de la galería también.
	if _, h := crear("acme", `{"template":"presupuesto","locale":"es","theme":"carbon"}`); !strings.Contains(contenido(h), `"fill":"#1F2937"`) {
		t.Errorf("tema en una hoja: %.300s", contenido(h))
	}
	if _, p := crear("acme", `{"template":"proyecto","locale":"es","theme":"marca"}`); !strings.Contains(contenido(p), `"custom-marca"`) {
		t.Errorf("marca en una presentación: %.300s", contenido(p))
	}

	// --- A un documento que ya existe ---
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Carta", OwnerSub: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	for quien, rol := range map[string]domain.Role{"edu": domain.RoleEditor, "vera": domain.RoleViewer} {
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: rol, GrantedBy: "ada"}); err != nil {
			t.Fatal(err)
		}
	}
	op, _ := domain.ContentOp(domain.KindDocument, "<h1>Carta</h1><p>Estimado cliente</p>")
	if _, _, err := hd.svc.ApplyOps(ctx, domain.Subject{Sub: "ada"}, doc.ID, []domain.Op{op}, nil); err != nil {
		t.Fatal(err)
	}
	id := map[string]string{"id": doc.ID}
	aplicar := func(sub, cuerpo string) *httptest.ResponseRecorder {
		return pedir("acme", "POST", sub, nil, id, cuerpo, hd.aplicarTema)
	}
	if rec := aplicar("vera", `{"theme":"marca"}`); rec.Code != http.StatusForbidden {
		t.Errorf("un lector aplicó un tema: %d", rec.Code)
	}
	if rec := aplicar("intruso", `{"theme":"marca"}`); rec.Code != http.StatusForbidden {
		t.Errorf("quien no tiene acceso aplicó un tema: %d", rec.Code)
	}
	if rec := aplicar("edu", `{"theme":"nada"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("tema desconocido: %d", rec.Code)
	}
	// Previsualizar: un lector no puede, un editor ve el diff y NO se escribe nada.
	if rec := aplicar("vera", `{"theme":"marca","preview":true}`); rec.Code != http.StatusForbidden {
		t.Errorf("un lector previsualizó: %d", rec.Code)
	}
	prev := aplicar("edu", `{"theme":"marca","preview":true}`)
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), `"scope":"headings"`) || !strings.Contains(prev.Body.String(), `"version":2`) {
		t.Fatalf("previsualizar: %d %s", prev.Code, prev.Body.String())
	}
	if c := contenido(doc); strings.Contains(c, "data-tema") {
		t.Fatalf("previsualizar escribió: %s", c)
	}
	if vs, _ := st.ListVersions(ctx, doc.ID, 10); len(vs) != 0 {
		t.Fatalf("previsualizar dejó un punto de restauración: %d", len(vs))
	}
	rec := aplicar("edu", `{"theme":"marca"}`)
	var res struct {
		Changed bool `json:"changed"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || !res.Changed {
		t.Fatalf("aplicar: %d %s", rec.Code, rec.Body.String())
	}
	c := contenido(doc)
	if !strings.Contains(c, `<h1 style="color:#0F766E;font-family:Georgia" data-tema="marca:color,font-family">Carta</h1>`) || !strings.Contains(c, "Estimado cliente") {
		t.Fatalf("contenido tras el tema: %s", c)
	}
	// Dejó un punto de restauración con el estado de partida, y restaurarlo lo deshace.
	vs, _ := st.ListVersions(ctx, doc.ID, 10)
	if len(vs) != 1 || vs[0].Label == nil || !strings.HasPrefix(*vs[0].Label, "Antes de la marca v2") {
		t.Fatalf("punto de restauración: %+v", vs)
	}
	// Aplicarlo otra vez no escribe ni crea otra versión.
	if rec := aplicar("edu", `{"theme":"marca"}`); !strings.Contains(rec.Body.String(), `"changed":false`) {
		t.Errorf("segunda aplicación: %s", rec.Body.String())
	}
	if vs, _ := st.ListVersions(ctx, doc.ID, 10); len(vs) != 1 {
		t.Errorf("la segunda aplicación creó una versión: %d", len(vs))
	}
	if _, err := hd.svc.RestoreVersion(ctx, domain.Subject{Sub: "ada"}, doc.ID, vs[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	if c := contenido(doc); strings.Contains(c, "data-tema") || !strings.Contains(c, "<h1>Carta</h1>") {
		t.Errorf("restaurar no devolvió el documento a como estaba: %s", c)
	}
}
