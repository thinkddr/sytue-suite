// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func TestDecodeSetTemplateEnabledReq(t *testing.T) {
	casos := []struct {
		nombre        string
		body          string
		wantEnabled   bool
		wantUpdatedBy string
	}{
		{
			nombre:        "solo enabled",
			body:          `{"enabled":false}`,
			wantEnabled:   false,
			wantUpdatedBy: "",
		},
		{
			nombre:        "con updated_by desde el proxy de plataforma",
			body:          `{"enabled":false,"updated_by":"admin@sytue.eu"}`,
			wantEnabled:   false,
			wantUpdatedBy: "admin@sytue.eu",
		},
		{
			nombre:        "enabled true",
			body:          `{"enabled":true}`,
			wantEnabled:   true,
			wantUpdatedBy: "",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/operator/templates/asana/enabled", bytes.NewBufferString(c.body))
			w := httptest.NewRecorder()
			var dst setTemplateEnabledReq
			if err := httpx.DecodeJSON(w, req, &dst); err != nil {
				t.Fatalf("DecodeJSON devolvió error inesperado: %v", err)
			}
			if dst.Enabled != c.wantEnabled {
				t.Errorf("got enabled=%v, want %v", dst.Enabled, c.wantEnabled)
			}
			if dst.UpdatedBy != c.wantUpdatedBy {
				t.Errorf("got updated_by=%q, want %q", dst.UpdatedBy, c.wantUpdatedBy)
			}
		})
	}
}
