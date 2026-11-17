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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestNativeDriveRecoveryCannotShareUnacknowledgedSetup(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_CATALOG_MIGRATION") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	upstream, _ := url.Parse("http://127.0.0.1:18585")
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "drive"}
	device := newUnverifiedStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	shared := &memorySecrets{values: map[string][]byte{}}
	valid := true
	device.Client.cfg.NativeRecovery = &NativeRecoveryConfig{Binding: strings.Repeat("b", 32), Secrets: shared, Authorize: func(ctx context.Context) error {
		if !valid {
			return ErrIdentity
		}
		return ctx.Err()
	}}
	recovery, err := device.Client.BootstrapRecovery(ctx, storageFixtureUIA(device.Matrix.UserID, device.Password))
	if err != nil || recovery == "" {
		t.Fatal("initialize actual owner recovery", err)
	}
	go func() { _ = device.Client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = device.Client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = device.Client.SyncNativeRecovery(ctx); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("pending owner setup was offered to another app", err)
	}
	if len(shared.values) != 0 {
		t.Fatal("unacknowledged recovery entered shared OS store")
	}
	if err = device.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	valid = false
	if err = device.Client.SyncNativeRecovery(ctx); !errors.Is(err, ErrIdentity) {
		t.Fatal("revoked owner published native recovery", err)
	}
	if len(shared.values) != 0 {
		t.Fatal("revoked owner changed shared OS store")
	}
	valid = true
	if err = device.Client.SyncNativeRecovery(ctx); err != nil {
		t.Fatal("verified acknowledged owner could not publish native recovery", err)
	}
	if len(shared.values) != 1 {
		t.Fatal("native recovery published device credentials or missing envelope")
	}
	t.Log("real SDK: unacknowledged or revoked owner cannot publish SSSS recovery; acknowledged owner publishes only one scope/login-bound envelope")
}
