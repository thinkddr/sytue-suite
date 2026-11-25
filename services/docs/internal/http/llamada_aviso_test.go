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

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// «Llamar desde el documento» avisa por el Hub (ADR 0070) al dueño y a las personas
// con permiso propio, nunca a quien llama, ni a un grupo, a la organización entera o a
// una dirección escrita tal cual. El aviso abre el documento, donde está «Unirse a la
// llamada». Una segunda llamada en la ventana no repite el aviso, y quien no puede ver
// el documento no avisa a nadie.
func TestLlamarDesdeElDocumentoAvisaAQuienColabora(t *testing.T) {
	avisos := make(chan hubnotify.Notification, 8)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	fs := &fakeStore{
		doc: &domain.Document{ID: "d1", OwnerSub: "owner", Kind: domain.KindDocument, Title: "Plan"},
		perms: []domain.Permission{
			{PrincipalKind: domain.PrincipalUser, PrincipalID: "bea", Role: domain.RoleEditor},
			{PrincipalKind: domain.PrincipalUser, PrincipalID: "carl", Role: domain.RoleViewer},
			{PrincipalKind: domain.PrincipalUser, PrincipalID: "dan@acme.es", Role: domain.RoleViewer},
			{PrincipalKind: domain.PrincipalGroup, PrincipalID: "ventas", Role: domain.RoleViewer},
			{PrincipalKind: domain.PrincipalTenant, PrincipalID: "acme", Role: domain.RoleViewer},
		},
	}
	h := newHandler(fs)
	h.hubNotifier, h.publicURL = hubnotify.New(hub.URL, "s"), "https://docs.sytue.eu"
	pedir := func(sub string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/documents/d1/call-notice", http.NoBody)
		ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"})
		ctx = authn.Into(ctx, authn.Claims{Subject: sub, Name: "Bea"})
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", "d1")
		r = r.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
		w := httptest.NewRecorder()
		h.avisarLlamada(w, r)
		return w.Code
	}

	if code := pedir("eve"); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Fatalf("quien no ve el documento => %d, quiero 403/404", code)
	}
	if code := pedir("bea"); code != http.StatusAccepted {
		t.Fatalf("Bea llama => %d, quiero 202", code)
	}
	var a []string
	for range 2 {
		select {
		case n := <-avisos:
			a = append(a, n.Subject)
			if n.Type != "docs.call" || n.Source != "docs" || n.URL != "https://docus.sytue.eu/docus/acme/edit?id=d1" ||
				n.Params["actor"] != "Bea" || n.Params["name"] != "Plan" {
				t.Errorf("aviso = %+v", n)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("llegaron %v", a)
		}
	}
	sort.Strings(a)
	if len(a) != 2 || a[0] != "carl" || a[1] != "owner" {
		t.Fatalf("avisos a %v, quiero carl y owner", a)
	}
	// Otra llamada en la ventana: se acepta, pero no se vuelve a avisar.
	if code := pedir("carl"); code != http.StatusAccepted {
		t.Fatalf("segunda llamada => %d", code)
	}
	select {
	case n := <-avisos:
		t.Fatalf("aviso de más: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}

// La ventana calla el documento diez minutos y luego vuelve a avisar; un documento no
// calla a otro.
func TestLaVentanaDelAvisoDeLlamada(t *testing.T) {
	ahora := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	a := avisosDeLlamada{ahora: func() time.Time { return ahora }}
	if !a.toca("d1") || a.toca("d1") || !a.toca("d2") {
		t.Fatal("dentro de la ventana el mismo documento no avisa dos veces, y otro sí")
	}
	ahora = ahora.Add(ventanaAvisoLlamada)
	if !a.toca("d1") {
		t.Fatal("pasada la ventana vuelve a avisar")
	}
}

// Como mucho maxAvisosDeLlamada personas, sin repetir al dueño que además tiene permiso.
func TestDestinatariosDeLlamadaTieneTope(t *testing.T) {
	doc := &domain.Document{OwnerSub: "owner"}
	perms := make([]domain.Permission, 0, maxAvisosDeLlamada+11)
	perms = append(perms, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner"})
	for i := range maxAvisosDeLlamada + 10 {
		perms = append(perms, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "p" + string(rune('a'+i%26)) + string(rune('a'+i/26))})
	}
	got := destinatariosDeLlamada(doc, perms, "quien")
	if len(got) != maxAvisosDeLlamada || got[0] != "owner" {
		t.Fatalf("destinatarios = %d (%v…), quiero %d empezando por el dueño", len(got), got[:3], maxAvisosDeLlamada)
	}
}
