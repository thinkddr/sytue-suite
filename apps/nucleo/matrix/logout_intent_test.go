// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func logoutFixture(t *testing.T, handler http.HandlerFunc) (*Client, Config) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	cfg := configFor(t, server)
	client := leaseClient(t, cfg)
	if err := client.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	leaseSaved(t, client, "artificial-retired-session")
	for _, key := range []string{client.masterKey(), client.backupSecretKey(), client.storageAuthSecretKey(), client.recoverySetupKey()} {
		if err := cfg.Secrets.Write(key, []byte("artificial private key to remove")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(client.directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.directory, "artificial.sqlite"), []byte("encrypted-device-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return client, cfg
}

func retireWithoutNetwork(t *testing.T, client *Client) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.Logout(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("offline logout did not report its pending revocation", err)
	}
	assertLogoutKeysRemoved(t, client)
	if raw, err := client.cfg.Secrets.Read(client.logoutIntentKey()); err != nil || len(raw) == 0 {
		t.Fatal("offline logout forgot its revocation intent", err)
	}
}

func assertLogoutKeysRemoved(t *testing.T, client *Client) {
	t.Helper()
	for _, key := range []string{client.sessionKey(), client.masterKey(), client.backupSecretKey(), client.storageAuthSecretKey(), client.recoverySetupKey()} {
		raw, err := client.cfg.Secrets.Read(key)
		if err != nil || len(raw) != 0 {
			clear(raw)
			t.Fatal("logout retained a private key or session", err)
		}
	}
	if _, err := os.Stat(client.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("logout retained the device database", err)
	}
}

func TestPendingLogoutReplaysBeforeOpenAndBothSSOFlows(t *testing.T) {
	for _, kind := range []string{"open", "desktop", "mobile"} {
		t.Run(kind, func(t *testing.T) {
			var revoked atomic.Int32
			client, cfg := logoutFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/_matrix/client/v3/logout" {
					if r.Header.Get("Authorization") != "Bearer artificial-retired-session" {
						t.Error("revocation used another credential")
					}
					revoked.Add(1)
					_, _ = w.Write([]byte(`{}`))
					return
				}
				if revoked.Load() != 1 {
					t.Error("SSO reached the network before retiring the previous device")
				}
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errcode":"M_FORBIDDEN","error":"artificial SSO fixture"}`))
			})
			retireWithoutNetwork(t, client)
			var err error
			switch kind {
			case "open":
				_, err = Open(t.Context(), cfg)
				if !errors.Is(err, ErrLoginRequired) {
					t.Fatal("retired device was resurrected", err)
				}
			case "desktop":
				_, err = Login(t.Context(), cfg, func(string) error { t.Error("unexpected SSO browser"); return context.Canceled })
			case "mobile":
				_, err = LoginNative(t.Context(), cfg, api.NewRooms(cfg.Bootstrap.HomeserverURL, nil, cfg.HTTPClient), func(context.Context, api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error) {
					t.Error("unexpected SSO callback")
					return api.MatrixNativeCallback{}, context.Canceled
				})
			}
			if err == nil || revoked.Load() != 1 {
				t.Fatal("pending logout was not completed before the constructor", err)
			}
			if raw, readErr := cfg.Secrets.Read(client.logoutIntentKey()); readErr != nil || len(raw) != 0 {
				t.Fatal("confirmed logout retained its token", readErr)
			}
			if _, err = Open(t.Context(), cfg); !errors.Is(err, ErrLoginRequired) || revoked.Load() != 1 {
				t.Fatal("completed logout repeated a request or created a new device", err)
			}
		})
	}
}

func TestPendingLogoutRequiresACompleteRevocationReceipt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		done   bool
	}{
		{"forbidden", http.StatusForbidden, `{"errcode":"M_FORBIDDEN","error":"unavailable"}`, false},
		{"soft_logout", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":true}`, false},
		{"invalid_soft_logout", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":"false"}`, false},
		{"already_revoked", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN"}`, true},
		{"explicitly_revoked", http.StatusUnauthorized, `{"errcode":"M_UNKNOWN_TOKEN","soft_logout":false}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var online atomic.Bool
			var calls atomic.Int32
			client, cfg := logoutFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/_matrix/client/v3/logout" {
					t.Error("retired account reached another API")
				}
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if online.Load() {
					_, _ = w.Write([]byte(`{}`))
				} else {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}
			})
			retireWithoutNetwork(t, client)
			_, err := Open(t.Context(), cfg)
			raw, readErr := cfg.Secrets.Read(client.logoutIntentKey())
			if readErr != nil || err == nil || (len(raw) == 0) != tc.done || errors.Is(err, ErrLoginRequired) != tc.done || calls.Load() != 1 {
				t.Fatal("logout accepted an incomplete receipt or forgot its retry", err, readErr)
			}
			assertLogoutKeysRemoved(t, client)
			if !tc.done {
				online.Store(true)
				if _, err = Open(t.Context(), cfg); !errors.Is(err, ErrLoginRequired) || calls.Load() != 2 {
					t.Fatal("failed logout could not resume", err)
				}
			}
		})
	}
}

func TestPendingLogoutCannotEraseOrRevokeAReplacement(t *testing.T) {
	var calls atomic.Int32
	client, cfg := logoutFixture(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	retireWithoutNetwork(t, client)
	replacement := leaseClient(t, cfg)
	if err := replacement.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	leaseSaved(t, replacement, "artificial-replacement")
	_ = replacement.Close()
	if _, err := Open(t.Context(), cfg); !errors.Is(err, ErrIdentity) {
		t.Fatal("pending logout accepted another saved session", err)
	}
	if err := client.Logout(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatal("old in-memory logout accepted another saved session", err)
	}
	saved, err := replacement.readSession()
	if err != nil || saved != replacement.session || calls.Load() != 0 {
		t.Fatal("replacement was erased or revoked", err)
	}
}

func TestCorruptOrForeignLogoutIntentCannotEraseCurrentKeys(t *testing.T) {
	for _, field := range []string{"json", "version", "account", "home", "tenant", "subject", "token"} {
		t.Run(field, func(t *testing.T) {
			var calls atomic.Int32
			client, cfg := logoutFixture(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
			_ = client.Close()
			intent := logoutIntent{Version: 1, Account: client.account, Session: client.session}
			switch field {
			case "version":
				intent.Version = 2
			case "account":
				intent.Account = "another"
			case "home":
				intent.Session.Homeserver = "https://another.test"
			case "tenant":
				intent.Session.Tenant = "another"
			case "subject":
				intent.Session.Subject = "another"
			case "token":
				intent.Session.AccessToken = ""
			}
			raw, _ := json.Marshal(intent)
			if field == "json" {
				raw = []byte(`{`)
			}
			if err := cfg.Secrets.Write(client.logoutIntentKey(), raw); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(t.Context(), cfg); err == nil || (!errors.Is(err, ErrIdentity) && !errors.Is(err, ErrSecureStore)) {
				t.Fatal("invalid logout intent accepted", err)
			}
			saved, err := client.readSession()
			key, keyErr := cfg.Secrets.Read(client.masterKey())
			if err != nil || keyErr != nil || saved != client.session || !strings.Contains(string(key), "artificial private") || calls.Load() != 0 {
				t.Fatal("invalid logout intent changed current keys", err, keyErr)
			}
		})
	}
}

type failingLogoutSecrets struct {
	Secrets
	writeKey  string
	deleteKey string
}

func (s *failingLogoutSecrets) Write(key string, raw []byte) error {
	if key == s.writeKey {
		return errors.New("artificial keychain write failure")
	}
	return s.Secrets.Write(key, raw)
}

func (s *failingLogoutSecrets) Delete(key string) error {
	if key == s.deleteKey {
		return errors.New("artificial keychain deletion failure")
	}
	return s.Secrets.Delete(key)
}

func TestPendingLogoutKeepsItsIntentAcrossPartialKeychainCleanup(t *testing.T) {
	for _, kind := range []string{"encryption_key", "receipt"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			client, cfg := logoutFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					_, _ = w.Write([]byte(`{}`))
				} else {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"errcode":"M_UNKNOWN_TOKEN"}`))
				}
			})
			secrets := &failingLogoutSecrets{Secrets: cfg.Secrets, deleteKey: client.masterKey()}
			if kind == "receipt" {
				secrets.deleteKey = client.logoutIntentKey()
			}
			cfg.Secrets, client.cfg.Secrets = secrets, secrets
			if err := client.Logout(t.Context()); err == nil {
				t.Fatal("partial cleanup was acknowledged")
			}
			if raw, err := secrets.Read(client.logoutIntentKey()); err != nil || len(raw) == 0 {
				t.Fatal("partial cleanup lost its retry", err)
			}
			secrets.deleteKey = ""
			if _, err := Open(t.Context(), cfg); !errors.Is(err, ErrLoginRequired) || calls.Load() != 2 {
				t.Fatal("partial cleanup could not finish after restart", err)
			}
			assertLogoutKeysRemoved(t, client)
			if raw, err := secrets.Read(client.logoutIntentKey()); err != nil || len(raw) != 0 {
				t.Fatal("completed cleanup retained its credential", err)
			}
		})
	}
}

func TestLogoutDoesNotRewriteItsDurableIntent(t *testing.T) {
	client, _ := logoutFixture(t, http.NotFound)
	retireWithoutNetwork(t, client)
	client.cfg.Secrets = &failingLogoutSecrets{Secrets: client.cfg.Secrets, writeKey: client.logoutIntentKey()}
	if err := client.Logout(t.Context()); err == nil || errors.Is(err, ErrSecureStore) {
		t.Fatal("retry rewrote an already durable intent", err)
	}
	if raw, err := client.cfg.Secrets.Read(client.logoutIntentKey()); err != nil || len(raw) == 0 {
		t.Fatal("retry lost its original intent", err)
	}
}
