// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// ValidationEvidence is public independently verified authority for a device
// running the original application validator. Possession of an editor signing
// key does not substitute for this separately granted Validate capability.
type ValidationEvidence struct {
	Access VerifiedAccess
	Device matrixproof.Device
}
type VerifiedPendingPublication struct {
	producer      matrixproof.Device
	access        VerifiedAccess
	value         CipherPublication
	hash          string
	candidateHash string
	epoch         VerifiedEpoch
}

func (p VerifiedPendingPublication) Value() CipherPublication { return p.value }
func (p VerifiedPendingPublication) SHA256() string           { return p.hash }
func (p VerifiedPendingPublication) CandidateSHA256() string  { return p.candidateHash }
func (p VerifiedPendingPublication) Epoch() VerifiedEpoch     { return p.epoch }

// VerifyCandidate verifies only an opaque publication for an already verified
// current access binding. The relay must still fence CURRENT account/device/
// ACL/epoch/base state before and after network acceptance. A cached verified
// value is not a lease and must never bypass those durable checks.
func VerifyProducerCandidate(ctx context.Context, candidate Candidate, access VerifiedAccess, producer matrixproof.Device) (VerifiedPendingPublication, error) {
	if !access.valid() || (access.action != Publish && access.action != Comment) {
		return VerifiedPendingPublication{}, ErrInvalid
	}
	value, hash, coreHash, err := verifyProducerTranscript(ctx, candidate, access.epoch.value, access.epoch.hash, access.capability.value, access.capability.hash, access.capability.member, access.capability.enrollment, access.action, producer.Identity(), producer.VerifyJSON, func(ctx context.Context, raw []byte, expected sessionproof.CertificateExpectation) (sessionproof.VerifiedCertificate, error) {
		return sessionproof.VerifyCertificate(ctx, raw, expected, producer)
	})
	if err != nil {
		return VerifiedPendingPublication{}, err
	}
	return VerifiedPendingPublication{value: value, hash: hash, candidateHash: coreHash, epoch: access.epoch, producer: producer, access: access}, nil
}

type (
	verifyTranscript  func(context.Context, storage.Scope, storage.PublicationTarget, any) error
	verifyCertificate func(context.Context, []byte, sessionproof.CertificateExpectation) (sessionproof.VerifiedCertificate, error)
)

func publicationMatchesEpoch(value CipherPublication, epoch Epoch, epochHash string, capability Capability, capabilityHash string, member Member, enrollment RecipientEnrollment, action Action, identity matrixproof.Identity) bool {
	if value.Resource != epoch.Resource || value.RoomID != epoch.RoomID || value.Epoch != epoch.ID || value.Generation != epoch.Generation || value.MembershipSHA256 != epoch.MembershipSHA256 || value.PolicySHA256 != epochHash || value.CapabilitySHA256 != capabilityHash || value.Action != action || value.Actor != enrollment.Issuer || !identityMatches(identity, value.Actor, member.Scope) {
		return false
	}
	base, _ := value.ExpectedBase.Seq.Int64()
	from, _ := capability.From.Seq.Int64()
	cutover, _ := epoch.Cutover.Seq.Int64()
	return base >= from && base >= cutover
}

// Both live and historical paths use these exact bindings. Only the live entry
// point returns a publication usable by Acquire; recovery returns a hold type.
func verifyProducerTranscript(ctx context.Context, candidate Candidate, epoch Epoch, epochHash string, capability Capability, capabilityHash string, member Member, enrollment RecipientEnrollment, action Action, identity matrixproof.Identity, verify verifyTranscript, certificate verifyCertificate) (CipherPublication, string, string, error) {
	if err := ctx.Err(); err != nil {
		return CipherPublication{}, "", "", err
	}
	if !epoch.Valid() || !capability.Valid() || !member.valid(epoch.Resource) || !enrollment.Valid() || !capability.Allows(action) || (action != Publish && action != Comment) {
		return CipherPublication{}, "", "", ErrInvalid
	}
	if err := CheckCandidate(candidate); err != nil {
		return CipherPublication{}, "", "", err
	}
	value := candidate.Proof.CipherPublication
	if !publicationMatchesEpoch(value, epoch, epochHash, capability, capabilityHash, member, enrollment, action, identity) {
		return CipherPublication{}, "", "", ErrInvalid
	}
	target := storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Actor.DeviceID}
	if err := verify(ctx, member.Scope, target, candidate.Proof); err != nil {
		return CipherPublication{}, "", "", err
	}
	expected := sessionproof.CertificateExpectation{Scope: member.Scope, Resource: value.Resource, Epoch: value.Epoch, Generation: value.Generation, PolicySHA256: value.PolicySHA256, Session: sessionproof.Session{RoomID: value.RoomID, SenderUser: value.Actor.UserID, SigningKey: identity.SigningKey.String()}, DeviceID: value.Actor.DeviceID, RootKey: value.Actor.RootKey}
	session, err := verifyEncryptedSessionCertificate(ctx, candidate.SessionCertificate, candidate.EncryptedEvent, expected, certificate)
	if err != nil || session.ID() != value.SessionCertificateID {
		return CipherPublication{}, "", "", ErrInvalid
	}
	hash, err := PublicationDigest(value)
	if err != nil {
		return CipherPublication{}, "", "", err
	}
	coreHash, err := CandidateDigest(candidate)
	if err != nil {
		return CipherPublication{}, "", "", err
	}
	return value, hash, coreHash, nil
}

func verifyValidation(ctx context.Context, certificate *ValidationCertificate, publicationHash string, epoch VerifiedEpoch, evidence *ValidationEvidence) error {
	if certificate == nil || evidence == nil {
		return ErrValidatorUnavailable
	}
	access := evidence.Access
	if !access.valid() || access.action != Validate || access.epoch.hash != epoch.hash {
		return ErrInvalid
	}
	return verifyValidationTranscript(ctx, certificate, publicationHash, epoch.value, access.capability.value, access.capability.hash, access.capability.member, access.capability.enrollment, evidence.Device.Identity(), evidence.Device.VerifyJSON)
}

func verifyValidationTranscript(ctx context.Context, certificate *ValidationCertificate, publicationHash string, epoch Epoch, capability Capability, capabilityHash string, member Member, enrollment RecipientEnrollment, identity matrixproof.Identity, verify verifyTranscript) error {
	if certificate == nil {
		return ErrValidatorUnavailable
	}
	if !certificate.Valid() || !certificate.Allow || !capability.Allows(Validate) || certificate.PublicationSHA256 != publicationHash || certificate.ValidatorCapabilitySHA256 != capabilityHash || certificate.RulesVersion != epoch.Validation.RulesVersion || certificate.Issuer != enrollment.Issuer || !identityMatches(identity, certificate.Issuer, member.Scope) {
		return ErrInvalid
	}
	return verify(ctx, member.Scope, storage.PublicationTarget{RoomID: epoch.RoomID, DeviceID: certificate.Issuer.DeviceID}, certificate)
}

// VerifiedPublication adds the required application validation to a producer-
// authenticated durable candidate. Neither type can be constructed from JSON.
type VerifiedPublication struct {
	pending   VerifiedPendingPublication
	validator *ValidationEvidence
}

func (p VerifiedPublication) Value() CipherPublication { return p.pending.Value() }
func (p VerifiedPublication) SHA256() string           { return p.pending.SHA256() }
func (p VerifiedPublication) CandidateSHA256() string  { return p.pending.CandidateSHA256() }
func (p VerifiedPublication) Epoch() VerifiedEpoch     { return p.pending.Epoch() }

func VerifyCandidate(ctx context.Context, candidate Candidate, access VerifiedAccess, producer matrixproof.Device, validator *ValidationEvidence) (VerifiedPublication, error) {
	pending, err := VerifyProducerCandidate(ctx, candidate, access, producer)
	if err != nil {
		return VerifiedPublication{}, err
	}
	return CompleteCandidate(ctx, candidate, pending, validator)
}

// CompleteCandidate may append a separately signed validation certificate to a
// durable candidate, but cannot change its producer proof, content or session.
func CompleteCandidate(ctx context.Context, candidate Candidate, pending VerifiedPendingPublication, validator *ValidationEvidence) (VerifiedPublication, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedPublication{}, err
	}
	hash, err := CandidateDigest(candidate)
	if err != nil || hash != pending.candidateHash || !digest(pending.hash) || !pending.epoch.valid() {
		return VerifiedPublication{}, ErrInvalid
	}
	if pending.epoch.value.Validation.PrivateRules || candidate.Validation != nil {
		if err := verifyValidation(ctx, candidate.Validation, pending.hash, pending.epoch, validator); err != nil {
			return VerifiedPublication{}, err
		}
	}
	var evidence *ValidationEvidence
	if validator != nil {
		copied := *validator
		evidence = &copied
	}
	return VerifiedPublication{pending: pending, validator: evidence}, nil
}

// Bootstrap and active-epoch messages share the maintained SDK session binding.
func verifyEncryptedSessionCertificate(ctx context.Context, raw, wire []byte, expected sessionproof.CertificateExpectation, verify verifyCertificate) (sessionproof.VerifiedCertificate, error) {
	var encrypted event.EncryptedEventContent
	if json.Unmarshal(wire, &encrypted) != nil {
		return sessionproof.VerifiedCertificate{}, ErrInvalid
	}
	expected.Session.SessionID = encrypted.SessionID.String()
	expected.Session.SenderKey = encrypted.SenderKey.String()
	return verify(ctx, raw, expected)
}
