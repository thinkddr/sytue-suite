// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type authorityPort struct {
	identity Identity
	calls    int
	rejectAt int
	check    func(context.Context) error
	after    func()
}

func (p *authorityPort) ID() string { return "retained_source_port" }

func (p *authorityPort) Current(ctx context.Context) (Identity, error) { return p.identity, ctx.Err() }

func (p *authorityPort) Call(ctx context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if method != Authorize || scope != p.identity.Scope || len(raw) != 0 {
		return nil, ErrRejected
	}
	p.calls++
	if p.check != nil {
		if err := p.check(ctx); err != nil {
			return nil, err
		}
	}
	if p.after != nil {
		p.after()
	}
	if p.calls == p.rejectAt {
		return nil, ErrRejected
	}
	return nil, ctx.Err()
}

type readerAtFunc func([]byte, int64) (int, error)

func (f readerAtFunc) ReadAt(p []byte, o int64) (int, error) { return f(p, o) }

func TestRetainedSourceRequiresFreshAuthorityBeforeReadAndBeforeOutput(t *testing.T) {
	for _, rejectAt := range []int{0, 1, 2} {
		t.Run(map[int]string{0: "allowed", 1: "pre", 2: "post"}[rejectAt], func(t *testing.T) {
			identity := fixtureIdentity()
			port := &authorityPort{identity: identity, rejectAt: rejectAt}
			reads := 0
			var owned []byte
			reader := readerAtFunc(func(out []byte, _ int64) (int, error) { reads++; owned = out; return copy(out, "secret"), io.EOF })
			sources := NewSources()
			id, release, err := sources.Add(t.Context(), port, identity, reader, 6)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			out, err := sources.Read(t.Context(), port, identity, id, 0, 6)
			if rejectAt == 0 {
				if err != nil || string(out) != "secret" || port.calls != 2 || reads != 1 {
					t.Fatal("fresh guards missing", err, port.calls, reads)
				}
				clear(out)
			} else {
				if !errors.Is(err, ErrRejected) || out != nil {
					t.Fatal("revoked authority released retained bytes", err)
				}
				if rejectAt == 1 && reads != 0 {
					t.Fatal("reader ran before fresh authority")
				}
				if rejectAt == 2 && (reads != 1 || !bytes.Equal(owned, make([]byte, 6))) {
					t.Fatal("post-denied owned bytes were not erased")
				}
			}
		})
	}
}

func TestFreshAuthorityRejectsIdentityChangeAndCancellationDuringHostGuard(t *testing.T) {
	for _, fault := range []string{"identity", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			identity := fixtureIdentity()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			port := &authorityPort{identity: identity}
			port.after = func() {
				if fault == "identity" {
					port.identity.Binding.DeviceID = "FOREIGN"
				} else {
					cancel()
				}
			}
			if err := RevalidateIdentity(ctx, port, identity); err == nil {
				t.Fatal("authority result survived changed identity/lifetime")
			}
		})
	}
}

func TestPublicationCallbackRequiresFreshAuthorityOnBothSides(t *testing.T) {
	for _, rejectAt := range []int{1, 2} {
		identity := fixtureIdentity()
		port := &authorityPort{identity: identity, rejectAt: rejectAt}
		target := storage.PublicationTarget{RoomID: "!room:test", DeviceID: identity.Binding.DeviceID}
		called := 0
		policies := NewPolicies()
		id, release, err := policies.Add(t.Context(), port, identity, target, func(context.Context) error { called++; return nil })
		if err != nil {
			t.Fatal(err)
		}
		if err = policies.Authorize(t.Context(), port, identity, id, target, "stable"); !errors.Is(err, ErrRejected) {
			t.Fatal("callback fresh guard bypassed", err)
		}
		release()
		if called != rejectAt-1 {
			t.Fatal("callback ran before pre-authorization", called)
		}
	}
}

func TestRetainedSourceKeepsOriginalContextValuesAgainstRPCCaller(t *testing.T) {
	type principalKey struct{}
	original := context.WithValue(t.Context(), principalKey{}, "original")
	caller := context.WithValue(t.Context(), principalKey{}, "replacement")
	identity := fixtureIdentity()
	port := &authorityPort{identity: identity}
	port.check = func(ctx context.Context) error {
		if ctx.Value(principalKey{}) != "original" {
			return ErrRejected
		}
		return nil
	}
	sources := NewSources()
	id, release, err := sources.Add(original, port, identity, bytes.NewReader([]byte("secret")), 6)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	out, err := sources.Read(caller, port, identity, id, 0, 6)
	defer clear(out)
	if err != nil || string(out) != "secret" || port.calls != 2 {
		t.Fatal("source caller replaced original authority context", err)
	}
}
