// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type TimelinePage struct {
	Events []DecryptedEvent
	Next   string
}

// History reads/decrypts a page through the maintained Matrix transport. It never
// returns an unencrypted message or room key. Next is an opaque pagination token.
func (c *Client) History(ctx context.Context, roomID, from string, limit int) (TimelinePage, error) {
	if limit < 1 || limit > 100 {
		return TimelinePage{}, errors.New("matrix: invalid history page size")
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if err := c.requireEncrypted(ctx, roomID); err != nil {
		return TimelinePage{}, err
	}
	response, err := c.cli.Messages(ctx, id.RoomID(roomID), from, "", mautrix.DirectionBackward, nil, limit)
	if err != nil {
		return TimelinePage{}, err
	}
	page := TimelinePage{Next: response.End}
	for _, encrypted := range response.Chunk {
		if encrypted.Type == event.EventMessage {
			return TimelinePage{}, ErrNotEncrypted
		}
		if encrypted.Type != event.EventEncrypted {
			continue
		}
		if encrypted.RoomID != "" && encrypted.RoomID != id.RoomID(roomID) {
			return TimelinePage{}, ErrIdentity
		}
		encrypted.RoomID = id.RoomID(roomID)
		if err := encrypted.Content.ParseRaw(encrypted.Type); err != nil {
			return TimelinePage{}, err
		}
		decrypted, decryptErr := c.crypto.Decrypt(ctx, encrypted)
		if decryptErr != nil {
			return TimelinePage{}, decryptErr
		}
		if decrypted.Type != event.EventMessage {
			continue
		}
		value, decodeErr := c.decryptedEvent(ctx, decrypted)
		if decodeErr != nil {
			return TimelinePage{}, decodeErr
		}
		page.Events = append(page.Events, value)
	}
	return page, nil
}

func (c *Client) decryptedEvent(ctx context.Context, evt *event.Event) (DecryptedEvent, error) {
	if !c.allowedRoom(evt.RoomID.String()) {
		return DecryptedEvent{}, ErrIdentity
	}
	if !evt.Mautrix.WasEncrypted {
		return DecryptedEvent{}, ErrNotEncrypted
	}
	// Restored sessions retain the SDK's explicit backup trust label. They came
	// only from the SSSS-authenticated recovery path, never an untrusted import.
	trust := evt.Mautrix.TrustState
	if isControl(evt.Type.Type) {
		if trust < id.TrustStateCrossSignedVerified {
			return DecryptedEvent{}, ErrUnverified
		}
		var content CallKeyContent
		raw, err := json.Marshal(&evt.Content)
		if err != nil || json.Unmarshal(raw, &content) != nil || !ValidCallContent(evt.Type.Type, content, time.Now()) {
			return DecryptedEvent{}, ErrIdentity
		}
	}
	if trust < id.TrustStateCrossSignedVerified && trust != id.TrustStateBackup {
		return DecryptedEvent{}, ErrUnverified
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return DecryptedEvent{}, err
	}
	if !status.Verified {
		return DecryptedEvent{}, ErrUnverified
	}
	content, err := json.Marshal(&evt.Content)
	if err != nil {
		return DecryptedEvent{}, err
	}
	channel := ""
	for _, binding := range c.cfg.Bootstrap.Channels {
		if binding.RoomID == evt.RoomID.String() {
			channel = binding.ChannelID
			break
		}
	}
	return DecryptedEvent{ChannelID: channel, Reference: EventRef{RoomID: evt.RoomID.String(), EventID: evt.ID.String()}, Sender: evt.Sender.String(), Timestamp: evt.Timestamp, Type: evt.Type.Type, Content: content, Trust: trust, Recovered: trust == id.TrustStateBackup}, nil
}
