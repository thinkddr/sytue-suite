//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Protocol ordering witness only, not a production SDK/provider. The native
// integration exercises the same stage using actual scoped SDK publication.
type contentPublisherWitness struct {
	*inputReaderWitness
	t             *testing.T
	plan          InputPlan
	transactions  map[string]storage.Reference
	prepare, puts int
	loseACK       bool
}

func (p *contentPublisherWitness) assertCaptured() {
	p.t.Helper()
	data := p.plan.data
	var root contentPublication
	found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputPublicationKey(data.root.Operation), &root)
	if err != nil || !found || root != expectedContentWork(data) {
		p.t.Fatal("SDK IO preceded durable content root", err)
	}
	for ordinal := uint64(0); ordinal < data.root.Count; ordinal++ {
		_, input, err := p.plan.readCapturedMember(p.t.Context(), ordinal)
		if err != nil {
			p.t.Fatal(err)
		}
		var member contentPublicationMember
		found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputContentReceiptKey(data.root.Operation, ordinal), &member)
		if err != nil || !found || !member.matches(data, ordinal, input) {
			p.t.Fatal("SDK IO preceded ALL exact member intents", ordinal, err)
		}
	}
}

func (p *contentPublisherWitness) PrepareStorage(ctx context.Context, scope storage.Scope) (storage.Binding, error) {
	p.prepare++
	p.assertCaptured()
	if scope != p.binding.Scope {
		return storage.Binding{}, storage.ErrRecord
	}
	return p.binding, ctx.Err()
}

func (p *contentPublisherWitness) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if p.retired || target != p.binding.Target {
		return storage.ErrRecord
	}
	return nil
}

func (p *contentPublisherWitness) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	p.puts++
	p.assertCaptured()
	if err := p.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	if err := record.Validate(p.binding.Scope); err != nil {
		return storage.Reference{}, err
	}
	ref, ok := p.transactions[transaction]
	if !ok {
		ref = storage.Reference{RoomID: target.RoomID, EventID: fmt.Sprintf("$actual_%d", len(p.transactions))}
		p.transactions[transaction] = ref
		record.Payload = bytes.Clone(record.Payload)
		record.Previous = slices.Clone(record.Previous)
		p.records[ref] = record
	} else if !record.MatchesExact(p.records[ref]) {
		return storage.Reference{}, storage.ErrRecord
	}
	if p.loseACK {
		p.loseACK = false
		return storage.Reference{}, errors.New("accepted test ACK lost")
	}
	return ref, ctx.Err()
}

func (p *contentPublisherWitness) PutStorageObjectAt(context.Context, storage.PublicationTarget, string, string, string, storage.ObjectCommit) (storage.ObjectVersion, error) {
	p.t.Fatal("content stage published application index")
	return storage.ObjectVersion{}, storage.ErrRecord
}

func (p *contentPublisherWitness) StorageObject(context.Context, storage.Reference, string) (storage.ObjectVersion, error) {
	p.t.Fatal("content stage invented ObjectVersion")
	return storage.ObjectVersion{}, storage.ErrRecord
}

func contentStageFixture(t *testing.T) (InputPlan, *contentPublisherWitness) {
	t.Helper()
	plan, reader, _, _ := inputReadbackFixture(t)
	// Use the existing captured three-input plan with file work already proven in
	// a checkpoint; no synthetic file uploader is introduced for this unit test.
	publisher := &contentPublisherWitness{inputReaderWitness: reader, t: t, plan: plan, transactions: map[string]storage.Reference{}}
	// A file port is deliberately absent. Record-only/deletion intents are captured
	// under a separate operation, preserving the exact source plan's durable bytes.
	values, _ := inputPlanValues()
	values = []Input{values[0], values[2]}
	var err error
	plan, err = CaptureInputs(t.Context(), plan.data.journal, plan.data.root.Scope, "content_stage", plan.data.root.Target, values)
	if err != nil {
		t.Fatal(err)
	}
	publisher.plan = plan
	publisher.records = map[storage.Reference]storage.Record{}
	return plan, publisher
}

func TestAtomicContentStageCapturesAllBeforeIOAndRecoversAcceptedACKWithoutNewIndex(t *testing.T) {
	plan, publisher := contentStageFixture(t)
	publisher.loseACK = true
	if _, err := PublishInputContents(t.Context(), plan, publisher, nil); err == nil {
		t.Fatal("lost accepted ACK became success")
	}
	if len(publisher.transactions) != 1 {
		t.Fatal("fixture did not accept exactly one content record")
	}
	if count, err := plan.data.journal.CountOwner(atomicOwner(plan.data.root.Scope), storage.PrivateJournalNamespaces(plan.data.root.Scope.Product)...); err != nil || count == 0 {
		t.Fatal("interrupted content publication lost pending work", count, err)
	}
	proofs, err := PublishInputContents(t.Context(), plan, publisher, nil)
	if err != nil || len(proofs) != 2 || len(publisher.transactions) != 1 || publisher.puts != 2 {
		t.Fatal("exact original transaction did not recover accepted content", err, publisher.puts)
	}
	for _, proof := range proofs {
		if _, err := proof.Mutation(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	beforePuts, beforePrepare := publisher.puts, publisher.prepare
	publisher.binding.Target.DeviceID = "CURRENT_B"
	if _, err := PublishInputContents(t.Context(), plan, publisher, nil); err != nil {
		t.Fatal("current reader could not verify already checkpointed content", err)
	}
	if publisher.puts != beforePuts || publisher.prepare != beforePrepare {
		t.Fatal("reader B prepared or resent original writer A")
	}
}

func TestAtomicContentStageForeignWriterCannotPrepareOrResendOriginalIntent(t *testing.T) {
	plan, publisher := contentStageFixture(t)
	publisher.binding.Target.DeviceID = "CURRENT_B"
	if _, err := PublishInputContents(t.Context(), plan, publisher, nil); err == nil {
		t.Fatal("current reader published original writer's intent")
	}
	if publisher.prepare != 0 || publisher.puts != 0 {
		t.Fatal("foreign target reached SDK preparation/publication")
	}
}
