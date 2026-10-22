// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"strings"
	"testing"
)

func TestPermisosPorPersonaYGrupoPorHTTP(t *testing.T) {
	p := &pega{}
	srv := servidorCon(t, p.montar(t))
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"T"}`))
	perms := "/bases/" + base + "/permissions"

	// Una persona por su dirección: el servicio la resuelve en el directorio y guarda cómo se llama.
	w := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"user","email":" Bea@acme.es ","role":"viewer"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"principal_id":"bea"`) || !strings.Contains(w.Body.String(), `"principal_label":"N. bea"`) {
		t.Fatalf("conceder por dirección: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, srv, "bea", "acme", "GET", "/tables/"+tabla+"/records", "").Code; c != 200 {
		t.Errorf("la persona concedida lee: %d", c)
	}
	// Alguien que no es de la organización no entra en la ACL por su dirección.
	if w := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"user","email":"eva@otra.es","role":"viewer"}`); w.Code != 422 || !strings.Contains(w.Body.String(), "not_a_member") {
		t.Errorf("dirección de fuera: %d %s", w.Code, w.Body)
	}
	// Dirección y sub a la vez, o dirección para lo que no es una persona: no.
	for _, cuerpo := range []string{
		`{"principal_kind":"user","principal_id":"cleo","email":"bea@acme.es","role":"viewer"}`,
		`{"principal_kind":"group","email":"bea@acme.es","role":"viewer"}`,
		`{"principal_kind":"tenant","email":"bea@acme.es","role":"viewer"}`,
	} {
		if c := llamar(t, srv, "ana", "acme", "POST", perms, cuerpo).Code; c != 422 {
			t.Errorf("cuerpo %s: %d", cuerpo, c)
		}
	}
	// Quien no gestiona los permisos no usa el directorio de oráculo: ni llega a preguntarle.
	p.mu.Lock()
	antes := p.consultas
	p.mu.Unlock()
	if c := llamar(t, srv, "bea", "acme", "POST", perms, `{"principal_kind":"user","email":"cleo@acme.es","role":"editor"}`).Code; c != 403 {
		t.Errorf("una lectora concede: %d", c)
	}
	if c := llamar(t, srv, "gus", "globex", "POST", perms, `{"principal_kind":"user","email":"cleo@acme.es","role":"editor"}`).Code; c != 404 {
		t.Errorf("otra organización concede: %d", c)
	}
	p.mu.Lock()
	if p.consultas != antes {
		t.Errorf("se consultó el directorio %d veces sin permiso", p.consultas-antes)
	}
	p.mu.Unlock()

	// Un grupo: aplica a quien lo trae en el token, y solo en esta organización.
	if c := llamar(t, srv, "ana", "acme", "POST", perms, `{"principal_kind":"group","principal_id":"ventas","role":"editor"}`).Code; c != 201 {
		t.Fatalf("conceder a un grupo: %d", c)
	}
	if c := llamarCon(t, srv, "cleo", "acme", "soporte,ventas", "POST", "/tables/"+tabla+"/records", `{"data":{}}`).Code; c != 201 {
		t.Errorf("miembro del grupo escribe: %d", c)
	}
	if c := llamar(t, srv, "cleo", "acme", "GET", "/tables/"+tabla+"/records", "").Code; c != 404 {
		t.Errorf("sin el grupo en el token: %d", c)
	}
	if c := llamarCon(t, srv, "gus", "globex", "ventas", "GET", "/tables/"+tabla+"/records", "").Code; c != 404 {
		t.Errorf("el mismo grupo en otra organización: %d", c)
	}
	// El BFF no puede colar grupos por el cuerpo ni por la consulta: salen del token.
	if c := llamar(t, srv, "cleo", "acme", "GET", "/tables/"+tabla+"/records?groups=ventas", "").Code; c != 404 {
		t.Errorf("grupos por la consulta: %d", c)
	}
	if w := llamarCon(t, srv, "cleo", "acme", "soporte,ventas", "GET", "/groups", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `["soporte","ventas"]`) {
		t.Errorf("mis grupos: %d %s", w.Code, w.Body)
	}
	if w := llamar(t, srv, "cleo", "acme", "GET", "/groups", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"groups":[]`) {
		t.Errorf("sin grupos: %d %s", w.Code, w.Body)
	}
}

func TestTransferirPorHTTP(t *testing.T) {
	p := &pega{}
	srv := servidorCon(t, p.montar(t))
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	ruta := "/bases/" + base + "/transfer"
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"editor"}`)

	// El destino solo se dice por su dirección: un `sub` del cliente no se acepta.
	for _, cuerpo := range []string{`{"to_sub":"bea"}`, `{"owner_sub":"bea"}`, `{"email":"bea@acme.es","sub":"cleo"}`, `{}`, `{"email":""}`} {
		if c := llamar(t, srv, "ana", "acme", "POST", ruta, cuerpo).Code; c != 400 && c != 422 {
			t.Errorf("cuerpo %s: %d", cuerpo, c)
		}
	}
	// Ni a alguien de fuera, ni a una misma; ni lo hace quien no es propietaria.
	if w := llamar(t, srv, "ana", "acme", "POST", ruta, `{"email":"eva@otra.es"}`); w.Code != 422 || !strings.Contains(w.Body.String(), "not_a_member") {
		t.Errorf("transferir a alguien de fuera: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, srv, "ana", "acme", "POST", ruta, `{"email":"ana@acme.es"}`).Code; c != 422 {
		t.Errorf("transferirse a una misma: %d", c)
	}
	p.mu.Lock()
	antes := p.consultas
	p.mu.Unlock()
	if c := llamar(t, srv, "bea", "acme", "POST", ruta, `{"email":"bea@acme.es"}`).Code; c != 403 {
		t.Errorf("una editora se la queda: %d", c)
	}
	if c := llamar(t, srv, "cleo", "acme", "POST", ruta, `{"email":"cleo@acme.es"}`).Code; c != 404 {
		t.Errorf("sin acceso: %d", c)
	}
	if c := llamar(t, srv, "gus", "globex", "POST", ruta, `{"email":"gus@globex.es"}`).Code; c != 404 {
		t.Errorf("otra organización: %d", c)
	}
	p.mu.Lock()
	if p.consultas != antes {
		t.Errorf("se consultó el directorio %d veces sin ser propietaria", p.consultas-antes)
	}
	p.mu.Unlock()
	if w := llamar(t, srv, "ana", "acme", "GET", "/bases/"+base, ""); !strings.Contains(w.Body.String(), `"owner_sub":"ana"`) {
		t.Fatalf("un intento fallido cambió la propiedad: %s", w.Body)
	}
	p.tomar(t, 0)

	if w := llamar(t, srv, "ana", "acme", "POST", ruta, `{"email":"bea@acme.es"}`); w.Code != 204 {
		t.Fatalf("transferir: %d %s", w.Code, w.Body)
	}
	as := p.tomar(t, 1)
	if len(as) != 1 || as[0].Type != "bases.transferred" || as[0].Subject != "bea" || as[0].Params["actor"] != "ana" ||
		as[0].URL != "https://bases.sytue.eu/bases/acme/base/"+base {
		t.Errorf("aviso de la transferencia: %+v", as)
	}
	if w := llamar(t, srv, "bea", "acme", "GET", "/bases/"+base, ""); !strings.Contains(w.Body.String(), `"role":"owner"`) {
		t.Errorf("la nueva propietaria: %s", w.Body)
	}
	if c := llamar(t, srv, "ana", "acme", "GET", "/bases/"+base+"/permissions", "").Code; c != 403 {
		t.Errorf("la antigua propietaria ya no gestiona permisos: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "POST", ruta, `{"email":"cleo@acme.es"}`).Code; c != 403 {
		t.Errorf("la antigua propietaria vuelve a transferir: %d", c)
	}
	if w := llamar(t, srv, "bea", "acme", "GET", "/bases/"+base+"/audit", ""); !strings.Contains(w.Body.String(), `"action":"base.transfer"`) {
		t.Errorf("la transferencia no está en la traza: %s", w.Body)
	}
}
