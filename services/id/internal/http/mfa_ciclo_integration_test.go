// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El ciclo COMPLETO del segundo factor: /login/ → /login/mfa → /oauth2/authorize.
//
// Todo lo que rodea a este tramo estaba probado por separado —que sin `mfa` en el
// `amr` se exige step-up, que un código correcto abre sesión— pero nadie
// comprobaba que la sesión que abre el paso 2 sea aceptada DESPUÉS por /authorize.
// Los tests que necesitaban una sesión la insertaban a mano con el `amr` ya
// puesto, así que daban por bueno precisamente el eslabón que aquí se mide.
//
// Ese hueco tiene una forma concreta: si la sesión recién creada llega a
// /authorize sin `mfa` en su `amr`, la verja de plataforma la manda otra vez al
// formulario del código, que vuelve a abrir sesión, que vuelve a /authorize. Un
// bucle cerrado del que el usuario no sale, y con los healthchecks en verde.
package http

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// El mismo secreto que siembra enrollMFA: hace falta en claro para generar un
// código válido y recorrer el paso 2 como lo recorre una persona.
var secretoEnrolado = []byte("0123456789abcdef01234567")

func TestSegundoFactorRealAbreSesionQueAuthorizeAcepta(t *testing.T) {
	l := newLiveServer(t)
	const clave = "Contrasena-De-Prueba-1"
	u := l.mkUser(t, "ciclo-mfa@id.test", clave)
	l.enrollMFA(t, u.ID)

	// Con alcance de plataforma /authorize exige segundo factor SIEMPRE, salvo que
	// la sesión ya lo acredite. Es el caso que convierte un `amr` perdido en bucle.
	if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, u.ID, "prueba"); err != nil {
		t.Fatalf("UpsertPlatformAdmin: %v", err)
	}

	cont := l.loginStep1(t, "ciclo-mfa@id.test", clave)
	rr := l.loginStep2(t, totp.Code(secretoEnrolado, time.Now()), cont)
	if rr.Code != http.StatusFound {
		t.Fatalf("login paso 2: status=%d, quiero 302; cuerpo=%s", rr.Code, rr.Body)
	}
	// findCookie devuelve también la cookie de BORRADO, que tiene el mismo nombre y
	// el valor vacío: sin mirar el valor, un cierre de sesión pasaría por apertura.
	sess := findCookie(rr, sessionCookieName)
	if sess == nil || sess.Value == "" {
		t.Fatalf("el código correcto no emitió cookie de sesión: %v", sess)
	}

	loc, _ := l.authorizeLoc(t, sess)
	if strings.Contains(loc, "step_up=1") {
		t.Fatalf("BUCLE: /authorize vuelve a pedir el segundo factor a una sesión que acaba de darlo; Location=%q", loc)
	}
	if !strings.Contains(loc, "code=") {
		t.Fatalf("/authorize no emitió código tras un segundo factor correcto; Location=%q", loc)
	}
}
