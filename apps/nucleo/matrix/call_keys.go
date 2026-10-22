// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// CallKeyTransport is authenticated SDK Olm delivery to an exact verified device.
// The server supplies only the roster/lease; it never receives a media key.
type CallKeyTransport interface {
	DeviceID() string
	UserForSubject(string) (string, error)
	SendDeviceControl(context.Context, string, string, string, CallKeyContent) error
}

// CallKeys owns one immutable call epoch. A new authenticated epoch requires a
// new instance, new key, new SFU connection and a fresh server ready acknowledgement.
type CallKeys struct {
	transport CallKeyTransport
	binding   api.RoomsCallEncryption
	subject   string
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	key       string
	ready     chan struct{}
	once      sync.Once
	requests  map[string]time.Time
}

func NewCallKeys(ctx context.Context, transport CallKeyTransport, subject string, binding api.RoomsCallEncryption) (*CallKeys, error) {
	if transport == nil || subject == "" || binding.CallID == "" || binding.MatrixRoomID == "" || binding.LiveKitRoom == "" || binding.Generation < 0 || binding.ExpiresAtMS <= time.Now().UnixMilli() || binding.ExpiresAtMS > time.Now().Add(10*time.Minute+time.Second).UnixMilli() {
		return nil, ErrIdentity
	}
	own, leader := false, false
	seen := map[string]struct{}{}
	for _, member := range binding.Members {
		if _, err := transport.UserForSubject(member.Subject); err != nil || member.DeviceID == "" {
			return nil, ErrIdentity
		}
		key := member.Subject + "\x00" + member.DeviceID
		if _, exists := seen[key]; exists {
			return nil, ErrIdentity
		}
		seen[key] = struct{}{}
		own = own || member.Subject == subject && member.DeviceID == transport.DeviceID()
		leader = leader || member.Subject == binding.LeaderSubject && member.DeviceID == binding.LeaderDeviceID
	}
	if !own || !leader {
		return nil, ErrIdentity
	}
	binding.Members = slices.Clone(binding.Members)
	lifetime, cancel := context.WithDeadline(ctx, time.UnixMilli(binding.ExpiresAtMS))
	keys := &CallKeys{transport: transport, binding: binding, subject: subject, ctx: lifetime, cancel: cancel, ready: make(chan struct{}), requests: map[string]time.Time{}}
	context.AfterFunc(lifetime, func() { keys.mu.Lock(); keys.key = ""; keys.mu.Unlock() })
	return keys, nil
}
func (k *CallKeys) Close() { k.cancel(); k.mu.Lock(); k.key = ""; k.mu.Unlock() }
func (k *CallKeys) isLeader() bool {
	return k.subject == k.binding.LeaderSubject && k.transport.DeviceID() == k.binding.LeaderDeviceID
}

func (k *CallKeys) content(key string) CallKeyContent {
	return CallKeyContent{Version: 1, CallID: k.binding.CallID, Generation: k.binding.Generation, Algorithm: "AES-GCM", ExpiresAt: k.binding.ExpiresAtMS, KeyBase64: key}
}

// Start never treats a user-wide Megolm room as a device authorization. Every
// target is selected from the current server lease and checked again by the SDK.
func (k *CallKeys) Start(ctx context.Context) error {
	if err := k.ctx.Err(); err != nil {
		return err
	}
	if !k.isLeader() {
		leader, err := k.transport.UserForSubject(k.binding.LeaderSubject)
		if err != nil {
			return err
		}
		return k.transport.SendDeviceControl(ctx, leader, k.binding.LeaderDeviceID, CallKeyRequestEvent, k.content(""))
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	key := base64.StdEncoding.EncodeToString(raw)
	clear(raw)
	k.mu.Lock()
	if err := k.ctx.Err(); err != nil {
		k.mu.Unlock()
		return err
	}
	if k.key != "" {
		k.mu.Unlock()
		return ErrIdentity
	}
	k.key = key
	k.mu.Unlock()
	for _, member := range k.binding.Members {
		if member.Subject == k.subject && member.DeviceID == k.transport.DeviceID() {
			continue
		}
		user, err := k.transport.UserForSubject(member.Subject)
		if err != nil {
			return err
		}
		if err := k.transport.SendDeviceControl(ctx, user, member.DeviceID, CallKeyEvent, k.content(key)); err != nil {
			return err
		}
	}
	k.once.Do(func() { close(k.ready) })
	return nil
}

// Receive accepts only SDK-authenticated live Olm controls. Backup/timeline
// material, old epochs, replaced devices and conflicting keys cannot authorize media.
func (k *CallKeys) Receive(value DecryptedEvent) bool {
	if k.ctx.Err() != nil || value.Recovered || value.Trust < id.TrustStateCrossSignedVerified || value.SenderDevice == "" || !isControl(value.Type) {
		return false
	}
	var content CallKeyContent
	if json.Unmarshal(value.Content, &content) != nil || !ValidCallContent(value.Type, content, time.Now()) || content.CallID != k.binding.CallID || content.Generation != k.binding.Generation || content.ExpiresAt != k.binding.ExpiresAtMS {
		return false
	}
	if value.Type == CallKeyRequestEvent {
		return k.respond(value)
	}
	leader, err := k.transport.UserForSubject(k.binding.LeaderSubject)
	if err != nil || value.Sender != leader || value.SenderDevice != k.binding.LeaderDeviceID || k.isLeader() {
		return false
	}
	k.mu.Lock()
	if k.key != "" && k.key != content.KeyBase64 {
		k.mu.Unlock()
		return false
	}
	k.key = content.KeyBase64
	k.once.Do(func() { close(k.ready) })
	k.mu.Unlock()
	return true
}

func (k *CallKeys) respond(value DecryptedEvent) bool {
	if !k.isLeader() {
		return false
	}
	allowed := false
	for _, member := range k.binding.Members {
		user, err := k.transport.UserForSubject(member.Subject)
		if err == nil && user == value.Sender && member.DeviceID == value.SenderDevice {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	k.mu.Lock()
	last := k.requests[value.Sender+"\x00"+value.SenderDevice]
	key := k.key
	if key == "" || time.Since(last) < time.Second {
		k.mu.Unlock()
		return false
	}
	k.requests[value.Sender+"\x00"+value.SenderDevice] = time.Now()
	k.mu.Unlock()
	go func() {
		_ = k.transport.SendDeviceControl(k.ctx, value.Sender, value.SenderDevice, CallKeyEvent, k.content(key))
	}()
	return true
}

// CurrentKey allows the call controller to reconcile authorization/rotation while
// waiting for a leader; it never blocks the UI or returns an expired epoch key.
func (k *CallKeys) CurrentKey() (string, time.Time, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.ctx.Err() != nil || k.key == "" {
		return "", time.Time{}, false
	}
	select {
	case <-k.ready:
		return k.key, time.UnixMilli(k.binding.ExpiresAtMS), true
	default:
		return "", time.Time{}, false
	}
}

func (k *CallKeys) Key(ctx context.Context) (string, time.Time, error) {
	select {
	case <-k.ready:
		k.mu.Lock()
		defer k.mu.Unlock()
		if err := k.ctx.Err(); err != nil {
			return "", time.Time{}, err
		}
		if k.key == "" {
			return "", time.Time{}, ErrNotReady
		}
		return k.key, time.UnixMilli(k.binding.ExpiresAtMS), nil
	case <-k.ctx.Done():
		return "", time.Time{}, k.ctx.Err()
	case <-ctx.Done():
		return "", time.Time{}, ctx.Err()
	}
}
