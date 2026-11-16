// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

func TestPrivateBoundSessionKeepsSharedRefreshAndIndependentDrivePreference(t *testing.T) {
	s, work, tokens := privateSuiteFixture(t)
	tokens.Access = "synthetic-bound-access"
	tokens.Expiry = time.Now().Add(time.Hour)
	s.Sesion = auth.NewSession(tokens, nil, s.almacen.Guardar)
	witness, err := s.TestigoTrabajo(t.Context(), work.scope)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := s.SesionVinculadaPrivada(t.Context(), witness)
	if err != nil {
		t.Fatal(err)
	}
	scoped, ok := bound.(interface {
		AccessTokenScoped(context.Context) (string, *auth.AuthorizationScope, error)
	})
	if !ok {
		t.Fatal("bound adapter lost request scope proof")
	}
	tokens.Refresh = "synthetic-rotated-refresh"
	if err := s.almacen.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	s.Sesion.Sincronizar(tokens)
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.DriveSistemaDesactivado = true }); err != nil {
		t.Fatal(err)
	}
	token, proof, err := scoped.AccessTokenScoped(t.Context())
	if err != nil || token != "synthetic-bound-access" || !proof.Current("Bearer "+token) {
		t.Fatal("shared refresh or Drive preference invalidated private HTTP", err)
	}
	if err := witness.Validar(); err != nil {
		t.Fatal("refresh rotated persistent witness", err)
	}
}

func TestPrivateBoundSessionRejectsOldWitnessAndLifetimeBeforeRenewal(t *testing.T) {
	for _, kind := range []string{"account-aba", "closed-lifetime"} {
		t.Run(kind, func(t *testing.T) {
			s, work, tokens := privateSuiteFixture(t)
			calls := 0
			s.Sesion = auth.NewSession(tokens, func(context.Context, auth.Tokens) (auth.Tokens, error) {
				calls++
				return auth.Tokens{}, errors.New("unexpected token request")
			}, s.almacen.Guardar)
			witness, err := s.TestigoTrabajo(t.Context(), work.scope)
			if err != nil {
				t.Fatal(err)
			}
			life, cancel := context.WithCancel(t.Context())
			defer cancel()
			bound, err := s.SesionVinculadaPrivada(life, witness)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "account-aba" {
				for _, subject := range []string{"bea", "ana"} {
					tokens.Subject = subject
					if err := s.Sesion.AdoptarCon(t.Context(), tokens); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				cancel()
			}
			if _, err := bound.AccessToken(t.Context()); !errors.Is(err, nube.ErrOtraCuenta) || calls != 0 {
				t.Fatal("expired private authorization acquired a token", calls, err)
			}
		})
	}
}

func TestSharedHTTPAdapterChecksIdentityBeforeAndAfterTokenAcquisition(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before", false: "after"}[before], func(t *testing.T) {
			tokens := auth.Tokens{Issuer: "https://id.example", Cliente: auth.ClientID, Subject: "ana", Organizacion: "acme"}
			calls := 0
			current := !before
			session := auth.NewSession(tokens, func(context.Context, auth.Tokens) (auth.Tokens, error) {
				calls++
				current = false
				tokens.Access = "synthetic-acquired"
				tokens.Expiry = time.Now().Add(time.Hour)
				return tokens, nil
			}, nil)
			bound := sesionDriveSistema{sesion: session, identidad: session.SnapshotIdentity(), vigente: func(context.Context) bool { return current }}
			token, proof, err := bound.AccessTokenScoped(t.Context())
			if token != "" || proof != nil || !errors.Is(err, nube.ErrOtraCuenta) {
				t.Fatal("stale HTTP authorization returned a token", err)
			}
			expected := 0
			if !before {
				expected = 1
			}
			if calls != expected {
				t.Fatal("incorrect token acquisition order", calls, expected)
			}
		})
	}
}
