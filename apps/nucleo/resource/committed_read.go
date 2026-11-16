// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// CommittedPublication contains independent immutable SQL columns, the exact
// captured reservation and the committed receipt. It is not a hold, a pending
// operation or a grant to restore the producer's live device.
type CommittedPublication struct {
	State             CandidateState
	Resource          Resource
	OpID              string
	AttemptID         string
	Epoch             string
	Generation        Decimal
	PolicySHA256      string
	CandidateSHA256   string
	PublicationSHA256 string
	Base              Position
	End               Decimal
	DescriptorSHA256  string
	Descriptor        []byte
	Candidate         Candidate
	Receipt           Receipt
}

// TrustedCommittedSource reads the original ACL/CAS store under CURRENT Read
// access. Implementations must reject every non-Committed state and compare the
// receipt against its separately persisted routing/sequence/reference columns.
// Public backup JSON or a REST hint is not an implementation of this contract.
type TrustedCommittedSource interface {
	PublicEvidenceSource
	CommittedPublication(context.Context, Resource, string) (CommittedPublication, error)
	CheckCommittedPublication(context.Context, Resource, string, string, Receipt) error
}

// VerifiedCommittedRead authorizes only an exact historical read by one current
// independently enrolled reader. It exposes no producer Device, pending value,
// acquisition or publication method and cannot be decoded from public JSON.
type VerifiedCommittedRead struct {
	producerScope   storage.Scope
	value           CipherPublication
	hash            string
	candidateHash   string
	reservationHash string
	receipt         Receipt
	epoch           VerifiedEpoch
	session         sessionproof.Session
	reader          VerifiedAccess
	binding         SessionBinding
}

// ProducerScope identifies the independently verified historical author.
// It is metadata for decoding the original record, never a current owner vault
// or permission to impersonate the producer.
func (p VerifiedCommittedRead) ProducerScope() storage.Scope { return p.producerScope }

func (p VerifiedCommittedRead) Value() CipherPublication      { return p.value }
func (p VerifiedCommittedRead) SHA256() string                { return p.hash }
func (p VerifiedCommittedRead) CandidateSHA256() string       { return p.candidateHash }
func (p VerifiedCommittedRead) Receipt() Receipt              { return p.receipt }
func (p VerifiedCommittedRead) Epoch() VerifiedEpoch          { return p.epoch }
func (p VerifiedCommittedRead) Session() sessionproof.Session { return p.session }

func (p VerifiedCommittedRead) valid() bool {
	return p.producerScope.Valid() && p.producerScope.Tenant == p.value.Resource.Tenant && p.producerScope.Product == p.value.Resource.Product && p.value.Valid() && digest(p.hash) && digest(p.candidateHash) && digest(p.reservationHash) && p.receipt.Valid() && p.epoch.valid() && p.reader.valid() && p.reader.action == Read && p.binding.Valid() && p.receipt.Resource == p.value.Resource && p.receipt.Reference.RoomID == p.value.RoomID && p.receipt.PublicationSHA256 == p.hash && p.receipt.Base == p.value.ExpectedBase && p.receipt.Head.HeadCipherSHA256 == p.value.OuterCipherSHA256
}

// RevalidateReader keeps historical producer A separate from current reader B.
// Matching account metadata or possession of an epoch key cannot substitute for
// B's exact enrolled SDK session, capability and current original ACL fence.
func (p VerifiedCommittedRead) RevalidateReader(ctx context.Context, reader VerifiedAccess, fence Fence) error {
	binding, ok := reader.CurrentSessionBinding()
	if !p.valid() || !ok || binding != p.binding || reader.action != Read || reader.epoch.hash != p.reader.epoch.hash || reader.capability.hash != p.reader.capability.hash {
		return ErrRevoked
	}
	return checkFence(ctx, fence, reader)
}

func decodeCommittedDescriptor(stored CommittedPublication, key Resource, opID string) (Reservation, string, error) {
	if stored.State != Committed || stored.Resource != key || stored.OpID != opID || !digest(stored.DescriptorSHA256) {
		return Reservation{}, "", ErrInvalid
	}
	var descriptor Reservation
	if err := decodeReservationValue(stored.Descriptor, &descriptor); err != nil {
		return Reservation{}, "", err
	}
	hash, err := CipherJSONDigest(stored.Descriptor)
	if err != nil || hash != stored.DescriptorSHA256 || !descriptor.valid() || descriptor.Resource != stored.Resource || descriptor.OpID != stored.OpID || descriptor.AttemptID != stored.AttemptID || descriptor.Epoch != stored.Epoch || descriptor.Generation != stored.Generation || descriptor.PolicySHA256 != stored.PolicySHA256 || descriptor.CandidateSHA256 != stored.CandidateSHA256 || descriptor.PublicationSHA256 != stored.PublicationSHA256 || descriptor.Base != stored.Base || descriptor.End != stored.End {
		return Reservation{}, "", ErrInvalid
	}
	return descriptor, hash, nil
}

func validCommittedReceipt(receipt Receipt, key Resource, opID string, descriptor Reservation, value CipherPublication, window EpochWindow) error {
	if !receipt.Valid() || receipt.Resource != key || receipt.OpID != opID || receipt.Epoch != descriptor.Epoch || receipt.PublicationSHA256 != descriptor.PublicationSHA256 || receipt.Reference.RoomID != value.RoomID || receipt.Base != descriptor.Base || receipt.Head.Seq != descriptor.End || receipt.Head.HeadCipherSHA256 != value.OuterCipherSHA256 || !window.contains(value.Publication) {
		return ErrInvalid
	}
	if window.through != nil && receipt.Head.Seq == window.through.Seq && receipt.Head != *window.through {
		return ErrFork
	}
	return nil
}

func canReadCommittedHistory(reader VerifiedAccess, epoch VerifiedEpoch, value CipherPublication) bool {
	from, _ := reader.capability.value.From.Seq.Int64()
	base, _ := value.ExpectedBase.Seq.Int64()
	if base < from || base == from && value.ExpectedBase != reader.capability.value.From {
		return false
	}
	return epoch.hash == reader.epoch.hash || reader.epoch.value.History != HistoryJoined || historicalMember(epoch, reader.capability.member)
}

// VerifyCommittedRead verifies captured historical signatures/certificates while
// requiring a live Read session only for the current reader. It never calls a
// live producer lookup, Stage, Acquire, Send or a Sending/Ambiguous restoration.
//
//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func VerifyCommittedRead(ctx context.Context, key Resource, opID string, reader VerifiedAccess, fence Fence, source TrustedCommittedSource) (VerifiedCommittedRead, error) {
	binding, current := reader.CurrentSessionBinding()
	if !current || !reader.valid() || reader.action != Read || !key.Valid() || reader.epoch.value.Resource != key || !storage.Identifier(opID) || source == nil {
		return VerifiedCommittedRead{}, ErrInvalid
	}
	if err := checkFence(ctx, fence, reader); err != nil {
		return VerifiedCommittedRead{}, err
	}
	stored, err := source.CommittedPublication(ctx, key, opID)
	if err != nil {
		return VerifiedCommittedRead{}, err
	}
	descriptor, hash, err := decodeCommittedDescriptor(stored, key, opID)
	if err != nil {
		return VerifiedCommittedRead{}, err
	}
	epoch, window, err := committedEpochWindow(reader.epoch, descriptor.PolicySHA256)
	if err != nil || epoch.value.Resource != key || epoch.value.ID != descriptor.Epoch || epoch.value.Generation != descriptor.Generation || epoch.value.MembershipSHA256 != descriptor.MembershipSHA256 {
		return VerifiedCommittedRead{}, ErrInvalid
	}
	verified, err := restoreHistoricalPublication(ctx, descriptor, hash, stored.Candidate, source)
	if err != nil {
		return VerifiedCommittedRead{}, err
	}
	value := verified.value
	receipt := stored.Receipt
	if err := validCommittedReceipt(receipt, key, opID, descriptor, value, window); err != nil {
		return VerifiedCommittedRead{}, err
	}
	if !canReadCommittedHistory(reader, epoch, value) {
		return VerifiedCommittedRead{}, ErrRevoked
	}
	certificate, err := sessionproof.DecodeCertificate(stored.Candidate.SessionCertificate)
	if err != nil {
		return VerifiedCommittedRead{}, err
	}
	if err := source.CheckCommittedPublication(ctx, key, opID, hash, receipt); err != nil {
		return VerifiedCommittedRead{}, err
	}
	if err := checkFence(ctx, fence, reader); err != nil {
		return VerifiedCommittedRead{}, err
	}
	return VerifiedCommittedRead{producerScope: verified.descriptor.Producer.Pin.Scope, value: value, hash: verified.descriptor.PublicationSHA256, candidateHash: descriptor.CandidateSHA256, reservationHash: hash, receipt: receipt, epoch: epoch, session: certificate.Session, reader: reader, binding: binding}, nil
}

// Resolve only an ancestor of the independently verified CURRENT policy chain.
// A captured self-signed historical policy is never a replacement for that pin.
func committedEpochWindow(current VerifiedEpoch, hash string) (VerifiedEpoch, EpochWindow, error) {
	if !current.valid() || !digest(hash) {
		return VerifiedEpoch{}, EpochWindow{}, ErrInvalid
	}
	var next *VerifiedEpoch
	for at := &current; at != nil; at = at.parent {
		if at.hash == hash {
			var window EpochWindow
			var err error
			if next == nil {
				window, err = CurrentEpoch(*at)
			} else {
				window, err = ClosedEpoch(*at, *next)
			}
			return *at, window, err
		}
		next = at
	}
	return VerifiedEpoch{}, EpochWindow{}, ErrRevoked
}

// OpenCommittedFrame accepts only the opaque read proof and the original
// inner-cipher commitment. It confers no publication or producer authority.
func (h *EpochHandle) OpenCommittedFrame(ctx context.Context, publication VerifiedCommittedRead, envelope pqc.Envelope, reader VerifiedAccess, fence Fence) ([]byte, error) {
	if err := publication.RevalidateReader(ctx, reader, fence); err != nil {
		return nil, err
	}
	return h.openVerifiedFrame(ctx, publication.value, publication.epoch.hash, envelope, reader, fence)
}
