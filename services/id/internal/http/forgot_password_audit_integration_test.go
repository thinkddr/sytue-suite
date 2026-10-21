// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El rastro de auditoría del reseteo es un CONTRATO, no un detalle:
// `id verify-access` (deploy/production/04) da por bueno el correo transaccional
// exactamente cuando aparece `auth.password.reset_requested`. Si ese apunte se
// escribiera también con el envío fallido, la verificación de producción se
// volvería muda justo en el único camino de recuperación de una cuenta —que es
// el fallo que existe para cazar—.
package http

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mailerRoto simula un MSA caído o que rechaza el token del IdP.
type mailerRoto struct{}

func (mailerRoto) SendPasswordReset(context.Context, string, string) error {
	return errors.New("MSA inalcanzable")
}
func (mailerRoto) SendInvitation(context.Context, string, string, string) error { return nil }
func (mailerRoto) SendEmailVerification(context.Context, string, string) error  { return nil }
func (mailerRoto) SendPlatformScopeChange(context.Context, string, string, string, string, bool) error {
	return nil
}

func hayRastroDeReseteo(t *testing.T, l *liveServer, userID string) bool {
	t.Helper()
	const sql = `SELECT EXISTS (
		SELECT 1 FROM audit_log
		WHERE actor_id = $1::uuid AND action = 'auth.password.reset_requested')`
	var hay bool
	if err := l.st.DB().QueryRow(l.ctx, sql, userID).Scan(&hay); err != nil {
		t.Fatalf("leer auditoría: %v", err)
	}
	return hay
}

func TestResetPassword_AuditoriaSoloSiElCorreoSale(t *testing.T) {
	l := newLiveServer(t)

	// Envío roto: la respuesta es la misma (uniforme, no enumera) pero NO puede
	// quedar rastro, porque ese rastro es lo que el despliegue lee como «salió».
	l.srv.mailer = mailerRoto{}
	roto := l.mkUser(t, "reset-roto@id.test", "login-secret-123")
	l.srv.dispatchResetEmail(l.ctx, roto.Email, "127.0.0.1", "test")
	if hayRastroDeReseteo(t, l, roto.ID) {
		t.Fatal("un envío FALLIDO dejó rastro de éxito: la verificación de producción sería muda")
	}

	// Envío correcto: el rastro aparece y el enlace se emitió de verdad.
	mc := &captureMailer{}
	l.srv.mailer = mc
	bien := l.mkUser(t, "reset-bien@id.test", "login-secret-123")
	l.srv.dispatchResetEmail(l.ctx, bien.Email, "127.0.0.1", "test")
	if mc.resetLink == "" {
		t.Fatal("no se envió el enlace de reseteo")
	}
	// El apunte se escribe tras el envío; el harness es síncrono, pero se deja un
	// margen mínimo para no depender del orden de vaciado del pool.
	esperaRastro(t, l, bien.ID)
}

func esperaRastro(t *testing.T, l *liveServer, userID string) {
	t.Helper()
	fin := time.Now().Add(5 * time.Second)
	for {
		if hayRastroDeReseteo(t, l, userID) {
			return
		}
		if time.Now().After(fin) {
			t.Fatal("un envío CORRECTO no dejó rastro: 04 daría por roto el correo transaccional")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
