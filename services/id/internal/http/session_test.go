// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieIsHostOnlyAndHostPrefixed(t *testing.T) {
	s := &Server{cfg: Config{Issuer: "https://id.sytue.eu", CookieSecure: true}}
	recorder := httptest.NewRecorder()
	s.setSessionCookie(recorder, "opaque", time.Now().Add(time.Hour))
	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies=%d; se esperaban borrado legacy + cookie nueva", len(cookies))
	}
	legacy, current := cookies[0], cookies[1]
	if legacy.Name != sessionCookieName || legacy.Domain != "sytue.eu" || legacy.MaxAge != -1 {
		t.Fatalf("borrado legacy inesperado: %#v", legacy)
	}
	if current.Name != "__Host-"+sessionCookieName || current.Domain != "" ||
		current.Path != "/" || !current.Secure || !current.HttpOnly {
		t.Fatalf("cookie host-only inválida: %#v", current)
	}
}

// TestAmrHasSecondFactor fija qué cuenta como segundo factor en la verja anti-bypass del
// Acceso Condicional. Es la diferencia entre que una política de MFA obligatorio se
// cumpla en los cinco caminos de login o solo en el de contraseña.
func TestAmrHasSecondFactor(t *testing.T) {
	casos := map[string]struct {
		amr  []string
		want bool
	}{
		// Un solo factor: no basta.
		"solo contraseña": {[]string{"pwd"}, false},
		// SSO de terceros: el IdP de arriba puede haber pedido MFA o no, y no nos lo
		// dice. Asumir que sí sería confiar en algo que no se ha comprobado.
		"federado": {[]string{"federated"}, false},
		"saml":     {[]string{"saml"}, false},
		"vacío":    {nil, false},
		// Segundo factor explícito.
		"contraseña + mfa": {[]string{"pwd", "mfa"}, true},
		// Passkey: posesión + verificación de usuario, resistente a phishing. Pedirle
		// encima un TOTP sería añadir un factor MÁS DÉBIL sobre uno más fuerte.
		"passkey": {[]string{"webauthn"}, true},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			if got := amrHasSecondFactor(c.amr); got != c.want {
				t.Fatalf("amrHasSecondFactor(%v) = %v, quiero %v", c.amr, got, c.want)
			}
		})
	}
}
