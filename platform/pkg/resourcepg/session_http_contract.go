// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// SessionOriginRequest selects a public current enrollment at a fixed service.
// It carries no roots, URLs, SDK tokens, grants or activity assertions.
type SessionOriginRequest struct {
	Version  int           `json:"v"`
	Scope    storage.Scope `json:"scope"`
	DeviceID string        `json:"device_id"`
	Nonce    string        `json:"nonce"`
}

func (q SessionOriginRequest) ValidForService(serviceID string) bool {
	return q.Version == 1 && q.Scope.Valid() && resource.OriginServiceID(q.Scope.Product) == serviceID && resource.OriginServiceID(serviceID) == serviceID && serviceID != "" && resource.ValidDeviceID(q.DeviceID) && httpx.ValidMetadataNonce(q.Nonce)
}

// NewSessionOriginRequest is called for EVERY lookup and binding check. Never
// satisfy a failed request with an old response or a retained nonce.
func NewSessionOriginRequest(scope storage.Scope, device string) (SessionOriginRequest, error) {
	q := SessionOriginRequest{Version: 1, Scope: scope, DeviceID: device}
	var err error
	q.Nonce, err = httpx.FreshMetadataNonce()
	if err != nil {
		return SessionOriginRequest{}, err
	}
	if !q.ValidForService(resource.OriginServiceID(scope.Product)) {
		return SessionOriginRequest{}, resource.ErrInvalid
	}
	return q, nil
}

// SessionOriginResponse is independently fetched only from configured Rooms.
// It is public input to the EXISTING SessionProvider and SDK verifier, never a
// decoded VerifiedSession or a recipient grant. Its MAC binds all evidence and
// the fresh complete request, not just the nonce.
type SessionOriginResponse struct {
	Version int                    `json:"v"`
	Record  resource.SessionRecord `json:"record"`
	Origin  matrixproof.Origin     `json:"origin"`
	Nonce   string                 `json:"nonce"`
	MAC     string                 `json:"mac"`
}

func ValidSessionServiceSecret(secret string) bool { return httpx.ValidMetadataSecret(secret) }

func sessionOriginTranscript(q SessionOriginRequest, r SessionOriginResponse) (SessionOriginResponse, error) {
	b := r.Record.Binding
	if !q.ValidForService(resource.OriginServiceID(q.Scope.Product)) || r.Version != 1 || r.Nonce != q.Nonce || !b.Valid() || b.Scope != q.Scope || b.DeviceID != q.DeviceID || r.Record.PinnedRoot != b.RootKey || r.Record.RootGeneration != b.RootGeneration || r.Origin.Scope != q.Scope || r.Origin.DeviceID.String() != q.DeviceID || r.Origin.UserID.String() != b.UserID || !r.Origin.AccountActive || !r.Origin.DeviceActive || len(r.Origin.Keys.Failures) != 0 {
		return SessionOriginResponse{}, resource.ErrInvalid
	}
	r.MAC = ""
	return r, nil
}

// Signing public liveness evidence does not replace the consuming provider's
// independent registry pin, consumed consent or maintained SDK verification.
func SignSessionOriginResponse(secret string, q SessionOriginRequest, r SessionOriginResponse) (SessionOriginResponse, error) {
	r.Nonce = q.Nonce
	value, err := sessionOriginTranscript(q, r)
	if err != nil {
		return SessionOriginResponse{}, err
	}
	r.MAC, err = httpx.MetadataMAC(secret, "sytue-resource-session-origin-v1", q, value)
	if err != nil {
		return SessionOriginResponse{}, err
	}
	return r, nil
}

func VerifySessionOriginResponse(secret string, q SessionOriginRequest, r SessionOriginResponse) error {
	value, err := sessionOriginTranscript(q, r)
	if err != nil {
		return err
	}
	return httpx.VerifyMetadataMAC(secret, "sytue-resource-session-origin-v1", q, value, r.MAC)
}
