// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type memorySecrets struct {
	mu     sync.Mutex
	values map[string][]byte
	fail   bool
}

func (s *memorySecrets) Read(k string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return nil, errors.New("locked")
	}
	return bytes.Clone(s.values[k]), nil
}

func (s *memorySecrets) Write(k string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("locked")
	}
	s.values[k] = bytes.Clone(v)
	return nil
}

func (s *memorySecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, k)
	return nil
}

func configFor(t *testing.T, server *httptest.Server) Config {
	t.Helper()
	return Config{Bootstrap: Bootstrap{ControllerUserID: "@rooms_controller:matrix.test", Actor: "person", Tenant: "org", HomeserverURL: server.URL, MatrixUserID: "@person:matrix.test", Encryption: string(id.AlgorithmMegolmV1), Channels: []RoomBinding{{ChannelID: "channel", RoomID: "!room:matrix.test"}}}, Subject: "person", Tenant: "org", StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: server.Client()}
}

func TestSendCannotFallBackToPlaintext(t *testing.T) {
	mutations := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"algorithm":"unsupported"}`))
	}))
	defer server.Close()
	c, err := newClient(configFor(t, server))
	if err != nil {
		t.Fatal(err)
	}
	c.ready.Store(true)
	_, err = c.SendText(context.Background(), "!room:matrix.test", "never send this plaintext")
	if !errors.Is(err, ErrNotEncrypted) || mutations != 0 {
		t.Fatalf("err=%v mutations=%d", err, mutations)
	}
	c.ready.Store(false)
	if _, err = c.SendText(context.Background(), "!room:matrix.test", "x"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("before sync: %v", err)
	}
	c.ready.Store(true)
	if _, err = c.SendText(context.Background(), "!other:matrix.test", "x"); !errors.Is(err, ErrIdentity) {
		t.Fatalf("unmapped room: %v", err)
	}
}

func TestMissingMasterNeverReplacesDeviceKeys(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	cfg := configFor(t, server)
	c, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	saved := credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(cfg.Bootstrap.MatrixUserID), DeviceID: "DEVICE", AccessToken: "test-session", Subject: cfg.Subject, Tenant: cfg.Tenant}
	body, _ := json.Marshal(saved)
	if err = cfg.Secrets.Write(c.sessionKey(), body); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), cfg); !errors.Is(err, ErrSecureStore) {
		t.Fatalf("missing key: %v", err)
	}
	if calls != 0 {
		t.Fatal("missing private key reached the homeserver")
	}
	key, _ := cfg.Secrets.Read(c.masterKey())
	if len(key) != 0 {
		t.Fatal("lost keys were silently regenerated")
	}
	cfg.Tenant = "another-org"
	cfg.Bootstrap.Tenant = cfg.Tenant
	if _, err = Open(context.Background(), cfg); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("another tenant reused the session: %v", err)
	}
}

func TestSyncReadinessChangesAfterMembershipProcessing(t *testing.T) {
	c := &Client{}
	s := &guardedSyncer{DefaultSyncer: mautrix.NewDefaultSyncer(), owner: c}
	observed := false
	s.OnEventType(event.StateMember, func(context.Context, *event.Event) {
		observed = true
		if c.ready.Load() {
			t.Error("send was enabled before membership was processed")
		}
	})
	member := &event.Event{StateKey: new("@person:matrix.test"), Type: event.StateMember, Content: event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipJoin}}}
	response := &mautrix.RespSync{}
	// Build wire data so the maintained response decoder owns the sync schema.
	wire, _ := json.Marshal(map[string]any{"next_batch": "one", "rooms": map[string]any{"join": map[string]any{"!room:matrix.test": map[string]any{"state": map[string]any{"events": []*event.Event{member}}}}}})
	if err := json.Unmarshal(wire, response); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessResponse(context.Background(), response, ""); err != nil {
		t.Fatal(err)
	}
	if !observed || !c.ready.Load() {
		t.Fatal("initial sync never opened after membership")
	}
	_, _ = s.OnFailedSync(nil, errors.New("network disconnected"))
	if c.ready.Load() {
		t.Fatal("failed sync left sending enabled")
	}
}

func TestSendWaitsForMembershipUpdateBeforeCheckingReadiness(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"algorithm":"unsupported"}`))
	}))
	defer server.Close()
	c, err := newClient(configFor(t, server))
	if err != nil {
		t.Fatal(err)
	}
	c.stateMu.Lock()
	result := make(chan error, 1)
	go func() {
		_, sendErr := c.SendText(t.Context(), "!room:matrix.test", "private")
		result <- sendErr
	}()
	select {
	case err := <-result:
		c.stateMu.Unlock()
		t.Fatal("send did not wait for the in-progress state update", err)
	case <-time.After(20 * time.Millisecond):
	}
	c.ready.Store(true)
	c.stateMu.Unlock()
	if err := <-result; !errors.Is(err, ErrNotEncrypted) {
		t.Fatal("send did not validate encryption after the update", err)
	}
}

func TestPendingSyncUpdateDoesNotInvalidateALockedRead(t *testing.T) {
	c := &Client{}
	c.ready.Store(true)
	s := &guardedSyncer{DefaultSyncer: mautrix.NewDefaultSyncer(), owner: c}
	c.stateMu.RLock()
	result := make(chan error, 1)
	go func() { result <- s.ProcessResponse(t.Context(), &mautrix.RespSync{}, "previous") }()
	select {
	case err := <-result:
		c.stateMu.RUnlock()
		t.Fatal("sync bypassed the active state reader", err)
	case <-time.After(20 * time.Millisecond):
	}
	ready := c.ready.Load()
	c.stateMu.RUnlock()
	if err := <-result; err != nil || !ready || !c.ready.Load() {
		t.Fatal("membership update invalidated a read before acquiring its lock", err)
	}
}
