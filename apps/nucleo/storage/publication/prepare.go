// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// PreparedRequest is public canonical data, never a work permit or SDK identity.
// A private SDK factory must independently bind and sign this exact request,
// then the origin must verify and durably reserve it before any message HTTP.
type PreparedRequest struct {
	Request Request
	Payload []byte
	SHA256  string
}

// BuildRequest is shared by actual native SDK preparation and the Wasm bridge.
// It derives the commitment from the encrypted wire, not a caller-supplied hash.
func BuildRequest(scope storage.Scope, namespace string, target storage.PublicationTarget, transaction string, generation int64, wire []byte) (PreparedRequest, error) {
	hash, marker, err := WireDigest(wire, false)
	if err != nil {
		return PreparedRequest{}, err
	}
	if marker.Valid() {
		return PreparedRequest{}, ErrInvalid
	}
	request := Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: namespace, Scope: scope, Target: target, TransactionID: transaction, CipherSHA256: hash, AuthorityGeneration: generation}
	if !request.Valid() {
		return PreparedRequest{}, ErrInvalid
	}
	payload, err := matrixproof.CanonicalBytes(request)
	if err != nil {
		return PreparedRequest{}, err
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		clear(payload)
		return PreparedRequest{}, err
	}
	return PreparedRequest{Request: request, Payload: payload, SHA256: digest}, nil
}

// BuildStorageRequest checks the active isolated authority's public transcript.
// It does not trust a JSON authority as an SDK root or sign on the user's behalf.
func BuildStorageRequest(scope storage.Scope, target storage.PublicationTarget, transaction string, wire []byte, authority matrixproof.Authority) (PreparedRequest, error) {
	if authority.RequireActive(scope, authority.Generation) != nil {
		return PreparedRequest{}, ErrInvalid
	}
	return BuildRequest(scope, "storage:"+scope.Product, target, transaction, authority.Generation, wire)
}
