// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// anclasDeMentira apunta lo que se le pidió y contesta lo que diga `err`.
type anclasDeMentira struct {
	llamadas      int
	antes, despue string
	err           error
}

func (a *anclasDeMentira) AnclarEnBoveda(_ context.Context, _, antes, despues string) error {
	a.llamadas++
	a.antes, a.despue = antes, despues
	return a.err
}

const (
	anclaUno = "vault:0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c"
	anclaDos = "vault:9f8e7d6c-5b4a-4321-8fed-cba987654321"
)

func pedirAncla(t *testing.T, h *Handler, claims *authn.Claims, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.anclarEnBoveda(rec, req(t, http.MethodPut, "/api/v1/documents/d1/vault-anchor", claims,
		map[string]string{"id": "d1"}, cuerpo))
	return rec
}

// El dueño ancla un documento sin fichero al que el navegador dejó cifrado en la
// bóveda; luego puede pasarlo a otro de la bóveda, siempre comparando con el que hay.
func TestAnclarEnBovedaDesdeSinAncla(t *testing.T) {
	a := &anclasDeMentira{}
	h := newHandler(&fakeStore{doc: ownedDoc()})
	h.anclaBoveda = a

	rec := pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaUno+`","expect":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	if a.llamadas != 1 || a.antes != "" || a.despue != anclaUno {
		t.Fatalf("se pidió %d veces (%q → %q)", a.llamadas, a.antes, a.despue)
	}

	doc := ownedDoc()
	doc.SourceRef = anclaUno
	h = newHandler(&fakeStore{doc: doc})
	a = &anclasDeMentira{}
	h.anclaBoveda = a
	rec = pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaDos+`","expect":"`+anclaUno+`"}`)
	if rec.Code != http.StatusNoContent || a.antes != anclaUno || a.despue != anclaDos {
		t.Fatalf("de bóveda a bóveda: %d (%q → %q)", rec.Code, a.antes, a.despue)
	}
	// Repetir la misma no es un error ni vuelve a escribir.
	a = &anclasDeMentira{}
	h.anclaBoveda = a
	rec = pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaUno+`","expect":""}`)
	if rec.Code != http.StatusNoContent || a.llamadas != 0 {
		t.Fatalf("repetida: %d, %d escrituras", rec.Code, a.llamadas)
	}
}

// Un «drive:» no se sustituye: la papelera de Drive lo sigue usando.
func TestAnclarEnBovedaNoPisaElDeDrive(t *testing.T) {
	doc := ownedDoc()
	doc.SourceRef = "drive:0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c"
	a := &anclasDeMentira{}
	h := newHandler(&fakeStore{doc: doc})
	h.anclaBoveda = a
	rec := pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaUno+`","expect":"`+doc.SourceRef+`"}`)
	if rec.Code != http.StatusConflict || codigoDe(rec) != "vault_anchor_not_allowed" {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	if a.llamadas != 0 {
		t.Fatal("no debía escribir")
	}
}

// Comparar y cambiar: quien cree que el ancla es otra recibe 409, y lo mismo si el
// almacén ve que cambió entre la autorización y la escritura.
func TestAnclarEnBovedaComparaConLaQueHay(t *testing.T) {
	a := &anclasDeMentira{}
	h := newHandler(&fakeStore{doc: ownedDoc()})
	h.anclaBoveda = a
	rec := pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaDos+`","expect":"`+anclaUno+`"}`)
	if rec.Code != http.StatusConflict || codigoDe(rec) != "vault_anchor_changed" || a.llamadas != 0 {
		t.Fatalf("expect distinto: %d %s", rec.Code, rec.Body.String())
	}

	h.anclaBoveda = &anclasDeMentira{err: domain.ErrNotFound}
	rec = pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaUno+`","expect":""}`)
	if rec.Code != http.StatusConflict || codigoDe(rec) != "vault_anchor_changed" {
		t.Fatalf("carrera: %d %s", rec.Code, rec.Body.String())
	}

	h.anclaBoveda = &anclasDeMentira{err: domain.ErrConflict}
	rec = pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+anclaUno+`","expect":""}`)
	if rec.Code != http.StatusConflict || codigoDe(rec) != "vault_anchor_taken" {
		t.Fatalf("ocupada: %d %s", rec.Code, rec.Body.String())
	}
}

// Solo «vault:<uuid canónico>»: ni otra cosa, ni un «drive:» por esta puerta.
func TestAnclarEnBovedaSoloAnclasDeLaBoveda(t *testing.T) {
	for _, ref := range []string{
		"",
		"vault:",
		"vault:raiz",
		"vault:0B5E7A1C-3F2D-4E6B-9A8C-1D2E3F4A5B6C",
		"vault:{0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c}",
		"drive:0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c",
		"vault:../0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c",
	} {
		a := &anclasDeMentira{}
		h := newHandler(&fakeStore{doc: ownedDoc()})
		h.anclaBoveda = a
		rec := pedirAncla(t, h, ownerClaims(), `{"source_ref":"`+ref+`","expect":""}`)
		if rec.Code != http.StatusBadRequest || a.llamadas != 0 {
			t.Errorf("%q: estado = %d, %d escrituras", ref, rec.Code, a.llamadas)
		}
	}
}

// Solo el dueño: la bóveda de otra persona no es el hogar del documento.
func TestAnclarEnBovedaSoloElDueno(t *testing.T) {
	a := &anclasDeMentira{}
	fs := &fakeStore{doc: ownedDoc(), perms: []domain.Permission{
		{PrincipalKind: domain.PrincipalUser, PrincipalID: "editora", Role: domain.RoleEditor},
	}}
	h := newHandler(fs)
	h.anclaBoveda = a
	rec := pedirAncla(t, h, &authn.Claims{Subject: "editora", Tenant: "acme"},
		`{"source_ref":"`+anclaUno+`","expect":""}`)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("estado = %d: %s", rec.Code, rec.Body.String())
	}
	if a.llamadas != 0 {
		t.Fatal("una editora no debía poder anclar")
	}
}
