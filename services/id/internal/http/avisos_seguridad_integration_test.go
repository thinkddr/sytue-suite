// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Cambiar la contraseña deja el aviso en la campana de la PERSONA, lo cambie ella
// (token: el tenant es el slug) o se la cambie un admin (el de la organización llega
// como id y tiene que salir como slug, o el Hub lo rechaza por tenant inválido).
func TestCambiarLaContrasenaAvisaEnLaCampana(t *testing.T) {
	l := newLiveServer(t)
	llegan := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		if r.URL.Path != "/internal/notifications" || r.Header.Get("Authorization") != "Bearer s" ||
			json.NewDecoder(r.Body).Decode(&n) != nil {
			t.Errorf("al Hub le llega %s %s sin el secreto o sin cuerpo", r.Method, r.URL.Path)
		}
		llegan <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	l.srv.hub = clienteHub(Config{HubBaseURL: hub.URL, HubInternalSecret: "s"})

	org, admin := l.seedOrgAdmin(t, "acme")
	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	if err := l.st.SetPassword(l.ctx, nil, member.ID, mustHash(t, "current-pass-123")); err != nil {
		t.Fatal(err)
	}
	esperar := func(quien string) {
		t.Helper()
		select {
		case n := <-llegan:
			if n.Tenant != "acme" || n.Subject != member.ID || n.Source != "id" || n.Type != "security.password.changed" || n.Title == "" {
				t.Fatalf("%s: aviso %+v, quiero security.password.changed para acme/%s", quien, n, member.ID)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: Sytue ID no avisó al Hub del cambio de contraseña", quien)
		}
	}

	if rr, _ := l.do(t, http.MethodPost, "/api/v1/me/password", l.token(t, member.ID, org.Slug, []string{"member"}),
		map[string]string{"current_password": "current-pass-123", "new_password": "otra-clave-larga-456"}); rr.Code != http.StatusOK {
		t.Fatalf("me/password: %d %s", rr.Code, rr.Body)
	}
	esperar("la propia persona")

	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/users/"+member.ID+"/password", l.token(t, admin.ID, org.Slug, []string{"admin"}),
		map[string]string{"new_password": "la-del-admin-789"}); rr.Code != http.StatusOK {
		t.Fatalf("admin set-password: %d %s", rr.Code, rr.Body)
	}
	esperar("un admin")
}

// Un navegador que la persona no había usado es un «nuevo inicio de sesión» en su
// campana; el primero de todos (el alta) y volver desde el mismo, no.
func TestUnDispositivoNuevoAvisaEnLaCampana(t *testing.T) {
	l := newLiveServer(t)
	llegan := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		llegan <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	l.srv.hub = clienteHub(Config{HubBaseURL: hub.URL, HubInternalSecret: "s"})
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-nuevo", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	u := caOrgUser(t, l, org, "nuevo@id.test")

	entrar := func(galletas ...*http.Cookie) []*http.Cookie {
		r := httptest.NewRequest(http.MethodGet, "/login", nil)
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15")
		for _, c := range galletas {
			r.AddCookie(c)
		}
		rr := httptest.NewRecorder()
		l.srv.registerDevice(rr, r, u.ID, org.ID)
		return rr.Result().Cookies()
	}
	nada := func(cuando string) {
		t.Helper()
		select {
		case n := <-llegan:
			t.Fatalf("%s: avisa %+v y no debía", cuando, n)
		case <-time.After(300 * time.Millisecond):
		}
	}

	suyo := entrar()
	nada("el alta")
	entrar(suyo...)
	nada("el mismo navegador")
	entrar()
	select {
	case n := <-llegan:
		if n.Tenant != "acme-nuevo" || n.Subject != u.ID || n.Type != "security.signin.new" || n.Params["device"] == "" {
			t.Fatalf("aviso %+v, quiero security.signin.new con el dispositivo", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("un navegador nuevo no avisó en la campana")
	}
}
