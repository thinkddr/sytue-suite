// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type storageCutoverRequest struct {
	Epoch   string             `json:"epoch"`
	Receipt *storage.Reference `json:"receipt,omitempty"`
}

func (r *Rooms) MatrixStorageMigration(ctx context.Context, scope storage.Scope) (storage.IdentityCutoverState, error) {
	return r.storageCutover(ctx, scope, nil)
}

func (r *Rooms) BeginMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.IdentityCutoverState, error) {
	return r.storageCutover(ctx, scope, &storageCutoverRequest{Epoch: epoch})
}

func (r *Rooms) CompleteMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	return r.storageCutover(ctx, scope, &storageCutoverRequest{Epoch: epoch, Receipt: &receipt})
}

func (r *Rooms) storageCutover(ctx context.Context, scope storage.Scope, body *storageCutoverRequest) (storage.IdentityCutoverState, error) {
	request, err := privateAppRequest(scope, scope.Product, "/matrix/storage/"+scope.Product+"/migration")
	if err != nil {
		return storage.IdentityCutoverState{}, err
	}
	if body != nil {
		if !storage.MigrationEpoch(body.Epoch) || body.Receipt != nil && !body.Receipt.Valid() {
			return storage.IdentityCutoverState{}, storage.ErrRecord
		}
		request.Method = http.MethodPost
		request.Header.Set("Content-Type", "application/json")
		request.Body, _ = json.Marshal(body)
		if body.Receipt != nil {
			request.Path += "/complete"
		}
	}
	state, err := JSON[storage.IdentityCutoverState](ctx, r.c, request)
	if err != nil {
		return storage.IdentityCutoverState{}, err
	}
	if err := state.Validate(scope); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	if body != nil {
		if state.Epoch != body.Epoch || state.Phase != "frozen" && state.Phase != "isolated" {
			return storage.IdentityCutoverState{}, storage.ErrRecord
		}
		if body.Receipt != nil && (state.Phase != "isolated" || state.Receipt == nil || *state.Receipt != *body.Receipt) {
			return storage.IdentityCutoverState{}, storage.ErrRecord
		}
	}
	return state, nil
}
