// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

func scopedTokens(subject, tenant string) Tokens {
	return Tokens{
		Issuer:       "https://issuer.invalid",
		Cliente:      "native",
		Subject:      subject,
		Organizacion: tenant,
		Access:       "synthetic-" + subject,
		Refresh:      "synthetic-refresh-" + subject,
		Expiry:       time.Now().Add(time.Hour),
	}
}

func TestAuthorizationScopeRejectsHeaderReplacementAndABA(t *testing.T) {
	a, b := scopedTokens("ada", "first"), scopedTokens("grace", "second")
	session := NewSession(a, nil, nil)
	token, scope, err := session.AccessTokenScoped(context.Background())
	if err != nil || token != a.Access || !scope.Current("Bearer "+token) || scope.Current("Bearer "+b.Access) {
		t.Fatal("authorization and witness did not describe the same acquisition")
	}
	ctx := WithAuthorizationScope(context.Background(), scope)
	if RequestAuthorizationScope(ctx) != scope || RequestAuthorizationScope(context.Background()) != nil {
		t.Fatal("opaque scope was not carried only by its request context")
	}
	session.PonerApariencia("graphite", "round")
	if !scope.Current("Bearer " + token) {
		t.Fatal("appearance changed the authorization identity")
	}
	session.Sincronizar(b)
	session.Sincronizar(a)
	if scope.Current("Bearer " + token) {
		t.Fatal("A→B→A revived a previous authorization witness")
	}
	_, fresh, err := session.AccessTokenScoped(context.Background())
	if err != nil || !fresh.Current("Bearer "+a.Access) {
		t.Fatal("a new acquisition of A cannot authorize caching")
	}
}

func TestEmptyAuthorizationNeverCreatesACacheWitness(t *testing.T) {
	session := NewSession(Tokens{}, func(context.Context, Tokens) (Tokens, error) { return Tokens{}, nil }, nil)
	token, scope, err := session.AccessTokenScoped(context.Background())
	if err != nil || token != "" || scope != nil {
		t.Fatal("an empty token was given an authorization witness")
	}
}

func TestAuthorizationScopeAcquisitionIncludesRenewal(t *testing.T) {
	a, b := scopedTokens("ada", "first"), scopedTokens("grace", "second")
	a.Expiry = time.Time{}
	entered, resume := make(chan struct{}), make(chan struct{})
	session := NewSession(a, func(context.Context, Tokens) (Tokens, error) {
		close(entered)
		<-resume
		renewed := a
		renewed.Access, renewed.Expiry = "synthetic-renewed", time.Now().Add(time.Hour)
		return renewed, nil
	}, nil)
	type result struct {
		token string
		scope *AuthorizationScope
		err   error
	}
	done := make(chan result, 1)
	go func() {
		token, scope, err := session.AccessTokenScoped(context.Background())
		done <- result{token, scope, err}
	}()
	<-entered
	changed := make(chan struct{})
	go func() { session.Sincronizar(b); close(changed) }()
	close(resume)
	out := <-done
	<-changed
	if out.err != nil || out.token != "synthetic-renewed" || out.scope.Current("Bearer "+out.token) {
		t.Fatal("a session change during renewal was paired with a current witness")
	}
	// A change before acquisition instead obtains B and a new witness for B.
	token, scope, err := session.AccessTokenScoped(context.Background())
	if err != nil || token != b.Access || !scope.Current("Bearer "+token) {
		t.Fatal("a new acquisition did not follow the already adopted account")
	}
}

func TestSnapshotIdentityNeverMixesAdoptions(t *testing.T) {
	a, b := scopedTokens("ada", "first"), scopedTokens("grace", "second")
	b.Issuer, b.Cliente = "https://other-issuer.invalid", "other-native"
	session := NewSession(a, nil, nil)
	wantA := session.SnapshotIdentity()
	session.Sincronizar(b)
	wantB := session.SnapshotIdentity()
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 2000 {
			session.Sincronizar(a)
			session.Sincronizar(b)
		}
	})
	for range 2000 {
		if got := session.SnapshotIdentity(); got != wantA && got != wantB {
			t.Error("identity snapshot mixed two adoptions")
			break
		}
	}
	workers.Wait()
}

func TestSincronizarNewIssuerOrClientRetiresPreviousAccess(t *testing.T) {
	for _, field := range []string{"issuer", "client"} {
		t.Run(field, func(t *testing.T) {
			a := scopedTokens("ada", "first")
			session := NewSession(a, nil, nil)
			b := a
			b.Access = ""
			if field == "issuer" {
				b.Issuer = "https://other-issuer.invalid"
			} else {
				b.Cliente = "other-native"
			}
			session.Sincronizar(b)
			if session.tokens.Access != "" {
				t.Fatal("new issuer/client retained the old access token")
			}
		})
	}
}

// TestLosTestigosSonLosDeAutorizacion: lo que el cliente `api`, `offline` y las pantallas
// leen con `nucleo/autorizacion` es lo que emite esta sesión, con los mismos errores.
func TestLosTestigosSonLosDeAutorizacion(t *testing.T) {
	if !errors.Is(ErrLoginRequired, autorizacion.ErrLoginRequired) || !errors.Is(ErrAuthorizationChanged, autorizacion.ErrAuthorizationChanged) {
		t.Fatal("los errores de auth no son los de autorizacion")
	}
	session := NewSession(scopedTokens("ada", "first"), nil, nil)
	ctx := session.WithAuthorizationIntent(context.Background())
	intent := autorizacion.RequestAuthorizationIntent(ctx)
	if intent == nil || intent != RequestAuthorizationIntent(ctx) || !intent.Current() {
		t.Fatal("la intención de la sesión no se lee con autorizacion")
	}
	token, scope, err := session.AccessTokenScoped(ctx)
	if err != nil || autorizacion.RequestAuthorizationScope(WithAuthorizationScope(ctx, scope)) != scope || !intent.Allows(scope, "Bearer "+token) {
		t.Fatal("el testigo de la sesión no se lee con autorizacion")
	}
	if autorizacion.RequestAuthorizationIntent(WithoutAuthorizationIntent(ctx)) != nil {
		t.Fatal("WithoutAuthorizationIntent no quita la intención que lee autorizacion")
	}
}
