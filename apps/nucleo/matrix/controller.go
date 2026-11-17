// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	sqlStore   *mxcrypto.SQLCryptoStore
	policy     string
}

func (c *Client) devicePolicy(store mxcrypto.Store) mxcrypto.Store {
	policy := controllerStore{Store: store, controller: id.UserID(c.cfg.Bootstrap.ControllerUserID)}
	if c.cfg.Bootstrap.StorageProduct != "" {
		policy.ownerOnly = c.cli.UserID
	}
	if sqlStore, ok := store.(*mxcrypto.SQLCryptoStore); ok {
		policy.sqlStore = sqlStore
		sum := sha256.Sum256([]byte("sytue-recipient-policy-v1\x00" + policy.controller.String() + "\x00" + policy.ownerOnly.String() + "\x00cross-signed-verified\x00no-key-share"))
		policy.policy = hex.EncodeToString(sum[:])
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

// rotateRestoredOutbound migrates sessions created under an older recipient
// policy. Current policy/SID evidence lives in the SAME SDK SQLite. Restart is
// not a membership change and must not discard a durably prepared operation.
func (c *Client) rotateRestoredOutbound(ctx context.Context) error {
	policy, err := c.currentOutboundRecipientPolicy()
	if err != nil {
		return err
	}
	rooms := map[id.RoomID]struct{}{}
	for _, binding := range c.cfg.Bootstrap.Channels {
		rooms[id.RoomID(binding.RoomID)] = struct{}{}
	}
	rooms[id.RoomID(c.cfg.Bootstrap.DraftRoomID)] = struct{}{}
	for _, room := range c.cfg.Bootstrap.DraftRoomIDs {
		rooms[id.RoomID(room)] = struct{}{}
	}
	rooms[id.RoomID(c.cfg.Bootstrap.StorageRoomID)] = struct{}{}
	for _, room := range c.cfg.Bootstrap.StorageRoomIDs {
		rooms[id.RoomID(room)] = struct{}{}
	}
	delete(rooms, "")
	return policy.ensureOutboundRecipientPolicy(ctx, rooms)
}
