// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// targetProbe exposes outbound attempts without granting the second device any
// authority to publish as the first. The base fixture is in-memory; a shared
// journal and two identities here do not prove that real devices share a WAL.
type targetProbe struct {
	*mutationHeadPort
	recordTargets []storage.PublicationTarget
	objectTargets []storage.PublicationTarget
}

func (p *targetProbe) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, txn string, record storage.Record) (storage.Reference, error) {
	p.recordTargets = append(p.recordTargets, target)
	return p.mutationHeadPort.PutStorageRecordAt(ctx, target, txn, record)
}

func (p *targetProbe) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	p.objectTargets = append(p.objectTargets, target)
	return p.mutationHeadPort.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
}

// TestRetainedMutationCannotAdoptDifferentTarget exercises an ordinary
// Mutate without Content or Acknowledge. A stops after persisting the mutation
// but before a publication plan. B is a different authenticated target with
// the same scope and, by construction, the same journal. The fake only
// authorizes B: it cannot verify or exercise a publication under original A.
// Therefore the safe outcome in this fixture is a specific conflict before
// outbound effects, retaining A's exact journal rather than adopting B.
func TestRetainedMutationCannotAdoptDifferentTarget(t *testing.T) {
	ctx := t.Context()
	server := &mutationHeadServer{objects: map[storage.Reference]storage.ObjectVersion{}, records: map[storage.Reference]storage.Record{}}
	journal := mutationHeadJournal(t)
	a := mutationHeadClient(t, server, journal, "DEVICE_A")
	b := mutationHeadClient(t, server, journal, "DEVICE_B")
	original := a.cfg.Crypto.(*mutationHeadPort)
	current := b.cfg.Crypto.(*mutationHeadPort)
	probe := &targetProbe{mutationHeadPort: current}
	b.cfg.Crypto = probe
	if a.cfg.Scope != b.cfg.Scope || original.binding.Target == current.binding.Target || original.binding.Target.RoomID != current.binding.Target.RoomID {
		t.Fatal("fixture did not retain scope and room while changing device target")
	}
	if err := probe.CheckStoragePublicationTarget(original.binding.Target); !errors.Is(err, ErrSource) {
		t.Fatal("second target unexpectedly authorized the original device", err)
	}
	if err := probe.CheckStoragePublicationTarget(current.binding.Target); err != nil {
		t.Fatal("second target is not authenticated by fixture", err)
	}

	const kind, id = "contact", "person"
	initial := []byte(`{"note":"initial"}`)
	pendingBody := []byte(`{"note":"pending under A"}`)
	if _, err := a.Mutate(ctx, "seed", id, func(context.Context) (string, string, string, []byte, error) {
		return kind, id, "live", bytes.Clone(initial), nil
	}); err != nil {
		t.Fatal("seed", err)
	}
	originalHead, err := a.ReadyVersion(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	input := struct{ ID, Note string }{id, "pending under A"}
	calls := 0
	prepare := func(context.Context) (string, string, string, []byte, error) {
		calls++
		return kind, id, "live", bytes.Clone(pendingBody), nil
	}
	stop := errors.New("synthetic interruption after mutation WAL write")
	valid := a.cfg.ValidateRow
	a.cfg.ValidateRow = func(context.Context, string, string, []byte) error { return stop }
	_, err = a.Mutate(ctx, "update-note", input, prepare)
	a.cfg.ValidateRow = valid
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatal("failed to retain original mutation at the intended cut", err, calls)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	key := "mutation_" + hashBytes([]byte("update-note\x00"+hashBytes(encoded)))
	var saved mutation
	if found, err := journal.Read(a.owner(), a.namespace(), key, &saved); err != nil || !found || !bytes.Equal(saved.JSON, pendingBody) {
		t.Fatal("original mutation WAL is missing or changed", err)
	}
	var plan publication
	if found, err := journal.Read(a.owner(), a.namespace(), key+"_publication", &plan); err != nil || found {
		t.Fatal("the cut did not occur before the publication plan", err)
	}
	// Prove B can read the same complete, single-head catalog before the
	// retry; an unrelated catalog failure must not count as target rejection.
	if err := b.refresh(ctx); err != nil {
		t.Fatal("second target cannot read the original catalog", err)
	}
	visible, err := b.ReadyVersion(kind, id)
	if err != nil || visible.Reference != originalHead.Reference {
		t.Fatal("second target does not see the original head", err)
	}
	beforeJournal, err := journal.List(a.owner(), a.namespace())
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(beforeJournal, func(x, y offline.Record) int { return strings.Compare(x.ID, y.ID) })
	beforeRecords := make(map[storage.Reference]storage.Record, len(server.records))
	for ref, record := range server.records {
		record.Payload = bytes.Clone(record.Payload)
		record.Previous = slices.Clone(record.Previous)
		beforeRecords[ref] = record
	}
	beforeObjects := make(map[storage.Reference]storage.ObjectVersion, len(server.objects))
	for ref, object := range server.objects {
		beforeObjects[ref] = cloneVersion(object)
	}

	_, retryErr := b.Mutate(ctx, "update-note", input, prepare)
	if !errors.Is(retryErr, ErrConflict) {
		t.Fatal("changed target did not fail with a retained-work conflict", retryErr)
	}
	if calls != 1 {
		t.Fatal("retry rebuilt the operation under the new device", calls)
	}
	if len(probe.recordTargets) != 0 || len(probe.objectTargets) != 0 {
		t.Fatal("changed target attempted an outbound publication", probe.recordTargets, probe.objectTargets)
	}
	if !reflect.DeepEqual(beforeRecords, server.records) || !reflect.DeepEqual(beforeObjects, server.objects) {
		t.Fatal("changed target altered remote contents or indices")
	}
	afterJournal, err := journal.List(a.owner(), a.namespace())
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(afterJournal, func(x, y offline.Record) int { return strings.Compare(x.ID, y.ID) })
	if !reflect.DeepEqual(beforeJournal, afterJournal) {
		t.Fatal("changed target altered the retained journal")
	}
	var after mutation
	if found, err := journal.Read(a.owner(), a.namespace(), key, &after); err != nil || !found || !reflect.DeepEqual(saved, after) {
		t.Fatal("changed target discarded or rewrote original mutation", err)
	}
	if found, err := journal.Read(a.owner(), a.namespace(), key+"_publication", &plan); err != nil || found {
		t.Fatal("changed target created a publication plan", err)
	}
	catalog, err := storage.LoadCatalog(ctx, probe, b.cfg.Scope)
	if err != nil || len(catalog.Heads) != 1 || catalog.Heads[0].Reference != originalHead.Reference {
		t.Fatal("changed target changed the original authenticated head", err)
	}
}
