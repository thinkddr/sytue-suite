// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
)

// The maintained SDK, hybrid age, original file uploader, upload WAL, manifest,
// indices and receipts are real. Ciphertext HTTP uses the existing typed file
// test gateway; this is not a PostgreSQL/provider or full-product proof.
func TestNativeAtomicFileCompletionRevalidatesSDKAfterRestartAndRetainsWrongDigestWAL(t *testing.T) {
	endpoint := os.Getenv("SYTUE_MATRIX_ATOMIC_SYNAPSE_URL")
	if endpoint == "" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	upstream, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	client := device.Client
	defer func() { _ = client.Close() }()
	scope := client.storageScope()
	target, err := client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	seed, _, _, _ := fileFixture(t)
	gateway := seed.gateway
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	request := storage.FileRequest{ObjectID: "atomic_file", RevisionID: "original_file_revision", SizeBytes: int64(len("actual atomic file body")), Metadata: []byte(`{"unknown":9007199254740993,"synthetic_body":true}`)}
	fileRef, err := client.UploadStorageFileAt(ctx, gateway, journal, "atomic_file_job", target, request, bytes.NewReader([]byte("actual atomic file body")))
	if err != nil {
		t.Fatal(err)
	}
	marshal := func(value any) []byte {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	put := func(txn string, record storage.Record) storage.Reference {
		ref, e := client.PutStorageRecordAt(ctx, target, txn, record)
		if e != nil {
			t.Fatal(e)
		}
		return ref
	}
	batchID := "atomic_file_completion_batch"
	members := []storage.AtomicMember{}
	var fileMember storage.Reference
	for i, id := range []string{request.ObjectID, "atomic_other_member"} {
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{}`)}
		if i == 0 {
			commit.State = "live"
			commit.Content = &storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: fileRef}
		}
		record := storage.Record{Version: 1, Scope: scope, ObjectID: id, RevisionID: "prepared_index", Role: "index", Payload: marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: batchID, Ordinal: uint64(i), Commit: commit})}
		ref := put(id+"_prepared", record)
		if i == 0 {
			fileMember = ref
		}
		sum, e := storage.AtomicRecordSHA256(record)
		if e != nil {
			t.Fatal(e)
		}
		members = append(members, storage.AtomicMember{Reference: ref, ObjectID: id, RevisionID: record.RevisionID, SHA256: sum})
	}
	page := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID(batchID), RevisionID: "manifest_page", Role: "manifest", Payload: marshal(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: batchID, Start: 0, Members: members})}
	pageRef := put("atomic_file_page", page)
	sum, err := storage.AtomicRecordSHA256(page)
	if err != nil {
		t.Fatal(err)
	}
	root := storage.Record{Version: 1, Scope: scope, ObjectID: page.ObjectID, RevisionID: "batch_root", Role: "index", Payload: marshal(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: batchID, Count: 2, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: page.RevisionID, SHA256: sum}})}
	rootRef := put("atomic_file_root", root)
	files, err := filesdk.New(scope, client, gateway, journal)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := storage.VerifyAtomicBatch(ctx, client, scope, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := verified.FileCompletion(ctx, files, fileMember)
	if err != nil {
		t.Fatal(err)
	}
	ioState := storageFileIO{scope: scope, journal: journal}
	var pending storageFileUpload
	if found, e := journal.Read(ioState.journalOwner(), storage.PendingUploadsNamespace, "atomic_file_job", &pending); e != nil || !found || !pending.Verified {
		t.Fatal("verified upload intent absent", e)
	}
	saved := marshal(pending)
	pending.SourceSHA256 = strings.Repeat("f", 64)
	pending.File.SHA256 = pending.SourceSHA256
	if err := journal.Write(ioState.journalOwner(), storage.PendingUploadsNamespace, "atomic_file_job", pending); err != nil {
		t.Fatal(err)
	}
	if err := files.AcknowledgeAtomic(ctx, "atomic_file_job", completion); err == nil {
		t.Fatal("wrong durable source digest consumed file WAL")
	}
	if err := json.Unmarshal(saved, &pending); err != nil {
		t.Fatal(err)
	}
	clear(saved)
	if err := journal.Write(ioState.journalOwner(), storage.PendingUploadsNamespace, "atomic_file_job", pending); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := files.AcknowledgeAtomic(ctx, "atomic_file_job", completion); err == nil {
		t.Fatal("retired SDK consumed accepted upload WAL")
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	files, err = filesdk.New(scope, client, gateway, journal)
	if err != nil {
		t.Fatal(err)
	}
	verified, err = storage.VerifyAtomicBatch(ctx, client, scope, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	completion, err = verified.FileCompletion(ctx, files, fileMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := files.AcknowledgeAtomic(ctx, "atomic_file_job", completion); err != nil {
		t.Fatal(err)
	}
	if found, e := journal.Read(ioState.journalOwner(), storage.PendingUploadsNamespace, "atomic_file_job", &pending); e != nil || found {
		t.Fatal("exact accepted batch did not retire upload WAL", e)
	}
	if _, err := client.ReadStorageRecord(ctx, fileRef, request.ObjectID, "manifest"); err != nil {
		t.Fatal("ACK removed retained file manifest", err)
	}
	t.Log("actual SDK/Synapse: root+all-members+page readback; original file manifest; wrong-digest/retired denial; SQLite restart; exact upload-WAL-only ACK; typed ciphertext test gateway, no PG/provider claim")
}
