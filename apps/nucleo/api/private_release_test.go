// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestSoltarFuentePrivadaPideLaEpocaLeidaConConfirmacion(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	const epoch = "12345678-1234-4234-8234-123456789abc"
	now := time.Now()
	mode, answer := "frozen", http.StatusOK
	var released []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if storage.ValidateRequestScope(r.Header, scope) != nil {
			t.Errorf("sin ámbito: %s %s", r.Method, r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/private-storage/migration":
			state := storage.LegacyMigrationState{Scope: scope, Mode: mode}
			if mode != "legacy" {
				state.Epoch, state.StartedAt, state.Rows = epoch, &now, 3
			}
			_ = json.NewEncoder(w).Encode(state)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/private-storage/migration/release":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			released = append(released, body)
			switch answer {
			case http.StatusUnauthorized:
				w.WriteHeader(answer)
				_, _ = w.Write([]byte(`{"error":{"code":"insufficient_user_authentication"}}`))
			case http.StatusConflict:
				w.WriteHeader(answer)
				_, _ = w.Write([]byte(`{"error":{"code":"private_storage_release_stale"}}`))
			default:
				_, _ = w.Write([]byte(`{"mode":"legacy","released_epoch":"` + epoch + `"}`))
			}
		default:
			t.Errorf("petición %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	answer = http.StatusUnauthorized
	if err := SoltarFuentePrivada(t.Context(), server.URL, nil, scope); !errors.Is(err, ErrReautenticacion) {
		t.Fatal("sin reautenticación:", err)
	}
	answer = http.StatusConflict
	if err := SoltarFuentePrivada(t.Context(), server.URL, nil, scope); !errors.Is(err, ErrFuenteCambiada) {
		t.Fatal("época vieja:", err)
	}
	answer = http.StatusOK
	if err := SoltarFuentePrivada(t.Context(), server.URL, nil, scope); err != nil {
		t.Fatal(err)
	}
	last := released[len(released)-1]
	if last["epoch"] != epoch || last["discard_private_writes"] != true {
		t.Fatal("cuerpo", last)
	}
	mode = "legacy"
	before := len(released)
	if err := SoltarFuentePrivada(t.Context(), server.URL, nil, scope); err != nil || len(released) != before {
		t.Fatal("una fuente legacy no se suelta", err)
	}
}
