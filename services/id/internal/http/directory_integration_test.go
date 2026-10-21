// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests del resolutor de directorio contra un PostgreSQL real (pgtest, build-tag
// `integration`). Lo que se comprueba es la VERJA, que es la razón de que este
// endpoint exista con cuidado: resuelve un correo a un identificador para
// cualquiera de la casa —lo necesita «compartir con una persona» de Drive— y por
// tanto tiene que ser incapaz de contar nada de fuera de la organización.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestDirectoryResolve_NoSaleDeLaOrganizacion(t *testing.T) {
	l := newLiveServer(t)
	acme, admin := l.seedOrgAdmin(t, "acme")
	companero := l.addMember(t, acme, "companero@acme.test", domain.RoleMember)
	otra, _ := l.seedOrgAdmin(t, "otra")
	ajeno := l.addMember(t, otra, "ajeno@otra.test", domain.RoleMember)

	// Un MIEMBRO cualquiera resuelve: no es una operación de administración.
	// `admin/members:resolve` existía y estaba reservado a owner/admin, así que
	// compartir un fichero exigía ser administrador o no se podía hacer.
	tok := l.token(t, companero.ID, acme.Slug, []string{"member"})
	rr, body := l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"email": "admin@acme.test"})
	if rr.Code != http.StatusOK {
		t.Fatalf("un miembro resuelve a otro de su org: status=%d, quiero 200 (%v)", rr.Code, body)
	}
	if body["user_id"] != admin.ID {
		t.Fatalf("user_id = %v, quiero %q", body["user_id"], admin.ID)
	}
	// El ROL no sale de aquí: quien comparte un fichero no tiene por qué saber si
	// la otra persona es administradora.
	if _, hay := body["role"]; hay {
		t.Fatalf("el rol no debe salir del resolutor: %v", body["role"])
	}

	// Alguien de OTRA organización responde igual que alguien que no existe. La
	// diferencia entre las dos respuestas es exactamente el dato que no se puede
	// regalar a quien va probando direcciones: dónde trabaja una persona.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"email": "ajeno@otra.test"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("dirección de otra org: status=%d, quiero 404", rr.Code)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"email": "nadie@acme.test"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("dirección inexistente: status=%d, quiero 404", rr.Code)
	}

	// El camino de VUELTA (identificador → correo) lleva la misma verja: si no,
	// pintar «compartido con quien sea» se convertiría en un lector del directorio
	// de cualquier organización a base de probar identificadores.
	rr, body = l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"user_id": admin.ID})
	if rr.Code != http.StatusOK || body["email"] != "admin@acme.test" {
		t.Fatalf("vuelta dentro de la org: status=%d body=%v", rr.Code, body)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"user_id": ajeno.ID})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("vuelta contra otra org: status=%d, quiero 404", rr.Code)
	}

	// Sin dirección válida no hay nada que resolver.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/directory:resolve", tok,
		map[string]string{"email": "no-arroba"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("correo inválido: status=%d, quiero 400", rr.Code)
	}
}
