// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Lo que hace falta para mover entre unidades y para los favoritos de la nube: la huella
// del contenido que da cada proveedor (para comprobar una copia sin volver a bajarla) y un
// identificador que no cambia al renombrar.
func TestHuellaEIdentificadorEstable(t *testing.T) {
	token := func(context.Context) (string, error) { return "tk", nil }

	// Google: md5Checksum y el ID.
	g := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("fields"), "md5Checksum") {
			t.Error("Google: no pide md5Checksum", r.URL.Query().Get("fields"))
		}
		_, _ = io.WriteString(w, `{"files":[{"id":"f1","name":"a.pdf","mimeType":"application/pdf","size":"3","md5Checksum":"ABCDEF0123"},
{"id":"d1","name":"Notas","mimeType":"application/vnd.google-apps.document"}]}`)
	}))
	defer g.Close()
	es, err := (&GoogleDrive{Token: token, HTTP: g.Client(), API: g.URL}).Listar(t.Context(), "")
	if err != nil || len(es) != 2 {
		t.Fatal(es, err)
	}
	for _, e := range es {
		switch e.ID {
		case "f1":
			if e.Huella != "md5:abcdef0123" || e.Estable != "f1" {
				t.Fatalf("Google: %+v", e)
			}
		case "d1":
			if e.Huella != "" || e.Estable != "d1" {
				t.Fatalf("Google, documento sin bytes: %+v", e)
			}
		}
	}

	// OneDrive: sha1Hash (personal) y el ID; el de empresa solo da quickXorHash.
	o := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"value":[{"id":"p1","name":"a.pdf","size":3,"file":{"mimeType":"application/pdf","hashes":{"sha1Hash":"A9993E36"}}},
{"id":"e1","name":"b.pdf","size":3,"file":{"mimeType":"application/pdf","hashes":{"quickXorHash":"AAAA"}}}]}`)
	}))
	defer o.Close()
	es, err = (&OneDriveGraph{Token: token, HTTP: o.Client(), API: o.URL}).Listar(t.Context(), "")
	if err != nil || len(es) != 2 || es[0].Huella != "sha1:a9993e36" || es[0].Estable != "p1" || es[1].Huella != "" || es[1].Estable != "e1" {
		t.Fatalf("OneDrive: %+v %v", es, err)
	}

	// WebDAV: el `oc:fileid` de Nextcloud si lo da; si no (un 404 aparte), sin estable.
	d := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(cuerpo), "oc:fileid") {
			t.Error("WebDAV: no pide oc:fileid")
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
<d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/a.txt</d:href><d:propstat><d:prop><d:getcontentlength>3</d:getcontentlength><oc:fileid>4711</oc:fileid><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
<d:response><d:href>/dav/b.txt</d:href><d:propstat><d:prop><d:getcontentlength>3</d:getcontentlength><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>
<d:propstat><d:prop><oc:fileid/></d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat></d:response>
</d:multistatus>`)
	}))
	defer d.Close()
	es, err = (&DAV{Base: d.URL + "/dav", HTTP: d.Client()}).Listar(t.Context(), "")
	if err != nil || len(es) != 2 || es[0].Estable != "oc:4711" || es[1].Estable != "" || es[1].Tamano != 3 {
		t.Fatalf("WebDAV: %+v %v", es, err)
	}
}
