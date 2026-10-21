// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func profileCfg() Config {
	return Config{
		Issuer:       "https://id.sytue.eu",
		MailIMAPAddr: "imap.sytue.eu:9993",
		MailSMTPAddr: "smtp.sytue.eu:9465",
		CalDAVURL:    "https://calendar.sytue.eu/dav/",
		CardDAVURL:   "https://contacts.sytue.eu:8443/dav/",
	}
}

var profileUser = domain.User{ID: "u-1", Email: "ana@sytue.eu", Name: "Ana Ruiz"}

// El perfil debe ser un plist BIEN FORMADO con las tres cuentas y sin secretos.
func TestBuildAppleProfile(t *testing.T) {
	got, err := buildAppleProfile(profileCfg(), profileUser, "", "")
	if err != nil {
		t.Fatalf("buildAppleProfile: %v", err)
	}

	// 1) XML bien formado y cabecera de plist de Apple.
	var plist struct {
		XMLName xml.Name `xml:"plist"`
		Version string   `xml:"version,attr"`
	}
	if err := xml.Unmarshal(got, &plist); err != nil {
		t.Fatalf("plist no parseable: %v\n%s", err, got)
	}
	if plist.Version != "1.0" {
		t.Fatalf("version=%q", plist.Version)
	}
	if !strings.Contains(string(got), plistDoctype) {
		t.Fatalf("falta el DOCTYPE de Apple:\n%s", got)
	}

	body := string(got)

	// 2) Las tres payloads, con los hosts/puertos de la CONFIG (nada fijo).
	for _, want := range []string{
		"com.apple.mail.managed", "com.apple.caldav.account", "com.apple.carddav.account",
		"<string>imap.sytue.eu</string>", "<integer>9993</integer>",
		"<string>smtp.sytue.eu</string>", "<integer>9465</integer>",
		"<string>calendar.sytue.eu</string>", "<string>contacts.sytue.eu</string>",
		"<integer>8443</integer>", // puerto explícito de la URL CardDAV
		"<string>/dav/</string>",
		"<string>ana@sytue.eu</string>",
		"eu.sytue.sytue-one", // identificador DNS invertido del dominio de la suite
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en el perfil:\n%s", want, body)
		}
	}
	// CalDAV sin puerto explícito en la URL → 443 (https). Booleanos canónicos.
	if !strings.Contains(body, "<key>CalDAVPort</key>") || !strings.Contains(body, "<integer>443</integer>") {
		t.Fatalf("CalDAVPort debería ser 443:\n%s", body)
	}
	if !strings.Contains(body, "<true/>") || strings.Contains(body, "<true></true>") {
		t.Fatalf("los booleanos del plist no están en forma canónica:\n%s", body)
	}

	// 3) NINGUNA contraseña dentro del perfil (la pide el sistema al instalar).
	for _, forbidden := range []string{"Password</key>\n\t\t\t\t<string>", "PayloadCertificate", "EmailAuthPasswordPlain"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("el perfil contiene %q:\n%s", forbidden, body)
		}
	}
	if strings.Contains(body, "<key>IncomingPassword</key>") || strings.Contains(body, "<key>OutgoingPassword</key>") {
		t.Fatalf("el perfil lleva contraseñas:\n%s", body)
	}

	// 4) UUID estable por usuario+servicio (reinstalar reemplaza, no duplica).
	again, err := buildAppleProfile(profileCfg(), profileUser, "", "")
	if err != nil {
		t.Fatalf("segunda generación: %v", err)
	}
	if string(again) != body {
		t.Fatal("el perfil no es determinista para el mismo usuario")
	}
	otro, err := buildAppleProfile(profileCfg(), domain.User{ID: "u-2", Email: "bea@sytue.eu"}, "", "")
	if err != nil {
		t.Fatalf("otro usuario: %v", err)
	}
	if strings.Contains(string(otro), "ana@sytue.eu") {
		t.Fatal("el perfil de otro usuario filtra la cuenta ajena")
	}
}

func TestBuildAppleProfileConfigErrors(t *testing.T) {
	tests := []struct {
		name  string
		mutar func(*Config)
	}{
		{name: "IMAP vacío", mutar: func(c *Config) { c.MailIMAPAddr = "" }},
		{name: "IMAP con puerto inválido", mutar: func(c *Config) { c.MailIMAPAddr = "imap.sytue.eu:0" }},
		{name: "CalDAV sin esquema", mutar: func(c *Config) { c.CalDAVURL = "calendar.sytue.eu/dav/" }},
		{name: "CardDAV vacío", mutar: func(c *Config) { c.CardDAVURL = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := profileCfg()
			tt.mutar(&cfg)
			if _, err := buildAppleProfile(cfg, profileUser, "", ""); err == nil {
				t.Fatal("se esperaba error de configuración")
			}
		})
	}
}

// Sin sesión no se emite perfil: redirige al login (no filtra cuentas).
func TestHandleAppleProfileRequiresSession(t *testing.T) {
	s := &Server{cfg: profileCfg()}
	rec := httptest.NewRecorder()
	s.handleAppleProfile(rec, httptest.NewRequest(http.MethodGet, "/account/apple.mobileconfig", http.NoBody))
	if rec.Code != http.StatusFound {
		t.Fatalf("código=%d, quería 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login/") {
		t.Fatalf("Location=%q", loc)
	}
}

func TestSuiteDomainYDefaults(t *testing.T) {
	tests := []struct {
		issuer string
		want   string
	}{
		{issuer: "https://id.sytue.eu", want: "sytue.eu"},
		{issuer: "https://id.sytue.localhost", want: "sytue.localhost"},
		{issuer: "https://auth.example.com:8443", want: "auth.example.com"},
		{issuer: "sytue.eu", want: "sytue.eu"},
	}
	for _, tt := range tests {
		if got := suiteDomain(tt.issuer); got != tt.want {
			t.Fatalf("suiteDomain(%q)=%q, quería %q", tt.issuer, got, tt.want)
		}
	}
	if got := reverseDomain("sytue.eu"); got != "eu.sytue" {
		t.Fatalf("reverseDomain=%q", got)
	}
}

func TestSplitDAVURL(t *testing.T) {
	tests := []struct {
		raw     string
		host    string
		port    int
		path    string
		ssl     bool
		wantErr bool
	}{
		{raw: "https://calendar.sytue.eu/dav/", host: "calendar.sytue.eu", port: 443, path: "/dav/", ssl: true},
		{raw: "http://calendar.sytue.localhost/dav/", host: "calendar.sytue.localhost", port: 80, path: "/dav/"},
		{raw: "https://calendar.sytue.eu:8443/dav", host: "calendar.sytue.eu", port: 8443, path: "/dav", ssl: true},
		{raw: "https://calendar.sytue.eu", host: "calendar.sytue.eu", port: 443, path: "/", ssl: true},
		{raw: "", wantErr: true},
		{raw: "ftp://calendar.sytue.eu/dav/", wantErr: true},
		{raw: "calendar.sytue.eu/dav/", wantErr: true},
	}
	for _, tt := range tests {
		host, port, path, ssl, err := splitDAVURL(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("splitDAVURL(%q) no falló", tt.raw)
			}
			continue
		}
		if err != nil || host != tt.host || port != tt.port || path != tt.path || ssl != tt.ssl {
			t.Fatalf("splitDAVURL(%q)=(%q,%d,%q,%v,%v)", tt.raw, host, port, path, ssl, err)
		}
	}
}

// --- La autoridad S/MIME dentro del perfil ---
//
// Publicar el `.pem` en una URL no instala nada: en un teléfono, abrir un
// certificado suelto desde el navegador acaba, con suerte, en Descargas. Dentro
// del perfil va al almacén correcto de una vez, junto a las cuentas.

// unCertificadoPEM genera uno de verdad: comprobar el perfil contra una cadena
// inventada no diría nada sobre si lo que entra es DER válido.
func unCertificadoPEM(t *testing.T) (string, []byte) {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("clave: %v", err)
	}
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Sytue S/MIME CA de prueba"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatalf("certificado: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), der
}

func TestPerfilLlevaLaAutoridadSMIME(t *testing.T) {
	pemTexto, der := unCertificadoPEM(t)
	got, err := buildAppleProfile(profileCfg(), profileUser, pemTexto, "")
	if err != nil {
		t.Fatalf("buildAppleProfile: %v", err)
	}
	xmlPerfil := string(got)

	if !strings.Contains(xmlPerfil, "com.apple.security.root") {
		t.Error("el perfil no lleva carga de certificado raíz")
	}
	// VA EL DER Y NO EL PEM. Con el PEM dentro, el perfil se instala igual y el
	// certificado no queda utilizable: es el fallo que no se ve.
	if !strings.Contains(xmlPerfil, base64.StdEncoding.EncodeToString(der)) {
		t.Error("el certificado no viaja como DER en base64")
	}
	if strings.Contains(xmlPerfil, "BEGIN CERTIFICATE") {
		t.Error("el PEM entero se coló en el perfil; `PayloadContent` es binario")
	}
	// En `<data>` y no en `<string>`: sin el tipo propio saldría como cadena.
	if !strings.Contains(xmlPerfil, "<data>") {
		t.Error("el certificado no va en <data>")
	}
}

func TestPerfilSinAutoridadSaleIgual(t *testing.T) {
	// Sin autoridad, o con un fichero que no es un certificado, el perfil sale con
	// sus tres cuentas: configurar el correo no puede depender de que la firma
	// esté montada.
	for _, entrada := range []string{"", "no soy un pem", "-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"} {
		got, err := buildAppleProfile(profileCfg(), profileUser, entrada, "")
		if err != nil {
			t.Fatalf("buildAppleProfile(%q): %v", entrada, err)
		}
		x := string(got)
		if strings.Contains(x, "com.apple.security.root") {
			t.Errorf("entrada %q metió una carga de certificado", entrada)
		}
		for _, imprescindible := range []string{"com.apple.mail.managed", "com.apple.caldav.account", "com.apple.carddav.account"} {
			if !strings.Contains(x, imprescindible) {
				t.Errorf("entrada %q se llevó por delante %s", entrada, imprescindible)
			}
		}
	}
}

func TestDerDelPEMSaltaLoQueNoEsCertificado(t *testing.T) {
	// Un fichero de cadena puede empezar por una clave o por parámetros;
	// quedarse con el primer bloque metería eso en el perfil.
	pemTexto, der := unCertificadoPEM(t)
	conBasuraDelante := "-----BEGIN DH PARAMETERS-----\nQUJD\n-----END DH PARAMETERS-----\n" + pemTexto
	if got := derDelPEM(conBasuraDelante); !bytes.Equal(got, der) {
		t.Error("no saltó el bloque que no era un certificado")
	}
}

// TestElPerfilConClaveNoPideNada: con la contraseña de aplicación dentro, las tres
// cuentas la llevan y el sistema no pide ninguna al instalar; sin ella, no va ninguna.
func TestElPerfilConClaveNoPideNada(t *testing.T) {
	con, err := buildAppleProfile(profileCfg(), profileUser, "", "abcd-efgh-ijkm-npqr-stuv")
	if err != nil {
		t.Fatal(err)
	}
	for _, campo := range []string{"IncomingPassword", "OutgoingPassword", "CalDAVPassword", "CardDAVPassword"} {
		if !strings.Contains(string(con), "<key>"+campo+"</key>") {
			t.Errorf("el perfil con clave no lleva %s", campo)
		}
	}
	if strings.Count(string(con), "abcd-efgh-ijkm-npqr-stuv") != 4 {
		t.Errorf("la clave va %d veces, y son cuatro campos", strings.Count(string(con), "abcd-efgh-ijkm-npqr-stuv"))
	}
	sin, err := buildAppleProfile(profileCfg(), profileUser, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, campo := range []string{"IncomingPassword", "OutgoingPassword", "CalDAVPassword", "CardDAVPassword"} {
		if strings.Contains(string(sin), "<key>"+campo+"</key>") {
			t.Errorf("el perfil sin clave lleva %s", campo)
		}
	}
}
