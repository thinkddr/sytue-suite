// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// El aviso del Hub de un despliegue lleva un tipo por resultado y el proyecto en
// `params`: con eso cada cliente compone el título en el idioma de quien lo lee
// (`notifications.kind.cloud.deploy.ready|failed`). Antes iba un solo `cloud.deploy`
// con «Despliegue fallido: web» escrito en castellano.
func TestAvisoDelHubDeUnDespliegue(t *testing.T) {
	casos := []struct {
		status, tipo, nivel string
	}{
		{"ready", "cloud.deploy.ready", "success"},
		{"error", "cloud.deploy.failed", "error"},
	}
	for _, c := range casos {
		t.Run(c.status, func(t *testing.T) {
			var got hubnotify.Notification
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				w.WriteHeader(http.StatusCreated)
			}))
			defer srv.Close()
			n := &DeployNotifier{hub: hubnotify.New(srv.URL, "s")}
			n.notifyHub(context.Background(), "acme", store.DeployNotifyTarget{
				ProjectSlug: "web", CommitSHA: "0123456789abcdef", URL: "https://web.example", CreatedBy: "u1",
			}, c.status, uuid.New())
			if got.Type != c.tipo || got.Level != c.nivel || got.Source != "cloud" || got.Subject != "u1" {
				t.Fatalf("aviso = %+v; quiero tipo %s y nivel %s", got, c.tipo, c.nivel)
			}
			if got.Params["project"] != "web" || got.Body != "0123456" || got.URL != "https://web.example" || got.Title == "" {
				t.Fatalf("aviso = %+v: faltan el proyecto, el commit corto, la URL o el título de respaldo", got)
			}
		})
	}
}
