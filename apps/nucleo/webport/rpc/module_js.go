//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package rpc

import (
	"context"
	"sync"
	"syscall/js"
)

// Module is the one RPC transport for the crypto kernel and lazy processors.
// A processor imports its original Go domain, never a second crypto engine.
type Module struct {
	ID           string
	Version      int
	Execute      func(string, js.Value) (any, error)
	Async        map[string]func(context.Context, js.Value) (any, error)
	RejectedCode string
	// RevokePort cancels and joins domain resources before the internal ACK.
	// It receives only a validated host port ID, never arbitrary request data.
	RevokePort func(context.Context, string) error
}
type moduleRequest struct {
	cancel context.CancelFunc
	port   string
	done   chan struct{}
}
type moduleDispatcher struct {
	module                 Module
	mu                     sync.Mutex
	entries                map[int64]moduleRequest
	retired                map[string]js.Value
	invoke, cancel, revoke js.Func
}

func Serve(module Module) {
	if module.ID == "" || module.Version < 1 || module.Execute == nil {
		panic("invalid module")
	}
	if module.RejectedCode == "" {
		module.RejectedCode = "operation_rejected"
	}
	d := &moduleDispatcher{module: module, entries: map[int64]moduleRequest{}, retired: map[string]js.Value{}}
	d.invoke = js.FuncOf(d.dispatch)
	d.cancel = js.FuncOf(d.cancelAsync)
	d.revoke = js.FuncOf(d.revokePort)
	js.Global().Set("sytuePqc", map[string]any{"version": module.Version, "module_id": module.ID, "invoke": d.invoke, "cancel": d.cancel, "revokePort": d.revoke})
	select {}
}

func (d *moduleDispatcher) dispatch(_ js.Value, args []js.Value) (result any) {
	result = map[string]any{"ok": false, "code": "invalid_request"}
	defer func() {
		if recover() != nil {
			result = map[string]any{"ok": false, "code": "invalid_request"}
		}
	}()
	if len(args) != 1 || args[0].Type() != js.TypeObject {
		return result
	}
	req := args[0]
	if port := req.Get("port"); port.Type() == js.TypeString {
		d.mu.Lock()
		_, retired := d.retired[port.String()]
		d.mu.Unlock()
		if retired {
			return map[string]any{"ok": false, "code": d.module.RejectedCode}
		}
	}
	operation := req.Get("operation").String()
	if handler, ok := d.module.Async[operation]; ok {
		return d.dispatchAsync(req, handler)
	}
	value, err := d.module.Execute(operation, req)
	if err != nil {
		return Rechazo(d.module.RejectedCode, err)
	}
	return map[string]any{"ok": true, "value": value}
}

func requestNumber(value js.Value) (int64, bool) {
	if value.Type() != js.TypeNumber || value.Float() < 1 || value.Float() > 9007199254740991 || value.Float() != float64(int64(value.Float())) {
		return 0, false
	}
	return int64(value.Float()), true
}

func (d *moduleDispatcher) dispatchAsync(req js.Value, handler func(context.Context, js.Value) (any, error)) js.Value {
	executor := js.FuncOf(func(_ js.Value, functions []js.Value) any {
		resolve := functions[0]
		id, ok := requestNumber(req.Get("_request_id"))
		if !ok {
			resolve.Invoke(map[string]any{"ok": false, "code": "invalid_request"})
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		entry := moduleRequest{cancel: cancel, done: make(chan struct{})}
		if port := req.Get("port"); port.Type() == js.TypeString {
			entry.port = port.String()
		}
		d.mu.Lock()
		_, duplicate := d.entries[id]
		_, retired := d.retired[entry.port]
		if duplicate || retired {
			d.mu.Unlock()
			cancel()
			resolve.Invoke(map[string]any{"ok": false, "code": "invalid_request"})
			return nil
		}
		d.entries[id] = entry
		d.mu.Unlock()
		go func() {
			defer close(entry.done)
			result := map[string]any{"ok": false, "code": d.module.RejectedCode}
			var value any
			defer func() {
				if recover() != nil {
					eraseModuleValue(value)
					result = map[string]any{"ok": false, "code": d.module.RejectedCode}
				}
				if ctx.Err() != nil {
					eraseModuleValue(value)
					result = map[string]any{"ok": false, "code": d.module.RejectedCode}
				}
				cancel()
				d.mu.Lock()
				delete(d.entries, id)
				d.mu.Unlock()
				resolve.Invoke(result)
			}()
			var err error
			value, err = handler(ctx, req)
			if err == nil && ctx.Err() == nil {
				result = map[string]any{"ok": true, "value": value}
			} else {
				eraseModuleValue(value)
				result = Rechazo(d.module.RejectedCode, err)
			}
		}()
		return nil
	})
	promise := js.Global().Get("Promise").New(executor)
	executor.Release()
	return promise
}

func (d *moduleDispatcher) cancelAsync(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return nil
	}
	id, ok := requestNumber(args[0])
	if !ok {
		return nil
	}
	d.mu.Lock()
	entry, found := d.entries[id]
	d.mu.Unlock()
	if found {
		entry.cancel()
	}
	return nil
}

func (d *moduleDispatcher) revokePort(_ js.Value, args []js.Value) any {
	if len(args) != 1 || args[0].Type() != js.TypeString || !ValidID(args[0].String()) {
		return js.Global().Get("Promise").Call("resolve", false)
	}
	port := args[0].String()
	d.mu.Lock()
	if previous, ok := d.retired[port]; ok {
		d.mu.Unlock()
		return previous
	}
	d.mu.Unlock()
	var resolve js.Value
	executor := js.FuncOf(func(_ js.Value, functions []js.Value) any { resolve = functions[0]; return nil })
	promise := js.Global().Get("Promise").New(executor)
	executor.Release()
	// Mark retired before starting callbacks, including any reentrant callback.
	d.mu.Lock()
	d.retired[port] = promise
	var wait []<-chan struct{}
	for _, entry := range d.entries {
		if entry.port == port {
			entry.cancel()
			wait = append(wait, entry.done)
		}
	}
	d.mu.Unlock()
	go func() {
		ok := true
		func() {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			if d.module.RevokePort != nil {
				ok = d.module.RevokePort(context.Background(), port) == nil
			}
		}()
		for _, done := range wait {
			<-done
		}
		resolve.Invoke(ok)
	}()
	return promise
}

func eraseModuleValue(value any) {
	switch v := value.(type) {
	case js.Value:
		if v.Type() == js.TypeObject && v.InstanceOf(js.Global().Get("Uint8Array")) {
			v.Call("fill", 0)
		}
	case map[string]any:
		for _, field := range v {
			eraseModuleValue(field)
		}
	case []any:
		for _, field := range v {
			eraseModuleValue(field)
		}
	case []byte:
		clear(v)
	}
}
