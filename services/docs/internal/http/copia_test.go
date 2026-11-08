// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// «Hacer una copia» siembra el documento nuevo con el contenido de AHORA del original,
// en la misma petición: su tipo, su título si no llega otro, y sus operaciones a nombre
// de un actor nuevo.
//
// VISTO EN ROJO (2026-11-05): sin la rama de `copy_of` en semillaDeCreacion, el documento
// nacía vacío y del `kind` del cuerpo, y el test lo decía en «semilla».
func TestHacerUnaCopia(t *testing.T) {
	st, err := domain.StateFromJSON(json.RawMessage(`{"registers":{"doc:sheet":{"value":"Partida","dot":{"actor":"a","counter":1},"lamport":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	original := &domain.Document{ID: "d1", OwnerSub: "owner", Kind: domain.KindSpreadsheet, Title: "Presupuesto"}
	fs := &fakeStore{doc: original, loaded: &domain.LoadedState{State: st}}
	h := newHandler(fs)

	rec := httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil,
		`{"kind":"document","title":"Copia de Presupuesto","copy_of":"d1"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("copiar: %d %s", rec.Code, rec.Body.String())
	}
	p := fs.creado
	if p == nil || p.Kind != domain.KindSpreadsheet || p.Title != "Copia de Presupuesto" {
		t.Fatalf("parámetros de la copia: %+v", p)
	}
	if len(p.Seed) != 1 || p.Seed[0].Key != "doc:sheet" || p.Seed[0].Kind != domain.OpSet {
		t.Fatalf("semilla: %+v", p.Seed)
	}
	if p.Seed[0].Dot.Actor == "a" || !strings.HasPrefix(p.Seed[0].Dot.Actor, "copia:") {
		t.Errorf("la copia reutiliza el actor del original: %+v", p.Seed[0].Dot)
	}
	var hoja string
	if err := json.Unmarshal(p.Seed[0].Value, &hoja); err != nil || hoja != "Partida" {
		t.Errorf("contenido copiado: %v %q", err, hoja)
	}

	// Sin título, el del original.
	h.createDocument(httptest.NewRecorder(), req(t, "POST", "/v1/documents", ownerClaims(), nil, `{"copy_of":"d1"}`))
	if fs.creado.Title != "Presupuesto" {
		t.Errorf("título por defecto: %q", fs.creado.Title)
	}

	// No se mezcla con una plantilla ni con un fichero de origen.
	for nombre, cuerpo := range map[string]string{
		"copia y plantilla": `{"copy_of":"d1","template":"carta"}`,
		"copia y ancla":     `{"copy_of":"d1","source_ref":"drive:abc"}`,
	} {
		rec := httptest.NewRecorder()
		h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, cuerpo))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", nombre, rec.Code, rec.Body.String())
		}
	}

	// De un documento que no se puede ver, nada: ni se crea.
	fs.creado = nil
	fs.doc = &domain.Document{ID: "d1", OwnerSub: "otra", Kind: domain.KindDocument, Title: "Ajeno"}
	rec = httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, `{"copy_of":"d1"}`))
	if rec.Code != http.StatusForbidden || fs.creado != nil {
		t.Errorf("copia de un documento ajeno: %d, creado %+v", rec.Code, fs.creado)
	}
}
