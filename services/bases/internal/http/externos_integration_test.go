// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Una persona de OTRA organización entra en una base compartida con su propio token (que
// trae SU organización): el servicio resuelve la anfitriona y la ACL de la base decide.
func TestCompartirConOtraOrganizacionPorHTTP(t *testing.T) {
	p := &pega{}
	srv := servidorCon(t, p.montar(t))
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"T"}`))
	reg := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{}}`))
	otra := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Nóminas"}`))
	tOtra := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+otra+"/tables", `{"name":"T"}`))
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+otra+"/permissions", `{"principal_kind":"tenant","principal_id":"acme","role":"editor"}`)
	// En acme hay además un grupo «ventas» con edición: gus trae un grupo con ese nombre de SU organización.
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"group","principal_id":"ventas","role":"editor"}`)
	perms := "/bases/" + base + "/permissions"
	registros := "/tables/" + tabla + "/records"

	// Sin invitación: 404, también diciendo a mano que la organización es otra.
	if c := llamarCon(t, srv, "gus", "globex", "ventas", "GET", registros, "").Code; c != 404 {
		t.Fatalf("sin invitación: %d", c)
	}
	// Ni siquiera hacia lo que cualquiera de acme SÍ podría leer (Nóminas, abierta a toda la
	// organización, y la base del grupo «ventas»): la organización no la dice quien llama.
	for _, cabecera := range []string{"X-Sytue-Org", "X-Sytue-Host-Org", "X-Tenant", "X-Forwarded-Host"} {
		for _, ruta := range []string{registros, "/tables/" + tOtra + "/records"} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1"+ruta+"?tenant=acme&org=acme&host_org=acme", http.NoBody)
			req.Header.Set("X-Test-Sub", "gus")
			req.Header.Set("X-Test-Tenant", "globex")
			req.Header.Set("X-Test-Groups", "ventas")
			req.Header.Set(cabecera, "acme")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != 404 {
				t.Errorf("con %s: acme el servicio cambió de organización en %s: %d", cabecera, ruta, w.Code)
			}
		}
	}

	// Invitar: solo por el directorio (un @usuario o una dirección), nunca con un sub pegado.
	for _, cuerpo := range []string{
		`{"principal_kind":"external","principal_id":"gus","role":"viewer"}`,
		`{"principal_kind":"external","principal_id":"gus","query":"@gus","role":"viewer"}`,
		`{"principal_kind":"user","query":"@gus","role":"viewer"}`,
		`{"principal_kind":"external","email":"gus@globex.es","role":"viewer"}`,
		`{"principal_kind":"external","query":"@gus","role":"owner"}`,
	} {
		if c := llamar(t, srv, "ana", "acme", "POST", perms, cuerpo).Code; c != 422 {
			t.Errorf("cuerpo %s: %d", cuerpo, c)
		}
	}
	if w := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"external","query":"@nadie","role":"viewer"}`); w.Code != 422 || !strings.Contains(w.Body.String(), "not_a_member") {
		t.Errorf("invitar a quien no existe: %d %s", w.Code, w.Body)
	}
	// Alguien de la propia organización no es «de fuera».
	if w := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"external","query":"bea@acme.es","role":"viewer"}`); w.Code != 422 || !strings.Contains(w.Body.String(), "is_member") {
		t.Errorf("invitar de fuera a alguien de dentro: %d %s", w.Code, w.Body)
	}
	// Quien no es propietaria no llega al directorio.
	p.mu.Lock()
	antes := p.consultas
	p.mu.Unlock()
	llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"user","principal_id":"bea","role":"editor"}`)
	if c := llamar(t, srv, "bea", "acme", "POST", perms, `{"principal_kind":"external","query":"@hal","role":"editor"}`).Code; c != 403 {
		t.Errorf("una editora invita: %d", c)
	}
	p.mu.Lock()
	if p.consultas != antes {
		t.Errorf("se consultó el directorio %d veces sin ser propietaria", p.consultas-antes)
	}
	p.mu.Unlock()

	w := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"external","query":"@gus","role":"commenter"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"principal_id":"gus"`) || !strings.Contains(w.Body.String(), `"principal_label":"N. gus"`) {
		t.Fatalf("invitar: %d %s", w.Code, w.Body)
	}
	permiso := idDeRespuesta(t, w)

	// gus entra con SU token: lee y comenta (comentarista); su grupo «ventas» de globex no le da la edición del de acme.
	if c := llamarCon(t, srv, "gus", "globex", "ventas", "GET", registros, "").Code; c != 200 {
		t.Fatalf("el invitado lee: %d", c)
	}
	if w := llamar(t, srv, "gus", "globex", "GET", "/bases/"+base, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"commenter"`) {
		t.Errorf("el invitado abre la base con su rol: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, srv, "gus", "globex", "POST", "/records/"+reg+"/comments", `{"body":"hola desde fuera"}`).Code; c != 201 {
		t.Errorf("el invitado comenta: %d", c)
	}
	if c := llamarCon(t, srv, "gus", "globex", "ventas", "POST", registros, `{"data":{}}`).Code; c != 403 {
		t.Errorf("el invitado comentarista escribe (¿por un grupo de la anfitriona?): %d", c)
	}
	for _, x := range [][3]string{
		{"GET", perms, ""},
		{"GET", "/bases/" + base + "/audit", ""},
		{"GET", "/bases/" + base + "/connections", ""},
		{"POST", "/bases/" + base + "/transfer", `{"email":"gus@globex.es"}`},
		{"DELETE", "/bases/" + base, ""},
		{"POST", "/tables/" + tabla + "/fields", `{"name":"X","kind":"text"}`},
	} {
		if c := llamar(t, srv, "gus", "globex", x[0], x[1], x[2]).Code; c != 403 {
			t.Errorf("el invitado en %s %s: %d", x[0], x[1], c)
		}
	}
	// Nada más de la organización anfitriona: ni lo abierto a «toda la organización».
	for _, ruta := range []string{"/bases/" + otra, "/bases/" + otra + "/tables", "/tables/" + tOtra + "/records"} {
		if c := llamar(t, srv, "gus", "globex", "GET", ruta, "").Code; c != 404 {
			t.Errorf("el invitado ve %s de la anfitriona: %d", ruta, c)
		}
	}
	// En su lista: la base compartida, con la anfitriona. Y solo esa.
	if w := llamar(t, srv, "gus", "globex", "GET", "/bases", ""); !strings.Contains(w.Body.String(), `"host_org":"acme"`) || strings.Contains(w.Body.String(), otra) {
		t.Errorf("lista del invitado: %s", w.Body)
	}
	// hal, compañero de gus sin invitación, sigue fuera.
	if c := llamar(t, srv, "hal", "globex", "GET", registros, "").Code; c != 404 {
		t.Errorf("otro de globex sin invitación: %d", c)
	}
	// La propietaria ve al invitado por su nombre en la lista de permisos.
	if w := llamar(t, srv, "ana", "acme", "GET", perms, ""); !strings.Contains(w.Body.String(), `"principal_kind":"external"`) || !strings.Contains(w.Body.String(), "N. gus") {
		t.Errorf("la lista de permisos no enseña al invitado: %s", w.Body)
	}

	// Revocar corta en el acto.
	if c := llamar(t, srv, "ana", "acme", "DELETE", perms+"/"+permiso, "").Code; c != 204 {
		t.Fatalf("revocar: %d", c)
	}
	if c := llamar(t, srv, "gus", "globex", "GET", registros, "").Code; c != 404 {
		t.Errorf("tras revocar: %d", c)
	}
	if c := llamar(t, srv, "gus", "globex", "POST", "/records/"+reg+"/comments", `{"body":"sigo aquí"}`).Code; c != 404 {
		t.Errorf("tras revocar, comentar: %d", c)
	}
}
