// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestCipherBlobsOwnerTenantProductQuotaAndRetryableDeletion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := store.WithOwner(tenantCtx("acme"), "alice")
	if _, err := st.EnsureQuota(ctx, 100); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("A", 26)
	blob := domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: id, Product: "mail", SizeBytes: 60, SHA256: strings.Repeat("a", 64)}, OwnerSub: "alice", StorageKey: "cipher/opaque"}
	if err := st.ReservarSubida(ctx, blob.StorageKey, blob.SizeBytes, time.Now().Add(time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCipherBlob(ctx, blob); err != nil {
		t.Fatal(err)
	}
	for _, other := range []struct{ tenant, actor, product string }{{"other", "alice", "mail"}, {"acme", "bob", "mail"}, {"acme", "alice", "drive"}} {
		if _, err := st.CipherBlob(store.WithOwner(tenantCtx(other.tenant), other.actor), other.product, id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("ciphertext metadata crossed identity/product", other, err)
		}
	}
	for _, visibility := range []struct {
		tenant, owner string
		want          int
	}{{"acme", "alice", 1}, {"acme", "bob", 0}, {"other", "alice", 0}} {
		if err := h.Pool.InTenantTx(tenantCtx(visibility.tenant), func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SELECT set_config('app.owner_sub',$1,true)`, visibility.owner); err != nil {
				return err
			}
			var n int
			if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM cipher_blobs`).Scan(&n); err != nil {
				return err
			}
			if n != visibility.want {
				return fmt.Errorf("RLS failed independently of application predicates: %v got %d", visibility, n)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	second := blob
	second.ID = strings.Repeat("B", 26)
	second.StorageKey = "cipher/second"
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), `UPDATE quotas SET limit_bytes=200`); return err }); err != nil {
		t.Fatal(err)
	}
	if err := st.ReservarSubida(ctx, second.StorageKey, second.SizeBytes, time.Now().Add(time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), `UPDATE quotas SET limit_bytes=100`); return err }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCipherBlob(ctx, second); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatal("ciphertext bypassed suite quota", err)
	}
	if _, err := st.CreateCipherBlob(ctx, blob); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("immutable ID overwritten", err)
	}
	deleting, err := st.DeleteCipherBlob(ctx, "mail", id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CipherBlob(ctx, "mail", id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("tombstone still readable", err)
	}
	quota, err := st.GetQuota(ctx)
	if err != nil || quota.UsedBytes != 60 {
		t.Fatal("provider failure would lose routing/quota", quota.UsedBytes, err)
	}
	if _, err := st.DeleteCipherBlob(ctx, "mail", id); err != nil {
		t.Fatal("physical deletion could not be retried", err)
	}
	if err := st.CompleteCipherBlobDelete(ctx, deleting); err != nil {
		t.Fatal(err)
	}
	quota, err = st.GetQuota(ctx)
	if err != nil || quota.UsedBytes != 0 {
		t.Fatal("confirmed deletion retained quota", err)
	}
}
