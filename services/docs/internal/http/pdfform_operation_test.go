// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func formOperationRequest(t *testing.T, pdf []byte, fields map[string]string, aux []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, part := range []struct {
		field, name string
		data        []byte
	}{{"file", "form.pdf", pdf}, {"datos", "data.xfdf", aux}} {
		if part.data == nil {
			continue
		}
		file, err := form.CreateFormFile(part.field, part.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	for field, value := range fields {
		if err := form.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pdf/form", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	return request.WithContext(authn.Into(request.Context(), authn.Claims{Subject: "owner", Tenant: "acme"}))
}

func TestPDFFormHTTPRoundTripPreservesSharedContract(t *testing.T) {
	writer := pdfsimple.Nuevo()
	writer.Texto("private form original", "F1", 12)
	run := func(raw []byte, fields map[string]string, aux []byte) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		(&Handler{}).pdfFormulario(response, formOperationRequest(t, raw, fields, aux))
		if response.Code != http.StatusOK {
			t.Fatal("form HTTP contract", fields["op"], response.Code, response.Body)
		}
		return response
	}
	created := run(writer.Bytes(), map[string]string{"op": "crear", "campos": `[{"tipo":"texto","nombre":"Name","pagina":0,"rect":[10,10,150,40]}]`}, nil)
	filled := run(created.Body.Bytes(), map[string]string{"op": "rellenar", "valores": `{"Name":"Ana & José"}`}, nil)
	exported := run(filled.Body.Bytes(), map[string]string{"op": "exportar-datos"}, nil)
	if exported.Header().Get("Content-Type") != "application/vnd.adobe.xfdf" {
		t.Fatal("XFDF content type lost")
	}
	imported := run(created.Body.Bytes(), map[string]string{"op": "importar-datos"}, exported.Body.Bytes())
	if imported.Header().Get("X-Sytue-Campos-Importados") != "1" {
		t.Fatal("XFDF import count lost")
	}
	read := run(imported.Body.Bytes(), map[string]string{"op": "leer"}, nil)
	var fields struct {
		Campos []domain.CampoFormulario `json:"campos"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &fields); err != nil || len(fields.Campos) != 1 || fields.Campos[0].Valor != "Ana & José" {
		t.Fatal("HTTP form data lost", err)
	}
	run(created.Body.Bytes(), map[string]string{"op": "detectar", "pagina": "1"}, nil)
	flattened := run(imported.Body.Bytes(), map[string]string{"op": "aplanar"}, nil)
	parsed, err := domain.LeerPDF(flattened.Body.Bytes())
	if err != nil || len(parsed.CamposDelFormulario()) != 0 {
		t.Fatal("HTTP flatten retained form fields", err)
	}
}

func TestPDFFormEditsRequireSignatureInvalidationConsent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "apps", "nucleo", "pdf", "testdata", "pdf", "certificacion-p1.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"crear", "aplanar"} {
		fields := map[string]string{"op": operation, "campos": `[{"tipo":"texto","nombre":"New","pagina":0,"rect":[10,10,150,40]}]`}
		response := httptest.NewRecorder()
		(&Handler{}).pdfFormulario(response, formOperationRequest(t, raw, fields, nil))
		if response.Code != http.StatusConflict {
			t.Fatal("signed form changed without confirmation", operation, response.Code)
		}
		fields["invalidar_firmas"] = "true"
		response = httptest.NewRecorder()
		(&Handler{}).pdfFormulario(response, formOperationRequest(t, raw, fields, nil))
		if response.Code != http.StatusOK {
			t.Fatal("explicitly confirmed form edit rejected", operation, response.Code)
		}
	}
}
