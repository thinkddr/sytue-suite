// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
)

// TestNoSeIndexaLaFotoDeOtraPersona (FILE-11): poder ver en Drive la foto de
// otra persona no la mete en la galería propia; ni siquiera se lee su cabecera.
func TestNoSeIndexaLaFotoDeOtraPersona(t *testing.T) {
	var leidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			leidas.Add(1)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id":"d-ajena","name":"vacaciones.jpg","content_type":"image/jpeg","size_bytes":4321,"owner_sub":"otra"}`))
	}))
	defer srv.Close()
	in := service.NuevaIngesta(nil, drive.Nuevo(srv.URL, 5*time.Second), nil, 0, nil)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "t1"})
	if _, err := in.Indexar(ctx, "tok", "d-ajena", "u1"); err == nil {
		t.Fatal("la foto de otra persona se indexó")
	}
	if leidas.Load() != 0 {
		t.Fatalf("se leyó la cabecera de la foto ajena %d veces", leidas.Load())
	}
}
