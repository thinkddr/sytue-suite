// SPDX-License-Identifier: Apache-2.0
package sessionproof

import (
	"bytes"
	"encoding/json"
	"io"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// DecodeCertificateOrigin is the single public chain codec for native and Wasm
// conformance. Decoding does NOT verify activity, trust a root, or authorize any
// resource. The session broker supplies independently resolved authority fields.
func DecodeCertificateOrigin(raw []byte) (matrixproof.Origin, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return matrixproof.Origin{}, ErrProof
	}
	var input struct {
		Scope         storage.Scope         `json:"scope"`
		ServerName    string                `json:"server_name"`
		UserID        id.UserID             `json:"user_id"`
		DeviceID      id.DeviceID           `json:"device_id"`
		AccountActive bool                  `json:"account_active"`
		DeviceActive  bool                  `json:"device_active"`
		Keys          mautrix.RespQueryKeys `json:"keys"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		return matrixproof.Origin{}, ErrProof
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return matrixproof.Origin{}, ErrProof
	}
	canonical, err := matrixproof.CanonicalBytes(json.RawMessage(raw))
	clear(canonical)
	if err != nil {
		return matrixproof.Origin{}, ErrProof
	}
	return matrixproof.Origin{Scope: input.Scope, ServerName: input.ServerName, UserID: input.UserID, DeviceID: input.DeviceID, AccountActive: input.AccountActive, DeviceActive: input.DeviceActive, Keys: input.Keys}, nil
}
