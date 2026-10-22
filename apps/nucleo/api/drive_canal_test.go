// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Los adjuntos de un canal suben y bajan por las rutas del canal, no por las de «Mi
// unidad»: con la personal, Drive devuelve 404 a propósito.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con "/files/…/content" en ChannelContent,
// la petición va a la ruta personal y el doble no la conoce.
func TestLosAdjuntosDelCanalVanPorElCanal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/channels/c1/files":
			if r.URL.Query().Get("name") != "Factura nº 3.pdf" || r.Header.Get("Content-Type") != "application/pdf" {
				t.Errorf("subida: %s %s", r.URL.RawQuery, r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"f1","name":"Factura nº 3.pdf","content_type":"application/pdf","size_bytes":3}`)
		case "GET /api/v1/channels/c1/files/f1/content":
			_, _ = io.WriteString(w, "pdf")
		case "GET /api/v1/channels/c1/files/f1/thumbnail":
			_, _ = io.WriteString(w, "png")
		case "POST /api/v1/channels/c1/files/f1/copy":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"f9","name":"Factura nº 3.pdf"}`)
		default:
			t.Errorf("ruta inesperada %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d, ctx := NewDrive(srv.URL, nil, nil), context.Background()
	f, err := d.UploadToChannel(ctx, "c1", "Factura nº 3.pdf", "application/pdf", strings.NewReader("pdf"), 3)
	if err != nil || f.ID != "f1" || f.SizeBytes != 3 {
		t.Fatalf("subida: %+v, %v", f, err)
	}
	for _, mini := range []bool{false, true} {
		rc, err := d.ChannelContent(ctx, "c1", "f1", mini)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if quiero := map[bool]string{false: "pdf", true: "png"}[mini]; string(b) != quiero {
			t.Errorf("miniatura=%v da %q", mini, b)
		}
	}
	if c, err := d.SaveChannelFileToMyDrive(ctx, "c1", "f1"); err != nil || c.ID != "f9" {
		t.Errorf("guardar en mi unidad: %+v, %v", c, err)
	}
	rutas := webref.Read(t, "services/drive/internal/http/server.go")
	for _, r := range []string{
		`api.Post("/channels/{channelID}/files", h.UploadChannelFile)`,
		`api.Get("/channels/{channelID}/files/{fileID}/content", h.DownloadChannelFile)`,
		`api.Get("/channels/{channelID}/files/{fileID}/thumbnail", h.ThumbnailChannelFile)`,
		`api.Post("/channels/{channelID}/files/{fileID}/copy", h.CopyChannelFileToMyDrive)`,
	} {
		if !strings.Contains(rutas, r) {
			t.Errorf("services/drive ya no monta %s", r)
		}
	}
}
