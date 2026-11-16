//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

type promiseResult struct {
	raw []byte
	err error
}

func pendingHostPromise() (js.Value, js.Value) {
	var resolve js.Value
	executor := js.FuncOf(func(_ js.Value, args []js.Value) any { resolve = args[0]; return nil })
	promise := js.Global().Get("Promise").New(executor)
	executor.Release()
	return promise, resolve
}

func replaceHostCall(t *testing.T, fn func(js.Value) any) {
	t.Helper()
	previous := js.Global().Get("sytueCoreHostCall")
	host := js.FuncOf(func(_ js.Value, args []js.Value) any { return fn(args[0]) })
	js.Global().Set("sytueCoreHostCall", host)
	t.Cleanup(func() { js.Global().Set("sytueCoreHostCall", previous); host.Release() })
}

func assertJSReplyErased(t *testing.T, value js.Value) {
	t.Helper()
	raw, err := BytesArg(value, 64<<10)
	defer clear(raw)
	if err != nil || !bytes.Equal(raw, make([]byte, len(raw))) {
		t.Fatal("settled JavaScript response retained plaintext", err)
	}
}

func TestJSPortCancellationJoinsPromiseBeforeReturningLateBytes(t *testing.T) {
	promise, resolve := pendingHostPromise()
	entered := make(chan struct{})
	replaceHostCall(t, func(request js.Value) any {
		if request.Get("method").String() != string(StorageReadRecord) {
			t.Error("unexpected host route")
		}
		request.Get("payload").Call("fill", 0)
		close(entered)
		return promise
	})
	port, err := NewJS(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan promiseResult, 1)
	go func() {
		raw, e := port.Call(ctx, fixtureIdentity().Scope, StorageReadRecord, nil)
		done <- promiseResult{raw, e}
	}()
	<-entered
	cancel()
	select {
	case result := <-done:
		t.Fatal("cancel released original operation before host Promise settled", result.err)
	case <-time.After(20 * time.Millisecond):
	}
	late := BytesValue([]byte("private late plaintext"))
	resolve.Invoke(late)
	result := <-done
	if result.raw != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatal("late result escaped cancellation", result.err)
	}
	assertJSReplyErased(t, late)
}

func TestJSPortCancellationJoinsUnadoptedHandleClose(t *testing.T) {
	promise, resolve := pendingHostPromise()
	cleanup, finishCleanup := pendingHostPromise()
	entered, cleanupEntered := make(chan struct{}), make(chan struct{})
	scope := fixtureIdentity().Scope
	id := strings.Repeat("b", 64)
	replaceHostCall(t, func(request js.Value) any {
		raw, e := BytesArg(request.Get("payload"), 64<<10)
		request.Get("payload").Call("fill", 0)
		defer clear(raw)
		if e != nil {
			t.Error(e)
		}
		if request.Get("port").String() != strings.Repeat("a", 64) || ScopeArg(request) != scope {
			t.Error("cleanup adopted a different original port/scope")
		}
		switch Method(request.Get("method").String()) {
		case SourceOpen:
			close(entered)
			return promise
		case SourceClose:
			var value map[string]string
			if json.Unmarshal(raw, &value) != nil || len(value) != 1 || value["stream"] != id {
				t.Error("cleanup changed fixed stream handle")
			}
			close(cleanupEntered)
			return cleanup
		default:
			t.Error("unrecognized cleanup route")
			return js.Global().Get("Promise").Call("reject", js.Global().Get("Error").New("rejected"))
		}
	})
	port, err := NewJS(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan promiseResult, 1)
	go func() { raw, e := port.Call(ctx, scope, SourceOpen, nil); done <- promiseResult{raw, e} }()
	<-entered
	cancel()
	raw, _ := json.Marshal(map[string]string{"stream": id})
	late := BytesValue(raw)
	clear(raw)
	resolve.Invoke(late)
	select {
	case <-cleanupEntered:
	case result := <-done:
		t.Fatal("unadopted stream returned without cleanup", result.err)
	}
	select {
	case result := <-done:
		t.Fatal("cancel returned before unadopted stream close joined", result.err)
	default:
	}
	finishCleanup.Invoke(BytesValue(nil))
	result := <-done
	if result.raw != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatal("discarded handle escaped cancellation", result.err)
	}
	assertJSReplyErased(t, late)
}

func TestJSPortCancellationRetainsCleanupErrorAndSuccessfulRepliesAreOwned(t *testing.T) {
	promise, resolve := pendingHostPromise()
	entered := make(chan struct{})
	fail := false
	replaceHostCall(t, func(request js.Value) any {
		request.Get("payload").Call("fill", 0)
		if Method(request.Get("method").String()) == ServiceClose {
			return js.Global().Get("Promise").Call("reject", js.Global().Get("Error").New("cleanup failed"))
		}
		if fail {
			close(entered)
			return promise
		}
		return js.Global().Get("Promise").Call("resolve", BytesValue([]byte("owned")))
	})
	port, err := NewJS(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := port.Call(t.Context(), fixtureIdentity().Scope, StorageReadRecord, nil)
	if err != nil || string(raw) != "owned" {
		t.Fatal("successful owned reply changed", err)
	}
	clear(raw)
	fail = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan promiseResult, 1)
	go func() {
		out, e := port.Call(ctx, fixtureIdentity().Scope, ServiceRequest, nil)
		done <- promiseResult{out, e}
	}()
	<-entered
	cancel()
	body, _ := json.Marshal(map[string]string{"stream": strings.Repeat("b", 64)})
	late := BytesValue(body)
	clear(body)
	resolve.Invoke(late)
	result := <-done
	if result.raw != nil || !errors.Is(result.err, context.Canceled) || !errors.Is(result.err, ErrRejected) {
		t.Fatal("cleanup failure or cancellation was hidden", result.err)
	}
	assertJSReplyErased(t, late)
}
