// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"strings"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func (p *PDF) localContentEdit(ctx context.Context, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
	input := localpdf.EditOperationInput{Fields: fields}
	if len(parts) > 0 {
		if strings.TrimSpace(fields["op"]) != "imagen" || len(parts) != 1 || parts[0].Campo != "imagen" {
			return PDFRespuesta{}, localPDFError(400, "bad_request", "Send a single imagen file.")
		}
		input.Image = parts[0].Datos
		if len(input.Image) > localpdf.MaxOperationFileBytes {
			return PDFRespuesta{}, localPDFError(400, "bad_request", `The "imagen" file is too large.`)
		}
		remaining := PDFMaxBytes - len(input.Image)
		for _, doc := range docs {
			if len(doc.Datos) > remaining {
				return PDFRespuesta{}, ErrPDFGrande
			}
			remaining -= len(doc.Datos)
		}
	}
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		result, err := localpdf.ApplyEditOperation(doc, input)
		response := localPDFResponse(result.ContentType, result.Data)
		for name, value := range localpdf.EditOperationHeaders(result) {
			response.Cabecera.Set(name, value)
		}
		return response, err
	}, docs...)
}
