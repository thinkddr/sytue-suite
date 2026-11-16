//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// A partial history can survive an interrupted capture. A conflicting row in
// that history must not be overwritten or promoted to a completion marker.
func TestFinalizationReceiptRejectsConflictingPartialHistory(t *testing.T) {
	ctx := t.Context()
	plan, sdk := batchStageFixture(t)
	if plan.Count() < 2 {
		t.Fatal("fixture must contain at least two members")
	}
	published, err := PublishInputBatch(ctx, plan, sdk, nil)
	if err != nil {
		t.Fatal(err)
	}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := CaptureFinalizationReceipt(ctx, plan, sdk, nil, lease); err != nil {
		t.Fatal(err)
	}
	journal := plan.data.journal
	owner := atomicOwner(plan.data.root.Scope)
	active := storage.AtomicJournalNamespace(plan.data.root.Scope.Product)
	history := storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
	markerKey := finalizingCaptureKey(plan.data.root.Operation)
	firstKey := finalizingCaptureMemberKey(plan.data.root.Operation, 0)
	secondKey := finalizingCaptureMemberKey(plan.data.root.Operation, 1)
	var original finalizingCaptureMember
	if found, err := journal.Read(owner, history, firstKey, &original); err != nil || !found {
		t.Fatal("missing original member", err)
	}
	var originalSecond finalizingCaptureMember
	if found, err := journal.Read(owner, history, secondKey, &originalSecond); err != nil || !found {
		t.Fatal("missing second original member", err)
	}
	if err := journal.Delete(owner, history, markerKey); err != nil {
		t.Fatal(err)
	}
	if err := journal.Delete(owner, history, secondKey); err != nil {
		t.Fatal(err)
	}
	conflict := original
	conflict.RecordSHA256 = strings.Repeat("0", 64)
	if conflict.RecordSHA256 == original.RecordSHA256 {
		conflict.RecordSHA256 = strings.Repeat("1", 64)
	}
	if err := journal.Write(owner, history, firstKey, conflict); err != nil {
		t.Fatal(err)
	}
	before := receiptJournalSnapshot(t, journal, owner, active, history)
	if len(before[active]) == 0 || len(before[history]) == 0 {
		t.Fatal("fixture lost pending or partial work")
	}
	puts, prepares := sdk.puts, sdk.prepare
	if err := CaptureFinalizationReceipt(ctx, plan, sdk, nil, lease); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("conflicting partial history was not rejected", err)
	}
	after := receiptJournalSnapshot(t, journal, owner, active, history)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed capture changed partial or active journal IDs or values")
	}
	var marker finalizingCapture
	if found, err := journal.Read(owner, history, markerKey, &marker); err != nil || found {
		t.Fatal("conflicting partial history acquired a completion marker", err)
	}
	var missing finalizingCaptureMember
	if found, err := journal.Read(owner, history, secondKey, &missing); err != nil || found {
		t.Fatal("failed capture created a later member", err)
	}
	if sdk.puts != puts || sdk.prepare != prepares {
		t.Fatal("failed capture resent the publication")
	}
	// An operator restoring the exact retained member can safely retry without
	// publishing the batch again. This does not test physical store corruption.
	if err := journal.Write(owner, history, firstKey, original); err != nil {
		t.Fatal(err)
	}
	if err := CaptureFinalizationReceipt(ctx, plan, sdk, nil, lease); err != nil {
		t.Fatal("exact partial history could not resume", err)
	}
	if found, err := journal.Read(owner, history, markerKey, &marker); err != nil || !found || marker.Batch.Root == nil || *marker.Batch.Root != published.Reference {
		t.Fatal("resumed capture did not retain its completion marker", err)
	}
	if found, err := journal.Read(owner, history, secondKey, &missing); err != nil || !found || !reflect.DeepEqual(missing, originalSecond) {
		t.Fatal("resumed capture did not retain its missing member", err)
	}
	if sdk.puts != puts || sdk.prepare != prepares {
		t.Fatal("resumed capture resent the publication")
	}
}
