// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"
	"testing"
)

type conditionalPDFData struct {
	privatePDFDataFixture
	expected File
	input    []byte
	failure  error
}

func (data *conditionalPDFData) ReplaceFile(_ context.Context, original File, _ string, input io.Reader, size int64) (File, error) {
	data.expected = original
	var err error
	data.input, err = io.ReadAll(input)
	if err != nil || int64(len(data.input)) != size {
		return File{}, errors.New("conditional PDF size mismatch")
	}
	original.Version++
	return original, data.failure
}

func TestPrivatePDFSaveRequiresConditionalWriterAndClearsFailedOutput(t *testing.T) {
	data := &conditionalPDFData{}
	private, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	original := File{ID: "private-file", FolderID: "private-folder", Name: "Private.pdf", Version: 7, Checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	change := PDFCambio{Op: "metadatos", Datos: map[string]string{"titulo": "Private conditional title"}}
	result, err := private.Aplicar(t.Context(), original, PDFFichero{Nombre: original.Name, Datos: PDFEnBlanco()}, "", change)
	if err != nil || data.expected.ID != original.ID || data.expected.Version != original.Version || data.expected.Checksum != original.Checksum || result.Fichero.Version != 8 || len(data.input) == 0 {
		t.Fatal("PDF save lost its opened version", err)
	}
	data.failure = errors.New("private write authority changed")
	result, err = private.Aplicar(t.Context(), original, PDFFichero{Nombre: original.Name, Datos: PDFEnBlanco()}, "", change)
	if !errors.Is(err, data.failure) || len(result.Datos) != 0 {
		t.Fatal("failed private write returned plaintext output", err)
	}
	readerOnly, err := NewPDF("https://unused.invalid", "https://unused.invalid", nil, nil).WithPrivateDrive(&privatePDFDataFixture{})
	if err != nil {
		t.Fatal(err)
	}
	result, err = readerOnly.Aplicar(t.Context(), original, PDFFichero{Nombre: original.Name, Datos: PDFEnBlanco()}, "", change)
	if !errors.Is(err, ErrPrivateDriveOperation) || len(result.Datos) != 0 {
		t.Fatal("private editor fell back to an unconditional upload", err)
	}
}
