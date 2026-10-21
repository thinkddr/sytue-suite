// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// SendDeviceControl uses the maintained Olm implementation and a concrete signed
// device, never a user-wide Megolm session. Call authorization remains in the
// controller, which pins this recipient to the authenticated live call lease.
func (c *Client) SendDeviceControl(ctx context.Context, user, device, kind string, content CallKeyContent) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return ErrClosed
	}
	if !c.ready.Load() {
		return ErrNotReady
	}
	if !ValidCallContent(kind, content, time.Now()) || user == "" || user == c.cfg.Bootstrap.ControllerUserID || device == "" {
		return ErrIdentity
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Verified {
		return ErrUnverified
	}
	machine := c.crypto.Machine()
	target, err := machine.GetOrFetchDevice(ctx, id.UserID(user), id.DeviceID(device))
	if err != nil {
		return err
	}
	if target == nil {
		return ErrIdentity
	}
	trust, err := machine.ResolveTrustContext(ctx, target)
	if err != nil {
		return err
	}
	if trust < id.TrustStateCrossSignedVerified {
		return ErrUnverified
	}
	return machine.SendEncryptedToDevice(ctx, target, deviceControlType(kind), event.Content{Parsed: &content})
}

func deviceControlType(kind string) event.Type {
	return event.Type{Type: kind, Class: event.ToDeviceEventType}
}

// routeDeviceControls adds application routing around the upstream processor.
// Every encrypted event is decrypted exactly once by HandleEncryptedEvent, which
// also retains the SDK's room-key/secret handling. The filtered response still
// reaches the maintained sync pipeline for plaintext SAS events, OTK counts,
// timeline and membership. No Olm envelope or ratchet is implemented here.
func (c *Client) routeDeviceControls(ctx context.Context, response *mautrix.RespSync, since string) *mautrix.RespSync {
	if c.crypto == nil {
		return response
	}
	machine := c.crypto.Machine()
	machine.HandleDeviceLists(ctx, &response.DeviceLists, since)
	filtered := *response
	filtered.DeviceLists = response.DeviceLists
	filtered.DeviceLists.Changed = nil // already processed by the maintained machine
	filtered.ToDevice.Events = nil
	for _, encrypted := range response.ToDevice.Events {
		if encrypted.Type.Type != event.ToDeviceEncrypted.Type {
			filtered.ToDevice.Events = append(filtered.ToDevice.Events, encrypted)
			continue
		}
		if encrypted.ToUserID != "" && (encrypted.ToUserID != c.cli.UserID || encrypted.ToDeviceID != c.cli.DeviceID) {
			continue
		}
		encrypted.Type = event.ToDeviceEncrypted
		if encrypted.Content.ParseRaw(encrypted.Type) != nil {
			continue
		}
		clearEvent := machine.HandleEncryptedEvent(ctx, encrypted)
		if clearEvent == nil || !isControl(clearEvent.Type.Type) || clearEvent.SenderDevice == nil || clearEvent.Recipient != c.cli.UserID || clearEvent.Sender == id.UserID(c.cfg.Bootstrap.ControllerUserID) {
			continue
		}
		trust, err := machine.ResolveTrustContextWithKeys(ctx, clearEvent.SenderDevice, clearEvent.SenderDeviceKeys)
		if err != nil || trust < id.TrustStateCrossSignedVerified {
			continue
		}
		raw, err := json.Marshal(&clearEvent.Content)
		var content CallKeyContent
		if err != nil || json.Unmarshal(raw, &content) != nil || !ValidCallContent(clearEvent.Type.Type, content, time.Now()) {
			continue
		}
		status, err := c.verificationStatus(ctx)
		if err != nil || !status.Verified {
			continue
		}
		c.deliver(DecryptedEvent{Sender: clearEvent.Sender.String(), SenderDevice: clearEvent.SenderDevice.DeviceID.String(), Type: clearEvent.Type.Type, Content: raw, Trust: trust}, nil)
	}
	return &filtered
}
