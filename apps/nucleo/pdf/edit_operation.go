// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// EditOperationInput separates the optional replacement image from form data.
// The caller owns authorization and parsing; this engine performs no I/O.
type EditOperationInput struct {
	Fields map[string]string
	Image  []byte
}

type EditOperationResult struct {
	Data         []byte
	ContentType  string
	Replacements *int
}

func editJSONResult(value any) (EditOperationResult, error) {
	raw, err := json.Marshal(value)
	return EditOperationResult{Data: raw, ContentType: "application/json"}, err
}

// ApplyEditOperation shares the existing content editor across HTTP and devices.
func ApplyEditOperation(doc *PDFDoc, input EditOperationInput) (EditOperationResult, error) {
	result := EditOperationResult{ContentType: "application/pdf"}
	if doc == nil {
		return result, inputError("The PDF is missing.")
	}
	fields := input.Fields
	op := strings.TrimSpace(fields["op"])
	readOnly := op == "" || op == "analizar" || op == "buscar-reemplazar" && fields["aplicar"] != "1"
	if !readOnly && doc.conservaFirmas() {
		return result, fmt.Errorf("%w: editing content changes a signed document", ErrInvalidaFirmas)
	}
	var err error
	switch op {
	case "", "analizar":
		page, err := ParseOptionalInteger(fields["pagina"], 1, doc.NumPaginas())
		if err != nil {
			return result, err
		}
		analysis, err := doc.AnalizarEdicion(page - 1)
		if err != nil {
			return result, err
		}
		return editJSONResult(analysis)
	case "texto":
		var changes []CambioDeTexto
		if err := DecodeOperationJSON(fields["cambios"], "cambios", &changes); err != nil {
			return result, err
		}
		result.Data, err = EditarTextoPDF(doc, changes)
	case "buscar-reemplazar":
		return applyEditSearch(doc, fields)
	case "imagen":
		return applyEditImage(doc, input)
	case "capas":
		var visible []int
		if strings.TrimSpace(fields["visibles"]) != "" {
			visible, err = ParseNonNegativeIndices(fields["visibles"])
			if err != nil {
				return result, err
			}
		}
		result.Data, err = CambiarCapasPDF(doc, visible)
	case "indice":
		var bookmarks []Marcador
		if strings.TrimSpace(fields["marcadores"]) != "" {
			if err := DecodeOperationJSON(fields["marcadores"], "marcadores", &bookmarks); err != nil {
				return result, err
			}
		}
		result.Data, err = EscribirIndicePDF(doc, bookmarks)
	default:
		return result, inputError(fmt.Sprintf("%q is not a valid operation. Use analizar, texto, buscar-reemplazar, imagen, capas or indice.", op))
	}
	return result, err
}

func applyEditSearch(doc *PDFDoc, fields map[string]string) (EditOperationResult, error) {
	result := EditOperationResult{ContentType: "application/pdf"}
	var search BusquedaReemplazo
	if err := DecodeOperationJSON(fields["busqueda"], "busqueda", &search); err != nil {
		return result, err
	}
	if fields["aplicar"] != "1" {
		matches, err := doc.BuscarParaReemplazar(search)
		if err != nil {
			return result, err
		}
		return editJSONResult(map[string]any{"coincidencias": matches})
	}
	var selected []Coincidencia
	if fields["elegidas"] != "" {
		if err := DecodeOperationJSON(fields["elegidas"], "elegidas", &selected); err != nil {
			return result, err
		}
	}
	data, count, err := ReemplazarEnDocumentoPDF(doc, search, selected)
	return EditOperationResult{Data: data, ContentType: "application/pdf", Replacements: &count}, err
}

func applyEditImage(doc *PDFDoc, input EditOperationInput) (EditOperationResult, error) {
	result := EditOperationResult{ContentType: "application/pdf"}
	page, err := ParseOptionalInteger(input.Fields["pagina"], 1, doc.NumPaginas())
	if err != nil {
		return result, err
	}
	id := strings.TrimSpace(input.Fields["id"])
	if id == "" {
		return result, inputError("The image id is missing.")
	}
	if len(input.Image) > MaxOperationFileBytes {
		return result, inputError(`The "imagen" file is too large.`)
	}
	result.Data, err = SustituirImagenPDF(doc, page-1, id, input.Image)
	return result, err
}

func EditOperationHeaders(result EditOperationResult) map[string]string {
	if result.Replacements == nil {
		return nil
	}
	return map[string]string{"X-Sytue-Reemplazos": strconv.Itoa(*result.Replacements)}
}
