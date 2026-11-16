// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestCipherUsageCountsExactOwnerProductAndRetainsPendingProviderDeletion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	seed := func(tenant, owner, product, id, key string, size int64) domain.CipherBlob {
		t.Helper()
		ctx := store.WithOwner(tenantCtx(tenant), owner)
		if _, err := st.EnsureQuota(ctx, 10000); err != nil {
			t.Fatal(err)
		}
		blob := domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: strings.Repeat(id, 26), Product: product, SizeBytes: size, SHA256: strings.Repeat("a", 64)}, OwnerSub: owner, StorageKey: key}
		if err := st.ReservarSubida(ctx, key, size, time.Now().Add(time.Minute), !store.EsExterno(key)); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateCipherBlob(ctx, blob); err != nil {
			t.Fatal(err)
		}
		return blob
	}
	local := seed("acme", "alice", "mail", "A", "cipher/local", 60)
	seed("acme", "alice", "mail", "B", store.ExternalPrefix+"external/opaque", 40)
	seed("acme", "alice", "drive", "C", "cipher/drive", 11)
	seed("acme", "bob", "mail", "D", "cipher/bob", 70)
	seed("other", "alice", "mail", "E", "cipher/other", 90)
	ctx := store.WithOwner(tenantCtx("acme"), "alice")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	check := func(total, sytue, external, pending, count int64) {
		t.Helper()
		value, err := st.CipherUsage(ctx, "mail")
		if err != nil || !value.Valid(scope) || value.Bytes != total || value.SytueBytes != sytue || value.ExternalBytes != external || value.PendingDeleteBytes != pending || value.Blobs != count {
			t.Fatal("cipher usage does not match physically retained scope", value, err)
		}
	}
	check(100, 60, 40, 0, 2)
	deleting, err := st.DeleteCipherBlob(ctx, "mail", local.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(100, 60, 40, 60, 2)
	if err = st.CompleteCipherBlobDelete(ctx, deleting); err != nil {
		t.Fatal(err)
	}
	check(40, 0, 40, 0, 1)
	for _, other := range []struct {
		tenant, owner, product string
		want                   int64
	}{{"acme", "bob", "mail", 70}, {"other", "alice", "mail", 90}, {"acme", "alice", "drive", 11}, {"acme", "alice", "notes", 0}} {
		value, err := st.CipherUsage(store.WithOwner(tenantCtx(other.tenant), other.owner), other.product)
		if err != nil || value.Bytes != other.want {
			t.Fatal("aggregate crossed owner/tenant/product", other, err)
		}
	}
	if _, err = st.CipherUsage(tenantCtx("acme"), "mail"); !errors.Is(err, domain.ErrPermission) {
		t.Fatal("ownerless aggregate accepted", err)
	}
}
