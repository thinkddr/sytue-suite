// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func viewDocument(t *testing.T) *PDFDoc {
	t.Helper()
	writer := pdfsimple.Nuevo()
	writer.Texto("Private view cross-transport sentinel", "F1", 12)
	doc, err := LeerPDF(writer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestViewPreservesRasterVectorAndTextFormats(t *testing.T) {
	doc := viewDocument(t)
	for _, format := range []string{"svg", "png"} {
		single, err := RenderView(t.Context(), doc, map[string]string{"formato": format, "pagina": "1"})
		if err != nil {
			t.Fatal(err)
		}
		if format == "png" {
			if _, err := png.Decode(bytes.NewReader(single.Data)); err != nil {
				t.Fatal("single PNG", err)
			}
		} else if !strings.Contains(string(single.Data), "Private view") {
			t.Fatal("SVG lost selectable text")
		}
		batch, err := RenderView(t.Context(), doc, map[string]string{"formato": format, "paginas": "1"})
		if err != nil || batch.ContentType != "application/json" {
			t.Fatal("batch", err)
		}
		var wire struct {
			Paginas map[string]string `json:"paginas"`
			Total   int               `json:"total"`
			Formato string            `json:"formato"`
		}
		if err = json.Unmarshal(batch.Data, &wire); err != nil || wire.Total != 1 || len(wire.Paginas) != 1 {
			t.Fatal("batch wire", err)
		}
		if format == "png" {
			raw, err := base64.StdEncoding.DecodeString(wire.Paginas["1"])
			if err != nil {
				t.Fatal(err)
			}
			if _, err = png.Decode(bytes.NewReader(raw)); err != nil || wire.Formato != "png" {
				t.Fatal("PNG batch lost format", err)
			}
		} else if !strings.Contains(wire.Paginas["1"], "<svg") {
			t.Fatal("SVG batch lost vector")
		}
	}
	spans, err := TextSpans(t.Context(), doc)
	if err != nil || len(spans) != 1 || len(spans[0]) == 0 || !strings.Contains(spans[0][0].Texto, "Private view") {
		t.Fatal("text view", spans, err)
	}
}

func TestViewBoundsAndCancellation(t *testing.T) {
	doc := viewDocument(t)
	for _, fields := range []map[string]string{
		{"escala": "NaN"},
		{"escala": "Inf"},
		{"escala": "0"},
		{"escala": "16.1"},
		{"pagina": "0"},
		{"pagina": "2"},
		{"paginas": "2-1"},
		{"formato": "html"},
		{"paginas": strings.TrimSuffix(strings.Repeat("1,", MaxSVGViewPages+1), ",")},
		{"paginas": strings.TrimSuffix(strings.Repeat("1,", MaxPNGViewPages+1), ","), "formato": "png"},
	} {
		if result, err := RenderView(t.Context(), doc, fields); err == nil || len(result.Data) > 0 {
			t.Fatal("unbounded or malformed view accepted", fields)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := RenderView(ctx, doc, nil); !errors.Is(err, context.Canceled) || len(result.Data) > 0 {
		t.Fatal("cancelled render exposed data", err)
	}
	if spans, err := TextSpans(ctx, doc); !errors.Is(err, context.Canceled) || len(spans) > 0 {
		t.Fatal("cancelled extraction exposed data", err)
	}
}
