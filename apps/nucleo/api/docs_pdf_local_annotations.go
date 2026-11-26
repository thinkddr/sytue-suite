// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func (p *PDF) localAnnotations(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		out, err := localpdf.ApplyAnnotationOperation(doc, localpdf.AnnotationOperationInput{Fields: fields, Author: p.annotationAuthor()})
		return localPDFResponse("application/pdf", out), err
	}, docs...)
}
