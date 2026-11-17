// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"strings"
	"unicode/utf8"
)

// ValidDriveName is the same byte-based rule in the legacy service and the
// private client repository. It is deliberately not the rule for Rooms channels.
func ValidDriveName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 || !utf8.ValidString(name) || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}
