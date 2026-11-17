// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func TestSharedPDFContentHTTPPreservesReplacementContract(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("Private original text", "F1", 12)
	raw := w.Bytes()
	response := httptest.NewRecorder()
	(&Handler{}).pdfEditar(response, formOperationRequest(t, raw, map[string]string{"op": "buscar-reemplazar", "busqueda": `{"buscar":"original","reemplazar":"changed"}`, "aplicar": "1"}, nil))
	if response.Code != http.StatusOK || response.Header().Get("X-Sytue-Reemplazos") != "1" {
		t.Fatal("HTTP replacement contract changed", response.Code, response.Header())
	}
	doc, err := domain.LeerPDF(response.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	text, err := doc.TextoPlano(0)
	if err != nil || !strings.Contains(text, "changed") {
		t.Fatal("HTTP replacement lost", err)
	}
	response = httptest.NewRecorder()
	(&Handler{}).pdfEditar(response, formOperationRequest(t, raw, map[string]string{"op": "texto", "cambios": "[] {}"}, nil))
	if response.Code != http.StatusBadRequest {
		t.Fatal("HTTP accepted more than one JSON request", response.Code)
	}
}
