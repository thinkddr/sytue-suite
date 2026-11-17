// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMutationContentLeavesLegacyEncodingUnchanged(t *testing.T) {
	work := mutation{Version: 1, Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}, Operation: "favorite", Input: "input", Entity: "asset", ID: "asset", State: "live", JSON: []byte(`{}`)}
	raw, err := json.Marshal(work)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"v":1,"scope":{"tenant":"acme","subject":"alice","product":"photos"},"operation":"favorite","input":"input","entity":"asset","id":"asset","state":"live","json":"e30="}`
	if string(raw) != expected {
		t.Fatal("legacy mutation encoding changed", string(raw))
	}
}

func TestMutationContentKeepsCapturedIntentWhenPreparationFailsOrTargetChanges(t *testing.T) {
	for _, change := range []string{"missing links", "changed target"} {
		t.Run(change, func(t *testing.T) {
			r, _, _ := retainedReaderFixture(t)
			target := r.binding.Target
			initial := []byte(`{"id":"edit","recipe":{"version":1,"pasos":[]},"unknown":900719925474099312345}`)
			capture := []byte(`{"origin":"original immutable revision","base":"retained base"}`)
			work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "recipe", Input: "input", Entity: "edit", ID: "edit", State: "live", JSON: bytes.Clone(initial), Content: true, Target: &target, ContentInput: bytes.Clone(capture)}
			key := "mutation_probe"
			if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
				t.Fatal(err)
			}
			calls := 0
			content := MutationContent{Prepare: func(_ context.Context, raw, captured []byte, original storage.PublicationTarget) ([]byte, error) {
				calls++
				if !bytes.Equal(raw, initial) || !bytes.Equal(captured, capture) || original != target {
					t.Fatal("preparation substituted the durable intent")
				}
				return bytes.Clone(raw), nil
			}, Validate: func(context.Context, []byte, []byte, storage.PublicationTarget) error { return ErrSource }}
			if change == "changed target" {
				r.binding.Target.DeviceID = "OTHER"
			}
			err := r.prepareMutationContent(t.Context(), mutationCall{operation: "recipe", input: "input", key: key, content: content}, &work)
			if !errors.Is(err, ErrSource) || work.Prepared {
				t.Fatal("incomplete preparation advanced publication", err)
			}
			if change == "changed target" && calls != 0 {
				t.Fatal("changed target reached preparation")
			}
			var saved mutation
			found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &saved)
			if err != nil || !found || saved.Prepared || !bytes.Equal(saved.JSON, initial) || !bytes.Equal(saved.ContentInput, capture) {
				t.Fatal("failed preparation lost its recoverable intent", err)
			}
		})
	}
}

func TestContentRetryReusesAcceptedReferenceAfterPublicationWALCleanup(t *testing.T) {
	r, probe, _ := retainedReaderFixture(t)
	raw := []byte(`{"id":"edit","owner_sub":"alice","recipe":{"version":1,"pasos":[]}}`)
	object := r.object("edit", "edit")
	payload, err := json.Marshal(content{Kind: r.cfg.EntityKind, JSON: raw})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(entity{Kind: r.cfg.EntityKind, Entity: "edit", ID: "edit", SHA256: hashBytes(raw)})
	if err != nil {
		t.Fatal(err)
	}
	reference := storage.Reference{RoomID: r.binding.Target.RoomID, EventID: "$accepted"}
	pointer := storage.ContentPointer{Kind: "record", RevisionID: "contentRevision", Reference: storage.Reference{RoomID: reference.RoomID, EventID: "$content"}}
	version := storage.ObjectVersion{Reference: reference, ObjectID: object, RevisionID: "indexRevision", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &pointer}}
	r.heads = map[string][]storage.ObjectVersion{object: {version}}
	probe.link = pointer.Reference
	probe.record = storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: pointer.RevisionID, Role: "content", Payload: payload}
	target := r.binding.Target
	work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "recipe", Input: "input", Entity: "edit", ID: "edit", State: "live", JSON: raw, Content: true, Prepared: true, Target: &target, ContentInput: []byte(`{"origin":"retained"}`), Accepted: &reference}
	reused, err := r.reuseAcceptedMutation(t.Context(), mutationCall{key: "mutation_after_partial_cleanup"}, work)
	if err != nil || !reused {
		t.Fatal("accepted index was not recovered without publication WAL", err)
	}
	work.JSON = []byte(`{"id":"edit","owner_sub":"alice","different":true}`)
	if reused, err := r.reuseAcceptedMutation(t.Context(), mutationCall{key: "mutation_after_partial_cleanup"}, work); err == nil || reused {
		t.Fatal("different intent reused an accepted revision")
	}
}
