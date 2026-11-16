// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// StorageAuthorityReader uses the existing private Rooms channel. Responses are
// public routing/proof metadata, not Matrix administrator or recovery keys.
type StorageAuthorityReader interface {
	ReadStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error)
	VerifyStorageAction(context.Context, matrixproof.Action, matrixproof.SignedAction) (matrixproof.Authorization, error)
}

func (c *Client) ReadStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	if !scope.Valid() {
		return matrixproof.Authority{}, ErrUnavailable
	}
	var out matrixproof.Authority
	if err := c.request(ctx, http.MethodPost, "/internal/storage-authority/state", struct {
		Scope storage.Scope `json:"scope"`
	}{scope}, &out); err != nil {
		return matrixproof.Authority{}, err
	}
	if out.Validate(scope) != nil {
		return matrixproof.Authority{}, ErrUnavailable
	}
	return out, nil
}

func (c *Client) VerifyStorageAction(ctx context.Context, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	if proof.Action != expected || !expected.Scope.Valid() {
		return matrixproof.Authorization{}, ErrUnavailable
	}
	var out matrixproof.Authorization
	if err := c.request(ctx, http.MethodPost, "/internal/storage-authority/verify", struct {
		Scope    storage.Scope            `json:"scope"`
		Expected matrixproof.Action       `json:"expected"`
		Proof    matrixproof.SignedAction `json:"proof"`
	}{expected.Scope, expected, proof}, &out); err != nil {
		return matrixproof.Authorization{}, err
	}
	if out.Validate(expected) != nil {
		return matrixproof.Authorization{}, ErrUnavailable
	}
	return out, nil
}
