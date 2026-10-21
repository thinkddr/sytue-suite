//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestElEnlaceDelPerfilValeUnaVez — el enlace que la app del iPhone abre en Safari da el
// perfil con su contraseña de aplicación UNA vez: la segunda descarga, o un token
// inventado, no dan nada. Es la única credencial que lleva Safari.
func TestElEnlaceDelPerfilValeUnaVez(t *testing.T) {
	l := newLiveServer(t)
	l.srv.cfg.MailIMAPAddr, l.srv.cfg.MailSMTPAddr = "imap.id.test:993", "smtp.id.test:465"
	l.srv.cfg.CalDAVURL, l.srv.cfg.CardDAVURL = "https://calendar.id.test/dav", "https://contacts.id.test/dav"
	u := l.mkUser(t, "perfil@id.test", "")
	tok := l.token(t, u.ID, "", nil)

	rr, out := l.do(t, http.MethodPost, "/api/v1/me/apple-profile/link", tok, map[string]string{"label": "iPhone de Ada"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("enlace: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	enlace, _ := out["url"].(string)
	i := strings.Index(enlace, "/account/apple-profile/")
	if i < 0 {
		t.Fatalf("el enlace no apunta al perfil: %q", enlace)
	}
	ruta := enlace[i:]

	bajar := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		w := httptest.NewRecorder()
		l.mux.ServeHTTP(w, r)
		return w
	}
	primera := bajar()
	if primera.Code != http.StatusOK || !strings.Contains(primera.Body.String(), "<key>IncomingPassword</key>") {
		t.Fatalf("primera descarga: status=%d, con contraseña dentro=%v", primera.Code, strings.Contains(primera.Body.String(), "IncomingPassword"))
	}
	if segunda := bajar(); segunda.Code != http.StatusGone {
		t.Fatalf("la segunda descarga del mismo enlace dio %d; quiero 410", segunda.Code)
	}

	// La contraseña creada sale en Mi cuenta con la etiqueta del enlace.
	_, out = l.do(t, http.MethodGet, "/api/v1/me/app-passwords", tok, nil)
	vs, _ := out["app_passwords"].([]any)
	if len(vs) != 1 || vs[0].(map[string]any)["label"] != "iPhone de Ada" {
		t.Fatalf("contraseñas tras el canje: %v", vs)
	}

	r := httptest.NewRequest(http.MethodGet, "/account/apple-profile/inventado", http.NoBody)
	w := httptest.NewRecorder()
	l.mux.ServeHTTP(w, r)
	if w.Code != http.StatusGone {
		t.Fatalf("un token inventado dio %d; quiero 410", w.Code)
	}
}
