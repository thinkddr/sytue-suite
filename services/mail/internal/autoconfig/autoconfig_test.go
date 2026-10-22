// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package autoconfig

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
)

// testCfg es un despliegue representativo: hosts públicos derivados del dominio
// y puertos de TLS implícito no estándar para comprobar que NADA está fijo en
// el código (todo sale de la config del servicio).
func testCfg() config.Config {
	return config.Config{
		PrimaryDomain:      "sytue.eu",
		IMAPPublicHost:     "imap.sytue.eu",
		SMTPPublicHost:     "smtp.sytue.eu",
		IMAPSAddr:          "0.0.0.0:9993",
		SMTPSImplicitAddr:  ":9465",
		SMTPSubmissionAddr: ":587",
		Hostname:           "mx.sytue.eu",
		MTASTSMode:         "testing",
	}
}

func testRouter() http.Handler {
	r := chi.NewRouter()
	Mount(r, testCfg())
	return r
}

// --- Thunderbird / Mozilla autoconfig ---

func TestMozillaAutoconfig(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantUser     string
		wantDomains  []string
		wantAbsentIn []string
	}{
		{
			name:        "well-known sin dirección",
			path:        "/.well-known/autoconfig/mail/config-v1.1.xml",
			wantUser:    placeholderAddress,
			wantDomains: []string{"sytue.eu"},
		},
		{
			name:        "alias histórico con dirección",
			path:        "/mail/config-v1.1.xml?emailaddress=ana@sytue.eu",
			wantUser:    "ana@sytue.eu",
			wantDomains: []string{"sytue.eu"},
		},
		{
			name:        "dominio de tenant se anuncia además del principal",
			path:        "/mail/config-v1.1.xml?emailaddress=ana@acme.example",
			wantUser:    "ana@acme.example",
			wantDomains: []string{"sytue.eu", "acme.example"},
		},
		{
			name:        "dirección inválida cae al marcador",
			path:        "/mail/config-v1.1.xml?emailaddress=no-es-una-direccion",
			wantUser:    placeholderAddress,
			wantDomains: []string{"sytue.eu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, http.NoBody))
			if rec.Code != http.StatusOK {
				t.Fatalf("código=%d body=%s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
				t.Fatalf("Content-Type=%q", ct)
			}

			var got clientConfig
			if err := xml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("XML no parseable: %v\n%s", err, rec.Body.String())
			}
			if got.Version != "1.1" {
				t.Fatalf("version=%q", got.Version)
			}
			if len(got.Provider.Domains) != len(tt.wantDomains) {
				t.Fatalf("dominios=%v, quería %v", got.Provider.Domains, tt.wantDomains)
			}
			for i, d := range tt.wantDomains {
				if got.Provider.Domains[i] != d {
					t.Fatalf("dominio[%d]=%q, quería %q", i, got.Provider.Domains[i], d)
				}
			}

			in := got.Provider.Incoming
			if in.Type != "imap" || in.Hostname != "imap.sytue.eu" || in.Port != 9993 {
				t.Fatalf("incomingServer=%+v", in)
			}
			if in.SocketType != "SSL" || in.Authentication != "password-cleartext" || in.Username != tt.wantUser {
				t.Fatalf("incomingServer auth=%+v", in)
			}

			out := got.Provider.Outgoing
			if out.Type != "smtp" || out.Hostname != "smtp.sytue.eu" || out.Port != 9465 {
				t.Fatalf("outgoingServer=%+v", out)
			}
			if out.SocketType != "SSL" || out.Username != tt.wantUser {
				t.Fatalf("outgoingServer auth=%+v", out)
			}

			// No anunciamos protocolos que el servicio no sirve.
			if body := rec.Body.String(); strings.Contains(strings.ToLower(body), "pop3") ||
				strings.Contains(body, "STARTTLS") {
				t.Fatalf("anuncia un protocolo no servido:\n%s", body)
			}
		})
	}
}

// --- Outlook Autodiscover ---

// Petición POX real de Outlook (namespace de requestschema/2006).
const outlookRequest = `<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006">
  <Request>
    <EMailAddress>ana@sytue.eu</EMailAddress>
    <AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema>
  </Request>
</Autodiscover>`

func TestAutodiscover(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{name: "POST de Outlook", method: http.MethodPost, path: "/autodiscover/autodiscover.xml", body: outlookRequest, wantCode: http.StatusOK},
		{name: "POST con la ruta capitalizada", method: http.MethodPost, path: "/Autodiscover/Autodiscover.xml", body: outlookRequest, wantCode: http.StatusOK},
		{name: "GET tolerante", method: http.MethodGet, path: "/autodiscover/autodiscover.xml?emailaddress=ana@sytue.eu", wantCode: http.StatusOK},
		{name: "XML malformado", method: http.MethodPost, path: "/autodiscover/autodiscover.xml", body: "<Autodiscover><Request>", wantCode: http.StatusBadRequest},
		{name: "cuerpo vacío", method: http.MethodPost, path: "/autodiscover/autodiscover.xml", body: "", wantCode: http.StatusBadRequest},
		{
			name: "sin dirección", method: http.MethodPost, path: "/autodiscover/autodiscover.xml",
			body: `<Autodiscover><Request><EMailAddress></EMailAddress></Request></Autodiscover>`, wantCode: http.StatusBadRequest,
		},
		{name: "basura no XML", method: http.MethodPost, path: "/autodiscover/autodiscover.xml", body: "{\"json\":true}", wantCode: http.StatusBadRequest},
		{name: "GET sin dirección", method: http.MethodGet, path: "/autodiscover/autodiscover.xml", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			testRouter().ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("código=%d, quería %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}

			var got autodiscoverResponse
			if err := xml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("XML no parseable: %v\n%s", err, rec.Body.String())
			}
			acc := got.Response.Account
			if acc.AccountType != "email" || acc.Action != "settings" || len(acc.Protocols) != 2 {
				t.Fatalf("account=%+v", acc)
			}
			imap, smtp := acc.Protocols[0], acc.Protocols[1]
			if imap.Type != "IMAP" || imap.Server != "imap.sytue.eu" || imap.Port != 9993 ||
				imap.SSL != "on" || imap.LoginName != "ana@sytue.eu" {
				t.Fatalf("protocolo IMAP=%+v", imap)
			}
			if smtp.Type != "SMTP" || smtp.Server != "smtp.sytue.eu" || smtp.Port != 9465 ||
				smtp.SSL != "on" || smtp.LoginName != "ana@sytue.eu" {
				t.Fatalf("protocolo SMTP=%+v", smtp)
			}
			// Los espacios de nombres POX son obligatorios para Outlook.
			if body := rec.Body.String(); !strings.Contains(body, nsAutodiscover) || !strings.Contains(body, nsOutlook) {
				t.Fatalf("faltan los namespaces POX:\n%s", body)
			}
		})
	}
}

func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "ana@sytue.eu", want: "ana@sytue.eu", ok: true},
		{in: "  ana@sytue.eu  ", want: "ana@sytue.eu", ok: true},
		{in: "", ok: false},
		{in: "ana", ok: false},
		{in: "@sytue.eu", ok: false},
		{in: "ana@", ok: false},
		{in: "ana@localhost", ok: false}, // sin punto en el dominio
		{in: "a@b.c d@e.f", ok: false},
		{in: "ana@sytue.eu\r\nX-Injected: 1", ok: false},
		{in: `"ana"@sytue.eu`, ok: false},
		{in: strings.Repeat("a", 321) + "@sytue.eu", ok: false},
	}
	for _, tt := range tests {
		got, ok := normalizeAddress(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("normalizeAddress(%q)=(%q,%v), quería (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPortOf(t *testing.T) {
	tests := []struct {
		addr string
		def  int
		want int
	}{
		{addr: ":993", def: 143, want: 993},
		{addr: "0.0.0.0:9993", def: 143, want: 9993},
		{addr: "[::]:465", def: 25, want: 465},
		{addr: "", def: 993, want: 993},
		{addr: "sin-puerto", def: 993, want: 993},
		{addr: ":0", def: 993, want: 993},
		{addr: ":99999", def: 993, want: 993},
	}
	for _, tt := range tests {
		if got := portOf(tt.addr, tt.def); got != tt.want {
			t.Fatalf("portOf(%q,%d)=%d, quería %d", tt.addr, tt.def, got, tt.want)
		}
	}
}

// Montar sobre el router REAL del servicio (con sus middlewares y sus rutas de
// salud) no debe provocar un conflicto de rutas: chi entra en pánico al
// registrar, y eso sería una caída al arrancar mail.
func TestMountOnServiceRouter(t *testing.T) {
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: 30 * time.Second})
	Mount(r, testCfg())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/autoconfig/mail/config-v1.1.xml", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("código=%d body=%s", rec.Code, rec.Body.String())
	}
}

// El cuerpo de Autodiscover está acotado: un POST enorme no debe procesarse.
func TestAutodiscoverBodyLimit(t *testing.T) {
	huge := `<Autodiscover><Request><EMailAddress>ana@sytue.eu</EMailAddress><Pad>` +
		strings.Repeat("x", maxRequestBytes) + `</Pad></Request></Autodiscover>`
	req := httptest.NewRequest(http.MethodPost, "/autodiscover/autodiscover.xml", strings.NewReader(huge))
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código=%d, quería 400", rec.Code)
	}
}

// --- MTA-STS (RFC 8461) ---

// La política la lee un MTA remoto, no un navegador: si el formato no es el del
// RFC la descarta entera y se pierde la protección contra downgrade sin que
// falle nada visible de este lado. Se comprueba el fichero completo, línea a
// línea, porque cada campo es obligatorio y el orden de `version` primero lo
// exige el RFC.
func TestMTASTSPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/mta-sts.txt", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, se esperaba 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, se esperaba text/plain", ct)
	}
	// El MX anunciado tiene que ser el del EHLO (Hostname), no el del webmail:
	// el remitente compara este nombre con el certificado que le presenta el MX.
	want := "version: STSv1\r\nmode: testing\r\nmx: mx.sytue.eu\r\nmax_age: 604800\r\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("política =\n%q\nse esperaba\n%q", got, want)
	}
}

// enforce sin certificado que cubra el MX corta el correo entrante, así que el
// modo tiene que salir de la configuración y no estar fijo en el código.
func TestMTASTSPolicyModoConfigurable(t *testing.T) {
	cfg := testCfg()
	cfg.MTASTSMode = "enforce"
	r := chi.NewRouter()
	Mount(r, cfg)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/mta-sts.txt", http.NoBody))

	if !strings.Contains(rec.Body.String(), "mode: enforce\r\n") {
		t.Errorf("el modo no sale de la config: %q", rec.Body.String())
	}
}
