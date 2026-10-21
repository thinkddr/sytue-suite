// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del cliente de Cloudflare contra un httptest que imita las respuestas
// REALES de la API v4 (sobre {success, errors, messages, result}), incluidos los
// fallos que de verdad ocurren: token sin permiso, zona inexistente, registro
// duplicado y dominio no disponible. Ni un test sale a la red.
package dns

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// newTestClient levanta un servidor con `h` y devuelve un cliente apuntado a él.
// httptest escucha en loopback, que la barrera SSRF bloquea en producción: se
// relaja SOLO durante el test con el hook oficial del paquete.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	t.Cleanup(restore)
	return New("cf-token-de-prueba", "acct-123", "zone-abc", srv.URL)
}

// ok escribe un sobre de éxito con `result` tal cual.
func ok(t *testing.T, w http.ResponseWriter, result string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":` + result + `}`))
}

// fail escribe un sobre de error como el que devuelve Cloudflare de verdad.
func fail(t *testing.T, w http.ResponseWriter, status, code int, msg string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]any{
		"success":  false,
		"messages": []any{},
		"result":   nil,
		"errors":   []map[string]any{{"code": code, "message": msg}},
	})
	_, _ = w.Write(body)
}

// --- Sin token: degradación honesta, no error de programa -------------------

func TestNewSinToken(t *testing.T) {
	if c := New("  ", "acct", "zone", ""); c != nil {
		t.Fatal("sin token el cliente debe ser nil (Cloudflare no configurado)")
	}
	var c *Client // nil: exactamente lo que guarda el Server cuando no hay token
	if c.HasZone() || c.HasAccount() {
		t.Fatal("un cliente nil no tiene ni zona ni cuenta")
	}
	if _, _, err := c.FindRecord(context.Background(), "acme.sytue.eu"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("FindRecord sin configurar = %v, quiero ErrNotConfigured", err)
	}
	if _, err := c.CreateRecord(context.Background(), Record{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CreateRecord sin configurar = %v, quiero ErrNotConfigured", err)
	}
	if _, err := c.CheckDomain(context.Background(), "acme.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CheckDomain sin configurar = %v, quiero ErrNotConfigured", err)
	}
}

func TestSinZonaNiCuenta(t *testing.T) {
	c := New("tok", "", "", "")
	if c.HasZone() || c.HasAccount() {
		t.Fatal("con token pero sin zona/cuenta no se puede operar")
	}
	if _, err := c.CheckDomain(context.Background(), "acme.com"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("CheckDomain sin cuenta = %v, quiero ErrNotConfigured", err)
	}
}

// --- Publicar el subdominio -------------------------------------------------

func TestCreateRecord(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody Record
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		ok(t, w, `{"id":"rec-1","type":"CNAME","name":"acme.sytue.eu",
			"content":"apps.sytue.eu","proxied":true,"ttl":1}`)
	})
	rec, err := c.CreateRecord(context.Background(), Record{
		Type: "CNAME", Name: "acme.sytue.eu", Content: "apps.sytue.eu", TTL: 1, Proxied: true,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if rec.ID != "rec-1" || rec.Name != "acme.sytue.eu" {
		t.Fatalf("registro devuelto inesperado: %+v", rec)
	}
	if gotPath != "/zones/zone-abc/dns_records" {
		t.Fatalf("ruta = %q", gotPath)
	}
	if gotAuth != "Bearer cf-token-de-prueba" {
		t.Fatalf("el token debe ir en la cabecera Authorization, no en otro sitio: %q", gotAuth)
	}
	if gotBody.Content != "apps.sytue.eu" || !gotBody.Proxied {
		t.Fatalf("cuerpo enviado inesperado: %+v", gotBody)
	}
}

// El registro duplicado NO es un fallo para nosotros: publicar es idempotente.
func TestCreateRecordYaExiste(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fail(t, w, http.StatusBadRequest, 81053, "An A, AAAA, or CNAME record with that host already exists.")
	})
	_, err := c.CreateRecord(context.Background(), Record{Type: "CNAME", Name: "acme.sytue.eu"})
	if err == nil {
		t.Fatal("Cloudflare devolvió error: el cliente debe propagarlo")
	}
	if !Exists(err) {
		t.Fatalf("Exists(err) debe ser true para el duplicado: %v", err)
	}
	if Denied(err) || Missing(err) {
		t.Fatalf("un duplicado no es ni permiso ni 404: %v", err)
	}
}

// Token sin «Zone · DNS · Edit»: es un fallo de CONFIGURACIÓN, se distingue.
func TestCreateRecordSinPermiso(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fail(t, w, http.StatusForbidden, 10000, "Authentication error")
	})
	_, err := c.CreateRecord(context.Background(), Record{Type: "CNAME", Name: "acme.sytue.eu"})
	if !Denied(err) {
		t.Fatalf("Denied(err) debe ser true con 403: %v", err)
	}
	if Exists(err) {
		t.Fatalf("un 403 no es un duplicado: %v", err)
	}
	if got := Detail(err); got != "Authentication error" {
		t.Fatalf("Detail = %q, quiero el mensaje real de Cloudflare", got)
	}
	if strings.Contains(err.Error(), "cf-token-de-prueba") {
		t.Fatal("el token NUNCA puede aparecer en un error")
	}
}

// Zona mal configurada (id que no existe): 404 con su propio predicado.
func TestFindRecordZonaInexistente(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fail(t, w, http.StatusNotFound, 7003, "Could not route to /zones/zone-abc/dns_records, perhaps your object identifier is invalid?")
	})
	_, _, err := c.FindRecord(context.Background(), "acme.sytue.eu")
	if !Missing(err) {
		t.Fatalf("Missing(err) debe ser true con 404: %v", err)
	}
	if Denied(err) {
		t.Fatalf("un 404 no es falta de permiso: %v", err)
	}
}

func TestFindRecord(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		ok(t, w, `[{"id":"rec-1","type":"CNAME","name":"acme.sytue.eu","content":"apps.sytue.eu","proxied":true}]`)
	})
	rec, found, err := c.FindRecord(context.Background(), "acme.sytue.eu")
	if err != nil || !found {
		t.Fatalf("FindRecord = (%+v, %v, %v)", rec, found, err)
	}
	if rec.ID != "rec-1" {
		t.Fatalf("registro inesperado: %+v", rec)
	}
	// El filtro exacto de la API v4 es name.exact (name suelto es el heredado).
	if !strings.Contains(gotQuery, "name.exact=acme.sytue.eu") {
		t.Fatalf("query = %q, quiero filtro name.exact", gotQuery)
	}
}

// EL CASO QUE COSTÓ UNA ORGANIZACIÓN SIN DIRECCIÓN. El subdominio de una
// organización con buzones lleva SPF (TXT) y MX. Preguntando por cualquier tipo,
// esos registros contestaban «ya está publicada» y `ensureTenantDNS` no creaba
// nunca la dirección: el alta decía `live` y el navegador no encontraba el
// servidor. Aquí solo cuentan CNAME, A y AAAA.
func TestFindRecordIgnoraCorreoYVerificaciones(t *testing.T) {
	var tiposPedidos []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		typ := r.URL.Query().Get("type")
		tiposPedidos = append(tiposPedidos, typ)
		// La zona SOLO tiene correo en ese nombre: ningún registro de dirección.
		switch typ {
		case "TXT":
			ok(t, w, `[{"id":"rec-spf","type":"TXT","name":"corporate.sytue.eu","content":"v=spf1 -all"}]`)
		case "MX":
			ok(t, w, `[{"id":"rec-mx","type":"MX","name":"corporate.sytue.eu","content":"mx.sytue.eu"}]`)
		default:
			ok(t, w, `[]`)
		}
	})
	_, found, err := c.FindRecord(context.Background(), "corporate.sytue.eu")
	if err != nil {
		t.Fatalf("FindRecord: %v", err)
	}
	if found {
		t.Fatal("un SPF y un MX NO publican una dirección: found debe ser false")
	}
	for _, typ := range tiposPedidos {
		if typ == "" {
			t.Fatal("se preguntó SIN filtro de tipo: eso es lo que traía el fallo de vuelta")
		}
	}
}

// Y una A sí cuenta: es lo que hay que publicar cuando el nombre ya lleva correo,
// porque un CNAME no puede convivir con MX ni TXT (RFC 1034/2181).
func TestFindRecordAceptaRegistroA(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") == "A" {
			ok(t, w, `[{"id":"rec-a","type":"A","name":"corporate.sytue.eu","content":"192.0.2.10"}]`)
			return
		}
		ok(t, w, `[]`)
	})
	rec, found, err := c.FindRecord(context.Background(), "corporate.sytue.eu")
	if err != nil || !found {
		t.Fatalf("FindRecord = (%+v, %v, %v)", rec, found, err)
	}
	if rec.Type != "A" {
		t.Fatalf("registro inesperado: %+v", rec)
	}
}

// «No existe» no es un error: es la respuesta a la pregunta.
func TestFindRecordNoEsta(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { ok(t, w, `[]`) })
	_, found, err := c.FindRecord(context.Background(), "acme.sytue.eu")
	if err != nil {
		t.Fatalf("una lista vacía no es un error: %v", err)
	}
	if found {
		t.Fatal("found debe ser false con lista vacía")
	}
}

// --- Registrar: disponibilidad y precio -------------------------------------

func TestCheckDomainDisponibleConPrecio(t *testing.T) {
	var gotPath string
	var gotBody map[string][]string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		ok(t, w, `{"domains":[{"name":"xq7mz9brand.com","registrable":true,"tier":"standard",
			"pricing":{"currency":"USD","registration_cost":"8.57","renewal_cost":"8.57"}}]}`)
	})
	av, err := c.CheckDomain(context.Background(), "xq7mz9brand.com")
	if err != nil {
		t.Fatalf("CheckDomain: %v", err)
	}
	if !av.Registrable || av.Pricing == nil {
		t.Fatalf("disponibilidad inesperada: %+v", av)
	}
	if av.Pricing.RegistrationCost != "8.57" || av.Pricing.Currency != "USD" {
		t.Fatalf("el precio debe salir tal cual de Cloudflare: %+v", av.Pricing)
	}
	if gotPath != "/accounts/acct-123/registrar/domain-check" {
		t.Fatalf("ruta = %q", gotPath)
	}
	if len(gotBody["domains"]) != 1 || gotBody["domains"][0] != "xq7mz9brand.com" {
		t.Fatalf("cuerpo enviado inesperado: %+v", gotBody)
	}
}

func TestCheckDomainNoDisponible(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ok(t, w, `{"domains":[{"name":"acme.com","registrable":false,"reason":"domain_unavailable"}]}`)
	})
	av, err := c.CheckDomain(context.Background(), "acme.com")
	if err != nil {
		t.Fatalf("un dominio ocupado no es un error de API: %v", err)
	}
	if av.Registrable || av.Reason != "domain_unavailable" {
		t.Fatalf("disponibilidad inesperada: %+v", av)
	}
	if av.Pricing != nil {
		t.Fatalf("sin precio cuando no se puede registrar: %+v", av.Pricing)
	}
}

func TestCheckDomainExtensionNoSoportada(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ok(t, w, `{"domains":[{"name":"mybrand.uk","registrable":false,"reason":"extension_not_supported_via_api"}]}`)
	})
	av, err := c.CheckDomain(context.Background(), "mybrand.uk")
	if err != nil {
		t.Fatalf("CheckDomain: %v", err)
	}
	if av.Reason != "extension_not_supported_via_api" {
		t.Fatalf("reason = %q", av.Reason)
	}
}

func TestCheckDomainRespuestaVacia(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { ok(t, w, `{"domains":[]}`) })
	if _, err := c.CheckDomain(context.Background(), "acme.com"); err == nil {
		t.Fatal("una respuesta sin dominios no puede pasar por buena")
	}
}

// --- El cliente NO registra dominios (no hay compra) ------------------------

// La API de Cloudflare permite comprar (POST /accounts/{id}/registrar/registrations),
// pero cobraría a la cuenta de plataforma de Sytue. Este test es el candado: si
// alguien añade una compra, aquí se entera de que hace falta cobrarle al cliente.
func TestSinCompraDeDominios(t *testing.T) {
	llamadas := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		llamadas++
		if strings.Contains(r.URL.Path, "/registrar/registrations") {
			t.Errorf("¡NADIE debe llamar al endpoint de registro! %s %s", r.Method, r.URL.Path)
		}
		ok(t, w, `{"domains":[{"name":"acme.com","registrable":true,
			"pricing":{"currency":"USD","registration_cost":"8.57","renewal_cost":"8.57"}}]}`)
	})
	if _, err := c.CheckDomain(context.Background(), "acme.com"); err != nil {
		t.Fatalf("CheckDomain: %v", err)
	}
	if llamadas != 1 {
		t.Fatalf("consultar el precio son %d llamadas, debe ser 1 (y de solo lectura)", llamadas)
	}
}

// --- Robustez del sobre -----------------------------------------------------

func TestRespuestaIlegible(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	})
	_, _, err := c.FindRecord(context.Background(), "acme.sytue.eu")
	if err == nil {
		t.Fatal("una respuesta que no es JSON debe ser un error")
	}
	if Exists(err) || Denied(err) {
		t.Fatalf("no se puede deducir nada de un cuerpo ilegible: %v", err)
	}
}

// success:false con 200 (Cloudflare lo hace) también es un error.
func TestExitoFalsoCon200(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1004,"message":"DNS Validation Error"}],"result":null}`))
	})
	if _, err := c.CreateRecord(context.Background(), Record{Type: "CNAME"}); err == nil {
		t.Fatal("success:false debe ser error aunque el HTTP sea 200")
	} else if Detail(err) != "DNS Validation Error" {
		t.Fatalf("Detail = %q", Detail(err))
	}
}
