// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la verificación de email contra un PostgreSQL real
// (pgtest, build-tag `integration`): envío autenticado (token firmado de un solo
// uso, correo capturado, fila persistida), confirmación pública (GET pinta / POST
// marca email_verified), token válido/manipulado/caducado/reusado, no-op si ya
// está verificado, y reenvío rate-limitado. Reusa el harness newLiveServer y el
// captureMailer.
package http

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// mkUnverified crea un usuario SIN verificar (mkUser del harness los crea ya
// verificados; aquí necesitamos el estado de partida real de un alta sin invitación).
func (l *liveServer) mkUnverified(t *testing.T, email string) domain.User {
	t.Helper()
	u, err := l.st.CreateUser(l.ctx, nil, domain.User{Email: email, Name: "Sin verificar", EmailVerified: false})
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", email, err)
	}
	return u
}

func TestEmailVerification_SendVerifyFlow(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	u := l.mkUnverified(t, "unv@id.test")
	tok := l.token(t, u.ID, "", nil)

	// Enviar → 200 {sent:true} + correo capturado (enlace) + fila de token persistida.
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/email/verify/send", tok, nil)
	if rr.Code != http.StatusOK || out["sent"] != true {
		t.Fatalf("enviar verificación: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if mc.verifyLink == "" || mc.verifyTo != "unv@id.test" {
		t.Fatalf("verificación no enviada/normalizada: %+v", mc)
	}
	var n int
	if err := l.st.DB().QueryRow(l.ctx,
		`SELECT count(*) FROM email_verification_tokens WHERE user_id=$1`, u.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tokens de verificación del usuario = %d, quiero 1", n)
	}
	token := tokenFromLink(t, mc.verifyLink)

	// GET válido → 200; manipulado (primer carácter) → 400 (firma inválida).
	if rr := l.get(t, "/verify-email?token="+url.QueryEscape(token)); rr.Code != http.StatusOK {
		t.Fatalf("GET verify válido: status=%d", rr.Code)
	}
	tampered := "A" + token[1:]
	if token[0] == 'A' {
		tampered = "B" + token[1:]
	}
	if rr := l.get(t, "/verify-email?token="+url.QueryEscape(tampered)); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET verify manipulado: status=%d, quiero 400", rr.Code)
	}

	// POST → marca el correo verificado.
	if rr := l.postForm(t, "/verify-email", url.Values{"token": {token}}); rr.Code != http.StatusOK {
		t.Fatalf("POST verify válido: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	got, _ := l.st.GetUserByID(l.ctx, u.ID)
	if !got.EmailVerified {
		t.Fatal("email_verified debería ser true tras confirmar")
	}

	// Reuso del MISMO token (ya consumido) → 400 (un solo uso).
	if rr := l.postForm(t, "/verify-email", url.Values{"token": {token}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("reuso del token: status=%d, quiero 400", rr.Code)
	}
}

func TestEmailVerification_AlreadyVerifiedNoOp(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	u := l.mkUser(t, "verified@id.test", "") // mkUser ⇒ EmailVerified: true
	tok := l.token(t, u.ID, "", nil)

	rr, out := l.do(t, http.MethodPost, "/api/v1/me/email/verify/send", tok, nil)
	if rr.Code != http.StatusOK || out["sent"] != false || out["verified"] != true {
		t.Fatalf("ya verificado: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if mc.verifyLink != "" {
		t.Fatal("no debe enviarse correo si el email ya está verificado")
	}
}

func TestEmailVerification_ExpiredRejected(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUnverified(t, "late@id.test")
	// Token FIRMADO válido (clave del servidor) pero fila caducada.
	token, hash, err := domain.NewResetToken(l.srv.resetKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.InTx(l.ctx, func(q pg.Querier) error {
		return l.st.InsertEmailVerification(l.ctx, q, hash, u.ID, u.Email, store.VerifyLogin, time.Now().Add(-time.Minute))
	}); err != nil {
		t.Fatalf("InsertEmailVerification: %v", err)
	}
	if rr := l.postForm(t, "/verify-email", url.Values{"token": {token}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("token caducado: status=%d, quiero 400", rr.Code)
	}
	if got, _ := l.st.GetUserByID(l.ctx, u.ID); got.EmailVerified {
		t.Fatal("un token caducado no debe verificar el correo")
	}
}

func TestEmailVerification_ResendRateLimited(t *testing.T) {
	l := newLiveServer(t)
	l.srv.mailer = &captureMailer{}
	u := l.mkUnverified(t, "spam@id.test")
	tok := l.token(t, u.ID, "", nil)
	// loginLimiter = 10/min con ráfaga 10 por IP; agotada la ráfaga, la siguiente
	// petición desde la misma IP (RemoteAddr del test) se frena con 429.
	got429 := false
	for i := 0; i < 20; i++ {
		rr, _ := l.do(t, http.MethodPost, "/api/v1/me/email/verify/send", tok, nil)
		if rr.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("el reenvío debería frenarse por rate-limit (429) tras agotar la ráfaga")
	}
}
