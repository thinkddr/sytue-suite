// SPDX-License-Identifier: Apache-2.0
package certipdf

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/pdfsimple"
)

func TestPrivatePDFScanNeverSendsImageToRemoteAssistant(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"private image escaped"}`))
	}))
	defer server.Close()
	writer := pdfsimple.Nuevo()
	writer.Texto("Private scan sentinel", "F1", 14)
	data := &privateDocumentData{raw: writer.Bytes()}
	pdf, err := api.NewPDF(server.URL, server.URL, nil, server.Client()).WithPrivateDrive(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := AbrirDocumento(t.Context(), pdf, "/PDF/Private.pdf")
	if err != nil {
		t.Fatal(err)
	}
	remote := api.NewIA(server.URL, nil, server.Client())
	if text := transcribir(t.Context(), doc, remote, 1); text != "" || calls.Load() != 0 {
		t.Fatal("private OCR leaked to service", text, calls.Load())
	}
}

func TestPrivatePDFTextNeverReachesRemoteAssistant(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	pdf, err := api.NewPDF(server.URL, server.URL, nil, server.Client()).WithPrivateDrive(&privateDocumentData{})
	if err != nil {
		t.Fatal(err)
	}
	doc := &Documento{pdf: pdf}
	response, err := doc.asistir(t.Context(), api.NewIA(server.URL, nil, server.Client()), api.PeticionIA{Contexto: "private extracted text", Accion: "resumir"})
	if !errors.Is(err, api.ErrPrivatePDFOperation) || response != "" || calls.Load() != 0 {
		t.Fatal("private text escaped", response, err, calls.Load())
	}
}

func TestLegacyPDFAssistantStillUsesConfiguredService(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"legacy response"}`))
	}))
	defer server.Close()
	doc := &Documento{pdf: api.NewPDF(server.URL, server.URL, nil, server.Client())}
	response, err := doc.asistir(t.Context(), api.NewIA(server.URL, nil, server.Client()), api.PeticionIA{Contexto: "legacy document"})
	if err != nil || response != "legacy response" || calls.Load() != 1 {
		t.Fatal("legacy assistant changed", response, err, calls.Load())
	}
}
