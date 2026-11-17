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

func acceptedLocalAcknowledgmentFixture(t *testing.T) (*Repository, mutationCall, mutation) {
	t.Helper()
	r, probe, _ := retainedReaderFixture(t)
	raw := []byte(`{"id":"edit","owner_sub":"alice","unknown":900719925474099312345}`)
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
	version := storage.ObjectVersion{Reference: reference, ObjectID: object, RevisionID: "revision_index", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &pointer}}
	r.heads = map[string][]storage.ObjectVersion{object: {version}}
	indexPayload, err := json.Marshal(version.Commit)
	if err != nil {
		t.Fatal(err)
	}
	r.entries = map[storage.Reference]storage.RecordEntry{reference: {Reference: reference, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: version.RevisionID, Role: "index", Payload: indexPayload}}}
	r.catalog = storage.AtomicCatalogSnapshot{Binding: r.binding, Entries: r.entries, History: []storage.ObjectVersion{version}}
	probe.link = pointer.Reference
	probe.record = storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: pointer.RevisionID, Role: "content", Payload: payload}
	target := r.binding.Target
	work := mutation{Version: 1, Scope: r.cfg.Scope, Operation: "edit", Input: "fingerprint", Entity: "edit", ID: "edit", State: "live", JSON: raw, Acknowledge: true, Target: &target}
	call := mutationCall{operation: work.Operation, input: work.Input, key: "mutation_local_ack"}
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), call.key, work); err != nil {
		t.Fatal(err)
	}
	publication := publication{Version: 1, Scope: r.cfg.Scope, Kind: work.Entity, ID: work.ID, State: work.State, Input: hashBytes(raw), Revision: "revision", Target: target, Index: &reference}
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), call.key+"_publication", publication); err != nil {
		t.Fatal(err)
	}
	return r, call, work
}

func TestLocalAcknowledgmentFailureRetainsExactAcceptedMutation(t *testing.T) {
	r, call, work := acceptedLocalAcknowledgmentFixture(t)
	failure := errors.New("local source acknowledgment unavailable")
	calls := 0
	call.acknowledge = func(_ context.Context, raw []byte) error {
		calls++
		if !bytes.Equal(raw, work.JSON) {
			t.Fatal("callback did not receive the exact accepted result")
		}
		var accepted mutation
		if found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), call.key, &accepted); err != nil || !found || accepted.Accepted == nil {
			t.Fatal("local acknowledgment ran before the accepted receipt was durable", err)
		}
		clear(raw)
		return failure
	}
	if err := r.acknowledgeMutation(t.Context(), call, work); !errors.Is(err, failure) || calls != 1 {
		t.Fatal("failed local acknowledgment was hidden", err, calls)
	}
	var saved mutation
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), call.key, &saved)
	if err != nil || !found || !bytes.Equal(saved.JSON, work.JSON) || saved.Accepted == nil || saved.Accepted.EventID != "$accepted" || !saved.Acknowledge {
		t.Fatal("local failure lost its exact accepted receipt", err)
	}
	call.prepare = func(context.Context) (string, string, string, []byte, error) {
		t.Fatal("accepted retry prepared another CAS edit")
		return "", "", "", nil, ErrSource
	}
	reopened, err := r.loadMutation(t.Context(), call)
	if err != nil || !bytes.Equal(reopened.JSON, work.JSON) {
		t.Fatal("retry replaced the retained result", err)
	}
	// A recovered device only reads the authenticated accepted revision. It must
	// never try to publish a new transaction using the previous device's target.
	r.binding.Target.DeviceID = "RECOVERED"
	r.catalog.Binding.Target.DeviceID = "RECOVERED"
	if err := r.cfg.Journal.Delete(r.owner(), r.namespace(), call.key+"_publication"); err != nil {
		t.Fatal(err)
	}
	if reused, err := r.reuseAcceptedMutation(t.Context(), call, reopened); err != nil || !reused {
		t.Fatal("accepted receipt could not be read after device recovery", err)
	}
	call.acknowledge = func(_ context.Context, raw []byte) error {
		calls++
		if !bytes.Equal(raw, work.JSON) {
			t.Fatal("retry changed accepted JSON")
		}
		return nil
	}
	if err := r.acknowledgeMutation(t.Context(), call, reopened); err != nil || calls != 2 {
		t.Fatal("local acknowledgment could not be retried", err, calls)
	}
}

func TestLocalAcknowledgmentRequiresAuthenticatedExactPublication(t *testing.T) {
	for _, change := range []string{"content", "reference", "target", "unpublished", "unretained", "foreign scope", "foreign object", "cancel"} {
		t.Run(change, func(t *testing.T) {
			r, call, work := acceptedLocalAcknowledgmentFixture(t)
			calls := 0
			call.acknowledge = func(context.Context, []byte) error { calls++; return nil }
			ctx := t.Context()
			switch change {
			case "content":
				work.Accepted = &storage.Reference{RoomID: work.Target.RoomID, EventID: "$accepted"}
				work.JSON = []byte(`{"id":"edit","different":true}`)
			case "reference":
				work.Accepted = &storage.Reference{RoomID: work.Target.RoomID, EventID: "$other"}
			case "target":
				target := *work.Target
				target.RoomID = "!foreign:test"
				work.Target = &target
			case "unpublished":
				if err := r.cfg.Journal.Delete(r.owner(), r.namespace(), call.key+"_publication"); err != nil {
					t.Fatal(err)
				}
			case "unretained":
				clear(r.entries)
			case "foreign scope":
				for ref, entry := range r.entries {
					entry.Record.Scope.Subject = "other"
					r.entries[ref] = entry
				}
			case "foreign object":
				for ref, entry := range r.entries {
					entry.Record.ObjectID = "other"
					r.entries[ref] = entry
				}
			case "cancel":
				var stop context.CancelFunc
				ctx, stop = context.WithCancel(ctx)
				stop()
			}
			if err := r.acknowledgeMutation(ctx, call, work); err == nil || calls != 0 {
				t.Fatal("unverified publication acknowledged the local source", change, err, calls)
			}
		})
	}
}

func TestLocalAcknowledgmentAcceptsRetainedReceiptAfterNewerRemoteEdit(t *testing.T) {
	r, call, work := acceptedLocalAcknowledgmentFixture(t)
	object := r.object(work.Entity, work.ID)
	// The complete catalog still authenticates the original accepted index;
	// only its selected head has changed. No current-head data may be returned
	// as though it were the result of this older, exact local request.
	newer := cloneVersion(r.heads[object][0])
	newer.Reference.EventID = "$newer"
	newer.RevisionID = "newer_index"
	r.heads[object] = []storage.ObjectVersion{newer}
	call.acknowledge = func(_ context.Context, raw []byte) error {
		if !bytes.Equal(raw, work.JSON) {
			t.Fatal("superseding remote content substituted the accepted request")
		}
		return nil
	}
	if err := r.acknowledgeMutation(t.Context(), call, work); err != nil {
		t.Fatal("retained accepted write could not acknowledge its local source", err)
	}
}

func TestLocalAcknowledgmentWireRejectsAmbiguousIntent(t *testing.T) {
	_, _, work := acceptedLocalAcknowledgmentFixture(t)
	if err := validateMutationContentIntent(work); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"content", "target", "capture", "ordinary"} {
		candidate := work
		switch change {
		case "content":
			candidate.Content = true
		case "target":
			candidate.Target = nil
		case "capture":
			candidate.ContentInput = []byte(`{}`)
		case "ordinary":
			candidate.Acknowledge = false
		}
		if err := validateMutationContentIntent(candidate); err == nil {
			t.Fatal("ambiguous local receipt accepted", change)
		}
	}
}

func TestResumeAcknowledgedMissingIntentNeverPreparesOrAcknowledges(t *testing.T) {
	r, _, _ := acceptedLocalAcknowledgmentFixture(t)
	calls := 0
	ack := func(context.Context, []byte) error { calls++; return nil }
	result, found, err := r.ResumeAcknowledged(t.Context(), "missing", map[string]string{"original": "request"}, ack)
	if err != nil || found || len(result) != 0 || calls != 0 {
		t.Fatal("missing resume applied or acknowledged a new request", err, found, calls)
	}
	r.ready = false
	if _, found, err := r.ResumeAcknowledged(t.Context(), "missing", "input", ack); !errors.Is(err, ErrNotReady) || found {
		t.Fatal("missing resume skipped readiness", err, found)
	}
	r.ready = true
	if _, err := r.loadMutation(t.Context(), mutationCall{key: "missing_without_prepare"}); !errors.Is(err, ErrSource) {
		t.Fatal("resume without a retained request attempted preparation", err)
	}
	if _, _, err := r.ResumeAcknowledged(t.Context(), "missing", "input", nil); !errors.Is(err, ErrSource) {
		t.Fatal("resume accepted a missing callback", err)
	}
}
