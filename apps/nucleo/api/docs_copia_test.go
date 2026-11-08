// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateCopyHablaElCableDelServicio: «Hacer una copia» es POST /documents con
// `copy_of` (services/docs/internal/http/creacion.go), el tipo del original y el título.
//
// VISTO EN ROJO (2026-11-05): con la etiqueta `json:"copyOf"`, el servicio recibía un
// campo que no conoce, creaba un documento en blanco y aquí se veía en el cuerpo.
func TestCreateCopyHablaElCableDelServicio(t *testing.T) {
	var cuerpo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/documents" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		cuerpo = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"n1","title":"Copia de Cuentas","kind":"spreadsheet"},"role":"owner"}`)
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, nil)
	doc, err := d.CreateCopy(context.Background(), Document{ID: "d1", Kind: DocSheet, Title: "Cuentas"}, "Copia de Cuentas")
	if err != nil || doc.ID != "n1" || doc.Kind != DocSheet {
		t.Fatalf("copia: %+v, %v", doc, err)
	}
	if cuerpo != `{"kind":"spreadsheet","title":"Copia de Cuentas","copy_of":"d1"}` {
		t.Errorf("cuerpo: %s", cuerpo)
	}
}
