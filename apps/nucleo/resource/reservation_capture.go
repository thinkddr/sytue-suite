// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sort"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// EvidenceRecord is canonical public Go/SDK JSON. SHA256 is the digest of the
// exact Value bytes, so a database can enforce digest(value,'sha256') directly.
// No body, file descriptor, private root or epoch identity belongs here.
type EvidenceRecord struct {
	SHA256 string          `json:"sha256"`
	Value  json.RawMessage `json:"value"`
}
type SignerEvidence struct {
	DeviceSHA256 string               `json:"device_sha256"`
	Pin          matrixproof.Identity `json:"pin"`
}
type StatementEvidence struct {
	StatementSHA256 string         `json:"statement_sha256"`
	Signer          SignerEvidence `json:"signer"`
}
type CapabilityEvidence struct {
	Capability       StatementEvidence `json:"capability"`
	Recipient        StatementEvidence `json:"recipient"`
	IssuerEnrollment StatementEvidence `json:"issuer_enrollment"`
}
type ValidatorReservation struct {
	Capability CapabilityEvidence `json:"capability"`
	Device     SignerEvidence     `json:"device"`
}
type Reservation struct {
	Version           int                   `json:"v"`
	Domain            string                `json:"domain"`
	Resource          Resource              `json:"resource"`
	OpID              string                `json:"op_id"`
	AttemptID         string                `json:"attempt_id"`
	Epoch             string                `json:"epoch"`
	Generation        Decimal               `json:"generation"`
	PolicySHA256      string                `json:"policy_sha256"`
	MembershipSHA256  string                `json:"membership_sha256"`
	CandidateSHA256   string                `json:"candidate_sha256"`
	PublicationSHA256 string                `json:"publication_sha256"`
	ValidationSHA256  string                `json:"validation_sha256,omitempty"`
	Actor             Actor                 `json:"actor"`
	Base              Position              `json:"base"`
	End               Decimal               `json:"end"`
	Policy            StatementEvidence     `json:"policy"`
	MembersSHA256     string                `json:"members_sha256"`
	Capability        CapabilityEvidence    `json:"capability"`
	Producer          SignerEvidence        `json:"producer"`
	Validator         *ValidatorReservation `json:"validator,omitempty"`
}

func (r Reservation) valid() bool {
	generation, err := r.Generation.Int64()
	base, e1 := r.Base.Seq.Int64()
	end, e2 := r.End.Int64()
	return r.Version == 1 && r.Domain == "org.sytue.resource.reservation.v1" && r.Resource.Valid() && storage.Identifier(r.OpID) && canonicalTransaction(r.AttemptID) && storage.Identifier(r.Epoch) && err == nil && generation > 0 && digest(r.PolicySHA256) && digest(r.MembershipSHA256) && digest(r.CandidateSHA256) && digest(r.PublicationSHA256) && (r.ValidationSHA256 == "" || digest(r.ValidationSHA256)) && r.Actor.Valid() && r.Base.Valid() && e1 == nil && e2 == nil && end > base && digest(r.MembersSHA256)
}

// CapturedReservation can only come from a fully verified publication. Acquire
// atomically persists Bytes/SHA256 plus the immutable Sending hold and upserts
// Records by digest. Proof blobs are shared by hash, not copied for every op.
type CapturedReservation struct {
	descriptor Reservation
	raw        json.RawMessage
	hash       string
	records    []EvidenceRecord
}

func (c CapturedReservation) Value() Reservation { return cloneReservation(c.descriptor) }
func (c CapturedReservation) Bytes() []byte      { return bytes.Clone(c.raw) }
func (c CapturedReservation) SHA256() string     { return c.hash }
func (c CapturedReservation) Records() []EvidenceRecord {
	out := make([]EvidenceRecord, len(c.records))
	for i, r := range c.records {
		out[i] = EvidenceRecord{SHA256: r.SHA256, Value: bytes.Clone(r.Value)}
	}
	return out
}

func cloneReservation(r Reservation) Reservation {
	if r.Validator != nil {
		copied := *r.Validator
		r.Validator = &copied
	}
	return r
}

type recordCollector map[string]json.RawMessage

func (r recordCollector) records() []EvidenceRecord {
	out := make([]EvidenceRecord, 0, len(r))
	for hash, raw := range r {
		out = append(out, EvidenceRecord{SHA256: hash, Value: bytes.Clone(raw)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA256 < out[j].SHA256 })
	return out
}

func (r recordCollector) add(value any) (string, error) {
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return "", err
	}
	hash, err := CipherJSONDigest(raw)
	if err != nil {
		clear(raw)
		return "", err
	}
	if previous, ok := r[hash]; ok {
		if !bytes.Equal(previous, raw) {
			clear(raw)
			return "", ErrFork
		}
		clear(raw)
		return hash, nil
	}
	r[hash] = raw
	return hash, nil
}

func (r recordCollector) signer(device publicEvidenceVerifier) (SignerEvidence, error) {
	if device == nil {
		return SignerEvidence{}, ErrInvalid
	}
	public, err := device.PublicEvidence()
	if err != nil {
		return SignerEvidence{}, err
	}
	hash, err := r.add(public)
	if err != nil {
		return SignerEvidence{}, err
	}
	return SignerEvidence{DeviceSHA256: hash, Pin: device.Identity()}, nil
}

func (r recordCollector) statement(raw json.RawMessage, device publicEvidenceVerifier) (StatementEvidence, error) {
	if len(raw) == 0 {
		return StatementEvidence{}, ErrInvalid
	}
	hash, err := r.add(raw)
	if err != nil {
		return StatementEvidence{}, err
	}
	signer, err := r.signer(device)
	if err != nil {
		return StatementEvidence{}, err
	}
	return StatementEvidence{StatementSHA256: hash, Signer: signer}, nil
}

func (r recordCollector) recipient(value VerifiedRecipient) (StatementEvidence, error) {
	if !value.valid() {
		return StatementEvidence{}, ErrInvalid
	}
	return r.statement(value.proof, value.device)
}

func (r recordCollector) capability(value VerifiedCapability, epoch VerifiedEpoch) (CapabilityEvidence, error) {
	if !value.valid() || value.epochHash != epoch.hash {
		return CapabilityEvidence{}, ErrInvalid
	}
	statement, err := r.statement(value.proof, value.device)
	if err != nil {
		return CapabilityEvidence{}, err
	}
	recipient, ok := epoch.roster.recipients[value.member.EnrollmentSHA256]
	if !ok {
		return CapabilityEvidence{}, ErrInvalid
	}
	recipientStatement, err := r.recipient(recipient)
	if err != nil {
		return CapabilityEvidence{}, err
	}
	identity := value.device.Identity()
	var manager VerifiedRecipient
	found := false
	for _, member := range epoch.roster.members {
		if member.Scope != identity.Scope || !slices.Contains(member.Actions, Manage) {
			continue
		}
		candidate, ok := epoch.roster.recipients[member.EnrollmentSHA256]
		if ok && identityMatches(identity, candidate.enrollment.Issuer, member.Scope) {
			manager = candidate
			found = true
			break
		}
	}
	if !found {
		return CapabilityEvidence{}, ErrInvalid
	}
	managerStatement, err := r.recipient(manager)
	if err != nil {
		return CapabilityEvidence{}, err
	}
	return CapabilityEvidence{Capability: statement, Recipient: recipientStatement, IssuerEnrollment: managerStatement}, nil
}

func (r recordCollector) validatorReservation(certificate *ValidationCertificate, validator *ValidationEvidence, epoch VerifiedEpoch) (string, *ValidatorReservation, error) {
	if certificate == nil {
		return "", nil, nil
	}
	validationHash, err := r.add(certificate)
	if err != nil {
		return "", nil, err
	}
	if validator == nil {
		return "", nil, ErrValidatorUnavailable
	}
	validationCap, err := r.capability(validator.Access.capability, epoch)
	if err != nil {
		return "", nil, err
	}
	validationSigner, err := r.signer(validator.Device)
	if err != nil {
		return "", nil, err
	}
	return validationHash, &ValidatorReservation{Capability: validationCap, Device: validationSigner}, nil
}

// CaptureReservation runs before Acquire and first HTTP. All inputs are already
// cryptographically verified, and the server still fences ACL/device/generation
// inside Acquire. It does not assert that any Matrix event has been accepted.
//
//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func CaptureReservation(ctx context.Context, candidate Candidate, publication VerifiedPublication, attemptID string) (CapturedReservation, error) {
	if err := ctx.Err(); err != nil {
		return CapturedReservation{}, err
	}
	if !canonicalTransaction(attemptID) || candidate.Proof.TransactionID != attemptID || !publication.pending.epoch.valid() || !digest(publication.SHA256()) {
		return CapturedReservation{}, ErrInvalid
	}
	coreHash, err := CandidateDigest(candidate)
	if err != nil || coreHash != publication.CandidateSHA256() {
		return CapturedReservation{}, ErrInvalid
	}
	if publication.pending.epoch.value.Validation.PrivateRules || candidate.Validation != nil {
		if err := verifyValidation(ctx, candidate.Validation, publication.SHA256(), publication.pending.epoch, publication.validator); err != nil {
			return CapturedReservation{}, err
		}
	}
	records := recordCollector{}
	epoch := publication.pending.epoch
	policy, err := records.statement(epoch.proof, epoch.device)
	if err != nil {
		return CapturedReservation{}, err
	}
	members := epoch.roster.recordHash
	if !digest(members) || len(epoch.roster.record) == 0 {
		return CapturedReservation{}, ErrInvalid
	}
	records[members] = epoch.roster.record
	capability, err := records.capability(publication.pending.access.capability, epoch)
	if err != nil {
		return CapturedReservation{}, err
	}
	producer, err := records.signer(publication.pending.producer)
	if err != nil {
		return CapturedReservation{}, err
	}
	value := publication.Value()
	end, err := value.End()
	if err != nil {
		return CapturedReservation{}, err
	}
	descriptor := Reservation{Version: 1, Domain: "org.sytue.resource.reservation.v1", Resource: value.Resource, OpID: value.OpID, AttemptID: attemptID, Epoch: value.Epoch, Generation: value.Generation, PolicySHA256: value.PolicySHA256, MembershipSHA256: value.MembershipSHA256, CandidateSHA256: coreHash, PublicationSHA256: publication.SHA256(), Actor: value.Actor, Base: value.ExpectedBase, End: end, Policy: policy, MembersSHA256: members, Capability: capability, Producer: producer}
	descriptor.ValidationSHA256, descriptor.Validator, err = records.validatorReservation(candidate.Validation, publication.validator, epoch)
	if err != nil {
		return CapturedReservation{}, err
	}
	if !descriptor.valid() {
		return CapturedReservation{}, ErrInvalid
	}
	raw, err := matrixproof.CanonicalBytes(descriptor)
	if err != nil {
		return CapturedReservation{}, err
	}
	hash, err := CipherJSONDigest(raw)
	if err != nil {
		clear(raw)
		return CapturedReservation{}, err
	}
	list := make([]EvidenceRecord, 0, len(records))
	for hash, raw := range records {
		list = append(list, EvidenceRecord{SHA256: hash, Value: raw})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SHA256 < list[j].SHA256 })
	return CapturedReservation{descriptor: descriptor, raw: raw, hash: hash, records: list}, nil
}

func (c CapturedReservation) Valid() bool {
	return c.descriptor.valid() && digest(c.hash) && len(c.raw) > 0
}

// EvidenceHashes lets Acquire query existing public records in one transaction
// before copying only missing records. A large immutable roster is not sent or
// canonicalized again for every publication.
func (c CapturedReservation) EvidenceHashes() []string { return evidenceHashes(c.records) }

func (c CapturedReservation) Evidence(hash string) (EvidenceRecord, error) {
	return evidenceRecord(c.records, hash)
}

func evidenceHashes(records []EvidenceRecord) []string {
	out := make([]string, len(records))
	for i, record := range records {
		out[i] = record.SHA256
	}
	return out
}

func evidenceRecord(records []EvidenceRecord, hash string) (EvidenceRecord, error) {
	for _, record := range records {
		if record.SHA256 == hash {
			return EvidenceRecord{SHA256: hash, Value: bytes.Clone(record.Value)}, nil
		}
	}
	return EvidenceRecord{}, ErrInvalid
}
