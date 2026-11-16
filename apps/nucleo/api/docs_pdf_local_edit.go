// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/http"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// localSinglePDFEdit shares the admission, signature confirmation and final
// authorization check of single-document operations. The edit only runs locally.
func (p *PDF) localSinglePDFEdit(ctx context.Context, fields map[string]string, edit func(*localpdf.PDFDoc) (PDFRespuesta, error), docs ...PDFFichero) (PDFRespuesta, error) {
	if len(docs) != 1 {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Send a single PDF.")
	}
	parsed, err := p.readLocalPDFs(ctx, fields["clave"], docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	if fields["invalidar_firmas"] == "1" || fields["invalidar_firmas"] == "true" {
		parsed[0].PermitirInvalidarFirmas()
	}
	response, err := edit(parsed[0])
	if err != nil {
		clear(response.Datos)
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	if response.Cabecera.Get("Content-Type") == "application/pdf" {
		if change := localpdf.ArchivalChange(parsed[0], response.Datos); change != "" {
			response.Cabecera.Set("X-Sytue-PDFA", change)
		}
	}
	if err := p.authorizeLocalPDF(ctx); err != nil {
		clear(response.Datos)
		return PDFRespuesta{}, err
	}
	return response, nil
}

func (p *PDF) localProtection(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		result, err := localpdf.ApplyProtectionOperation(doc, fields)
		response := localPDFResponse("application/pdf", result.Data)
		if result.UnchangedXMP {
			response.Cabecera.Set("X-Sytue-Metadatos-Xmp", "1")
		}
		return response, err
	}, docs...)
}
