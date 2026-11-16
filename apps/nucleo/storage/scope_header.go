// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// ScopeHeader pins a job to the identity that started it, independently of a
// refreshed access token or a changed active organization. It is public routing
// data, not authorization: the gateway must compare it with authenticated context.
const ScopeHeader = "X-Sytue-Storage-Scope"

const maxScopeHeaderBytes = 4096

func EncodeScopeHeader(scope Scope) (string, error) {
	if !scope.Valid() {
		return "", ErrRecord
	}
	raw, err := json.Marshal(scope)
	if err != nil {
		return "", ErrRecord
	}
	return "v1." + base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeScopeHeader(value string) (Scope, error) {
	if len(value) > maxScopeHeaderBytes || !strings.HasPrefix(value, "v1.") {
		return Scope{}, ErrRecord
	}
	encoded := value[3:]
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(raw) || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return Scope{}, ErrRecord
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var scope Scope
	if decoder.Decode(&scope) != nil || !scope.Valid() || decoder.Decode(new(any)) != io.EOF {
		return Scope{}, ErrRecord
	}
	return scope, nil
}
