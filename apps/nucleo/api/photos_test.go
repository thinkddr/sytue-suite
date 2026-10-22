// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestPhotos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/timeline":
			if r.URL.RawQuery != "cursor=c1&favoritas=1&limit=60&q=playa" {
				t.Errorf("consulta de la línea de tiempo: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"f1","name":"playa.jpg","width":4000,"height":3000,"takenAt":"2026-08-02T17:30:00Z","takenAtFromExif":true,"favorite":true}],"nextCursor":"c2"}`)
		case "GET /api/v1/assets/f1/variants/miniatura":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = io.WriteString(w, "JPEG")
		case "PUT /api/v1/assets/f1/favorite":
			if cuerpo, _ := io.ReadAll(r.Body); string(cuerpo) != `{"favorite":false}` {
				t.Errorf("cuerpo de favorita: %s", cuerpo)
			}
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/v1/assets/f1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, ctx := NewPhotos(srv.URL, nil, nil), context.Background()
	fotos, siguiente, err := c.Timeline(ctx, "c1", 60, true, "playa")
	if err != nil || siguiente != "c2" || len(fotos) != 1 || !fotos[0].Favorite || fotos[0].Width != 4000 || !fotos[0].TakenFromEXIF || fotos[0].TakenAt.IsZero() {
		t.Fatalf("línea de tiempo: %+v, %q, %v", fotos, siguiente, err)
	}
	imagen, err := c.Variant(ctx, "f1", PhotoThumb)
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(imagen)
	_ = imagen.Close()
	if string(datos) != "JPEG" {
		t.Errorf("variante: %q", datos)
	}
	if err := c.SetFavorite(ctx, "f1", false); err != nil {
		t.Error(err)
	}
	if err := c.Trash(ctx, "f1"); err != nil {
		t.Error(err)
	}
}

func TestLosCamposDePhotosSonLosDelServicio(t *testing.T) {
	manejadores := webref.Read(t, "services/syphotos/internal/http/handlers.go")
	for _, literal := range []string{
		`json:"contentType"`, `json:"sizeBytes"`, `json:"takenAt"`, `json:"takenAtFromExif"`, `json:"favorite"`, `json:"width"`, `json:"camera,omitempty"`,
		`"items": salida`, `cuerpo["nextCursor"]`, `params.Get("favoritas") == "1"`, `params.Get("cursor")`, `params.Get("q")`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/syphotos/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/syphotos/internal/http/server.go")
	for _, ruta := range []string{`"/timeline"`, `"/assets/{id}/variants/{variante}"`, `api.Put("/assets/{id}/favorite"`, `api.Delete("/assets/{id}"`, `api.Post("/assets", h.Indexar)`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/syphotos ya no monta %s", ruta)
		}
	}
	// `driveFileId` es lo que pide POST /assets, que es el segundo paso de subir.
	if derivadas := webref.Read(t, "services/syphotos/internal/http/derivadas.go"); !strings.Contains(derivadas, `json:"driveFileId"`) {
		t.Error("services/syphotos ya no indexa por driveFileId")
	}
	variantes := webref.Read(t, "services/syphotos/internal/domain/derivada.go")
	for _, v := range []string{`VarianteMiniatura Variante = "` + PhotoThumb + `"`, `VariantePrevia Variante = "` + PhotoPreview + `"`} {
		if !strings.Contains(variantes, v) {
			t.Errorf("services/syphotos ya no declara %s", v)
		}
	}
}

// TestLoQueLaWebPideASyphotos recorre las llamadas que la web hace desde photosApi.ts y
// que el visor, los álbumes, la papelera y «parecidas» necesitan: ruta, método y cuerpo.
func TestLoQueLaWebPideASyphotos(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/assets/f1":
			_, _ = io.WriteString(w, `{"id":"f1","name":"cala.jpg","tags":["mar","barca"],"location":{"lat":42.28871,"lon":3.27794},"driveFileId":"d1"}`)
		case "GET /api/v1/assets/f1/recipe":
			_, _ = io.WriteString(w, `{"recipe":{"version":1,"pasos":[{"tipo":"exposicion","params":{"ev":0.5}}]},"fingerprint":"h0"}`)
		case "PUT /api/v1/assets/f1/recipe":
			_, _ = io.WriteString(w, `{"fingerprint":"h1"}`)
		case "GET /api/v1/assets/f1/similar", "GET /api/v1/similar", "GET /api/v1/assets/f1/nearby":
			_, _ = io.WriteString(w, `{"items":[{"id":"f2","name":"otra.jpg"}]}`)
		case "GET /api/v1/themes":
			_, _ = io.WriteString(w, `{"themes":[{"items":[{"id":"f2"},{"id":"f3"}]}]}`)
		case "GET /api/v1/memories":
			_, _ = io.WriteString(w, `{"memories":[{"year":2024,"items":[{"id":"f4"}]}]}`)
		case "GET /api/v1/albums/a1":
			_, _ = io.WriteString(w, `{"id":"a1","title":"Cadaqués","coverAssetId":null,"count":3,"shared":true}`)
		case "POST /api/v1/albums/a1/assets":
			_, _ = io.WriteString(w, `{"added":2}`)
		case "POST /api/v1/albums/a1/share":
			_, _ = io.WriteString(w, `{"token":"tk2"}`)
		case "GET /api/v1/assets/f1/export", "GET /api/v1/assets/f1/variants/grande":
			_, _ = io.WriteString(w, "IMG")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, ctx := NewPhotos(srv.URL, nil, nil), context.Background()

	f, err := c.Photo(ctx, "f1")
	if err != nil || f.Location == nil || f.Location.Lat != 42.28871 || len(f.Tags) != 2 || f.DriveFileID != "d1" {
		t.Fatalf("foto: %+v %v", f, err)
	}
	if huella, err := c.Rotar(ctx, "f1"); err != nil || huella != "h1" {
		t.Fatalf("girar: %q %v", huella, err)
	}
	if l, err := c.Similar(ctx, "f1", 24); err != nil || len(l) != 1 {
		t.Fatalf("parecidas: %v %v", l, err)
	}
	ids := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"}
	if _, err := c.SimilarMany(ctx, ids, 24); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Nearby(ctx, "f1", 48); err != nil {
		t.Fatal(err)
	}
	if m, err := c.Themes(ctx); err != nil || len(m) != 1 || len(m[0]) != 2 {
		t.Fatalf("temas: %v %v", m, err)
	}
	if r, err := c.Memories(ctx); err != nil || len(r) != 1 || r[0].Year != 2024 {
		t.Fatalf("recuerdos: %v %v", r, err)
	}
	if a, err := c.Album(ctx, "a1"); err != nil || !a.Shared || a.CoverAssetID != "" {
		t.Fatalf("álbum: %+v %v", a, err)
	}
	if n, err := c.AddToAlbum(ctx, "a1", []string{"f1", "f2"}); err != nil || n != 2 {
		t.Fatalf("añadir: %d %v", n, err)
	}
	if tk, err := c.ShareAlbum(ctx, "a1"); err != nil || tk != "tk2" {
		t.Fatalf("compartir: %q %v", tk, err)
	}
	for _, err := range []error{
		c.RemoveFromAlbum(ctx, "a1", "f1"), c.UnshareAlbum(ctx, "a1"), c.DeleteAlbum(ctx, "a1"),
		c.SetDescription(ctx, "f1", "Una cala", nil),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, pedir := range []func() (io.ReadCloser, error){
		func() (io.ReadCloser, error) { return c.Export(ctx, "f1", "png") },
		func() (io.ReadCloser, error) { return c.VariantDe(ctx, "f1", PhotoLarge, "h1") },
	} {
		r, err := pedir()
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
	}
	if _, _, err := c.Timeline(ctx, "", 100, false, "playa", "arena", "costa"); err != nil {
		t.Fatal(err)
	}

	quiero := []string{
		"GET /api/v1/assets/f1? ",
		"GET /api/v1/assets/f1/recipe? ",
		`PUT /api/v1/assets/f1/recipe? {"recipe":{"version":1,"pasos":[{"tipo":"exposicion","params":{"ev":0.5}},{"tipo":"giro","params":{"grados":90}}]}}`,
		"GET /api/v1/assets/f1/similar?limit=24 ",
		"GET /api/v1/similar?ids=1%2C2%2C3%2C4%2C5%2C6%2C7%2C8%2C9%2C10%2C11%2C12&limit=24 ",
		"GET /api/v1/assets/f1/nearby?limit=48 ",
		"GET /api/v1/themes? ",
		"GET /api/v1/memories? ",
		"GET /api/v1/albums/a1? ",
		`POST /api/v1/albums/a1/assets? {"ids":["f1","f2"]}`,
		"POST /api/v1/albums/a1/share? ",
		"DELETE /api/v1/albums/a1/assets/f1? ",
		"DELETE /api/v1/albums/a1/share? ",
		"DELETE /api/v1/albums/a1? ",
		`PUT /api/v1/assets/f1/description? {"description":"Una cala","tags":[]}`,
		"GET /api/v1/assets/f1/export?formato=png ",
		"GET /api/v1/assets/f1/variants/grande?f=h1 ",
		"GET /api/v1/timeline?limit=100&q=playa&tambien=arena%2Ccosta ",
	}
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\n\nquería:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestLasRutasNuevasSonLasDelServicio ata cada llamada de arriba a la ruta que monta
// services/syphotos: si el servicio la renombra, esto se pone rojo antes que la app.
func TestLasRutasNuevasSonLasDelServicio(t *testing.T) {
	rutas := webref.Read(t, "services/syphotos/internal/http/server.go")
	for _, ruta := range []string{
		`api.Get("/assets/{id}", h.Activo)`, `api.Put("/assets/{id}/description"`,
		`api.Get("/assets/{id}/similar"`, `api.Get("/assets/{id}/nearby"`, `"/similar", h.ParecidasAVarias`,
		`api.Get("/themes"`, `api.Get("/memories"`, `api.Get("/assets/{id}/recipe"`, `api.Put("/assets/{id}/recipe"`,
		`api.Get("/assets/{id}/export"`, `api.Get("/albums/{album}", h.Album)`, `api.Delete("/albums/{album}", h.BorrarAlbum)`,
		`api.Post("/albums/{album}/assets"`, `api.Delete("/albums/{album}/assets/{id}"`,
		`api.Post("/albums/{album}/share"`, `api.Delete("/albums/{album}/share"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/syphotos ya no monta %s", ruta)
		}
	}
	manejadores := webref.Read(t, "services/syphotos/internal/http/handlers.go")
	for _, literal := range []string{
		`json:"location,omitempty"`, `json:"tags,omitempty"`, `json:"driveFileId"`, `json:"lat"`,
		`params.Get("tambien")`, `params.Get("ids")`, `"themes": salida`, `"memories": salida`,
		`"year": rec.Anyo`, `"fingerprint": huella`, `json:"recipe"`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/syphotos/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	for fichero, literales := range map[string][]string{
		"services/syphotos/internal/http/albums.go":     {`map[string]int{"added": n}`, `json:"ids"`},
		"services/syphotos/internal/http/compartir.go":  {`map[string]string{"token": token}`},
		"services/syphotos/internal/http/derivadas.go":  {`Query().Get("formato")`, `Query().Get("f")`},
		"services/syphotos/internal/domain/receta.go":   {`Pasos   []Paso `, `json:"params,omitempty"`, `TipoPaso = "giro"`},
		"services/syphotos/internal/domain/derivada.go": {`VarianteGrande Variante = "` + PhotoLarge + `"`},
	} {
		texto := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(texto, l) {
				t.Errorf("%s ya no dice %s", fichero, l)
			}
		}
	}
}

// TestElAlbumPublicoVaSinSesion: el álbum compartido y sus imágenes se piden por
// `/api/v1/public/…` —lo que sale por el ingress— y SIN el token de quien mira, aunque
// el cliente tenga sesión.
func TestElAlbumPublicoVaSinSesion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "" {
			t.Errorf("%s llevó %q", r.URL.Path, a)
		}
		switch r.URL.EscapedPath() {
		case "/api/v1/public/albums/tok%2Fen":
			_, _ = io.WriteString(w, `{"album":{"title":"Verano","count":1},"items":[{"id":"f1","name":"cala.jpg","width":4000,"height":3000,"takenAt":"2026-08-02T17:30:00Z"}]}`)
		case "/api/v1/public/albums/tok%2Fen/assets/f1/variants/miniatura":
			_, _ = io.WriteString(w, "JPEG")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, ctx := NewPhotos(srv.URL, &sesionFalsa{tokens: []string{"de-quien-mira"}}, nil), context.Background()
	al, err := c.PublicAlbum(ctx, "tok/en")
	if err != nil || al.Title != "Verano" || al.Count != 1 || len(al.Items) != 1 || al.Items[0].Width != 4000 {
		t.Fatalf("álbum público: %+v, %v", al, err)
	}
	r, err := c.PublicVariant(ctx, "tok/en", "f1", PhotoThumb)
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(r)
	_ = r.Close()
	if string(datos) != "JPEG" {
		t.Errorf("variante pública: %q", datos)
	}
	if _, err := c.PublicAlbum(ctx, "otro"); !EsNoEncontrado(err) {
		t.Errorf("un token que no existe: %v", err)
	}
}
