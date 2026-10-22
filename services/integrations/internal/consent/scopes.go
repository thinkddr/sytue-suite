// SPDX-License-Identifier: Apache-2.0
package consent

import (
	"encoding/json"
	"strings"
)

// ParseScopes conserva la diferencia entre un scope omitido (RFC 6749 §5.1) y
// uno concedido explícitamente vacío. Algunos proveedores devuelven una lista.
func ParseScopes(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var text string
	if string(raw) != "null" && json.Unmarshal(raw, &text) == nil {
		return append([]string{}, strings.Fields(text)...), nil
	}
	var list []string
	if json.Unmarshal(raw, &list) != nil || list == nil {
		return nil, ErrProvider
	}
	return list, nil
}
