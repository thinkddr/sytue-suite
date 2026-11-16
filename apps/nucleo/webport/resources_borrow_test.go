// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type borrowingResourcePort struct {
	sourcePort
	attempts, acquired, released, active atomic.Int32
}

func (p *borrowingResourcePort) Call(ctx context.Context, scope storage.Scope, method Method, _ []byte) ([]byte, error) {
	if scope != p.identity.Scope {
		return nil, ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch method {
	case Authorize:
		return nil, nil
	case LeaseAcquire:
		p.attempts.Add(1)
		// The real host has an outstanding owner/revocation waiter. A second
		// physical acquisition cannot stand in for borrowing the original witness.
		if !p.active.CompareAndSwap(0, 1) {
			return nil, offline.ErrWorkLease
		}
		p.acquired.Add(1)
		return json.Marshal(map[string]string{"lease": strings.Repeat("b", 64)})
	case LeaseRelease:
		if !p.active.CompareAndSwap(1, 0) {
			return nil, ErrRejected
		}
		p.released.Add(1)
		return nil, nil
	default:
		return nil, ErrRejected
	}
}

func borrowingResources(t *testing.T, port *borrowingResourcePort) *Resources {
	t.Helper()
	owner, err := OpenStorage(t.Context(), port, NewSources())
	if err != nil {
		t.Fatal(err)
	}
	resources, err := OpenResources(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resources.CloseAndDrain(t.Context()); err != nil {
			t.Error(err)
		}
	})
	return resources
}

func TestWebResourcesBorrowTheStableOriginalHostLeaseAndDrainBeforeRelease(t *testing.T) {
	port := &borrowingResourcePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: fixtureIdentity()}}
	resources := borrowingResources(t, port)
	outer := offline.NewWorkGateWithLease(resources.Context(), resources.WorkLease())
	defer outer.Close()
	original, finish, err := outer.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	nested := offline.NewWorkGateWithLease(resources.Context(), resources.WorkLease())
	defer nested.Close()
	inner, finishInner, err := nested.Start(original)
	if err != nil {
		t.Fatal("nested web domain reacquired original host lease", err)
	}
	defer finishInner()
	releaseSigner, err := resources.WorkLease()(inner)
	if err != nil {
		t.Fatal("web signer reacquired original host lease", err)
	}
	defer releaseSigner()
	if port.attempts.Load() != 1 || port.acquired.Load() != 1 {
		t.Fatal("web layers did not share one actual host witness")
	}
	other := borrowingResources(t, port)
	if release, err := other.WorkLease()(inner); !errors.Is(err, offline.ErrWorkLease) || release != nil {
		if release != nil {
			release()
		}
		t.Fatal("another resources lifetime borrowed original host witness", err)
	}
	attempted := port.attempts.Load()
	finished := make(chan struct{})
	go func() { finish(); close(finished) }()
	select {
	case <-original.Done():
	case <-time.After(time.Second):
		t.Fatal("outer web completion did not cancel nested IO")
	}
	if release, err := resources.WorkLease()(context.WithoutCancel(inner)); err == nil || release != nil {
		if release != nil {
			release()
		}
		t.Fatal("retired web operation revived a host lease", err)
	}
	if port.attempts.Load() != attempted || port.released.Load() != 0 {
		t.Fatal("outer web completion released host lease before nested borrowers drained")
	}
	releaseSigner()
	finishInner()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("original web host lease failed to drain")
	}
	if port.active.Load() != 0 || port.released.Load() != 1 {
		t.Fatal("web original host witness not released exactly once")
	}
}
