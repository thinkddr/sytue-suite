// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPublicationPoliciesStayWithOriginalPortOwnerTarget(t *testing.T) {
	identity := fixtureIdentity()
	target := storage.PublicationTarget{RoomID: "!room:test", DeviceID: identity.Binding.DeviceID}
	port := sourcePort{id: strings.Repeat("a", 64), identity: identity}
	registry := NewPolicies()
	calls := 0
	id, release, err := registry.Add(t.Context(), port, identity, target, func(context.Context) error { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	foreign := identity
	foreign.Scope.Subject = "other"
	for _, attempt := range []struct {
		port     sourcePort
		identity Identity
		target   storage.PublicationTarget
	}{
		{sourcePort{id: strings.Repeat("b", 64), identity: identity}, identity, target},
		{port, foreign, target},
		{port, identity, storage.PublicationTarget{RoomID: "!foreign:test", DeviceID: identity.Binding.DeviceID}},
	} {
		if err = registry.Authorize(t.Context(), attempt.port, attempt.identity, id, attempt.target, "stable"); !errors.Is(err, ErrRejected) {
			t.Fatal("foreign callback admitted", err)
		}
	}
	if calls != 0 {
		t.Fatal("foreign call ran original policy")
	}
	if err = registry.Authorize(t.Context(), port, identity, id, target, "stable"); err != nil || calls != 1 {
		t.Fatal("original callback missing", err)
	}
	release()
	if err = registry.Authorize(t.Context(), port, identity, id, target, "stable"); !errors.Is(err, ErrRejected) || calls != 1 {
		t.Fatal("retired capability reused", err)
	}
}

func TestPublicationPolicyRevocationJoinsActualCallbackBeforeReturning(t *testing.T) {
	identity := fixtureIdentity()
	target := storage.PublicationTarget{RoomID: "!room:test", DeviceID: identity.Binding.DeviceID}
	port := sourcePort{id: strings.Repeat("a", 64), identity: identity}
	registry := NewPolicies()
	entered, finish := make(chan struct{}), make(chan struct{})
	id, release, err := registry.Add(t.Context(), port, identity, target, func(context.Context) error { close(entered); <-finish; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	call := make(chan error, 1)
	go func() { call <- registry.Authorize(t.Context(), port, identity, id, target, "stable") }()
	<-entered
	revoked := make(chan error, 1)
	go func() { revoked <- registry.RevokePort(t.Context(), port.ID()) }()
	select {
	case <-revoked:
		t.Fatal("revocation released real IO early")
	case <-time.After(20 * time.Millisecond):
	}
	if err = registry.Authorize(t.Context(), port, identity, id, target, "late"); !errors.Is(err, ErrRejected) {
		t.Fatal("new callback admitted during drain", err)
	}
	close(finish)
	if err = <-call; !errors.Is(err, context.Canceled) {
		t.Fatal("late policy reported success", err)
	}
	if err = <-revoked; err != nil {
		t.Fatal(err)
	}
}

func TestPublicationPolicyKeepsOriginalContextValuesAgainstRPCCaller(t *testing.T) {
	type principalKey struct{}
	original := context.WithValue(t.Context(), principalKey{}, "original")
	caller := context.WithValue(t.Context(), principalKey{}, "replacement")
	identity := fixtureIdentity()
	target := storage.PublicationTarget{RoomID: "!room:test", DeviceID: identity.Binding.DeviceID}
	port := sourcePort{id: strings.Repeat("a", 64), identity: identity}
	registry := NewPolicies()
	id, release, err := registry.Add(original, port, identity, target, func(ctx context.Context) error {
		if ctx.Value(principalKey{}) != "original" {
			return ErrRejected
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err = registry.Authorize(caller, port, identity, id, target, "stable"); err != nil {
		t.Fatal("original authorization context replaced", err)
	}
}
