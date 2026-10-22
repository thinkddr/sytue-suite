// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

// El callback llega sin sesión: solo dirige a un BFF conocido. El canje conserva
// el estado intacto y sigue exigiendo la persona y organización que lo iniciaron.
func TestElCallbackDeAlmacenamientoVuelveADriveSinEntrarEnAdmin(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{
		oauthCipher: cipher, consoleURL: "https://admin.sytue.example/admin",
		drivePublicURL: "https://drive.sytue.example", roomsPublicURL: "https://rooms.sytue.example", mailPublicURL: "https://mail.sytue.example",
	}
	for _, tc := range []struct {
		name, section, providerError, code string
		age                                time.Duration
		drive, rooms, external, mail       bool
	}{
		{name: "mail autorizado", section: "external-mail", code: "code", mail: true},
		{name: "mail denegado", section: "external-mail", providerError: "access_denied", mail: true},
		{name: "autorizado", section: "storage", code: "code", drive: true},
		{name: "denegado", section: "storage", providerError: "access_denied", drive: true},
		{name: "rooms autorizado", section: "rooms", code: "code", rooms: true},
		{name: "rooms denegado", section: "rooms", providerError: "access_denied", rooms: true},
		{name: "reunión Teams", section: teamsMeetingSection, code: "code", external: true},
		{name: "external autorizado", section: "external-chat", code: "code", external: true},
		{name: "external denegado", section: "external-chat", providerError: "access_denied", external: true},
		{name: "error del proveedor", section: "storage", providerError: "invalid_scope", drive: true},
		{name: "falta código", section: "storage", drive: true},
		{name: "caducado", section: "storage", code: "code", age: -11 * time.Minute, drive: true},
		{name: "otra sección", section: "migrations", code: "code"},
		{name: "sección manipulada", section: "https://evil.example", code: "code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := consent.Begin(cipher, consent.Flow{AuthorizeURL: "https://www.dropbox.com/oauth2/authorize", ClientID: "app"},
				"p-personal", "ana", "instance", "https://integrations.sytue.example/callback", tc.section, time.Now().Add(tc.age))
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(st.AuthorizeURL)
			if err != nil {
				t.Fatal(err)
			}
			state := u.Query().Get("state")
			q := url.Values{"state": {state}, "code": {tc.code}, "error": {tc.providerError}, "return_to": {"https://evil.example"}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/callback?"+q.Encode(), http.NoBody)
			req.Host = "evil.example"
			req.Header.Set("X-Forwarded-Host", "evil.example")
			w := httptest.NewRecorder()
			h.OAuthCallback(w, req)
			location, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			host, path := "admin.sytue.example", "/admin"+rutaFinEnLaConsola
			if tc.drive {
				host, path = "drive.sytue.example", rutaFinEnDrive
			}
			if tc.rooms {
				host, path = "rooms.sytue.example", rutaFinEnRooms
			}
			if tc.external {
				host, path = "rooms.sytue.example", "/rooms/api/external-conversations-finish"
			}
			if tc.mail {
				host, path = "mail.sytue.example", "/mail/api/external-mail-finish"
			}
			if w.Code != http.StatusFound || location.Host != host || location.Path != path || location.Scheme != "https" {
				t.Fatalf("vuelta inesperada: %d %s", w.Code, location)
			}
			if location.Query().Get("state") != state || location.Query().Get("code") != tc.code ||
				location.Query().Get("error") != tc.providerError || location.Query().Has("return_to") {
				t.Fatalf("el callback debe pasar solo el estado y el resultado del proveedor: %s", location)
			}
		})
	}
}

func TestUnEstadoFabricadoNoEligeElCallbackDeDrive(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{oauthCipher: cipher, consoleURL: "https://admin.sytue.example/admin", drivePublicURL: "https://drive.sytue.example"}
	w := httptest.NewRecorder()
	h.OAuthCallback(w, httptest.NewRequest(http.MethodGet,
		"/api/v1/oauth/callback?state=fabricado&code=x&section=storage&return_to=https://evil.example", http.NoBody))
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "admin.sytue.example" || u.Path != "/admin"+rutaFinEnLaConsola {
		t.Fatalf("un estado fabricado seleccionó el destino: %s", u)
	}
}

func TestCallbackNativoSoloEligeEsquemasFijosDelEstadoSellado(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{oauthCipher: cipher, consoleURL: "https://admin.example.eu"}
	for _, tc := range []struct{ section, scheme string }{{"native-mail", "sytue-mail"}, {"native-storage", "sytue-drive"}, {"native-rooms", "sytue-rooms"}, {"native-apps", "sytue-one"}, {"native-apps:out", "sytue-one"}} {
		st, err := consent.Begin(cipher, consent.Flow{AuthorizeURL: "https://accounts.google.com/authorize", ClientID: "app"}, "acme", "ana", "instance", "https://integrations.example.eu/callback", tc.section, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(st.AuthorizeURL)
		q := url.Values{"state": {u.Query().Get("state")}, "code": {"codigo"}, "return_to": {"evil://steal"}}
		w := httptest.NewRecorder()
		h.OAuthCallback(w, httptest.NewRequest(http.MethodGet, "/api/v1/oauth/callback?"+q.Encode(), http.NoBody))
		dest, err := url.Parse(w.Header().Get("Location"))
		if err != nil || w.Code != http.StatusFound || dest.Scheme != tc.scheme || dest.Host != "connect" || dest.Path != "/callback" || dest.Query().Get("state") != q.Get("state") || dest.Query().Get("code") != "codigo" || dest.Query().Has("return_to") {
			t.Fatalf("destino nativo inseguro: %s (%v)", dest, err)
		}
	}
}
