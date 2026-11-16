// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type retainedFixture struct {
	scope           storage.Scope
	drained, closed bool
}

func (w *retainedFixture) PrivateStorageScope() storage.Scope { return w.scope }
func (w *retainedFixture) Pending(context.Context) (offline.PendingWork, error) {
	return offline.PendingWork{Scope: w.scope, Count: 1}, nil
}
func (w *retainedFixture) Drain(context.Context) error { w.drained = true; return nil }
func (w *retainedFixture) Resume() error               { return nil }
func (w *retainedFixture) Close()                      { w.closed = true }
func TestResourcesRetainRealWorkWitnessUntilExplicitReleaseOrPortCleanup(t *testing.T) {
	identity := fixtureIdentity()
	checks, acquired, released := 0, 0, 0
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, raw []byte) ([]byte, error) {
		switch method {
		case Authorize:
			checks++
			return nil, nil
		case LeaseAcquire:
			acquired++
			return json.Marshal(map[string]string{"lease": strings.Repeat("b", 64)})
		case LeaseRelease:
			released++
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	resources, err := OpenResources(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	release, err := resources.WorkLease()(t.Context())
	if err != nil || release == nil {
		t.Fatal("missing actual host lease", err)
	}
	release()
	release()
	if acquired != 1 || released != 1 || checks < 3 {
		t.Fatal("lease checks/release lost", checks, acquired, released)
	}
	work := &retainedFixture{scope: identity.Scope}
	unregister, err := resources.Register(t.Context(), work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = resources.Register(t.Context(), work); !errors.Is(err, ErrRejected) {
		t.Fatal("work double registered", err)
	}
	retainedRelease, err := resources.WorkLease()(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resources.RetirePort()
	if err = resources.CloseAndDrain(t.Context()); err != nil || !work.drained || work.closed {
		t.Fatal("pending work discarded instead of drained", err)
	}
	retainedRelease()
	if released != 1 {
		t.Fatal("retiring port sent release before Go ACK; host cleanup owns it", released)
	}
	if _, err = resources.WorkLease()(t.Context()); err == nil {
		t.Fatal("retiring session admitted lease")
	}
	unregister()
	unregister()
}

func TestResourcesRejectMissingLeaseAndForeignOrTypedNilWork(t *testing.T) {
	identity := fixtureIdentity()
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		if method == Authorize {
			return nil, nil
		}
		if method == LeaseAcquire {
			return []byte("{}"), nil
		}
		return nil, ErrRejected
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	resources, err := OpenResources(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = resources.WorkLease()(t.Context()); !errors.Is(err, offline.ErrWorkLease) {
		t.Fatal("fake empty witness accepted", err)
	}
	var empty *retainedFixture
	if _, err = resources.Register(t.Context(), empty); !errors.Is(err, ErrRejected) {
		t.Fatal("typed nil work accepted", err)
	}
	foreign := &retainedFixture{scope: identity.Scope}
	foreign.scope.Subject = "other"
	if _, err = resources.Register(t.Context(), foreign); !errors.Is(err, ErrRejected) {
		t.Fatal("foreign private work admitted", err)
	}
}

func TestResourcesConstructionCancellationDoesNotReplaceThePersistentWorkLifetime(t *testing.T) {
	identity := fixtureIdentity()
	released := 0
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		switch method {
		case Authorize:
			return nil, nil
		case LeaseAcquire:
			return json.Marshal(map[string]string{"lease": strings.Repeat("b", 64)})
		case LeaseRelease:
			released++
			return nil, nil
		default:
			return nil, ErrRejected
		}
	}}
	owner, _ := OpenStorage(t.Context(), port, NewSources())
	construction, cancel := context.WithCancel(t.Context())
	life, stop := context.WithCancel(t.Context())
	defer stop()
	resources, err := OpenResourcesForLifetime(construction, life, owner)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = resources.Check(t.Context()); err != nil {
		t.Fatal("opening RPC owned the persistent session lifetime", err)
	}
	release, err := resources.WorkLease()(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = resources.CloseAndDrain(t.Context()); err != nil {
		t.Fatal(err)
	}
	release()
	if released != 1 {
		t.Fatal("ordinary failed-open cleanup retained an already joined host witness", released)
	}
}
