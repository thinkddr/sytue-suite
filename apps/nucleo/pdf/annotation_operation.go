// SPDX-License-Identifier: Apache-2.0
package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// AnnotationOperationInput keeps transport fields separate from the actor
// authenticated by the caller. A form's autor field cannot author a reply.
type AnnotationOperationInput struct {
	Fields map[string]string
	Author string
}

const MaxCommentIndex = 100000

// ParseNonNegativeIndices preserves the existing comma-separated, zero-based
// annotation/attachment selection. It does not interpret page ranges.
func ParseNonNegativeIndices(raw string) ([]int, error) {
	var result []int
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, inputError(fmt.Sprintf("%q is not a valid index.", part))
		}
		result = append(result, n)
		if len(result) > MaxSelectedPages {
			return nil, inputError("Too many items in a single request.")
		}
	}
	if len(result) == 0 {
		return nil, inputError("No item was given.")
	}
	return result, nil
}

// ApplyAnnotationOperation is shared by the legacy HTTP adapter and authorized
// devices. It edits the supplied PDF in memory and performs no network/storage IO.
func ApplyAnnotationOperation(doc *PDFDoc, input AnnotationOperationInput) ([]byte, error) {
	if doc == nil {
		return nil, inputError("The PDF is missing.")
	}
	switch op := strings.TrimSpace(input.Fields["op"]); op {
	case "", "poner":
		var annotations []Anotacion
		if err := DecodeOperationJSON(input.Fields["anotaciones"], "anotaciones", &annotations); err != nil {
			return nil, err
		}
		return AnadirAnotacionesPDF(doc, annotations)
	case "borrar":
		indices, err := ParseNonNegativeIndices(input.Fields["indices"])
		if err != nil {
			return nil, err
		}
		return BorrarAnotacionesPDF(doc, indices)
	case "aplanar":
		return AplanarAnotacionesPDF(doc)
	case "responder", "marcar":
		index, err := ParseOptionalInteger(input.Fields["indice"], 1, MaxCommentIndex)
		if err != nil {
			return nil, err
		}
		if op == "responder" {
			return ResponderAAnotacionPDF(doc, index-1, input.Fields["texto"], input.Author)
		}
		return MarcarComentarioPDF(doc, index-1, input.Fields["estado"], input.Author)
	default:
		return nil, inputError(fmt.Sprintf("%q is not a valid operation. Use poner, borrar, responder, marcar or aplanar.", op))
	}
}
