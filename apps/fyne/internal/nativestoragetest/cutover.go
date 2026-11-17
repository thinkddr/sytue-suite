// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package nativestoragetest

import (
	"context"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// NewCutover retains a disposable server-side routing port. Begin freezes the
// actual historical Matrix room; it never forges a client status or recovery ACK.
func NewCutover(scope storage.Scope, controller *mautrix.Client, source id.RoomID) *Cutover {
	return &Cutover{state: storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{source.String()}}, controller: controller, source: source}
}

type Cutover struct {
	mu         sync.Mutex
	state      storage.IdentityCutoverState
	controller *mautrix.Client
	source     id.RoomID
}

func (r *Cutover) MatrixStorageMigration(ctx context.Context, scope storage.Scope) (storage.IdentityCutoverState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope != r.state.Scope {
		return storage.IdentityCutoverState{}, records.ErrSource
	}
	return r.state, ctx.Err()
}

func (r *Cutover) BeginMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.IdentityCutoverState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope != r.state.Scope || r.state.Phase != "legacy" {
		return storage.IdentityCutoverState{}, records.ErrSource
	}
	if _, err := r.controller.SendStateEvent(ctx, r.source, event.StatePowerLevels, "", matrixprooftest.StoragePowers(true)); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	r.state.Phase = "frozen"
	r.state.Epoch = epoch
	return r.state, nil
}

func (r *Cutover) CompleteMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope != r.state.Scope || epoch != r.state.Epoch || r.state.Phase != "frozen" || !receipt.Valid() || receipt.RoomID == r.source.String() {
		return storage.IdentityCutoverState{}, records.ErrSource
	}
	r.state.Phase = "isolated"
	r.state.Receipt = &receipt
	return r.state, ctx.Err()
}
