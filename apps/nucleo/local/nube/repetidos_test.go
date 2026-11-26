// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Google Drive admite dos ficheros con el mismo nombre en una carpeta, y los lista por su
// identificador: Listar pone primero el creado antes (siempre el mismo, también para Subir
// por el nombre), y Reemplazar sube encima del que se le da, por su id.
func TestGoogleConDosQueSeLlamanIgual(t *testing.T) {
	var mu sync.Mutex
	var hechos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hechos = append(hechos, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
			if !strings.Contains(r.URL.Query().Get("fields"), "createdTime") {
				t.Error("sin createdTime", r.URL.Query().Get("fields"))
			}
			_, _ = io.WriteString(w, `{"files":[`+
				`{"id":"a","name":"Acta.docx","createdTime":"2026-10-01T09:00:00Z","version":"4"},`+
				`{"id":"b","name":"Acta.docx","createdTime":"2026-09-01T09:00:00Z","version":"7"}]}`)
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/"):
			id := strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files/")
			_, _ = io.WriteString(w, `{"id":"`+id+`","name":"Acta.docx","version":"9"}`)
		default:
			t.Error("ruta", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	g := &GoogleDrive{Token: token, HTTP: srv.Client(), API: srv.URL + "/drive/v3", Subidas: srv.URL + "/upload/drive/v3"}
	ctx := t.Context()
	es, err := g.Listar(ctx, "inf")
	if err != nil || len(es) != 2 || es[0].ID != "b" || es[1].ID != "a" || es[0].Creado.IsZero() {
		t.Fatalf("el creado antes, primero: %+v %v", es, err)
	}
	if e, err := g.Subir(ctx, "inf", "Acta.docx", []byte("x")); err != nil || e.ID != "b" {
		t.Fatal("Subir por el nombre sustituye el primero", e, err)
	}
	if e, err := g.Reemplazar(ctx, es[1], []byte("y")); err != nil || e.ID != "a" || e.Version != "9" {
		t.Fatal("Reemplazar, el que es", e, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(hechos, "\n"), "PATCH /upload/drive/v3/files/a?uploadType=media") {
		t.Fatal(hechos)
	}
}

// WebDAV no tiene dos recursos en la misma ruta, pero el `displayname` que da el servidor
// es libre: dos pueden verse con el mismo nombre, y ese nombre no es por fuerza el de su
// ruta. Reemplazar sube a la ruta (el id), no a «carpeta/nombre».
func TestWebDAVConDosQueSeVenIgual(t *testing.T) {
	var mu sync.Mutex
	puestos := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">
<d:response><d:href>/dav/Actas/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/Actas/acta-1.docx</d:href><d:propstat><d:prop><d:displayname>Acta.docx</d:displayname><d:creationdate>2026-10-01T09:00:00Z</d:creationdate><d:getetag>"1"</d:getetag><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/Actas/acta-2.docx</d:href><d:propstat><d:prop><d:displayname>Acta.docx</d:displayname><d:creationdate>2026-09-01T09:00:00Z</d:creationdate><d:getetag>"2"</d:getetag><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`)
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			puestos[r.URL.Path] = string(b)
			w.Header().Set("ETag", `"3"`)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	d := &DAV{Base: srv.URL + "/dav", Usuario: "u", Clave: "c", HTTP: srv.Client()}
	es, err := d.Listar(t.Context(), "/Actas")
	if err != nil || len(es) != 2 || es[0].Nombre != "Acta.docx" || es[1].Nombre != "Acta.docx" || es[0].ID != "/Actas/acta-2.docx" {
		t.Fatalf("los dos, el creado antes primero: %+v %v", es, err)
	}
	if e, err := d.Reemplazar(t.Context(), es[1], []byte("nuevo")); err != nil || e.ID != "/Actas/acta-1.docx" || e.Version != `"3"` {
		t.Fatal(e, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(puestos) != 1 || puestos["/dav/Actas/acta-1.docx"] != "nuevo" {
		t.Fatal("subió a otra ruta", puestos)
	}
}

// OneDrive no admite dos con el mismo nombre en una carpeta; Reemplazar sube igual por el
// id del elemento.
func TestOneDriveReemplazaPorSuID(t *testing.T) {
	var ruta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta = r.Method + " " + r.URL.Path
		_, _ = io.WriteString(w, `{"id":"i7","name":"Acta.docx","eTag":"e2","createdDateTime":"2026-09-01T09:00:00Z","file":{}}`)
	}))
	defer srv.Close()
	o := &OneDriveGraph{Token: token, HTTP: srv.Client(), API: srv.URL + "/v1.0"}
	e, err := o.Reemplazar(t.Context(), Elemento{ID: "i7", Nombre: "Acta.docx"}, []byte("x"))
	if err != nil || e.ID != "i7" || e.Version != "e2" || e.Creado.IsZero() || ruta != "PUT /v1.0/me/drive/items/i7/content" {
		t.Fatal(e, err, ruta)
	}
}
