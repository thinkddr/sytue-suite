// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ToolOperationResult preserves the PDF/JSON result and compression summary
// shared by authorized devices and the HTTP adapter.
type ToolOperationResult struct {
	Data        []byte
	ContentType string
	Compression *ResultadoCompresion
}

// ParseMargins reads the existing left,bottom,right,top selection in points.
func ParseMargins(raw string) ([4]float64, error) {
	var margins [4]float64
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return margins, inputError(`Give four margins in points: "left,bottom,right,top".`)
	}
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 10000 {
			return margins, inputError(fmt.Sprintf("%q is not a valid margin.", part))
		}
		margins[i] = value
	}
	return margins, nil
}

// ApplyToolOperation transforms only the supplied in-memory document. Image
// import uses ImagenesAPDF directly, since it has no input PDF to parse.
func ApplyToolOperation(doc *PDFDoc, fields map[string]string) (ToolOperationResult, error) {
	result := ToolOperationResult{ContentType: "application/pdf"}
	if doc == nil {
		return result, inputError("The PDF is missing.")
	}
	var err error
	switch op := strings.TrimSpace(fields["op"]); op {
	case "comprimir":
		level := NivelCompresion(strings.TrimSpace(fields["nivel"]))
		switch level {
		case CompresionSinPerdida, CompresionEquilibrada, CompresionFuerte:
		case "":
			level = CompresionEquilibrada
		default:
			return result, inputError("The level must be sin-perdida, equilibrada or fuerte.")
		}
		var summary ResultadoCompresion
		result.Data, summary, err = ComprimirPDF(doc, level)
		result.Compression = &summary
	case "recortar":
		margins, parseErr := ParseMargins(fields["margenes"])
		if parseErr != nil {
			return result, parseErr
		}
		result.Data, err = RecortarPDF(doc, margins)
	case "archivo":
		level := PDFA2B
		if strings.TrimSpace(fields["nivel"]) == "2A" {
			level = PDFA2A
		}
		result.Data, err = PrepararArchivoPDF(doc, OpcionesArchivo{Nivel: level, Idioma: strings.TrimSpace(fields["idioma"]), Titulo: strings.TrimSpace(fields["titulo"]), Autor: strings.TrimSpace(fields["autor"])})
		if err != nil {
			return result, &OperationFailure{Status: 400, Code: "pdf_archive_not_possible", Message: err.Error()}
		}
	case "comprobar-archivo":
		result.ContentType = "application/json"
		result.Data, err = json.Marshal(doc.ComprobarArchivo())
	default:
		return result, inputError(fmt.Sprintf("%q is not a valid operation.", op))
	}
	return result, err
}

// ToolOperationHeaders retains the existing informational response names.
func ToolOperationHeaders(result ToolOperationResult) map[string]string {
	if result.Compression == nil {
		return nil
	}
	return map[string]string{"X-Sytue-Bytes-Antes": strconv.Itoa(result.Compression.Antes), "X-Sytue-Bytes-Despues": strconv.Itoa(result.Compression.Despues), "X-Sytue-Ahorro": strconv.Itoa(result.Compression.Ahorro())}
}
