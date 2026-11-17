//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var asyncDomainOperations = map[string]func(context.Context, js.Value) (any, error){}

func init() {
	asyncDomainOperations["core.host.call"] = hostPortCall
	domainOperations["core.host.contract"] = func(js.Value) (any, error) {
		raw, err := json.Marshal(webport.Contract())
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		return bytesValue(raw), nil
	}
}

func hostPortCall(ctx context.Context, req js.Value) (any, error) {
	port, err := webport.NewJS(req.Get("port").String())
	if err != nil {
		return nil, err
	}
	current, err := port.Current(ctx)
	if err != nil {
		return nil, err
	}
	method := webport.Method(req.Get("method").String())
	limit, ok := webport.Limits(method)
	if !ok {
		return nil, pqc.ErrEnvelope
	}
	if scopeArg(req) != current.Scope {
		return nil, webport.ErrRejected
	}
	if method == webport.Current {
		raw, err := json.Marshal(current)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		return bytesValue(raw), nil
	}
	input, err := bytesArg(req.Get("payload"), limit.Request)
	if err != nil {
		return nil, err
	}
	defer clear(input)
	output, err := port.Call(ctx, current.Scope, method, input)
	if err != nil {
		return nil, err
	}
	defer clear(output)
	return bytesValue(output), nil
}
