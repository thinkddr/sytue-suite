// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lifetimeValueKey struct{}

func TestWithoutAuthorizationIntentPreservesDeadlineCancellationScopeAndOtherValues(t *testing.T) {
	session := NewSession(scopedTokens("ada", "first"), nil, nil)
	parent, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	deadline := time.Now().Add(time.Hour)
	bounded, stop := context.WithDeadline(parent, deadline)
	defer stop()
	ctx := session.WithAuthorizationIntent(context.WithValue(bounded, lifetimeValueKey{}, "unrelated value"))
	token, scope, err := session.AccessTokenScoped(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx = WithAuthorizationScope(ctx, scope)
	intent := RequestAuthorizationIntent(ctx)
	life := WithoutAuthorizationIntent(ctx)
	if RequestAuthorizationIntent(life) != nil || RequestAuthorizationIntent(ctx) != intent || !intent.Current() {
		t.Fatal("short intent was retained or mutated")
	}
	if RequestAuthorizationScope(life) != scope || !scope.Current("Bearer "+token) || scope.Current("Bearer another-header") {
		t.Fatal("removing intent weakened request ScopeProof")
	}
	if actual, ok := life.Deadline(); !ok || !actual.Equal(deadline) || life.Done() != ctx.Done() || life.Value(lifetimeValueKey{}) != "unrelated value" {
		t.Fatal("native lifetime lost parent cancellation/deadline/values")
	}
	cause := errors.New("original presenter retired")
	cancel(cause)
	if !errors.Is(life.Err(), context.Canceled) || !errors.Is(context.Cause(life), cause) {
		t.Fatal("native lifetime detached from its original cancellation")
	}
}

func TestWithoutAuthorizationIntentKeepsFreshScopeProofAcrossSameIdentityRefresh(t *testing.T) {
	tokens := scopedTokens("ada", "first")
	session := NewSession(tokens, nil, nil)
	ctx := session.WithAuthorizationIntent(t.Context())
	life := WithoutAuthorizationIntent(ctx)
	_, oldScope, err := session.AccessTokenScoped(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A different native app updates the same persisted login. Short actions
	// expire, while the caller's separately verified work witness owns the life.
	tokens.Refresh = "synthetic-refresh-from-another-app"
	session.Sincronizar(tokens)
	if RequestAuthorizationIntent(ctx).Current() || oldScope.Current("Bearer "+tokens.Access) {
		t.Fatal("refresh revived a previous action/request witness")
	}
	if _, _, err := session.AccessTokenScoped(ctx); !errors.Is(err, ErrAuthorizationChanged) {
		t.Fatal("original short action stopped rejecting replacement", err)
	}
	token, fresh, err := session.AccessTokenScoped(life)
	if err != nil || fresh == nil || !fresh.Current("Bearer "+token) || fresh == oldScope {
		t.Fatal("long-lived bound caller could not obtain a fresh request proof", err)
	}
	session.Sincronizar(scopedTokens("grace", "second"))
	session.Sincronizar(tokens)
	if oldScope.Current("Bearer "+token) || fresh.Current("Bearer "+token) || RequestAuthorizationIntent(ctx).Current() {
		t.Fatal("helper weakened ABA rejection of existing action/request proofs")
	}
}
