// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"bytes"
	"encoding/json"
	"io"
)

// Control replies stay raw until Go enforces their exact schema and size.
func decodeControlJSON(raw []byte, maximum int, out any) error {
	if len(raw) == 0 || len(raw) > maximum {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
