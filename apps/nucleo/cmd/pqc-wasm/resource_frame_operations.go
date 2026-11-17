//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func init() {
	domainOperations["resource.frame.encode"] = func(req js.Value) (any, error) {
		raw, err := bytesArg(req.Get("envelope"), 2*pqc.MaxEnvelopeCiphertext)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		frame, err := resource.EncodeFrameEnvelope(raw)
		if err != nil {
			return nil, err
		}
		defer clear(frame)
		return map[string]any{"frame": bytesValue(frame)}, nil
	}
	domainOperations["resource.frame.decode"] = func(req js.Value) (any, error) {
		raw, err := bytesArg(req.Get("frame"), 2*pqc.MaxEnvelopeCiphertext)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		envelope, err := resource.DecodeFrame(raw)
		if err != nil {
			return nil, err
		}
		defer clear(envelope.Ciphertext)
		canonical, err := matrixproof.CanonicalBytes(envelope)
		if err != nil {
			return nil, err
		}
		defer clear(canonical)
		return map[string]any{"envelope": bytesValue(canonical)}, nil
	}
}
