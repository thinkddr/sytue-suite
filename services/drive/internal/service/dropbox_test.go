// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestElEnrutadorAbreDropboxPorConsentimiento(t *testing.T) {
	r := &blobRouter{conn: newIntegrationsClient("https://integrations.sytue.eu", "interno")}
	b, err := r.abrir(tenancy.Tenant{Slug: "acme"}, store.ExternalStorage{Kind: "dropbox", InstanceID: "instance"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*objectstore.Dropbox); !ok {
		t.Fatalf("Dropbox no debe caer en el backend S3: %T", b)
	}
}
