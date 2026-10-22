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

// Los bloques reutilizables contra PostgreSQL real: el catálogo de Sytue sale en el idioma
// pedido y con la marca vigente (o con la versión que se pida); un documento de la
// biblioteca marcado como bloque se ofrece para insertar y NO para crear, con sus
// versiones; su HTML llega saneado; y los bloques de una organización no existen para otra.
func TestBloquesReutilizables(t *testing.T) {
	pg := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(pg.Pool)
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024, marcaOrg: st, plantillasOrg: st,
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	admin := []string{"admin"}

	pedir := func(tenant, metodo, ruta, sub string, roles []string, params map[string]string, cuerpo string, f http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		r := req(t, metodo, ruta, &authn.Claims{Subject: sub, Tenant: tenant, Roles: roles}, params, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: tenant}))
		rec := httptest.NewRecorder()
		f(rec, r)
		return rec
	}
	type lista struct {
		Blocks    []struct{ Key, Name string } `json:"blocks"`
		OrgBlocks []domain.PlantillaOrg        `json:"org_blocks"`
	}
	listar := func(tenant, kind string) lista {
		t.Helper()
		rec := pedir(tenant, "GET", "/x?kind="+kind+"&locale=es", "marta", nil, nil, "", hd.listarBloques)
		var l lista
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &l) != nil {
			t.Fatalf("listar %s: %d %s", kind, rec.Code, rec.Body.String())
		}
		return l
	}
	type bloque struct {
		Kind         string `json:"kind"`
		Content      string `json:"content"`
		BrandVersion int    `json:"brand_version"`
		Version      int    `json:"version"`
	}
	obtener := func(tenant, clave, consulta string) (int, bloque) {
		t.Helper()
		rec := pedir(tenant, "GET", "/x?locale=es"+consulta, "marta", nil, map[string]string{"clave": clave}, "", hd.obtenerBloque)
		var b bloque
		_ = json.Unmarshal(rec.Body.Bytes(), &b)
		return rec.Code, b
	}

	// --- El catálogo de Sytue: por tipo, en el idioma pedido ---
	for kind, quiere := range map[string]string{"document": "b_firmas", "spreadsheet": "b_totales", "presentation": "b_agenda"} {
		l := listar("acme", kind)
		hay := false
		for _, b := range l.Blocks {
			hay = hay || b.Key == quiere
		}
		if !hay || len(l.Blocks) < 3 || len(l.OrgBlocks) != 0 {
			t.Errorf("%s: falta %s o sobran bloques de organización: %+v", kind, quiere, l)
		}
	}
	if rec := pedir("acme", "GET", "/x?kind=nada", "marta", nil, nil, "", hd.listarBloques); rec.Code != http.StatusBadRequest {
		t.Errorf("tipo desconocido: %d", rec.Code)
	}
	if rec := pedir("acme", "GET", "/x?kind=document", "", nil, nil, "", func(w http.ResponseWriter, r *http.Request) {
		hd.listarBloques(w, r.WithContext(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})))
	}); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin sesión: %d", rec.Code)
	}

	// --- Sin marca sale neutro; con marca, vestido con la vigente o con la versión pedida ---
	code, neutro := obtener("acme", "b_portada", "")
	if code != http.StatusOK || neutro.Kind != "document" || !strings.Contains(neutro.Content, "<h1>[Título del documento]</h1>") || neutro.BrandVersion != 0 {
		t.Fatalf("bloque sin marca: %d %+v", code, neutro)
	}
	if code, _ := obtener("acme", "b_portada", "&theme=marca"); code != http.StatusNotFound {
		t.Errorf("pedir la marca de una organización que no tiene: %d", code)
	}
	for _, ac := range []string{"#B45309", "#0F766E"} {
		cuerpo := `{"name":"Acme","bg":"#FFF7ED","ink":"#1C1917","accent":"` + ac + `","heading_font":"Georgia"}`
		if rec := pedir("acme", "PUT", "/x", "ada", admin, nil, cuerpo, hd.guardarMarcaOrg); rec.Code != http.StatusCreated {
			t.Fatalf("guardar la marca: %d %s", rec.Code, rec.Body.String())
		}
	}
	if _, b := obtener("acme", "b_portada", ""); b.BrandVersion != 2 || !strings.Contains(b.Content, "color:#0F766E") || !strings.Contains(b.Content, `data-tema="marca:`) {
		t.Errorf("la marca vigente no vistió el bloque: %+v", b)
	}
	if _, b := obtener("acme", "b_portada", "&brand_version=1"); b.BrandVersion != 1 || !strings.Contains(b.Content, "#B45309") {
		t.Errorf("brand_version=1: %+v", b)
	}
	if _, b := obtener("acme", "b_portada", "&theme=papel"); strings.Contains(b.Content, "style=") {
		t.Errorf("theme=papel debe salir neutro: %+v", b)
	}
	if _, b := obtener("acme", "b_grafico", ""); !strings.Contains(b.Content, `"colors":["#0F766E"]`) || !strings.Contains(b.Content, `"fill":"#0F766E"`) {
		t.Errorf("el gráfico y la cabecera de un bloque de hoja llevan la marca: %+v", b)
	}
	if _, b := obtener("otra", "b_portada", ""); strings.Contains(b.Content, "0F766E") {
		t.Errorf("otra organización recibió la marca de acme: %+v", b)
	}
	if code, _ := obtener("acme", "no-existe", ""); code != http.StatusNotFound {
		t.Errorf("bloque desconocido: %d", code)
	}
	if code, _ := obtener("acme", "b_portada", "&theme=inventado"); code != http.StatusBadRequest {
		t.Errorf("tema desconocido: %d", code)
	}

	// --- Un bloque de la organización: un documento de su biblioteca, con versiones ---
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Pie de Acme", OwnerSub: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	reloj := uint64(0)
	escribir := func(contenido string) {
		t.Helper()
		op, err := domain.ContentOp(domain.KindDocument, contenido)
		if err != nil {
			t.Fatal(err)
		}
		reloj++
		op.Lamport = reloj
		if _, _, err := hd.svc.ApplyOps(ctx, domain.Subject{Sub: "ada"}, doc.ID, []domain.Op{op}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Con HTML hostil: lo que salga va al documento de otra persona.
	escribir(`<h2 onclick="alert(1)">Pie legal</h2><p>Acme, S. L.<script>alert(2)</script><img src=x onerror="alert(3)"></p>`)
	id := map[string]string{"id": doc.ID}
	if rec := pedir("acme", "PUT", "/x", "edu", nil, id, `{"block":true}`, hd.marcarPlantillaOrg); rec.Code != http.StatusForbidden {
		t.Errorf("marcar un bloque sin administrar: %d", rec.Code)
	}
	if rec := pedir("acme", "PUT", "/x", "ada", admin, id, `{"block":true}`, hd.marcarPlantillaOrg); rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"block":true`) {
		t.Fatalf("marcar como bloque: %d %s", rec.Code, rec.Body.String())
	}
	clave := domain.PrefijoPlantillaOrg + doc.ID
	if l := listar("acme", "document"); len(l.OrgBlocks) != 1 || l.OrgBlocks[0].Key != clave || len(l.OrgBlocks[0].Versions) != 1 {
		t.Fatalf("el bloque no está en la lista de su organización: %+v", l.OrgBlocks)
	}
	if l := listar("acme", "spreadsheet"); len(l.OrgBlocks) != 0 {
		t.Errorf("un bloque de documento no se ofrece en una hoja: %+v", l.OrgBlocks)
	}
	// No es una plantilla para crear: la galería no lo enseña.
	if g := hd.plantillasOrgEnGaleria(req(t, "GET", "/x", &authn.Claims{Subject: "marta", Tenant: "acme"}, nil, "").WithContext(ctx)); len(g) != 0 {
		t.Errorf("la galería de «Crear nuevo» enseña un bloque: %+v", g)
	}
	code, b := obtener("acme", clave, "")
	if code != http.StatusOK || b.Version != 1 || !strings.Contains(b.Content, "Pie legal") || !strings.Contains(b.Content, "Acme, S. L.") {
		t.Fatalf("bloque de la organización: %d %+v", code, b)
	}
	for _, malo := range []string{"<script", "onclick", "onerror", "alert("} {
		if strings.Contains(b.Content, malo) {
			t.Errorf("el bloque de la organización conserva %q: %s", malo, b.Content)
		}
	}
	if !strings.Contains(b.Content, "#0F766E") {
		t.Errorf("el bloque de la organización también lleva la marca vigente: %s", b.Content)
	}
	// Una versión nueva: la vigente cambia y la anterior sigue pidiéndose por su número.
	escribir("<h2>Pie legal</h2><p>Acme, S. A.</p>")
	if rec := pedir("acme", "POST", "/x", "ada", nil, id, "", hd.publicarPlantillaOrg); rec.Code != http.StatusCreated {
		t.Fatalf("publicar la versión 2: %d %s", rec.Code, rec.Body.String())
	}
	if _, b := obtener("acme", clave, ""); b.Version != 2 || !strings.Contains(b.Content, "S. A.") {
		t.Errorf("la vigente debe ser la 2: %+v", b)
	}
	if _, b := obtener("acme", clave, "&version=1"); b.Version != 1 || !strings.Contains(b.Content, "S. L.") {
		t.Errorf("version=1: %+v", b)
	}
	if code, _ := obtener("acme", clave, "&version=9"); code != http.StatusNotFound {
		t.Errorf("versión que no existe: %d", code)
	}
	// Aislamiento: otra organización ni lo lista ni lo obtiene, aun sabiendo el id.
	if l := listar("otra", "document"); len(l.OrgBlocks) != 0 {
		t.Errorf("otra organización lista el bloque de acme: %+v", l.OrgBlocks)
	}
	if code, b := obtener("otra", clave, ""); code != http.StatusNotFound || b.Content != "" {
		t.Errorf("otra organización obtuvo el bloque de acme: %d %+v", code, b)
	}
	// Una plantilla corriente (no bloque) no se sirve como bloque.
	otro, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Carta", OwnerSub: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	op, _ := domain.ContentOp(domain.KindDocument, "<p>Carta</p>")
	if _, _, err := hd.svc.ApplyOps(ctx, domain.Subject{Sub: "ada"}, otro.ID, []domain.Op{op}, nil); err != nil {
		t.Fatal(err)
	}
	if rec := pedir("acme", "PUT", "/x", "ada", admin, map[string]string{"id": otro.ID}, `{}`, hd.marcarPlantillaOrg); rec.Code != http.StatusCreated {
		t.Fatalf("marcar una plantilla: %d", rec.Code)
	}
	if code, _ := obtener("acme", domain.PrefijoPlantillaOrg+otro.ID, ""); code != http.StatusNotFound {
		t.Errorf("una plantilla que no es bloque se sirvió como bloque: %d", code)
	}
	if g := hd.plantillasOrgEnGaleria(req(t, "GET", "/x", &authn.Claims{Subject: "marta", Tenant: "acme"}, nil, "").WithContext(ctx)); len(g) != 1 {
		t.Errorf("la galería debe enseñar la plantilla y no el bloque: %+v", g)
	}
}
