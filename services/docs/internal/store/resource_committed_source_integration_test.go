//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type officeCommittedGate struct {
	*officeResourceGate
	current resource.VerifiedSession
}

func (g *officeCommittedGate) Access(ctx context.Context, key resource.Resource, action resource.Action, raw json.RawMessage) (resource.VerifiedAccess, error) {
	if action != resource.Read {
		return g.officeResourceGate.Access(ctx, key, action, raw)
	}
	var policy resource.SignedEpoch
	if json.Unmarshal(raw, &policy) != nil {
		return resource.VerifiedAccess{}, resource.ErrInvalid
	}
	hash, err := resource.EpochDigest(policy.Epoch)
	if err != nil || hash != g.f.Epoch.SHA256() || policy.Resource != key {
		return resource.VerifiedAccess{}, resource.ErrRevoked
	}
	return resource.AuthorizeSession(ctx, g.f.Epoch, g.f.Cap, g.current, resource.Read)
}

type officeCommittedFence struct{ current resource.VerifiedSession }

func (f officeCommittedFence) Revalidate(ctx context.Context, access resource.VerifiedAccess) error {
	actual, ok := access.CurrentSessionBinding()
	if !ok || actual != f.current.Binding() {
		return resource.ErrRevoked
	}
	return f.current.Revalidate(ctx)
}

// SQL/RLS/receipt/CAS use production code and real migrations. SDK signatures
// use the production verifiers. The existing relay fixture's outer ciphertext
// is opaque; Native SDK/PQ stream interoperability is a separate proof.
func TestOfficeCommittedSourceRequiresActualReceiptAndCurrentReadSession(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	f := resourcetest.New(t, true)
	ctx := tenantCtx(f.Scope.Tenant)
	b := matrixprooftest.NewDevice(t, f.Chain, "COMMITTED_READER_B")
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, registry := resourcetest.EnrolledSession(t, b, key)
	gate := &officeCommittedGate{officeResourceGate: &officeResourceGate{f: f}, current: current}
	relay, err := store.NewResourceRelay(st, gate, f.Scope.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if err = relay.Bootstrap(ctx, f.SignedEpoch); err != nil {
		t.Fatal(err)
	}
	candidate := officeValidate(t, f, f.Candidate)
	staged, err := relay.Stage(ctx, candidate)
	if err != nil || staged.State != resource.Prepared {
		t.Fatal("stage", err)
	}
	source, reader, err := relay.CommittedSource(ctx, candidate.Proof.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Actor() != current.Actor() || reader.Capability().Enrollment().Issuer != f.Actor {
		t.Fatal("current B replaced by historical A")
	}
	if _, err = source.CommittedPublication(ctx, candidate.Proof.Resource, candidate.Proof.OpID); !errors.Is(err, resource.ErrPending) {
		t.Fatal("prepared operation gained a committed read", err)
	}
	sending, err := relay.AcquireSending(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.CommittedPublication(ctx, candidate.Proof.Resource, candidate.Proof.OpID); !errors.Is(err, resource.ErrPending) {
		t.Fatal("sending hold gained a committed read", err)
	}
	reference := storage.Reference{RoomID: candidate.Proof.RoomID, EventID: "$original-committed"}
	gate.accepted = &event.Event{ID: id.EventID(reference.EventID), RoomID: id.RoomID(reference.RoomID), Sender: id.UserID(candidate.Proof.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: candidate.EncryptedEvent}}
	receipt, err := relay.Commit(ctx, sending, reference)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := source.CommittedPublication(ctx, candidate.Proof.Resource, candidate.Proof.OpID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != resource.Committed || stored.Receipt != receipt || stored.Base != receipt.Base || stored.End != receipt.Head.Seq || len(stored.Descriptor) == 0 {
		t.Fatal("SQL columns lost original commit bindings")
	}
	f.Chain.Origin.DeviceActive = false
	gate.producerBlocked = true
	gets := gate.gets
	proof, err := resource.VerifyCommittedRead(ctx, candidate.Proof.Resource, candidate.Proof.OpID, reader, officeCommittedFence{current}, source)
	if err != nil {
		t.Fatal("historical read needed live A", err)
	}
	if proof.Receipt() != receipt || proof.Value().Actor != f.Actor || gate.gets != gets {
		t.Fatal("read reacquired producer or acceptance authority")
	}
	page, err := relay.Page(ctx, candidate.Proof.Resource, resource.Cursor{Position: receipt.Base}, 2)
	if err != nil || len(page.Records) != 1 || page.Records[0] != receipt {
		t.Fatal("read changed durable receipt chain", err)
	}
	changed := receipt
	changed.Reference.EventID = "$another"
	if err = source.CheckCommittedPublication(ctx, candidate.Proof.Resource, candidate.Proof.OpID, stored.DescriptorSHA256, changed); !errors.Is(err, resource.ErrFork) {
		t.Fatal("changed reference accepted", err)
	}
	other := candidate.Proof.Resource
	other.Product = "rooms"
	if _, err = source.CommittedPublication(ctx, other, candidate.Proof.OpID); err == nil {
		t.Fatal("source crossed products")
	}
	if _, err = source.CommittedPublication(tenantCtx("other-tenant"), candidate.Proof.Resource, candidate.Proof.OpID); err == nil {
		t.Fatal("source crossed original RLS tenant")
	}
	registry.Retire()
	if _, err = source.CommittedPublication(ctx, candidate.Proof.Resource, candidate.Proof.OpID); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("retired B retained SQL read", err)
	}
	if err = proof.RevalidateReader(ctx, reader, officeCommittedFence{current}); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("retired B retained cached proof", err)
	}
}
