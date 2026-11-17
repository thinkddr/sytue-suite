// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// RequireStorageAuthority compares server metadata with this real SDK's own
// verified root. Parsing an active authority never enrolls or pins that root.
func (s *Storage) RequireStorageAuthority(ctx context.Context, authority matrixproof.Authority) error {
	scope := s.identity.Scope
	if s.StorageIdentityVersion() != storage.IsolatedIdentity || authority.RequireActive(scope, authority.Generation) != nil || authority.UserID != s.identity.Binding.MatrixUserID {
		return ErrRejected
	}
	binding, err := s.PrepareStorage(ctx, scope)
	if err != nil {
		return err
	}
	if _, err = s.RequireRecoverableIngress(ctx, scope, binding.Target); err != nil {
		return err
	}
	var evidence struct {
		RootKey string `json:"root_key"`
	}
	if err := s.call(ctx, IdentityRoot, nil, &evidence); err != nil {
		return err
	}
	if evidence.RootKey != authority.RootKey {
		return ErrRejected
	}
	return ctx.Err()
}
