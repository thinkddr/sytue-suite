// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import "testing"

func TestDropboxSoloUsaLaInstanciaConsentida(t *testing.T) {
	if _, _, err := configDe(externalStorageRequest{Kind: "dropbox"}); err == nil {
		t.Fatal("Dropbox sin una instancia consentida no se puede conectar")
	}
	cfg, secret, err := configDe(externalStorageRequest{
		Kind: "dropbox", InstanceID: " cuenta ", SecretKey: "no-usar",
	})
	if err != nil || cfg.Kind != "dropbox" || cfg.InstanceID != "cuenta" || secret != "" {
		t.Fatalf("Dropbox debe reutilizar el vault de integraciones: cfg=%+v secret=%q err=%v", cfg, secret, err)
	}
}
