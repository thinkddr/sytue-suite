// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequireStepUp vigila las dos condiciones y que hagan falta LAS DOS. El
// caso que de verdad importa es el tercero: segundo factor sí, pero de una
// sesión de hace días —que es exactamente lo que pasa con refresh, donde el
// access token es nuevo y la identificación es vieja—.
func TestRequireStepUp(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	pasa := false
	h := s.requireStepUp(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { pasa = true }))

	llamar := func(c tokenClaims) *httptest.ResponseRecorder {
		pasa = false
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/domain", http.NoBody)
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyClaims{}, c))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	ahora := time.Now()
	casos := []struct {
		nombre string
		claims tokenClaims
		quiero int
	}{
		{
			nombre: "con segundo factor y recién identificado",
			claims: tokenClaims{Subject: "u1", AMR: []string{"pwd", "webauthn"}, AuthTime: ahora.Add(-time.Minute)},
			quiero: http.StatusOK,
		},
		{
			nombre: "sin segundo factor aunque acabe de entrar",
			claims: tokenClaims{Subject: "u1", AMR: []string{"pwd"}, AuthTime: ahora},
			quiero: http.StatusUnauthorized,
		},
		{
			nombre: "con segundo factor pero de hace dos semanas",
			claims: tokenClaims{Subject: "u1", AMR: []string{"mfa"}, AuthTime: ahora.Add(-14 * 24 * time.Hour)},
			quiero: http.StatusUnauthorized,
		},
		{
			nombre: "sin auth_time (token viejo, antes del claim): no se da por bueno",
			claims: tokenClaims{Subject: "u1", AMR: []string{"mfa"}},
			quiero: http.StatusUnauthorized,
		},
		{
			nombre: "justo en el borde de la frescura",
			claims: tokenClaims{Subject: "u1", AMR: []string{"otp"}, AuthTime: ahora.Add(-frescuraStepUp + time.Second)},
			quiero: http.StatusOK,
		},
	}
	for _, c := range casos {
		w := llamar(c.claims)
		if w.Code != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, w.Code, c.quiero)
		}
		if (w.Code == http.StatusOK) != pasa {
			t.Errorf("%s: el handler de detrás se ejecutó = %v con código %d", c.nombre, pasa, w.Code)
		}
		if c.quiero != http.StatusOK {
			// Sin decir QUÉ falta, el cliente no puede arreglarlo: un 401 pelado
			// se confunde con «vuelve a iniciar sesión» y pierde la acción.
			cab := w.Header().Get("WWW-Authenticate")
			if !strings.Contains(cab, "insufficient_user_authentication") || !strings.Contains(cab, "max_age=") {
				t.Errorf("%s: WWW-Authenticate = %q, quiero el reto del RFC 9470 con max_age", c.nombre, cab)
			}
		}
	}

	// Sin claims no hay a quién medir: 401, y nunca pasar.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/domain", http.NoBody)
	w := httptest.NewRecorder()
	pasa = false
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || pasa {
		t.Errorf("sin claims: %d pasa=%v", w.Code, pasa)
	}
}
