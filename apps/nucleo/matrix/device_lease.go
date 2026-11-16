// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// ErrDeviceInUse keeps a second process from opening the same Olm device store.
// Callers can retry or hand the request to its existing authorized owner.
var ErrDeviceInUse = errors.New("matrix: private device is open in another process")

const deviceLeaseWait = 5 * time.Second

func (c *Client) acquireStore(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.storeMu.Lock()
	defer c.storeMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	return c.lockStore(ctx)
}

func (c *Client) loadStoreMaster(ctx context.Context) error {
	if err := c.acquireStore(ctx); err != nil {
		return err
	}
	return c.loadMaster(false)
}

// The caller holds storeMu. This lock lives outside the account directory so
// deleting a retired device cannot unlink the inode another process waits on.
func (c *Client) lockStore(ctx context.Context) error {
	if c.releaseStore != nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(c.cfg.StoreDir, ".device-leases")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, c.account+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	wait, cancel := context.WithTimeout(ctx, deviceLeaseWait)
	defer cancel()
	release, err := ajustes.CerrojoFichero(wait, file)
	if err != nil {
		if parentErr := ctx.Err(); parentErr != nil {
			return parentErr
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrDeviceInUse
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		release()
		return err
	}
	c.releaseStore = release
	return nil
}

func (c *Client) unlockStore() {
	if c.releaseStore != nil {
		c.releaseStore()
		c.releaseStore = nil
	}
}
