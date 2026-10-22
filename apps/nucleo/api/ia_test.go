// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestElModoViajaSoloSiSePide: el Asistente de CertiPDF pide el modelo `calidad`, como
// la web; sin Modo, el campo no va y el servicio usa el rápido.
func TestElModoViajaSoloSiSePide(t *testing.T) {
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibido = nil
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer srv.Close()
	ia := NewIA(srv.URL, nil, nil)
	if _, err := ia.Asistir(context.Background(), PeticionIA{Accion: "resumir", Instruccion: "x", Modo: "calidad"}); err != nil {
		t.Fatal(err)
	}
	if recibido["modo"] != "calidad" {
		t.Errorf("el modo llega como %v", recibido["modo"])
	}
	if _, err := ia.Asistir(context.Background(), PeticionIA{Accion: "resumir", Instruccion: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, va := recibido["modo"]; va {
		t.Error("sin Modo no se manda el campo")
	}
}
