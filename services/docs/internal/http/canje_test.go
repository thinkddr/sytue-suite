// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// fakeCanje es el canjeStore de los tests: un enlace fijo y lo que se canjeó.
type fakeCanje struct {
	enlace     *store.EnlaceCanjeable
	anfitriona string
	canjeado   bool
	externo    bool
	sub        string
	tenant     string
	canjearon  []string
	deFuera    []domain.Document
	noEscribe  bool
}

func (f *fakeCanje) EnlaceParaCanjear(context.Context, string) (*store.EnlaceCanjeable, error) {
	if f.enlace == nil {
		return nil, domain.ErrShareExpired
	}
	return f.enlace, nil
}

func (f *fakeCanje) Canjear(ctx context.Context, _ *store.EnlaceCanjeable, sub string, externo bool) (bool, error) {
	t, _ := tenancy.From(ctx)
	f.canjeado, f.externo, f.sub, f.tenant = true, externo, sub, t.Slug
	return !f.noEscribe, nil
}

func (f *fakeCanje) Anfitrion(context.Context, string, string) (string, error) {
	return f.anfitriona, nil
}

func (f *fakeCanje) CanjeadosDe(context.Context, string, string) ([]string, error) {
	return f.canjearon, nil
}

func (f *fakeCanje) DeFuera(context.Context, string, domain.ListFilter) ([]domain.Document, error) {
	return f.deFuera, nil
}

func enlaceDe(rol domain.Role) *store.EnlaceCanjeable {
	return &store.EnlaceCanjeable{LinkID: "l1", ShareResolution: domain.ShareResolution{DocumentID: "d1", Tenant: "acme", Role: rol}}
}

func canjear(t *testing.T, fc *fakeCanje, claims *authn.Claims) int {
	t.Helper()
	h := newHandler(&fakeStore{doc: ownedDoc()})
	h.canje = fc
	rec := httptest.NewRecorder()
	h.redeemShare(rec, req(t, "POST", "/v1/share/redeem", claims, nil, `{"token":"tok"}`))
	return rec.Code
}

func TestCanjearEnlace(t *testing.T) {
	// Un enlace de lector no se canjea: para leer está `/s/{token}`.
	if c := canjear(t, &fakeCanje{enlace: enlaceDe(domain.RoleViewer)}, &authn.Claims{Subject: "ana", Tenant: "acme"}); c != http.StatusForbidden {
		t.Fatalf("lector: code=%d", c)
	}
	// Caducado o revocado: 410, como al abrirlo.
	if c := canjear(t, &fakeCanje{}, &authn.Claims{Subject: "ana", Tenant: "acme"}); c != http.StatusGone {
		t.Fatalf("caducado: code=%d", c)
	}
	// De la misma organización y sin acceso: permiso NO externo, en su organización.
	fc := &fakeCanje{enlace: enlaceDe(domain.RoleEditor)}
	if c := canjear(t, fc, &authn.Claims{Subject: "ana", Tenant: "acme"}); c != http.StatusOK || !fc.canjeado || fc.externo || fc.tenant != "acme" {
		t.Fatalf("misma organización: code=%d %+v", c, fc)
	}
	// De OTRA organización: permiso externo, escrito en la ANFITRIONA.
	fc = &fakeCanje{enlace: enlaceDe(domain.RoleCommenter)}
	if c := canjear(t, fc, &authn.Claims{Subject: "bea", Tenant: "otra", Roles: []string{"admin"}}); c != http.StatusOK || !fc.canjeado || !fc.externo || fc.tenant != "acme" || fc.sub != "bea" {
		t.Fatalf("otra organización: code=%d %+v", c, fc)
	}
	// Quien ya puede lo mismo o más (el dueño) no escribe nada.
	fc = &fakeCanje{enlace: enlaceDe(domain.RoleEditor)}
	if c := canjear(t, fc, ownerClaims()); c != http.StatusOK || fc.canjeado {
		t.Fatalf("dueño: code=%d %+v", c, fc)
	}
}

// enAnfitrion pasa la petición a la organización del documento SOLO si en la del
// token no está y quien llama es externo allí; y entonces sin grupos ni organización.
func TestEnAnfitrion(t *testing.T) {
	var vio struct {
		tenant  string
		subject domain.Subject
	}
	siguiente := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		tt, _ := tenancy.From(r.Context())
		vio.tenant = tt.Slug
		vio.subject, _ = subjectFrom(r)
	})
	claims := &authn.Claims{Subject: "bea", Tenant: "otra", Roles: []string{"admin"}}
	pedir := func(fs *fakeStore, fc *fakeCanje) {
		vio.tenant, vio.subject = "", domain.Subject{}
		h := newHandler(fs)
		h.canje = fc
		r := req(t, "GET", "/v1/documents/d1", claims, map[string]string{"id": "d1"}, "")
		ot, _ := tenancy.New("otra")
		r = r.WithContext(tenancy.Into(r.Context(), ot))
		h.enAnfitrion(siguiente).ServeHTTP(httptest.NewRecorder(), r)
	}

	pedir(&fakeStore{getErr: domain.ErrNotFound}, &fakeCanje{anfitriona: "acme"})
	if vio.tenant != "acme" || len(vio.subject.Groups) != 0 || vio.subject.Tenant != "" || vio.subject.Sub != "bea" {
		t.Fatalf("externo: %+v", vio)
	}
	// Sin permiso externo, nada cambia: decide el RLS de su organización.
	pedir(&fakeStore{getErr: domain.ErrNotFound}, &fakeCanje{})
	if vio.tenant != "otra" || vio.subject.Tenant != "otra" || len(vio.subject.Groups) == 0 {
		t.Fatalf("sin permiso: %+v", vio)
	}
	// El documento está en su organización: ni se pregunta.
	pedir(&fakeStore{doc: ownedDoc()}, &fakeCanje{anfitriona: "acme"})
	if vio.tenant != "otra" {
		t.Fatalf("propio: %+v", vio)
	}
}

// Quitado por el dueño: el canje no escribe y no hay otro acceso, así que 403.
func TestCanjearUnEnlaceQueElDuenoLeQuito(t *testing.T) {
	fc := &fakeCanje{enlace: enlaceDe(domain.RoleEditor), noEscribe: true}
	if c := canjear(t, fc, &authn.Claims{Subject: "bea", Tenant: "otra"}); c != http.StatusForbidden {
		t.Fatalf("code=%d, quiero 403", c)
	}
}

// La lista de quien canjeó desde fuera lleva también esos documentos, por fecha y con
// el tope de la página.
func TestListaConLosDeFuera(t *testing.T) {
	ahora := time.Now()
	fs := &fakeStore{doc: &domain.Document{ID: "d1", OwnerSub: "bea", Title: "mío", UpdatedAt: ahora.Add(-time.Hour)}}
	h := newHandler(fs)
	h.canje = &fakeCanje{deFuera: []domain.Document{{ID: "d9", Title: "de fuera", UpdatedAt: ahora}}}
	rec := httptest.NewRecorder()
	h.listDocuments(rec, req(t, "GET", "/v1/documents?limit=1", &authn.Claims{Subject: "bea", Tenant: "globex"}, nil, ""))
	body := decodeBody(t, rec)
	docs, _ := body["documents"].([]any)
	if len(docs) != 1 || docs[0].(map[string]any)["id"] != "d9" {
		t.Fatalf("la página no lleva el de fuera, más reciente: %v", docs)
	}
}
