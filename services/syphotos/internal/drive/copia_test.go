// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// La copia va por la puerta interna de Drive con el secreto (y no con el token de nadie),
// lleva la organización, el fichero y la dueña, y un 507 es ErrSinCuota.
func TestCopiarParaDuenya(t *testing.T) {
	var visto map[string]string
	var auth string
	lleno := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rutaCopiaFotos || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&visto)
		if lleno {
			w.WriteHeader(http.StatusInsufficientStorage)
			return
		}
		_, _ = w.Write([]byte(`{"file":{"id":"copia-1"},"reused":true}`))
	}))
	defer srv.Close()
	c := Nuevo(srv.URL, 0)
	id, reusada, err := c.CopiarParaDuenya(context.Background(), "s3creto", "acme", "f1", "ada", "bob")
	if err != nil || id != "copia-1" || !reusada {
		t.Fatalf("copia: %q %v %v", id, reusada, err)
	}
	if auth != "Bearer s3creto" || visto["tenant"] != "acme" || visto["file_id"] != "f1" || visto["owner_sub"] != "ada" || visto["editor_sub"] != "bob" {
		t.Errorf("a Drive llegó %q %+v", auth, visto)
	}
	lleno = true
	if _, _, err := c.CopiarParaDuenya(context.Background(), "s3creto", "acme", "f1", "ada", "bob"); !errors.Is(err, ErrSinCuota) {
		t.Errorf("sin espacio: %v", err)
	}
}
