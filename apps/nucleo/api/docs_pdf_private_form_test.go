// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func TestPrivatePDFFormsRoundTripXFDFAndFlattenWithoutTransport(t *testing.T) {
	calls := 0
	original := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("private form reached plaintext transport")
	})})
	private, err := original.WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	blank := PDFEnBlanco()
	before := bytes.Clone(blank)
	run := func(fields map[string]string, parts []PDFParte, data []byte) PDFRespuesta {
		out, err := private.Motor(t.Context(), "/pdf/form", fields, parts, PDFFichero{Nombre: `C:\private\Form.pdf`, Datos: data})
		if err != nil {
			t.Fatal("private form operation", fields["op"], err)
		}
		if calls != 0 {
			t.Fatal("private form sent content to HTTP motor")
		}
		return out
	}
	created := run(map[string]string{"op": "crear", "campos": `[{"tipo":"texto","nombre":"FullName","pagina":0,"rect":[20,20,240,45]}]`}, nil, blank)
	if created.Cabecera.Get("Content-Type") != "application/pdf" {
		t.Fatal("form creation lost PDF response")
	}
	filled := run(map[string]string{"op": "rellenar", "valores": `{"FullName":"Ana & José — 東京"}`}, nil, created.Datos)
	exported := run(map[string]string{"op": "exportar-datos"}, nil, filled.Datos)
	values, err := localpdf.ImportarXFDF(exported.Datos)
	if err != nil || values["FullName"] != "Ana & José — 東京" || !strings.Contains(string(exported.Datos), `href="Form.pdf"`) {
		t.Fatal("private XFDF lost values or exposed local path", err)
	}
	imported := run(map[string]string{"op": "importar-datos"}, []PDFParte{{Campo: "datos", Nombre: "datos.xfdf", Datos: exported.Datos}}, created.Datos)
	if imported.Cabecera.Get("X-Sytue-Campos-Importados") != "1" {
		t.Fatal("import did not report matching form data")
	}
	read := run(map[string]string{"op": "leer"}, nil, imported.Datos)
	var summary struct {
		Campos []localpdf.CampoFormulario `json:"campos"`
	}
	if err = json.Unmarshal(read.Datos, &summary); err != nil || len(summary.Campos) != 1 || summary.Campos[0].Valor != values["FullName"] {
		t.Fatal("private imported form did not recover exact value", err)
	}
	detected := run(map[string]string{"op": "detectar", "pagina": "1"}, nil, created.Datos)
	if !json.Valid(detected.Datos) {
		t.Fatal("private field detection lost JSON contract")
	}
	flattened := run(map[string]string{"op": "aplanar"}, nil, imported.Datos)
	doc, err := localpdf.LeerPDF(flattened.Datos)
	if err != nil || len(doc.CamposDelFormulario()) != 0 {
		t.Fatal("private flatten left active form fields", err)
	}
	if !bytes.Equal(blank, before) {
		t.Fatal("private form processing changed original bytes")
	}
	originalDoc, err := localpdf.LeerPDF(created.Datos)
	if err != nil || len(originalDoc.CamposDelFormulario()) != 1 || originalDoc.CamposDelFormulario()[0].Valor != "" {
		t.Fatal("import or fill overwrote independent original form", err)
	}
}

func TestPrivatePDFFormsRejectMalformedRequestsAndRevokedResults(t *testing.T) {
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := PDFFichero{Nombre: "form.pdf", Datos: PDFEnBlanco()}
	for _, test := range []struct {
		name   string
		fields map[string]string
		parts  []PDFParte
		status int
	}{
		{"missing", map[string]string{"op": "crear"}, nil, http.StatusBadRequest},
		{"unknown-field", map[string]string{"op": "crear", "campos": `[{"unknown":1}]`}, nil, http.StatusBadRequest},
		{"trailing-json", map[string]string{"op": "crear", "campos": `[] {}`}, nil, http.StatusBadRequest},
		{"large-json", map[string]string{"op": "crear", "campos": "[]" + strings.Repeat(" ", localpdf.MaxOperationJSONBytes)}, nil, http.StatusRequestEntityTooLarge},
		{"invalid-page", map[string]string{"op": "detectar", "pagina": "2"}, nil, http.StatusBadRequest},
		{"missing-xfdf", map[string]string{"op": "importar-datos"}, nil, http.StatusBadRequest},
		{"duplicate-xfdf", map[string]string{"op": "importar-datos"}, []PDFParte{{Campo: "datos"}, {Campo: "datos"}}, http.StatusBadRequest},
	} {
		out, err := private.Motor(t.Context(), "/pdf/form", test.fields, test.parts, doc)
		var failure *Error
		if len(out.Datos) != 0 || !errors.As(err, &failure) || failure.Status != test.status {
			t.Fatal("invalid private form request accepted or misclassified", test.name, err)
		}
	}
	data.rootFailureAt = data.roots + 2
	if out, err := private.Motor(t.Context(), "/pdf/form", map[string]string{"op": "leer"}, nil, doc); err == nil || len(out.Datos) != 0 {
		t.Fatal("revoked form returned private contents", err)
	}
}
