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
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func inputPlanValues() ([]Input, [][]byte) {
	bodies := [][]byte{[]byte(`{"original":9007199254740993,"body":"private input plan JSON"}`), bytes.Repeat([]byte("private input plan binary "), 25000)}
	inputs := []Input{}
	for i, id := range []string{"json_object", "binary_object"} {
		raw := bodies[i]
		sum := sha256.Sum256(raw)
		kind := []string{"record", "file"}[i]
		inputs = append(inputs, Input{ObjectID: id, State: "live", Metadata: []byte(`{"unknown":9007199254740995}`), Content: &ContentInput{Kind: kind, Request: storage.FileRequest{ObjectID: id, RevisionID: "content_revision", SizeBytes: int64(len(raw)), Metadata: []byte(`{"unknown":9007199254740997}`)}, SHA256: hex.EncodeToString(sum[:]), Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }}})
	}
	inputs = append(inputs, Input{ObjectID: "deleted_object", State: "deleted", Metadata: []byte(`{"deleted":true}`)})
	return inputs, bodies
}

func TestAtomicInputsRootAndSourceIDsPrecedeIOAndResumeExactPayloadsAfterReopen(t *testing.T) {
	dir := t.TempDir()
	keys := new(authtest.Secrets)
	journal, err := offline.OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	inputs, bodies := inputPlanValues()
	originalOpen := inputs[1].Content.Open
	opened := 0
	for i := range 2 {
		ordinal := uint64(i)
		previous := inputs[i].Content.Open
		inputs[i].Content.Open = func(ctx context.Context) (io.ReadCloser, error) {
			root, err := readInputRoot(ctx, journal, scope, "explicit_operation")
			if err != nil || root.State != StateStaging || root.Count != 3 {
				t.Fatal("source IO preceded durable root/count/fingerprint", err)
			}
			member, err := readInputMember(ctx, journal, root, ordinal)
			if err != nil || member.Source == nil {
				t.Fatal("source IO preceded reserved source ID/exact intent", err)
			}
			opened++
			if ordinal == 1 {
				return nil, io.ErrClosedPipe
			}
			return previous(ctx)
		}
	}
	if plan, err := CaptureInputs(t.Context(), journal, scope, "explicit_operation", target, inputs); !errors.Is(err, io.ErrClosedPipe) || plan.data != nil {
		t.Fatal("partial input capture returned publishable handle", err)
	}
	if opened != 2 {
		t.Fatal("source fixture did not reach interrupted second member", opened)
	}
	root, err := readInputRoot(t.Context(), journal, scope, "explicit_operation")
	if err != nil || root.State != StateStaging {
		t.Fatal(err)
	}
	changed := append([]Input(nil), inputs...)
	changed[2].Metadata = []byte(`{"changed_unstaged_member":true}`)
	beforeOpen := opened
	if _, err := CaptureInputs(t.Context(), journal, scope, root.Operation, target, changed); err == nil || opened != beforeOpen {
		t.Fatal("changed unstaged input reached original source before fingerprint guard", err)
	}
	if count, err := journal.CountOwner(atomicOwner(scope), storage.PrivateJournalNamespaces(scope.Product)...); err != nil || count == 0 {
		t.Fatal("interrupted source capture reported no pending work", count, err)
	}
	if _, err := LoadInputs(t.Context(), journal, scope, root.Operation); err == nil {
		t.Fatal("partial inputs became loaded publishable plan")
	}
	before, err := readInputMember(t.Context(), journal, root, 1)
	if err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(dir, keys)
	if err != nil {
		t.Fatal(err)
	}
	inputs[0].Content.Open = nil
	inputs[1].Content.Open = nil
	if _, err := CaptureInputs(t.Context(), journal, scope, root.Operation, target, inputs); err == nil {
		t.Fatal("missing original source invented replayable bytes")
	}
	inputs[1].Content.Open = originalOpen
	plan, err := CaptureInputs(t.Context(), journal, scope, root.Operation, target, inputs)
	if err != nil || plan.Count() != 3 {
		t.Fatal("same exact input could not resume", err)
	}
	after, err := readInputMember(t.Context(), journal, plan.data.root, 1)
	if err != nil || *before.Source != *after.Source || root.ID != plan.data.root.ID {
		t.Fatal("restart replaced reserved input/source identity", err)
	}
	loaded, err := LoadInputs(t.Context(), journal, scope, root.Operation)
	if err != nil {
		t.Fatal(err)
	}
	for i := range bodies {
		reader, err := loaded.OpenContent(t.Context(), uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(io.NewSectionReader(reader, 0, int64(len(bodies[i]))))
		_ = reader.Close()
		if err != nil || !bytes.Equal(got, bodies[i]) {
			t.Fatal("captured source bytes changed after sealed-store restart", i, err)
		}
		clear(got)
	}
}

func TestAtomicInputsRejectRetargetChangedIntentLostSourceAndJSONHandle(t *testing.T) {
	for _, fault := range []string{"target", "metadata", "parents", "payload-hash", "request-metadata", "lost-member", "lost-source", "forged-json"} {
		t.Run(fault, func(t *testing.T) {
			journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
			if err != nil {
				t.Fatal(err)
			}
			scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
			target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
			inputs, _ := inputPlanValues()
			plan, err := CaptureInputs(t.Context(), journal, scope, "explicit_operation", target, inputs)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "target":
				target.DeviceID = "CURRENT_B"
			case "metadata":
				inputs[0].Metadata = []byte(`{"changed":true}`)
			case "parents":
				inputs[0].Parents = []storage.Reference{{RoomID: "!private:test", EventID: "$newparent"}}
			case "payload-hash":
				inputs[0].Content.SHA256 = hex.EncodeToString(bytes.Repeat([]byte{2}, 32))
			case "request-metadata":
				inputs[0].Content.Request.Metadata = []byte(`{"changed":true}`)
			case "lost-member":
				if err := journal.Delete(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputMemberKey("explicit_operation", 1)); err != nil {
					t.Fatal(err)
				}
			case "lost-source":
				member, err := readInputMember(t.Context(), journal, plan.data.root, 1)
				if err != nil {
					t.Fatal(err)
				}
				if err := journal.DeleteStagedFile(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), *member.Source); err != nil {
					t.Fatal(err)
				}
			case "forged-json":
				raw, _ := json.Marshal(plan) //nolint:staticcheck // SA9005: intentional test that unexported plan fields do not serialize.
				plan = InputPlan{}
				if err := json.Unmarshal(raw, &plan); err != nil { //nolint:staticcheck // SA9005: intentional test that unexported plan fields do not serialize.
					t.Fatal(err)
				}
				if _, err := plan.OpenContent(t.Context(), 0); err == nil {
					t.Fatal("JSON minted an input plan")
				}
				return
			}
			opened := 0
			for i := range 2 {
				inputs[i].Content.Open = func(context.Context) (io.ReadCloser, error) { opened++; return nil, io.ErrClosedPipe }
			}
			if _, err := CaptureInputs(t.Context(), journal, scope, "explicit_operation", target, inputs); err == nil || opened != 0 {
				t.Fatal("captured inputs repaired/rebound or reopened missing old source", err, opened)
			}
			if count, err := journal.CountOwner(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product)); err != nil || count == 0 {
				t.Fatal("invalid captured plan discarded pending work", count, err)
			}
		})
	}
}

func TestAtomicInputsNewExplicitIntentKeepsOldRetryAndBranchesIndependent(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	inputs, _ := inputPlanValues()
	first, err := CaptureInputs(t.Context(), journal, scope, "intent_one", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	inputs[0].Metadata = []byte(`{"deliberate_revert":9007199254740993}`)
	second, err := CaptureInputs(t.Context(), journal, scope, "intent_two", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	old, err := LoadInputs(t.Context(), journal, scope, "intent_one")
	if err != nil || old.data.root != first.data.root || first.data.root.ID == second.data.root.ID {
		t.Fatal("new intent replaced retained old retry", err)
	}
	oldJob, _ := old.ContentJob(0)
	newJob, _ := second.ContentJob(0)
	if oldJob == newJob {
		t.Fatal("distinct operations reused physical content jobs")
	}
}

func TestAtomicInputsOpaqueHandleRejectsValidButChangedMemberBeforeRead(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	target := storage.PublicationTarget{RoomID: "!private:test", DeviceID: "ORIGINAL_A"}
	inputs, _ := inputPlanValues()
	plan, err := CaptureInputs(t.Context(), journal, scope, "operation", target, inputs)
	if err != nil {
		t.Fatal(err)
	}
	member, err := readInputMember(t.Context(), journal, plan.data.root, 0)
	if err != nil {
		t.Fatal(err)
	}
	member.Spec.Metadata = []byte(`{"changed":true}`)
	if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputMemberKey("operation", 0), member); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.OpenContent(t.Context(), 0); err == nil {
		t.Fatal("captured handle accepted a changed valid member")
	}
	if _, err := LoadInputs(t.Context(), journal, scope, "operation"); err == nil {
		t.Fatal("changed member stream retained original root fingerprint")
	}
}
