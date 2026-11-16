// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestNativeDriveConsumerCannotReuseDriveDeviceOrMaster(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	drive, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	primary := credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(cfg.Bootstrap.MatrixUserID), DeviceID: "DRIVE_DEVICE", AccessToken: "synthetic-drive-session", Subject: cfg.Subject, Tenant: cfg.Tenant}
	raw, err := json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.Secrets.Write(drive.sessionKey(), raw); err != nil {
		t.Fatal(err)
	}
	if err = drive.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	cfg.NativeConsumer = "pdf"
	cfg.StoreDir = t.TempDir()
	consumer, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if consumer.account == drive.account || consumer.sessionKey() == drive.sessionKey() || consumer.masterKey() == drive.masterKey() {
		t.Fatal("native consumer selected Drive device/master namespace")
	}
	if saved, err := consumer.readSession(); !errors.Is(err, ErrLoginRequired) || saved.DeviceID != "" {
		t.Fatal("native consumer adopted Drive SDK session", err)
	}
	if err = consumer.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(consumer.master, drive.master) {
		t.Fatal("native consumer reused Drive private database key")
	}
	reopened, err := newClient(cfg)
	if err != nil || reopened.account != consumer.account {
		t.Fatal("consumer namespace did not survive restart", err)
	}
	cfg.NativeConsumer = "docs"
	another, err := newClient(cfg)
	if err != nil || another.account == consumer.account {
		t.Fatal("different native consumers reused one device", err)
	}
}

func TestNativeConsumerRejectsAmbiguousOrForeignDataProducts(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	for _, tc := range []struct{ product, consumer string }{{"drive", "drive"}, {"drive", "PDF"}, {"drive", "pdf "}, {"drive", "../pdf"}, {"mail", "pdf"}, {"rooms", "pdf"}} {
		cfg := storageConfig(t, server, tc.product)
		cfg.NativeConsumer = tc.consumer
		if client, err := newClient(cfg); client != nil || !errors.Is(err, ErrIdentity) {
			t.Fatal("invalid consumer namespace accepted", tc, err)
		}
	}
	cfg := storageConfig(t, server, "drive")
	cfg.NativeConsumer = "pdf"
	consumer, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	saved := credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(cfg.Bootstrap.MatrixUserID), DeviceID: "PDF_DEVICE", AccessToken: "synthetic-pdf-session", Subject: cfg.Subject, Tenant: cfg.Tenant}
	if consumer.validSession(saved) {
		t.Fatal("consumer accepted a device envelope without its immutable consumer binding")
	}
	saved.NativeConsumer = "pdf"
	if !consumer.validSession(saved) {
		t.Fatal("consumer rejected its exact device envelope")
	}
	saved.NativeConsumer = "docs"
	if consumer.validSession(saved) {
		t.Fatal("consumer accepted another app's device envelope")
	}
}
