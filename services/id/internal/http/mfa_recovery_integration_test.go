// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los códigos de recuperación TOTP: la confirmación del
// enrolamiento (API) devuelve el lote una sola vez, y en el login sirven como
// segundo factor de un solo uso. Reusa el harness newLiveServer.
package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// TestMeTOTPConfirmRecoveryCodes: al confirmar el enrolamiento se devuelven 10
// códigos de recuperación (una vez), quedan persistidos, y regenerarlos entrega un
// lote nuevo invalidando el anterior.
func TestMeTOTPConfirmRecoveryCodes(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "rc-api@id.test", "a-password-123")
	tok := l.token(t, u.ID, "", nil)

	_, out := l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/enroll", tok, nil)
	b32, _ := out["secret"].(string)
	secret, err := totp.DecodeSecret(b32)
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/confirm", tok,
		map[string]string{"code": totp.Code(secret, time.Now())})
	if rr.Code != http.StatusOK {
		t.Fatalf("confirm: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	codes, _ := out["recovery_codes"].([]any)
	if len(codes) != 10 {
		t.Fatalf("recovery_codes = %d, quiero 10", len(codes))
	}
	if n, _ := l.st.CountRecoveryCodes(l.ctx, u.ID); n != 10 {
		t.Fatalf("persistidos = %d, quiero 10", n)
	}

	// Guardar un código del primer lote para comprobar que la regeneración lo anula.
	first, _ := codes[0].(string)

	rr, out = l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/recovery-codes", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("regenerate: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if newCodes, _ := out["recovery_codes"].([]any); len(newCodes) != 10 {
		t.Fatalf("recovery_codes regenerados = %d, quiero 10", len(newCodes))
	}
	// El código del lote anterior ya no debe poder consumirse (regenerar lo anuló).
	norm := domain.NormalizeRecoveryCode(first)
	if used, _ := l.st.ConsumeRecoveryCode(l.ctx, nil, u.ID, domain.HashToken(norm)); used {
		t.Fatal("un código del lote anterior no debería servir tras regenerar")
	}
}

// TestLoginRecoveryCodeSingleUse: con TOTP activo, un código de recuperación supera
// el segundo factor del login UNA vez; reutilizarlo falla y no vuelve a abrir sesión.
func TestLoginRecoveryCodeSingleUse(t *testing.T) {
	l := newLiveServer(t)
	const pass = "a-password-123"
	u := l.mkUser(t, "rc-login@id.test", pass)

	// Activar TOTP directamente (secreto confirmado + mfa_enabled) e insertar un
	// lote de códigos conocido.
	secret := []byte("0123456789abcdefghij")
	if err := l.st.UpsertTOTPSecret(l.ctx, nil, u.ID, secret, true); err != nil {
		t.Fatalf("UpsertTOTPSecret: %v", err)
	}
	if err := l.st.SetMFAEnabled(l.ctx, nil, u.ID, true); err != nil {
		t.Fatalf("SetMFAEnabled: %v", err)
	}
	display, hashes, err := domain.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("NewRecoveryCodes: %v", err)
	}
	if err := l.st.InsertRecoveryCodes(l.ctx, nil, u.ID, hashes); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}
	code := display[0] // "XXXXX-XXXXX"

	// Paso 1: contraseña → 302 a step-up + cookie de continuación MFA.
	mfaCookie := l.loginStep1(t, u.Email, pass)
	// Paso 2: código de recuperación → 302 (sesión creada) con cookie de sesión.
	rr := l.loginStep2(t, code, mfaCookie)
	if rr.Code != http.StatusFound {
		t.Fatalf("login con código de recuperación: status=%d, quiero 302; cuerpo=%s", rr.Code, rr.Body)
	}
	if !hasCookie(rr, sessionCookieName) {
		t.Fatal("no se abrió sesión (falta la cookie de sesión)")
	}
	if n, _ := l.st.CountRecoveryCodes(l.ctx, u.ID); n != 9 {
		t.Fatalf("códigos restantes = %d, quiero 9 (uno consumido)", n)
	}

	// Reuso: nueva continuación + el MISMO código → 401 y sin consumir otro.
	mfaCookie2 := l.loginStep1(t, u.Email, pass)
	rr = l.loginStep2(t, code, mfaCookie2)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("reuso del código: status=%d, quiero 401", rr.Code)
	}
	if n, _ := l.st.CountRecoveryCodes(l.ctx, u.ID); n != 9 {
		t.Fatalf("códigos tras reuso = %d, quiero 9 (sin cambios)", n)
	}
}

// loginStep1 postea las credenciales y devuelve la cookie de continuación MFA.
func (l *liveServer) loginStep1(t *testing.T, email, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"email": {email}, "password": {password}, "return_to": {"/account"}}
	rr := l.postFormRR(t, "/login/", form, nil)
	if rr.Code != http.StatusFound {
		t.Fatalf("login paso 1: status=%d, quiero 302; cuerpo=%s", rr.Code, rr.Body)
	}
	c := findCookie(rr, mfaCookieName)
	if c == nil {
		t.Fatal("login paso 1 no fijó la cookie de continuación MFA")
	}
	return c
}

// loginStep2 postea el segundo factor con la cookie de continuación adjunta.
func (l *liveServer) loginStep2(t *testing.T, code string, mfaCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"code": {code}, "return_to": {"/account"}}
	return l.postFormRR(t, "/login/mfa", form, mfaCookie)
}

// postFormRR postea un formulario opcionalmente con una cookie, con IP/UA por
// defecto estables (el binding IP/UA de la continuación debe coincidir entre pasos).
func (l *liveServer) postFormRR(t *testing.T, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func findCookie(rr *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func hasCookie(rr *httptest.ResponseRecorder, name string) bool {
	c := findCookie(rr, name)
	return c != nil && c.Value != ""
}
