// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import "fmt"

// ApplyPageOperation applies the shared page editor without I/O or persistence.
func ApplyPageOperation(op, pages, rotation string, docs ...*PDFDoc) ([]byte, error) {
	if op == "combinar" {
		if len(docs) < 2 {
			return nil, inputError("Combining needs at least two files.")
		}
		if len(docs) > MaxInputFiles {
			return nil, inputError(fmt.Sprintf("Too many files in a single request (maximum %d).", MaxInputFiles))
		}
		if err := pageSourcesMayBeRewritten(docs); err != nil {
			return nil, err
		}
		return CombinarPDF(docs...)
	}
	switch op {
	case "extraer", "borrar", "rotar":
	default:
		return nil, inputError(fmt.Sprintf("Unknown op: %q.", op))
	}
	if len(docs) != 1 {
		return nil, inputError(fmt.Sprintf("“%s” works on a single file.", op))
	}
	if err := pageSourcesMayBeRewritten(docs); err != nil {
		return nil, err
	}
	doc := docs[0]
	indices, err := ParsePageSelection(pages, doc.NumPaginas())
	if err != nil {
		return nil, err
	}
	switch op {
	case "extraer":
		return ExtraerPaginasPDF(doc, indices)
	case "borrar":
		selected := make(map[int]bool, len(indices))
		for _, i := range indices {
			selected[i] = true
		}
		if len(selected) == doc.NumPaginas() {
			return nil, inputError("All the pages of the document cannot be deleted.")
		}
		return BorrarPaginasPDF(doc, indices)
	default:
		degrees, err := ParsePageRotation(rotation)
		if err != nil {
			return nil, err
		}
		rotations := make(map[int]int, len(indices))
		for _, i := range indices {
			rotations[i] = degrees
		}
		return RotarPaginasPDF(doc, rotations)
	}
}

// Page assembly rewrites all object offsets; existing signatures cannot survive it.
func pageSourcesMayBeRewritten(docs []*PDFDoc) error {
	for _, doc := range docs {
		if doc == nil {
			return inputError("The PDF is missing.")
		}
		if doc.conservaFirmas() {
			return fmt.Errorf("%w: page assembly requires rewriting the document", ErrInvalidaFirmas)
		}
	}
	return nil
}
