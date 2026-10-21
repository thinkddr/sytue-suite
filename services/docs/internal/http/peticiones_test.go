// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type fakePeticiones struct {
	pedidas  map[string]peticiones.Peticion
	resuelta string // «sub:concede» de la última resolución
}

func (f *fakePeticiones) PedirEdicion(_ context.Context, p peticiones.Peticion) (bool, error) {
	_, habia := f.pedidas[p.Sujeto]
	f.pedidas[p.Sujeto] = p
	return !habia, nil
}

func (f *fakePeticiones) PeticionesEdicion(context.Context, string) ([]peticiones.Peticion, error) {
	out := make([]peticiones.Peticion, 0, len(f.pedidas))
	for _, p := range f.pedidas {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakePeticiones) ResolverPeticionEdicion(_ context.Context, _, sujeto, concede string) error {
	if _, ok := f.pedidas[sujeto]; !ok {
		return domain.ErrNotFound
	}
	delete(f.pedidas, sujeto)
	f.resuelta = sujeto + ":" + concede
	return nil
}

func (f *fakePeticiones) HayPeticionEdicion(_ context.Context, _, sujeto string) (bool, error) {
	_, ok := f.pedidas[sujeto]
	return ok, nil
}

// El permiso de «Pedir acceso de edición»: pide quien solo lee; ve y concede SOLO el
// dueño; y solo a quien lo pidió. Un lector que se concede a sí mismo, o un dueño al
// que le cuelan el `sub` de alguien que no pidió nada, serían una escalada.
func TestPedirYConcederEdicion(t *testing.T) {
	fs := &fakeStore{doc: ownedDoc(), perms: []domain.Permission{
		{PrincipalKind: domain.PrincipalUser, PrincipalID: "ana", Role: domain.RoleViewer},
	}}
	h := newHandler(fs)
	fp := &fakePeticiones{pedidas: map[string]peticiones.Peticion{}}
	h.peticiones = fp
	ana := &authn.Claims{Subject: "ana", Tenant: "acme", Name: "Ana"}
	id := map[string]string{"id": "d1"}
	con := func(sub string) map[string]string { return map[string]string{"id": "d1", "sub": sub} }

	rec := httptest.NewRecorder()
	h.pedirEdicion(rec, req(t, "POST", "/", ana, id, ""))
	if rec.Code != http.StatusOK || fp.pedidas["ana"].Nombre != "Ana" {
		t.Fatalf("la lectora no pudo pedir: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.getDocument(rec, req(t, "GET", "/", ana, id, ""))
	if decodeBody(t, rec)["edit_requested"] != true {
		t.Error("al volver a abrir, la franja no sabe que ya lo pidió")
	}

	for nombre, quien := range map[string]*authn.Claims{"la propia lectora": ana, "un extraño": {Subject: "x", Tenant: "acme"}} {
		rec = httptest.NewRecorder()
		h.listarPeticiones(rec, req(t, "GET", "/", quien, id, ""))
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("%s ve las peticiones: %d", nombre, rec.Code)
		}
		rec = httptest.NewRecorder()
		h.concederPeticion(rec, req(t, "POST", "/", quien, con("ana"), ""))
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("%s concede: %d", nombre, rec.Code)
		}
	}
	if fp.resuelta != "" {
		t.Fatalf("se resolvió sin el dueño: %q", fp.resuelta)
	}

	rec = httptest.NewRecorder()
	h.concederPeticion(rec, req(t, "POST", "/", ownerClaims(), con("nadie"), ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("concedió a quien no lo pidió: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.concederPeticion(rec, req(t, "POST", "/", ownerClaims(), con("ana"), ""))
	if rec.Code != http.StatusNoContent || fp.resuelta != "ana:owner" {
		t.Fatalf("el dueño no pudo conceder: %d %q", rec.Code, fp.resuelta)
	}
}

// Quien ya puede editar no deja petición: la respuesta le dice a la franja que se quite.
func TestPedirEdicionConPermisoNoDejaPeticion(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})
	fp := &fakePeticiones{pedidas: map[string]peticiones.Peticion{}}
	h.peticiones = fp
	rec := httptest.NewRecorder()
	h.pedirEdicion(rec, req(t, "POST", "/", ownerClaims(), map[string]string{"id": "d1"}, ""))
	if rec.Code != http.StatusOK || len(fp.pedidas) != 0 || decodeBody(t, rec)["role"] != "owner" {
		t.Fatalf("el dueño dejó una petición: %d %v", rec.Code, fp.pedidas)
	}
}
