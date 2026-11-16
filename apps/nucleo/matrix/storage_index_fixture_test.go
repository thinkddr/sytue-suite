// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// One controlled accepted index acknowledgment can be lost by every private app
// fixture. The maintained SDK and homeserver perform the actual write first.
type lostStorageIndexACK struct {
	*Client
	mu       sync.Mutex
	drop     bool
	accepted storage.Reference
}

func (c *lostStorageIndexACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := c.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil && c.drop {
		c.drop = false
		c.accepted = version.Reference
		return version, errors.New("synthetic lost accepted storage index ACK")
	}
	return version, err
}
