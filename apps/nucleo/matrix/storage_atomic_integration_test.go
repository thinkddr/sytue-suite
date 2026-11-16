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
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Actual maintained SDK, Go hybrid age and disposable Synapse. This proves the
// catalog protocol only; no Mail batch writer or migration readiness is enabled.
func TestNativeStorageAtomicCatalogPublishesSingleCommitAcrossSDKRestart(t *testing.T) {
	endpoint := os.Getenv("SYTUE_MATRIX_ATOMIC_SYNAPSE_URL")
	if endpoint == "" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	targetURL, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(targetURL))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	client := device.Client
	defer func() { _ = client.Close() }()
	scope := client.storageScope()
	target, err := client.StoragePublicationTarget()
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
			t.Fatal("actual atomic record publication", e)
		}
		return ref
	}
	batch := "artificial_atomic_batch"
	members := []storage.AtomicMember{}
	for i, id := range []string{"atomic_object_a", "atomic_object_b"} {
		content := storage.Record{Version: 1, Scope: scope, ObjectID: id, RevisionID: "old_content", Role: "content", Payload: []byte(`{"unknown":9007199254740993,"body":"artificial private atomic body"}`)}
		body := put(id+"_content", content)
		old := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"integer":9007199254740993,"old":true}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: content.RevisionID, Reference: body}}
		prior, e := client.PutStorageObjectAt(ctx, target, id+"_old", id, "old_index", old)
		if e != nil {
			t.Fatal(e)
		}
		changed := old
		changed.Metadata = []byte(`{"integer":9007199254740993,"unknown":{"retained":true},"changed":true}`)
		changed.Parents = []storage.Reference{prior.Reference}
		prepared := storage.Record{Version: 1, Scope: scope, ObjectID: id, RevisionID: "prepared_index", Role: "index", Previous: []string{prior.Reference.EventID}, Payload: marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: batch, Ordinal: uint64(i), Commit: changed})}
		ref := put(id+"_prepared", prepared)
		sum, e := storage.AtomicRecordSHA256(prepared)
		if e != nil {
			t.Fatal(e)
		}
		members = append(members, storage.AtomicMember{Reference: ref, ObjectID: id, RevisionID: prepared.RevisionID, SHA256: sum})
	}
	before, err := storage.LoadAtomicCatalog(ctx, client, scope)
	if err != nil || len(before.Heads) != 2 || len(before.Entries) != 4 || len(before.Pages) != 0 {
		t.Fatal("prepared records altered prior visibility", err)
	}
	for _, v := range before.Heads {
		if !bytes.Contains(v.Commit.Metadata, []byte(`"old":true`)) {
			t.Fatal("prepared subset visible")
		}
	}
	if _, err := storage.LoadCatalog(ctx, client, scope); err == nil {
		t.Fatal("old reader accepted prepared protocol")
	}
	var last *storage.AtomicPagePointer
	for i, member := range members {
		revision := []string{"page_a", "page_b"}[i]
		record := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID(batch), RevisionID: revision, Role: "manifest", Payload: marshal(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: batch, Start: uint64(i), Members: []storage.AtomicMember{member}, Previous: last})}
		ref := put("atomic_"+revision, record)
		sum, e := storage.AtomicRecordSHA256(record)
		if e != nil {
			t.Fatal(e)
		}
		last = &storage.AtomicPagePointer{Reference: ref, RevisionID: revision, SHA256: sum}
	}
	root := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID(batch), RevisionID: "atomic_commit", Role: "index", Payload: marshal(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: batch, Count: 2, Last: *last})}
	accepted := put("atomic_commit", root) // Discard the caller ACK before the real SDK restart.
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if retry := put("atomic_commit", root); retry != accepted {
		t.Fatal("root retry duplicated exact accepted Matrix event")
	}
	after, err := storage.LoadAtomicCatalog(ctx, client, scope)
	if err != nil || len(after.Heads) != 2 || len(after.History) != 4 || len(after.Entries) != 5 || len(after.Pages) != 2 || after.Batches[batch] != accepted {
		t.Fatal("committed catalog incomplete after SDK restart", err, len(after.Heads), len(after.Entries))
	}
	for _, v := range after.Heads {
		if !bytes.Contains(v.Commit.Metadata, []byte(`"changed":true`)) || !bytes.Contains(v.Commit.Metadata, []byte("9007199254740993")) {
			t.Fatal("atomic commit lost exact metadata")
		}
	}
	actual, err := client.ReadStorageRecord(ctx, accepted, root.ObjectID, "index")
	if err != nil || !root.MatchesExact(actual) {
		t.Fatal("root ACK not bound to original exact private record", err)
	}
	t.Log("actual SDK/Synapse/PQC: prepared hidden; single root; two pages; raw history; exact ACK after SDK SQLite/key reopen")
}
