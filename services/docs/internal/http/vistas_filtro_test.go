// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type fakeVistas struct{ filas map[string]json.RawMessage } // «doc/sub» → estado

func (f *fakeVistas) VistasFiltro(_ context.Context, docID, sub string) (json.RawMessage, bool, error) {
	e, ok := f.filas[docID+"/"+sub]
	return e, ok, nil
}

func (f *fakeVistas) GuardarVistasFiltro(_ context.Context, docID, sub string, e json.RawMessage) error {
	f.filas[docID+"/"+sub] = e
	return nil
}

func hojaConVistas(rol domain.Role, kind domain.Kind) (*Handler, *fakeVistas) {
	fs := &fakeStore{doc: &domain.Document{ID: "d1", OwnerSub: "owner", Kind: kind, Title: "T"}}
	if rol != "" {
		fs.perms = []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "ana", Role: rol}}
	}
	h := newHandler(fs)
	fv := &fakeVistas{filas: map[string]json.RawMessage{}}
	h.vistasFiltro = fv
	return h, fv
}

func llamar(t *testing.T, h *Handler, metodo string, quien *authn.Claims, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f := h.getVistasFiltro
	if metodo == "PUT" {
		f = h.putVistasFiltro
	}
	f(rec, req(t, metodo, "/", quien, map[string]string{"id": "d1"}, body))
	return rec
}

const vistasAna = `{"state":{"views":{"Hoja1":[{"id":"v1","name":"Solo pagados","excl":{"2":["no"]}}]},"active":{"Hoja1":"v1"}}}`

// Las vistas son de quien las crea: quien solo lee las guarda y las recupera, y NADIE más
// las ve —tampoco el propietario del documento—. El `sub` sale de la sesión, no de la ruta.
func TestVistasDeFiltroSonDeCadaPersona(t *testing.T) {
	h, fv := hojaConVistas(domain.RoleViewer, domain.KindSpreadsheet)
	ana := &authn.Claims{Subject: "ana", Tenant: "acme"}
	duenyo := ownerClaims()

	if rec := llamar(t, h, "PUT", ana, vistasAna); rec.Code != http.StatusNoContent {
		t.Fatalf("una lectora no puede guardar sus vistas: %d %s", rec.Code, rec.Body)
	}
	rec := llamar(t, h, "GET", ana, "")
	var got struct {
		State struct {
			Active map[string]string `json:"active"`
		} `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.State.Active["Hoja1"] != "v1" {
		t.Fatalf("no recupera sus vistas: %d %s", rec.Code, rec.Body)
	}

	// El propietario no ve las de Ana: no hay nada a su nombre.
	if rec := llamar(t, h, "GET", duenyo, ""); rec.Code != http.StatusOK || decodeBody(t, rec)["state"] != nil {
		t.Fatalf("el propietario ve algo que no es suyo: %d %s", rec.Code, rec.Body)
	}
	// Y lo que guarda el propietario no pisa las de Ana.
	if rec := llamar(t, h, "PUT", duenyo, `{"state":{"views":{},"active":{}}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("el propietario no guarda las suyas: %d", rec.Code)
	}
	if len(fv.filas) != 2 || !strings.Contains(string(fv.filas["d1/ana"]), "Solo pagados") {
		t.Fatalf("las vistas de Ana cambiaron: %v", fv.filas)
	}
	// Un extraño (sin permiso) no lee ni escribe.
	extra := &authn.Claims{Subject: "x", Tenant: "acme"}
	for _, m := range []string{"GET", "PUT"} {
		if rec := llamar(t, h, m, extra, vistasAna); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("%s de un extraño: %d", m, rec.Code)
		}
	}
	if len(fv.filas) != 2 {
		t.Fatalf("un extraño dejó vistas: %v", fv.filas)
	}
}

func TestVistasDeFiltroSoloEnHojasDeCalculo(t *testing.T) {
	h, _ := hojaConVistas(domain.RoleViewer, domain.KindDocument)
	rec := llamar(t, h, "GET", &authn.Claims{Subject: "ana", Tenant: "acme"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un texto no tiene vistas de filtro: %d", rec.Code)
	}
}

func TestVistasDeFiltroRechazanLoQueRebasaLasCotas(t *testing.T) {
	h, fv := hojaConVistas(domain.RoleViewer, domain.KindSpreadsheet)
	ana := &authn.Claims{Subject: "ana", Tenant: "acme"}
	vistas := func(n int) string {
		vs := make([]string, n)
		for i := range vs {
			vs[i] = `{"id":"v` + strconv.Itoa(i) + `","name":"n"}`
		}
		return `{"state":{"views":{"H":[` + strings.Join(vs, ",") + `]},"active":{}}}`
	}
	casos := map[string]string{
		"51 vistas en una hoja": vistas(maxVistasPorHoja + 1),
		"campo desconocido":     `{"state":{"views":{},"active":{},"extra":1}}`,
		"no es JSON":            `{"state":`,
		"datos detrás":          `{"state":{"views":{},"active":{}}} {}`,
		"demasiado grande":      `{"state":{"views":{"H":[{"id":"a","name":"` + strings.Repeat("x", maxBytesVistasFiltro) + `"}]},"active":{}}}`,
	}
	for nombre, body := range casos {
		if rec := llamar(t, h, "PUT", ana, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: se esperaba 400 y salió %d", nombre, rec.Code)
		}
	}
	if len(fv.filas) != 0 {
		t.Fatalf("se guardó algo rechazado: %v", fv.filas)
	}
	// Justo en el tope pasa.
	if rec := llamar(t, h, "PUT", ana, vistas(maxVistasPorHoja)); rec.Code != http.StatusNoContent {
		t.Fatalf("50 vistas deberían caber: %d %s", rec.Code, rec.Body)
	}
}
