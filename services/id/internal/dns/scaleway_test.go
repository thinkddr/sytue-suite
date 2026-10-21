// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dns

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// newTestScaleway levanta un servidor con `h` y devuelve un publicador apuntado
// a él. httptest escucha en loopback, que la barrera SSRF bloquea en producción:
// se relaja SOLO durante el test, con el hook oficial del paquete.
func newTestScaleway(t *testing.T, clave string, h http.HandlerFunc) *Scaleway {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	t.Cleanup(restore)
	return NewScaleway(clave, "sytue.eu", srv.URL)
}

// Scaleway nombra los registros en formato CORTO y el resto del código trabaja
// con el FQDN. Esa traducción es lo único que puede publicar el subdominio en el
// sitio equivocado —`acme.sytue.eu.sytue.eu`— sin que nada falle a la vista.
func TestScalewayCreaConNombreCortoYTTLUsable(t *testing.T) {
	var visto struct {
		method, path, token string
		body                map[string]any
	}
	c := newTestScaleway(t, "clave", func(w http.ResponseWriter, r *http.Request) {
		visto.method, visto.path = r.Method, r.URL.Path
		visto.token = r.Header.Get("X-Auth-Token")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &visto.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[]}`))
	})
	if !c.HasZone() {
		t.Fatal("HasZone=false con clave y zona")
	}
	if _, err := c.CreateRecord(context.Background(), Record{
		Type: "CNAME", Name: "acme.sytue.eu", Content: "apps.sytue.eu",
		// El 1 de Cloudflare significa «automático»; en Scaleway sería UN segundo.
		TTL: 1, Proxied: true,
	}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if visto.method != http.MethodPatch || visto.path != "/dns-zones/sytue.eu/records" {
		t.Fatalf("petición=%s %s", visto.method, visto.path)
	}
	if visto.token != "clave" {
		t.Fatalf("token=%q", visto.token)
	}
	cambios, _ := visto.body["changes"].([]any)
	if len(cambios) != 1 {
		t.Fatalf("changes=%v", visto.body["changes"])
	}
	add, _ := cambios[0].(map[string]any)["add"].(map[string]any)
	recs, _ := add["records"].([]any)
	if len(recs) != 1 {
		t.Fatalf("records=%v", add["records"])
	}
	rec, _ := recs[0].(map[string]any)
	if rec["name"] != "acme" {
		t.Fatalf("name=%v; quería el nombre corto «acme»", rec["name"])
	}
	if rec["data"] != "apps.sytue.eu" {
		t.Fatalf("data=%v (Scaleway lo llama data, no content)", rec["data"])
	}
	if ttl, _ := rec["ttl"].(float64); ttl != 3600 {
		t.Fatalf("ttl=%v; el 1 de Cloudflare no puede viajar tal cual", rec["ttl"])
	}
}

// Buscar tiene que distinguir el registro pedido de cualquier otro de la zona:
// quedarse con el primero publicaría un «ya existe» falso sobre el subdominio de
// otra organización, y esa nunca se crearía.
func TestScalewayFindRecordNoSeQuedaConElPrimero(t *testing.T) {
	c := newTestScaleway(t, "clave", func(w http.ResponseWriter, r *http.Request) {
		// El filtro se pide igualmente, en corto: si la API lo respeta, la
		// respuesta es de una fila en vez de la zona entera.
		if n := r.URL.Query().Get("name"); n != "acme" && n != "nadie" {
			t.Errorf("filtro name=%q", n)
		}
		w.Header().Set("Content-Type", "application/json")
		// Como si la API lo ignorara y devolviera la zona entera.
		_, _ = w.Write([]byte(`{"records":[
			{"id":"1","name":"otra","type":"CNAME","data":"apps.sytue.eu","ttl":3600},
			{"id":"2","name":"acme","type":"CNAME","data":"apps.sytue.eu","ttl":3600}]}`))
	})
	rec, ok, err := c.FindRecord(context.Background(), "acme.sytue.eu")
	if err != nil || !ok {
		t.Fatalf("FindRecord: ok=%v err=%v", ok, err)
	}
	if rec.ID != "2" || rec.Name != "acme.sytue.eu" || rec.Content != "apps.sytue.eu" {
		t.Fatalf("registro=%+v", rec)
	}

	// Y el que no está NO se inventa.
	rec, ok, err = c.FindRecord(context.Background(), "nadie.sytue.eu")
	if err != nil {
		t.Fatalf("FindRecord(nadie): %v", err)
	}
	if ok {
		t.Fatalf("encontró un registro que no está: %+v", rec)
	}
}

// Sin clave o sin zona no hay publicador: «este despliegue no publica en
// Scaleway» es un estado válido, no un fallo, y el alta tiene que seguir.
func TestScalewaySinConfigurar(t *testing.T) {
	for _, c := range []struct{ clave, zona string }{{"", ""}, {"clave", ""}, {"", "sytue.eu"}} {
		cl := NewScaleway(c.clave, c.zona, "")
		if cl.HasZone() {
			t.Fatalf("clave=%q zona=%q: HasZone=true", c.clave, c.zona)
		}
		if _, _, err := cl.FindRecord(context.Background(), "acme.sytue.eu"); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("FindRecord sin configurar: %v", err)
		}
		if _, err := cl.CreateRecord(context.Background(), Record{Name: "acme.sytue.eu"}); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("CreateRecord sin configurar: %v", err)
		}
	}
}

// Un conflicto es «ya estaba», que para publicar un subdominio es éxito. Sin
// esto, un segundo intento (otra réplica, o el alta retomada) se leería como
// fallo y la pantalla diría que la dirección no se pudo publicar.
func TestScalewayConflictoEsExiste(t *testing.T) {
	c := newTestScaleway(t, "clave", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"record already exists","type":"conflict"}`))
	})
	_, err := c.CreateRecord(context.Background(), Record{
		Type: "CNAME", Name: "acme.sytue.eu", Content: "apps.sytue.eu",
	})
	if !Exists(err) {
		t.Fatalf("Exists(%v)=false", err)
	}
	if Denied(err) || Missing(err) {
		t.Fatalf("un conflicto no es ni denegado ni ausente: %v", err)
	}
}

// El mensaje del proveedor llega al operador; la CLAVE no viaja en ningún error.
func TestScalewayErrorLlevaElMensajeYNoLaClave(t *testing.T) {
	c := newTestScaleway(t, "clave-secreta", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"insufficient permissions","type":"permissions_denied"}`))
	})
	_, err := c.CreateRecord(context.Background(), Record{Type: "CNAME", Name: "acme.sytue.eu"})
	if !Denied(err) {
		t.Fatalf("Denied(%v)=false", err)
	}
	if d := Detail(err); !strings.Contains(d, "insufficient permissions") {
		t.Fatalf("Detail=%q", d)
	}
	if strings.Contains(err.Error(), "clave-secreta") {
		t.Fatal("la clave viaja dentro del error")
	}
}
