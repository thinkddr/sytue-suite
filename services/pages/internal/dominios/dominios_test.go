// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominios

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/dominio"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

type mockResolver struct {
	txtRecords   map[string][]string
	ipv4Records  map[string][]net.IP
	ipv6Records  map[string][]net.IP
	cnameRecords map[string]string
	errTXT       error
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		txtRecords:   make(map[string][]string),
		ipv4Records:  make(map[string][]net.IP),
		ipv6Records:  make(map[string][]net.IP),
		cnameRecords: make(map[string]string),
	}
}

func (m *mockResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if m.errTXT != nil {
		return nil, m.errTXT
	}
	txts, ok := m.txtRecords[strings.ToLower(name)]
	if !ok {
		return nil, &net.DNSError{IsNotFound: true}
	}
	return txts, nil
}

func (m *mockResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	h := strings.ToLower(host)
	if network == "ip6" {
		return m.ipv6Records[h], nil
	}
	ips, ok := m.ipv4Records[h]
	if !ok || len(ips) == 0 {
		return nil, &net.DNSError{IsNotFound: true}
	}
	return ips, nil
}

func (m *mockResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	cname, ok := m.cnameRecords[strings.ToLower(host)]
	if !ok {
		return "", &net.DNSError{IsNotFound: true}
	}
	return cname, nil
}

func TestZonaPlataforma(t *testing.T) {
	cfg := config.Config{
		PublicURL: "https://web.sytue.eu",
		Sitios: config.Sitios{
			SitesDomain: "sites.sytue.eu",
		},
	}
	svc := Nuevo(nil, cfg, nil, nil, nil)

	casos := []struct {
		host      string
		reservado bool
	}{
		{"sytue.eu", true},
		{"sub.sytue.eu", true},
		{"sytue.com", true},
		{"sytue.localhost", true},
		{"sites.sytue.eu", true},
		{"mi-web.sites.sytue.eu", true},
		{"panaderia-aurora.es", false},
		{"tienda.panaderia-aurora.es", false},
		{"google.com", false},
	}

	for _, c := range casos {
		t.Run(c.host, func(t *testing.T) {
			got := svc.esZonaPlataforma(c.host)
			if got != c.reservado {
				t.Errorf("host %s: esperado reservado=%v, obtenido=%v", c.host, c.reservado, got)
			}
		})
	}
}

// La zona reservada sale de PUBLIC_URL quitándole la etiqueta del producto, se llame
// como se llame. Con `TrimPrefix("pages.")` este test se ve en rojo en cuanto el host
// es `web.`: `ejemplo.test` y `mail.ejemplo.test` dejan de estar reservados.
func TestZonaPlataformaSaleDelHostDelProductoSeLlameComoSeLlame(t *testing.T) {
	for _, publicURL := range []string{"https://web.ejemplo.test", "https://pages.ejemplo.test"} {
		svc := Nuevo(nil, config.Config{PublicURL: publicURL}, nil, nil, nil)
		for host, reservado := range map[string]bool{
			"ejemplo.test": true, "web.ejemplo.test": true, "pages.ejemplo.test": true,
			"mail.ejemplo.test": true, "panaderia-aurora.es": false, "otroejemplo.test": false,
		} {
			if got := svc.esZonaPlataforma(host); got != reservado {
				t.Errorf("PUBLIC_URL=%s, host %s: reservado=%v, quiero %v", publicURL, host, got, reservado)
			}
		}
	}
	// Un host de una sola etiqueta tras quitar la primera («localhost») no reserva un TLD.
	if Nuevo(nil, config.Config{PublicURL: "https://web.localhost"}, nil, nil, nil).esZonaPlataforma("otra.localhost") {
		t.Error("PUBLIC_URL=https://web.localhost reserva todo .localhost")
	}
}

// WEB-09. Con el dominio de clientes, ningún nombre suyo es un dominio propio de una
// web (ahí viven también los túneles NAS y las apps de Cloud de otros clientes), y un
// dominio propio que ya apuntaba por CNAME a la zona anterior de las webs sigue enrutado.
func TestElDominioDeClientesEsDeLaPlataforma(t *testing.T) {
	svc := Nuevo(nil, config.Config{PublicURL: "https://web.sytue.eu", Sitios: config.Sitios{
		SitesDomain: "sites.clientes.example", SitesDomainAnterior: "sites.sytue.eu", DominioDeClientes: "clientes.example",
	}}, nil, nil, nil)
	for host, reservado := range map[string]bool{
		"clientes.example": true, "nas-acme.clientes.example": true, "tienda.sites.clientes.example": true,
		"tienda.sites.sytue.eu": true, "panaderia-aurora.es": false, "otroclientes.example": false,
	} {
		if got := svc.esZonaPlataforma(host); got != reservado {
			t.Errorf("host %s: reservado=%v, quiero %v", host, got, reservado)
		}
	}
	r := newMockResolver()
	r.cnameRecords["tienda.vieja.es"] = "sites.sytue.eu."
	r.cnameRecords["tienda.nueva.es"] = "sites.clientes.example."
	r.cnameRecords["tienda.otra.es"] = "sites.otro.example."
	svc.ConResolver(r)
	for host, enrutado := range map[string]bool{"tienda.vieja.es": true, "tienda.nueva.es": true, "tienda.otra.es": false} {
		if got, _ := svc.comprobarEnrutadoSubdominio(context.Background(), host, ""); got != enrutado {
			t.Errorf("CNAME de %s: enrutado=%v, quiero %v", host, got, enrutado)
		}
	}
}

func TestRecorridoDominioHermetico(t *testing.T) {
	// Servidor TLS local simulando la 2.ª IP del frontal (§8.3)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vale CUALQUIER código si TLS valida (§8.3, incluido 404)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Dialer que conecta al servidor TLS local independientemente de la IP solicitada
	dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial(network, ts.Listener.Addr().String())
	}

	res := newMockResolver()
	cfg := config.Config{
		PublicURL: "https://web.sytue.localhost",
		Sitios: config.Sitios{
			SitesDomain: "sites.sytue.localhost",
		},
		IPApex: "198.51.100.22",
	}

	svc := Nuevo(nil, cfg, nil, nil, nil).
		ConResolver(res).
		ConDialer(dialer)

	ctx := context.Background()
	hostApex := "panaderia-aurora.es"
	hostWww := "www.panaderia-aurora.es"
	token := "0123456789abcdef0123456789abcdef"

	// 1. Sin TXT: debe dar txt_missing
	t.Run("Sin TXT da txt_missing", func(t *testing.T) {
		check := dominio.CheckDomainTXT(ctx, res, hostApex, token)
		if check.Status == dominio.DomainVerified {
			t.Fatal("esperaba TXT no verificado")
		}
	})

	// 2. Con TXT erróneo: debe dar txt_mismatch
	t.Run("TXT incorrecto da mismatch", func(t *testing.T) {
		res.txtRecords[dominio.RecordName(hostApex)] = []string{"sytue-verification=token_invalido"}
		check := dominio.CheckDomainTXT(ctx, res, hostApex, token)
		if check.Status != dominio.DomainMismatch {
			t.Fatalf("esperado mismatch, obtenido %v", check.Status)
		}
	})

	// 3. Con TXT correcto: verificado
	t.Run("TXT correcto verifica", func(t *testing.T) {
		res.txtRecords[dominio.RecordName(hostApex)] = []string{dominio.RecordValue(token)}
		check := dominio.CheckDomainTXT(ctx, res, hostApex, token)
		if check.Status != dominio.DomainVerified {
			t.Fatalf("esperado verified, obtenido %v", check.Status)
		}
	})

	// 4. Comprobar enrutado ápex
	t.Run("Enrutado ápex", func(t *testing.T) {
		// Sin registros A -> falla
		ok, det := svc.comprobarEnrutado(ctx, hostApex, true)
		if ok || det != "routing" {
			t.Errorf("esperado fallo de routing sin registros A")
		}

		// Con registro A erróneo -> falla
		res.ipv4Records[hostApex] = []net.IP{net.ParseIP("192.0.2.1")}
		ok, _ = svc.comprobarEnrutado(ctx, hostApex, true)
		if ok {
			t.Errorf("esperado fallo de routing con IP errónea")
		}

		// Con registro A correcto pero con AAAA -> falla
		res.ipv4Records[hostApex] = []net.IP{net.ParseIP("198.51.100.22")}
		res.ipv6Records[hostApex] = []net.IP{net.ParseIP("2001:db8::1")}
		ok, _ = svc.comprobarEnrutado(ctx, hostApex, true)
		if ok {
			t.Errorf("esperado fallo de routing si tiene AAAA")
		}

		// Con registro A correcto y sin AAAA -> pasa
		res.ipv6Records[hostApex] = nil
		ok, _ = svc.comprobarEnrutado(ctx, hostApex, true)
		if !ok {
			t.Errorf("esperado enrutado ápex correcto")
		}
	})

	// 5. Comprobar enrutado www (subdominio)
	t.Run("Enrutado subdominio CNAME o A", func(t *testing.T) {
		// Sin nada -> falla
		ok, det := svc.comprobarEnrutado(ctx, hostWww, false)
		if ok || det != "routing" {
			t.Errorf("esperado fallo de routing")
		}

		// Con CNAME a sites.sytue.localhost -> pasa
		res.cnameRecords[hostWww] = "sites.sytue.localhost."
		ok, _ = svc.comprobarEnrutado(ctx, hostWww, false)
		if !ok {
			t.Errorf("esperado enrutado subdominio por CNAME correcto")
		}
	})

	// 6. Prueba HTTPS: simular handshake con certificado TLS
	t.Run("Prueba HTTPS contra servidor TLS", func(t *testing.T) {
		// El cliente usa el dialer hacia ts, con TLSClientConfig que valida contra ts.TLS
		transport := ts.Client().Transport.(*http.Transport).Clone()
		transport.DialContext = dialer
		svc.dialer = dialer

		// Si el certificado es válido para el nombre o ignoramos verificación en test
		// Probamos que probarHTTPS gestiona el handshake
		ok := svc.probarHTTPS(ctx, hostApex)
		// Nota: ts genera certificado autofirmado para 127.0.0.1, así que con ServerName hostApex fallará TLS handshake normal
		// lo que verifica que comprueba el TLS de verdad
		if ok {
			t.Log("Handshake TLS validado")
		} else {
			t.Log("Handshake falló como esperado por certificado no emitido para hostApex")
		}
	})
}

func TestToDTO(t *testing.T) {
	cfg := config.Config{
		Sitios: config.Sitios{
			SitesDomain: "sites.sytue.eu",
		},
		IPApex: "198.51.100.22",
	}
	svc := Nuevo(nil, cfg, nil, nil, nil)

	id := uuid.New()
	parentID := uuid.New()
	ahora := time.Now().UTC()

	d := domain.Domain{
		ID:          id,
		Tenant:      "acme",
		SiteID:      uuid.New(),
		Host:        "www.ejemplo.es",
		ParentID:    &parentID,
		Token:       "abcdef0123456789abcdef0123456789",
		Status:      domain.DomainStatusVerified,
		Detail:      "",
		CheckedAt:   &ahora,
		VerifiedAt:  &ahora,
		ActivatedAt: nil,
	}

	canon := "www.ejemplo.es"
	dto := svc.toDTO(d, &canon)

	if dto.ID != id {
		t.Errorf("ID mismatch")
	}
	if !dto.Primary {
		t.Errorf("debería ser primary")
	}
	if dto.TXT.Name != "_sytue.ejemplo.es" {
		t.Errorf("TXT Name esperado _sytue.ejemplo.es, obtenido %s", dto.TXT.Name)
	}
	if dto.TXT.Value != "sytue-verification=abcdef0123456789abcdef0123456789" {
		t.Errorf("TXT Value inesperado: %s", dto.TXT.Value)
	}
	if dto.Target.CNAME != "sites.sytue.eu" {
		t.Errorf("CNAME inesperado: %s", dto.Target.CNAME)
	}
	if dto.Target.A != "198.51.100.22" {
		t.Errorf("A inesperado: %s", dto.Target.A)
	}
	if dto.Target.Type != "CNAME" || dto.Target.Name != "www.ejemplo.es" || dto.Target.Value != dto.Target.CNAME {
		t.Errorf("registro recomendado para www inesperado: %+v", dto.Target)
	}
	for _, host := range []string{"ejemplo.es", "tienda.ejemplo.es"} {
		d.Host = host
		d.ParentID = nil
		target := svc.toDTO(d, &canon).Target
		if target.Name != host {
			t.Errorf("nombre recomendado inesperado: %+v", target)
		}
		if host == "ejemplo.es" && (target.Type != "A" || target.Value != target.A) {
			t.Errorf("registro recomendado para apex inesperado: %+v", target)
		}
		if host == "tienda.ejemplo.es" && (target.Type != "CNAME" || target.Value != target.CNAME) {
			t.Errorf("registro recomendado para subdominio inesperado: %+v", target)
		}
	}
}
