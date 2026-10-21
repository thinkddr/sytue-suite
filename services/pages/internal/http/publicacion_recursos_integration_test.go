// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
	"git.sytue.eu/sytue/one/services/pages/internal/sitios"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/testutil"
)

// El renderer TypeScript real produce el multipart que recibe la API; PostgreSQL
// y el cliente S3 son reales. La primera preview funciona antes del primer live.
func TestPublicacionInicialConRecursosReales(t *testing.T) {
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	draft, err := os.ReadFile("testdata/publicacion-ir.json")
	if err != nil {
		t.Fatal(err)
	}
	legal := domain.DatosLegales{HolderName: "Titular de pruebas", TaxID: "TEST-01", Country: "FR", Email: "publicacion@example.test", Address: domain.DireccionLegal{Street: "Rue de test", PostalCode: "75001", City: "Paris"}}
	legalJSON, err := json.Marshal(legal)
	if err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO sites (tenant,slug,name,draft,created_by,legal) VALUES ('acme','primero','Primero',$1,'sub',$2) RETURNING id`, draft, legalJSON).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	cat, err := catalogo.CargarPorDefecto()
	if err != nil {
		t.Fatal(err)
	}
	almacen := testutil.NuevoAlmacen(t)
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	pdfBytes := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF")
	nombres := make([]string, 0, 2)
	for _, asset := range []struct {
		ID, Ext   string
		Contenido []byte
	}{{"aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "png", pngBytes.Bytes()}, {"bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb", "pdf", pdfBytes}} {
		hash := sha256.Sum256(asset.Contenido)
		file := hex.EncodeToString(hash[:16])
		if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO assets (id,tenant,site_id,file,ext,width,height,bytes,origin,created_by) VALUES ($1,'acme',$2,$3,$4,1,1,$5,'upload','sub')`, asset.ID, id, file, asset.Ext, len(asset.Contenido))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		nombre := file + "." + asset.Ext
		nombres = append(nombres, nombre)
		almacen.Escribir("shared/sitios/"+id.String()+"/recursos/"+nombre, asset.Contenido)
	}
	st := store.New(pg.Pool)
	h := &Handlers{Deps: Deps{Cfg: config.Config{Sitios: config.Sitios{SitesDomain: "sites.example.test"}}, Store: st, Objetos: almacen.Objetos, Catalogo: cat, Publicar: publicar.Nuevo(), Mapa: mapa.NuevoEscritor(st, almacen.Objetos, "sites.example.test")}}
	router := chi.NewRouter()
	router.Post("/sites/{id}/publications", h.subirPublicacion)
	router.Get("/sites/{id}/files/{nombre}", h.sitioFichero)
	router.Post("/sites/{id}/assets", h.subirRecurso)
	router.Get("/catalog/files/{nombre}", h.catalogoFichero)
	site, err := st.SitioCompleto(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	datos := h.construirDatosRender(ctx, site)
	token := "0123456789abcdef0123456789abcdef"
	render := func(base string, vista bool) []publicar.FicheroSalida {
		t.Helper()
		input, err := json.Marshal(map[string]any{"ir": json.RawMessage(draft), "datos": datos, "base": base, "vista": vista})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, "bun", "run", "testdata/render-publicacion.ts")
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("renderer: %v: %s", err, stderr.String())
		}
		var raw []struct {
			Ruta      string `json:"ruta"`
			Contenido string `json:"contenido"`
		}
		if err := json.Unmarshal(output, &raw); err != nil {
			t.Fatal(err)
		}
		files := make([]publicar.FicheroSalida, 0, len(raw))
		for _, f := range raw {
			files = append(files, publicar.FicheroSalida{Ruta: f.Ruta, Contenido: []byte(f.Contenido)})
		}
		return files
	}
	var versionForzada uuid.UUID
	subir := func(kind, token string, files []publicar.FicheroSalida, roles []string) *httptest.ResponseRecorder {
		t.Helper()
		purpose := kind
		if kind == "live" {
			purpose = "publish"
		}
		version, err := st.CrearVersionConTipo(ctx, id, "", purpose, draft, site.DraftRev, "sub")
		if err != nil {
			t.Fatal(err)
		}
		if versionForzada != uuid.Nil {
			version = versionForzada
		}
		manifest := ManifiestoPublicacion{VersionID: version, Kind: kind, RendererVersion: "1.0.0"}
		for _, f := range files {
			hash := sha256.Sum256(f.Contenido)
			manifest.Files = append(manifest.Files, FicheroManifiesto{Path: f.Ruta, Bytes: int64(len(f.Contenido)), SHA256: hex.EncodeToString(hash[:])})
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormField("manifest")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(part).Encode(manifest); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			part, err = writer.CreateFormFile("file", f.Ruta)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(f.Contenido); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := conClaims(httptest.NewRequest(http.MethodPost, "/sites/"+id.String()+"/publications?kind="+kind+"&token="+token, &body), roles, "sub")
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"}))
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, tenant := range []string{"acme", "otro"} {
		req := httptest.NewRequest(http.MethodGet, "/sites/"+id.String()+"/files/"+nombres[0], http.NoBody)
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: tenant}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		esperado := http.StatusOK
		if tenant != "acme" {
			esperado = http.StatusNotFound
		}
		if rec.Code != esperado {
			t.Fatalf("lectura recurso tenant=%s status=%d", tenant, rec.Code)
		}
	}
	escritos := almacen.Escrituras()
	ajena := conClaims(httptest.NewRequest(http.MethodPost, "/sites/"+id.String()+"/assets", bytes.NewReader(pdfBytes)), []string{"admin"}, "sub")
	ajena = ajena.WithContext(tenancy.Into(ajena.Context(), tenancy.Tenant{Slug: "otro"}))
	rechazo := httptest.NewRecorder()
	router.ServeHTTP(rechazo, ajena)
	if rechazo.Code != 404 || almacen.Escrituras() != escritos {
		t.Fatalf("subida ajena status=%d escrituras=%d antes=%d", rechazo.Code, almacen.Escrituras(), escritos)
	}
	preview := render("/_vista/"+token, true)
	if err := h.validarPublicacionSalida(ctx, site, "preview", token, preview); err == nil {
		t.Fatal("recursos embebidos aún ausentes en S3 pasan validación")
	}
	if err := cat.Materializar(ctx, almacen.Objetos); err != nil {
		t.Fatal(err)
	}
	if err := h.validarPublicacionSalida(ctx, site, "preview", token, preview); err != nil {
		t.Fatalf("validación real preview: %v", err)
	}
	if rec := subir("preview", token, preview, []string{"admin"}); rec.Code != 200 {
		t.Fatalf("preview inicial=%d: %s", rec.Code, rec.Body.String())
	}
	lector := mapa.NuevoLector(almacen.Objetos, time.Minute)
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatal(err)
	}
	publico := sitios.NuevoRouter("test", config.Sitios{}, lector, almacen.Objetos)
	pedir := func(ruta string, status int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		req.Host = "primero.sites.example.test"
		rec := httptest.NewRecorder()
		publico.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("GET %s=%d want %d: %s", ruta, rec.Code, status, rec.Body.String())
		}
	}
	pedir("/es/", 404)
	pedir("/_vista/"+token+"/es/", 200)
	for _, face := range datos.Fonts["work-sans"].Faces {
		pedir("/_vista/"+token+"/_astro/"+face.File, 200)
		pedir("/_astro/"+face.File, 404)
	}
	for _, nombre := range nombres {
		pedir("/_vista/"+token+"/_astro/"+nombre, 200)
	}
	foto := cat.Fotos[0]
	pedir("/_vista/"+token+"/_astro/"+foto.Fichero, 200)
	for _, tenant := range []string{"acme", "otro"} {
		org := tenancy.Into(context.Background(), tenancy.Tenant{Slug: tenant})
		var otroSitio uuid.UUID
		if err := pg.Pool.InTenantTx(org, func(tx pgx.Tx) error {
			return tx.QueryRow(org, `INSERT INTO sites (tenant,slug,name,draft,created_by) VALUES ($1,$2,'Otro',$3,'sub') RETURNING id`, tenant, "ajeno-"+tenant, draft).Scan(&otroSitio)
		}); err != nil {
			t.Fatal(err)
		}
		ajena, err := st.CrearVersionConTipo(org, otroSitio, "", "preview", draft, 1, "sub")
		if err != nil {
			t.Fatal(err)
		}
		versionForzada = ajena
		antes := almacen.Escrituras()
		if rec := subir("preview", token, preview, []string{"admin"}); rec.Code != 404 || almacen.Escrituras() != antes {
			t.Fatalf("versión ajena tenant=%s status=%d escrituras=%d antes=%d", tenant, rec.Code, almacen.Escrituras(), antes)
		}
	}
	versionForzada = uuid.New()
	antes := almacen.Escrituras()
	if rec := subir("preview", token, preview, []string{"admin"}); rec.Code != 404 || almacen.Escrituras() != antes {
		t.Fatal("versión inexistente llega al almacén")
	}
	versionForzada = uuid.Nil
	live := render("", false)
	if rec := subir("live", "", live, []string{"member"}); rec.Code != 403 {
		t.Fatalf("member=%d", rec.Code)
	}
	face := datos.Fonts["work-sans"].Faces[0]
	clave, _ := cat.ClaveRecurso(face.File)
	almacen.Borrar(clave)
	if rec := subir("live", "", live, []string{"admin"}); rec.Code != 422 {
		t.Fatalf("fuente inexistente=%d: %s", rec.Code, rec.Body.String())
	}
	if err := cat.Materializar(ctx, almacen.Objetos); err != nil {
		t.Fatal(err)
	}
	almacen.Fallar(true)
	if rec := subir("live", "", live, []string{"admin"}); rec.Code != 503 {
		t.Fatalf("S3 desconectado=%d: %s", rec.Code, rec.Body.String())
	}
	almacen.Fallar(false)
	almacen.FallarPut("shared/sitios/" + id.String() + "/v/")
	if rec := subir("live", "", live, []string{"admin"}); rec.Code != 503 {
		t.Fatalf("PUT caído=%d: %s", rec.Code, rec.Body.String())
	}
	site, err = st.SitioCompleto(ctx, id)
	if err != nil || site.PublishedNumber != nil {
		t.Fatal("publicación sin bytes confirmó el puntero público")
	}
	almacen.FallarPut("")
	if rec := subir("live", "", live, []string{"admin"}); rec.Code != 200 {
		t.Fatalf("live=%d: %s", rec.Code, rec.Body.String())
	}
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatal(err)
	}
	pedir("/es/", 200)
	for _, f := range live {
		if strings.HasSuffix(f.Ruta, ".css") {
			pedir("/"+f.Ruta, 200)
		}
	}
	for _, face := range datos.Fonts["work-sans"].Faces {
		pedir("/_astro/"+face.File, 200)
	}
	pedir("/_astro/"+foto.Fichero, 200)
	for _, nombre := range nombres {
		pedir("/_astro/"+nombre, 200)
	}
	var rutaCSS string
	for _, f := range live {
		if strings.HasSuffix(f.Ruta, ".css") {
			rutaCSS = f.Ruta
		}
	}
	claveCSS := "shared/sitios/" + id.String() + "/recursos/" + strings.TrimPrefix(rutaCSS, "_astro/")
	original := almacen.Leer(claveCSS)
	token2 := "abcdef0123456789abcdef0123456789"
	falsificada := render("/_vista/"+token2, true)
	var rutaOriginal string
	for _, f := range falsificada {
		if strings.HasSuffix(f.Ruta, ".css") {
			rutaOriginal = f.Ruta
		}
	}
	for i := range falsificada {
		if strings.HasSuffix(falsificada[i].Ruta, ".css") {
			falsificada[i].Ruta = rutaCSS
		} else if strings.HasSuffix(falsificada[i].Ruta, ".html") {
			falsificada[i].Contenido = bytes.ReplaceAll(falsificada[i].Contenido, []byte(rutaOriginal), []byte(rutaCSS))
		}
	}
	if err := h.validarPublicacionSalida(ctx, site, "preview", token2, falsificada); err == nil || !strings.Contains(err.Error(), "CSS no coincide") {
		t.Fatalf("CSS con nombre falsificado: %v", err)
	}
	antes = almacen.Escrituras()
	if rec := subir("preview", token2, falsificada, []string{"admin"}); rec.Code != 422 || almacen.Escrituras() != antes || !bytes.Equal(original, almacen.Leer(claveCSS)) {
		t.Fatal("preview sobrescribe CSS del live")
	}
	pedir("/"+rutaCSS, 200)
	almacen.FallarPut(mapa.ClaveHostsJSON)
	nuevaPreview := render("/_vista/"+token2, true)
	if rec := subir("preview", token2, nuevaPreview, []string{"admin"}); rec.Code != 503 {
		t.Fatalf("mapa caído=%d: %s", rec.Code, rec.Body.String())
	}
	almacen.FallarPut("")
	if rec := subir("preview", token2, nuevaPreview, []string{"admin"}); rec.Code != 200 {
		t.Fatalf("reintento mapa=%d: %s", rec.Code, rec.Body.String())
	}
	almacen.Fallar(true)
	for _, ruta := range []string{"/sites/" + id.String() + "/files/" + nombres[0], "/catalog/files/" + face.File} {
		req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 503 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("S3 caído GET %s=%d cache=%q", ruta, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/_astro/"+face.File, http.NoBody)
	req.Host = "primero.sites.example.test"
	rec := httptest.NewRecorder()
	publico.ServeHTTP(rec, req)
	if rec.Code != 503 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("servidor público cachea caída como404")
	}
	almacen.Fallar(false)
}
