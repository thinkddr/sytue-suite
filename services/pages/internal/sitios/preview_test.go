// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sitios

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
)

// El mapa y el cliente S3 reales sirven un sitio sin publicación live; el token
// autoriza tanto HTML como recursos, sin abrir la raíz ni esquivar suspensión.
func TestVistaPreviaAntesDePublicar(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	digest := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(digest[:16])
	doc := mapa.HostsMapa{Version: 1, Sites: map[string]mapa.SiteMapa{}, Hosts: map[string]string{}}
	for _, nombre := range []string{"nuevo", "otro", "caducado", "suspendido", "retirado"} {
		site := mapa.SiteMapa{Status: "unpublished", Languages: []string{"es", "en"}, DefaultLanguage: "es"}
		if nombre != "otro" && nombre != "retirado" {
			expires := time.Now().Add(time.Hour)
			if nombre == "caducado" {
				expires = time.Now().Add(-time.Hour)
			}
			site.Previews = map[string]string{key: expires.UTC().Format(time.RFC3339)}
		}
		if nombre == "suspendido" {
			site.Status = "suspended"
		}
		doc.Sites[nombre] = site
		doc.Hosts[nombre+".sites.example.test"] = nombre
	}
	mapaJSON, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"/pages/" + mapa.ClaveHostsJSON:                               string(mapaJSON),
		"/pages/shared/sitios/nuevo/vistas/" + key + "/es/index.html": "<h1>Preview privada</h1>",
		"/pages/shared/sitios/nuevo/recursos/estilos.css":             "h1{color:blue}",
		"/pages/shared/sitios/nuevo/recursos/fuente.woff2":            "fontbytes",
		"/pages/shared/sitios/nuevo/recursos/foto.png":                "pngbytes",
		"/pages/shared/sitios/nuevo/recursos/documento.pdf":           "%PDF-1.4",
	}
	var lecturas atomic.Int64
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lecturas.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(body))
		}
	}))
	defer s3.Close()
	objects, err := objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(s3.URL, "http://"), Region: "eu-local-1", AccessKey: "test", SecretKey: "test", Bucket: "pages"})
	if err != nil {
		t.Fatal(err)
	}
	lector := mapa.NuevoLector(objects, time.Minute)
	if err := lector.Refrescar(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := NuevoRouter("test", config.Sitios{}, lector, objects)
	for _, tc := range []struct {
		name, host, path, method string
		status                   int
		contenido                string
	}{
		{"raiz preview al idioma", "nuevo", "/_vista/" + token + "/", "GET", 302, ""},
		{"HTML GET", "nuevo", "/_vista/" + token + "/es/", "GET", 200, "Preview privada"},
		{"HTML HEAD", "nuevo", "/_vista/" + token + "/es/", "HEAD", 200, ""},
		{"CSS GET", "nuevo", "/_vista/" + token + "/_astro/estilos.css", "GET", 200, "h1{color:blue}"},
		{"CSS HEAD", "nuevo", "/_vista/" + token + "/_astro/estilos.css", "HEAD", 200, ""},
		{"fuente", "nuevo", "/_vista/" + token + "/_astro/fuente.woff2", "GET", 200, "fontbytes"},
		{"imagen", "nuevo", "/_vista/" + token + "/_astro/foto.png", "GET", 200, "pngbytes"},
		{"PDF", "nuevo", "/_vista/" + token + "/_astro/documento.pdf", "GET", 200, "%PDF-1.4"},
		{"raiz no publica", "nuevo", "/", "GET", 404, ""},
		{"CSS sin token", "nuevo", "/_astro/estilos.css", "GET", 404, ""},
		{"token ausente", "nuevo", "/_vista/", "GET", 404, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
			req.Host = tc.host + ".sites.example.test"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d; %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.contenido != "" && !strings.Contains(rec.Body.String(), tc.contenido) {
				t.Fatalf("contenido=%s", rec.Body.String())
			}
			if strings.HasSuffix(tc.path, ".pdf") && (rec.Header().Get("Content-Disposition") != "attachment" || rec.Header().Get("Content-Security-Policy") != "sandbox") {
				t.Fatal("PDF sin descarga segura")
			}
			if tc.status == 302 && rec.Header().Get("Location") != "/_vista/"+token+"/es/" {
				t.Fatalf("Location=%q", rec.Header().Get("Location"))
			}
			if tc.status == 200 && (rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Robots-Tag") != "noindex") {
				t.Fatalf("preview cacheable/indexable: %v", rec.Header())
			}
		})
	}
	for _, host := range []string{"nuevo", "otro", "caducado", "suspendido", "retirado"} {
		for _, recurso := range []string{"es/", "_astro/estilos.css", "_astro/fuente.woff2", "_astro/foto.png", "_astro/documento.pdf"} {
			testToken := token
			if host == "nuevo" {
				testToken = "tokeninvalido"
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				t.Run("denegado/"+host+"/"+recurso+"/"+method, func(t *testing.T) {
					before := lecturas.Load()
					req := httptest.NewRequest(method, "/_vista/"+testToken+"/"+recurso, http.NoBody)
					req.Host = host + ".sites.example.test"
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, req)
					want := http.StatusNotFound
					if host == "suspendido" {
						want = http.StatusServiceUnavailable
					}
					if rec.Code != want {
						t.Fatalf("status=%d want %d", rec.Code, want)
					}
					if lecturas.Load() != before {
						t.Fatal("petición denegada llegó a leer S3")
					}
				})
			}
		}
	}
}
