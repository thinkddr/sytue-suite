// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Subject is the Sytue identity bound to this device's encrypted store.
func (c *Client) Subject() string { return c.cfg.Subject }

// NewCallKeys registers live Olm delivery before the first key request. Closing
// the call, device sync or account erases the epoch and removes the subscription.
func (c *Client) NewCallKeys(ctx context.Context, binding api.RoomsCallEncryption) (*CallKeys, error) {
	if err := c.WaitReady(ctx); err != nil {
		return nil, err
	}
	status, err := c.VerificationStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Verified {
		return nil, ErrUnverified
	}
	keys, err := NewCallKeys(ctx, c, c.Subject(), binding)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed.Load() || c.deliveryCtx == nil || c.deliveryCtx.Err() != nil {
		c.mu.Unlock()
		keys.Close()
		return nil, ErrClosed
	}
	if c.callKeys == nil {
		c.callKeys = map[*CallKeys]struct{}{}
	}
	c.callKeys[keys] = struct{}{}
	stop := context.AfterFunc(c.deliveryCtx, keys.Close) //nolint:contextcheck // Device-sync shutdown must also revoke keys owned by a longer-lived call context.
	c.mu.Unlock()
	context.AfterFunc(keys.ctx, func() { //nolint:contextcheck // Remove the registry entry at the authenticated key lease deadline, not just at caller cancellation.
		stop()
		c.mu.Lock()
		delete(c.callKeys, keys)
		c.mu.Unlock()
	})
	return keys, nil
}
