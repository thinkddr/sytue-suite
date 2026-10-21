// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Un check de conexión correcto no basta: la configuración debe guardarse en el
// esquema real sin inventar buckets ni endpoints para los proveedores OAuth.
func TestCadaProveedorGuardaSoloLosCamposQueNecesita(t *testing.T) {
	st := store.New(pgtest.New(t, db.Migrations, db.MigrationsDir).Pool)
	for _, cfg := range []store.ExternalStorage{
		{Kind: "gdrive", InstanceID: "google-consent"},
		{Kind: "onedrive", InstanceID: "microsoft-consent"},
		{Kind: "dropbox", InstanceID: "dropbox-consent"},
		{Kind: "webdav", Endpoint: "https://dav.example.eu", AccessKey: "ana", SecretKeyEnc: "sealed"},
		{Kind: "nas", Endpoint: "https://nas.example.eu", AccessKey: "ana", SecretKeyEnc: "sealed"},
		{Kind: "s3", Endpoint: "s3.example.eu", Bucket: "files", AccessKey: "key", SecretKeyEnc: "sealed"},
	} {
		t.Run(cfg.Kind, func(t *testing.T) {
			cfg.Enabled, cfg.AcceptedTermsAt = true, time.Now()
			ctx := tenantCtx("provider-" + cfg.Kind)
			if _, err := st.SaveExternalStorage(ctx, cfg, "ana"); err != nil {
				t.Fatalf("guardar %s sin campos de otro proveedor: %v", cfg.Kind, err)
			}
			saved, err := st.ExternalStorage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Kind != cfg.Kind || saved.InstanceID != cfg.InstanceID || saved.Endpoint != cfg.Endpoint || saved.Bucket != cfg.Bucket {
				t.Fatalf("la configuración cambió al guardarse: %+v", saved)
			}
		})
	}
}

// La ampliación no elimina las garantías de S3 ni permite OAuth sin una cuenta.
func TestElEsquemaSigueRechazandoConfiguracionesIncompletas(t *testing.T) {
	st := store.New(pgtest.New(t, db.Migrations, db.MigrationsDir).Pool)
	for name, cfg := range map[string]store.ExternalStorage{
		"s3-sin-endpoint":  {Kind: "s3", Bucket: "files"},
		"s3-sin-bucket":    {Kind: "s3", Endpoint: "s3.example.eu"},
		"webdav-sin-url":   {Kind: "webdav"},
		"oauth-sin-cuenta": {Kind: "dropbox"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.AcceptedTermsAt = time.Now()
			if _, err := st.SaveExternalStorage(tenantCtx("invalid"), cfg, "ana"); err == nil {
				t.Fatalf("el esquema aceptó una configuración incompleta: %+v", cfg)
			}
		})
	}
}
