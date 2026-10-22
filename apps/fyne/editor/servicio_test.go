// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La plantilla pasa por el mismo guardado previo y la misma carpeta de descargas.
// El servidor espía el protocolo; el fichero comprueba extensión y contenido reales.
func TestServicioDescargaPlantillaDespuesDeGuardar(t *testing.T) {
	var guardado atomic.Bool
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		if !guardado.Load() {
			t.Error("se pidió la plantilla antes de guardar")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/documents/d1/exports":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"format":"ooxml","template":true}` {
				t.Errorf("descarga plantilla: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"export":{"id":"p1"}}`)
		case "GET /api/v1/documents/d1/exports/p1":
			if r.URL.Query().Get("inline") != "true" {
				t.Error("la descarga no es inline")
			}
			_, _ = io.WriteString(w, "PK!plantilla")
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := Servicio{Svc: api.NewDocs(srv.URL, nil, srv.Client()), Lanzar: func(f func()) { f() }, Carpeta: func() (string, error) { return dir, nil }}
	s.DescargarPlantilla("d1", "Acta: junio", Documento, "ooxml", func() error { guardado.Store(true); return nil }, nil)
	datos, err := os.ReadFile(filepath.Join(dir, "Acta_ junio.dotx"))
	if err != nil || string(datos) != "PK!plantilla" || peticiones.Load() != 2 {
		t.Fatalf("descarga %q, peticiones %d: %v", datos, peticiones.Load(), err)
	}
	// Un guardado fallido cancela tanto la exportación como la descarga.
	s.DescargarPlantilla("d1", "Otra", Documento, "ooxml", func() error { return errors.New("no guardado") }, nil)
	if peticiones.Load() != 2 {
		t.Error("se exportó una plantilla cuyo guardado falló")
	}
}
