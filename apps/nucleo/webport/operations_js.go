// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package webport

import (
	"context"
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (sources *Sources) ReadJS(ctx context.Context, req js.Value) (any, error) {
	port, err := NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	identity, err := port.Current(ctx)
	if err != nil {
		return nil, err
	}
	if identity.Validate() != nil || ScopeArg(req) != identity.Scope {
		return nil, ErrRejected
	}
	raw, err := BytesArg(req.Get("input"), 8192)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Source string `json:"source"`
		Offset int64  `json:"offset"`
		Length int    `json:"length"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, ErrRejected
	}
	out, err := sources.Read(ctx, port, identity, input.Source, input.Offset, input.Length)
	if err != nil {
		return nil, err
	}
	defer clear(out)
	return BytesValue(out), nil
}

func (sources *Sources) FinalizeJS(ctx context.Context, req js.Value) (any, error) {
	port, err := NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	identity, err := port.Current(ctx)
	if err != nil || identity.Validate() != nil || ScopeArg(req) != identity.Scope {
		return nil, ErrRejected
	}
	raw, err := BytesArg(req.Get("input"), 8192)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, ErrRejected
	}
	checksum, err := sources.FinalizeSource(ctx, port, identity, input.Source)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sha256": checksum}, nil
}

func (policies *Policies) AuthorizeJS(ctx context.Context, req js.Value) (any, error) {
	port, err := NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	identity, err := port.Current(ctx)
	if err != nil {
		return nil, err
	}
	if identity.Validate() != nil || ScopeArg(req) != identity.Scope {
		return nil, ErrRejected
	}
	raw, err := BytesArg(req.Get("input"), 8192)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var input struct {
		Authorization string                    `json:"authorization"`
		Target        storage.PublicationTarget `json:"target"`
		Transaction   string                    `json:"transaction"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, ErrRejected
	}
	err = policies.Authorize(ctx, port, identity, input.Authorization, input.Target, input.Transaction)
	return nil, err
}
