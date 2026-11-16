//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func hashBytesForRecovery(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func captureRecoveryFixture(t *testing.T) (InputPlan, *batchPublisherWitness, finalizingCapture, finalizingCaptureMember) {
	t.Helper()
	plan, sdk := batchStageFixture(t)
	if _, err := PublishInputBatch(t.Context(), plan, sdk, nil); err != nil {
		t.Fatal(err)
	}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, nil, lease); err != nil {
		t.Fatal(err)
	}
	owner, history := atomicOwner(plan.data.root.Scope), storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
	var capture finalizingCapture
	found, err := plan.data.journal.Read(owner, history, finalizingCaptureKey(plan.data.root.Operation), &capture)
	if err != nil || !found {
		t.Fatal(err)
	}
	var member finalizingCaptureMember
	found, err = plan.data.journal.Read(owner, history, finalizingCaptureMemberKey(plan.data.root.Operation, 0), &member)
	if err != nil || !found {
		t.Fatal(err)
	}
	return plan, sdk, capture, member
}

func TestFinalizationReceiptRecoveryDoesNotRequireRetiredInputPlan(t *testing.T) {
	plan, sdk, _, _ := captureRecoveryFixture(t)
	owner, active := atomicOwner(plan.data.root.Scope), storage.AtomicJournalNamespace(plan.data.root.Scope.Product)
	for i := uint64(0); i < plan.Count(); i++ {
		_, member, err := plan.readCapturedMember(t.Context(), i)
		if err != nil {
			t.Fatal(err)
		}
		if member.Source != nil {
			if err := plan.data.journal.DeleteStagedFile(owner, active, *member.Source); err != nil {
				t.Fatal(err)
			}
		}
		if err := plan.data.journal.Delete(owner, active, inputMemberKey(plan.data.root.Operation, i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := plan.data.journal.Delete(owner, active, inputRootKey(plan.data.root.Operation)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInputs(t.Context(), plan.data.journal, plan.data.root.Scope, plan.data.root.Operation); err == nil {
		t.Fatal("fixture unexpectedly retained the plan")
	}
	puts, prepares, reads := sdk.puts, sdk.prepare, sdk.reads
	sdk.binding.Target.DeviceID = "CURRENT_B"
	acquired, released := 0, 0
	lease := func(ctx context.Context) (func(), error) { acquired++; return func() { released++ }, ctx.Err() }
	if err := CheckFinalizationReceipt(t.Context(), plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, nil, lease); err != nil {
		t.Fatal("fresh historical readback failed", err)
	}
	if acquired != 1 || released != 1 || sdk.reads <= reads || sdk.puts != puts || sdk.prepare != prepares {
		t.Fatal("recovery skipped lease/readback or resent as A")
	}
}

func TestFinalizationReceiptRecoveryRejectsChangedEvidence(t *testing.T) {
	for _, scenario := range []string{"missing history", "changed source hash", "missing root", "changed member", "changed content", "binding", "retired", "no lease", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			plan, sdk, capture, member := captureRecoveryFixture(t)
			owner, history := atomicOwner(plan.data.root.Scope), storage.AtomicHistoryNamespace(plan.data.root.Scope.Product)
			key := finalizingCaptureMemberKey(plan.data.root.Operation, 0)
			lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
			ctx := t.Context()
			switch scenario {
			case "missing history":
				if err := plan.data.journal.Delete(owner, history, key); err != nil {
					t.Fatal(err)
				}
			case "changed source hash":
				member.Input.Spec.Content.SHA256 = hashBytesForRecovery([]byte("different"))
				if err := plan.data.journal.Write(owner, history, key, member); err != nil {
					t.Fatal(err)
				}
			case "missing root":
				delete(sdk.records, *capture.Batch.Root)
			case "changed member":
				record := sdk.records[member.Reference]
				record.Payload = []byte(`{"kind":"changed"}`)
				sdk.records[member.Reference] = record
			case "changed content":
				pointer := member.Prepared.Mutation.Commit.Content
				if pointer == nil {
					t.Fatal("record fixture missing")
				}
				record := sdk.records[pointer.Reference]
				record.Payload = []byte(`{"changed":true}`)
				sdk.records[pointer.Reference] = record
			case "binding":
				sdk.binding.Scope.Subject = "different"
			case "retired":
				sdk.retired = true
			case "no lease":
				lease = nil
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err := CheckFinalizationReceipt(ctx, plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, nil, lease); err == nil {
				t.Fatal("changed or unauthorized receipt accepted", scenario)
			} else if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

type recoveryBoundFile struct {
	*inputFileWitness
}

func (f *recoveryBoundFile) FileScope() storage.Scope { return f.inputFileWitness.FileScope() }
func (f *recoveryBoundFile) Commitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	return f.inputFileWitness.Commitment(ctx, ref, object, revision)
}

func (f *recoveryBoundFile) Upload(_ context.Context, _ string, target storage.PublicationTarget, _ storage.FileRequest, _ io.ReaderAt) (storage.Reference, error) {
	return storage.Reference{RoomID: target.RoomID, EventID: "$actual_file"}, nil
}

func (*recoveryBoundFile) Prepare(context.Context) error { return errors.New("unexpected Prepare") }
func (*recoveryBoundFile) Info(context.Context, storage.Reference, string, string) (storage.FileRequest, error) {
	return storage.FileRequest{}, errors.New("unexpected Info")
}

func (*recoveryBoundFile) Download(context.Context, storage.Reference, string, string, io.Writer) (storage.FileRequest, error) {
	return storage.FileRequest{}, errors.New("unexpected Download")
}

func (*recoveryBoundFile) Acknowledge(context.Context, string, storage.Reference) error {
	return errors.New("unexpected Acknowledge")
}

func TestFinalizationReceiptRecoveryChecksFileCommitment(t *testing.T) {
	plan, reader, file, _ := inputReadbackFixture(t)
	sdk := &batchPublisherWitness{contentPublisherWitness: &contentPublisherWitness{inputReaderWitness: reader, t: t, plan: plan, transactions: map[string]storage.Reference{}}}
	files := &recoveryBoundFile{inputFileWitness: file}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
	if _, err := PublishInputBatch(t.Context(), plan, sdk, files); err != nil {
		t.Fatal(err)
	}
	if err := CaptureFinalizationReceipt(t.Context(), plan, sdk, files, lease); err != nil {
		t.Fatal(err)
	}
	if err := CheckFinalizationReceipt(t.Context(), plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, files, lease); err != nil {
		t.Fatal("fresh file commitment rejected", err)
	}
	original := *file
	for _, scenario := range []string{"digest", "request", "scope"} {
		*file = original
		switch scenario {
		case "digest":
			file.sha = hashBytesForRecovery([]byte("changed"))
		case "request":
			file.request.ObjectID = "foreign"
		case "scope":
			file.scope.Subject = "foreign"
		}
		if err := CheckFinalizationReceipt(t.Context(), plan.data.journal, plan.data.root.Scope, plan.data.root.Operation, sdk, files, lease); err == nil {
			t.Fatal("changed file commitment accepted", scenario)
		}
	}
}
