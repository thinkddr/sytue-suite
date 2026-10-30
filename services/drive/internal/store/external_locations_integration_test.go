// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestExternalLocationsRemainPinnedAcrossAccountSwitchAndDisable(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	a, err := st.SaveExternalStorage(ctx, store.ExternalStorage{Kind: "gdrive", InstanceID: "first-account", Endpoint: "oauth", Bucket: "drive", Enabled: true, AcceptedTermsAt: time.Now()}, "alice")
	if err != nil || a.LocationID == uuid.Nil {
		t.Fatal("missing initial location", err)
	}
	cfg := a
	cfg.InstanceID = "second-account"
	b, err := st.SaveExternalStorage(ctx, cfg, "alice")
	if err != nil || a.LocationID == b.LocationID {
		t.Fatal("account switch reused original location", err)
	}
	old, err := st.ExternalStorageLocation(ctx, "acme", a.LocationID)
	if err != nil || old.InstanceID != "first-account" {
		t.Fatal("old bytes redirected to new account", err)
	}
	cfg = b
	cfg.Enabled = false
	disabled, err := st.SaveExternalStorage(ctx, cfg, "alice")
	if err != nil || disabled.LocationID != b.LocationID {
		t.Fatal("toggle changed physical destination", err)
	}
	if _, err = st.ExternalStorageFor(ctx, "acme"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("disabled location still selected for writes", err)
	}
	if old, err = st.ExternalStorageLocation(ctx, "acme", b.LocationID); err != nil || old.InstanceID != "second-account" {
		t.Fatal("disabled previous ciphertext inaccessible", err)
	}
	if _, err = st.ExternalStorageLocation(ctx, "other", a.LocationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("location crossed tenant", err)
	}
}
