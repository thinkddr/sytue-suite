// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type basesSoloLoBasico struct{ BasesData }

func (basesSoloLoBasico) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "local", Subject: "yo", Product: "bases"}
}

// Unos datos privados que no cumplen las interfaces opcionales de Bases siguen
// fallando cerrados en esas rutas, sin tocar la red (§5.7).
func TestBasesSinCaminoLocalFallaCerrado(t *testing.T) {
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { peticiones.Add(1) }))
	defer srv.Close()
	b, err := NewBases(srv.URL, nil, nil).WithPrivateData(basesSoloLoBasico{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	fallos := make([]error, 0, 11)
	fallos = append(fallos,
		b.SetFieldFormula(ctx, "c", "1"),
		b.SetFieldChoices(ctx, "c", []string{"a"}),
		b.ReorderFields(ctx, "t", []string{"c"}),
		b.DeleteComment(ctx, "x"),
		b.DeleteAttachment(ctx, "r", "a"),
	)
	_, e1 := b.UpdateView(ctx, "v", "n", BasesViewConfig{})
	_, e2 := b.ListRecordsCon(ctx, "t", "", "", 0, BasesLectura{Q: "x"})
	_, e3 := b.ImportCSV(ctx, "b", "n", []byte("a\n1\n"))
	_, e4 := b.UploadAttachment(ctx, "r", "c", "a.txt", []byte("hola"))
	_, e5 := b.ListComments(ctx, "r")
	_, e6 := b.DownloadAttachment(ctx, "r", "a")
	fallos = append(fallos, e1, e2, e3, e4, e5, e6)
	for i, err := range fallos {
		if err == nil {
			t.Errorf("la ruta %d no ha fallado", i)
		}
	}
	if peticiones.Load() != 0 {
		t.Fatal("la fachada privada ha salido a la red", peticiones.Load())
	}
}
