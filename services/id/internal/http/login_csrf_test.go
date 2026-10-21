// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFormulariosDeLoginRechazanElOrigenAjeno cierra el login-CSRF: /account,
// /forgot-password y /reset-password ya comprobaban el origen, y los tres
// formularios de ACCESO —contraseña, segundo factor y «entrar con tu
// organización»— eran los únicos que no.
//
// Sin la comprobación, una página cualquiera podía enviar SUS credenciales desde
// el navegador de otra persona y dejarla dentro de la cuenta del atacante sin que
// se notara; a partir de ahí todo lo que esa persona escriba o suba en la suite
// cae en la cuenta ajena. Ni SameSite ni el bucket por IP lo tapan: la petición
// no necesita ninguna cookie previa.
//
// El guard va ANTES de mirar la base de datos, así que el Server de prueba puede
// ir sin store: si alguna vez se reordena y el handler intenta consultar, el test
// muere y se entera quien lo mueva.
func TestFormulariosDeLoginRechazanElOrigenAjeno(t *testing.T) {
	s := &Server{cfg: Config{Issuer: "https://id.sytue.eu", RPOrigins: []string{"https://id.sytue.eu"}}}
	rutas := map[string]struct {
		path    string
		handler http.HandlerFunc
	}{
		"contraseña":   {"/login/", s.handleLoginSubmit},
		"segundo paso": {"/login/mfa", s.handleMFAVerify},
		"organización": {"/login/sso", s.handleSSOSubmit},
	}
	for nombre, caso := range rutas {
		t.Run(nombre, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, caso.path,
				strings.NewReader("email=victima@sytue.eu&password=x&code=000000"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Origin", "https://evil.example")
			r.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			caso.handler(rec, r)
			if rec.Code != http.StatusForbidden {
				t.Errorf("POST %s desde otro origen: status=%d, quiero 403", caso.path, rec.Code)
			}
		})
	}
}
