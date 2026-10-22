// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
)

func TestAltchaScript_EstaEnLaCSP(t *testing.T) {
	sum := sha256.Sum256([]byte(altchaScript))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(cspScriptHashes, hash) {
		t.Fatalf("falta el hash de altchaScript en cspScriptHashes: %s", cspScriptHashes)
	}
}

func TestSignupFormHTML_CaptchaRender(t *testing.T) {
	conCaptcha := signupFormHTML("es", "/", "ana", "Ana", "+34 600", "sytue.eu", "", true)
	if !strings.Contains(conCaptcha, `id="altcha-box"`) {
		t.Error("signupFormHTML con captchaEnabled=true debe incluir el contenedor altcha-box")
	}
	if !strings.Contains(conCaptcha, `name="altcha"`) {
		t.Error("signupFormHTML con captchaEnabled=true debe incluir input altcha")
	}
	if !strings.Contains(conCaptcha, altchaScript) {
		t.Error("signupFormHTML con captchaEnabled=true debe incluir altchaScript")
	}
	// Se elige un @usuario, no se da un correo: el campo y el dominio que lo
	// completa tienen que estar, y el correo no (ADR 0040).
	if !strings.Contains(conCaptcha, `name="username"`) || !strings.Contains(conCaptcha, "@sytue.eu") {
		t.Error("el alta debe pedir el @usuario con su dominio al lado")
	}
	if strings.Contains(conCaptcha, `name="email"`) {
		t.Error("el alta ya no pide un correo")
	}
	// Y pide el móvil al que va el código (ADR 0041).
	if !strings.Contains(conCaptcha, `name="phone"`) || !strings.Contains(conCaptcha, `type="tel"`) {
		t.Error("el alta debe pedir el móvil")
	}

	sinCaptcha := signupFormHTML("es", "/", "ana", "Ana", "", "sytue.eu", "", false)
	if strings.Contains(sinCaptcha, `id="altcha-box"`) {
		t.Error("signupFormHTML con captchaEnabled=false no debe incluir altcha-box")
	}
	if strings.Contains(sinCaptcha, altchaScript) {
		t.Error("signupFormHTML con captchaEnabled=false no debe incluir altchaScript")
	}
}

func TestHandleSignupCaptcha_Endpoint(t *testing.T) {
	v := altcha.New([]byte("secreto-de-prueba-32-bytes-clave!"))
	s := &Server{
		cfg:     Config{SignupEnabled: true, SignupCaptchaEnabled: true},
		captcha: v,
		// Sin SMS no hay alta (ni su captcha): el de desarrollo basta aquí.
		sms: logSMS{log: slog.New(slog.NewTextHandler(io.Discard, nil))},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/signup/captcha", http.NoBody)
	s.handleSignupCaptcha(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleSignupCaptcha: status = %d, quiero 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, quiero application/json", ct)
	}

	var ch altcha.Challenge
	if err := json.NewDecoder(rec.Body).Decode(&ch); err != nil {
		t.Fatalf("json decode challenge: %v", err)
	}
	if ch.Algorithm != "SHA-256" || ch.Challenge == "" || ch.Salt == "" || ch.Signature == "" {
		t.Fatalf("reto devuelto incompleto: %+v", ch)
	}

	// Resolver el reto generado por el endpoint
	sol, err := altcha.SolveChallenge(ch)
	if err != nil {
		t.Fatalf("SolveChallenge: %v", err)
	}
	ok, err := v.Verify(context.Background(), sol)
	if !ok || err != nil {
		t.Fatalf("Verify(sol): ok=%v, err=%v", ok, err)
	}
}

func TestHandleSignupCaptcha_Disabled(t *testing.T) {
	s := &Server{
		cfg: Config{SignupEnabled: false},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/signup/captcha", http.NoBody)
	s.handleSignupCaptcha(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("handleSignupCaptcha cuando signup está desactivado: status=%d, quiero 404", rec.Code)
	}
}
