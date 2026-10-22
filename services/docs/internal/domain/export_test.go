// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func textState(t *testing.T, chars string) *State {
	t.Helper()
	s := NewState()
	var prev *Dot
	for i, r := range chars {
		v, _ := json.Marshal(string(r))
		dot := Dot{Actor: "A", Counter: uint64(i + 1)}
		op := Op{Kind: OpInsert, Dot: dot, After: prev, Value: v, Lamport: uint64(i + 1)}
		if _, err := s.Apply(op); err != nil {
			t.Fatalf("apply: %v", err)
		}
		d := dot
		prev = &d
	}
	return s
}

func TestExportMarkdownAndHTML(t *testing.T) {
	s := textState(t, "Hola")
	md, ct, ext, err := Export(KindDocument, "Doc", s, FormatMarkdown)
	if err != nil || ext != "md" || !strings.Contains(ct, "markdown") {
		t.Fatalf("markdown: %v %s %s", err, ct, ext)
	}
	if !strings.Contains(string(md), "# Doc") || !strings.Contains(string(md), "Hola") {
		t.Fatalf("markdown contenido inesperado: %q", md)
	}

	htmlOut, _, _, err := Export(KindDocument, "<b>", s, FormatHTML)
	if err != nil {
		t.Fatalf("html: %v", err)
	}
	if strings.Contains(string(htmlOut), "<b>") {
		t.Fatal("el título debe escaparse en HTML")
	}
}

// unzipPart devuelve el contenido de un miembro del zip por nombre.
func unzipPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip inválido: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, _ := f.Open()
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		return buf.String()
	}
	t.Fatalf("falta el miembro %q en el zip", name)
	return ""
}

// El editor v1 guarda el documento como HTML en el registro LWW doc:html: los
// exports deben tomar el contenido de ahí (no del modelo RGA, que está vacío).
func TestExportDocumentFromHTMLRegister(t *testing.T) {
	s := NewState()
	v, _ := json.Marshal("<h1>Título</h1><p>Hola &amp; adiós</p>")
	if _, err := s.Apply(Op{Kind: OpSet, Key: "doc:html", Value: v, Dot: Dot{Actor: "A", Counter: 1}, Lamport: 1}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	htmlOut, _, _, _ := Export(KindDocument, "T", s, FormatHTML)
	if !strings.Contains(string(htmlOut), "<h1>Título</h1>") {
		t.Fatalf("html debe emitir el HTML del editor: %q", htmlOut)
	}

	md, _, _, _ := Export(KindDocument, "T", s, FormatMarkdown)
	if !strings.Contains(string(md), "Título") || !strings.Contains(string(md), "Hola & adiós") {
		t.Fatalf("markdown sin el texto del editor (entidades decodificadas): %q", md)
	}

	docx, _, _, err := Export(KindDocument, "T", s, FormatOOXML)
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if body := unzipPart(t, docx, "word/document.xml"); !strings.Contains(body, "adiós") || !strings.Contains(body, "Título") {
		t.Fatalf("docx sin el texto del editor: %s", body)
	}

	odt, _, _, err := Export(KindDocument, "T", s, FormatODF)
	if err != nil {
		t.Fatalf("odt: %v", err)
	}
	if body := unzipPart(t, odt, "content.xml"); !strings.Contains(body, "adiós") {
		t.Fatalf("odt sin el texto del editor: %s", body)
	}
}

func TestExportDocxIsValidZip(t *testing.T) {
	s := textState(t, "Hi")
	data, ct, ext, err := Export(KindDocument, "T", s, FormatOOXML)
	if err != nil || ext != "docx" {
		t.Fatalf("docx: %v %s", err, ext)
	}
	if !strings.Contains(ct, "wordprocessingml") {
		t.Fatalf("content-type docx inesperado: %s", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("docx no es un zip válido: %v", err)
	}
	want := map[string]bool{"[Content_Types].xml": false, "word/document.xml": false}
	for _, f := range zr.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("falta parte OPC %q en el docx", name)
		}
	}
}

func TestExportODFMimetypeFirstAndStored(t *testing.T) {
	s := textState(t, "x")
	data, _, ext, err := Export(KindDocument, "T", s, FormatODF)
	if err != nil || ext != "odt" {
		t.Fatalf("odt: %v %s", err, ext)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("odt no es zip: %v", err)
	}
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatal("ODF: 'mimetype' debe ser el primer miembro del zip")
	}
	if zr.File[0].Method != zip.Store {
		t.Fatal("ODF: 'mimetype' debe ir sin comprimir (Store)")
	}
}

func TestExportRejectsUnknownFormat(t *testing.T) {
	if _, _, _, err := Export(KindDocument, "T", NewState(), BlobFormat("xyz")); err == nil {
		t.Fatal("formato desconocido debe fallar")
	}
}

func TestExportXlsxUsesA1Refs(t *testing.T) {
	s := NewState()
	v, _ := json.Marshal("42")
	// Celda fila 3, columna 2 -> referencia A1 "B3".
	s.Apply(Op{Kind: OpSet, Key: CellKey("Hoja1", 3, 2), Value: v, Dot: Dot{Actor: "A", Counter: 1}, Lamport: 1})

	data, ct, ext, err := Export(KindSpreadsheet, "Hoja", s, FormatOOXML)
	if err != nil || ext != "xlsx" {
		t.Fatalf("xlsx: %v %s", err, ext)
	}
	if !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content-type xlsx inesperado: %s", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("xlsx no es zip: %v", err)
	}
	var sheet string
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, _ := f.Open()
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		sheet = buf.String()
	}
	if sheet == "" {
		t.Fatal("falta xl/worksheets/sheet1.xml")
	}
	if !strings.Contains(sheet, `<row r="3">`) {
		t.Fatalf("la fila debe llevar índice r=3: %s", sheet)
	}
	if !strings.Contains(sheet, `r="B3"`) {
		t.Fatalf("la celda debe usar referencia A1 B3: %s", sheet)
	}
}
