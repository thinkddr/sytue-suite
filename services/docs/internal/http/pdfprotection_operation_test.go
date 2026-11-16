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

func TestPDFProtectionHTTPMetadataReturnsOneCompleteDocument(t *testing.T) {
	writer := pdfsimple.Nuevo()
	writer.Texto("unchanged original", "F1", 12)
	response := httptest.NewRecorder()
	(&Handler{}).pdfProteger(response, formOperationRequest(t, writer.Bytes(), map[string]string{"op": "metadatos", "metadatos": `{"titulo":"Corrected title"}`}, nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("HTTP metadata response framing changed", response.Code, response.Header())
	}
	parsed, err := domain.LeerPDF(response.Body.Bytes())
	if err != nil || parsed.Huella().Titulo != "Corrected title" {
		t.Fatal("HTTP metadata did not retain the shared edit", err)
	}
}
