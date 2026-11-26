// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// Lo que el Drive privado hacía en el servidor y ya no puede (ADR 0086, cuarta
// entrega): poner capas (sellos, marcas de agua, numeración), comparar dos PDF,
// verificar sus firmas y sacar un adjunto. Las mismas funciones del motor que usan
// el servicio y el estudio del navegador, en local y sin red.

// localLayer superpone una capa de `/pdf/pages` (`marca-agua`, `fondo`, `numerar`,
// `bates`, `encabezado`, `pie`) a un único documento.
func (p *PDF) localLayer(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		out, err := localpdf.ApplyLayerOperation(fields["op"], doc, fields)
		return localPDFResponse("application/pdf", out), err
	}, docs...)
}

// localCompare compara dos documentos (`/pdf/compare`) por texto o por píxeles.
func (p *PDF) localCompare(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	if len(docs) != 2 {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Comparing needs exactly two PDFs.")
	}
	parsed, err := p.readLocalPDFs(ctx, fields["clave"], docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	out, err := localpdf.ApplyCompareOperation(ctx, parsed[0], parsed[1], fields)
	if err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return PDFRespuesta{}, cancelled
		}
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	if err := p.authorizeLocalPDF(ctx); err != nil {
		return PDFRespuesta{}, err
	}
	return localPDFResponse("application/json", out), nil
}

// localVerify es el informe de `/pdf/verify`. Sin anclas de confianza el dispositivo
// no tiene en quién confiar, y la pantalla lo dice (`anclas: false`); Sytue Office le
// pone las que eligió la persona (ConLocal, P-06). Integridad, cambios posteriores y
// sellos se comprueban siempre.
func (p *PDF) localVerify(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	anchors := p.anclasLocales()
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		out, err := json.Marshal(map[string]any{
			"firmas": doc.VerificarFirmas(localpdf.OpcionesVerificacion{Anclas: anchors, AnclasSello: anchors}),
			"anclas": anchors != nil,
		})
		return localPDFResponse("application/json", out), err
	}, docs...)
}

// localAttachment saca un fichero incrustado (`/pdf/attachment`), siempre como bytes
// opacos: el tipo que declara el documento no decide nada.
func (p *PDF) localAttachment(ctx context.Context, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	name := strings.TrimSpace(fields["nombre"])
	if name == "" {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Say which attachment to extract.")
	}
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		out, err := doc.ExtraerAdjunto(name)
		if err != nil {
			return PDFRespuesta{}, &localpdf.OperationFailure{Status: http.StatusNotFound, Code: "not_found", Message: "This document has no such attachment."}
		}
		return localPDFResponse("application/octet-stream", out), nil
	}, docs...)
}
