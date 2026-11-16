// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
)

func probar(t *testing.T, h *Handlers, proveedor, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/conexiones/"+proveedor+"/probar", strings.NewReader(cuerpo))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer quien-prueba")
	rc := chi.NewRouteContext()
	rc.URLParams.Add("proveedor", proveedor)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	h.ProbarConexion(w, r)
	return w
}

// TestProbarConexionPideUnaFilaSinEscribir: la prueba del asistente pregunta a la
// pasarela por UNA fila del primer objeto del proveedor, con el token de quien
// prueba, y no toca la base (el handler no tiene store y no falla).
func TestProbarConexionPideUnaFilaSinEscribir(t *testing.T) {
	const instancia = "6f1c2a4e-1b2c-4d5e-8f90-123456789abc"
	for proveedor, imp := range importadores {
		var spec map[string]any
		var auth, ruta string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ruta, auth = r.URL.Path, r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&spec)
			_, _ = w.Write([]byte(`{"columns":[{"name":"id","type":"string"}],"rows":[["\"1\""]]}`))
		}))
		h := &Handlers{pasarela: conectores.NewPasarela(srv.URL)}
		w := probar(t, h, proveedor, `{"instancia":"`+instancia+`"}`)
		srv.Close()
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", proveedor, w.Code, w.Body)
		}
		var res struct {
			OK     bool   `json:"ok"`
			Objeto string `json:"objeto"`
			Filas  int    `json:"filas"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if !res.OK || res.Filas != 1 || res.Objeto != imp.mapeos[0].Clave() {
			t.Errorf("%s: respuesta %+v", proveedor, res)
		}
		if ruta != "/api/v1/instances/"+instancia+"/data/query" || auth != "Bearer quien-prueba" {
			t.Errorf("%s: ruta %q auth %q", proveedor, ruta, auth)
		}
		limite := spec["limit"]
		if limite == nil {
			limite = spec["max_rows"]
		}
		if limite != float64(1) {
			t.Errorf("%s: límite %v en %v", proveedor, limite, spec)
		}
	}
}

// TestProbarConexionCuentaElMotivo: si la pasarela dice que no, el motivo llega al
// asistente como 422 y no como un «algo ha ido mal».
func TestProbarConexionCuentaElMotivo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":"credenciales"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	h := &Handlers{pasarela: conectores.NewPasarela(srv.URL)}
	w := probar(t, h, "holded", `{"instancia":"6f1c2a4e-1b2c-4d5e-8f90-123456789abc"}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "401") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := probar(t, h, "stripe", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("stripe no es de importación: %d", w.Code)
	}
}
