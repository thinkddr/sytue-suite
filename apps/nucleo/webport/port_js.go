//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"
	"errors"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type JSPort struct{ id string }

func NewJS(id string) (*JSPort, error) {
	if !ValidID(id) || js.Global().Get("sytueCoreHostCall").Type() != js.TypeFunction {
		return nil, ErrRejected
	}
	return &JSPort{id: id}, nil
}
func (p *JSPort) ID() string { return p.id }
func BytesArg(value js.Value, maximum int) ([]byte, error) {
	if value.Type() != js.TypeObject || !value.InstanceOf(js.Global().Get("Uint8Array")) {
		return nil, ErrRejected
	}
	size := value.Get("byteLength").Int()
	if size < 0 || size > maximum {
		return nil, ErrRejected
	}
	out := make([]byte, size)
	if js.CopyBytesToGo(out, value) != size {
		clear(out)
		return nil, ErrRejected
	}
	return out, nil
}

func BytesValue(raw []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(out, raw)
	return out
}

func (p *JSPort) Current(ctx context.Context) (Identity, error) {
	raw, err := p.Call(ctx, storage.Scope{}, Current, nil)
	if err != nil {
		return Identity{}, err
	}
	defer clear(raw)
	var current Identity
	if json.Unmarshal(raw, &current) != nil || current.Validate() != nil {
		return Identity{}, ErrRejected
	}
	return current, nil
}

func (p *JSPort) Call(ctx context.Context, scope storage.Scope, method Method, payload []byte) ([]byte, error) {
	limit, ok := Limits(method)
	if !ok || len(payload) > limit.Request || method != Current && !scope.Valid() {
		return nil, ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scopeValue := any(nil)
	if scope.Valid() {
		scopeValue = map[string]any{"tenant": scope.Tenant, "subject": scope.Subject, "product": scope.Product}
	}
	promise := js.Global().Get("sytueCoreHostCall").Invoke(map[string]any{"port": p.id, "scope": scopeValue, "method": string(method), "payload": BytesValue(payload)})
	type outcome struct {
		body []byte
		err  error
	}
	complete := make(chan outcome, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			complete <- outcome{err: ErrRejected}
			return nil
		}
		body, err := BytesArg(args[0], limit.Response)
		if args[0].Type() == js.TypeObject && args[0].InstanceOf(js.Global().Get("Uint8Array")) {
			args[0].Call("fill", 0)
		}
		complete <- outcome{body, err}
		return nil
	})
	reject := js.FuncOf(func(js.Value, []js.Value) any { complete <- outcome{err: ErrRejected}; return nil })
	promise.Call("then", resolve).Call("catch", reject)
	defer resolve.Release()
	defer reject.Release()
	// Cancellation ends admission, not the outstanding host IO. Keep the original
	// Go witness until the Promise settles; a late capability must be closed before
	// this operation can return and release that witness.
	result := <-complete
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, discardHostReply(ctx, p, scope, method, result.body))
	}
	return result.body, result.err
}

// ScopeArg decodes routing only; it is never an authority grant.
func ScopeArg(req js.Value) storage.Scope {
	value := req.Get("scope")
	return storage.Scope{Tenant: value.Get("tenant").String(), Subject: value.Get("subject").String(), Product: value.Get("product").String()}
}

// AuthorizedJS binds a request to its host-owned account/product/device lease.
// Processors receive only authorized data; keys and network credentials remain
// with the host's crypto kernel and maintained SDK.
func AuthorizedJS(ctx context.Context, req js.Value) (*JSPort, Identity, error) {
	port, err := NewJS(req.Get("port").String())
	if err != nil {
		return nil, Identity{}, err
	}
	identity, err := port.Current(ctx)
	if err != nil {
		return nil, identity, err
	}
	if ScopeArg(req) != identity.Scope {
		return nil, identity, ErrRejected
	}
	if err = RevalidateIdentity(ctx, port, identity); err != nil {
		return nil, identity, err
	}
	return port, identity, nil
}

func RevalidateJS(ctx context.Context, port *JSPort, expected Identity) error {
	return RevalidateIdentity(ctx, port, expected)
}
