//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package rpc

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BytesArg copia un Uint8Array acotado a Go; cualquier otra cosa se rechaza.
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

// BytesValue devuelve una copia de raw como Uint8Array.
func BytesValue(raw []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(out, raw)
	return out
}

// ScopeArg decodes routing only; it is never an authority grant.
func ScopeArg(req js.Value) storage.Scope {
	value := req.Get("scope")
	return storage.Scope{Tenant: value.Get("tenant").String(), Subject: value.Get("subject").String(), Product: value.Get("product").String()}
}
