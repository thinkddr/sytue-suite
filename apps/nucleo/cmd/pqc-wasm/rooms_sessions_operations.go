//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
)

func init() {
	domainOperations["matrix.session.proof.encode"] = encodeMatrixSessionProof
	domainOperations["matrix.session.proof.decode"] = decodeMatrixSessionProof
}

func encodeMatrixSessionProof(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("session"), 4096)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	session, err := sessionproof.DecodeSession(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := sessionproof.Encode(scopeArg(req), session)
	if err != nil {
		return nil, err
	}
	defer clear(encoded.Payload)
	return map[string]any{"id": encoded.ID, "payload": bytesValue(encoded.Payload)}, nil
}

func decodeMatrixSessionProof(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("session"), 4096)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	session, err := sessionproof.DecodeSession(raw)
	if err != nil {
		return nil, err
	}
	payload, err := bytesArg(req.Get("payload"), 4096)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	return sessionproof.Matches(scopeArg(req), session, payload), nil
}
