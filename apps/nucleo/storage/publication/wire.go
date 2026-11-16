// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"encoding/json"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

var ErrInvalid = errors.New("publication: invalid ciphertext admission")

func clonePublicEvidence(e matrixproof.DeviceEvidence) (matrixproof.DeviceEvidence, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return matrixproof.DeviceEvidence{}, err
	}
	var cloned matrixproof.DeviceEvidence
	err = json.Unmarshal(raw, &cloned)
	return cloned, err
}

// unmarkedWire is the one parser used by digests, normalized journal cores and
// marker attachment. It strips only the exact validated opaque admission marker.
func unmarkedWire(raw []byte, requireMarker bool) (map[string]json.RawMessage, Marker, error) {
	if len(raw) == 0 || len(raw) > 65536 {
		return nil, Marker{}, ErrInvalid
	}
	// The maintained SDK rejects duplicate keys and noncanonical Matrix numbers.
	// Parsing to a map first would silently erase a duplicate field.
	checked, err := matrixproof.CanonicalBytes(json.RawMessage(raw))
	if err != nil {
		return nil, Marker{}, ErrInvalid
	}
	defer clear(checked)
	var content map[string]json.RawMessage
	if json.Unmarshal(checked, &content) != nil || content == nil {
		return nil, Marker{}, ErrInvalid
	}
	var algorithm, ciphertext, session string
	if json.Unmarshal(content["algorithm"], &algorithm) != nil || algorithm != "m.megolm.v1.aes-sha2" || json.Unmarshal(content["ciphertext"], &ciphertext) != nil || ciphertext == "" || json.Unmarshal(content["session_id"], &session) != nil || session == "" {
		return nil, Marker{}, ErrInvalid
	}
	var marker Marker
	if value, ok := content[MarkerName]; ok {
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil || len(fields) != 2 || json.Unmarshal(value, &marker) != nil || !marker.Valid() {
			return nil, Marker{}, ErrInvalid
		}
		if _, ok = fields["v"]; !ok {
			return nil, Marker{}, ErrInvalid
		}
		if _, ok = fields["id"]; !ok {
			return nil, Marker{}, ErrInvalid
		}
		delete(content, MarkerName)
	} else if requireMarker {
		return nil, Marker{}, ErrInvalid
	}
	return content, marker, nil
}

// CanonicalWire returns the SDK's complete encrypted content without only the
// exact admission marker. It is also the stable immutable candidate journal core.
func CanonicalWire(raw []byte, requireMarker bool) ([]byte, Marker, error) {
	content, marker, err := unmarkedWire(raw, requireMarker)
	if err != nil {
		return nil, Marker{}, err
	}
	canonical, err := matrixproof.CanonicalBytes(content)
	if err != nil {
		return nil, Marker{}, ErrInvalid
	}
	return canonical, marker, nil
}

// WireDigest covers the exact SDK ciphertext independently of the marker issued
// after durable admission. Native and browser sign the same maintained Go wire.
func WireDigest(raw []byte, requireMarker bool) (string, Marker, error) {
	content, marker, err := unmarkedWire(raw, requireMarker)
	if err != nil {
		return "", Marker{}, err
	}
	hash, err := matrixproof.RequestDigest(content)
	if err != nil {
		return "", Marker{}, ErrInvalid
	}
	return hash, marker, nil
}

// AttachMarker is idempotent for the same durable admission. Replacing an
// existing marker with another attempt is forbidden, including during retries.
func AttachMarker(raw []byte, marker Marker) ([]byte, error) {
	if !marker.Valid() {
		return nil, ErrInvalid
	}
	content, previous, err := unmarkedWire(raw, false)
	if err != nil {
		return nil, err
	}
	if previous.Valid() && previous != marker {
		return nil, ErrInvalid
	}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return nil, ErrInvalid
	}
	content[MarkerName] = encoded
	output, err := matrixproof.CanonicalBytes(content)
	if err != nil || len(output) > 65536 {
		clear(output)
		return nil, ErrInvalid
	}
	return output, nil
}
