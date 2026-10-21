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

func TestListarPlantillas(t *testing.T) {
	h := newHandler(&fakeStore{})

	rec := httptest.NewRecorder()
	h.listarPlantillas(rec, req(t, "GET", "/api/v1/templates", nil, nil, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.listarPlantillas(rec, req(t, "GET", "/api/v1/templates?locale=es-ES", ownerClaims(), nil, ""))
	var body struct {
		Templates []struct {
			Key, Kind, Name, Description string
		} `json:"templates"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("listar: %d %s", rec.Code, rec.Body.String())
	}
	tipos := map[string]int{}
	for _, p := range body.Templates {
		tipos[p.Kind]++
	}
	if tipos["document"] == 0 || tipos["spreadsheet"] == 0 || tipos["presentation"] == 0 {
		t.Errorf("la galería no cubre los tres tipos: %v", tipos)
	}
	if body.Templates[0].Key != "carta" || body.Templates[0].Name != "Carta formal" {
		t.Errorf("primera plantilla en español: %+v", body.Templates[0])
	}
}

// Crear desde una plantilla siembra su contenido EN EL SERVIDOR, en la misma
// petición: el documento nace con el tipo, el título y el contenido de la
// plantilla, sin que el cliente mande ninguna operación.
func TestCrearDesdePlantilla(t *testing.T) {
	fs := &fakeStore{}
	h := newHandler(fs)

	// El `kind` del cuerpo no manda: la plantilla es una hoja.
	rec := httptest.NewRecorder()
	h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil,
		`{"kind":"document","template":"presupuesto","locale":"es"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", rec.Code, rec.Body.String())
	}
	p := fs.creado
	if p == nil || p.Kind != domain.KindSpreadsheet || p.Title != "Presupuesto" {
		t.Fatalf("parámetros de creación: %+v", p)
	}
	if len(p.Seed) != 1 || p.Seed[0].Key != "doc:sheet" || p.Seed[0].Kind != domain.OpSet {
		t.Fatalf("semilla: %+v", p.Seed)
	}
	var hoja string
	if err := json.Unmarshal(p.Seed[0].Value, &hoja); err != nil || !strings.Contains(hoja, "Partida") {
		t.Errorf("contenido sembrado: %v %s", err, hoja)
	}

	// Con título propio, se respeta.
	h.createDocument(httptest.NewRecorder(), req(t, "POST", "/v1/documents", ownerClaims(), nil,
		`{"template":"carta","locale":"en","title":"Carta al banco"}`))
	if fs.creado.Title != "Carta al banco" || fs.creado.Kind != domain.KindDocument {
		t.Errorf("título propio: %+v", fs.creado)
	}

	// Sin plantilla, nada que sembrar: el documento en blanco de siempre.
	h.createDocument(httptest.NewRecorder(), req(t, "POST", "/v1/documents", ownerClaims(), nil,
		`{"kind":"presentation"}`))
	if len(fs.creado.Seed) != 0 {
		t.Errorf("en blanco con semilla: %+v", fs.creado.Seed)
	}

	for nombre, cuerpo := range map[string]string{
		"plantilla desconocida": `{"template":"no-existe"}`,
		"plantilla y ancla":     `{"template":"carta","source_ref":"drive:abc"}`,
	} {
		rec := httptest.NewRecorder()
		h.createDocument(rec, req(t, "POST", "/v1/documents", ownerClaims(), nil, cuerpo))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", nombre, rec.Code, rec.Body.String())
		}
	}
}
