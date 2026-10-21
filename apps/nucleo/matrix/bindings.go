// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"slices"
	"strings"
)

// UpdateBindings accepts only another authenticated bootstrap for this identity.
// It extends routing after channel creation without replacing the crypto device.
func (c *Client) UpdateBindings(next Bootstrap) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if next.ControllerUserID != c.cfg.Bootstrap.ControllerUserID || next.Actor != c.cfg.Subject || next.Tenant != c.cfg.Tenant || next.MatrixUserID != c.cfg.Bootstrap.MatrixUserID || strings.TrimRight(next.HomeserverURL, "/") != c.cfg.Bootstrap.HomeserverURL || next.Encryption != c.cfg.Bootstrap.Encryption {
		return ErrIdentity
	}
	next.Channels = slices.Clone(next.Channels)
	next.DraftRoomIDs = slices.Clone(next.DraftRoomIDs)
	c.cfg.Bootstrap = next
	return nil
}

func (c *Client) RoomID(channel string) (string, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	for _, binding := range c.cfg.Bootstrap.Channels {
		if binding.ChannelID == channel {
			return binding.RoomID, nil
		}
	}
	return "", ErrIdentity
}

func (c *Client) DraftRooms() (string, []string) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.cfg.Bootstrap.DraftRoomID, slices.Clone(c.cfg.Bootstrap.DraftRoomIDs)
}

// Identity and DeviceID contain public identifiers, never keys or bearer tokens.
func (c *Client) Identity() (string, string) {
	return c.session.UserID.String(), c.session.Tenant
}
func (c *Client) DeviceID() string { return c.session.DeviceID.String() }

// WaitReady blocks until the maintained syncer has processed the first response.
func (c *Client) WaitReady(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	select {
	case <-c.firstSync:
		if c.closed.Load() {
			return ErrClosed
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
