// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del INVARIANTE DE DESAPROVISIONAMIENTO contra PostgreSQL real
// (pgtest): dar de baja una cuenta —por cualquiera de los tres caminos de SCIM o por
// la Console— REVOCA sus credenciales, y una cuenta borrada no se puede resucitar.
//
// Hallazgo de la caza del patrón «autorización apoyada en la fila equivocada»:
// DeleteUser era un solo `UPDATE users SET status='deleted'` sin cascada, y UpdateUser
// —la vía por la que SCIM aplica `active:false`— no revocaba nada ni llevaba el guard
// de `deleted` que sí tenía SetUserStatus. Resultado: el offboarding estándar de
// empresa (Entra/Okta) dejaba la sesión SSO viva, y con ella se seguían obteniendo
// códigos, tokens y Assertions SAML firmadas hacia SPs de terceros.
//
// Las verjas complementarias en la capa HTTP (currentSession y redeemAuthCode) viven
// en internal/http; aquí se fija la parte de datos, que es la raíz.
package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestDeleteUserRevokesCredentials: el DELETE de SCIM revoca sesión y refresh.
func TestDeleteUserRevokesCredentials(t *testing.T) {
	st, ctx := newStore(t)
	ada := seedUser(t, ctx, st, "ada@acme.example")

	sessHash := hashOf("sess-ada")
	if _, err := st.InsertSession(ctx, nil, domain.Session{
		TokenHash: sessHash, UserID: ada.ID, AuthTime: time.Now().UTC(), AMR: []string{"pwd"},
		ExpiresAt: time.Now().UTC().Add(12 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if _, err := st.InsertRefreshToken(ctx, nil, domain.RefreshToken{
		TokenHash: hashOf("rt-ada"), ClientID: "sytue-console", UserID: ada.ID,
		Scope: "openid", AuthTime: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(720 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertRefreshToken: %v", err)
	}

	// Antes de la baja: la sesión es válida.
	if ss, err := st.GetSessionByToken(ctx, sessHash); err != nil {
		t.Fatalf("GetSessionByToken: %v", err)
	} else if !ss.Valid(time.Now()) {
		t.Fatal("la sesión debería ser válida antes de la baja")
	}

	// DESAPROVISIONAMIENTO (el DELETE /scim/v2/Users/{id}).
	if err := st.DeleteUser(ctx, nil, ada.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	ss, err := st.GetSessionByToken(ctx, sessHash)
	if err != nil {
		t.Fatalf("GetSessionByToken tras la baja: %v", err)
	}
	if ss.RevokedAt == nil {
		t.Fatal("el desaprovisionamiento DEBE revocar la sesión SSO: con ella se siguen obteniendo tokens y Assertions SAML")
	}
	rt, err := st.GetRefreshToken(ctx, nil, hashOf("rt-ada"))
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if rt.RevokedAt == nil {
		t.Fatal("el desaprovisionamiento DEBE revocar los refresh tokens en cascada")
	}
}

// TestSCIMDeactivateRevokesAndCannotResurrect: `active:false` (que SCIM aplica por
// UpdateUser) revoca; y una cuenta ya BORRADA no se puede devolver a la vida con un
// `active:true`.
func TestSCIMDeactivateRevokesAndCannotResurrect(t *testing.T) {
	st, ctx := newStore(t)

	// (1) active:false ⇒ suspendido + credenciales revocadas.
	bob := seedUser(t, ctx, st, "bob@acme.example")
	if _, err := st.InsertSession(ctx, nil, domain.Session{
		TokenHash: hashOf("sess-bob"), UserID: bob.ID, AuthTime: time.Now().UTC(), AMR: []string{"pwd"},
		ExpiresAt: time.Now().UTC().Add(12 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	bob.Status = domain.StatusSuspended
	if _, err := st.UpdateUser(ctx, nil, bob); err != nil {
		t.Fatalf("UpdateUser(suspended): %v", err)
	}
	ss, err := st.GetSessionByToken(ctx, hashOf("sess-bob"))
	if err != nil {
		t.Fatalf("GetSessionByToken: %v", err)
	}
	if ss.RevokedAt == nil {
		t.Fatal("SCIM active:false debe revocar la sesión (es la vía de UpdateUser, no de SetUserStatus)")
	}

	// (2) Una cuenta BORRADA no resucita con active:true.
	eve := seedUser(t, ctx, st, "eve@acme.example")
	if err := st.DeleteUser(ctx, nil, eve.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	eve.Status = domain.StatusActive
	if _, err := st.UpdateUser(ctx, nil, eve); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("un active:true sobre una cuenta borrada NO debe resucitarla; err=%v", err)
	}
	got, err := st.GetUserByID(ctx, eve.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Active() {
		t.Fatalf("la cuenta borrada sigue borrada; status=%q", got.Status)
	}
}
