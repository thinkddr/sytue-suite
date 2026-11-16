// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// BootstrapPublication has its own pre-policy semantic domain. It never lies
// about a final policy hash, operation sequence or already accepted baseline.
// ContextSHA is the Go BootstrapContext commitment used by its SDK certificate.
type BootstrapPublication struct {
	Version              int           `json:"v"`
	Domain               string        `json:"domain"`
	PlanSHA256           string        `json:"plan_sha256"`
	Kind                 BootstrapKind `json:"kind"`
	TransactionID        string        `json:"transaction_id"`
	ContextSHA256        string        `json:"context_sha256"`
	InnerCipherSHA256    string        `json:"inner_cipher_sha256"`
	OuterCipherSHA256    string        `json:"outer_cipher_sha256"`
	SessionCertificateID string        `json:"session_certificate_id"`
	Actor                Actor         `json:"actor"`
}

func (p BootstrapPublication) Valid() bool {
	return p.Version == 1 && p.Domain == "org.sytue.resource.bootstrap.publication.v1" && digest(p.PlanSHA256) && (p.Kind == BootstrapBaseline || p.Kind == BootstrapCheckpoint) && canonicalTransaction(p.TransactionID) && digest(p.ContextSHA256) && digest(p.InnerCipherSHA256) && digest(p.OuterCipherSHA256) && storage.Identifier(p.SessionCertificateID) && p.Actor.Valid()
}

type SignedBootstrapPublication struct {
	BootstrapPublication
	Signatures signatures.Signatures `json:"signatures"`
}
type BootstrapCandidate struct {
	Proof              SignedBootstrapPublication `json:"proof"`
	EncryptedEvent     json.RawMessage            `json:"encrypted_event"`
	SessionCertificate json.RawMessage            `json:"session_certificate"`
}

func BootstrapCandidateSHA256(c BootstrapCandidate) (string, error) {
	if !c.Proof.Valid() || !json.Valid(c.SessionCertificate) {
		return "", ErrInvalid
	}
	if err := checkSDKCiphertext(c.EncryptedEvent, c.Proof.Actor, c.Proof.OuterCipherSHA256); err != nil {
		return "", err
	}
	wire, _, err := publication.CanonicalWire(c.EncryptedEvent, false)
	if err != nil {
		return "", err
	}
	defer clear(wire)
	return matrixproof.RequestDigest(struct {
		Proof              SignedBootstrapPublication `json:"proof"`
		EncryptedEvent     json.RawMessage            `json:"encrypted_event"`
		SessionCertificate json.RawMessage            `json:"session_certificate"`
	}{c.Proof, wire, c.SessionCertificate})
}

type VerifiedBootstrapCandidate struct {
	plan    VerifiedBootstrapPlan
	value   BootstrapCandidate
	hash    string
	context BootstrapContext
}

func (c VerifiedBootstrapCandidate) SHA256() string { return c.hash }
func (c VerifiedBootstrapCandidate) Publication() BootstrapPublication {
	return c.value.Proof.BootstrapPublication
}
func (c VerifiedBootstrapCandidate) Context() BootstrapContext { return c.context }
func (c VerifiedBootstrapCandidate) valid() bool {
	return c.plan.valid() && digest(c.hash) && c.value.Proof.Valid() && c.context.Valid() && c.value.Proof.PlanSHA256 == c.plan.hash
}

func (p VerifiedBootstrapPlan) CheckpointContext(baseline VerifiedBootstrapAcceptance) (BootstrapContext, error) {
	if !p.valid() || !baseline.valid() || baseline.planHash != p.hash || baseline.kind != BootstrapBaseline {
		return BootstrapContext{}, ErrInvalid
	}
	return NewBootstrapContext(BootstrapCheckpoint, p.value.Source.Resource, p.value.RoomID, p.value.Epoch, p.value.Recipient, Position{Seq: p.value.Source.SourceSeq, HeadCipherSHA256: baseline.cipherHash}, p.value.CreatorScope, p.value.Creator)
}

// VerifyBootstrapCandidate uses original source authority instead of a made-up
// final epoch. The caller must persist its immutable slot BEFORE admission HTTP.
func VerifyBootstrapCandidate(ctx context.Context, c BootstrapCandidate, plan VerifiedBootstrapPlan, baseline *VerifiedBootstrapAcceptance) (VerifiedBootstrapCandidate, error) {
	if !plan.valid() {
		return VerifiedBootstrapCandidate{}, ErrInvalid
	}
	hash, err := BootstrapCandidateSHA256(c)
	if err != nil {
		return VerifiedBootstrapCandidate{}, err
	}
	p := c.Proof.BootstrapPublication
	if p.PlanSHA256 != plan.hash || p.Actor != plan.value.Creator {
		return VerifiedBootstrapCandidate{}, ErrInvalid
	}
	var bootstrapContext BootstrapContext
	if p.Kind == BootstrapBaseline {
		if baseline != nil {
			return VerifiedBootstrapCandidate{}, ErrInvalid
		}
		bootstrapContext, err = plan.BaselineContext()
	} else {
		if baseline == nil {
			return VerifiedBootstrapCandidate{}, ErrInvalid
		}
		bootstrapContext, err = plan.CheckpointContext(*baseline)
	}
	if err != nil {
		return VerifiedBootstrapCandidate{}, err
	}
	contextHash, err := BootstrapContextSHA256(bootstrapContext)
	if err != nil || contextHash != p.ContextSHA256 {
		return VerifiedBootstrapCandidate{}, ErrInvalid
	}
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapCandidate{}, err
	}
	device := plan.session.Device()
	target := storage.PublicationTarget{RoomID: plan.value.RoomID, DeviceID: p.Actor.DeviceID}
	if err := device.VerifyJSON(ctx, plan.value.CreatorScope, target, c.Proof); err != nil {
		return VerifiedBootstrapCandidate{}, err
	}
	identity := device.Identity()
	expected := sessionproof.CertificateExpectation{Scope: plan.value.CreatorScope, Resource: plan.value.Source.Resource, Epoch: plan.value.Epoch, Generation: "1", PolicySHA256: contextHash, Session: sessionproof.Session{RoomID: plan.value.RoomID, SenderUser: p.Actor.UserID, SigningKey: identity.SigningKey.String()}, DeviceID: p.Actor.DeviceID, RootKey: p.Actor.RootKey}
	certificate, err := verifyEncryptedSessionCertificate(ctx, c.SessionCertificate, c.EncryptedEvent, expected, func(ctx context.Context, raw []byte, expected sessionproof.CertificateExpectation) (sessionproof.VerifiedCertificate, error) {
		return sessionproof.VerifyCertificate(ctx, raw, expected, device)
	})
	if err != nil || certificate.ID() != p.SessionCertificateID {
		return VerifiedBootstrapCandidate{}, ErrInvalid
	}
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapCandidate{}, err
	}
	return VerifiedBootstrapCandidate{plan: plan, value: c, hash: hash, context: bootstrapContext}, nil
}

type VerifiedBootstrapAcceptance struct {
	planHash, candidateHash, cipherHash, certificate string
	kind                                             BootstrapKind
	context                                          BootstrapContext
	reference                                        storage.Reference
}

func (a VerifiedBootstrapAcceptance) valid() bool {
	return digest(a.planHash) && digest(a.candidateHash) && digest(a.cipherHash) && storage.Identifier(a.certificate) && a.context.Valid() && a.context.Kind == a.kind && a.reference.Valid() && a.reference.RoomID == a.context.RoomID
}
func (a VerifiedBootstrapAcceptance) Reference() storage.Reference { return a.reference }
func (a VerifiedBootstrapAcceptance) CandidateSHA256() string      { return a.candidateHash }
func (a VerifiedBootstrapAcceptance) CipherSHA256() string         { return a.cipherHash }
func (a VerifiedBootstrapAcceptance) PlanSHA256() string           { return a.planHash }
func (a VerifiedBootstrapAcceptance) Kind() BootstrapKind          { return a.kind }
func (a VerifiedBootstrapAcceptance) Baseline() (Baseline, error) {
	if !a.valid() || a.kind != BootstrapBaseline {
		return Baseline{}, ErrInvalid
	}
	hash, err := BootstrapContextSHA256(a.context)
	if err != nil {
		return Baseline{}, err
	}
	return Baseline{Position: Position{Seq: a.context.SourceCutover.Seq, HeadCipherSHA256: a.cipherHash}, Reference: a.reference, ContextSHA256: hash, SessionCertificateID: a.certificate}, nil
}

func (a VerifiedBootstrapAcceptance) Checkpoint() (Checkpoint, error) {
	if !a.valid() || a.kind != BootstrapCheckpoint {
		return Checkpoint{}, ErrInvalid
	}
	hash, err := BootstrapContextSHA256(a.context)
	if err != nil {
		return Checkpoint{}, err
	}
	return Checkpoint{SourceCutover: a.context.SourceCutover, Reference: a.reference, CipherSHA256: a.cipherHash, ContextSHA256: hash, SessionCertificateID: a.certificate}, nil
}

func ConfirmBootstrapAcceptance(ctx context.Context, candidate VerifiedBootstrapCandidate, reference storage.Reference, source AcceptanceSource) (VerifiedBootstrapAcceptance, error) {
	if !candidate.valid() || !reference.Valid() || reference.RoomID != candidate.plan.value.RoomID || source == nil {
		return VerifiedBootstrapAcceptance{}, ErrInvalid
	}
	if err := confirmAcceptedCipher(ctx, candidate.value.Proof.Actor, candidate.value.Proof.OuterCipherSHA256, reference, source); err != nil {
		return VerifiedBootstrapAcceptance{}, err
	}
	p := candidate.Publication()
	return VerifiedBootstrapAcceptance{planHash: p.PlanSHA256, candidateHash: candidate.hash, cipherHash: p.OuterCipherSHA256, certificate: p.SessionCertificateID, kind: p.Kind, context: candidate.context, reference: reference}, nil
}
