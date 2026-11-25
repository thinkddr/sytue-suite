// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestConfinarAOrgActivaLlegaALaBase: lo que corre detrás de confinarAOrgActiva no ve
// filas de otra organización AUNQUE SU SQL NO LAS FILTRE. El manejador de la prueba
// cuenta los dispositivos sin ningún WHERE, que es exactamente el fallo que la RLS
// tiene que parar: si el middleware no fijara el ámbito, contaría los dos.
func TestConfinarAOrgActivaLlegaALaBase(t *testing.T) {
	l := newLiveServer(t)
	orgA, _ := l.orgAdmin(t, "confin-a", "admin@confin-a.test")
	orgB, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "confin-b", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	ana := caOrgUser(t, l, orgA, "ana@confin.test")
	bruno := caOrgUser(t, l, orgB, "bruno@confin.test")
	for _, d := range []domain.Device{
		{OrgID: orgA.ID, UserID: ana.ID, DeviceID: "confin-ana", Name: "Ana"},
		{OrgID: orgB.ID, UserID: bruno.ID, DeviceID: "confin-bruno", Name: "Bruno"},
	} {
		if _, err := l.st.UpsertDevice(l.ctx, nil, d); err != nil {
			t.Fatal(err)
		}
	}

	contar := func(mw func(http.Handler) http.Handler, c tokenClaims) (int, int) {
		t.Helper()
		var n int
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := l.srv.store.DB().QueryRow(r.Context(), `SELECT count(*) FROM devices`).Scan(&n); err != nil {
				t.Errorf("contar dispositivos: %v", err)
			}
		}))
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/devices", http.NoBody)
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyClaims{}, c))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code, n
	}

	if code, n := contar(l.srv.confinarAOrgActiva, tokenClaims{Subject: ana.ID, Tenant: orgA.Slug}); code != http.StatusOK || n != 1 {
		t.Errorf("confinado a A: status=%d dispositivos=%d, quiero 200 y 1", code, n)
	}
	if code, n := contar(l.srv.confinarAPersona, tokenClaims{Subject: bruno.ID, Tenant: orgB.Slug}); code != http.StatusOK || n != 1 {
		t.Errorf("confinado a Bruno: status=%d dispositivos=%d, quiero 200 y 1", code, n)
	}
	// Sin organización activa no se sirve nada sin confinar: se rechaza.
	if code, _ := contar(l.srv.confinarAOrgActiva, tokenClaims{Subject: ana.ID}); code != http.StatusForbidden {
		t.Errorf("sin organización activa: status=%d, quiero 403", code)
	}
	// Un sujeto que no es una persona (un client) no entra en una ruta de persona.
	if code, _ := contar(l.srv.confinarAPersona, tokenClaims{Subject: "client-de-servicio"}); code != http.StatusForbidden {
		t.Errorf("sujeto que no es uuid: status=%d, quiero 403", code)
	}
}

// TestRutasDeAdministracionConfinadas: por la puerta de verdad. Revocar por la API de
// administración un dispositivo de OTRA organización no lo borra, y el listado solo
// trae los de la organización activa.
func TestRutasDeAdministracionConfinadas(t *testing.T) {
	l := newLiveServer(t)
	orgA, tokA := l.orgAdmin(t, "rutas-a", "admin@rutas-a.test")
	orgB, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "rutas-b", Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	bruno := caOrgUser(t, l, orgB, "bruno@rutas.test")
	ana := caOrgUser(t, l, orgA, "ana@rutas.test")
	devB, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{OrgID: orgB.ID, UserID: bruno.ID, DeviceID: "rutas-bruno", Name: "Bruno"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{OrgID: orgA.ID, UserID: ana.ID, DeviceID: "rutas-ana", Name: "Ana"}); err != nil {
		t.Fatal(err)
	}

	rr, body := l.do(t, http.MethodGet, "/api/v1/admin/devices", tokA, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listado: status=%d body=%s", rr.Code, rr.Body)
	}
	items, _ := body["devices"].([]any)
	if len(items) != 1 {
		t.Errorf("listado de A trae %d dispositivos, quiero 1: %s", len(items), rr.Body)
	}
	if rr, _ := l.do(t, http.MethodDelete, "/api/v1/admin/devices/"+devB.ID, tokA, nil); rr.Code == http.StatusOK || rr.Code == http.StatusNoContent {
		t.Errorf("revocar un dispositivo de B desde A: status=%d", rr.Code)
	}
	if _, err := l.st.GetDeviceForUser(l.ctx, bruno.ID, devB.ID); err != nil {
		t.Fatalf("el dispositivo de B desapareció: %v", err)
	}
}
