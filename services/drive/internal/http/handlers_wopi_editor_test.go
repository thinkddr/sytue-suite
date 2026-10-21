// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/wopi"
)

// editoresFalsos satisface wopiEditorStore con un editor ya conectado.
type editoresFalsos struct{ e store.WopiEditor }

func (f *editoresFalsos) GetWopiEditor(context.Context) (store.WopiEditor, error) { return f.e, nil }

func (f *editoresFalsos) WopiEditorFor(context.Context, string) (store.WopiEditor, error) {
	return f.e, nil
}

func (f *editoresFalsos) SaveWopiEditor(_ context.Context, e store.WopiEditor, _ string) (store.WopiEditor, error) {
	f.e = e
	return e, nil
}

func (f *editoresFalsos) DeleteWopiEditor(context.Context) error {
	f.e = store.WopiEditor{}
	return nil
}

// GET /wopi/editor tiene que decir si ESTE despliegue puede acuñar tokens WOPI.
//
// Sin `WOPI_KEY` el host WOPI ni se monta (server.go), así que la consola cantaba «editor
// conectado» y nadie de la organización veía jamás la acción en Sytue Drive, sin un solo
// error por ningún lado. Es el campo que distingue «no lo has configurado» de «esta
// instalación no puede».
func TestGetWopiEditorDeclaraSiElDespliegueAcunaTokens(t *testing.T) {
	editores := &editoresFalsos{e: store.WopiEditor{DiscoveryURL: "https://office.example/hosting/discovery", Enabled: true}}

	cipher, err := keycrypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}

	casos := []struct {
		nombre  string
		minter  *wopi.Minter
		esperar bool
	}{
		{"sin clave WOPI", nil, false},
		{"con clave WOPI", wopi.NewMinter(cipher, 0), true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := &Handlers{wopi: c.minter, editors: editores}
			rec := httptest.NewRecorder()
			h.GetWopiEditor(rec, httptest.NewRequest(http.MethodGet, "/api/v1/wopi/editor", http.NoBody))

			if rec.Code != http.StatusOK {
				t.Fatalf("código %d, esperado 200", rec.Code)
			}
			var body struct {
				Editor      map[string]any `json:"editor"`
				HostEnabled bool           `json:"host_enabled"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("json: %v (%s)", err, rec.Body.String())
			}
			if body.HostEnabled != c.esperar {
				t.Fatalf("host_enabled = %v, esperado %v", body.HostEnabled, c.esperar)
			}
			if body.Editor == nil {
				t.Fatal("el editor conectado tiene que seguir viajando")
			}
		})
	}
}
