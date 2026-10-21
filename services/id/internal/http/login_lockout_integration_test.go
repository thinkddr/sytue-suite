// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El lockout suave por cuenta: frena la fuerza bruta distribuida (muchas IP
// contra un mismo email, que el límite por IP no ve) SIN que un tercero pueda
// dejar a nadie fuera de la plataforma.
//
// Antes, el bucket se gastaba en cada POST /login —antes de mirar la contraseña—,
// así que quien conociera un correo podía mantenerlo en deuda y la persona no
// volvía a entrar aunque tecleara bien. Estos tres casos son justo esa frontera:
// el tope sigue existiendo, pero la contraseña correcta siempre pasa.
package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// postLoginDesde envía las credenciales desde una IP concreta. La IP importa: el
// bucket por IP del login (10/min) frenaría un bucle largo desde una sola, y lo
// que se quiere ejercitar aquí es el bucket por CUENTA, que es el que un ataque
// distribuido consume.
func postLoginDesde(t *testing.T, l *liveServer, ip, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"email": {email}, "password": {password}}
	r := httptest.NewRequest(http.MethodPost, "/login/", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// La cabecera solo cuenta si quien conecta es un proxy de confianza (ver
	// domain.TrustedProxies): se simula el salto de Traefik desde la red del clúster.
	r.RemoteAddr = "10.42.0.7:44321"
	r.Header.Set("X-Forwarded-For", ip)
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func bloqueado(rr *httptest.ResponseRecorder) bool {
	return strings.Contains(rr.Body.String(), i18n.T("en", "login.too_many_tries"))
}

func TestLoginLockoutNoSeLePuedeAplicarANadieDesdeFuera(t *testing.T) {
	l := newLiveServer(t)
	const clave = "clave-correcta-123"
	l.mkUser(t, "victima@id.test", clave)

	// (a) El tope por cuenta sigue vivo: con la contraseña MAL, la ráfaga de 5 se
	// agota y el siguiente intento dice «demasiados intentos» aunque cada uno
	// venga de una IP distinta.
	var seBloqueo bool
	for i := 0; i < 8; i++ {
		rr := postLoginDesde(t, l, "203.0.113."+strconv.Itoa(i+1), "victima@id.test", "no-es-esta")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("intento %d con contraseña mala: status=%d, quiero 401", i, rr.Code)
		}
		if bloqueado(rr) {
			seBloqueo = true
			break
		}
	}
	if !seBloqueo {
		t.Fatal("la fuerza bruta distribuida contra una cuenta no se frenó nunca")
	}

	// (b) Con el bucket agotado por esos intentos AJENOS, la contraseña CORRECTA
	// entra. Es el arreglo: nadie puede cerrarle la puerta a otro.
	rr := postLoginDesde(t, l, "198.51.100.7", "victima@id.test", clave)
	if rr.Code != http.StatusFound {
		t.Fatalf("contraseña correcta con el bucket agotado: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if !sessionCookieSet(rr) {
		t.Fatal("la contraseña correcta no abrió sesión")
	}

	// (c) Y los aciertos no gastan nada: veinte seguidos, ninguno bloqueado.
	for i := 0; i < 20; i++ {
		rr := postLoginDesde(t, l, "198.51.100."+strconv.Itoa(i+10), "victima@id.test", clave)
		if rr.Code != http.StatusFound {
			t.Fatalf("login correcto %d: status=%d cuerpo=%s", i, rr.Code, rr.Body)
		}
	}
}
