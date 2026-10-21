// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// multipartImport construye una petición POST /api/v1/import con el fichero en el
// campo "file" (multipart/form-data) y claims de sesión opcionales.
func multipartImport(t *testing.T, claims *authn.Claims, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("crear form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("escribir form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("cerrar multipart: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if claims != nil {
		r = r.WithContext(authn.Into(r.Context(), *claims))
	}
	return r
}

func TestImportHandlerRequiresAuth(t *testing.T) {
	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.importDocument(rec, multipartImport(t, nil, "x.csv", []byte("a,b\n1,2")))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth: code=%d", rec.Code)
	}
}

func TestImportHandlerCSV(t *testing.T) {
	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.importDocument(rec, multipartImport(t, ownerClaims(), "ventas.csv", []byte("Nombre;Precio\nCafé;1,50")))
	if rec.Code != http.StatusOK {
		t.Fatalf("csv: code=%d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	if m["kind"] != "spreadsheet" {
		t.Errorf("kind = %v, quiero spreadsheet", m["kind"])
	}
	if m["title"] != "ventas" {
		t.Errorf("title = %v, quiero ventas", m["title"])
	}
	content, _ := m["content"].(string)
	if !strings.Contains(content, "Nombre") || !strings.Contains(content, `"cells"`) {
		t.Errorf("content sin celdas esperadas: %s", content)
	}
}

func TestImportHandlerDocx(t *testing.T) {
	doc := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Hola</w:t></w:r></w:p></w:body></w:document>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(doc))
	_ = zw.Close()

	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.importDocument(rec, multipartImport(t, ownerClaims(), "carta.docx", buf.Bytes()))
	if rec.Code != http.StatusOK {
		t.Fatalf("docx: code=%d body=%s", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	if m["kind"] != "document" {
		t.Errorf("kind = %v, quiero document", m["kind"])
	}
	if content, _ := m["content"].(string); !strings.Contains(content, "<strong>Hola</strong>") {
		t.Errorf("content = %v", m["content"])
	}
}

func TestImportHandlerMissingFile(t *testing.T) {
	// Multipart válido pero sin el campo "file".
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("otro", "x")
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = r.WithContext(authn.Into(r.Context(), *ownerClaims()))

	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.importDocument(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin fichero: code=%d", rec.Code)
	}
}

func TestImportHandlerBadFormat(t *testing.T) {
	// ZIP válido pero sin partes ofimáticas -> 400 (formato no reconocido).
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("nada.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()

	h := newHandler(&fakeStore{})
	rec := httptest.NewRecorder()
	h.importDocument(rec, multipartImport(t, ownerClaims(), "raro.bin", buf.Bytes()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("formato malo: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestLoImportadoCabeEnLaOperacionQueLoSiembra ata el presupuesto de la importación
// (`domain.MaxContenidoSembrado`) al límite de verdad del cuerpo de una operación
// (`httpx.DecodeJSON`): lo convertido más grande que Import deja salir, sembrado como lo
// siembra `open-from-drive.ts` —una operación `set` con el contenido como cadena—, entra.
// Si alguien sube el presupuesto por encima del MiB, esto se pone en rojo antes que
// Drive.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el presupuesto a 1 100 000, DecodeJSON
// contesta «body too large».
func TestLoImportadoCabeEnLaOperacionQueLoSiembra(t *testing.T) {
	contenido := strings.Repeat("a", domain.MaxContenidoSembrado-2) // con sus comillas, justo el presupuesto
	valor, err := json.Marshal(contenido)
	if err != nil {
		t.Fatal(err)
	}
	cuerpo, err := json.Marshal(map[string]any{"ops": []map[string]any{{
		"kind": "set", "dot": map[string]any{"a": "8f14e45f-ceea-467a-9575-2bd1e3f0b8a1", "c": 1},
		"key": "doc:slides", "value": json.RawMessage(valor), "lamport": 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/documents/x/ops", bytes.NewReader(cuerpo))
	var req postOpsReq
	if err := httpx.DecodeJSON(httptest.NewRecorder(), r, &req); err != nil {
		t.Fatalf("una siembra de %d bytes no entra en una operación: %v", len(cuerpo), err)
	}
}
