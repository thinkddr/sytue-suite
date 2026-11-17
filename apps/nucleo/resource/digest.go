// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"encoding/json"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// PublicationDigest hashes the unsigned typed publication transcript. Matrix SDK
// signatures use the same canonical JSON wire semantics; signatures themselves
// do not change the operation identity or validation certificate commitment.
func PublicationDigest(value CipherPublication) (string, error) {
	if !value.Valid() {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(value)
}

// CipherJSONDigest commits the complete SDK/age ciphertext object canonically.
// Equivalent JSON property order/whitespace cannot change a relay receipt, but
// any ciphertext, algorithm, producer or session field substitution does.
func CipherJSONDigest(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(raw)
}

// CheckCandidate verifies ciphertext integrity and the maintained SDK's event
// shape only. It does NOT verify signatures, capability/role, authority freshness
// or private protection rules; only VerifyCandidate can yield a trusted value.
func CheckCandidate(candidate Candidate) error {
	if !candidate.Valid() {
		return ErrInvalid
	}
	return checkSDKCiphertext(candidate.EncryptedEvent, candidate.Proof.Actor, candidate.Proof.OuterCipherSHA256)
}

// The pre-epoch bootstrap and normal publications use the same SDK wire gate.
func checkSDKCiphertext(raw json.RawMessage, actor Actor, expected string) error {
	hash, err := OuterCipherDigest(raw)
	if err != nil || hash != expected {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrInvalid
	}
	for name := range fields {
		switch name {
		case "algorithm", "ciphertext", "session_id", "sender_key", "device_id", "m.relates_to", "m.mentions", publication.MarkerName:
		default:
			return ErrInvalid
		}
	}
	var encrypted event.EncryptedEventContent
	if json.Unmarshal(raw, &encrypted) != nil || encrypted.Algorithm != id.AlgorithmMegolmV1 || encrypted.SessionID == "" || encrypted.SenderKey == "" || len(encrypted.MegolmCiphertext) == 0 {
		return ErrInvalid
	}
	if encrypted.DeviceID != "" && encrypted.DeviceID.String() != actor.DeviceID { //nolint:staticcheck // SA1019: legacy Matrix device_id wire check
		return ErrInvalid
	}
	return nil
}

// CandidateDigest binds the immutable producer-authenticated WAL core. A later
// validation certificate is separately verified before acquire/commit and may
// be appended without replacing an already durable encrypted candidate.
func CandidateDigest(candidate Candidate) (string, error) {
	if err := CheckCandidate(candidate); err != nil {
		return "", err
	}
	wire, _, err := publication.CanonicalWire(candidate.EncryptedEvent, false)
	if err != nil {
		return "", ErrInvalid
	}
	defer clear(wire)
	return matrixproof.RequestDigest(struct {
		Proof              SignedPublication `json:"proof"`
		EncryptedEvent     json.RawMessage   `json:"encrypted_event"`
		SessionCertificate json.RawMessage   `json:"session_certificate"`
	}{Proof: candidate.Proof, EncryptedEvent: wire, SessionCertificate: candidate.SessionCertificate})
}

// OuterCipherDigest shares the admission core wire and strips only its exact
// validated opaque marker. Generic age/evidence JSON keeps CipherJSONDigest.
func OuterCipherDigest(raw json.RawMessage) (string, error) {
	hash, _, err := publication.WireDigest(raw, false)
	if err != nil {
		return "", ErrInvalid
	}
	return hash, nil
}
