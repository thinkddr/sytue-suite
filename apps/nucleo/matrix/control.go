// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/base64"
	"reflect"
	"time"

	"maunium.net/go/mautrix/event"
)

const (
	CallKeyEvent        = "org.sytue.rooms.call_key.v1"
	CallKeyRequestEvent = "org.sytue.rooms.call_key_request.v1"
)

type CallKeyContent struct {
	Version    int    `json:"v"`
	CallID     string `json:"call_id"`
	Generation int64  `json:"generation"`
	Algorithm  string `json:"algorithm"`
	ExpiresAt  int64  `json:"expires_at"`
	KeyBase64  string `json:"key_b64,omitempty"`
}

func init() {
	// This is the SDK's supported custom content schema extension, registered
	// before any client goroutine starts. All cryptography stays in the SDK.
	for _, name := range []string{CallKeyEvent, CallKeyRequestEvent} {
		event.TypeMap[controlType(name)] = reflect.TypeFor[CallKeyContent]()
		event.TypeMap[deviceControlType(name)] = reflect.TypeFor[CallKeyContent]()
	}
}

func controlType(name string) event.Type {
	return event.Type{Type: name, Class: event.MessageEventType}
}

// ValidCallContent matches the web client wire contract. The call controller must
// additionally bind call_id, participant, generation and replay policy to the
// authenticated active meeting; this parser alone does not authorize a key.
func ValidCallContent(kind string, content CallKeyContent, now time.Time) bool {
	if kind != CallKeyEvent && kind != CallKeyRequestEvent || content.Version != 1 || content.CallID == "" || len(content.CallID) > 256 || content.Generation < 0 || content.Generation > 9007199254740991 || content.Algorithm != "AES-GCM" || content.ExpiresAt <= now.UnixMilli() || content.ExpiresAt > now.Add(24*time.Hour).UnixMilli() {
		return false
	}
	if kind == CallKeyRequestEvent {
		return content.KeyBase64 == ""
	}
	key, err := base64.StdEncoding.Strict().DecodeString(content.KeyBase64)
	defer clear(key)
	return err == nil && len(key) == 32 && len(content.KeyBase64) == 44
}

func (c *Client) SendEncryptedControl(ctx context.Context, roomID, kind string, content CallKeyContent) (EventRef, error) {
	// Media leases authorize individual devices. A room message would also share
	// this key with an actor's other devices, including one replaced in the call.
	return EventRef{}, ErrUnsupported
}

func isControl(kind string) bool { return kind == CallKeyEvent || kind == CallKeyRequestEvent }
