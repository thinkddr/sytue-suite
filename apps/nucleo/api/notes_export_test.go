// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateNotesExportProbe struct {
	*privateNotesProbe
	book, note, format string
}

func (p *privateNotesExportProbe) ExportNotebook(_ context.Context, book, note, format string) (io.ReadCloser, string, error) {
	p.book, p.note, p.format = book, note, format
	if p.cause != nil {
		return nil, "", p.cause
	}
	return io.NopCloser(strings.NewReader("private device export")), "private.onex", nil
}

func TestPrivateNotesExportNeverUsesReadableHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	probe := &privateNotesExportProbe{privateNotesProbe: &privateNotesProbe{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}}}
	private, err := NewNotes(server.URL, nil, server.Client()).WithPrivateData(probe)
	if err != nil {
		t.Fatal(err)
	}
	body, name, err := private.ExportNotebook(t.Context(), "book", "note", "onex")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "private device export" || name != "private.onex" || probe.book != "book" || probe.note != "note" || probe.format != "onex" {
		t.Fatal("private export changed", name, err)
	}
	probe.cause = errors.New("device unavailable")
	body, _, err = private.ExportNotebook(t.Context(), "book", "", "html")
	if body != nil || !errors.Is(err, probe.cause) || requests.Load() != 0 {
		t.Fatal("export fell back to readable HTTP", err, requests.Load())
	}
}
