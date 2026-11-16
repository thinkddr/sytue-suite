// SPDX-License-Identifier: Apache-2.0
package web

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

type constructorLoanPort struct {
	borrow             offline.WorkLease
	identity           webport.Identity
	mu                 sync.Mutex
	held               bool
	acquired, released int
	borrowed           bool
}

func (p *constructorLoanPort) ID() string { return strings.Repeat("a", 64) }
func (p *constructorLoanPort) Current(ctx context.Context) (webport.Identity, error) {
	return p.identity, ctx.Err()
}

func (p *constructorLoanPort) Call(ctx context.Context, _ storage.Scope, method webport.Method, _ []byte) ([]byte, error) {
	switch method {
	case webport.Authorize:
		return nil, ctx.Err()
	case webport.LeaseAcquire:
		p.mu.Lock()
		defer p.mu.Unlock()
		p.acquired++
		if p.held {
			return nil, offline.ErrWorkLease
		}
		p.held = true
		return json.Marshal(map[string]string{"lease": strings.Repeat("b", 64)})
	case webport.LeaseRelease:
		p.mu.Lock()
		defer p.mu.Unlock()
		p.held = false
		p.released++
		return nil, nil
	case webport.ServiceRequest:
		if p.borrow == nil {
			return nil, webport.ErrRejected
		}
		// The first authenticated HTTP operation lends the SAME real work witness.
		// A second host acquisition models the queued writer refusing another SH lock.
		release, err := p.borrow(ctx)
		if err != nil {
			return nil, err
		}
		release()
		p.borrowed = true
		return nil, webport.ErrRejected // This test deliberately ends before domain IO.
	default:
		return nil, webport.ErrRejected
	}
}

func TestMailConstructionLendsItsOriginalGateLeaseToAuthenticatedServiceWork(t *testing.T) {
	scope := storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}
	port := &constructorLoanPort{identity: webport.Identity{Version: 1, Scope: scope, Binding: webport.Binding{MatrixUserID: storage.IsolatedMatrixUserPrefix(scope) + ":matrix.test", DeviceID: "DEVICE"}}}
	core, err := webport.NewManager[mailCommand](webport.NewSources(), webport.NewPolicies(), "mail", func(ctx context.Context, input webport.ProductBuildContext) (webport.ManagedProduct[mailCommand], error) {
		port.borrow = input.Resources.WorkLease()
		return buildMailProduct(ctx, input)
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{core: core}
	_, err = manager.Open(t.Context(), Ports{Isolated: port})
	if !errors.Is(err, webport.ErrRejected) || !port.borrowed || port.acquired != 1 || port.released != 1 || port.held {
		t.Fatal("Mail construction lost its original gate witness or acquired another scope lock", err, port.borrowed, port.acquired, port.released, port.held)
	}
}
