// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del punto de DESPACHO: las tres decisiones que toma dispatch.go antes de que
// un driver toque la red. No hay store ni PostgreSQL — son funciones puras y la
// fabricación del Runtime, que no necesita ninguna dependencia poblada.
//
// Las tres deciden algo que no se nota hasta que falla:
//   - qué destinos pueden ser una IP privada (Internal), que es la diferencia entre
//     una pasarela de egress y un proxy SSRF abierto a nombre del cliente;
//   - qué secretos ve el driver, que es la garantía de que el token del vault no sale
//     de la pasarela;
//   - si un conector desactivado sigue hablando con el tercero.
package integrationshttp

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// TestEgressRuntime_soloLaSuiteEsInterna fija la línea de dispatch.go que decide si se
// omiten las DOS barreras SSRF: `Internal: inst.TemplateKey == suiteTemplateKey`.
//
// Se corre con la política de IP de PRODUCCIÓN, sin tocar el seam: precisamente lo que
// se comprueba es que la barrera está puesta para todo lo que no sea la propia suite.
// Si esa condición se relajara —a `true`, o a una plantilla que un administrador puede
// elegir—, cualquier organización podría hacer que la pasarela leyera por ella el
// servicio de metadatos de la nube o cualquier servicio interno del clúster, con
// auditoría y credenciales de la pasarela. Es el peor fallo posible de este servicio.
func TestEgressRuntime_soloLaSuiteEsInterna(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := &Handlers{}
	mudo := slog.New(slog.NewTextHandler(io.Discard, nil))

	// (1) Plantilla de un tercero: el destino de loopback lo pone la configuración de la
	// instancia, que la teclea un administrador. La barrera lo corta.
	for _, tpl := range []string{"jira", "salesforce", "odoo", ""} {
		rt := h.egressRuntime(connector.Instance{ID: "i", OrgID: "acme", TemplateKey: tpl}, mudo)
		resp, err := rt.HTTP.Get(srv.URL)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("plantilla %q: la pasarela alcanzó un destino privado; la barrera SSRF no está puesta", tpl)
		}
		if !errors.Is(err, ssrfhttp.ErrBlocked) {
			t.Fatalf("plantilla %q: err = %v; se esperaba ssrfhttp.ErrBlocked", tpl, err)
		}
	}

	// (2) Plantilla de la PROPIA suite: su destino sale de la configuración del servicio
	// (ID_URL/DRIVE_URL, ver loadInstance) y vive en una IP privada. Aquí la barrera se
	// omite a propósito, y si no se omitiera el conector interno no funcionaría.
	rt := h.egressRuntime(connector.Instance{ID: "i", OrgID: "acme", TemplateKey: suiteTemplateKey}, mudo)
	resp, err := rt.HTTP.Get(srv.URL)
	if err != nil {
		t.Fatalf("la plantilla de la suite debe poder hablar con un servicio interno: %v", err)
	}
	_ = resp.Body.Close()
}

// TestDriverSecrets_elBearerNoLlegaAlDriver: el secreto convencional `bearer` lo
// inyecta el Runtime como cabecera, y el driver NO debe recibirlo. Los demás sí: hay
// protocolos que llevan la credencial en el CUERPO (Odoo JSON-RPC) y no pueden usar la
// inyección en cabecera.
//
// Si `bearer` se colara, un driver podría reenviarlo a donde quisiera —incluido otro
// destino— y se acabaría la garantía de que el token del vault no sale de la pasarela.
func TestDriverSecrets_elBearerNoLlegaAlDriver(t *testing.T) {
	got := driverSecrets(map[string]string{
		store.SecretKeyBearer: "token-del-vault",
		"api_key":             "clave-de-odoo",
		"client_secret":       "secreto-del-client",
	})
	if _, hay := got[store.SecretKeyBearer]; hay {
		t.Fatalf("el driver recibió el %q del vault: %v", store.SecretKeyBearer, got)
	}
	for k, want := range map[string]string{"api_key": "clave-de-odoo", "client_secret": "secreto-del-client"} {
		if got[k] != want {
			t.Errorf("%s = %q, quiero %q: los secretos que van en el cuerpo sí se entregan", k, got[k], want)
		}
	}

	// Header-auth puro (OData): sin más secretos que el bearer, el driver recibe nil.
	if got := driverSecrets(map[string]string{store.SecretKeyBearer: "token-del-vault"}); got != nil {
		t.Errorf("sin otros secretos el driver debe recibir nil, no %v", got)
	}
	if got := driverSecrets(nil); got != nil {
		t.Errorf("sin secretos el driver debe recibir nil, no %v", got)
	}
}

// TestCapabilityEnabled: el interruptor por capacidad que ve el administrador en la
// consola. Si dejara de gatear, «desactivar el aprovisionamiento» seguiría dando de
// alta y de baja personas en el SaaS del cliente sin que nadie lo notara.
func TestCapabilityEnabled(t *testing.T) {
	casos := []struct {
		nombre    string
		habilitad []string
		capab     manifest.Capability
		quiero    bool
	}{
		{"vacío = todas (filas anteriores al interruptor)", nil, manifest.CapData, true},
		{"vacío explícito = todas", []string{}, manifest.CapProvision, true},
		{"la capacidad está en la lista", []string{"data"}, manifest.CapData, true},
		{"la capacidad NO está en la lista", []string{"data"}, manifest.CapProvision, false},
		{"varias, la pedida está", []string{"provisioning", "data"}, manifest.CapProvision, true},
		{"varias, la pedida no está", []string{"sso", "data"}, manifest.CapProvision, false},
	}
	for _, c := range casos {
		if got := capabilityEnabled(c.habilitad, c.capab); got != c.quiero {
			t.Errorf("%s: capabilityEnabled(%v, %q) = %v, quiero %v", c.nombre, c.habilitad, c.capab, got, c.quiero)
		}
	}
}
