// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestIsolatedRecoveryCustodySurvivesLostAcknowledgmentAndFreshDevices(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_RECOVERY_CUSTODY") != "1" {
		t.Skip("requires disposable Synapse on remote development host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	upstream := httputil.NewSingleHostReverseProxy(target)
	var puts atomic.Int32
	var dropDefault atomic.Bool
	upstream.ModifyResponse = func(r *http.Response) error {
		if r.Request.Method == http.MethodPut {
			puts.Add(1)
		}
		if r.StatusCode == http.StatusOK && strings.HasSuffix(r.Request.URL.Path, "/account_data/m.secret_storage.default_key") && r.Request.Method == http.MethodPut && dropDefault.CompareAndSwap(true, false) {
			_ = r.Body.Close()
			r.StatusCode = http.StatusBadRequest
			r.Body = io.NopCloser(strings.NewReader(`{"errcode":"M_UNKNOWN","error":"artificial lost acknowledgment after commit"}`))
			r.ContentLength = -1
			r.Header.Del("Content-Length")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(upstream)
	t.Cleanup(proxy.Close)
	actor := uuid.NewString()
	legacy := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), storage.Scope{Subject: actor, Tenant: "acme", Product: "drive"}, storage.LegacyIdentity)
	keys := make([]string, 1, 4)
	keys[0] = legacy.Recovery
	accounts := map[string]bool{legacy.Client.account: true}
	for i, scope := range []storage.Scope{{Subject: actor, Tenant: "acme", Product: "drive"}, {Subject: actor, Tenant: "acme", Product: "mail"}, {Subject: actor, Tenant: "beta", Product: "drive"}} {
		device := newUnverifiedStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
		client, cli, cfg := device.Client, device.Matrix, device.Config
		if accounts[client.account] {
			t.Fatal("products or organizations share a local key namespace")
		}
		accounts[client.account] = true
		if i == 0 {
			base := client.cfg.Secrets
			client.cfg.Secrets = &recoveryFaultSecrets{Secrets: base, writeErr: true}
			before := puts.Load()
			if key, err := client.BootstrapRecovery(ctx, storageFixtureUIA(cli.UserID, device.Password)); key != "" || !errors.Is(err, ErrSecureStore) || puts.Load() != before {
				t.Fatal("public keys escaped before durable private custody", err)
			}
			client.cfg.Secrets = base
			dropDefault.Store(true)
			if key, err := client.BootstrapRecovery(ctx, storageFixtureUIA(cli.UserID, device.Password)); key != "" || err == nil || dropDefault.Load() {
				t.Fatal("fixture failed to interrupt an accepted remote setup", err)
			}
			pending, err := client.readRecoverySetup()
			if err != nil || pending == nil || pending.Complete {
				t.Fatal("interrupted setup lost its private material", err)
			}
			originalKey := pending.storageKey().RecoveryKey()
			pending.clear()
			publishedKeys, err := cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{cli.UserID: {}}})
			if err != nil || len(publishedKeys.MasterKeys[cli.UserID].Keys) != 1 {
				t.Fatal("fixture did not commit its identity", err)
			}
			if err = client.Close(); err != nil {
				t.Fatal(err)
			}
			client, err = Open(ctx, cfg)
			if err != nil {
				t.Fatal("interrupted private device could not reopen", err)
			}
			t.Cleanup(func() { _ = client.Close() })
			device.Client = client
			recovered, err := client.BootstrapRecovery(ctx, storageFixtureUIA(cli.UserID, device.Password))
			if err != nil || recovered != originalKey {
				t.Fatal("retry replaced an interrupted identity", err)
			}
			after, err := cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{cli.UserID: {}}})
			if err != nil || !maps.Equal(publishedKeys.MasterKeys[cli.UserID].Keys, after.MasterKeys[cli.UserID].Keys) {
				t.Fatal("retry rotated an already published master key", err)
			}
			device.Recovery = recovered
		} else {
			key, err := client.BootstrapRecovery(ctx, storageFixtureUIA(cli.UserID, device.Password))
			if err != nil {
				t.Fatal("private scope setup failed", err)
			}
			device.Recovery = key
		}
		go func(c *Client) { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
		if err := client.WaitReady(ctx); err != nil {
			t.Fatal(err)
		}
		status, err := client.VerificationStatus(ctx)
		if err != nil || !status.Verified || !status.RecoverySetupPending {
			t.Fatal("completed setup skipped owner acknowledgment", err)
		}
		base := client.cfg.Secrets
		client.cfg.Secrets = &failingLogoutSecrets{Secrets: base, deleteKey: client.recoverySetupKey()}
		if err = client.AcknowledgeRecoverySetup(ctx); !errors.Is(err, ErrSecureStore) {
			t.Fatal("failed keychain deletion acknowledged recovery", err)
		}
		client.cfg.Secrets = base
		if err = client.AcknowledgeRecoverySetup(ctx); err != nil {
			t.Fatal("owner acknowledgment failed", err)
		}
		if pending, err := client.readRecoverySetup(); !errors.Is(err, errNoRecoverySetup) || pending != nil {
			t.Fatal("acknowledged recovery retained the temporary journal", err)
		}
		record := storage.Record{Version: 1, Scope: scope, ObjectID: "private-recovery-proof", RevisionID: "first", Role: "content", Payload: []byte(`{"text":"private isolated application content","precise":9007199254740993}`)}
		ref, err := writeStorageRecordVerified(ctx, client, "custody-proof", record)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := cli.GetEvent(ctx, device.Room, id.EventID(ref.EventID))
		if err != nil || wire.Type != event.EventEncrypted {
			t.Fatal("private storage did not use maintained E2EE", err)
		}
		encoded, err := json.Marshal(wire)
		if err != nil || bytes.Contains(encoded, record.Payload) || bytes.Contains(encoded, []byte("private isolated application content")) {
			t.Fatal("server retained private application plaintext", err)
		}
		if err = client.Backup(ctx); err != nil {
			t.Fatal(err)
		}
		anon, err := mautrix.NewClient(proxy.URL, "", "")
		if err != nil {
			t.Fatal(err)
		}
		anon.Client = proxy.Client()
		login, err := anon.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: cli.UserID.String()}, Password: device.Password, DeviceID: "FRESH_CUSTODY"})
		if err != nil {
			t.Fatal("fresh fixture device login failed", err)
		}
		freshCfg := cfg
		freshCfg.StoreDir = t.TempDir()
		freshCfg.Secrets = &memorySecrets{values: map[string][]byte{}}
		fresh := storageSynapseSession(ctx, t, freshCfg, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
		go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
		if err = fresh.WaitReady(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = fresh.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role); err == nil {
			t.Fatal("access token alone opened private history")
		}
		for _, foreign := range keys {
			if foreign == device.Recovery {
				t.Fatal("scopes share recovery material")
			}
			if err = fresh.Recover(ctx, foreign); err == nil {
				t.Fatal("another product or organization's recovery key unlocked this scope")
			}
		}
		if err = fresh.Recover(ctx, device.Recovery); err != nil {
			t.Fatal("fresh device could not recover isolated keys", err)
		}
		got, err := fresh.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role)
		if err != nil || !bytes.Equal(got.Payload, record.Payload) {
			t.Fatal("private history did not survive independent key recovery", err)
		}
		keys = append(keys, device.Recovery)
	}
	t.Log("real Synapse: durable custody before publication, lost-ACK restart, unchanged identity, explicit acknowledgment and isolated fresh-device recovery verified")
}
