// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestWebDAVGestionaCarpetasYDaETag(t *testing.T) {
	var mu sync.Mutex
	var hechos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hechos = append(hechos, r.Method+" "+r.URL.Path+" "+r.Header.Get("Destination")+" "+r.Header.Get("Overwrite"))
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">
<d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/a.txt</d:href><d:propstat><d:prop><d:getetag>"v7"</d:getetag><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`)
		case "MKCOL", "MOVE":
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	d := &DAV{Base: srv.URL + "/dav", Usuario: "u", Clave: "c", HTTP: srv.Client()}
	ctx := t.Context()
	es, err := d.Listar(ctx, "")
	if err != nil || len(es) != 1 || es[0].Version != `"v7"` {
		t.Fatalf("%+v %v", es, err)
	}
	c, err := d.CrearCarpeta(ctx, "", "Informes")
	if err != nil || !c.Carpeta || c.ID != "/Informes" {
		t.Fatal(c, err)
	}
	r, err := d.Renombrar(ctx, es[0], "b.txt")
	if err != nil || r.ID != "/b.txt" {
		t.Fatal(r, err)
	}
	if _, err := d.Mover(ctx, r, "/Informes"); err != nil {
		t.Fatal(err)
	}
	if err := d.Borrar(ctx, c); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	quiero := []string{"MKCOL /dav/Informes/", "MOVE /dav/a.txt " + srv.URL + "/dav/b.txt F", "MOVE /dav/b.txt " + srv.URL + "/dav/Informes/b.txt F", "DELETE /dav/Informes/"}
	todo := strings.Join(hechos, "\n")
	for _, q := range quiero {
		if !strings.Contains(todo, q) {
			t.Fatalf("falta %q en:\n%s", q, todo)
		}
	}
}

func TestOneDriveGestionaYBorraALaPapelera(t *testing.T) {
	var hechos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		raw, _ := json.Marshal(cuerpo)
		hechos = append(hechos, r.Method+" "+r.URL.Path+" "+string(raw))
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, `{"id":"n1","name":"X","eTag":"e1","folder":{}}`)
	}))
	defer srv.Close()
	o := &OneDriveGraph{Token: token, HTTP: srv.Client(), API: srv.URL + "/v1.0"}
	ctx := t.Context()
	c, err := o.CrearCarpeta(ctx, "", "X")
	if err != nil || c.ID != "n1" || c.Version != "e1" || !c.Carpeta {
		t.Fatal(c, err)
	}
	if _, err := o.Renombrar(ctx, c, "Y"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Mover(ctx, c, "p2"); err != nil {
		t.Fatal(err)
	}
	if err := o.Borrar(ctx, c); err != nil {
		t.Fatal(err)
	}
	todo := strings.Join(hechos, "\n")
	for _, q := range []string{"POST /v1.0/me/drive/root/children", `"conflictBehavior":"fail"`, `PATCH /v1.0/me/drive/items/n1 {"name":"Y"}`, `"parentReference":{"id":"p2"}`, "DELETE /v1.0/me/drive/items/n1"} {
		if !strings.Contains(strings.ReplaceAll(todo, "@microsoft.graph.", ""), q) {
			t.Fatalf("falta %q en:\n%s", q, todo)
		}
	}
}

func TestGoogleMueveQuitandoLosPadresYBorraALaPapelera(t *testing.T) {
	var hechos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hechos = append(hechos, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(b))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"parents":["viejo"]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"f1","name":"X","mimeType":"application/vnd.google-apps.folder","version":"9"}`)
	}))
	defer srv.Close()
	g := &GoogleDrive{Token: token, HTTP: srv.Client(), API: srv.URL + "/drive/v3"}
	ctx := t.Context()
	c, err := g.CrearCarpeta(ctx, "", "X")
	if err != nil || !c.Carpeta || c.Version != "9" {
		t.Fatal(c, err)
	}
	if _, err := g.Mover(ctx, Elemento{ID: "f2"}, "nuevo"); err != nil {
		t.Fatal(err)
	}
	if err := g.Borrar(ctx, Elemento{ID: "f2"}); err != nil {
		t.Fatal(err)
	}
	todo := strings.Join(hechos, "\n")
	for _, q := range []string{`"parents":["root"]`, "addParents=nuevo", "removeParents=viejo", `{"trashed":true}`} {
		if !strings.Contains(todo, q) {
			t.Fatalf("falta %q en:\n%s", q, todo)
		}
	}
}
