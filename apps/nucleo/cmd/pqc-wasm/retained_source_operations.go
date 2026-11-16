//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"io"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func init() { asyncDomainOperations["core.source.call"] = sourceCall }
func sourceCall(ctx context.Context, req js.Value) (any, error) {
	port, err := webport.NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	owner, err := webport.OpenStorage(ctx, port, coreSources, corePolicies)
	if err != nil {
		return nil, err
	}
	if scopeArg(req) != owner.FileScope() {
		return nil, webport.ErrRejected
	}
	raw, err := bytesArg(req.Get("input"), 64<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Action    string                     `json:"action"`
		Epoch     string                     `json:"epoch"`
		Provision bool                       `json:"provision"`
		After     storage.LegacySourceCursor `json:"after"`
		Row       storage.StreamSourceRow    `json:"row"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, webport.ErrRejected
	}
	source, err := webport.NewSource(owner, input.Provision)
	if err != nil {
		return nil, err
	}
	var result any
	switch input.Action {
	case "state":
		result, err = source.PrivateStorageState(ctx, owner.FileScope())
	case "begin":
		result, err = source.BeginPrivateStorageMigration(ctx, owner.FileScope(), input.Epoch)
	case "page":
		result, err = source.PrivateStorageSourcePage(ctx, owner.FileScope(), input.Epoch, input.After)
	case "verify":
		var reader io.ReadCloser
		reader, err = source.OpenPrivateStorageSource(ctx, owner.FileScope(), input.Epoch, input.Row)
		if err == nil {
			_, err = io.Copy(io.Discard, reader)
			closeErr := reader.Close()
			if err == nil {
				err = closeErr
			}
			if err == nil {
				result = input.Row
			}
		}
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
