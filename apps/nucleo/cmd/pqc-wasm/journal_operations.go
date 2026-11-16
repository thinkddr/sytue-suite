//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"strings"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func init() { asyncDomainOperations["core.journal.call"] = journalCall }
func journalCall(ctx context.Context, req js.Value) (any, error) {
	port, err := webport.NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	journal, current, err := webport.OpenJournal(ctx, port)
	if err != nil {
		return nil, err
	}
	if scopeArg(req) != current.Scope {
		return nil, webport.ErrRejected
	}
	raw, err := bytesArg(req.Get("input"), 8<<20)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Action    string          `json:"action"`
		Namespace string          `json:"namespace"`
		ID        string          `json:"id"`
		Value     json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &input) != nil || input.Namespace == "" || len(input.Namespace) > 255 || input.ID == "" || len(input.ID) > 4096 {
		return nil, webport.ErrRejected
	}
	defer clear(input.Value)
	owner := strings.Join([]string{current.Scope.Tenant, current.Scope.Subject, current.Scope.Product}, "\x00")
	switch input.Action {
	case "put":
		if !json.Valid(input.Value) {
			return nil, webport.ErrRejected
		}
		err = journal.Exclusive(ctx, owner, input.Namespace, input.ID, func() error { return journal.Write(owner, input.Namespace, input.ID, input.Value) })
		if err != nil {
			return nil, err
		}
		return nil, nil
	case "get":
		var value json.RawMessage
		found, err := journal.Read(owner, input.Namespace, input.ID, &value)
		if err != nil {
			return nil, err
		}
		defer clear(value)
		response, err := json.Marshal(struct {
			Found bool            `json:"found"`
			Value json.RawMessage `json:"value"`
		}{found, value})
		if err != nil {
			return nil, err
		}
		defer clear(response)
		return bytesValue(response), nil
	case "delete":
		if err := journal.Exclusive(ctx, owner, input.Namespace, input.ID, func() error { return journal.Delete(owner, input.Namespace, input.ID) }); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, webport.ErrRejected
	}
}
