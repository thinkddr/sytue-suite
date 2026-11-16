// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func TestPDFToolsHTTPPreserveCompressionSummaryAndRejectInvalidMargins(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("preserved contract", "F1", 12)
	raw := w.Bytes()
	response := httptest.NewRecorder()
	(&Handler{}).pdfHerramientas(response, formOperationRequest(t, raw, map[string]string{"op": "comprimir", "nivel": "sin-perdida"}, nil))
	if response.Code != http.StatusOK || response.Header().Get("X-Sytue-Bytes-Antes") != strconv.Itoa(len(raw)) || response.Header().Get("X-Sytue-Bytes-Despues") != strconv.Itoa(response.Body.Len()) {
		t.Fatal("HTTP compression contract changed", response.Code, response.Header())
	}
	if _, err := domain.LeerPDF(response.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, margins := range []string{"NaN,0,0,0", "0,Inf,0,0"} {
		response = httptest.NewRecorder()
		(&Handler{}).pdfHerramientas(response, formOperationRequest(t, raw, map[string]string{"op": "recortar", "margenes": margins}, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatal("HTTP accepted non-finite margins", response.Code)
		}
	}
}
