// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestNativeSecretMigrationDecoderRejectsForeignOrMalformedSession(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := storageConfig(t, server, "drive")
	cfg.NativeConsumer = "pdf"
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	saved := credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(cfg.Bootstrap.MatrixUserID), DeviceID: "PDF_DEVICE", AccessToken: "synthetic", Subject: cfg.Subject, Tenant: cfg.Tenant, NativeConsumer: "pdf"}
	encode := func(value credentials) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if recovered, err := client.decodeSession(encode(saved)); err != nil || recovered != saved {
		t.Fatal("owned existing credential changed", err)
	}
	for _, change := range []func(*credentials){
		func(v *credentials) { v.Subject = "other" }, func(v *credentials) { v.Tenant = "other" },
		func(v *credentials) { v.Homeserver = "https://other.invalid" }, func(v *credentials) { v.UserID = "@other:invalid" },
		func(v *credentials) { v.NativeConsumer = "docs" }, func(v *credentials) { v.DeviceID = "" }, func(v *credentials) { v.AccessToken = "" },
	} {
		foreign := saved
		change(&foreign)
		if _, err := client.decodeSession(encode(foreign)); !errors.Is(err, ErrIdentity) {
			t.Fatal("foreign legacy credential was accepted", err)
		}
	}
	for _, malformed := range [][]byte{nil, []byte("null"), []byte("[]"), []byte("{invalid}")} {
		if _, err := client.decodeSession(malformed); !errors.Is(err, ErrIdentity) {
			t.Fatal("malformed legacy credential was accepted", err)
		}
	}
}
