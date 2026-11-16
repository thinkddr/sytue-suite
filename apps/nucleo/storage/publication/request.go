// SPDX-License-Identifier: Apache-2.0
// Package publication binds opaque Matrix writes to durable server admission.
// It contains only public routing, ciphertext commitments and SDK signatures.
package publication

import (
	"context"
	"encoding/base64"
	"encoding/hex"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

const MarkerName = "org.sytue.publication.admission"

// Request is signed by the maintained SDK after encryption and backup.
// Namespace is selected by the broker; a caller's field cannot enroll a device.
type Request struct {
	Version             int                       `json:"v"`
	Domain              string                    `json:"domain"`
	Namespace           string                    `json:"namespace"`
	Scope               storage.Scope             `json:"scope"`
	Target              storage.PublicationTarget `json:"target"`
	TransactionID       string                    `json:"transaction_id"`
	CipherSHA256        string                    `json:"cipher_sha256"`
	AuthorityGeneration int64                     `json:"authority_generation,string"`
}

func validDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

func ValidNamespace(scope storage.Scope, namespace string) bool {
	return scope.Valid() && (namespace == "storage:"+scope.Product || namespace == "resources:"+scope.Product)
}

func (r Request) Valid() bool {
	if r.Version != 1 || r.Domain != "org.sytue.publication.admission.v1" || !ValidNamespace(r.Scope, r.Namespace) || !r.Target.Valid() || !validDigest(r.CipherSHA256) || r.AuthorityGeneration < 1 || !validTransactionID(r.TransactionID) {
		return false
	}
	return true
}

func validTransactionID(value string) bool {
	if len(value) < 1 || len(value) > 255 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

type SignedRequest struct {
	Request
	Signatures signatures.Signatures `json:"signatures"`
}

// VerifiedRequest proves a live SDK device signed the exact retained request.
// The server must still resolve namespace, current role, root generation and
// operation policy independently, then COMMIT its hold before allowing IO.
type VerifiedRequest struct {
	request  Request
	sha      string
	evidence matrixproof.DeviceEvidence
}

func VerifyRequest(ctx context.Context, signed SignedRequest, expected Request, device matrixproof.Device) (VerifiedRequest, error) {
	if !expected.Valid() || signed.Request != expected {
		return VerifiedRequest{}, matrixproof.ErrUnverified
	}
	if err := device.VerifyJSON(ctx, expected.Scope, expected.Target, signed); err != nil {
		return VerifiedRequest{}, err
	}
	sha, err := matrixproof.RequestDigest(expected)
	if err != nil {
		return VerifiedRequest{}, err
	}
	evidence, err := device.PublicEvidence()
	if err != nil {
		return VerifiedRequest{}, err
	}
	return VerifiedRequest{request: expected, sha: sha, evidence: evidence}, nil
}
func (v VerifiedRequest) Value() Request { return v.request }
func (v VerifiedRequest) SHA256() string { return v.sha }
func (v VerifiedRequest) PublicEvidence() (matrixproof.DeviceEvidence, error) {
	if !v.request.Valid() || !validDigest(v.sha) {
		return matrixproof.DeviceEvidence{}, matrixproof.ErrUnverified
	}
	// Return a detached copy of the public evidence captured during live verification.
	return clonePublicEvidence(v.evidence)
}

// Marker is public and opaque. It conveys no permission without the server's
// exact ciphertext/producer/namespace hold and its one-publication state.
type Marker struct {
	Version int    `json:"v"`
	ID      string `json:"id"`
}

func (m Marker) Valid() bool {
	raw, err := base64.RawURLEncoding.DecodeString(m.ID)
	return m.Version == 1 && err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == m.ID
}
