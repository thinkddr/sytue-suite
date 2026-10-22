// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// basesFalso imita a Sytue Bases: acepta UN token y lo deja de aceptar al revocar.
func basesFalso(t *testing.T, valido *string, vistas *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*vistas++
		if r.Method != http.MethodGet || r.URL.Path != "/_bases/v1/connector/read" || r.URL.RawQuery != "" {
			t.Errorf("petición inesperada: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer "+*valido {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"columns":[{"name":"Cliente","type":"text"},{"name":"Importe","type":"number"}],
			"rows":[["Ana",10],["Bea",20]],
			"provenance":{"source":"sytue-bases","base_name":"Tienda","table_name":"Pedidos","granted_by":"ana","version":3}}`))
	}))
}

func TestBasesLeeConProcedenciaYDejaDeLeerAlRevocar(t *testing.T) {
	token := "sbc_" + strings.Repeat("a", 43)
	vistas := 0
	srv := basesFalso(t, &token, &vistas)
	defer srv.Close()
	e := NewExecutor(Limits{}, "").ConBases(srv.URL)

	res, err := e.Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{Secret: token}, QuerySpec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 || res.Columns[1].Type != "number" {
		t.Errorf("tabla: %+v", res)
	}
	var p struct {
		Source  string `json:"source"`
		Version int    `json:"version"`
	}
	if json.Unmarshal(res.Provenance, &p) != nil || p.Source != "sytue-bases" || p.Version != 3 {
		t.Errorf("procedencia: %s", res.Provenance)
	}

	// Los «pasos aplicados» transforman las filas pero la procedencia se conserva.
	res, err = e.Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{Secret: token}, QuerySpec{Transform: &Transform{Select: []string{"Cliente"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Columns) != 1 || len(res.Provenance) == 0 {
		t.Errorf("transform: %d columnas, procedencia %q", len(res.Columns), res.Provenance)
	}

	// Revocada en Bases: la SIGUIENTE lectura falla (no hay caché) con un mensaje claro.
	antes := vistas
	revocado := token
	token = "sbc_" + strings.Repeat("b", 43)
	_, err = e.Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{Secret: revocado}, QuerySpec{})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "revocado") {
		t.Errorf("tras revocar: %v", err)
	}
	if vistas != antes+1 {
		t.Error("la lectura tras revocar no llegó a Bases: hay una caché que no debería existir")
	}
}

func TestBasesNoMandaOtrosSecretosNiSaleSinConfigurar(t *testing.T) {
	token := "sbc_" + strings.Repeat("a", 43)
	vistas := 0
	srv := basesFalso(t, &token, &vistas)
	defer srv.Close()

	// Un secreto que no es un token de Bases (la contraseña de otra conexión) no sale.
	e := NewExecutor(Limits{}, "").ConBases(srv.URL)
	if _, err := e.Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{Secret: "contraseña-de-postgres"}, QuerySpec{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("secreto ajeno: %v", err)
	}
	if _, err := e.Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{}, QuerySpec{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("sin token: %v", err)
	}
	if vistas != 0 {
		t.Errorf("se llamó a Bases %d veces con un secreto que no era suyo", vistas)
	}
	// Sin raíz configurada no se llama a ningún sitio.
	if _, err := NewExecutor(Limits{}, "").Execute(context.Background(), KindBases, json.RawMessage(`{}`), Credentials{Secret: token}, QuerySpec{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("sin BASES_URL: %v", err)
	}
	// La config de la conexión no puede traer un host: se canoniza a {}.
	cfg, err := SanitizeConfig(KindBases, json.RawMessage(`{"base_url":"https://atacante.example"}`))
	if err != nil || string(cfg) != `{}` {
		t.Errorf("config: %s %v", cfg, err)
	}
	if !KindBases.Valid() {
		t.Error("KindBases no es válido")
	}
}
