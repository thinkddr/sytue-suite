// SPDX-License-Identifier: Apache-2.0
package sessionproof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	resource "git.sytue.eu/sytue/one/apps/nucleo/resource/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

const (
	CertificateKind     = "org.sytue.matrix.session.certificate.v1"
	MaxCertificateBytes = 32 << 10
)

// CertificateExpectation is established by the verified session provider and a
// pinned resource policy. It MUST NOT be copied from a certificate, backup,
// message's unsigned fields, or a server's claim about a sender/root.
// Scope identifies the sender's product broker, not the reader's personal vault.
type CertificateExpectation struct {
	Scope        storage.Scope     `json:"scope"`
	Resource     resource.Resource `json:"resource"`
	Epoch        string            `json:"epoch"`
	Generation   resource.Decimal  `json:"generation"`
	PolicySHA256 string            `json:"policy_sha256"`
	Session      Session           `json:"session"`
	DeviceID     string            `json:"device_id"`
	RootKey      string            `json:"root_key"`
}

func (e CertificateExpectation) Valid() bool {
	generation, err := e.Generation.Int64()
	target := storage.PublicationTarget{RoomID: e.Session.RoomID, DeviceID: e.DeviceID}
	hash, hashErr := hex.DecodeString(e.PolicySHA256)
	return e.Scope.Valid() && e.Resource.Valid() && e.Scope.Product == e.Resource.Product && e.Scope.Tenant == e.Resource.Tenant && storage.Identifier(e.Epoch) && err == nil && generation > 0 && hashErr == nil && len(hash) == 32 && hex.EncodeToString(hash) == e.PolicySHA256 && e.Session.Valid() && target.Valid() && key(e.RootKey)
}

type Certificate struct {
	Version int    `json:"v"`
	Kind    string `json:"kind"`
	CertificateExpectation
	Signatures signatures.Signatures `json:"signatures,omitempty"`
}

func (c Certificate) Valid() bool {
	return c.Version == 1 && c.Kind == CertificateKind && c.CertificateExpectation.Valid()
}

// VerifiedCertificate cannot be constructed from JSON. Only verification of an
// independently resolved live SDK device against an already pinned account root
// can produce a usable value. It remains scoped to one resource/epoch/session.
type VerifiedCertificate struct {
	expected CertificateExpectation
	id       string
}

func (v VerifiedCertificate) Valid() bool { return v.id != "" && v.expected.Valid() }
func (v VerifiedCertificate) ID() string  { return v.id }
func (v VerifiedCertificate) Expectation() (CertificateExpectation, error) {
	if !v.Valid() {
		return CertificateExpectation{}, ErrProof
	}
	return v.expected, nil
}

// EncodeCertificate returns the canonical unsigned Matrix JSON transcript.
// Maintained SDK signJSON must sign this object; publication must ACK the signed
// public certificate before sending the corresponding encrypted message.
func EncodeCertificate(expected CertificateExpectation) (Encoded, error) {
	if !expected.Valid() {
		return Encoded{}, ErrProof
	}
	payload, err := matrixproof.CanonicalBytes(Certificate{Version: 1, Kind: CertificateKind, CertificateExpectation: expected})
	if err != nil {
		return Encoded{}, ErrProof
	}
	sum := sha256.Sum256(payload)
	return Encoded{ID: "sessioncert1_" + hex.EncodeToString(sum[:]), Payload: payload}, nil
}

func decodeCertificateJSON(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > MaxCertificateBytes {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return false
	}
	// Reject duplicate keys and JSON that cannot be canonicalized by the same
	// maintained SDK used for signature verification; never hand-frame a signature.
	canonical, err := matrixproof.CanonicalBytes(json.RawMessage(raw))
	clear(canonical)
	return err == nil
}

func DecodeCertificateExpectation(raw []byte) (CertificateExpectation, error) {
	var expected CertificateExpectation
	if !decodeCertificateJSON(raw, &expected) || !expected.Valid() {
		return CertificateExpectation{}, ErrProof
	}
	return expected, nil
}

func DecodeCertificate(raw []byte) (Certificate, error) {
	var certificate Certificate
	if !decodeCertificateJSON(raw, &certificate) || !certificate.Valid() {
		return Certificate{}, ErrProof
	}
	return certificate, nil
}

// VerifyCertificate accepts independently verified Device metadata, not public
// backup session fields. A valid device/root signature alone does not authorize a
// recipient or role: expected must come from an already verified policy/grant.
// Revocation checks occur at the broker before/after this operation; historical
// offline certificates need the authenticated historical policy, never a newly
// accepted root fetched opportunistically from the homeserver.
func VerifyCertificate(ctx context.Context, raw []byte, expected CertificateExpectation, device matrixproof.Device) (VerifiedCertificate, error) {
	return verifyCertificateSignature(ctx, raw, expected, device.Identity(), device.VerifyJSON)
}

func verifyCertificateSignature(ctx context.Context, raw []byte, expected CertificateExpectation, identity matrixproof.Identity, verify func(context.Context, storage.Scope, storage.PublicationTarget, any) error) (VerifiedCertificate, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedCertificate{}, err
	}
	certificate, err := DecodeCertificate(raw)
	if err != nil || !expected.Valid() || certificate.CertificateExpectation != expected {
		return VerifiedCertificate{}, ErrProof
	}
	if identity.Scope != expected.Scope || identity.UserID.String() != expected.Session.SenderUser || identity.DeviceID.String() != expected.DeviceID || identity.RootKey.String() != expected.RootKey || identity.SigningKey.String() != expected.Session.SigningKey || identity.EncryptionKey.String() != expected.Session.SenderKey {
		return VerifiedCertificate{}, ErrProof
	}
	target := storage.PublicationTarget{RoomID: expected.Session.RoomID, DeviceID: expected.DeviceID}
	if err := verify(ctx, expected.Scope, target, certificate); err != nil {
		return VerifiedCertificate{}, ErrProof
	}
	encoded, err := EncodeCertificate(expected)
	if err != nil {
		return VerifiedCertificate{}, err
	}
	clear(encoded.Payload)
	return VerifiedCertificate{expected: expected, id: encoded.ID}, nil
}

// VerifyCertificateOrigin resolves the maintained SDK chain against the caller's
// independently pinned expected root/user. Origin must be fetched independently
// and its account/device activity fenced by the trusted session provider.
func VerifyCertificateOrigin(ctx context.Context, raw []byte, expected CertificateExpectation, origin matrixproof.Origin) (VerifiedCertificate, error) {
	if !expected.Valid() || origin.Scope != expected.Scope || origin.DeviceID.String() != expected.DeviceID {
		return VerifiedCertificate{}, ErrProof
	}
	device, err := matrixproof.VerifyUser(ctx, origin, id.UserID(expected.Session.SenderUser), id.Ed25519(expected.RootKey))
	if err != nil {
		return VerifiedCertificate{}, err
	}
	return VerifyCertificate(ctx, raw, expected, device)
}
