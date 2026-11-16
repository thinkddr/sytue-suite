// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/pdf/pdftest"
)

func TestSummaryPreservesStudioWireFields(t *testing.T) {
	doc, err := LeerPDF(pdftest.Minimo(t, "<< /Root 1 0 R /Size 4 >>", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Summary(doc, 0))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"paginas", "total", "indice", "enlaces", "adjuntos", "anotaciones", "campos", "firmas", "cifrado", "permisos", "huella"} {
		if _, ok := wire[field]; !ok {
			t.Fatalf("studio field lost: %s", field)
		}
	}
	var pages []map[string]json.RawMessage
	if err = json.Unmarshal(wire["paginas"], &pages); err != nil || len(pages) != 1 {
		t.Fatal("page wire metadata", err)
	}
	for _, field := range []string{"numero", "ancho", "alto", "rotacion", "caja", "extracto"} {
		if _, ok := pages[0][field]; !ok {
			t.Fatalf("page field lost: %s", field)
		}
	}
}
