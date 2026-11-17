//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"syscall/js"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func resourceSessionChallenge(req js.Value) (resource.SessionChallenge, error) {
	raw, err := bytesArg(req.Get("challenge"), 64<<10)
	if err != nil {
		return resource.SessionChallenge{}, err
	}
	defer clear(raw)
	var value resource.SessionChallenge
	if json.Unmarshal(raw, &value) != nil {
		return resource.SessionChallenge{}, resource.ErrInvalid
	}
	if _, err := resource.SessionChallengeSHA256(value); err != nil {
		return resource.SessionChallenge{}, err
	}
	return value, nil
}

func init() {
	domainOperations["resource.session.challenge.open"] = func(req js.Value) (any, error) {
		challenge, err := resourceSessionChallenge(req)
		if err != nil {
			return nil, err
		}
		identity, err := identityArg(req)
		if err != nil {
			return nil, err
		}
		nonce, err := resource.OpenSessionChallenge(context.Background(), challenge, identity, time.Now())
		if err != nil {
			return nil, err
		}
		return map[string]any{"nonce": nonce}, nil
	}
	domainOperations["resource.session.enrollment.prepare"] = func(req js.Value) (any, error) {
		challenge, err := resourceSessionChallenge(req)
		if err != nil {
			return nil, err
		}
		value, err := resource.PrepareSessionEnrollment(challenge, req.Get("nonce").String(), req.Get("root_decision").String(), time.Now())
		if err != nil {
			return nil, err
		}
		canonical, err := matrixproof.CanonicalBytes(value)
		if err != nil {
			return nil, err
		}
		defer clear(canonical)
		return map[string]any{"payload": bytesValue(canonical)}, nil
	}
}
