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
	"testing"

	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func ownerCacheFixture(t *testing.T) (*Client, *ssss.Key) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	cfg := storageConfig(t, server, "drive")
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	key, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clear(key.Key) })
	return client, key
}

func TestStorageOwnerKeyPersistsPrivatelyAndCannotBeReplacedImplicitly(t *testing.T) {
	client, key := ownerCacheFixture(t)
	if got, err := client.storageAuthKey(t.Context(), ""); got != nil || !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("missing owner secret did not require recovery", err)
	}
	if err := client.saveStorageAuthKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	got, err := client.storageAuthKey(t.Context(), key.ID)
	if err != nil || !bytes.Equal(got.Key, key.Key) {
		t.Fatal("owner key did not persist", err)
	}
	clear(got.Key)
	got, err = client.storageAuthKey(t.Context(), key.ID)
	if err != nil || !bytes.Equal(got.Key, key.Key) {
		t.Fatal("returned key mutated the secure store", err)
	}
	clear(got.Key)
	other, err := ssss.NewKey("")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(other.Key)
	before, _ := client.cfg.Secrets.Read(client.storageAuthSecretKey())
	defer clear(before)
	if err = client.saveStorageAuthKey(t.Context(), other); !errors.Is(err, ErrRecoveryExists) {
		t.Fatal("owner key replaced without migrating history", err)
	}
	after, _ := client.cfg.Secrets.Read(client.storageAuthSecretKey())
	defer clear(after)
	if !bytes.Equal(before, after) {
		t.Fatal("failed rotation modified the owner secret")
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newClient(client.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err = reopened.storageAuthKey(t.Context(), key.ID)
	if err != nil || !bytes.Equal(got.Key, key.Key) {
		t.Fatal("owner key did not survive restart", err)
	}
	clear(got.Key)
	// This key-store fixture does not perform SSO; bind an explicit synthetic
	// session so logout can prove it owns the credentials it removes.
	leaseSaved(t, reopened, "artificial-owner-cache")
	if err = reopened.Logout(t.Context()); err != nil {
		t.Fatal(err)
	}
	raw, err := client.cfg.Secrets.Read(client.storageAuthSecretKey())
	if err != nil || len(raw) != 0 {
		t.Fatal("logout retained the owner key", err)
	}
}

func TestStorageOwnerKeyRejectsCorruptionForeignScopeLockedStoreAndClosedClient(t *testing.T) {
	client, key := ownerCacheFixture(t)
	if err := client.saveStorageAuthKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	raw, err := client.cfg.Secrets.Read(client.storageAuthSecretKey())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var original storageAuthSecret
	if err = json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	defer clear(original.Key)
	for _, field := range []string{"version", "tenant", "subject", "product", "home", "user", "key-id", "key", "metadata"} {
		t.Run(field, func(t *testing.T) {
			candidate := original
			switch field {
			case "version":
				candidate.Version = 2
			case "tenant":
				candidate.Scope.Tenant = "foreign"
			case "subject":
				candidate.Scope.Subject = "foreign"
			case "product":
				candidate.Scope.Product = "mail"
			case "home":
				candidate.Homeserver = "https://foreign.test"
			case "user":
				candidate.UserID = "@foreign:matrix.test"
			case "key-id":
				candidate.KeyID = ""
			case "key":
				candidate.Key = make([]byte, 32)
			case "metadata":
				candidate.Metadata = nil
			}
			encoded, _ := json.Marshal(candidate)
			defer clear(encoded)
			if got, err := client.decodeStorageAuthKey(encoded); got != nil || !errors.Is(err, ErrSecureStore) {
				t.Fatal("corrupt or foreign owner secret accepted", err)
			}
		})
	}
	secrets := client.cfg.Secrets.(*memorySecrets)
	secrets.fail = true
	if got, err := client.storageAuthKey(t.Context(), key.ID); got != nil || !errors.Is(err, ErrSecureStore) {
		t.Fatal("locked keychain fell back to unprotected data", err)
	}
	secrets.fail = false
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := client.storageAuthKey(ctx, key.ID); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled owner lookup succeeded", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := client.storageAuthKey(t.Context(), key.ID); got != nil || !errors.Is(err, ErrClosed) {
		t.Fatal("closed owner lookup succeeded", err)
	}
}

func TestStorageOwnerAttestationNeverFallsBackToLiveSenderTrust(t *testing.T) {
	client, key := ownerCacheFixture(t)
	if err := client.saveStorageAuthKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	record := storage.Record{Version: 1, Scope: client.storageScope(), ObjectID: "object", RevisionID: "revision", Role: "content", Payload: []byte(`{"integer":900719925474099312345}`)}
	room := client.cfg.Bootstrap.StorageRoomID
	proof, err := sealStorageOwnerRecord(room, record, key)
	if err != nil {
		t.Fatal(err)
	}
	content := func(proof any, present bool) []byte {
		t.Helper()
		body := map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": record}
		if present {
			body["org.sytue.storage_auth"] = proof
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	value := DecryptedEvent{Reference: EventRef{RoomID: room, EventID: "$record"}, Sender: client.cfg.Bootstrap.MatrixUserID, Type: event.EventMessage.Type, Trust: id.TrustStateBackup, Recovered: true, Content: content(proof, true)}
	got, err := client.readPrivateStorageRecord(t.Context(), value, record.Scope, value.Sender)
	if err != nil || !bytes.Equal(got.Payload, record.Payload) {
		t.Fatal("owner-attested recovered record rejected", err)
	}
	value.Content = content(nil, false)
	if _, err = client.readPrivateStorageRecord(t.Context(), value, record.Scope, value.Sender); !errors.Is(err, ErrUnverified) {
		t.Fatal("anonymous backup accepted without owner attestation", err)
	}
	value.Recovered = false
	value.Trust = id.TrustStateCrossSignedVerified
	if _, err = client.readPrivateStorageRecord(t.Context(), value, record.Scope, value.Sender); err != nil {
		t.Fatal("original verified device cannot migrate legacy records", err)
	}
	value.Content = content(nil, true)
	if _, err = client.readPrivateStorageRecord(t.Context(), value, record.Scope, value.Sender); !errors.Is(err, ErrUnverified) {
		t.Fatal("null owner proof bypassed verification", err)
	}
	proof.MAC = "AA"
	value.Content = content(proof, true)
	client.cfg.Secrets.(*memorySecrets).fail = true
	if _, err = client.readPrivateStorageRecord(t.Context(), value, record.Scope, value.Sender); !errors.Is(err, ErrUnverified) {
		t.Fatal("malformed proof requested a key or bypassed live sender trust", err)
	}
}

func TestStorageOwnerKeyLookupDoesNotRaceBindingRefresh(t *testing.T) {
	client, key := ownerCacheFixture(t)
	if err := client.saveStorageAuthKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	next := client.cfg.Bootstrap
	start := make(chan struct{})
	result := make(chan error, 2)
	go func() {
		<-start
		for range 1000 {
			if err := client.UpdateBindings(next); err != nil {
				result <- err
				return
			}
		}
		result <- nil
	}()
	go func() {
		<-start
		for range 1000 {
			got, err := client.storageAuthKey(t.Context(), key.ID)
			if err != nil {
				result <- err
				return
			}
			clear(got.Key)
		}
		result <- nil
	}()
	close(start)
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
}
