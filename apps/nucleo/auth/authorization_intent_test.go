// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAuthorizationIntentRejectsAdoptionBeforeAcquisitionAndABA(t *testing.T) {
	a, b := scopedTokens("ada", "first"), scopedTokens("grace", "second")
	s := NewSession(a, nil, nil)
	ctx := s.WithAuthorizationIntent(context.Background())
	s.PonerApariencia("graphite", "round")
	if !RequestAuthorizationIntent(ctx).Current() {
		t.Fatal("appearance retired an authorization intent")
	}
	s.Sincronizar(b)
	if _, err := s.AccessToken(ctx); !errors.Is(err, ErrAuthorizationChanged) {
		t.Fatal("intent adopted the next account")
	}
	s.Sincronizar(a)
	if _, _, err := s.AccessTokenScoped(ctx); !errors.Is(err, ErrAuthorizationChanged) {
		t.Fatal("ABA revived an old intent")
	}
	other := NewSession(a, nil, nil)
	if _, _, err := other.AccessTokenScoped(s.WithAuthorizationIntent(context.Background())); !errors.Is(err, ErrAuthorizationChanged) {
		t.Fatal("intent authorized another Session instance")
	}
}

func TestAuthorizationIntentAllowsItsOwnRenewalButRejectsAnotherIdentity(t *testing.T) {
	for _, change := range []bool{false, true} {
		a := scopedTokens("ada", "first")
		a.Expiry = time.Time{}
		s := NewSession(a, func(context.Context, Tokens) (Tokens, error) {
			renewed := scopedTokens("ada", "first")
			renewed.Access = "synthetic-renewed"
			if change {
				renewed.Subject = "grace"
			}
			return renewed, nil
		}, nil)
		ctx := s.WithAuthorizationIntent(context.Background())
		token, scope, err := s.AccessTokenScoped(ctx)
		if change {
			if !errors.Is(err, ErrAuthorizationChanged) || token != "" || scope != nil {
				t.Fatal("renewal returned another identity's authorization")
			}
		} else if err != nil || token != "synthetic-renewed" || !scope.Current("Bearer "+token) || !RequestAuthorizationIntent(ctx).Current() {
			t.Fatal("same-identity renewal invalidated its own intent")
		}
	}
}
