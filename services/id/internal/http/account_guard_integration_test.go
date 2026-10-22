// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Ninguna ruta de /account atiende sin sesión.
//
// Estas rutas NO llevan middleware de autenticación: el router las monta con el
// rate limiter y nada más, y «el guard de sesión y Origin vive en cada handler»
// (server.go). El patrón funciona —los diez handlers lo comprueban, verificado en
// la auditoría del 2026-08-18— pero no tiene red: quien añada el handler once y
// olvide el guard abre `/account` a cualquiera, y no lo delata ni el compilador
// ni el router, porque la ruta sigue existiendo y respondiendo.
//
// Esto es esa red. Recorre las rutas reales y comprueba que SIN cookie de sesión
// ninguna llega a ejecutar su trabajo. Al añadir una nueva hay que ponerla aquí:
// la lista es el contrato.
package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Las rutas de seguridad self-service, tal como las monta server.go.
var rutasDeCuenta = []struct {
	metodo string
	ruta   string
}{
	{http.MethodGet, "/account"},
	{http.MethodPost, "/account/devices/trust"},
	{http.MethodGet, "/account/apple.mobileconfig"},
	{http.MethodPost, "/account/password"},
	{http.MethodPost, "/account/profile"},
	{http.MethodPost, "/account/mfa/totp/enroll"},
	{http.MethodPost, "/account/mfa/totp/confirm"},
	{http.MethodPost, "/account/mfa/totp/disable"},
	{http.MethodPost, "/account/mfa/totp/recovery/regenerate"},
	{http.MethodPost, "/account/app-passwords"},
	{http.MethodPost, "/account/app-passwords/revoke"},
	{http.MethodPost, "/account/passkeys/begin"},
	{http.MethodPost, "/account/passkeys/finish"},
	{http.MethodPost, "/account/passkeys/delete"},
	{http.MethodPost, "/account/sessions/revoke"},
	{http.MethodGet, "/account/delete"},
	{http.MethodPost, "/account/delete"},
}

func TestCuentaNoAtiendeSinSesion(t *testing.T) {
	l := newLiveServer(t)

	for _, c := range rutasDeCuenta {
		t.Run(c.metodo+" "+c.ruta, func(t *testing.T) {
			cuerpo := strings.NewReader("")
			req := httptest.NewRequest(c.metodo, c.ruta, cuerpo)
			if c.metodo == http.MethodPost {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			rec := httptest.NewRecorder()
			l.mux.ServeHTTP(rec, req)

			// Lo que se guarda es que NO se atienda. Vale cualquier forma de negarse
			// —302 al login, 401, 403— y también un 400 por falta de Origin, que es
			// otra puerta cerrada. Lo que NO vale es un 2xx: eso significa que el
			// handler hizo su trabajo sin saber quién lo pedía.
			//
			// Un 404 tampoco vale: querría decir que la ruta cambió de sitio y este
			// test dejó de cubrirla sin que nadie se enterase, que es exactamente el
			// agujero que viene a tapar.
			switch cod := rec.Code; {
			case cod == http.StatusNotFound:
				t.Fatalf("%s %s → 404: la ruta ya no existe aquí y este test dejó de "+
					"protegerla; actualizar la lista", c.metodo, c.ruta)
			case cod >= 200 && cod < 300:
				t.Fatalf("%s %s → %d SIN SESIÓN: el handler atendió a un desconocido",
					c.metodo, c.ruta, cod)
			}
		})
	}
}
