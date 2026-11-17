// SPDX-License-Identifier: Apache-2.0
package resourcetest

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// CommittedSource is a public-metadata test witness, not a substitute for original
// durable SQL/RLS or an actual SDK GET/decryption. All Verified values still pass
// the real production signature, policy, current-session and receipt verifiers.
type CommittedSource struct {
	Stored     resource.CommittedPublication
	Evidence   resource.PublicEvidenceSource
	Checks     int
	CheckError error
	records    map[string]json.RawMessage
}

func NewCommittedSource(t testing.TB, capture resource.CapturedReservation, candidate resource.Candidate) *CommittedSource {
	t.Helper()
	value := capture.Value()
	receipt := resource.Receipt{Resource: value.Resource, Epoch: value.Epoch, OpID: value.OpID, PublicationSHA256: value.PublicationSHA256, Reference: storage.Reference{RoomID: candidate.Proof.RoomID, EventID: "$committed-original"}, Base: value.Base, Head: resource.Position{Seq: value.End, HeadCipherSHA256: candidate.Proof.OuterCipherSHA256}}
	s := &CommittedSource{Stored: resource.CommittedPublication{State: resource.Committed, Resource: value.Resource, OpID: value.OpID, AttemptID: value.AttemptID, Epoch: value.Epoch, Generation: value.Generation, PolicySHA256: value.PolicySHA256, CandidateSHA256: value.CandidateSHA256, PublicationSHA256: value.PublicationSHA256, Base: value.Base, End: value.End, DescriptorSHA256: capture.SHA256(), Descriptor: capture.Bytes(), Candidate: candidate, Receipt: receipt}, records: map[string]json.RawMessage{}}
	for _, hash := range capture.EvidenceHashes() {
		record, err := capture.Evidence(hash)
		if err != nil {
			t.Fatal(err)
		}
		s.records[hash] = record.Value
	}
	return s
}

func (s *CommittedSource) CommittedPublication(ctx context.Context, key resource.Resource, op string) (resource.CommittedPublication, error) {
	if err := ctx.Err(); err != nil {
		return resource.CommittedPublication{}, err
	}
	if key != s.Stored.Resource || op != s.Stored.OpID {
		return resource.CommittedPublication{}, resource.ErrInvalid
	}
	return s.Stored, nil
}

func (s *CommittedSource) CheckCommittedPublication(ctx context.Context, key resource.Resource, op, hash string, receipt resource.Receipt) error {
	s.Checks++
	if s.CheckError != nil {
		return s.CheckError
	}
	if key != s.Stored.Resource || op != s.Stored.OpID || hash != s.Stored.DescriptorSHA256 || receipt != s.Stored.Receipt {
		return resource.ErrFork
	}
	return ctx.Err()
}

func (s *CommittedSource) PublicEvidence(ctx context.Context, key resource.Resource, hash string) (json.RawMessage, error) {
	if s.Evidence != nil {
		return s.Evidence.PublicEvidence(ctx, key, hash)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, ok := s.records[hash]
	if key != s.Stored.Resource || !ok {
		return nil, resource.ErrInvalid
	}
	return bytes.Clone(raw), nil
}
