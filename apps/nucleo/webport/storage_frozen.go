// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// FrozenStorageBindings asks the centrally bound legacy SDK for actual Matrix
// fences and checks the same routing again before exposing its snapshot.
func (s *Storage) FrozenStorageBindings(ctx context.Context) (storage.FrozenBinding, error) {
	var frozen storage.FrozenBinding
	if s.StorageIdentityVersion() != storage.LegacyIdentity {
		return frozen, ErrRejected
	}
	before, err := s.StorageBindings(ctx)
	if err != nil {
		return frozen, err
	}
	if err := s.call(ctx, StorageFrozen, nil, &frozen); err != nil {
		return storage.FrozenBinding{}, err
	}
	if !frozen.Valid() || !frozen.Binding.Same(before) {
		return storage.FrozenBinding{}, ErrRejected
	}
	after, err := s.StorageBindings(ctx)
	if err != nil {
		return storage.FrozenBinding{}, err
	}
	if !before.Same(after) {
		return storage.FrozenBinding{}, ErrRejected
	}
	return frozen, nil
}
