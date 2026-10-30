// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package logsafe encodes untrusted text before it reaches a logging sink.
package logsafe

import (
	"fmt"
	"reflect"
	"strings"
)

// Text keeps a value readable while preventing an input from adding log lines.
// Structured loggers still handle field encoding and redaction independently.
func Text(value any) string {
	text := fmt.Sprint(value)
	text = strings.ReplaceAll(text, "\r", "\\r")
	return strings.ReplaceAll(text, "\n", "\\n")
}

// Args copies a structured argument list, escaping text/errors while retaining
// numeric, boolean and other typed values for the logger's normal encoding.
func Args(values []any) []any {
	out := append([]any(nil), values...)
	for i, value := range values {
		if err, ok := value.(error); ok {
			out[i] = Text(err)
			continue
		}
		if value != nil && reflect.TypeOf(value).Kind() == reflect.String {
			out[i] = Text(reflect.ValueOf(value).String())
		}
	}
	return out
}
