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

// La biblioteca de plantillas de la organización contra PostgreSQL real: quién puede
// marcar, que solo la ven los miembros de la organización, que quitar la marca no borra
// nada, que cada publicación es una versión elegible y que usarla copia el contenido a
// un documento NUEVO de quien la usa.
func TestPlantillasDeOrganizacion(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(h.Pool)
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024, plantillasOrg: st,
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})

	// Una carta de «acme»: la dueña es admin; Edu edita sin administrar; Vera solo lee.
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Carta tipo", OwnerSub: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	for quien, rol := range map[string]domain.Role{"edu": domain.RoleEditor, "vera": domain.RoleViewer} {
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: rol, GrantedBy: "ada"}); err != nil {
			t.Fatal(err)
		}
	}
	reloj := uint64(0)
	escribir := func(contenido string) {
		t.Helper()
		op, err := domain.ContentOp(domain.KindDocument, contenido)
		if err != nil {
			t.Fatal(err)
		}
		reloj++
		op.Lamport = reloj // el LWW decide por el reloj: cada escritura debe ganar a la anterior
		if _, _, err := hd.svc.ApplyOps(ctx, domain.Subject{Sub: "ada"}, doc.ID, []domain.Op{op}, nil); err != nil {
			t.Fatal(err)
		}
	}
	escribir("<p>Estimado cliente, versión uno</p>")

	pedir := func(tenant, metodo, ruta, sub string, roles []string, params map[string]string, cuerpo string, f http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		r := req(t, metodo, ruta, &authn.Claims{Subject: sub, Tenant: tenant, Roles: roles}, params, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: tenant}))
		rec := httptest.NewRecorder()
		f(rec, r)
		return rec
	}
	admin := []string{"admin"}
	id := map[string]string{"id": doc.ID}

	// --- Permisos al marcar ---
	for _, c := range []struct {
		quien string
		roles []string
		code  int
		por   string
	}{
		{"edu", nil, http.StatusForbidden, "editor sin ser administrador"},
		{"ada", nil, http.StatusForbidden, "propietaria sin ser administradora"},
		{"vera", admin, http.StatusForbidden, "administradora pero solo lectora del documento"},
		{"intruso", admin, http.StatusForbidden, "administrador sin acceso al documento"},
	} {
		if rec := pedir("acme", "PUT", "/org-templates/"+doc.ID, c.quien, c.roles, id, `{}`, hd.marcarPlantillaOrg); rec.Code != c.code {
			t.Errorf("%s: %d %s", c.por, rec.Code, rec.Body.String())
		}
	}
	if lista, _ := st.ListarPlantillasOrg(ctx, ""); len(lista) != 0 {
		t.Fatalf("un intento denegado dejó marca: %+v", lista)
	}

	// --- Marcar (administrador y editor del documento) ---
	rec := pedir("acme", "PUT", "/org-templates/"+doc.ID, "ada", admin, id, `{"description":"Para clientes"}`, hd.marcarPlantillaOrg)
	if rec.Code != http.StatusCreated {
		t.Fatalf("marcar: %d %s", rec.Code, rec.Body.String())
	}
	p, err := st.GetPlantillaOrg(ctx, doc.ID)
	if err != nil || p.Name != "Carta tipo" || len(p.Versions) != 1 || p.Versions[0].No != 1 {
		t.Fatalf("tras marcar: %+v %v", p, err)
	}
	v1 := p.Versions[0].VersionID

	// --- Solo la ven los miembros de la organización ---
	galeria := func(tenant, sub string) []struct{ Key, Name string } {
		t.Helper()
		rec := pedir(tenant, "GET", "/api/v1/templates", sub, nil, nil, "", hd.listarPlantillas)
		var body struct {
			Org []struct{ Key, Name string } `json:"org_templates"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err, rec.Body.String())
		}
		return body.Org
	}
	if g := galeria("acme", "cualquiera"); len(g) != 1 || g[0].Key != "org:"+doc.ID {
		t.Errorf("un miembro debe ver la plantilla: %+v", g)
	}
	if g := galeria("otra", "intruso"); len(g) != 0 {
		t.Errorf("otra organización ve plantillas ajenas: %+v", g)
	}
	if rec := pedir("otra", "GET", "/org-templates/"+doc.ID, "intruso", admin, id, "", hd.verPlantillaOrg); rec.Code != http.StatusNotFound {
		t.Errorf("ficha desde otra organización: %d", rec.Code)
	}
	if rec := pedir("otra", "POST", "/documents", "intruso", nil, nil, `{"template":"org:`+doc.ID+`"}`, hd.createDocument); rec.Code != http.StatusNotFound {
		t.Errorf("usarla desde otra organización: %d %s", rec.Code, rec.Body.String())
	}

	// --- Editar el documento NO cambia lo que reciben los miembros hasta publicar ---
	escribir("<p>Estimado cliente, versión dos</p>")
	usar := func(sub, cuerpo string) *domain.Document {
		t.Helper()
		rec := pedir("acme", "POST", "/documents", sub, nil, nil, cuerpo, hd.createDocument)
		if rec.Code != http.StatusCreated {
			t.Fatalf("crear desde plantilla: %d %s", rec.Code, rec.Body.String())
		}
		var r struct{ Document domain.Document }
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return &r.Document
	}
	contenido := func(d *domain.Document) string {
		t.Helper()
		est, _, err := hd.svc.LoadFull(ctx, domain.Subject{Sub: d.OwnerSub}, d.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		return string(est.Materialize().Registers["doc:html"])
	}
	copia := usar("marta", `{"template":"org:`+doc.ID+`"}`)
	if copia.OwnerSub != "marta" || copia.ID == doc.ID || copia.Title != "Carta tipo" || copia.Kind != domain.KindDocument {
		t.Fatalf("la copia debe ser un documento nuevo de quien la usa: %+v", copia)
	}
	if c := contenido(copia); !strings.Contains(c, "versión uno") {
		t.Errorf("sin publicar debe llegar la versión uno: %s", c)
	}
	// La copia es de Marta: ni Ada ni Edu la ven (no se heredan los permisos).
	if _, _, err := hd.svc.Authorize(ctx, copia.ID, domain.Subject{Sub: "edu"}, domain.RoleViewer); err == nil {
		t.Error("la copia heredó permisos de la plantilla")
	}

	// --- Publicar: la ejecuta quien edita el documento, no hace falta ser administrador ---
	if rec := pedir("acme", "POST", "/org-templates/"+doc.ID+"/versions", "vera", nil, id, "", hd.publicarPlantillaOrg); rec.Code != http.StatusForbidden {
		t.Errorf("una lectora publicó: %d", rec.Code)
	}
	if rec := pedir("acme", "POST", "/org-templates/"+doc.ID+"/versions", "edu", nil, id, `{"label":"revisada"}`, hd.publicarPlantillaOrg); rec.Code != http.StatusCreated {
		t.Fatalf("publicar: %d %s", rec.Code, rec.Body.String())
	}
	if c := contenido(usar("marta", `{"template":"org:`+doc.ID+`"}`)); !strings.Contains(c, "versión dos") {
		t.Errorf("la vigente debe ser la dos: %s", c)
	}
	if c := contenido(usar("marta", `{"template":"org:`+doc.ID+`","template_version":"`+v1+`"}`)); !strings.Contains(c, "versión uno") {
		t.Errorf("elegir la versión uno: %s", c)
	}
	if rec := pedir("acme", "POST", "/documents", "marta", nil, nil, `{"template":"org:`+doc.ID+`","template_version":"00000000-0000-0000-0000-000000000000"}`, hd.createDocument); rec.Code != http.StatusNotFound {
		t.Errorf("versión inexistente: %d", rec.Code)
	}

	// --- Quitar la marca: solo administradores, y no borra nada ---
	if rec := pedir("acme", "DELETE", "/org-templates/"+doc.ID, "edu", nil, id, "", hd.quitarPlantillaOrg); rec.Code != http.StatusForbidden {
		t.Errorf("quitar sin ser administrador: %d", rec.Code)
	}
	if rec := pedir("acme", "DELETE", "/org-templates/"+doc.ID, "ada", admin, id, "", hd.quitarPlantillaOrg); rec.Code != http.StatusNoContent {
		t.Fatalf("quitar: %d", rec.Code)
	}
	if g := galeria("acme", "marta"); len(g) != 0 {
		t.Errorf("quitada pero sigue en la galería: %+v", g)
	}
	if _, _, err := hd.svc.Authorize(ctx, doc.ID, domain.Subject{Sub: "ada"}, domain.RoleOwner); err != nil {
		t.Errorf("quitar la marca tocó el documento: %v", err)
	}
	if _, err := st.ListVersions(ctx, doc.ID, 10); err != nil {
		t.Error(err)
	}
	// Volver a marcarla sigue la numeración y conserva las versiones viejas.
	if rec := pedir("acme", "PUT", "/org-templates/"+doc.ID, "ada", admin, id, `{"name":"Carta de la casa"}`, hd.marcarPlantillaOrg); rec.Code != http.StatusCreated {
		t.Fatalf("remarcar: %d %s", rec.Code, rec.Body.String())
	}
	if p, _ := st.GetPlantillaOrg(ctx, doc.ID); p == nil || p.Name != "Carta de la casa" || len(p.Versions) != 3 || p.Versions[0].No != 3 {
		t.Errorf("tras remarcar: %+v", p)
	}

	// --- Auditoría: solo administradores ---
	if rec := pedir("acme", "GET", "/org-templates/"+doc.ID+"/log", "edu", nil, id, "", hd.registroPlantillaOrg); rec.Code != http.StatusForbidden {
		t.Errorf("registro sin ser administrador: %d", rec.Code)
	}
	rec = pedir("acme", "GET", "/org-templates/"+doc.ID+"/log", "ada", admin, id, "", hd.registroPlantillaOrg)
	var log struct {
		Entries []struct{ Action string } `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &log); err != nil {
		t.Fatal(err)
	}
	cuenta := map[string]int{}
	for _, e := range log.Entries {
		cuenta[e.Action]++
	}
	if cuenta["marked"] != 2 || cuenta["published"] != 1 || cuenta["unmarked"] != 1 || cuenta["used"] != 3 {
		t.Errorf("registro de auditoría: %v", cuenta)
	}
}
