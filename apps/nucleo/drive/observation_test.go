// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestBodyUsesSameObservationWhenOrphanBecomesCommitted(t *testing.T) {
	r, crypto, _ := pinFixture(t, "file")
	q := crypto.queryRecords
	var body storage.ObjectVersion
	for _, value := range q.extra {
		body = value
	}
	delete(q.extra, body.Reference)
	payload, err := json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "pending", Commit: body.Commit})
	if err != nil {
		t.Fatal(err)
	}
	prepared := storage.RecordEntry{Reference: body.Reference, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: body.ObjectID, RevisionID: body.RevisionID, Role: "index", Payload: payload}}
	q.rawExtra = []storage.RecordEntry{prepared}
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	index, err := r.version(ctx, "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	item, data, raw, err := r.readEntry(ctx, index)
	clear(raw)
	if err != nil || data.Binary == nil {
		t.Fatal("could not capture parent", err)
	}
	old := data.observation
	sum, err := storage.AtomicRecordSHA256(prepared.Record)
	if err != nil {
		t.Fatal(err)
	}
	pageRef := storage.Reference{RoomID: body.Reference.RoomID, EventID: "$page"}
	page := storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "pending", Members: []storage.AtomicMember{{Reference: body.Reference, ObjectID: body.ObjectID, RevisionID: body.RevisionID, SHA256: sum}}}
	pagePayload, _ := json.Marshal(page)
	pageRecord := storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: storage.AtomicBatchObjectID("pending"), RevisionID: "page", Role: "manifest", Payload: pagePayload}
	pageSum, err := storage.AtomicRecordSHA256(pageRecord)
	if err != nil {
		t.Fatal(err)
	}
	q.records[pageRef] = pageRecord
	root := storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "pending", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSum}}
	rootPayload, _ := json.Marshal(root)
	q.rawExtra = append(q.rawExtra, storage.RecordEntry{Reference: storage.Reference{RoomID: body.Reference.RoomID, EventID: "$root"}, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: storage.AtomicBatchObjectID("pending"), RevisionID: "root", Role: "index", Payload: rootPayload}})
	if err := r.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.bodyVersion(ctx, old, *data.Binary, item, "binary"); !errors.Is(err, ErrSource) {
		t.Fatal("old observation exposed a subsequently committed body", err)
	}
	newCtx, err := r.observeCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	item, data, raw, err = r.readEntry(newCtx, index)
	clear(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, got, err := r.bodyVersion(newCtx, data.observation, *data.Binary, item, "binary"); err != nil || got.Reference != body.Reference {
		t.Fatal("new observation did not expose committed body", err)
	}
	done()
	if len(old.snapshot.Entries) != 0 {
		t.Fatal("retired observation remained live after its admission ended")
	}
}

func TestPinnedAncestorRemainsReadableAfterHeadAdvances(t *testing.T) {
	r, crypto, original := pinFixture(t, "file")
	pin, err := r.PinRetainedOriginal(t.Context(), "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	q := crypto.queryRecords
	old := r.heads[objectID("file", "original")][0]
	old.Commit.Metadata = bytes.Clone(old.Commit.Metadata)
	next := old
	next.Reference.EventID = "$next_index"
	next.RevisionID = "next_index"
	next.Commit.Parents = []storage.Reference{old.Reference}
	q.retained = map[storage.Reference]storage.ObjectVersion{old.Reference: old}
	r.heads[old.ObjectID] = []storage.ObjectVersion{next}
	var out bytes.Buffer
	if err := r.WritePinnedOriginal(t.Context(), pin, &out); err != nil || !bytes.Equal(out.Bytes(), original) {
		t.Fatal("retained ancestor was lost when head advanced", err)
	}
}

func TestStreamRetainsObservationUntilWriterJoins(t *testing.T) {
	r, crypto, original := pinFixture(t, "file")
	started, resume := make(chan struct{}), make(chan struct{})
	crypto.reading = func() { close(started); <-resume }
	body, err := r.Download(t.Context(), "original")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(resume)
		_ = body.Close()
		t.Fatal("stream did not reach the authenticated file read")
	}
	r.mu.Lock()
	old := r.observation
	r.mu.Unlock()
	if err := r.refresh(t.Context()); err != nil {
		close(resume)
		_ = body.Close()
		t.Fatal(err)
	}
	if len(old.snapshot.Entries) == 0 {
		close(resume)
		_ = body.Close()
		t.Fatal("refresh discarded a live stream observation")
	}
	close(resume)
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("stream changed across refresh", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if len(old.snapshot.Entries) != 0 {
		t.Fatal("retired stream observation survived drain")
	}
}

func TestCanceledStreamRetainsObservationUntilWorkerReturns(t *testing.T) {
	r, crypto, _ := pinFixture(t, "file")
	started, resume := make(chan struct{}), make(chan struct{})
	crypto.reading = func() { close(started); <-resume }
	ctx, cancel := context.WithCancel(t.Context())
	body, err := r.Download(ctx, "original")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		close(resume)
		_ = body.Close()
		t.Fatal("stream did not reach the authenticated file read")
	}
	r.mu.Lock()
	old := r.observation
	r.mu.Unlock()
	cancel()
	_ = body.Close()
	if err := r.refresh(t.Context()); err != nil {
		close(resume)
		t.Fatal(err)
	}
	if len(old.snapshot.Entries) == 0 {
		close(resume)
		t.Fatal("cancellation erased data while the worker still used it")
	}
	close(resume)
	r.Close()
	if len(old.snapshot.Entries) != 0 {
		t.Fatal("canceled stream observation survived worker drain")
	}
}
