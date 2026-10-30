// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"strings"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

// controllerStore excludes the fixed membership controller at the SDK's device
// lookup boundary. An empty (non-nil) inventory prevents fallback key fetching,
// including for a controller device registered and cross-signed in the future.
// All other persistence, key handling and crypto remain in the maintained store.
type controllerStore struct {
	mxcrypto.Store
	controller id.UserID
	ownerOnly  id.UserID
}

func (c *Client) devicePolicy(store mxcrypto.Store) mxcrypto.Store {
	policy := controllerStore{Store: store, controller: id.UserID(c.cfg.Bootstrap.ControllerUserID)}
	if c.cfg.Bootstrap.StorageProduct != "" {
		policy.ownerOnly = c.cli.UserID
	}
	return policy
}

func (s controllerStore) GetDevices(ctx context.Context, user id.UserID) (map[id.DeviceID]*id.Device, error) {
	if user == s.controller || s.ownerOnly != "" && user != s.ownerOnly {
		return map[id.DeviceID]*id.Device{}, nil
	}
	return s.Store.GetDevices(ctx, user)
}

func validController(bootstrap Bootstrap) bool {
	_, domain, ok := strings.Cut(bootstrap.MatrixUserID, ":")
	return ok && domain != "" && bootstrap.ControllerUserID == "@rooms_controller:"+domain && bootstrap.ControllerUserID != bootstrap.MatrixUserID
}

func (c *Client) UserForSubject(subject string) (string, error) {
	if subject == "" || strings.ContainsAny(subject, ":/@\x00 ") {
		return "", ErrIdentity
	}
	own, _ := c.Identity()
	_, domain, ok := strings.Cut(own, ":")
	if !ok {
		return "", ErrIdentity
	}
	return "@sytue_" + subject + ":" + domain, nil
}

// rotateRestoredOutbound discards cached sending sessions before using a restored
// device under the current recipient policy. Inbound history is preserved; the
// maintained SDK creates fresh sending sessions for the current trusted roster.
func (c *Client) rotateRestoredOutbound(ctx context.Context) error {
	rooms := map[string]struct{}{}
	for _, binding := range c.cfg.Bootstrap.Channels {
		rooms[binding.RoomID] = struct{}{}
	}
	rooms[c.cfg.Bootstrap.DraftRoomID] = struct{}{}
	for _, room := range c.cfg.Bootstrap.DraftRoomIDs {
		rooms[room] = struct{}{}
	}
	rooms[c.cfg.Bootstrap.StorageRoomID] = struct{}{}
	for _, room := range c.cfg.Bootstrap.StorageRoomIDs {
		rooms[room] = struct{}{}
	}
	delete(rooms, "")
	for room := range rooms {
		if err := c.crypto.Machine().CryptoStore.RemoveOutboundGroupSession(ctx, id.RoomID(room)); err != nil {
			return err
		}
	}
	return nil
}
