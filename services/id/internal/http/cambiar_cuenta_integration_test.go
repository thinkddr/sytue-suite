// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Cambiar el @usuario y el móvil desde Mi cuenta (mig 0126) contra un PostgreSQL real:
// los dos piden la contraseña; el @usuario anterior queda reservado para quien lo
// dejó y no se cambia dos veces en 30 días; el móvil nuevo no se fija sin el código, y
// que es de otra cuenta solo se dice después; y el tope global de SMS por hora.
package http

import (
	"net/http"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func codigoDeError(out map[string]any) string {
	if e, ok := out["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

func TestMiCuenta_CambiarElUsuario(t *testing.T) {
	l := newLiveServer(t)
	const pw = "una-contraseña-larga-2026"
	ana := l.mkUser(t, "ana@acme.example", pw)
	tok := l.token(t, ana.ID, "", nil)
	viejo := ana.Username

	for _, c := range []struct {
		username, password, code string
		status                   int
	}{
		{"ana.nueva", "mala", "", http.StatusUnauthorized},
		{"a", pw, "username_invalid", http.StatusBadRequest},
		{"soporte", pw, "username_taken", http.StatusConflict},
	} {
		rr, out := l.do(t, http.MethodPut, "/api/v1/me/username", tok, map[string]string{"username": c.username, "password": c.password})
		if rr.Code != c.status || (c.code != "" && codigoDeError(out) != c.code) {
			t.Fatalf("%q: status=%d %v", c.username, rr.Code, out)
		}
	}
	otra := l.mkUser(t, "bea@acme.example", pw)
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/username", tok, map[string]string{"username": "@" + otra.Username, "password": pw}); rr.Code != http.StatusConflict || codigoDeError(out) != "username_taken" {
		t.Fatalf("el de otra persona: status=%d %v", rr.Code, out)
	}

	l.sinLimites(t)
	rr, out := l.do(t, http.MethodPut, "/api/v1/me/username", tok, map[string]string{"username": "@Ana.Nueva", "password": pw})
	if rr.Code != http.StatusOK || out["username"] != "ana.nueva" {
		t.Fatalf("cambiar: status=%d %v", rr.Code, out)
	}
	if got, err := l.st.GetUserByUsername(l.ctx, "ana.nueva"); err != nil || got.ID != ana.ID {
		t.Fatalf("el nuevo no es suyo: %+v %v", got, err)
	}
	// El anterior queda reservado: otra persona no lo coge, ni eligiéndolo ni asignado.
	if libre, err := l.st.UsernameLibre(l.ctx, viejo); err != nil || libre {
		t.Fatalf("el @usuario que deja está libre para otros: %v %v", libre, err)
	}
	if libre, err := l.st.UsernameLibrePara(l.ctx, otra.ID, viejo); err != nil || libre {
		t.Fatalf("otra persona puede quedarse con el reservado: %v %v", libre, err)
	}
	if libre, err := l.st.UsernameLibrePara(l.ctx, ana.ID, viejo); err != nil || !libre {
		t.Fatalf("quien lo dejó no puede recuperarlo: %v %v", libre, err)
	}
	// Y no se cambia otra vez en 30 días.
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/username", tok, map[string]string{"username": "ana.otra", "password": pw}); rr.Code != http.StatusTooManyRequests || codigoDeError(out) != "username_recent" {
		t.Fatalf("segundo cambio: status=%d %v", rr.Code, out)
	}

	// En una cuenta personal el @usuario es su dirección: no se cambia desde aquí.
	l.sinLimites(t)
	if rr := l.alta(t, url.Values{"username": {"marta"}, "password": {pw}}); rr.Code != http.StatusOK {
		t.Fatalf("alta: %d", rr.Code)
	}
	marta, _ := l.st.GetUserByUsername(l.ctx, "marta")
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/username", l.token(t, marta.ID, "", nil), map[string]string{"username": "marta.otra", "password": pw}); rr.Code != http.StatusConflict || codigoDeError(out) != "username_is_address" {
		t.Fatalf("cuenta personal: status=%d %v", rr.Code, out)
	}
}

func TestMiCuenta_CambiarOAnadirElMovil(t *testing.T) {
	l := newLiveServer(t)
	sms := l.smsDePrueba()
	const pw = "una-contraseña-larga-2026"
	// Una cuenta de antes de la 0066: sin móvil.
	luis := l.mkUser(t, "luis@acme.example", pw)
	tok := l.token(t, luis.ID, "", nil)
	const movil = "+34611222333"

	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/phone", tok, map[string]string{"phone": movil, "password": "mala"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin contraseña: status=%d", rr.Code)
	}
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/phone", tok, map[string]string{"phone": "611222333", "password": pw}); rr.Code != http.StatusBadRequest || codigoDeError(out) != "phone_invalid" {
		t.Fatalf("sin prefijo: status=%d %v", rr.Code, out)
	}
	l.sinLimites(t)
	rr, out := l.do(t, http.MethodPut, "/api/v1/me/phone", tok, map[string]string{"phone": "+34 611 222 333", "password": pw})
	if rr.Code != http.StatusAccepted || out["pending"] != movil {
		t.Fatalf("pedir el código: status=%d %v", rr.Code, out)
	}
	if got := mustUser(t, l, luis.ID); got.Phone != "" {
		t.Fatal("el móvil no se fija antes del código")
	}
	// Otro código antes del minuto, no.
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/phone", tok, map[string]string{"phone": movil, "password": pw}); rr.Code != http.StatusTooManyRequests || codigoDeError(out) != "code_wait" {
		t.Fatalf("reenvío inmediato: status=%d %v", rr.Code, out)
	}
	if rr, out := l.do(t, http.MethodPost, "/api/v1/me/phone/verify", tok, map[string]string{"code": "000000"}); rr.Code != http.StatusBadRequest || codigoDeError(out) != "code_wrong" {
		t.Fatalf("código malo: status=%d %v", rr.Code, out)
	}
	if rr, out := l.do(t, http.MethodPost, "/api/v1/me/phone/verify", tok, map[string]string{"code": sms.codigo(t, movil)}); rr.Code != http.StatusOK || out["phone"] != movil {
		t.Fatalf("código bueno: status=%d %v", rr.Code, out)
	}
	if got := mustUser(t, l, luis.ID); got.Phone != movil {
		t.Fatalf("móvil sin fijar: %+v", got)
	}
	if rr := l.formulario(t, "/login/", url.Values{"email": {"+34 611 222 333"}, "password": {pw}}); rr.Code != http.StatusFound {
		t.Fatalf("entrar con el móvil nuevo: status=%d", rr.Code)
	}

	// El de otra cuenta se puede pedir (no delata a nadie) y solo tras el código se dice.
	l.sinLimites(t)
	eva := l.mkUser(t, "eva@acme.example", pw)
	tokEva := l.token(t, eva.ID, "", nil)
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/phone", tokEva, map[string]string{"phone": movil, "password": pw}); rr.Code != http.StatusAccepted {
		t.Fatalf("pedir un móvil con cuenta: status=%d", rr.Code)
	}
	if rr, out := l.do(t, http.MethodPost, "/api/v1/me/phone/verify", tokEva, map[string]string{"code": sms.codigo(t, movil)}); rr.Code != http.StatusConflict || codigoDeError(out) != "phone_has_account" {
		t.Fatalf("móvil de otra cuenta: status=%d %v", rr.Code, out)
	}
	if got := mustUser(t, l, eva.ID); got.Phone != "" {
		t.Fatal("se fijó el móvil de otra cuenta")
	}
	// Sin código pendiente, verificar es «pide otro».
	if rr, out := l.do(t, http.MethodPost, "/api/v1/me/phone/verify", tokEva, map[string]string{"code": "123456"}); rr.Code != http.StatusGone || codigoDeError(out) != "code_expired" {
		t.Fatalf("sin código pendiente: status=%d %v", rr.Code, out)
	}
}

// El tope global de SMS por hora frena aunque cada número y cada IP estén dentro de
// los suyos (ADR 0041), y lo frenado no llega a mandarse.
func TestMiCuenta_TopeGlobalDeSMS(t *testing.T) {
	l := newLiveServer(t)
	sms := l.smsDePrueba()
	l.srv.smsGlobalLimiter = pg.NewRateLimiter(l.st.DB(), pg.RateLimiterConfig{PerSecond: 1.0 / 3600, Burst: 1})
	const pw = "una-contraseña-larga-2026"
	a := l.mkUser(t, "a@acme.example", pw)
	b := l.mkUser(t, "b@acme.example", pw)
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/phone", l.token(t, a.ID, "", nil), map[string]string{"phone": "+34622000001", "password": pw}); rr.Code != http.StatusAccepted {
		t.Fatalf("primer SMS: status=%d", rr.Code)
	}
	rr, out := l.do(t, http.MethodPut, "/api/v1/me/phone", l.token(t, b.ID, "", nil), map[string]string{"phone": "+34622000002", "password": pw})
	if rr.Code != http.StatusTooManyRequests || codigoDeError(out) != "sms_limit" {
		t.Fatalf("pasado el tope global: status=%d %v", rr.Code, out)
	}
	sms.mu.Lock()
	_, mandado := sms.texto["+34622000002"]
	sms.mu.Unlock()
	if mandado {
		t.Fatal("pasado el tope se mandó el SMS")
	}
}
