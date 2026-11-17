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

func init() { asyncDomainOperations["core.storage.call"] = storageCall }
func storageCall(ctx context.Context, req js.Value) (any, error) {
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
	raw, err := bytesArg(req.Get("input"), 1<<20)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Action      string                    `json:"action"`
		Reference   storage.Reference         `json:"reference"`
		Object      string                    `json:"object"`
		Revision    string                    `json:"revision"`
		Role        string                    `json:"role"`
		Room        string                    `json:"room"`
		Cursor      string                    `json:"cursor"`
		Limit       int                       `json:"limit"`
		Target      storage.PublicationTarget `json:"target"`
		Job         string                    `json:"job"`
		Transaction string                    `json:"transaction"`
		Record      storage.Record            `json:"record"`
		Commit      storage.ObjectCommit      `json:"commit"`
		Request     storage.FileRequest       `json:"request"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, webport.ErrRejected
	}
	defer clear(input.Record.Payload)
	defer clear(input.Commit.Metadata)
	defer clear(input.Request.Metadata)
	if input.Action != "prepare" {
		if _, err = repository.StorageBindings(ctx); err != nil {
			return nil, err
		}
	}
	var result any
	switch input.Action {
	case "prepare":
		result, err = repository.PrepareStorage(ctx, repository.FileScope())
	case "recoverable":
		_, err = repository.RequireRecoverableIngress(ctx, repository.FileScope(), input.Target)
	case "bindings":
		result, err = repository.StorageBindings(ctx)
	case "history":
		result, err = repository.StorageObjectHistory(ctx, input.Room, input.Cursor, input.Limit)
	case "record.read":
		result, err = repository.ReadStorageRecord(ctx, input.Reference, input.Object, input.Role)
	case "record.put":
		result, err = repository.PutStorageRecordAt(ctx, input.Target, input.Transaction, input.Record)
	case "object.read":
		result, err = repository.StorageObject(ctx, input.Reference, input.Object)
	case "object.put":
		result, err = repository.PutStorageObjectAt(ctx, input.Target, input.Transaction, input.Object, input.Revision, input.Commit)
	case "file.info":
		result, err = repository.Info(ctx, input.Reference, input.Object, input.Revision)
	case "file.verify":
		result, err = repository.Download(ctx, input.Reference, input.Object, input.Revision, io.Discard)
	case "file.copy":
		result, err = repository.Copy(ctx, input.Reference, input.Object, input.Revision, input.Job, input.Target, input.Request)
	case "file.ack":
		err = repository.Acknowledge(ctx, input.Job, input.Reference)
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
