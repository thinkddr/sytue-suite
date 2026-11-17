//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestFinalizingCaptureRetainsExactReceiptBeforeAnyCleanup(t *testing.T) {
	plan, sdk := batchStageFixture(t)
	published, err := PublishInputBatch(t.Context(), plan, sdk, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := atomicOwner(plan.data.root.Scope)
	active := storage.AtomicJournalNamespace(plan.data.root.Scope.Product)
	history := storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
	before, err := plan.data.journal.CountOwner(owner, active)
	if err != nil || before == 0 {
		t.Fatal("missing active work", before, err)
	}
	acquired, released := 0, 0
	lease := func(ctx context.Context) (func(), error) { acquired++; return func() { released++ }, ctx.Err() }
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
		t.Fatal(err)
	}
	if acquired != 1 || released != 1 {
		t.Fatal("lease not held and released", acquired, released)
	}
	var receipt finalizingCapture
	found, err := plan.data.journal.Read(owner, history, finalizingCaptureKey(plan.data.root.Operation), &receipt)
	if err != nil || !found || receipt.Version != 1 || receipt.Batch.Root == nil || *receipt.Batch.Root != published.Reference || receipt.Batch.Count != plan.Count() || !atomicValidHash(receipt.MembersSHA256) {
		t.Fatal("missing exact receipt", receipt, err)
	}
	for i := uint64(0); i < plan.Count(); i++ {
		var member finalizingCaptureMember
		found, err := plan.data.journal.Read(owner, history, finalizingCaptureMemberKey(plan.data.root.Operation, i), &member)
		if err != nil || !found || member.Ordinal != i || member.BatchID != receipt.Batch.BatchID || !member.Reference.Valid() || !atomicValidHash(member.RecordSHA256) || inputMemberHash(member.Input) != plan.data.members[i] {
			t.Fatal("missing exact member", i, err)
		}
	}
	after, err := plan.data.journal.CountOwner(owner, active)
	if err != nil || after != before {
		t.Fatal("capture removed active work", before, after, err)
	}
	puts, prepares := sdk.puts, sdk.prepare
	sdk.binding.Target.DeviceID = "CURRENT_B"
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
		t.Fatal("historical readback with B", err)
	}
	if sdk.puts != puts || sdk.prepare != prepares {
		t.Fatal("history capture resent as A")
	}
}

func TestFinalizingCaptureRefusesMissingHistoryAndChangedSDK(t *testing.T) {
	for _, scenario := range []string{"no lease", "missing member", "changed member", "retired SDK"} {
		t.Run(scenario, func(t *testing.T) {
			plan, sdk := batchStageFixture(t)
			if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
				t.Fatal(err)
			}
			lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
			if scenario == "no lease" {
				if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, nil); err == nil {
					t.Fatal("lease-less capture accepted")
				}
				return
			}
			if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
				t.Fatal(err)
			}
			owner, history := atomicOwner(plan.data.root.Scope), storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
			key := finalizingCaptureMemberKey(plan.data.root.Operation, 0)
			switch scenario {
			case "missing member":
				if err := plan.data.journal.Delete(owner, history, key); err != nil {
					t.Fatal(err)
				}
			case "changed member":
				var member finalizingCaptureMember
				found, err := plan.data.journal.Read(owner, history, key, &member)
				if err != nil || !found {
					t.Fatal(err)
				}
				member.Ordinal++
				if err := plan.data.journal.Write(owner, history, key, member); err != nil {
					t.Fatal(err)
				}
			case "retired SDK":
				sdk.retired = true
			}
			if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err == nil {
				t.Fatal("missing or changed evidence accepted")
			}
			if scenario == "missing member" {
				var member finalizingCaptureMember
				found, err := plan.data.journal.Read(owner, history, key, &member)
				if err != nil || found {
					t.Fatal("captured missing history was recreated", err)
				}
			}
			if count, err := plan.data.journal.CountOwner(owner, storage.AtomicJournalNamespace(plan.data.root.Scope.Product)); err != nil || count == 0 {
				t.Fatal("failed capture retired pending work", count, err)
			}
		})
	}
}

func TestFinalizingCaptureHonorsCancelledLease(t *testing.T) {
	plan, sdk := batchStageFixture(t)
	if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := CaptureFinalizationReceipt(ctx, plan, sdk, nil, func(context.Context) (func(), error) {
		t.Fatal("acquired after cancellation")
		return nil, nil //nolint:nilnil // test double: deliberately returns (nil, nil) to prove callers reject it
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFinalizingCaptureNeverMarksAnUnverifiedBatchComplete(t *testing.T) {
	plan, sdk := batchStageFixture(t)
	if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
		t.Fatal(err)
	}
	sdk.afterRootRead = func() { sdk.retired = true }
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err == nil {
		t.Fatal("retired SDK became durable completion")
	}
	owner := atomicOwner(plan.data.root.Scope)
	history := storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
	var receipt finalizingCapture
	found, err := plan.data.journal.Read(owner, history, finalizingCaptureKey(plan.data.root.Operation), &receipt)
	if err != nil || found {
		t.Fatal("unverified batch acquired final marker", err)
	}
}

func TestFinalizingCaptureRechecksOwnedSourceBeforeMarker(t *testing.T) {
	plan, sdk := batchStageFixture(t)
	if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
		t.Fatal(err)
	}
	_, member, err := plan.readCapturedMember(t.Context(), 0)
	if err != nil || member.Source == nil {
		t.Fatal("missing staged source", err)
	}
	owner := atomicOwner(plan.data.root.Scope)
	active := storage.AtomicJournalNamespace(plan.data.root.Scope.Product)
	sdk.afterRootRead = func() {
		if err := plan.data.journal.DeleteStagedFile(owner, active, *member.Source); err != nil {
			t.Fatal(err)
		}
	}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err == nil {
		t.Fatal("missing original staged input became complete receipt")
	}
	var receipt finalizingCapture
	found, err := plan.data.journal.Read(owner, storage.AtomicHistoryNamespace(plan.data.root.Scope.Product), finalizingCaptureKey(plan.data.root.Operation), &receipt)
	if err != nil || found {
		t.Fatal("missing source acquired completion marker", err)
	}
}

func TestFinalizingCaptureResumesPartialHistoryBeforeMarker(t *testing.T) {
	plan, sdk := batchStageFixture(t)
	if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
		t.Fatal(err)
	}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
		t.Fatal(err)
	}
	owner, history := atomicOwner(plan.data.root.Scope), storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
	// Simulate interruption after writing one member but before the completion marker.
	if err := plan.data.journal.Delete(owner, history, finalizingCaptureKey(plan.data.root.Operation)); err != nil {
		t.Fatal(err)
	}
	if err := plan.data.journal.Delete(owner, history, finalizingCaptureMemberKey(plan.data.root.Operation, 1)); err != nil {
		t.Fatal(err)
	}
	puts, prepares := sdk.puts, sdk.prepare
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
		t.Fatal("partial receipt did not resume", err)
	}
	if sdk.puts != puts || sdk.prepare != prepares {
		t.Fatal("partial receipt resent original publication")
	}
	var receipt finalizingCapture
	found, err := plan.data.journal.Read(owner, history, finalizingCaptureKey(plan.data.root.Operation), &receipt)
	if err != nil || !found || receipt.Batch.Root == nil {
		t.Fatal("resumed receipt missing", err)
	}
}
