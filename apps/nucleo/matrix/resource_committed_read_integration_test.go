//go:build integration

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This witness covers captured public SDK metadata over the event the existing
// test actually published to Synapse. It does not replace the separate original
// SQL/RLS committed-source test or assert that the complete AS gateway is live.
type nativeCommittedWitness struct {
	value    resource.CommittedPublication
	evidence map[string]json.RawMessage
}

func (s *nativeCommittedWitness) CommittedPublication(ctx context.Context, key resource.Resource, op string) (resource.CommittedPublication, error) {
	if s.value.Resource != key || s.value.OpID != op {
		return resource.CommittedPublication{}, resource.ErrInvalid
	}
	return s.value, ctx.Err()
}

func (s *nativeCommittedWitness) CheckCommittedPublication(ctx context.Context, key resource.Resource, op, hash string, receipt resource.Receipt) error {
	if s.value.Resource != key || s.value.OpID != op || s.value.DescriptorSHA256 != hash || s.value.Receipt != receipt {
		return resource.ErrFork
	}
	return ctx.Err()
}

func (s *nativeCommittedWitness) PublicEvidence(ctx context.Context, key resource.Resource, hash string) (json.RawMessage, error) {
	if key != s.value.Resource {
		return nil, resource.ErrInvalid
	}
	raw, ok := s.evidence[hash]
	if !ok {
		return nil, resource.ErrInvalid
	}
	return bytes.Clone(raw), ctx.Err()
}

func checkNativeCommittedRead(t *testing.T, ctx context.Context, reader *ResourceRoom, writer resource.VerifiedAccess, candidate resource.Candidate, reference storage.Reference, payload []byte) {
	t.Helper()
	full, err := resource.VerifyCandidate(ctx, candidate, writer, reader.current.Device(), nil)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := resource.CaptureReservation(ctx, candidate, full, candidate.Proof.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	value := captured.Value()
	receipt := resource.Receipt{Resource: value.Resource, Epoch: value.Epoch, OpID: value.OpID, PublicationSHA256: value.PublicationSHA256, Reference: reference, Base: value.Base, Head: resource.Position{Seq: value.End, HeadCipherSHA256: candidate.Proof.OuterCipherSHA256}}
	source := &nativeCommittedWitness{value: resource.CommittedPublication{State: resource.Committed, Resource: value.Resource, OpID: value.OpID, AttemptID: value.AttemptID, Epoch: value.Epoch, Generation: value.Generation, PolicySHA256: value.PolicySHA256, CandidateSHA256: value.CandidateSHA256, PublicationSHA256: value.PublicationSHA256, Base: value.Base, End: value.End, DescriptorSHA256: captured.SHA256(), Descriptor: captured.Bytes(), Candidate: candidate, Receipt: receipt}, evidence: map[string]json.RawMessage{}}
	for _, hash := range captured.EvidenceHashes() {
		record, err := captured.Evidence(hash)
		if err != nil {
			t.Fatal(err)
		}
		source.evidence[hash] = record.Value
	}
	proof, err := resource.VerifyCommittedRead(ctx, value.Resource, value.OpID, reader.access, reader.fence, source)
	if err != nil {
		t.Fatal("actual SDK committed read proof", err)
	}
	actual, err := reader.ReadCommitted(ctx, proof)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("actual Native SDK/age committed read", err)
	}
	clear(actual)
	var expectedRecord storage.Record
	if json.Unmarshal(payload, &expectedRecord) != nil {
		t.Fatal("native test original record invalid")
	}
	decoded, err := reader.ReadRecordCommitted(ctx, proof, expectedRecord.ObjectID, expectedRecord.RevisionID, expectedRecord.Role)
	if err != nil || !expectedRecord.MatchesExact(decoded) || proof.ProducerScope() != expectedRecord.Scope {
		t.Fatal("actual Native SDK lost private record context", err)
	}
	clear(decoded.Payload)
	if out, err := reader.ReadRecordCommitted(ctx, proof, "other-object", expectedRecord.RevisionID, expectedRecord.Role); err == nil || len(out.Payload) != 0 {
		t.Fatal("actual Native SDK released another object record")
	}
	owner, err := reader.ActorScope().JournalOwner()
	if err != nil {
		t.Fatal(err)
	}
	key, err := resource.PublicationWorkKey(value.Resource, value.OpID)
	if err != nil {
		t.Fatal(err)
	}
	var original resource.PublicationWork
	found, err := reader.journal.Read(owner, storage.ResourcePublicationNamespace, key, &original)
	if err != nil || !found || original.Candidate == nil {
		t.Fatal("prepared resource work missing from scoped suite journal", err)
	}
	defer clear(original.Payload)
	if n, err := reader.journal.CountOwner(owner, storage.ResourcePublicationNamespace); err != nil || n != 1 {
		t.Fatal("regular resource work not counted", n, err)
	}
	changed := original
	changed.Payload = []byte("different private original intent")
	if err := reader.journal.Write(owner, storage.ResourcePublicationNamespace, key, changed); err != nil {
		t.Fatal(err)
	}
	if ok, err := reader.AcknowledgeCommitted(ctx, proof); err == nil || ok {
		t.Fatal("actual SDK payload mismatch retired resource work")
	}
	if n, err := reader.journal.CountOwner(owner, storage.ResourcePublicationNamespace); err != nil || n != 1 {
		t.Fatal("failed SDK readback dropped pending work", n, err)
	}
	if err := reader.journal.Write(owner, storage.ResourcePublicationNamespace, key, original); err != nil {
		t.Fatal(err)
	}
	if ok, err := reader.AcknowledgeCommitted(ctx, resource.VerifiedCommittedRead{}); err == nil || ok {
		t.Fatal("public/empty receipt retired pending work")
	}
	if ok, err := reader.AcknowledgeCommitted(ctx, proof); err != nil || !ok {
		t.Fatal("actual committed SDK readback did not acknowledge original work", ok, err)
	}
	if n, err := reader.journal.CountOwner(owner, storage.ResourcePublicationNamespace); err != nil || n != 0 {
		t.Fatal("acknowledged work still pending", n, err)
	}
	if n, err := reader.journal.CountOwner(owner, storage.ResourcePublicationHistoryNamespace); err != nil || n != 1 {
		t.Fatal("accepted original resource history lost", n, err)
	}
	if ok, err := reader.AcknowledgeCommitted(ctx, proof); err != nil || !ok {
		t.Fatal("historical ACK retry failed", ok, err)
	}
	if out, err := reader.ReadCommitted(ctx, resource.VerifiedCommittedRead{}); err == nil || out != nil {
		t.Fatal("empty committed proof opened actual SDK bytes")
	}
}
