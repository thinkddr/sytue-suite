// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// El PDF sale accesible por defecto y con «accessible:false» sale el simple de siempre.
func TestElPDFSaleAccesiblePorDefecto(t *testing.T) {
	doc := &domain.Document{Kind: domain.KindDocument, Title: "Informe"}
	state := domain.NewState()
	no, si := false, true
	for _, c := range []struct {
		nombre     string
		accessible *bool
		etiquetado bool
	}{
		{"sin indicar", nil, true},
		{"true", &si, true},
		{"false", &no, false},
	} {
		var req createExportReq
		if err := json.Unmarshal([]byte(`{"format":"pdf"}`), &req); err != nil {
			t.Fatal(err)
		}
		req.Accessible = c.accessible
		data, ct, ext, informe, err := materializarExport(doc, state, req, nil, "el")
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		cuerpo := string(data)
		if ct != "application/pdf" || ext != "pdf" || !strings.HasPrefix(cuerpo, "%PDF-") {
			t.Errorf("%s: no es un PDF (%s, %s)", c.nombre, ct, ext)
		}
		if got := strings.Contains(cuerpo, "/StructTreeRoot") && strings.Contains(cuerpo, "<pdfuaid:part>1</pdfuaid:part>"); got != c.etiquetado {
			t.Errorf("%s: etiquetado = %v, quería %v", c.nombre, got, c.etiquetado)
		}
		if (informe != nil) != c.etiquetado {
			t.Errorf("%s: informe = %v", c.nombre, informe)
		}
		if c.etiquetado && !strings.Contains(cuerpo, "/Lang (el)") {
			t.Errorf("%s: el idioma de la petición no llega al /Lang", c.nombre)
		}
	}
	// Otros formatos no cambian.
	data, ct, _, informe, err := materializarExport(doc, state, createExportReq{Format: domain.FormatHTML}, nil, "es")
	if err != nil || informe != nil || !strings.HasPrefix(ct, "text/html") || len(data) == 0 {
		t.Errorf("HTML: %v %v %s", err, informe, ct)
	}
}

// Un export servido desde el propio servicio se descarga, no se pinta: el HTML exportado
// es el del editor, sin sanear, y pintarlo aquí sería ejecutarlo en este origen.
func TestUnExportSeDescargaYNoSePinta(t *testing.T) {
	h := http.Header{}
	cabecerasDeExportacion(h, "text/html; charset=utf-8")
	if got := h.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := h.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Errorf("Content-Disposition = %q, esperaba attachment", got)
	}
	if got := h.Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Errorf("Content-Security-Policy = %q, esperaba sandbox", got)
	}
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}
