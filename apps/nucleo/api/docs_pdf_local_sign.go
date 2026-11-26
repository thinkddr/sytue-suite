// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
)

func collectPDFSignFiles(parts []PDFParte, docs []PDFFichero) (map[string][]byte, error) {
	files := make(map[string][]byte, len(parts))
	remaining := PDFMaxBytes
	for _, part := range parts {
		if _, exists := files[part.Campo]; exists || len(part.Datos) > localpdf.MaxOperationFileBytes {
			return nil, localPDFError(http.StatusBadRequest, "bad_request", "Invalid signing file.")
		}
		if len(part.Datos) > remaining {
			return nil, ErrPDFGrande
		}
		remaining -= len(part.Datos)
		files[part.Campo] = part.Datos
	}
	for _, doc := range docs {
		if len(doc.Datos) > remaining {
			return nil, ErrPDFGrande
		}
		remaining -= len(doc.Datos)
	}
	return files, nil
}

func validCertSigningFiles(files map[string][]byte) bool {
	_, p12 := files["pkcs12"]
	if p12 {
		return len(files) == 1
	}
	_, certificate := files["certificado"]
	_, key := files["clave_priv"]
	return certificate && key && len(files) == 2
}

// localSign never sends the document, signing material, or password to the
// former server motor. Parts are borrowed from the caller, not cleared here.
// The time stamp is requested only from the authority the person chose (P-05).
func (p *PDF) localSign(ctx context.Context, fields map[string]string, parts []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
	op := strings.TrimSpace(fields["op"])
	if op == "" {
		op = "dibujada"
	}
	if op != "dibujada" && op != "certificado" {
		return PDFRespuesta{}, ErrPrivatePDFOperation
	}
	files, err := collectPDFSignFiles(parts, docs)
	if err != nil {
		return PDFRespuesta{}, err
	}
	if op == "dibujada" {
		if len(files) > 1 || len(files) == 1 && files["imagen"] == nil {
			return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Invalid signature image.")
		}
		var signature localpdf.Firma
		if err := localpdf.DecodeOperationJSON(fields["firma"], "firma", &signature); err != nil {
			failure := localpdf.DescribeOperationError(err)
			return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
		}
		signature.ImagenPNG = files["imagen"]
		return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
			out, err := localpdf.FirmarDibujadoPDF(doc, signature)
			return localPDFResponse("application/pdf", out), err
		}, docs...)
	}
	if !validCertSigningFiles(files) {
		return PDFRespuesta{}, localPDFError(http.StatusBadRequest, "bad_request", "Invalid signing identity.")
	}
	// This wire type excludes key material and function values from JSON. The
	// identity is accepted only in the explicit file parts, as in the UI.
	var options struct {
		Nombre, Motivo, Lugar, Contacto string
		Fecha                           time.Time
		Pagina                          int
		Rect                            [4]float64
		Trazos                          [][][2]float64
		Bloquear                        bool
	}
	if err := localpdf.DecodeOperationJSON(fields["firma"], "firma", &options); err != nil {
		failure := localpdf.DescribeOperationError(err)
		return PDFRespuesta{}, localPDFError(failure.Status, failure.Code, failure.Message)
	}
	opts := localpdf.OpcionesFirma{
		Nombre: options.Nombre, Motivo: options.Motivo, Lugar: options.Lugar, Contacto: options.Contacto,
		Fecha: options.Fecha, Pagina: options.Pagina, Rect: options.Rect, Trazos: options.Trazos, Bloquear: options.Bloquear,
		CertificadoPEM: files["certificado"], ClavePEM: files["clave_priv"], PKCS12: files["pkcs12"], ClaveDelPKCS12: fields["clave_p12"],
	}
	// El sello, si la persona eligió autoridad (P-05). Como en el servicio, con un fichero
	// de certificado una autoridad caída no tumba la firma: sale sin sello y se dice en
	// X-Sytue-Sello.
	sellado := false
	if sello := p.selloLocal(); sello.URL != "" && fields["sin_sello"] != "1" {
		opts.Sello = func(resumen []byte) ([]byte, error) {
			token, err := localpdf.PedirSello(ctx, resumen, sello)
			sellado = err == nil && len(token) > 0
			return token, err
		}
	}
	return p.localSinglePDFEdit(ctx, fields, func(doc *localpdf.PDFDoc) (PDFRespuesta, error) {
		out, err := localpdf.FirmarIncrementalPDF(doc, opts)
		appended := err == nil
		if errors.Is(err, localpdf.ErrNoIncremental) {
			out, err = localpdf.FirmarConCertificadoPDF(doc, opts)
		}
		response := localPDFResponse("application/pdf", out)
		response.Cabecera.Set("X-Sytue-Firma-Anexada", strconv.FormatBool(appended))
		response.Cabecera.Set("X-Sytue-Sello", map[bool]string{true: "si", false: "no"}[sellado])
		return response, err
	}, docs...)
}
