// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PublicationWork is the private durable intent, shared by native and Wasm.
// Field names retain the existing sealed native journal representation. Payload
// is never public evidence; Candidate contains only the original encrypted wire.
// Capturing work cannot create a publication or reader authority.
type PublicationWork struct {
	Version     int
	Binding     SessionBinding
	Publication Publication
	Payload     []byte
	Candidate   *Candidate
}

func PublicationWorkKey(key Resource, opID string) (string, error) {
	if !key.Valid() || !storage.Identifier(opID) {
		return "", ErrInvalid
	}
	return key.Tenant + "/" + key.Product + "/" + key.Kind + "/" + key.ID + "/" + opID, nil
}

func (w PublicationWork) Valid() bool {
	if w.Version != 1 || !w.Binding.Valid() || !w.Publication.Valid() || w.Binding.Scope.Tenant != w.Publication.Resource.Tenant || w.Binding.Scope.Product != w.Publication.Resource.Product || w.Binding.Actor() != w.Publication.Actor {
		return false
	}
	return w.Candidate == nil || CheckCandidate(*w.Candidate) == nil && w.Candidate.Proof.Publication == w.Publication
}

func (w PublicationWork) Matches(binding SessionBinding, header Publication, payload []byte) bool {
	return w.Valid() && w.Binding == binding && w.Publication == header && bytes.Equal(w.Payload, payload)
}

// AcknowledgedPublicationWork is retained encrypted BEFORE deleting pending work.
// It is a local readback receipt, never a new authority or permission to resend.
type AcknowledgedPublicationWork struct {
	Version int
	Work    PublicationWork
	Receipt Receipt
}

func (h AcknowledgedPublicationWork) Valid() bool {
	w := h.Work
	if h.Version != 1 || !w.Valid() || w.Candidate == nil || !h.Receipt.Valid() {
		return false
	}
	value := w.Candidate.Proof.CipherPublication
	hash, err := PublicationDigest(value)
	end, endErr := value.End()
	return err == nil && endErr == nil && h.Receipt.Resource == value.Resource && h.Receipt.Epoch == value.Epoch && h.Receipt.OpID == value.OpID && h.Receipt.Reference.RoomID == value.RoomID && h.Receipt.PublicationSHA256 == hash && h.Receipt.Base == value.ExpectedBase && h.Receipt.Head.Seq == end && h.Receipt.Head.HeadCipherSHA256 == value.OuterCipherSHA256
}

// CheckCommittedPublicationWork verifies the original work against the opaque
// committed proof and CURRENT reader. Historical producer A is not resolved live.
// This is metadata checking only: the maintained SDK must also read/decrypt the
// exact committed event and compare its plaintext against Work.Payload before ACK.
func CheckCommittedPublicationWork(ctx context.Context, w PublicationWork, proof VerifiedCommittedRead, reader VerifiedAccess, fence Fence) error {
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		return err
	}
	if !w.Valid() || w.Candidate == nil || w.Binding.Scope != reader.Scope() || w.Publication != proof.Value().Publication || w.Candidate.Proof.CipherPublication != proof.Value() {
		return ErrInvalid
	}
	candidate, err := CandidateDigest(*w.Candidate)
	if err != nil || candidate != proof.CandidateSHA256() {
		return ErrInvalid
	}
	return proof.RevalidateReader(ctx, reader, fence)
}
