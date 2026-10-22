// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la parte del cliente que habla con la cuenta de Cloudflare DEL
// CLIENTE: localizar su zona, leer sus registros y sustituir uno con permiso.
// Sin red: un httptest imita la API v4.
//
// Lo que más se comprueba aquí es la DISTINCIÓN entre los dos fracasos que el
// usuario confunde y que exigen acciones distintas: «tu token no vale» y «tu
// token vale pero ese dominio no está en esa cuenta».
package dns

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- Localizar la zona ------------------------------------------------------

func TestFindZoneEncontrada(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		ok(t, w, `[{"id":"zone-cliente","name":"acme.com","status":"active"}]`)
	}).ForCustomer("tok-del-cliente", "")

	zone, found, err := c.FindZone(context.Background(), "acme.com")
	if err != nil || !found {
		t.Fatalf("FindZone = (%+v, %v, %v)", zone, found, err)
	}
	if zone.ID != "zone-cliente" || zone.Name != "acme.com" || zone.Status != "active" {
		t.Fatalf("zona inesperada: %+v", zone)
	}
	if gotPath != "/zones" || !strings.Contains(gotQuery, "name=acme.com") {
		t.Fatalf("petición = %s?%s", gotPath, gotQuery)
	}
	// Va el token DEL CLIENTE, no el de plataforma.
	if gotAuth != "Bearer tok-del-cliente" {
		t.Fatalf("authorization = %q; debe ir el token del cliente", gotAuth)
	}
}

// El token vale pero la zona no está a su alcance: NO es un error de API, es la
// respuesta a la pregunta, y se distingue de la falta de permiso.
func TestFindZoneNoEstaEnEsaCuenta(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { ok(t, w, `[]`) }).
		ForCustomer("tok-del-cliente", "")
	zone, found, err := c.FindZone(context.Background(), "acme.com")
	if err != nil {
		t.Fatalf("una lista vacía no es un error: %v", err)
	}
	if found || zone.ID != "" {
		t.Fatalf("found = %v, zona = %+v", found, zone)
	}
}

// Token sin «Zone · DNS · Edit» (o caducado): error con Denied(err) true, que es
// lo que separa este caso del anterior de cara al mensaje que lee el usuario.
func TestFindZoneTokenSinPermiso(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fail(t, w, http.StatusForbidden, 9109, "Unauthorized to access requested resource")
	}).ForCustomer("tok-del-cliente", "")

	_, found, err := c.FindZone(context.Background(), "acme.com")
	if !Denied(err) {
		t.Fatalf("Denied(err) debe ser true con 403: %v", err)
	}
	if found {
		t.Fatal("con un 403 no se ha encontrado nada")
	}
	if strings.Contains(err.Error(), "tok-del-cliente") {
		t.Fatalf("el token del cliente NUNCA puede aparecer en un error: %v", err)
	}
}

// --- ForCustomer: credenciales separadas ------------------------------------

// El cliente del cliente NO hereda el token ni la cuenta de plataforma: si lo
// hiciera, un token nuestro acabaría operando sobre la zona de un tercero (o al
// revés, un token ajeno sobre la nuestra).
func TestForCustomerNoHeredaCredencialesDePlataforma(t *testing.T) {
	var gotAuth, gotPath string
	plat := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		ok(t, w, `[]`)
	})
	cust := plat.ForCustomer("tok-del-cliente", "zone-cliente")
	if _, err := cust.ListRecords(context.Background(), "acme.com", "MX"); err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if gotAuth != "Bearer tok-del-cliente" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotPath != "/zones/zone-cliente/dns_records" {
		t.Fatalf("ruta = %q; debe ser la zona del cliente, no la nuestra", gotPath)
	}
	if cust.HasAccount() {
		t.Fatal("la cuenta de plataforma (Registrar) no se hereda al cliente")
	}
	// Y el cliente de plataforma sigue intacto.
	if !plat.HasZone() || !plat.HasAccount() {
		t.Fatal("ForCustomer no puede mutar el cliente de plataforma")
	}
}

// Sytue puede no tener Cloudflare configurado y el cliente traer el suyo: sobre
// un receptor nil, ForCustomer tiene que seguir devolviendo un cliente usable.
func TestForCustomerSobreClienteNil(t *testing.T) {
	var nilClient *Client
	c := nilClient.ForCustomer("tok-del-cliente", "zone-cliente")
	if c == nil || !c.HasZone() {
		t.Fatal("sin Cloudflare de plataforma, el token del cliente debe bastar")
	}
}

// --- Leer y sustituir registros ---------------------------------------------

func TestListRecordsFiltraPorTipo(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		ok(t, w, `[{"id":"r1","type":"TXT","name":"acme.com","content":"\"v=spf1 -all\""},
			{"id":"r2","type":"TXT","name":"acme.com","content":"google-site-verification=x"}]`)
	}).ForCustomer("tok", "zone-cliente")

	recs, err := c.ListRecords(context.Background(), "acme.com", "TXT")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(recs) != 2 || recs[1].ID != "r2" {
		t.Fatalf("registros = %+v; se devuelven TODOS los del hueco, no solo el primero", recs)
	}
	if !strings.Contains(gotQuery, "name.exact=acme.com") || !strings.Contains(gotQuery, "type=TXT") {
		t.Fatalf("query = %q; hace falta name.exact + type", gotQuery)
	}
}

func TestUpdateRecordSustituye(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody Record
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		ok(t, w, `{"id":"r1","type":"MX","name":"acme.com","content":"mx.sytue.eu","priority":10}`)
	}).ForCustomer("tok", "zone-cliente")

	out, err := c.UpdateRecord(context.Background(), "r1",
		Record{Type: "MX", Name: "acme.com", Content: "mx.sytue.eu", Priority: 10, TTL: 1})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if out.ID != "r1" || out.Priority != 10 {
		t.Fatalf("registro devuelto: %+v", out)
	}
	if gotMethod != http.MethodPut || gotPath != "/zones/zone-cliente/dns_records/r1" {
		t.Fatalf("%s %s; sustituir es un PUT sobre el id", gotMethod, gotPath)
	}
	if gotBody.Priority != 10 || gotBody.Content != "mx.sytue.eu" {
		t.Fatalf("cuerpo enviado: %+v", gotBody)
	}
}

// `proxied` solo existe en A/AAAA/CNAME: Cloudflare rechaza el campo en TXT y MX,
// así que no puede viajar cuando vale false.
func TestRegistroSinProxyNoEnviaElCampo(t *testing.T) {
	var raw map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		ok(t, w, `{"id":"r1","type":"TXT","name":"_sytue.acme.com","content":"x"}`)
	}).ForCustomer("tok", "zone-cliente")

	if _, err := c.CreateRecord(context.Background(),
		Record{Type: "TXT", Name: "_sytue.acme.com", Content: "x", TTL: 1}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if _, hay := raw["proxied"]; hay {
		t.Fatalf("un TXT no puede llevar `proxied`: %v", raw)
	}
	if _, hay := raw["priority"]; hay {
		t.Fatalf("solo MX lleva `priority`: %v", raw)
	}
}
