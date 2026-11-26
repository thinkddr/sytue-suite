// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

// authorizeLocalPDF revalidates the borrowed repository before and after CPU work.
// Root performs the repository's current work-lease, scope and readiness checks;
// no additional identity, WAL, transport or permission is created here.
func (p *PDF) authorizeLocalPDF(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := p.drive.Root(ctx)
	return err
}

func localPDFError(status int, code, message string) error {
	body, _ := json.Marshal(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{code, message})
	return &Error{Status: status, Body: string(body)}
}

func (p *PDF) readLocalPDFs(ctx context.Context, clave string, docs ...PDFFichero) ([]*localpdf.PDFDoc, error) {
	if err := p.authorizeLocalPDF(ctx); err != nil {
		return nil, err
	}
	if err := validateLocalPDFInputs(docs); err != nil {
		return nil, err
	}
	parsed := make([]*localpdf.PDFDoc, 0, len(docs))
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pdf, err := localpdf.LeerPDFConClave(doc.Datos, clave)
		if err != nil {
			failure := localpdf.DescribeReadError(err, doc.Nombre)
			return nil, localPDFError(400, failure.Code, failure.Message)
		}
		parsed = append(parsed, pdf)
	}
	return parsed, nil
}

func validateLocalPDFInputs(docs []PDFFichero) error {
	if len(docs) == 0 {
		return localPDFError(400, "bad_request", `The PDF is missing ("file" field).`)
	}
	if len(docs) > localpdf.MaxInputFiles {
		return localPDFError(400, "bad_request", fmt.Sprintf("Too many files in a single request (maximum %d).", localpdf.MaxInputFiles))
	}
	remaining := PDFMaxBytes
	for _, doc := range docs {
		if len(doc.Datos) > remaining {
			return ErrPDFGrande
		}
		remaining -= len(doc.Datos)
	}
	return nil
}

func (p *PDF) localSummary(ctx context.Context, doc PDFFichero, clave string) ([]byte, error) {
	docs, err := p.readLocalPDFs(ctx, clave, doc)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(localpdf.Summary(docs[0], 300*time.Millisecond))
	if err != nil {
		return nil, err
	}
	if err = p.authorizeLocalPDF(ctx); err != nil {
		clear(raw)
		return nil, err
	}
	return raw, nil
}

func (p *PDF) localContent(ctx context.Context, doc PDFFichero, clave string) (PDFContenido, error) {
	raw, err := p.localSummary(ctx, doc, clave)
	if err != nil {
		return PDFContenido{}, err
	}
	defer clear(raw)
	var out PDFContenido
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (p *PDF) localPages(ctx context.Context, operation, pages string, degrees int, clave string, docs ...PDFFichero) ([]byte, error) {
	response, err := p.localPageOperation(ctx, operation, pages, strconv.Itoa(degrees), clave, false, docs...)
	return response.Datos, err
}

func (p *PDF) localPageOperation(ctx context.Context, operation, pages, rotation, clave string, invalidateSignatures bool, docs ...PDFFichero) (PDFRespuesta, error) {
	parsed, err := p.readLocalPDFs(ctx, clave, docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	if invalidateSignatures {
		for _, doc := range parsed {
			doc.PermitirInvalidarFirmas()
		}
	}
	out, err := localpdf.ApplyPageOperation(operation, pages, rotation, parsed...)
	if err != nil {
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	response := localPDFResponse("application/pdf", out)
	if len(parsed) == 1 {
		if change := localpdf.ArchivalChange(parsed[0], out); change != "" {
			response.Cabecera.Set("X-Sytue-PDFA", change)
		}
	}
	if err = p.authorizeLocalPDF(ctx); err != nil {
		clear(out)
		return PDFRespuesta{}, err
	}
	return response, nil
}

func localPDFResponse(contentType string, out []byte) PDFRespuesta {
	return PDFRespuesta{Datos: out, Cabecera: http.Header{"Content-Type": {contentType}, "Content-Length": {strconv.Itoa(len(out))}, "Cache-Control": {"no-store"}}}
}

// localRoute es una operación del motor hecha en el dispositivo.
type localRoute func(p *PDF, ctx context.Context, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error)

// localRoutes son las rutas del motor que el Drive privado hace en local; las
// demás (pedir firma) contestan ErrPrivatePDFOperation. Reconocer texto solo con el
// reconocedor del dispositivo que pone Sytue Office (ConLocal); sin él, tampoco.
var localRoutes = map[string]localRoute{
	"/pdf/form": (*PDF).localForm,
	"/pdf/annotate": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localAnnotations(ctx, fields, docs...)
	},
	"/pdf/protect": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localProtection(ctx, fields, docs...)
	},
	"/pdf/tools": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localTools(ctx, fields, docs...)
	},
	"/pdf/edit":         (*PDF).localContentEdit,
	"/pdf/sign":         (*PDF).localSign,
	"/pdf/sign/prepare": (*PDF).localSignPrepare,
	"/pdf/sign/complete": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localSignComplete(ctx, fields, docs...)
	},
	"/pdf/compare": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localCompare(ctx, fields, docs...)
	},
	"/pdf/verify": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localVerify(ctx, fields, docs...)
	},
	"/pdf/attachment": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localAttachment(ctx, fields, docs...)
	},
	"/pdf/ocr": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localOCR(ctx, fields, docs...)
	},
	"/pdf/content": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		if len(docs) != 1 {
			return PDFRespuesta{}, localPDFError(400, "bad_request", "Send a single PDF.")
		}
		out, err := p.localSummary(ctx, docs[0], fields["clave"])
		if err != nil {
			return PDFRespuesta{}, err
		}
		return localPDFResponse("application/json", out), nil
	},
	"/pdf/render": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localView(ctx, "/pdf/render", fields, docs...)
	},
	"/pdf/text": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		return p.localView(ctx, "/pdf/text", fields, docs...)
	},
	"/pdf/pages": func(p *PDF, ctx context.Context, fields map[string]string, _ []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
		if localpdf.IsLayerOperation(fields["op"]) {
			return p.localLayer(ctx, fields, docs...)
		}
		invalidate := fields["invalidar_firmas"] == "1" || fields["invalidar_firmas"] == "true"
		return p.localPageOperation(ctx, fields["op"], fields["paginas"], fields["grados"], fields["clave"], invalidate, docs...)
	},
}

// localRoutesWithParts son las que reciben ficheros sueltos además del documento: el
// XFDF, la imagen nueva, la identidad con que se firma o el certificado elegido.
var localRoutesWithParts = map[string]bool{"/pdf/form": true, "/pdf/edit": true, "/pdf/sign": true, "/pdf/sign/prepare": true}

func (p *PDF) localMotor(ctx context.Context, route string, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
	run, ok := localRoutes[route]
	if !ok || len(parts) > 0 && !localRoutesWithParts[route] {
		return PDFRespuesta{}, ErrPrivatePDFOperation
	}
	return run(p, ctx, fields, parts, docs...)
}

func (p *PDF) localView(ctx context.Context, route string, fields map[string]string, docs ...PDFFichero) (PDFRespuesta, error) {
	if len(docs) != 1 {
		return PDFRespuesta{}, localPDFError(400, "bad_request", "Send a single PDF.")
	}
	parsed, err := p.readLocalPDFs(ctx, fields["clave"], docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	var response PDFRespuesta
	if route == "/pdf/text" {
		pages, err := localpdf.TextSpans(ctx, parsed[0])
		if err != nil {
			return PDFRespuesta{}, err
		}
		raw, err := json.Marshal(map[string]any{"paginas": pages})
		if err != nil {
			return PDFRespuesta{}, err
		}
		response = localPDFResponse("application/json", raw)
	} else {
		result, err := localpdf.RenderView(ctx, parsed[0], fields)
		if err != nil {
			if cancelled := ctx.Err(); cancelled != nil {
				return PDFRespuesta{}, cancelled
			}
			failure := localpdf.DescribeOperationError(err)
			return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
		}
		response = localPDFResponse(result.ContentType, result.Data)
		response.Cabecera.Set("X-Content-Type-Options", "nosniff")
	}
	if err = p.authorizeLocalPDF(ctx); err != nil {
		clear(response.Datos)
		return PDFRespuesta{}, err
	}
	return response, nil
}

func (p *PDF) localForm(ctx context.Context, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
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
	input := localpdf.FormOperationInput{Fields: fields, Filename: docs[0].Nombre}
	for _, part := range parts {
		if part.Campo != "datos" || input.DataProvided {
			return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Send a single datos file.")
		}
		input.Data, input.DataProvided = part.Datos, true
	}
	result, err := localpdf.ApplyFormOperation(parsed[0], input)
	if err != nil {
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	response := localPDFResponse(result.ContentType, result.Data)
	if imported := localpdf.FormImportedFieldsHeader(result); imported != "" {
		response.Cabecera.Set("X-Sytue-Campos-Importados", imported)
	}
	if result.ContentType == "application/pdf" {
		if change := localpdf.ArchivalChange(parsed[0], result.Data); change != "" {
			response.Cabecera.Set("X-Sytue-PDFA", change)
		}
	}
	if err = p.authorizeLocalPDF(ctx); err != nil {
		clear(result.Data)
		return PDFRespuesta{}, err
	}
	return response, nil
}
