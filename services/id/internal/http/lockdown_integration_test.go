// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests del BLOQUEO DE EMERGENCIA (mig 0035) contra un PostgreSQL real. Lo que
// verifican es que la marca corta la EMISIÓN en sus dos puntos —abrir sesión y
// entregar tokens— y que se puede levantar: sin lo primero el interruptor es
// decorativo, y sin lo segundo es una puerta cerrada por dentro.
package http

import (
	"net/http"
	"net/url"
	"testing"
)

func TestLockdown_CierraLaEmisionYSeLevanta(t *testing.T) {
	l := newLiveServer(t)
	l.mkUser(t, "lockdown@id.test", "login-secret-123")

	credenciales := url.Values{
		"email":    {"lockdown@id.test"},
		"password": {"login-secret-123"},
	}

	// En reposo se entra con normalidad: la marca existe y está a false.
	rr := l.postForm(t, "/login/", credenciales)
	if !sessionCookieSet(rr) {
		t.Fatalf("sin bloqueo el login debe abrir sesión (code=%d)", rr.Code)
	}

	if err := l.st.SetLockdown(l.ctx, nil, true, "test", "sesión comprometida"); err != nil {
		t.Fatalf("SetLockdown(on): %v", err)
	}

	// Con el bloqueo puesto, la MISMA credencial correcta no abre sesión. Es el
	// punto del interruptor: rotar la clave de firma invalida lo emitido, pero
	// quien tiene la contraseña volvería a entrar acto seguido.
	rr = l.postForm(t, "/login/", credenciales)
	if sessionCookieSet(rr) {
		t.Fatal("con el bloqueo activo una credencial válida NO puede abrir sesión")
	}

	// Y el token endpoint deja de emitir, sea cual sea el grant.
	rr = l.postForm(t, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"lo-que-sea"}})
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("token endpoint bajo bloqueo: code=%d, quiero 503", rr.Code)
	}

	// El rescate devuelve el servicio: si no, el bloqueo sería irreversible.
	if err := l.st.SetLockdown(l.ctx, nil, false, "test", "incidente cerrado"); err != nil {
		t.Fatalf("SetLockdown(off): %v", err)
	}
	rr = l.postForm(t, "/login/", credenciales)
	if !sessionCookieSet(rr) {
		t.Fatalf("tras levantar el bloqueo se debe poder entrar (code=%d)", rr.Code)
	}
}
