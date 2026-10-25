// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package casthttp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func TestQuotaHTTPPreservesInternalExternalAndUnknownStorage(t *testing.T) {
	database := pgtest.New(t, db.Migrations, db.MigrationsDir)
	for _, tc := range []struct{ name, field, want string }{
		{name: "internal", field: `,"external_storage":false`, want: "false"},
		{name: "external", field: `,"external_storage":true`, want: "true"},
		{name: "older Drive", want: "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			drive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/api/v1/quota" {
					t.Errorf("unexpected quota request: %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, `{"limit_bytes":40,"used_bytes":13,"available_bytes":27`+tc.field+`}`)
			}))
			defer drive.Close()
			svc := service.New(service.Deps{Store: store.New(database.Pool), Cuota: cuota.New(drive.URL, ""), Firmante: media.New("test-key")})
			handler := &Handlers{svc: svc}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/quota", nil)
			req.Header.Set("Authorization", "Bearer token-for-test")
			req = req.WithContext(authn.Into(req.Context(), authn.Claims{Subject: "reader", Tenant: "acme"}))
			response := httptest.NewRecorder()
			handler.Cuota(response, req)
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("la cuota personal no debe compartirse por caché: %v", response.Header())
			}
			var body map[string]json.RawMessage
			err := json.Unmarshal(response.Body.Bytes(), &body)
			if response.Code != http.StatusOK || err != nil || string(body["external_storage"]) != tc.want || string(body["used_bytes"]) != "13" || requests.Load() != 1 {
				t.Fatalf("quota HTTP lost origin or added queries: %d %s requests=%d %v", response.Code, response.Body, requests.Load(), err)
			}
		})
	}
}
