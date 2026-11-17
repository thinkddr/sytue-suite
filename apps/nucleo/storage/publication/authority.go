// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// DecodeActiveAuthority parses raw response bytes without a JavaScript number
// round-trip. It preserves Authority's existing bounds and never enrolls a root;
// the caller must also match the SDK's independently trusted own identity.
func DecodeActiveAuthority(raw []byte, scope storage.Scope) (matrixproof.Authority, error) {
	var value matrixproof.Authority
	if len(raw) == 0 || len(raw) > 16384 || json.Unmarshal(raw, &value) != nil || value.Validate(scope) != nil || value.Status != "active" {
		return matrixproof.Authority{}, ErrInvalid
	}
	return value, nil
}
