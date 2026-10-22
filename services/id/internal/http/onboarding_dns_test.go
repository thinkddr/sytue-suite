// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la parte Cloudflare del alta inicial, SIN red y SIN base de datos:
// un httptest imita la API v4 de Cloudflare y se comprueba que el estado que
// acaba viendo el asistente es el REAL —incluido el camino de degradación
// cuando Sytue no tiene token configurado, que no puede fingir éxito ni romper
// el alta—. La publicación con auditoría (que sí toca BD) va en el test de
// integración.
package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/id/internal/dns"
)

// dnsTestServer monta un Server mínimo (sin store: estas rutas no lo tocan)
// apuntado a un Cloudflare de mentira. `token` vacío ⇒ Cloudflare no configurado.
func dnsTestServer(t *testing.T, token string, h http.HandlerFunc) *Server {
	t.Helper()
	base := ""
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
		base = srv.URL
	}
	// El cliente va a los DOS campos, igual que hace el constructor real
	// (`s.zoneDNS = s.cf` en server.go): `ensureTenantDNS` publica por `zoneDNS`
	// —la interfaz que abstrae Cloudflare de Scaleway— y construir el Server a
	// mano dejaba ese campo a nil, así que TODOS estos casos salían
	// «unconfigured» sin tocar la lógica que dicen probar.
	cl := dns.New(token, "acct-123", "zone-abc", base)
	srv := &Server{
		cfg: Config{
			Issuer:           "https://id.sytue.test",
			TenantDNSTarget:  "apps.sytue.test",
			TenantDNSProxied: true,
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		cf:  cl,
	}
	// Nil explícito y no `srv.zoneDNS = cl`: un *dns.Client nil metido en la
	// interfaz da un valor NO nil, y `ensureTenantDNS` comprueba `== nil`.
	if cl != nil {
		srv.zoneDNS = cl
	}
	return srv
}

// --- Publicación del subdominio: el estado que se pinta es el que hay --------

// Sin token de Cloudflare la funcionalidad se degrada: se dice «unconfigured»,
// no se inventa que la dirección resuelve y NADA falla.
func TestEnsureTenantDNS_SinToken(t *testing.T) {
	s := dnsTestServer(t, "", nil)
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "unconfigured" {
		t.Fatalf("status = %v, quiero unconfigured", got["status"])
	}
	if published {
		t.Fatal("sin Cloudflare no se ha publicado nada")
	}
	if got["host"] != "acme.sytue.test" {
		t.Fatalf("host = %v", got["host"])
	}
	if _, hay := got["detail"]; hay {
		t.Fatalf("«no configurado» no es un fallo del cliente: %v", got)
	}
}

// Con token pero sin zona tampoco se toca nada (config a medias).
func TestEnsureTenantDNS_SinZona(t *testing.T) {
	s := dnsTestServer(t, "", nil)
	cl := dns.New("tok", "acct", "", "")
	s.cf, s.zoneDNS = cl, cl
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "unconfigured" || published {
		t.Fatalf("sin zona no se publica: %v", got)
	}
}

// El registro no está ⇒ se CREA de verdad, con el CNAME exacto, y se reporta
// «live» + published (que es lo que dispara la auditoría en orgWire).
func TestEnsureTenantDNS_Publica(t *testing.T) {
	var creado dns.Record
	var rutas []string
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		rutas = append(rutas, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&creado)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1","type":"CNAME",
			"name":"acme.sytue.test","content":"apps.sytue.test","proxied":true}}`))
	})
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || !published {
		t.Fatalf("debía publicarse: %v (published=%v)", got, published)
	}
	if creado.Type != "CNAME" || creado.Name != "acme.sytue.test" || creado.Content != "apps.sytue.test" {
		t.Fatalf("el CNAME creado no es el esperado: %+v", creado)
	}
	if !creado.Proxied {
		t.Fatal("el registro debe ir por el proxy (TLS del comodín de la zona)")
	}
	// Primero se MIRA y solo luego se crea. La comprobación va sobre la forma y no
	// sobre un número: la consulta pregunta por cada tipo que publica una dirección
	// (CNAME, A, AAAA), así que son varios GET y un único POST al final. Fijar «2»
	// ataba el test a cuántos tipos se miran, que es un detalle que puede cambiar.
	if len(rutas) < 2 || !strings.HasPrefix(rutas[len(rutas)-1], "POST ") {
		t.Fatalf("primero se mira, solo luego se crea: %v", rutas)
	}
	for _, ruta := range rutas[:len(rutas)-1] {
		if !strings.HasPrefix(ruta, "GET ") {
			t.Fatalf("antes del POST solo debe haber lecturas: %v", rutas)
		}
	}
}

// Con MX/SPF en el mismo nombre el CNAME es ILEGAL (RFC 1034 §3.6.2): la
// dirección se publica con A/AAAA. Es el caso de `corporate`, que se quedó con
// correo y sin dirección durante semanas.
func TestEnsureTenantDNS_ConCorreoPublicaDirecciones(t *testing.T) {
	var creados []dns.Record
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// Sin CNAME/A/AAAA, pero CON el MX de los buzones de la organización.
			if r.URL.Query().Get("type") == "MX" {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-mx",
					"type":"MX","name":"acme.sytue.test","content":"mx.sytue.test"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		creados = append(creados, rec)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1"}}`))
	})
	// SIN proxy: aquí un CNAME es un CNAME y la regla del RFC se aplica entera,
	// así que la dirección tiene que ir con A/AAAA.
	s.cfg.TenantDNSProxied = false
	s.cfg.TenantDNSAddress = "203.0.113.10, 2001:db8::1"

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || !published {
		t.Fatalf("debía publicarse: %v (published=%v)", got, published)
	}
	if len(creados) != 2 {
		t.Fatalf("quiero un A y un AAAA, tengo %d: %+v", len(creados), creados)
	}
	if creados[0].Type != "A" || creados[0].Content != "203.0.113.10" {
		t.Fatalf("el A no es el esperado: %+v", creados[0])
	}
	if creados[1].Type != "AAAA" || creados[1].Content != "2001:db8::1" {
		t.Fatalf("el AAAA no es el esperado: %+v", creados[1])
	}
	for _, rec := range creados {
		if rec.Type == "CNAME" {
			t.Fatal("sin proxy, un CNAME junto al MX es lo que el proveedor rechaza")
		}
		if rec.Name != "acme.sytue.test" {
			t.Fatalf("nombre = %q", rec.Name)
		}
	}
}

// CON el proxy encendido y un proveedor que APLANA, el correo deja de estorbar:
// Cloudflare no publica un CNAME sino las direcciones de su edge, así que se
// vuelve al CNAME de siempre y no hace falta ninguna IP en los values.
//
// Es lo que permite que el subdominio de una organización con buzones se sirva
// por el túnel. Con A/AAAA en una conexión doméstica, ese registro se quedaría
// apuntando a la IP vieja en el siguiente cambio de ISP: el DDNS solo persigue
// los nombres de DDNS_HOSTS.
func TestEnsureTenantDNS_ConCorreoYProxyUsaCNAME(t *testing.T) {
	var creados []dns.Record
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("type") == "MX" {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-mx",
					"type":"MX","name":"acme.sytue.test","content":"mx.sytue.test"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		creados = append(creados, rec)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1"}}`))
	})
	// A propósito vacío: con el CNAME no hace falta, y así se comprueba que no se
	// cae al camino de las direcciones.
	s.cfg.TenantDNSAddress = ""

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || !published {
		t.Fatalf("debía publicarse: %v (published=%v)", got, published)
	}
	if len(creados) != 1 || creados[0].Type != "CNAME" {
		t.Fatalf("quiero un solo CNAME, tengo %d: %+v", len(creados), creados)
	}
	if !creados[0].Proxied {
		t.Fatal("el CNAME TIENE que ir proxied: es lo único que lo hace legal junto al MX")
	}
}

// Hay correo y el despliegue no dice con qué IP publicar: no hay forma legal de
// hacerlo. Se dice «failed» con motivo y NO se intenta un CNAME condenado —que
// es lo que dejaba el alta en verde sin dirección.
func TestEnsureTenantDNS_ConCorreoSinDireccionConfigurada(t *testing.T) {
	posts := 0
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
		}
		if r.URL.Query().Get("type") == "TXT" {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-spf",
				"type":"TXT","name":"acme.sytue.test","content":"v=spf1 -all"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
	})
	// Sin proxy no hay aplanado, así que el CNAME está descartado y sin
	// direcciones configuradas no queda forma legal de publicar.
	s.cfg.TenantDNSProxied = false
	s.cfg.TenantDNSAddress = ""

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "failed" || published {
		t.Fatalf("sin salida legal no se publica y se dice: %v (published=%v)", got, published)
	}
	if detail, _ := got["detail"].(string); detail == "" {
		t.Fatal("un fallo debe explicar el motivo")
	}
	if posts != 0 {
		t.Fatalf("no se intenta crear nada (%d POST)", posts)
	}
}

// Ya estaba ⇒ «live» sin crear nada y SIN auditar una publicación que no hubo.
func TestEnsureTenantDNS_YaEstaba(t *testing.T) {
	posts := 0
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-1","type":"CNAME",
			"name":"acme.sytue.test","content":"apps.sytue.test","proxied":true}]}`))
	})
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" {
		t.Fatalf("status = %v, quiero live", got["status"])
	}
	if published {
		t.Fatal("no se ha publicado nada nuevo: no hay que auditar")
	}
	if posts != 0 {
		t.Fatalf("no se debe crear un registro que ya existe (%d POST)", posts)
	}
}

// Carrera entre réplicas: el GET no lo vio, el POST choca con el duplicado.
// Es éxito (el registro está), pero no es «lo he publicado yo».
func TestEnsureTenantDNS_CarreraDuplicado(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"result":null,"errors":[{"code":81053,
			"message":"An A, AAAA, or CNAME record with that host already exists."}]}`))
	})
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || published {
		t.Fatalf("duplicado = ya publicado, sin auditar: %v (published=%v)", got, published)
	}
}

// Token sin permiso ⇒ «failed» con motivo. Jamás «live».
func TestEnsureTenantDNS_SinPermiso(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"result":null,"errors":[{"code":10000,"message":"Authentication error"}]}`))
	})
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "failed" || published {
		t.Fatalf("un 403 no puede pasar por publicado: %v", got)
	}
	detail, _ := got["detail"].(string)
	if detail == "" {
		t.Fatal("un fallo debe explicar el motivo")
	}
	if strings.Contains(detail, "tok") && strings.Contains(detail, "Bearer") {
		t.Fatalf("el token no puede salir en el detalle: %q", detail)
	}
}

// Zona mal configurada ⇒ «failed», con su propio motivo.
func TestEnsureTenantDNS_ZonaInexistente(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"result":null,"errors":[{"code":7003,
			"message":"Could not route to /zones/zone-abc/dns_records"}]}`))
	})
	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "failed" || published {
		t.Fatalf("una zona inexistente no publica nada: %v", got)
	}
	if !strings.Contains(got["detail"].(string), "zona") {
		t.Fatalf("detail = %v", got["detail"])
	}
}

// --- Disponibilidad y precio: consulta REAL, nunca una compra ---------------

// callAvailability ejecuta el endpoint y devuelve el cuerpo decodificado.
func callAvailability(t *testing.T, s *Server, q string) map[string]any {
	t.Helper()
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/onboarding/domain-availability?domain="+q, http.NoBody)
	s.handleOnboardingDomainAvailability(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("la consulta de disponibilidad responde siempre 200: %d %s", rr.Code, rr.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("cuerpo ilegible: %v (%s)", err, rr.Body)
	}
	// Invariante del producto: aquí NO se compran dominios, y se dice siempre.
	if out["purchasable_here"] != false {
		t.Fatalf("purchasable_here debe ser false SIEMPRE: %v", out)
	}
	return out
}

func TestDomainAvailability_DisponibleConPrecio(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/acct-123/registrar/domain-check" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"domains":[
			{"name":"xq7mz9brand.com","registrable":true,"tier":"standard",
			 "pricing":{"currency":"USD","registration_cost":"8.57","renewal_cost":"8.57"}}]}}`))
	})
	out := callAvailability(t, s, "xq7mz9brand.com")
	if out["status"] != "available" {
		t.Fatalf("status = %v, quiero available", out["status"])
	}
	price, _ := out["price"].(map[string]any)
	if price["registration"] != "8.57" || price["currency"] != "USD" {
		t.Fatalf("el precio debe ser el de Cloudflare, tal cual: %v", price)
	}
}

func TestDomainAvailability_Ocupado(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"domains":[
			{"name":"acme.com","registrable":false,"reason":"domain_unavailable"}]}}`))
	})
	out := callAvailability(t, s, "acme.com")
	if out["status"] != "taken" {
		t.Fatalf("status = %v, quiero taken", out["status"])
	}
	if _, hay := out["price"]; hay {
		t.Fatalf("un dominio ocupado no tiene precio que enseñar: %v", out)
	}
}

func TestDomainAvailability_ExtensionNoSoportada(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"domains":[
			{"name":"mybrand.uk","registrable":false,"reason":"extension_not_supported_via_api"}]}}`))
	})
	out := callAvailability(t, s, "mybrand.uk")
	// NO se dice «ocupado» (no lo sabemos) ni «libre»: se dice que no lo sabemos.
	if out["status"] != "unsupported" {
		t.Fatalf("status = %v, quiero unsupported", out["status"])
	}
	if out["reason"] != "extension_not_supported_via_api" {
		t.Fatalf("reason = %v", out["reason"])
	}
}

// Sin Cloudflare configurado no se afirma nada sobre el dominio.
func TestDomainAvailability_SinConfigurar(t *testing.T) {
	s := dnsTestServer(t, "", nil)
	out := callAvailability(t, s, "acme.com")
	if out["status"] != "unconfigured" {
		t.Fatalf("status = %v, quiero unconfigured", out["status"])
	}
	if _, hay := out["price"]; hay {
		t.Fatalf("sin consulta no hay precio: %v", out)
	}
}

// Cloudflare falla ⇒ «unknown»: no se dice ni libre ni ocupado.
func TestDomainAvailability_FalloDeCloudflare(t *testing.T) {
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"result":null,"errors":[{"code":10000,"message":"Authentication error"}]}`))
	})
	out := callAvailability(t, s, "acme.com")
	if out["status"] != "unknown" {
		t.Fatalf("status = %v, quiero unknown", out["status"])
	}
}

func TestDomainAvailability_NoEsUnDominio(t *testing.T) {
	llamadas := 0
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) { llamadas++ })
	out := callAvailability(t, s, "no%20es%20un%20dominio")
	if out["status"] != "invalid" {
		t.Fatalf("status = %v, quiero invalid", out["status"])
	}
	if llamadas != 0 {
		t.Fatal("una entrada inválida no gasta una llamada a Cloudflare")
	}
}

// El asistente normaliza como el resto del flujo (https://, www., barra final).
func TestDomainAvailability_Normaliza(t *testing.T) {
	var pedido map[string][]string
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&pedido)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"domains":[
			{"name":"acme.com","registrable":false,"reason":"domain_unavailable"}]}}`))
	})
	out := callAvailability(t, s, "https%3A%2F%2Fwww.ACME.com%2F")
	if out["domain"] != "acme.com" {
		t.Fatalf("dominio devuelto = %v, quiero el normalizado", out["domain"])
	}
	if len(pedido["domains"]) != 1 || pedido["domains"][0] != "acme.com" {
		t.Fatalf("a Cloudflare se le pregunta por el dominio normalizado: %v", pedido)
	}
}

// Con un comodín en la zona NO se crea nada: `*.sytue.test` ya sirve
// `acme.sytue.test`, y crear un registro propio ahí no añade nada y sí quita
// —en cuanto el nombre existe, el comodín deja de aplicarse a él (RFC 4592
// §2.2.1)—. Es lo que dejó `corporate.sytue.eu` en NODATA: un registro suyo en
// el nombre, sin dirección utilizable, y el comodín anulado por existir.
func TestEnsureTenantDNS_ConComodinNoCreaNada(t *testing.T) {
	posts := 0
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
		}
		// El nombre concreto no está; el comodín sí. El parámetro es
		// `name.exact`, que es con el que consulta ListRecords.
		if r.URL.Query().Get("name.exact") == "*.sytue.test" {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-wild",
				"type":"CNAME","name":"*.sytue.test","content":"sytue.test"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
	})

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" {
		t.Fatalf("con comodín el nombre YA resuelve: %v", got)
	}
	if published {
		t.Fatal("published solo es true cuando esta llamada creó algo")
	}
	if got["via"] != "wildcard" {
		t.Fatalf("debe constar que resuelve por el comodín: %v", got)
	}
	if posts != 0 {
		t.Fatalf("no se crea NADA con comodín; hubo %d POST", posts)
	}
}

// Sin comodín en la zona el camino de siempre sigue intacto: hay despliegues
// —una zona por cliente— donde el registro por organización es imprescindible.
func TestEnsureTenantDNS_SinComodinSiguePublicando(t *testing.T) {
	var creados []dns.Record
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		creados = append(creados, rec)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1"}}`))
	})

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || !published {
		t.Fatalf("sin comodín hay que publicar: %v (published=%v)", got, published)
	}
	if len(creados) != 1 || creados[0].Type != "CNAME" {
		t.Fatalf("quiero un CNAME, tengo %+v", creados)
	}
}

// EL CASO DE `corporate`, exacto: hay comodín en la zona Y hay un TXT en el
// nombre. El comodín NO cubre el nombre —un solo registro lo hace existir y lo
// desactiva para él (RFC 4592 §2.2.1)— así que la dirección hay que publicarla,
// y con el proxy encendido va como CNAME, que Cloudflare aplana y por eso
// convive con el TXT.
//
// Sin esta comprobación el atajo del comodín reportaría «live» sin crear nada y
// el subdominio seguiría devolviendo NODATA: existe y no tiene dirección.
func TestEnsureTenantDNS_ComodinNoCubreUnNombreConTXT(t *testing.T) {
	var creados []dns.Record
	s := dnsTestServer(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// El comodín existe…
			if r.URL.Query().Get("name.exact") == "*.sytue.test" {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-wild",
					"type":"CNAME","name":"*.sytue.test","content":"sytue.test"}]}`))
				return
			}
			// …y el nombre tiene su SPF, que es lo que lo anula para él.
			if r.URL.Query().Get("type") == "TXT" {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec-spf",
					"type":"TXT","name":"acme.sytue.test",
					"content":"v=spf1 mx a:mx.sytue.test include:spf.brevo.com -all"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		var rec dns.Record
		_ = json.NewDecoder(r.Body).Decode(&rec)
		creados = append(creados, rec)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec-1"}}`))
	})
	// A propósito vacío: con el proxy encendido va CNAME y no hace falta ninguna
	// IP, que además aquí sería dinámica.
	s.cfg.TenantDNSAddress = ""

	got, published := s.ensureTenantDNS(context.Background(), "acme")
	if got["status"] != "live" || !published {
		t.Fatalf("con un TXT en el nombre HAY que publicar la dirección: %v (published=%v)", got, published)
	}
	if got["via"] == "wildcard" {
		t.Fatal("el comodín no cubre un nombre que ya existe")
	}
	if len(creados) != 1 || creados[0].Type != "CNAME" || !creados[0].Proxied {
		t.Fatalf("quiero un CNAME proxied, que es lo que convive con el TXT: %+v", creados)
	}
}
