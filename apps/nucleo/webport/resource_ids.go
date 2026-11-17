// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"crypto/rand"
	"encoding/hex"
)

func newCapabilityID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	clear(raw[:])
	return id, nil
}

// NewCapabilityID allocates the same opaque transient IDs used by every core
// owner registry; the caller must bind it to a verified port and scope.
func NewCapabilityID() (string, error) { return newCapabilityID() }
