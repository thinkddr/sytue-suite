// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourceEpochProvider can supply an already accepted domain plan or a stable
// first candidate. BeginSource always prefers its retained intent and accepted
// source state; this provider cannot replace either with another epoch.
type SourceEpochProvider interface {
	SnapshotEpoch(context.Context, storage.Scope) (string, error)
}

// InitialSourceEpoch is a metadata identifier, never an encryption key. A fresh
// device proposes the same first epoch for this exact scope. A conflicting old
// enrollment must still fail against the server's canonical scope binding.
func InitialSourceEpoch(scope storage.Scope) (string, error) {
	if !scope.Valid() {
		return "", ErrSource
	}
	return storage.InitialSourceEpoch(scope)
}
