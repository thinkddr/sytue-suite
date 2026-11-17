// SPDX-License-Identifier: Apache-2.0
package storage

import "github.com/google/uuid"

// InitialSourceEpoch is a metadata identifier, never an encryption key. A fresh
// device proposes the same first epoch for this exact scope. A conflicting old
// enrollment still fails against the server's canonical scope binding.
func InitialSourceEpoch(scope Scope) (string, error) {
	if !scope.Valid() {
		return "", ErrRecord
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("eu.sytue.private-source.v1\x00"+scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product)).String(), nil
}
