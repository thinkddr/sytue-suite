// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPortRevocationJoinsEveryHookBeforeReturning(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	hooks := PortRevocations{
		func(context.Context, string) error { panic("fixture cleanup failure") },
		func(context.Context, string) error { close(entered); <-release; return nil },
	}
	go func() { done <- hooks.Revoke(ctx, strings.Repeat("a", 64)) }()
	<-entered
	cancel()
	select {
	case <-done:
		t.Fatal("cleanup returned while resource IO was still alive")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-done; err == nil || !errors.Is(err, context.Canceled) {
		t.Fatal("joined failures/cancellation omitted", err)
	}
}

func TestPortRevocationRejectsUnregisteredIdentifierShape(t *testing.T) {
	called := false
	hooks := PortRevocations{func(context.Context, string) error { called = true; return nil }}
	if err := hooks.Revoke(t.Context(), "request-url-or-token"); !errors.Is(err, ErrRejected) || called {
		t.Fatal("unvalidated port reached cleanup", err)
	}
}
