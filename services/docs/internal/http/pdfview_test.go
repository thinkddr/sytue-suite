// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

func TestPDFRenderRejectsNonFiniteScale(t *testing.T) {
	writer := pdfsimple.Nuevo()
	writer.Texto("private render", "F1", 12)
	for _, scale := range []string{"NaN", "+Inf", "-Inf"} {
		response := httptest.NewRecorder()
		(&Handler{}).pdfRender(response, multipartFirma(t, "owner", "/api/v1/pdf/render", map[string]string{"escala": scale}, writer.Bytes(), nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("nonfinite render scale %s accepted with status %d", scale, response.Code)
		}
	}
}
