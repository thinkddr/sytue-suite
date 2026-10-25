// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package logsafe encodes untrusted text before it reaches a logging sink.
package logsafe

import (
	"fmt"
	"strings"
)

// Text keeps a value readable while preventing an input from adding log lines.
// Structured loggers still handle field encoding and redaction independently.
func Text(value any) string {
	text := fmt.Sprint(value)
	text = strings.ReplaceAll(text, "\r", "\\r")
	return strings.ReplaceAll(text, "\n", "\\n")
}
