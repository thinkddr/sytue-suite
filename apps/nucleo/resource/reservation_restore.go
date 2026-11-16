// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// HeldReservation is loaded from the original immutable Sending/Ambiguous hold,
// not decoded from request JSON. All scalar bindings are read independently from
// its reservation descriptor. Source implementations require a current resource
// manager or the authenticated internal relay and apply ordinary tenant RLS.
type HeldReservation struct {
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
	Descriptor        json.RawMessage
	Candidate         Candidate
}
type PublicEvidenceSource interface {
	PublicEvidence(context.Context, Resource, string) (json.RawMessage, error)
}

type TrustedHoldSource interface {
	PublicEvidenceSource
	HeldReservation(context.Context, Resource, string) (HeldReservation, error)
}

// VerifiedReservation can only reconcile the exact already held operation. It
// exposes no VerifiedAccess, live Device or VerifiedPublication that could be
// passed into Stage/Acquire or used to produce a new ciphertext as a revoked A.
type VerifiedReservation struct {
	descriptor Reservation
	hash       string
	value      CipherPublication
}

func (r VerifiedReservation) Value() Reservation { return cloneReservation(r.descriptor) }
func (r VerifiedReservation) SHA256() string     { return r.hash }
func (r VerifiedReservation) valid() bool {
	return r.descriptor.valid() && digest(r.hash) && r.value.Valid() && r.descriptor.PublicationSHA256 != "" && r.value.Resource == r.descriptor.Resource && r.value.OpID == r.descriptor.OpID
}

type reservationReader struct {
	source   PublicEvidenceSource
	resource Resource
	cache    map[string]json.RawMessage
}

func decodeReservationValue(raw []byte, out any) error {
	if len(raw) == 0 {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	canonical, err := matrixproof.CanonicalBytes(json.RawMessage(raw))
	if err != nil {
		return ErrInvalid
	}
	defer clear(canonical)
	if !bytes.Equal(raw, canonical) {
		return ErrInvalid
	}
	return nil
}

func (r *reservationReader) record(ctx context.Context, hash string, out any) error {
	if !digest(hash) {
		return ErrInvalid
	}
	raw, ok := r.cache[hash]
	if !ok {
		var err error
		raw, err = r.source.PublicEvidence(ctx, r.resource, hash)
		if err != nil {
			return err
		}
		actual, err := CipherJSONDigest(raw)
		if err != nil || actual != hash {
			return ErrInvalid
		}
		r.cache[hash] = bytes.Clone(raw)
	}
	return decodeReservationValue(raw, out)
}

func (r *reservationReader) signer(ctx context.Context, ref SignerEvidence) (matrixproof.HistoricalDevice, error) {
	var evidence matrixproof.DeviceEvidence
	if err := r.record(ctx, ref.DeviceSHA256, &evidence); err != nil {
		return matrixproof.HistoricalDevice{}, err
	}
	return matrixproof.VerifyHistoricalDevice(ctx, evidence, ref.Pin)
}

func (r *reservationReader) statement(ctx context.Context, ref StatementEvidence, out any, actor Actor, scope storage.Scope, room string) (matrixproof.HistoricalDevice, error) {
	if scope.Tenant != r.resource.Tenant || scope.Product != r.resource.Product {
		return matrixproof.HistoricalDevice{}, ErrInvalid
	}
	if err := r.record(ctx, ref.StatementSHA256, out); err != nil {
		return matrixproof.HistoricalDevice{}, err
	}
	signer, err := r.signer(ctx, ref.Signer)
	if err != nil {
		return matrixproof.HistoricalDevice{}, err
	}
	if !identityMatches(signer.Identity(), actor, scope) {
		return matrixproof.HistoricalDevice{}, ErrInvalid
	}
	if err := signer.VerifyJSON(ctx, scope, storage.PublicationTarget{RoomID: room, DeviceID: actor.DeviceID}, out); err != nil {
		return matrixproof.HistoricalDevice{}, err
	}
	return signer, nil
}

func (r *reservationReader) enrollment(ctx context.Context, ref StatementEvidence, member Member, room string) (RecipientEnrollment, error) {
	var proof SignedRecipient
	if err := r.record(ctx, ref.StatementSHA256, &proof); err != nil {
		return RecipientEnrollment{}, err
	}
	if !proof.Valid() || !memberMatchesEnrollment(member, proof.RecipientEnrollment) {
		return RecipientEnrollment{}, ErrInvalid
	}
	hash, err := matrixproof.RequestDigest(proof.RecipientEnrollment)
	if err != nil || hash != member.EnrollmentSHA256 {
		return RecipientEnrollment{}, ErrInvalid
	}
	if _, err := r.statement(ctx, ref, &proof, proof.Issuer, member.Scope, room); err != nil {
		return RecipientEnrollment{}, err
	}
	return proof.RecipientEnrollment, nil
}

type restoredCapability struct {
	value      Capability
	hash       string
	member     Member
	enrollment RecipientEnrollment
}

func (r *reservationReader) capability(ctx context.Context, ref CapabilityEvidence, epoch Epoch, policyHash string, members []Member) (restoredCapability, error) {
	var proof SignedCapability
	if err := r.record(ctx, ref.Capability.StatementSHA256, &proof); err != nil {
		return restoredCapability{}, err
	}
	selected, err := selectCapabilityMember(proof.Capability, epoch, policyHash, members)
	if err != nil {
		return restoredCapability{}, err
	}
	scope := ref.Capability.Signer.Pin.Scope
	var manager Member
	found := false
	for _, member := range members {
		if member.Scope == scope && member.UserID == proof.Issuer.UserID && member.RootKey == proof.Issuer.RootKey && slices.Contains(member.Actions, Manage) {
			enrolled, err := r.enrollment(ctx, ref.IssuerEnrollment, member, epoch.RoomID)
			if err == nil && enrolled.Issuer == proof.Issuer {
				manager = member
				found = true
				break
			}
		}
	}
	if !found || !manager.valid(epoch.Resource) {
		return restoredCapability{}, ErrInvalid
	}
	if _, err := r.statement(ctx, ref.Capability, &proof, proof.Issuer, scope, epoch.RoomID); err != nil {
		return restoredCapability{}, err
	}
	recipient, err := r.enrollment(ctx, ref.Recipient, selected, epoch.RoomID)
	if err != nil {
		return restoredCapability{}, err
	}
	hash, err := CapabilityDigest(proof.Capability)
	if err != nil {
		return restoredCapability{}, err
	}
	return restoredCapability{value: proof.Capability, hash: hash, member: selected, enrollment: recipient}, nil
}

func decodeHeldDescriptor(held HeldReservation, resource Resource, opID string) (Reservation, string, error) {
	if (held.State != Sending && held.State != Ambiguous) || held.Resource != resource || held.OpID != opID || !digest(held.DescriptorSHA256) {
		return Reservation{}, "", ErrInvalid
	}
	var descriptor Reservation
	if err := decodeReservationValue(held.Descriptor, &descriptor); err != nil {
		return Reservation{}, "", err
	}
	hash, err := CipherJSONDigest(held.Descriptor)
	if err != nil || hash != held.DescriptorSHA256 || !descriptor.valid() || descriptor.Resource != held.Resource || descriptor.OpID != held.OpID || descriptor.AttemptID != held.AttemptID || descriptor.Epoch != held.Epoch || descriptor.Generation != held.Generation || descriptor.PolicySHA256 != held.PolicySHA256 || descriptor.CandidateSHA256 != held.CandidateSHA256 || descriptor.PublicationSHA256 != held.PublicationSHA256 || descriptor.Base != held.Base || descriptor.End != held.End {
		return Reservation{}, "", ErrInvalid
	}
	return descriptor, hash, nil
}

// RestoreReservation does not treat a public backup, JSON row or current key
// query as previous authorization. It requires an existing immutable hold and
// its independently persisted digest/bindings, then rechecks all public SDK
// signatures and exact historical pins captured by CaptureReservation at Acquire.
func RestoreReservation(ctx context.Context, resource Resource, opID string, source TrustedHoldSource) (VerifiedReservation, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedReservation{}, err
	}
	if source == nil || !resource.Valid() || !storage.Identifier(opID) {
		return VerifiedReservation{}, ErrInvalid
	}
	held, err := source.HeldReservation(ctx, resource, opID)
	if err != nil {
		return VerifiedReservation{}, err
	}
	descriptor, hash, err := decodeHeldDescriptor(held, resource, opID)
	if err != nil {
		return VerifiedReservation{}, err
	}
	verified, err := restoreHistoricalPublication(ctx, descriptor, hash, held.Candidate, source)
	if err != nil {
		return VerifiedReservation{}, err
	}
	return VerifiedReservation(verified), nil
}

// historicalPublication contains only captured signed public evidence. It is
// never a live producer, VerifiedPendingPublication, acquisition or send grant.
type historicalPublication struct {
	descriptor Reservation
	hash       string
	value      CipherPublication
}

func (r *reservationReader) verifyHistoricalValidation(ctx context.Context, descriptor Reservation, candidate Candidate, policy SignedEpoch, policyHash, publicationHash string, members []Member) error {
	if !policy.Validation.PrivateRules && candidate.Validation == nil && descriptor.Validator == nil && descriptor.ValidationSHA256 == "" {
		return nil
	}
	if descriptor.Validator == nil || candidate.Validation == nil || !digest(descriptor.ValidationSHA256) {
		return ErrValidatorUnavailable
	}
	validationHash, err := matrixproof.RequestDigest(candidate.Validation)
	if err != nil || validationHash != descriptor.ValidationSHA256 {
		return ErrInvalid
	}
	var certificate ValidationCertificate
	if err := r.record(ctx, descriptor.ValidationSHA256, &certificate); err != nil {
		return err
	}
	validatorCap, err := r.capability(ctx, descriptor.Validator.Capability, policy.Epoch, policyHash, members)
	if err != nil {
		return err
	}
	validator, err := r.signer(ctx, descriptor.Validator.Device)
	if err != nil {
		return err
	}
	return verifyValidationTranscript(ctx, &certificate, publicationHash, policy.Epoch, validatorCap.value, validatorCap.hash, validatorCap.member, validatorCap.enrollment, validator.Identity(), validator.VerifyJSON)
}

//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func restoreHistoricalPublication(ctx context.Context, descriptor Reservation, hash string, candidate Candidate, source PublicEvidenceSource) (historicalPublication, error) {
	resource := descriptor.Resource
	if !descriptor.valid() || !digest(hash) || source == nil {
		return historicalPublication{}, ErrInvalid
	}
	records := reservationReader{source: source, resource: descriptor.Resource, cache: map[string]json.RawMessage{}}
	var policy SignedEpoch
	if err := records.record(ctx, descriptor.Policy.StatementSHA256, &policy); err != nil {
		return historicalPublication{}, err
	}
	policyHash, err := EpochDigest(policy.Epoch)
	if err != nil || policyHash != descriptor.PolicySHA256 || policy.Resource != resource || policy.ID != descriptor.Epoch || policy.Generation != descriptor.Generation || policy.MembershipSHA256 != descriptor.MembershipSHA256 {
		return historicalPublication{}, ErrInvalid
	}
	if _, err := records.statement(ctx, descriptor.Policy, &policy, policy.Issuer, descriptor.Policy.Signer.Pin.Scope, policy.RoomID); err != nil {
		return historicalPublication{}, err
	}
	var members []Member
	if err := records.record(ctx, descriptor.MembersSHA256, &members); err != nil {
		return historicalPublication{}, err
	}
	membershipHash, err := MembershipDigest(resource, members)
	if err != nil || membershipHash != policy.MembershipSHA256 {
		return historicalPublication{}, ErrInvalid
	}
	capability, err := records.capability(ctx, descriptor.Capability, policy.Epoch, policyHash, members)
	if err != nil {
		return historicalPublication{}, err
	}
	producer, err := records.signer(ctx, descriptor.Producer)
	if err != nil {
		return historicalPublication{}, err
	}
	value, publicationHash, coreHash, err := verifyProducerTranscript(ctx, candidate, policy.Epoch, policyHash, capability.value, capability.hash, capability.member, capability.enrollment, candidate.Proof.Action, producer.Identity(), producer.VerifyJSON, func(ctx context.Context, raw []byte, expected sessionproof.CertificateExpectation) (sessionproof.VerifiedCertificate, error) {
		return sessionproof.VerifyHistoricalCertificate(ctx, raw, expected, producer)
	})
	if err != nil || publicationHash != descriptor.PublicationSHA256 || coreHash != descriptor.CandidateSHA256 || value.Actor != descriptor.Actor || value.ExpectedBase != descriptor.Base || value.TransactionID != descriptor.AttemptID {
		return historicalPublication{}, ErrInvalid
	}
	end, err := value.End()
	if err != nil || end != descriptor.End {
		return historicalPublication{}, ErrInvalid
	}
	if err := records.verifyHistoricalValidation(ctx, descriptor, candidate, policy, policyHash, publicationHash, members); err != nil {
		return historicalPublication{}, err
	}
	if err := ctx.Err(); err != nil {
		return historicalPublication{}, err
	}
	return historicalPublication{descriptor: descriptor, hash: hash, value: value}, nil
}
