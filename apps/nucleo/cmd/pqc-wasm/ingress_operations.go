//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func init() { asyncDomainOperations["core.ingress.call"] = ingressCall }
func ingressCall(ctx context.Context, req js.Value) (any, error) {
	port, err := webport.NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	repository, err := webport.OpenStorage(ctx, port, coreSources, corePolicies)
	if err != nil {
		return nil, err
	}
	if scopeArg(req) != repository.FileScope() {
		return nil, webport.ErrRejected
	}
	raw, err := bytesArg(req.Get("input"), 32<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Action     string          `json:"action"`
		Epoch      string          `json:"epoch"`
		Generation int64           `json:"generation"`
		Plan       ingress.KeyPlan `json:"plan"`
		Task       ingress.Task    `json:"task"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, webport.ErrRejected
	}
	if _, err = repository.PrepareStorage(ctx, repository.FileScope()); err != nil {
		return nil, err
	}
	var result any
	switch input.Action {
	case "create":
		result, err = repository.CreateIngressKey(ctx, repository.FileScope(), input.Epoch, input.Generation)
	case "recover":
		if input.Plan.Scope != repository.FileScope() {
			return nil, webport.ErrRejected
		}
		result, err = repository.RecoverIngressEnrollment(ctx, input.Plan)
	case "open":
		if input.Task.Scope != repository.FileScope() {
			return nil, webport.ErrRejected
		}
		// This bounded public core operation accepts an owned ciphertext buffer.
		// Application ingress workers use the same keyring.Open with a staged
		// ReaderAt, retaining streaming for full-size messages and attachments.
		cipher, readErr := bytesArg(req.Get("cipher"), 512<<10)
		if readErr != nil {
			return nil, readErr
		}
		defer clear(cipher)
		metadata, body, openErr := repository.OpenIngressBody(ctx, input.Task, bytes.NewReader(cipher))
		if openErr != nil {
			return nil, openErr
		}
		defer clear(metadata)
		original, readErr := io.ReadAll(body)
		defer clear(original)
		if readErr != nil {
			return nil, readErr
		}
		result = struct {
			Metadata []byte `json:"metadata"`
			Body     []byte `json:"body"`
		}{metadata, original}
	default:
		return nil, webport.ErrRejected
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	defer clear(encoded)
	return bytesValue(encoded), nil
}
