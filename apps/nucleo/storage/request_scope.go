// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"errors"
	"strings"
)

var ErrRequestScopeMismatch = errors.New("storage: request scope differs from authenticated scope")

// ValidateRequestScope compares public job routing with the server's separately
// authenticated identity. It rejects missing and repeated headers, including
// maps assembled with different key casing, before callers touch a data source.
func ValidateRequestScope(header map[string][]string, expected Scope) error {
	if !expected.Valid() {
		return ErrRecord
	}
	var values []string
	for key, entry := range header {
		if strings.EqualFold(key, ScopeHeader) {
			values = append(values, entry...)
		}
	}
	if len(values) != 1 {
		return ErrRecord
	}
	actual, err := DecodeScopeHeader(values[0])
	if err != nil {
		return ErrRecord
	}
	if actual != expected {
		return ErrRequestScopeMismatch
	}
	return nil
}
