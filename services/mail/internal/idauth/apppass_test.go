// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El camino por el que entra un cliente de correo real: la contraseña de
// aplicación que la persona crea en /account de Sytue ID. Antes de esto el AUTH
// solo aceptaba un JWT con scope `mail` (minutos de vida), o sea que ni
// Thunderbird ni Outlook ni el móvil podían conectarse a nada.
package idauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAppPassVerify(t *testing.T) {
	var visto struct {
		auth, ctype, identity, password string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/app-passwords/verify" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		visto.auth = r.Header.Get("Authorization")
		visto.ctype = r.Header.Get("Content-Type")
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		visto.identity, visto.password = in["identity"], in["password"]

		if in["password"] != "abcd-efgh-ijkl-mnpq-rstu" {
			// Igual que Sytue ID: una contraseña que no vale y una cuenta que no
			// existe se responden igual (401), para no enumerar direcciones.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(appPassResp{
			Subject: "u-1", Tenant: "acme",
			Email: "ana@example.org", Mailbox: "Ana@acme.sytue.eu",
		})
	}))
	defer srv.Close()

	c := &appPassClient{baseURL: srv.URL, secret: "s3cr3t", http: &http.Client{Timeout: 5 * time.Second}}

	r, err := c.verify(context.Background(), "ana@acme.sytue.eu", "abcd-efgh-ijkl-mnpq-rstu")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if r.Subject != "u-1" || r.Tenant != "acme" || r.Mailbox != "Ana@acme.sytue.eu" {
		t.Fatalf("respuesta inesperada: %+v", r)
	}
	// El secreto compartido ES la autorización del endpoint: sin cabecera, Sytue
	// ID responde 401 y ningún cliente de correo entraría.
	if visto.auth != "Bearer s3cr3t" {
		t.Fatalf("Authorization = %q", visto.auth)
	}
	if visto.ctype != "application/json" {
		t.Fatalf("Content-Type = %q", visto.ctype)
	}
	if visto.identity != "ana@acme.sytue.eu" || visto.password != "abcd-efgh-ijkl-mnpq-rstu" {
		t.Fatalf("cuerpo mal formado: %+v", visto)
	}

	if _, err := c.verify(context.Background(), "ana@acme.sytue.eu", "otra"); err == nil {
		t.Fatal("una contraseña rechazada por Sytue ID debe ser un error")
	}
}

// Sin secreto (o sin URL) el segundo camino NO se activa: se queda como estaba,
// solo JWT. Una verja abierta aquí dejaría entrar en el correo de cualquiera.
func TestWithAppPasswordsFailClosed(t *testing.T) {
	for _, c := range []struct{ url, secret string }{
		{"", "s"}, {"http://id", ""}, {"  ", "s"},
	} {
		if v := (&Verifier{}).WithAppPasswords(c.url, c.secret); v.appPass != nil {
			t.Fatalf("se activó con url=%q secret=%q", c.url, c.secret)
		}
	}
	if v := (&Verifier{}).WithAppPasswords("http://id/", "s"); v.appPass == nil || v.appPass.baseURL != "http://id" {
		t.Fatal("no se activó, o no se limpió la barra final de la URL")
	}
}
