// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type keyDelivery struct {
	user, device, kind string
	content            CallKeyContent
}
type keyTransport struct {
	device     string
	mu         sync.Mutex
	deliveries []keyDelivery
}

func (t *keyTransport) DeviceID() string { return t.device }
func (*keyTransport) UserForSubject(subject string) (string, error) {
	if subject == "controller" {
		return "", ErrIdentity
	}
	return "@sytue_" + subject + ":matrix.test", nil
}

func (t *keyTransport) SendDeviceControl(_ context.Context, user, device, kind string, content CallKeyContent) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deliveries = append(t.deliveries, keyDelivery{user, device, kind, content})
	return nil
}

func callBinding() api.RoomsCallEncryption {
	return api.RoomsCallEncryption{CallID: "call", MatrixRoomID: "!epoch", LiveKitRoom: "sfu-epoch", Generation: 1, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli(), LeaderSubject: "leader", LeaderDeviceID: "ACTIVE", Members: []api.RoomsCallMember{{Subject: "leader", DeviceID: "ACTIVE"}, {Subject: "person", DeviceID: "CURRENT"}}}
}

func liveKey(t *testing.T, binding api.RoomsCallEncryption, key string) DecryptedEvent {
	t.Helper()
	content := CallKeyContent{Version: 1, CallID: binding.CallID, Generation: binding.Generation, Algorithm: "AES-GCM", ExpiresAt: binding.ExpiresAtMS, KeyBase64: key}
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return DecryptedEvent{Type: CallKeyEvent, Sender: "@sytue_leader:matrix.test", SenderDevice: "ACTIVE", Trust: id.TrustStateCrossSignedVerified, Content: raw}
}

func TestCallKeysTargetOnlyCurrentDeviceAndPinLeader(t *testing.T) {
	binding := callBinding()
	sender := &keyTransport{device: "ACTIVE"}
	leader, err := NewCallKeys(t.Context(), sender, "leader", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer leader.Close()
	if err = leader.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	key, _, err := leader.Key(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.deliveries) != 1 || sender.deliveries[0].device != "CURRENT" || sender.deliveries[0].user != "@sytue_person:matrix.test" || sender.deliveries[0].kind != CallKeyEvent {
		t.Fatal("key was not bound to exact current device")
	}
	receiver, err := NewCallKeys(t.Context(), &keyTransport{device: "CURRENT"}, "person", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	if _, _, available := receiver.CurrentKey(); available {
		t.Fatal("receiver key available before verified delivery")
	}
	valid := liveKey(t, binding, key)
	cases := []DecryptedEvent{valid, valid, valid, valid, valid}
	cases[0].SenderDevice = "REPLACED"
	cases[1].Sender = "@sytue_other:matrix.test"
	cases[2].Trust = id.TrustStateCrossSignedTOFU
	cases[3].Recovered = true
	cases[4].SenderDevice = ""
	for _, rejected := range cases {
		if receiver.Receive(rejected) {
			t.Fatal("accepted unauthorized call control")
		}
	}
	old := binding
	old.Generation--
	if receiver.Receive(liveKey(t, old, key)) {
		t.Fatal("accepted key replay from previous epoch")
	}
	if !receiver.Receive(valid) {
		t.Fatal("current leader key rejected")
	}
	actual, _, err := receiver.Key(t.Context())
	if err != nil || actual != key {
		t.Fatal("Olm key not installed", err)
	}
	receiver.Close()
	if _, _, available := receiver.CurrentKey(); available {
		t.Fatal("closed epoch retained current key")
	}
	if _, _, err = receiver.Key(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatal("closed epoch retained a usable key", err)
	}
}

func TestCallKeysRejectsReplacedOwnDeviceAndController(t *testing.T) {
	binding := callBinding()
	if _, err := NewCallKeys(t.Context(), &keyTransport{device: "OLD"}, "person", binding); !errors.Is(err, ErrIdentity) {
		t.Fatal("replaced own device accepted", err)
	}
	binding.Members = append(binding.Members, api.RoomsCallMember{Subject: "controller", DeviceID: "SIGNED"})
	if _, err := NewCallKeys(t.Context(), &keyTransport{device: "CURRENT"}, "person", binding); !errors.Is(err, ErrIdentity) {
		t.Fatal("controller accepted", err)
	}
}

func TestCallKeysExpireWithoutNetworkRefresh(t *testing.T) {
	binding := callBinding()
	binding.ExpiresAtMS = time.Now().Add(100 * time.Millisecond).UnixMilli()
	keys, err := NewCallKeys(t.Context(), &keyTransport{device: "ACTIVE"}, "leader", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Close()
	if err = keys.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-keys.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("media key did not expire")
	}
	if _, _, available := keys.CurrentKey(); available {
		t.Fatal("expired call key was still available")
	}
}
