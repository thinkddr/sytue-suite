// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"

	"maunium.net/go/mautrix"
)

func TestPendingLogoutAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_STORAGE_SYNAPSE") != "1" {
		t.Skip("requires disposable Synapse on the remote development host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	retireWithoutNetwork(t, device.Client)
	if _, err := device.Matrix.Whoami(ctx); err != nil {
		t.Fatal("offline fixture unexpectedly revoked its server session", err)
	}
	if _, err := Open(ctx, device.Config); !errors.Is(err, ErrLoginRequired) {
		t.Fatal("pending logout resurrected the old device", err)
	}
	if _, err := device.Matrix.Whoami(ctx); !errors.Is(err, mautrix.MUnknownToken) {
		t.Fatal("resumed logout did not invalidate the actual Matrix token", err)
	}
	inspector, err := mautrix.NewClient(proxy.URL, device.Matrix.UserID, "")
	if err != nil {
		t.Fatal(err)
	}
	inspector.Client = proxy.Client()
	_, err = inspector.Login(ctx, &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: device.Matrix.UserID.String()},
		Password:   device.Password, DeviceID: "ARTIFICIAL_LOGOUT_INSPECTOR", StoreCredentials: true,
	})
	if err != nil {
		t.Fatal("artificial inspector could not sign in", err)
	}
	defer func() { _, _ = inspector.Logout(ctx) }()
	if _, err = inspector.GetDeviceInfo(ctx, device.Matrix.DeviceID); !errors.Is(err, mautrix.MNotFound) {
		t.Fatal("retired device is still registered on Synapse", err)
	}
	raw, err := device.Config.Secrets.Read(device.Client.logoutIntentKey())
	if err != nil || len(raw) != 0 {
		t.Fatal("confirmed remote device deletion retained its credential", err)
	}
}
