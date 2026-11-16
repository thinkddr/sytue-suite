// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func TestPrivatePDFProtectionStaysLocalAndRevalidatesAccess(t *testing.T) {
	requests := 0
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("private protection reached HTTP")
	})}).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	writer := pdfsimple.Nuevo()
	writer.Texto("confidential local text", "F1", 12)
	source := writer.Bytes()
	before := bytes.Clone(source)
	apply := func(body []byte, fields map[string]string) []byte {
		t.Helper()
		response, err := private.Motor(t.Context(), "/pdf/protect", fields, nil, PDFFichero{Nombre: "private.pdf", Datos: body})
		if err != nil {
			t.Fatal(fields["op"], err)
		}
		if response.Cabecera.Get("Cache-Control") != "no-store" {
			t.Fatal("private result was cacheable")
		}
		return response.Datos
	}
	metadata := apply(source, map[string]string{"op": "metadatos", "metadatos": `{"titulo":"Private title","autor":"Private author"}`})
	parsed, err := localpdf.LeerPDF(metadata)
	if err != nil || parsed.Huella().Titulo != "Private title" || parsed.Huella().Autor != "Private author" {
		t.Fatal("local metadata lost", err)
	}
	cleaned := apply(metadata, map[string]string{"op": "sanear"})
	parsed, err = localpdf.LeerPDF(cleaned)
	if err != nil || parsed.Huella().Titulo != "" || parsed.Huella().Autor != "" {
		t.Fatal("local metadata was not removed", err)
	}
	protected := apply(source, map[string]string{"op": "proteger", "usuario": "fixture-passphrase", "permisos": "imprimir"})
	if _, err := localpdf.LeerPDF(protected); !errors.Is(err, localpdf.ErrPDFClaveIncorrecta) {
		t.Fatal("protected PDF opened without a password", err)
	}
	unlocked := apply(protected, map[string]string{"op": "desproteger", "clave": "fixture-passphrase"})
	parsed, err = localpdf.LeerPDF(unlocked)
	if err != nil {
		t.Fatal(err)
	}
	text, err := parsed.TextoPlano(0)
	if err != nil || !strings.Contains(text, "confidential local text") {
		t.Fatal("password round trip lost content", err)
	}
	redacted := apply(source, map[string]string{"op": "redactar", "areas": `[{"Pagina":0,"X1":0,"Y1":0,"X2":600,"Y2":850}]`})
	parsed, err = localpdf.LeerPDF(redacted)
	if err != nil {
		t.Fatal(err)
	}
	text, err = parsed.TextoPlano(0)
	if err != nil || strings.Contains(text, "confidential") || bytes.Contains(redacted, []byte("confidential")) {
		t.Fatal("redacted text remained recoverable", err)
	}
	if !bytes.Equal(source, before) || requests != 0 {
		t.Fatal("private edit altered its original or reached HTTP", requests)
	}
	data.rootFailureAt = data.roots + 2
	response, err := private.Motor(t.Context(), "/pdf/protect", map[string]string{"op": "metadatos", "metadatos": `{"titulo":"Revoked"}`}, nil, PDFFichero{Nombre: "private.pdf", Datos: source})
	if err == nil || len(response.Datos) != 0 || requests != 0 {
		t.Fatal("revoked edit returned private content", err)
	}
}
