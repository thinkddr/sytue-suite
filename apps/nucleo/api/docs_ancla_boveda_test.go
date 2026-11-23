// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAnclarEnBoveda: la petición es la de la web (`/api/vault-anchor` del BFF de la
// ofimática): PUT con el ancla y la que se espera, también vacía. Un 409 llega con su
// código, y sin servicio (datos privados) no se pide nada.
func TestAnclarEnBoveda(t *testing.T) {
	const ancla = "vault:0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e62"
	var cuerpos []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/documents/d1/vault-anchor" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		datos, _ := io.ReadAll(r.Body)
		var cuerpo map[string]string
		if err := json.Unmarshal(datos, &cuerpo); err != nil {
			t.Errorf("cuerpo %q: %v", datos, err)
		}
		cuerpos = append(cuerpos, cuerpo)
		if len(cuerpos) > 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"vault_anchor_changed","message":"x"}}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, nil)
	ctx := context.Background()
	if err := d.AnclarEnBoveda(ctx, "d1", ancla, ""); err != nil {
		t.Fatal(err)
	}
	if c := cuerpos[0]; c["source_ref"] != ancla || c["expect"] != "" || len(c) != 2 {
		t.Fatalf("cuerpo = %v", c)
	}
	var e *Error
	if err := d.AnclarEnBoveda(ctx, "d1", ancla, "vault:otro"); !errors.As(err, &e) || e.Code() != "vault_anchor_changed" {
		t.Fatalf("anclar con otra ancla esperada = %v", err)
	}
	if err := NewPrivateDocs().AnclarEnBoveda(ctx, "d1", ancla, ""); !errors.Is(err, ErrPrivateDataOnly) {
		t.Fatalf("con los datos privados = %v", err)
	}
}
