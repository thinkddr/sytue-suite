// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	localpdf "git.sytue.eu/sytue/one/apps/nucleo/pdf"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This fixture supplies the existing authorized data port. Actual encryption and
// recovery are covered separately against Synapse; this test owns only UI wiring.
type privateDocumentData struct {
	api.DriveData
	raw     []byte
	failure error
}

func (*privateDocumentData) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "owner", Product: "drive"}
}

func (data *privateDocumentData) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "root"}, data.failure
}

func (data *privateDocumentData) Children(_ context.Context, id, _ string) (api.Children, error) {
	if data.failure != nil {
		return api.Children{}, data.failure
	}
	if id == "root" {
		return api.Children{Folders: []api.Folder{{ID: "pdf-folder", Name: api.PDFCarpeta}}}, nil
	}
	if id != "pdf-folder" {
		return api.Children{}, errors.New("unexpected private folder")
	}
	return api.Children{Files: []api.File{{ID: "private-doc", FolderID: "pdf-folder", Name: "Private.pdf", Version: 1, ContentType: "application/pdf"}}}, nil
}

func (data *privateDocumentData) Download(_ context.Context, id string) (io.ReadCloser, error) {
	if data.failure != nil {
		return nil, data.failure
	}
	if id != "private-doc" {
		return nil, errors.New("unexpected private file")
	}
	return io.NopCloser(bytes.NewReader(data.raw)), nil
}

func TestPrivatePDFDocumentUsesLocalStudioForTextSelectionAndImages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	writer := pdfsimple.Nuevo()
	writer.Texto("Private native document sentinel", "F1", 14)
	data := &privateDocumentData{raw: writer.Bytes()}
	original := bytes.Clone(data.raw)
	pdf, err := api.NewPDF(server.URL, server.URL, nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := AbrirDocumento(t.Context(), pdf, "/PDF/Private.pdf")
	if err != nil {
		t.Fatal(err)
	}
	content, err := doc.Contenido(t.Context())
	if err != nil || content.Total != 1 || len(content.Paginas) != 1 || content.Paginas[0].Caja[2] <= 0 {
		t.Fatal("native private page metadata", err)
	}
	spans, err := doc.Texto(t.Context())
	if err != nil || len(spans) != 1 || len(spans[0]) == 0 || !strings.Contains(spans[0][0].Texto, "Private native") {
		t.Fatal("native private text", err)
	}
	svg, err := doc.SVG(t.Context(), 1)
	if err != nil || !strings.Contains(svg, "Private native") {
		t.Fatal("native private text selection SVG", err)
	}
	images, err := doc.Imagenes(t.Context(), 1, 1, 1)
	if err != nil || len(images) != 1 || images[1].Bounds().Dx() <= 0 {
		t.Fatal("native private page image", err)
	}
	if calls.Load() != 0 || !bytes.Equal(data.raw, original) {
		t.Fatal("native private view used HTTP or changed the original", calls.Load())
	}
	data.failure = errors.New("private work authority changed")
	if _, err := doc.Contenido(t.Context()); !errors.Is(err, data.failure) {
		t.Fatal("native metadata outlived authorization", err)
	}
	if out, err := doc.Imagenes(t.Context(), 1, 1, 1); !errors.Is(err, data.failure) || len(out) > 0 {
		t.Fatal("native rendering outlived authorization", err)
	}
}

func TestPrivatePDFDocumentReadsAndExportsRecoveredFormsLocally(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	base, err := localpdf.LeerPDF(api.PDFEnBlanco())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := localpdf.AnadirCamposPDF(base, []localpdf.CampoNuevo{{Tipo: localpdf.CampoTexto, Nombre: "PrivateName", Pagina: 0, Rect: [4]float64{20, 20, 220, 45}}})
	if err != nil {
		t.Fatal(err)
	}
	form, err := localpdf.LeerPDF(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = localpdf.RellenarFormularioPDF(form, map[string]string{"PrivateName": "Ana & José — 東京"})
	if err != nil {
		t.Fatal(err)
	}
	data := &privateDocumentData{raw: raw}
	pdf, err := api.NewPDF(server.URL, server.URL, nil, nil).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := AbrirDocumento(t.Context(), pdf, "/PDF/Private.pdf")
	if err != nil {
		t.Fatal(err)
	}
	content, err := doc.Contenido(t.Context())
	if err != nil || len(content.Campos) != 1 || content.Campos[0].Valor != "Ana & José — 東京" {
		t.Fatal("native document lost private form values", err)
	}
	exported, err := doc.Binario(t.Context(), "/pdf/form", map[string]string{"op": "exportar-datos"})
	if err != nil {
		t.Fatal(err)
	}
	values, err := localpdf.ImportarXFDF(exported)
	if err != nil || values["PrivateName"] != "Ana & José — 東京" || calls.Load() != 0 {
		t.Fatal("native form export used transport or lost data", err, calls.Load())
	}
	data.failure = errors.New("private form work authority changed")
	if out, err := doc.Binario(t.Context(), "/pdf/form", map[string]string{"op": "exportar-datos"}); !errors.Is(err, data.failure) || len(out) != 0 {
		t.Fatal("native form export outlived authorization", err)
	}
}
