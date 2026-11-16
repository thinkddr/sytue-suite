// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"maunium.net/go/mautrix"
)

func TestDeviceLeaseAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_STORAGE_SYNAPSE") != "1" {
		t.Skip("requires disposable Synapse on the remote development host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	old, cfg := device.Client, device.Config
	blocked, stop := context.WithTimeout(ctx, 40*time.Millisecond)
	_, err := Open(blocked, cfg)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("live SDK database opened twice", err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	// Replace only this artificial device's Matrix token, as reauthentication
	// does. The retired client must not revoke it or erase its local keychain.
	writer := leaseClient(t, cfg)
	if err = writer.acquireStore(ctx); err != nil {
		t.Fatal(err)
	}
	response, err := device.Matrix.Login(ctx, &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: old.session.UserID.String()},
		Password:   device.Password, DeviceID: old.session.DeviceID,
	})
	if err != nil || response == nil || response.DeviceID != old.session.DeviceID || response.AccessToken == old.session.AccessToken {
		t.Fatal("artificial reauthentication failed", err)
	}
	saved := old.session
	saved.AccessToken = response.AccessToken
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if err = cfg.Secrets.Write(writer.sessionKey(), raw); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = old.Logout(ctx); !errors.Is(err, ErrIdentity) {
		t.Fatal("retired SDK revoked its replacement", err)
	}
	current, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal("replacement SDK could not restore its database", err)
	}
	defer func() { _ = current.Close() }()
	go func() { _ = current.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = current.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if status, statusErr := current.VerificationStatus(ctx); statusErr != nil || !status.Verified {
		t.Fatal("replacement lost its verified device", statusErr)
	}
	if err = current.Logout(ctx); err != nil {
		t.Fatal("active logout failed", err)
	}
	if _, err = current.cli.Whoami(ctx); err == nil {
		t.Fatal("logout left the Matrix access token active")
	}
	for _, key := range []string{current.sessionKey(), current.masterKey(), current.backupSecretKey(), current.storageAuthSecretKey()} {
		value, readErr := cfg.Secrets.Read(key)
		if readErr != nil || len(value) != 0 {
			clear(value)
			t.Fatal("logout retained private credentials", readErr)
		}
	}
	if _, err = os.Stat(current.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("logout retained the private device database", err)
	}
	if _, err = os.Stat(filepath.Join(cfg.StoreDir, ".device-leases", current.account+".lock")); err != nil {
		t.Fatal("logout removed the stable lease inode", err)
	}
	if _, err = Open(ctx, cfg); !errors.Is(err, ErrLoginRequired) {
		t.Fatal("logout did not release its device lease", err)
	}
}
