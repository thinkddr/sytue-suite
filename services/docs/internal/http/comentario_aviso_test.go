// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Una respuesta en una hoja de cálculo avisa al dueño y a quien escribe en el hilo,
// nunca a quien responde; la mención por dirección avisa a esa dirección. Todos abren
// el documento, y en nativo lo abre Calcus (origen `sheets`).
func TestUnComentarioAvisaAQuienTieneQueEnterarse(t *testing.T) {
	avisos := make(chan hubnotify.Notification, 8)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	raiz := "root"
	fs := &fakeStore{
		doc: &domain.Document{ID: "d1", OwnerSub: "owner", Kind: domain.KindSpreadsheet, Title: "Presupuesto"},
		comments: []domain.Comment{
			{ID: raiz, AuthorSub: "carl"},
			{ID: "r2", ParentID: &raiz, AuthorSub: "owner"},
			{ID: "otro", AuthorSub: "eva"}, // de otro hilo: no se entera
		},
	}
	h := newHandler(fs)
	h.hubNotifier, h.publicURL = hubnotify.New(hub.URL, "s"), "https://docs.sytue.eu"
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "bea", Name: "Bea"})

	h.avisarComentario(ctx, "bea", &domain.Comment{ID: "c3", DocumentID: "d1", ParentID: &raiz, AuthorSub: "bea", Body: "De acuerdo, (@Dan@Acme.es) lo mira."})

	var a []string
	for range 3 {
		select {
		case n := <-avisos:
			a = append(a, n.Subject+" "+n.Type)
			if n.Source != "sheets" || n.URL != "https://calcus.sytue.eu/calcus/acme/edit?id=d1" || n.Params["actor"] != "Bea" || n.Params["name"] != "Presupuesto" {
				t.Errorf("aviso = %+v", n)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("llegaron %v", a)
		}
	}
	sort.Strings(a)
	quiero := []string{"carl docs.comment", "email:dan@acme.es docs.mention", "owner docs.comment"}
	for i := range quiero {
		if a[i] != quiero[i] {
			t.Fatalf("avisos = %v, quiero %v", a, quiero)
		}
	}
	select {
	case n := <-avisos:
		t.Fatalf("aviso de más: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}
