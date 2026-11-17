// SPDX-License-Identifier: Apache-2.0
// Package web defines the private Contacts operations consumed by browser hosts.
// It has no session management, transport, parser, or fallback to the service.
package web

import (
	"context"
	"errors"
	"reflect"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrRejected = errors.New("contacts web: rejected operation")

// Command is closed to this package. A host decodes its strict wire format into
// one of the exported value types; it cannot dispatch an arbitrary function.
type Command interface {
	execute(context.Context, api.ContactsData) (Result, error)
}

// Result is valid only for the duration of the consume callback. Body contains
// a locally generated vCard, photo or CSV; it is cleared after the callback.
// The buffer is cleared after consumption, but copies made by the callback are
// outside this package's control. Callers must serialize Value within the same gate.
type Result struct {
	Value any
	Body  []byte
}

type Dispatcher struct {
	data  api.ContactsData
	scope storage.Scope
}

// New requires a private Contacts repository. It does not establish that the
// device is trusted: the owning session must do that and hold its original work
// lease from before Perform through completion of the consume callback.
func New(data api.ContactsData) (*Dispatcher, error) {
	if data == nil {
		return nil, ErrRejected
	}
	value := reflect.ValueOf(data)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, ErrRejected
		}
	default:
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "contacts" {
		return nil, ErrRejected
	}
	return &Dispatcher{data: data, scope: scope}, nil
}

// Perform invokes one typed command and consumes its result while the caller's
// session gate is held. Cancellation signaled through ctx or a scope change
// suppresses a response before consumption; neither undoes a committed mutation.
// The session must also revalidate the lease after consumption before exposing
// output: this dispatcher cannot observe a revocation that the host does not signal.
// A nil Result.Value is used by successful mutations and is not an acknowledgement:
// a host may report success only after Perform returns nil and its gate revalidates.
// No wire protocol or consumer is provided here. Product argument validation remains
// with the supplied ContactsData implementation.
//
// The host transfers exclusive ownership of each command and all nested mutable
// input (including ImportCSV.Data and batch slices) for the entire call. It must
// not mutate, reuse or clear those values concurrently. Once Perform returns,
// the host owns input cleanup on every path, including cancellation and errors.
// A byte slice can be cleared; immutable Go strings and copies made elsewhere
// cannot be reliably erased by this dispatcher.
func (d *Dispatcher) Perform(ctx context.Context, command Command, consume func(Result) error) error {
	if d == nil || ctx == nil || command == nil || consume == nil || d.data == nil {
		return ErrRejected
	}
	// Commands are values, never pointers, so typed nils cannot panic on dispatch.
	if reflect.ValueOf(command).Kind() == reflect.Pointer {
		return ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.data.PrivateStorageScope() != d.scope {
		return ErrRejected
	}
	result, err := command.execute(ctx, d.data)
	defer clear(result.Body)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if d.data.PrivateStorageScope() != d.scope {
		return ErrRejected
	}
	if err != nil {
		return err
	}
	return consume(result)
}

// PartialImportError preserves non-sensitive progress when a CSV import fails.
// Retrying follows the repository's own idempotency rules. A future host must
// report a fixed incomplete-import error code and the counts, never the wrapped
// cause; it must not translate this error into a success acknowledgement.
type PartialImportError struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	cause    error
}

func (e *PartialImportError) Error() string { return "contacts web: import incomplete" }
func (e *PartialImportError) Unwrap() error { return e.cause }
