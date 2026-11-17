// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func TestPrivatePDFViewRendersAndSelectsTextWithoutTransport(t *testing.T) {
	calls := 0
	private, err := NewPDF("https://docs.example", "https://drive.example", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected plaintext transport")
	})}).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	writer := pdfsimple.Nuevo()
	writer.Texto("Private device rendering sentinel", "F1", 16)
	doc := PDFFichero{Nombre: "private.pdf", Datos: writer.Bytes()}
	source := bytes.Clone(doc.Datos)
	text, err := private.Texto(t.Context(), doc, "")
	if err != nil || len(text) != 1 || len(text[0]) == 0 || !strings.Contains(text[0][0].Texto, "Private device") {
		t.Fatal("private selectable text lost", text, err)
	}
	svg, err := private.Render(t.Context(), doc, 1, 1, "")
	if err != nil || !strings.Contains(svg, "<svg") || !strings.Contains(svg, "Private device") {
		t.Fatal("private vector render lost text", err)
	}
	raster, err := private.RenderPNG(t.Context(), doc, "1", 1, "")
	if err != nil || len(raster) != 1 {
		t.Fatal("private raster batch", err)
	}
	image, err := png.Decode(bytes.NewReader(raster[1]))
	if err != nil || image.Bounds().Dx() < 1 {
		t.Fatal("private raster cannot be drawn", err)
	}
	batch, err := private.Motor(t.Context(), "/pdf/render", map[string]string{"paginas": "1"}, nil, doc)
	var wire struct {
		Paginas map[string]string `json:"paginas"`
		Total   int               `json:"total"`
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(batch.Datos, &wire); err != nil || wire.Total != 1 || !strings.Contains(wire.Paginas["1"], "<svg") {
		t.Fatal("private vector batch wire format", err)
	}
	single, err := private.Motor(t.Context(), "/pdf/render", map[string]string{"formato": "png", "pagina": "1"}, nil, doc)
	if err != nil || single.Cabecera.Get("Content-Type") != "image/png" || single.Cabecera.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("private single raster response", err)
	}
	if calls != 0 || !bytes.Equal(doc.Datos, source) {
		t.Fatal("private view sent contents or changed original", calls)
	}
}

func TestPrivatePDFViewRejectsInvalidRequestsAndRevokedResults(t *testing.T) {
	data := &privatePDFDataFixture{}
	private, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := PDFFichero{Nombre: "private.pdf", Datos: PDFEnBlanco()}
	for _, fields := range []map[string]string{{"escala": "NaN"}, {"escala": "+Inf"}, {"escala": "0"}, {"pagina": "0"}, {"pagina": "2"}, {"paginas": "2-1"}, {"formato": "html"}} {
		if out, err := private.Motor(t.Context(), "/pdf/render", fields, nil, doc); err == nil || len(out.Datos) > 0 {
			t.Fatal("invalid private render accepted", fields)
		}
	}
	data.rootFailureAt = data.roots + 2
	if out, err := private.Motor(t.Context(), "/pdf/render", map[string]string{"pagina": "1"}, nil, doc); err == nil || len(out.Datos) > 0 {
		t.Fatal("revoked view returned document contents", err)
	}
	data.rootFailureAt = 0
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out, err := private.Motor(ctx, "/pdf/text", nil, nil, doc); !errors.Is(err, context.Canceled) || len(out.Datos) > 0 {
		t.Fatal("cancelled text extraction returned data", err)
	}
}
