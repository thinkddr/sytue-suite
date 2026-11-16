// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"strings"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func (p *PDF) localTools(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	if strings.TrimSpace(fields["op"]) == "imagenes-a-pdf" {
		return p.localImagesToPDF(ctx, docs...)
	}
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		result, err := localpdf.ApplyToolOperation(doc, fields)
		response := localPDFResponse(result.ContentType, result.Data)
		for name, value := range localpdf.ToolOperationHeaders(result) {
			response.Cabecera.Set(name, value)
		}
		return response, err
	}, docs...)
}

func (p *PDF) localImagesToPDF(ctx context.Context, docs ...PDFFichero) (PDFRespuesta, error) {
	if err := p.authorizeLocalPDF(ctx); err != nil {
		return PDFRespuesta{}, err
	}
	if err := validateLocalPDFInputs(docs); err != nil {
		return PDFRespuesta{}, err
	}
	images := make([]localpdf.ImagenAPagina, len(docs))
	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return PDFRespuesta{}, err
		}
		images[i] = localpdf.ImagenAPagina{Datos: doc.Datos, Nombre: doc.Nombre}
	}
	out, err := localpdf.ImagenesAPDF(images)
	if err != nil {
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	if err := p.authorizeLocalPDF(ctx); err != nil {
		clear(out)
		return PDFRespuesta{}, err
	}
	return localPDFResponse("application/pdf", out), nil
}
