//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Ordering/transaction witnesses only. They are not a replacement for the
// separate maintained Native SDK + Synapse proof of the composed publisher.
type batchPublisherWitness struct {
	*contentPublisherWitness
	longReferences bool
	loseRootACK    bool
	stopPrepared   bool
	afterRootRead  func()
	afterPage      func()
}

func (p *batchPublisherWitness) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	var root batchPublication
	found, err := p.plan.data.journal.Read(atomicOwner(p.plan.data.root.Scope), storage.AtomicJournalNamespace(p.plan.data.root.Scope.Product), inputBatchKey(p.plan.data.root.Operation), &root)
	if err != nil || !found || !root.Captured {
		p.t.Fatal("batch IO preceded ALL original intent capture", err)
	}
	reference, err := p.contentPublisherWitness.PutStorageRecordAt(ctx, target, transaction, record)
	if err != nil {
		return reference, err
	}
	if p.longReferences {
		sum := sha256.Sum256([]byte(transaction))
		long := storage.Reference{RoomID: reference.RoomID, EventID: "$" + strings.Repeat("x", 190) + hex.EncodeToString(sum[:20])}
		record := p.records[reference]
		delete(p.records, reference)
		p.records[long] = record
		p.transactions[transaction] = long
		reference = long
	}
	var tag struct{ Kind string }
	_ = json.Unmarshal(record.Payload, &tag)
	if tag.Kind == storage.ObjectBatchPageKind && p.afterPage != nil {
		callback := p.afterPage
		p.afterPage = nil
		callback()
	}
	if tag.Kind == storage.ObjectPreparedKind && p.stopPrepared {
		p.stopPrepared = false
		return storage.Reference{}, errors.New("prepared accepted; simulated interruption")
	}
	if tag.Kind == storage.ObjectBatchKind && p.loseRootACK {
		p.loseRootACK = false
		return storage.Reference{}, errors.New("root accepted; simulated lost ACK")
	}
	return reference, nil
}

func (p *batchPublisherWitness) StorageObjectHistory(ctx context.Context, room, cursor string, limit int) (storage.ObjectPage, error) {
	if room != p.binding.Target.RoomID || limit != 100 {
		return storage.ObjectPage{}, storage.ErrRecord
	}
	entries := []storage.RecordEntry{}
	for reference, record := range p.records {
		if record.Role == "index" {
			entries = append(entries, storage.RecordEntry{Reference: reference, Record: record})
		}
	}
	slices.SortFunc(entries, func(a, b storage.RecordEntry) int {
		if a.Reference.EventID < b.Reference.EventID {
			return -1
		}
		if a.Reference.EventID > b.Reference.EventID {
			return 1
		}
		return 0
	})
	offset := 0
	var err error
	if cursor != "" {
		offset, err = strconv.Atoi(cursor)
	}
	if err != nil || offset < 0 || offset > len(entries) {
		return storage.ObjectPage{}, storage.ErrRecord
	}
	end := min(offset+limit, len(entries))
	page := storage.ObjectPage{Entries: entries[offset:end]}
	if end < len(entries) {
		page.Next = strconv.Itoa(end)
	}
	for _, entry := range page.Entries {
		var tag struct{ Kind string }
		_ = json.Unmarshal(entry.Record.Payload, &tag)
		if tag.Kind == storage.ObjectBatchKind && p.afterRootRead != nil {
			callback := p.afterRootRead
			p.afterRootRead = nil
			callback()
		}
	}
	return page, ctx.Err()
}

func (p *batchPublisherWitness) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, err := p.inputReaderWitness.ReadStorageRecord(ctx, ref, object, role)
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record, err
}

func mustBatchRoot(t *testing.T, plan InputPlan) batchPublication {
	t.Helper()
	var root batchPublication
	found, err := plan.data.journal.Read(atomicOwner(plan.data.root.Scope), storage.AtomicJournalNamespace(plan.data.root.Scope.Product), inputBatchKey(plan.data.root.Operation), &root)
	if err != nil || !found {
		t.Fatal("batch root missing", err)
	}
	return root
}

func batchStageFixture(t *testing.T) (InputPlan, *batchPublisherWitness) {
	t.Helper()
	plan, p := contentStageFixture(t)
	return plan, &batchPublisherWitness{contentPublisherWitness: p}
}

func TestAtomicBatchPublisherOneRootMakesAllMembersVisibleAndRetainsPending(t *testing.T) {
	plan, p := batchStageFixture(t)
	result, err := PublishInputBatch(t.Context(), plan, p, nil)
	if err != nil || !result.Reference.Valid() {
		t.Fatal("batch did not publish", err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), p, plan.data.root.Scope)
	if err != nil || len(snapshot.History) != 2 || len(snapshot.Batches) != 1 || len(snapshot.Pages) != 1 {
		t.Fatal("batch projection incomplete", err)
	}
	root := mustBatchRoot(t, plan)
	if root.State != "accepted" || root.Root == nil || *root.Root != result.Reference {
		t.Fatal("accepted root receipt missing")
	}
	if count, err := plan.data.journal.CountOwner(atomicOwner(root.Scope), storage.PrivateJournalNamespaces(root.Scope.Product)...); err != nil || count == 0 {
		t.Fatal("publication incorrectly retired pending cleanup", count, err)
	}
	for _, entry := range snapshot.Entries {
		if entry.Reference == result.Reference {
			continue
		}
		if _, _, err := storage.DecodeAtomicPrepared(entry, root.Scope); err != nil {
			t.Fatal("unexpected visible index", err)
		}
	}
}

func TestAtomicBatchPublisherAcceptedRootRecoveryWithCurrentBDoesNotPrepareOrResendA(t *testing.T) {
	plan, p := batchStageFixture(t)
	p.loseRootACK = true
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err == nil {
		t.Fatal("lost root ACK became success")
	}
	root := mustBatchRoot(t, plan)
	if root.Root != nil || root.State != StateStaging {
		t.Fatal("unread accepted root was checkpointed")
	}
	prepared, puts := p.prepare, p.puts
	p.binding.Target.DeviceID = "CURRENT_B"
	result, err := PublishInputBatch(t.Context(), plan, p, nil)
	if err != nil || !result.Reference.Valid() {
		t.Fatal("current B failed to recover exact accepted root", err)
	}
	if p.prepare != prepared || p.puts != puts {
		t.Fatal("current B prepared or resent original A")
	}
	root = mustBatchRoot(t, plan)
	if root.Root == nil || root.State != "accepted" {
		t.Fatal("readback did not checkpoint exact root")
	}
}

func TestAtomicBatchPublisherPartialPreparedWorkStaysInvisibleAndForeignWriterCannotFinish(t *testing.T) {
	plan, p := batchStageFixture(t)
	p.stopPrepared = true
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err == nil {
		t.Fatal("interrupted prepared publication succeeded")
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), p, plan.data.root.Scope)
	if err != nil || len(snapshot.History) != 0 || len(snapshot.Batches) != 0 {
		t.Fatal("partial batch became visible", err)
	}
	prepared, puts := p.prepare, p.puts
	p.binding.Target.DeviceID = "CURRENT_B"
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err == nil {
		t.Fatal("foreign writer completed original pending A")
	}
	if p.prepare != prepared || p.puts != puts {
		t.Fatal("foreign writer reached preparation or send")
	}
	if mustBatchRoot(t, plan).State != StateStaging {
		t.Fatal("incomplete root lost staging state")
	}
}

func TestAtomicBatchPublisherRecoveryRechecksOriginalContentAndLiveBinding(t *testing.T) {
	for _, fault := range []string{"content", "member-metadata", "binding-post", "cancel-post", "missing-root"} {
		t.Run(fault, func(t *testing.T) {
			plan, p := batchStageFixture(t)
			result, err := PublishInputBatch(t.Context(), plan, p, nil)
			if err != nil {
				t.Fatal(err)
			}
			p.binding.Target.DeviceID = "CURRENT_B"
			prepared, puts := p.prepare, p.puts
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch fault {
			case "content":
				for reference, record := range p.records {
					if record.Role == "content" {
						record.Payload = []byte("changed plaintext")
						p.records[reference] = record
					}
				}
			case "member-metadata":
				_, member, err := plan.readCapturedMember(t.Context(), 0)
				if err != nil {
					t.Fatal(err)
				}
				member.Spec.Metadata = []byte(`{"different":true}`)
				if err := plan.data.journal.Write(atomicOwner(plan.data.root.Scope), storage.AtomicJournalNamespace(plan.data.root.Scope.Product), inputMemberKey(plan.data.root.Operation, 0), member); err != nil {
					t.Fatal(err)
				}
			case "binding-post":
				p.afterRootRead = func() { p.binding.Target.DeviceID = "CURRENT_C" }
			case "cancel-post":
				p.afterRootRead = cancel
			case "missing-root":
				delete(p.records, result.Reference)
			}
			if _, err := PublishInputBatch(ctx, plan, p, nil); err == nil {
				t.Fatal("accepted receipt survived changed original content/current source", fault)
			}
			if p.prepare != prepared || p.puts != puts {
				t.Fatal("failed historical verification resent A")
			}
			if (fault == "binding-post" || fault == "cancel-post") && p.afterRootRead != nil {
				t.Fatal("history test never reached original accepted root")
			}
		})
	}
}

func TestAtomicBatchPublisherPagesBeyondOnePagePreserveOriginalOrder(t *testing.T) {
	plan, p := batchStageFixture(t)
	inputs := make([]Input, inputBatchPageMembers+1)
	for i := range inputs {
		inputs[i] = Input{ObjectID: fmt.Sprintf("deleted_%d", i), State: "deleted", Metadata: []byte(`{"precise":9007199254740993}`)}
	}
	var err error
	plan, err = CaptureInputs(t.Context(), plan.data.journal, plan.data.root.Scope, "paged_batch", plan.data.root.Target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	p.plan = plan
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), p, plan.data.root.Scope)
	if err != nil || len(snapshot.History) != len(inputs) || len(snapshot.Pages) != 2 {
		t.Fatal("paged batch lost complete membership", len(snapshot.History), len(snapshot.Pages), err)
	}
}

func TestAtomicBatchPublisherRechecksAllOriginalContentBeforeVisibilityRoot(t *testing.T) {
	plan, p := batchStageFixture(t)
	p.afterPage = func() {
		for reference, record := range p.records {
			if record.Role == "content" {
				record.Payload = []byte("changed after page ACK")
				p.records[reference] = record
			}
		}
	}
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err == nil {
		t.Fatal("changed original content became a visible root")
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), p, plan.data.root.Scope)
	if err != nil || len(snapshot.Batches) != 0 || len(snapshot.History) != 0 {
		t.Fatal("unverified content made partial batch visible", err)
	}
	if mustBatchRoot(t, plan).State != StateStaging {
		t.Fatal("failed visibility lost pending work")
	}
}

func TestAtomicBatchPublisherPagesFitActualRecordBytesWithLongValidReferences(t *testing.T) {
	plan, p := batchStageFixture(t)
	p.longReferences = true
	target := storage.PublicationTarget{RoomID: "!" + strings.Repeat("r", 254), DeviceID: plan.data.root.Target.DeviceID}
	p.binding.Target = target
	p.binding.Rooms = []string{target.RoomID}
	inputs := make([]Input, 65)
	for i := range inputs {
		inputs[i] = Input{ObjectID: fmt.Sprintf("%s_%03d", strings.Repeat("o", 120), i), State: "deleted", Metadata: []byte(`{"precise":9007199254740993}`)}
	}
	var err error
	plan, err = CaptureInputs(t.Context(), plan.data.journal, plan.data.root.Scope, "byte_bound_batch", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	p.plan = plan
	if _, err := PublishInputBatch(t.Context(), plan, p, nil); err != nil {
		t.Fatal("valid long references could not be paginated", err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), p, plan.data.root.Scope)
	if err != nil || len(snapshot.History) != len(inputs) || len(snapshot.Pages) < 2 {
		t.Fatal("byte bound did not split before member-count bound", len(snapshot.History), len(snapshot.Pages), err)
	}
	for _, entry := range snapshot.Pages {
		if entry.Record.Validate(plan.data.root.Scope) != nil {
			t.Fatal("page exceeds actual Record byte bound")
		}
	}
}
