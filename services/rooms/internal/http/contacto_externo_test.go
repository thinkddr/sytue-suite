// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// La política de contacto con gente de fuera (mig 0083) se lee y se guarda limpia: la
// lista de confianza sin la propia ni repetidos, y vacía si no es ese modo. Un nombre
// que no es de organización vuelve con su código y el nombre en los detalles, para que
// la consola diga cuál.
func TestPoliticaContactoExternoHandlers(t *testing.T) {
	f := &fakeService{}
	h := newHandlers(f)

	rec := httptest.NewRecorder()
	h.GuardarPoliticaContactoExterno(rec, req(http.MethodPut, "/", `{"mode":"confianza","trusted":["Otra","otra","acme"," tercera "]}`, claims(), nil))
	var p struct {
		Mode    string   `json:"mode"`
		Trusted []string `json:"trusted"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &p) != nil || p.Mode != "confianza" ||
		len(p.Trusted) != 2 || p.Trusted[0] != "otra" || p.Trusted[1] != "tercera" {
		t.Fatalf("guardar: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	h.PoliticaContactoExterno(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &p) != nil || p.Mode != "confianza" {
		t.Fatalf("leer: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	h.GuardarPoliticaContactoExterno(rec, req(http.MethodPut, "/", `{"mode":"confianza","trusted":["no vale"]}`, claims(), nil))
	var e struct {
		Error struct {
			Code    string            `json:"code"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &e) != nil ||
		e.Error.Code != "rooms_external_policy_org" || e.Error.Details["org"] != "no vale" {
		t.Fatalf("nombre que no es de organización: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	h.GuardarPoliticaContactoExterno(rec, req(http.MethodPut, "/", `{"mode":"a medias"}`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("modo desconocido: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.GuardarPoliticaContactoExterno(rec, req(http.MethodPut, "/", `{"mode":"cerrada","trusted":["otra"]}`, claims(), nil))
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &p) != nil || p.Mode != "cerrada" || len(p.Trusted) != 0 {
		t.Fatalf("cerrada vacía la lista: %d %s", rec.Code, rec.Body)
	}
}
