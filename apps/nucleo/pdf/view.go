// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// MaxSVGViewPages bounds a batch of vector pages.
	MaxSVGViewPages = 20
	// MaxPNGViewPages bounds the larger raster output and its working memory.
	MaxPNGViewPages = 8
	maxViewBytes    = 4 * MaxPDFBytes
)

// ViewResult is a local rendering result, ready for either UI or HTTP transport.
type ViewResult struct {
	ContentType string
	Data        []byte
}

// TextSpan retains selectable text and its PDF coordinates in the studio wire format.
type TextSpan struct {
	Texto  string  `json:"texto"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Tamano float64 `json:"tamano"`
}

// TextSpans uses the existing page interpreter; unreadable pages retain a nil slot.
func TextSpans(ctx context.Context, doc *PDFDoc) ([][]TextSpan, error) {
	pages := make([][]TextSpan, 0, doc.NumPaginas())
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fragments, err := doc.TextoDePagina(i)
		if err != nil {
			pages = append(pages, nil)
			continue
		}
		spans := make([]TextSpan, 0, len(fragments))
		for _, fragment := range fragments {
			if strings.TrimSpace(fragment.Texto) == "" {
				continue
			}
			spans = append(spans, TextSpan{Texto: fragment.Texto, X: fragment.X, Y: fragment.Y, Tamano: fragment.Tamano})
		}
		pages = append(pages, spans)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return pages, nil
}

func renderOptions(fields map[string]string) (OpcionesRender, string, error) {
	scale := 1.0
	if raw := strings.TrimSpace(fields["escala"]); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 16 {
			return OpcionesRender{}, "", inputError("The scale must be between 0 and 16.")
		}
		scale = value
	}
	format := strings.TrimSpace(fields["formato"])
	if format != "" && format != "svg" && format != "png" {
		return OpcionesRender{}, "", inputError(`The format must be "svg" or "png".`)
	}
	return OpcionesRender{Escala: scale, SinAnotaciones: fields["sin_anotaciones"] == "1"}, format, nil
}

// RenderView preserves the studio's SVG, PNG and JSON-batch formats without I/O.
// Cancellation is checked between pages and before publishing any derived data.
func RenderView(ctx context.Context, doc *PDFDoc, fields map[string]string) (ViewResult, error) {
	if err := ctx.Err(); err != nil {
		return ViewResult{}, err
	}
	opts, format, err := renderOptions(fields)
	if err != nil {
		return ViewResult{}, err
	}
	if pages := strings.TrimSpace(fields["paginas"]); pages != "" {
		indices, err := viewBatchIndices(pages, doc.NumPaginas(), format)
		if err != nil {
			return ViewResult{}, err
		}
		return renderViewBatch(ctx, doc, indices, opts, format)
	}
	page, err := ParseOptionalInteger(fields["pagina"], 1, doc.NumPaginas())
	if err != nil {
		return ViewResult{}, err
	}
	out, err := renderViewPage(doc, page-1, opts, format)
	if cancelled := ctx.Err(); cancelled != nil {
		clear(out)
		return ViewResult{}, cancelled
	}
	if err != nil {
		return ViewResult{}, inputError("That page could not be drawn.")
	}
	contentType := "image/svg+xml; charset=utf-8"
	if format == "png" {
		contentType = "image/png"
	}
	return ViewResult{ContentType: contentType, Data: out}, nil
}

func viewBatchIndices(pages string, total int, format string) ([]int, error) {
	indices, err := ParsePageSelection(pages, total)
	if err != nil {
		return nil, err
	}
	maximum := MaxSVGViewPages
	if format == "png" {
		maximum = MaxPNGViewPages
	}
	if len(indices) > maximum {
		return nil, inputError(fmt.Sprintf("Too many pages in a single batch (maximum %d).", maximum))
	}
	return indices, nil
}

func renderViewPage(doc *PDFDoc, index int, opts OpcionesRender, format string) ([]byte, error) {
	if format == "png" {
		return doc.RenderizarPaginaPNG(index, opts)
	}
	svg, err := doc.RenderizarPaginaSVG(index, opts)
	return []byte(svg), err
}

func encodeViewPage(raw []byte, format string) ([]byte, error) {
	var rendered string
	if format == "png" {
		rendered = base64.StdEncoding.EncodeToString(raw)
	} else {
		rendered = string(raw)
	}
	return json.Marshal(rendered)
}

func renderViewBatch(ctx context.Context, doc *PDFDoc, indices []int, opts OpcionesRender, format string) (ViewResult, error) {
	out := make(map[string]json.RawMessage, len(indices))
	remaining := maxViewBytes
	for _, index := range indices {
		if err := ctx.Err(); err != nil {
			return ViewResult{}, err
		}
		raw, err := renderViewPage(doc, index, opts, format)
		if err != nil {
			continue
		}
		encoded, err := encodeViewPage(raw, format)
		clear(raw)
		if err != nil {
			return ViewResult{}, err
		}
		if len(encoded) > remaining {
			clear(encoded)
			return ViewResult{}, inputError("The rendered pages exceed the output limit.")
		}
		remaining -= len(encoded)
		out[strconv.Itoa(index+1)] = encoded
	}
	body := map[string]any{"paginas": out, "total": doc.NumPaginas()}
	if format == "png" {
		body["formato"] = "png"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ViewResult{}, err
	}
	if len(raw) > maxViewBytes {
		clear(raw)
		return ViewResult{}, inputError("The rendered pages exceed the output limit.")
	}
	if err = ctx.Err(); err != nil {
		clear(raw)
		return ViewResult{}, err
	}
	return ViewResult{ContentType: "application/json", Data: raw}, nil
}
