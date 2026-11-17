// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func TestPageSelectionRejectsBothDescendingRangeEndpoints(t *testing.T) {
	for _, spec := range []string{"4-2", "2-0", "4-0"} {
		if _, err := parsearPaginas(spec, 3); err == nil {
			t.Fatalf("out-of-document descending range accepted: %q", spec)
		}
	}
}

func TestPageEditsPreserveSignatureConfirmationFailure(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "apps", "nucleo", "pdf", "testdata", "pdf", "certificacion-p1.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.LeerPDF(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := aplicarOperacionDePagina("rotar", []*domain.PDFDoc{doc}, pet(map[string]string{"paginas": "1", "grados": "90"}))
	var failure *httpx.Error
	if len(out) > 0 || !errors.As(err, &failure) || failure.Status != http.StatusConflict || failure.Code != "pdf_invalidates_signatures" {
		t.Fatal("page operation lost signature confirmation", err)
	}
}
