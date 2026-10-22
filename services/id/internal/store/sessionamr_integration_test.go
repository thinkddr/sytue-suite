// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El `amr` de una sesión es lo que un relying party usa para decidir si exige un
// segundo paso. El endpoint de token lo fijaba a «pwd» a secas pasara lo que
// pasara en el login, así que TODA política de MFA era insatisfacible: el admin
// global recibía 403 de la consola de plataforma por culpa de su propio token,
// que afirmaba que solo había habido contraseña.
//
// Contra un PostgreSQL real (pgtest, build-tag `integration`) porque lo que se
// prueba es que el valor sobrevive el viaje de ida y vuelta por la columna.
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestSessionAMRDevuelveLoQueSeGuardo(t *testing.T) {
	st, ctx := newStore(t)
	u := seedUser(t, ctx, st, "amr@sytue.eu")

	casos := map[string][]string{
		"solo contraseña":        {"pwd"},
		"contraseña y TOTP":      {"pwd", "mfa"},
		"contraseña y passkey":   {"pwd", "webauthn"},
		"passkey sin contraseña": {"webauthn"},
	}
	for nombre, amr := range casos {
		t.Run(nombre, func(t *testing.T) {
			ss := domain.Session{
				TokenHash: "hash-" + nombre,
				UserID:    u.ID,
				AuthTime:  time.Now().UTC(),
				AMR:       amr,
				ExpiresAt: time.Now().Add(time.Hour),
			}
			id, err := st.InsertSession(ctx, nil, ss)
			if err != nil {
				t.Fatalf("InsertSession: %v", err)
			}
			leido, err := st.SessionAMR(ctx, id)
			if err != nil {
				t.Fatalf("SessionAMR: %v", err)
			}
			if len(leido) != len(amr) {
				t.Fatalf("amr = %v; se guardó %v", leido, amr)
			}
			for i := range amr {
				if leido[i] != amr[i] {
					t.Fatalf("amr = %v; se guardó %v", leido, amr)
				}
			}
		})
	}
}

// Una sesión revocada no puede seguir acreditando un segundo factor: si el
// logout no cortara esto, un refresh token de una sesión cerrada seguiría
// emitiendo tokens que dicen «esta persona pasó por MFA».
func TestSessionAMRNoAcreditaSesionRevocada(t *testing.T) {
	st, ctx := newStore(t)
	u := seedUser(t, ctx, st, "amr-revocada@sytue.eu")

	id, err := st.InsertSession(ctx, nil, domain.Session{
		TokenHash: "hash-revocada",
		UserID:    u.ID,
		AuthTime:  time.Now().UTC(),
		AMR:       []string{"pwd", "mfa"},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if err := st.RevokeSession(ctx, nil, "hash-revocada"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	if _, err := st.SessionAMR(ctx, id); err == nil {
		t.Fatal("una sesión revocada no debe devolver amr")
	}
}

// Sin sesión no se inventa nada: es preferible un token sin `amr` a uno que
// afirme un método de autenticación que no ocurrió.
func TestSessionAMRSinSesionNoInventa(t *testing.T) {
	st, ctx := newStore(t)
	amr, err := st.SessionAMR(ctx, "")
	if err != nil {
		t.Fatalf("SessionAMR(\"\"): %v", err)
	}
	if len(amr) != 0 {
		t.Fatalf("amr = %v; se esperaba vacío", amr)
	}
}
