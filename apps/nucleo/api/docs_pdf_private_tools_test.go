// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"net/http"
	"strconv"
	"testing"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func TestPrivatePDFToolsImportCompressCropAndAuditWithoutHTTP(t *testing.T) {
	requests := 0
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://docs.invalid", "https://drive.invalid", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("private PDF tool reached HTTP")
	})}).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 200, 100))); err != nil {
		t.Fatal(err)
	}
	imageBytes := bytes.Clone(encoded.Bytes())
	imported, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, PDFFichero{Nombre: "private.png", Datos: imageBytes})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := localpdf.LeerPDF(imported.Datos)
	if err != nil || doc.NumPaginas() != 1 {
		t.Fatal("local image import failed", err)
	}
	page, err := doc.Pagina(0)
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "comprimir", "nivel": "sin-perdida"}, nil, PDFFichero{Nombre: "private.pdf", Datos: imported.Datos})
	if err != nil || compressed.Cabecera.Get("X-Sytue-Bytes-Antes") != strconv.Itoa(len(imported.Datos)) || compressed.Cabecera.Get("X-Sytue-Bytes-Despues") != strconv.Itoa(len(compressed.Datos)) || compressed.Cabecera.Get("X-Sytue-Ahorro") == "" {
		t.Fatal("local compression summary lost", err)
	}
	cropped, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "recortar", "margenes": "10,20,10,20"}, nil, PDFFichero{Nombre: "private.pdf", Datos: imported.Datos})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = localpdf.LeerPDF(cropped.Datos)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := doc.Pagina(0)
	if err != nil || actual.Ancho != page.Ancho || actual.Alto != page.Alto || !bytes.Contains(cropped.Datos, []byte("/CropBox")) {
		t.Fatal("local crop lost its crop box or changed the original media bounds", actual, err)
	}
	report, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "comprobar-archivo"}, nil, PDFFichero{Nombre: "private.pdf", Datos: imported.Datos})
	if err != nil || report.Cabecera.Get("Content-Type") != "application/json" || len(report.Datos) == 0 {
		t.Fatal("local archive audit failed", err)
	}
	archived, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "archivo", "idioma": "es", "titulo": "Private archive"}, nil, PDFFichero{Nombre: "private.pdf", Datos: imported.Datos})
	if err != nil {
		t.Fatal("local archive preparation", err)
	}
	doc, err = localpdf.LeerPDF(archived.Datos)
	if err != nil || doc.DeclaraPDFA() != "2b" {
		t.Fatal("local PDF/A declaration lost", err)
	}
	if requests != 0 || !bytes.Equal(imageBytes, encoded.Bytes()) {
		t.Fatal("private tools reached HTTP or altered image", requests)
	}
	data.rootFailureAt = data.roots + 2
	revoked, err := private.Motor(t.Context(), "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, PDFFichero{Nombre: "private.png", Datos: imageBytes})
	if err == nil || len(revoked.Datos) != 0 || requests != 0 {
		t.Fatal("revoked image import returned content", err)
	}
}
