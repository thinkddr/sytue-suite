// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// TestAlmacenExterno: cada llamada va a su ruta del servicio, guardar devuelve el motivo
// del servicio en el error, y el NAS manda los nombres de campo del túnel.
func TestAlmacenExterno(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/storage/external":
			_, _ = io.WriteString(w, `{"connected":true,"available":true,"kind":"s3","bucket":"copias","endpoint":"s3.fr-par.scw.cloud"}`)
		case "PUT /api/v1/storage/external":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":"bad_request","message":"Falta el bucket."}}`)
		case "GET /api/v1/storage/external/instances":
			_, _ = io.WriteString(w, `{"items":[{"id":"i1","template_key":"google-drive","name":"Ventas","connected":true}]}`)
		case "POST /api/v1/storage/external/nas":
			_, _ = io.WriteString(w, `{"endpoint":"https://nas-x.sytue.eu","command":"docker run newt"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	if e, err := d.ExternalStorage(ctx); err != nil || !e.Connected || e.Bucket != "copias" {
		t.Fatalf("ExternalStorage = %+v, %v", e, err)
	}
	c := ConfigAlmacen{Kind: "s3", Endpoint: "e", Bucket: "b", AccessKey: "a", SecretKey: "s", AcceptTerms: true}
	if err := d.SaveExternalStorage(ctx, c); MotivoDe(err) != "Falta el bucket." {
		t.Errorf("el motivo del servicio es %q", MotivoDe(err))
	}
	if cs, err := d.ExternalStorageAccounts(ctx); err != nil || len(cs.Items) != 1 || cs.Items[0].TemplateKey != "google-drive" {
		t.Errorf("instancias = %+v, %v", cs, err)
	}
	nas, err := d.ConnectNAS(ctx, ConfigAlmacen{Endpoint: "http://192.168.1.10:5005", AccessKey: "yo", SecretKey: "x", AcceptTerms: true})
	if err != nil || nas.Command != "docker run newt" {
		t.Errorf("NAS = %+v, %v", nas, err)
	}
	if err := d.DeleteExternalStorage(ctx); err != nil {
		t.Error(err)
	}
	if !strings.Contains(pedidas[3], `"local_endpoint":"http://192.168.1.10:5005"`) || !strings.Contains(pedidas[3], `"user":"yo"`) {
		t.Errorf("el NAS no manda los campos del túnel: %s", pedidas[3])
	}
	if pedidas[4] != "DELETE /api/v1/storage/external" {
		t.Errorf("desconectar = %q", pedidas[4])
	}
}

// TestImportaciones: solo se ofrecen las conexiones ACTIVAS de Google o Microsoft, por la
// clave de su plantilla, y solo se enseñan los trabajos del contenido de esta app.
func TestImportaciones(t *testing.T) {
	var lanzada string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/instances":
			_, _ = io.WriteString(w, `{"instances":[
				{"id":"g","name":"Google de ventas","template_key":"google-workspace-eu","status":"active"},
				{"id":"m","name":"M365","template_key":"microsoft365","status":"active"},
				{"id":"p","name":"Pausada","template_key":"google","status":"paused"},
				{"id":"j","name":"Jira","template_key":"jira","status":"active"}]}`)
		case "GET /api/v1/migrations/self":
			_, _ = io.WriteString(w, `{"migrations":[{"id":"1","domain":"drive","status":"running","counts":{"migrated":3}},{"id":"2","domain":"mail","status":"done"}]}`)
		case "POST /api/v1/migrations/self":
			b, _ := io.ReadAll(r.Body)
			lanzada = string(b)
			_, _ = io.WriteString(w, `{"migrations":[{"id":"3","domain":"drive","status":"queued"}]}`)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	i := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	origenes, err := i.OrigenesImportacion(ctx)
	ids := make([]string, 0, len(origenes))
	for _, o := range origenes {
		ids = append(ids, o.InstanceID+":"+o.Source)
	}
	if err != nil || !slices.Equal(ids, []string{"g:google", "m:m365"}) {
		t.Fatalf("orígenes = %q, %v", ids, err)
	}
	ms, err := i.MisImportaciones(ctx, []string{"drive"})
	if err != nil || len(ms) != 1 || !ms[0].EnCurso() || ms[0].Counts.Migrated != 3 {
		t.Errorf("importaciones = %+v, %v", ms, err)
	}
	nuevas, err := i.Importar(ctx, origenes[0], []string{"drive"})
	if err != nil || len(nuevas) != 1 || lanzada != `{"domains":["drive"],"instance_id":"g","source":"google"}` {
		t.Errorf("importar = %+v %v; cuerpo %s", nuevas, err, lanzada)
	}
}
