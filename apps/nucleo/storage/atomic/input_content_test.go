//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Public metadata witnesses, not SDK provenance or crypto providers. Actual
// SDK readback is exercised separately by the native integration fixture.
type inputReaderWitness struct {
	binding   storage.Binding
	records   map[storage.Reference]storage.Record
	reads     int
	retired   bool
	afterRead func()
}

func (r *inputReaderWitness) StorageBindings(ctx context.Context) (storage.Binding, error) {
	if r.retired {
		return storage.Binding{}, errors.New("retired witness")
	}
	return r.binding, ctx.Err()
}

func (r *inputReaderWitness) StorageObjectHistory(context.Context, string, string, int) (storage.ObjectPage, error) {
	return storage.ObjectPage{}, errors.New("content proof cannot use invented catalog heads")
}

func (r *inputReaderWitness) ReadStorageRecord(ctx context.Context, ref storage.Reference, _, _ string) (storage.Record, error) {
	r.reads++
	if r.afterRead != nil {
		r.afterRead()
	}
	record, ok := r.records[ref]
	if !ok {
		return storage.Record{}, storage.ErrRecord
	}
	return record, ctx.Err()
}

type inputFileWitness struct {
	scope   storage.Scope
	request storage.FileRequest
	sha     string
}

func (f *inputFileWitness) FileScope() storage.Scope { return f.scope }
func (f *inputFileWitness) Commitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
	return f.request, f.sha, nil
}

func inputReadbackFixture(t *testing.T) (InputPlan, *inputReaderWitness, *inputFileWitness, []*storage.ContentPointer) {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	inputs, bodies := inputPlanValues()
	plan, err := CaptureInputs(t.Context(), journal, scope, "readback_operation", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	reader := &inputReaderWitness{binding: storage.Binding{Scope: scope, Target: target, Rooms: []string{target.RoomID}}, records: map[storage.Reference]storage.Record{}}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$actual_content"}
	reader.records[ref] = storage.Record{Version: 1, Scope: scope, ObjectID: inputs[0].ObjectID, RevisionID: inputs[0].Content.Request.RevisionID, Role: "content", Payload: bodies[0]}
	file := &inputFileWitness{scope: scope, request: inputs[1].Content.Request, sha: inputs[1].Content.SHA256}
	return plan, reader, file, []*storage.ContentPointer{{Kind: "record", RevisionID: inputs[0].Content.Request.RevisionID, Reference: ref}, {Kind: "file", RevisionID: file.request.RevisionID, Reference: storage.Reference{RoomID: target.RoomID, EventID: "$actual_file"}}, nil}
}

func TestAtomicInputContentCapturesOnlyExactSDKReadbackInOriginalOrder(t *testing.T) {
	plan, reader, files, pointers := inputReadbackFixture(t)
	proofs := make([]VerifiedInputContent, 0, len(pointers))
	for ordinal, pointer := range pointers {
		proof, err := VerifyInputContent(t.Context(), plan, uint64(ordinal), reader, files, pointer)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, proof)
	}
	swapped := []VerifiedInputContent{proofs[1], proofs[0], proofs[2]}
	if _, err := CaptureVerified(t.Context(), plan, swapped); err == nil {
		t.Fatal("verified proofs could be reordered")
	}
	work, err := CaptureVerified(t.Context(), plan, proofs)
	if err != nil || work.Count != 3 {
		t.Fatal("verified content could not become captured index data", err)
	}
	for ordinal, proof := range proofs {
		actual, err := ReadMember(t.Context(), plan.data.journal, work, uint64(ordinal))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := proof.Mutation(t.Context())
		if err != nil || !actual.matches(work, uint64(ordinal), expected) {
			t.Fatal("captured index changed original pointer/content digest", err)
		}
	}
	if _, err := CaptureVerified(t.Context(), plan, proofs[:2]); err == nil {
		t.Fatal("partial verified subset became complete batch")
	}
	inputs, _ := inputPlanValues()
	inputs[0].Metadata = []byte(`{"new_intent":true}`)
	other, err := CaptureInputs(t.Context(), plan.data.journal, plan.data.root.Scope, "different_intent", plan.data.root.Target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureVerified(t.Context(), other, proofs); err == nil {
		t.Fatal("foreign plan consumed old index metadata proof")
	}
	raw, _ := json.Marshal(proofs[0]) //nolint:staticcheck // SA9005: intentional test that unexported proof fields do not serialize.
	var forged VerifiedInputContent
	if err := json.Unmarshal(raw, &forged); err != nil { //nolint:staticcheck // SA9005: intentional test that unexported proof fields do not serialize.
		t.Fatal(err)
	}
	if _, err := forged.Mutation(t.Context()); err == nil {
		t.Fatal("JSON constructed accepted content proof")
	}
}

func TestAtomicInputContentRejectsChangedRecordFileBindingAndRetiredProof(t *testing.T) {
	for _, fault := range []string{"record-bytes", "record-scope", "record-object", "record-revision", "record-role", "record-parents", "file-request", "file-digest", "file-scope", "binding", "binding-after-read", "retired", "cancel-after-read", "member-changed", "foreign-plan"} {
		t.Run(fault, func(t *testing.T) {
			plan, reader, files, pointers := inputReadbackFixture(t)
			ordinal := uint64(0)
			if fault == "file-request" || fault == "file-digest" || fault == "file-scope" {
				ordinal = 1
			}
			proof, err := VerifyInputContent(t.Context(), plan, ordinal, reader, files, pointers[ordinal])
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			record := reader.records[pointers[0].Reference]
			switch fault {
			case "record-bytes":
				record.Payload = bytes.ReplaceAll(record.Payload, []byte("9007199254740993"), []byte("9007199254740995"))
			case "record-scope":
				record.Scope.Subject = "foreign"
			case "record-object":
				record.ObjectID = "foreign_object"
			case "record-revision":
				record.RevisionID = "foreign_revision"
			case "record-role":
				record.Role = "index"
			case "record-parents":
				record.Previous = []string{"$foreign_parent"}
			case "file-request":
				files.request.Metadata = []byte(`{"changed":true}`)
			case "file-digest":
				files.sha = strings.Repeat("f", 64)
			case "file-scope":
				files.scope.Subject = "foreign"
			case "binding":
				reader.binding.Target.DeviceID = "CURRENT_B"
			case "binding-after-read":
				reader.afterRead = func() { reader.binding.Target.DeviceID = "CURRENT_B" }
			case "retired":
				reader.retired = true
			case "cancel-after-read":
				reader.afterRead = cancel
			case "member-changed":
				member, e := readInputMember(t.Context(), plan.data.journal, plan.data.root, 0)
				if e != nil {
					t.Fatal(e)
				}
				member.Spec.Metadata = []byte(`{"changed":true}`)
				if e := plan.data.journal.Write(atomicOwner(plan.data.root.Scope), storage.AtomicJournalNamespace(plan.data.root.Scope.Product), inputMemberKey(plan.data.root.Operation, 0), member); e != nil {
					t.Fatal(e)
				}
			case "foreign-plan":
				cloned := *plan.data
				cloned.root.ID = "foreign_plan"
				other := InputPlan{&cloned}
				if _, err := CaptureVerified(t.Context(), other, []VerifiedInputContent{proof}); err == nil {
					t.Fatal("foreign plan accepted content proof")
				}
				return
			}
			reader.records[pointers[0].Reference] = record
			if _, err := proof.Mutation(ctx); err == nil {
				t.Fatal("changed/retired SDK readback became captured data", fault)
			}
		})
	}
}

func TestAtomicInputContentPointerCannotSelectForeignScopeOrRoomBeforeSDKRead(t *testing.T) {
	plan, reader, files, pointers := inputReadbackFixture(t)
	for _, fault := range []string{"scope", "room", "kind", "revision", "missing"} {
		r := *reader
		pointer := *pointers[0]
		selected := &pointer
		switch fault {
		case "scope":
			r.binding.Scope.Subject = "foreign"
		case "room":
			pointer.Reference.RoomID = "!foreign:test"
		case "kind":
			pointer.Kind = "file"
		case "revision":
			pointer.RevisionID = "foreign"
		case "missing":
			selected = nil
		}
		if _, err := VerifyInputContent(t.Context(), plan, 0, &r, files, selected); err == nil || r.reads != 0 {
			t.Fatal("invalid content pointer reached actual SDK readback", fault, err, r.reads)
		}
	}
	var empty InputPlan
	if _, err := VerifyInputContent(t.Context(), empty, 0, reader, files, pointers[0]); err == nil {
		t.Fatal("zero plan accepted a content pointer")
	}
}
