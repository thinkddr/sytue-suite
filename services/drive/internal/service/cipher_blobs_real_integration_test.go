// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/testsupport/cipherfixture"
)

// TestCipherBlobPersistsOnlyCiphertext uses the actual Drive service, its RLS
// store and an explicitly configured disposable S3-compatible service. It
// inspects both persistence boundaries, not HTTP auth, Matrix or migration.
func TestCipherBlobPersistsOnlyCiphertext(t *testing.T) {
	ctx := t.Context()
	fixture := cipherfixture.New(t)
	rawS3, bucket, h, svc := fixture.S3, fixture.Bucket, fixture.PG, fixture.Service
	tenant := tenancy.Tenant{Slug: "acme"}
	owner := store.WithOwner(tenancy.Into(ctx, tenant), "alice")
	scope := storage.Scope{Subject: "alice", Tenant: tenant.Slug, Product: "drive"}
	plaintext := []byte("artificial private content: cedar violet 946215")
	block, ciphertext, err := filesdk.PrepareBlock(scope, "object1", "revision1", 0, bytes.NewReader(plaintext), int64(len(plaintext)))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := svc.PutCipherBlob(owner, scope.Product, block.Blob.ID, int64(len(ciphertext)), bytes.NewReader(ciphertext))
	if err != nil {
		t.Fatal("persist ciphertext:", err)
	}
	if saved.BlobReceipt != block.Blob {
		t.Fatal("persisted receipt differs from client descriptor")
	}
	// Read the provider directly, not through Drive's decoding path.
	object, err := rawS3.GetObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil || !bytes.Equal(stored, ciphertext) || bytes.Contains(stored, plaintext) {
		t.Fatal("provider did not persist the original opaque bytes:", err)
	}
	var row []byte
	err = h.Pool.InTenantTx(owner, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, scope.Subject); e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT row_to_json(c)::text FROM cipher_blobs c WHERE id=$1`, block.Blob.ID).Scan(&row)
	})
	if err != nil {
		t.Fatal("inspect persisted row:", err)
	}
	descriptor, err := json.Marshal(block.File)
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		Key struct {
			K string `json:"k"`
		} `json:"key"`
	}
	if err = json.Unmarshal(descriptor, &encoded); err != nil || encoded.Key.K == "" {
		t.Fatal("client descriptor is missing its key:", err)
	}
	if bytes.Contains(row, plaintext) || bytes.Contains(row, []byte(encoded.Key.K)) {
		t.Fatal("private source or descriptor reached the database")
	}
	if err = filesdk.DecryptBlockInPlace(scope, "object1", "revision1", 0, block, stored); err != nil || !bytes.Equal(stored, plaintext) {
		t.Fatal("client could not authenticate persisted block:", err)
	}
	// The same request succeeds while the committed provider object is intact.
	// This distinguishes corruption detection from a general failure on retries.
	retried, err := svc.PutCipherBlob(owner, scope.Product, block.Blob.ID, int64(len(ciphertext)), bytes.NewReader(ciphertext))
	if err != nil || retried != saved {
		t.Fatalf("retry intact ciphertext: got %+v, err %v", retried, err)
	}
	// Simulate a provider corrupting an object after acknowledging the upload.
	// Retrying a lost receipt must not claim success while persisted bytes differ.
	corrupted := bytes.Clone(ciphertext)
	corrupted[0] ^= 1
	if _, err = rawS3.PutObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, bytes.NewReader(corrupted), int64(len(corrupted)), minio.PutObjectOptions{}); err != nil {
		t.Fatal("corrupt isolated provider object:", err)
	}
	_, err = svc.PutCipherBlob(owner, scope.Product, block.Blob.ID, int64(len(ciphertext)), bytes.NewReader(ciphertext))
	if !errors.Is(err, service.ErrCipherBlobCorrupt) {
		t.Fatalf("retry corrupted ciphertext: got %v, want ErrCipherBlobCorrupt", err)
	}
	// Inspect the provider after the retry: rejecting corruption must not repair
	// or overwrite the committed object, and the database receipt stays pinned.
	object, err = rawS3.GetObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal("reopen corrupted object:", err)
	}
	storedAfterRetry, err := io.ReadAll(object)
	closeErr := object.Close()
	if err != nil || closeErr != nil || !bytes.Equal(storedAfterRetry, corrupted) {
		t.Fatalf("retry changed the provider object: read %v, close %v", err, closeErr)
	}
	retained, err := svc.CipherBlob(owner, scope.Product, block.Blob.ID)
	if err != nil || retained != saved {
		t.Fatalf("retry changed the committed receipt: got %+v, err %v", retained, err)
	}
	_, stream, err := svc.OpenCipherBlob(owner, scope.Product, block.Blob.ID)
	if err != nil {
		t.Fatal("open corrupted object:", err)
	}
	actual, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil || filesdk.DecryptBlockInPlace(scope, "object1", "revision1", 0, block, actual) == nil {
		t.Fatal("client accepted altered provider bytes:", err)
	}
	other := store.WithOwner(tenancy.Into(ctx, tenant), "bob")
	if _, _, err = svc.OpenCipherBlob(other, scope.Product, block.Blob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("another owner could read ciphertext:", err)
	}
	// Missing bytes have a distinct error and do not make retry recreate them.
	if err = rawS3.RemoveObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.RemoveObjectOptions{}); err != nil {
		t.Fatal("remove isolated provider object:", err)
	}
	if _, err = svc.PutCipherBlob(owner, scope.Product, block.Blob.ID, int64(len(ciphertext)), bytes.NewReader(ciphertext)); !errors.Is(err, service.ErrCipherBlobMissing) {
		t.Fatalf("retry missing ciphertext: got %v, want ErrCipherBlobMissing", err)
	}
	if _, err = rawS3.StatObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.StatObjectOptions{}); err == nil || minio.ToErrorResponse(err).StatusCode != 404 {
		t.Fatal("retry recreated a missing object or provider failed:", err)
	}
	// Restore the exact committed ciphertext to make the deletion assertion
	// meaningful: an already-absent object cannot prove that Drive removed it.
	if _, err = rawS3.PutObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, bytes.NewReader(ciphertext), int64(len(ciphertext)), minio.PutObjectOptions{}); err != nil {
		t.Fatal("restore committed ciphertext before deletion:", err)
	}
	if _, err = rawS3.StatObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.StatObjectOptions{}); err != nil {
		t.Fatal("restored provider object is not present:", err)
	}
	if err = svc.DeleteCipherBlob(owner, scope.Product, block.Blob.ID); err != nil {
		t.Fatal("delete ciphertext:", err)
	}
	if _, err = rawS3.StatObject(ctx, bucket, tenant.StoragePrefix()+saved.StorageKey, minio.StatObjectOptions{}); err == nil || minio.ToErrorResponse(err).StatusCode != 404 {
		t.Fatal("provider did not confirm object absence after deletion:", err)
	}
	if exists, err := rawS3.BucketExists(ctx, bucket); err != nil || !exists {
		t.Fatalf("test bucket unavailable after object deletion: exists %t, err %v", exists, err)
	}
	if _, err = svc.CipherBlob(owner, scope.Product, block.Blob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted receipt is still readable: %v", err)
	}
}
