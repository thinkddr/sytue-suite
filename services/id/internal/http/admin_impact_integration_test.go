// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El impacto de una acción admin, contra PostgreSQL real.
//
// La consola preguntaba «¿seguro?» delante de suspender, cambiar el rol o
// expulsar a alguien, que no dice nada que quien lo pulsa no supiera ya. Lo que
// hace parar a tiempo es el número —cuántas sesiones se van a cerrar, si es el
// único propietario—, y ese número solo lo tiene el servidor. Esto prueba que lo
// da bien, incluida la frontera de organización: un impacto que contara sesiones
// de otra organización sería una fuga por una pantalla informativa.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestAdminUserImpact(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// Un miembro con dos sesiones abiertas y un grupo.
	miembro := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	l.liveSession(t, org, miembro.ID, "sesion-1")
	l.liveSession(t, org, miembro.ID, "sesion-2")
	grupo, err := l.st.CreateGroup(l.ctx, nil, org.ID, "Soporte", "")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := l.st.AddGroupMember(l.ctx, nil, org.ID, grupo.ID, miembro.ID); err != nil {
		t.Fatalf("AddGroupMember: %v", err)
	}

	rr, body := l.do(t, http.MethodGet, "/api/v1/admin/users/"+miembro.ID+"/impact", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("impacto: código=%d body=%v", rr.Code, body)
	}
	if n, _ := body["active_sessions"].(float64); int(n) != 2 {
		t.Errorf("sesiones = %v, quiero 2: la confirmación diría un número que no es", body["active_sessions"])
	}
	if n, _ := body["groups"].(float64); int(n) != 1 {
		t.Errorf("grupos = %v, quiero 1", body["groups"])
	}
	if body["last_owner"] != false {
		t.Errorf("last_owner = %v: un miembro raso no es propietario de nada", body["last_owner"])
	}

	// El propietario único SÍ lo es, y eso es lo que impide dejar la organización
	// sin nadie que pueda administrarla.
	owner := l.addMember(t, org, "owner@acme.test", domain.RoleOwner)
	rr, body = l.do(t, http.MethodGet, "/api/v1/admin/users/"+owner.ID+"/impact", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("impacto del owner: código=%d", rr.Code)
	}
	if body["last_owner"] != true {
		t.Errorf("last_owner = %v con un solo propietario", body["last_owner"])
	}

	// Con dos propietarios deja de serlo: el aviso tiene que apagarse solo, o se
	// convierte en ruido que se aprende a ignorar.
	l.addMember(t, org, "owner2@acme.test", domain.RoleOwner)
	_, body = l.do(t, http.MethodGet, "/api/v1/admin/users/"+owner.ID+"/impact", adminTok, nil)
	if body["last_owner"] != false {
		t.Errorf("last_owner sigue siendo %v con dos propietarios", body["last_owner"])
	}

	// Frontera de organización: alguien de otra organización es 404, no un
	// impacto. Es una pantalla informativa, y por ahí también se filtra.
	otra, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	ajeno := l.addMember(t, otra, "ajeno@otra.test", domain.RoleMember)
	rr, _ = l.do(t, http.MethodGet, "/api/v1/admin/users/"+ajeno.ID+"/impact", adminTok, nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("impacto de otra organización: código=%d, quiero 404", rr.Code)
	}
}
