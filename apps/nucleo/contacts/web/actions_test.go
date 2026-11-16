// SPDX-License-Identifier: Apache-2.0
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Unimplemented operations panic if the dispatcher reaches an unintended path.
type fixture struct {
	api.ContactsData
	scope     storage.Scope
	body      []byte
	onRead    func()
	onImport  func()
	importErr error
	book      string
	input     api.ContactInput
}

func (f *fixture) PrivateStorageScope() storage.Scope { return f.scope }
func (f *fixture) VCard(_ context.Context, id string) ([]byte, error) {
	if id != "private-id" {
		panic("wrong contact")
	}
	if f.onRead != nil {
		f.onRead()
	}
	return f.body, nil
}

func (f *fixture) Create(_ context.Context, book string, input api.ContactInput) (api.Contact, error) {
	f.book, f.input = book, input
	return api.Contact{ID: "created", FormattedName: input.FormattedName}, nil
}

func (f *fixture) ImportCSV(_ context.Context, book string, data []byte) (int, int, error) {
	if book != "book" || string(data) != "csv" {
		panic("wrong import")
	}
	if f.onImport != nil {
		f.onImport()
	}
	return 2, 1, f.importErr
}

func validFixture() *fixture {
	return &fixture{scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "contacts"}}
}

func TestRejectsForeignScopeNilAndPointerCommands(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrRejected) {
		t.Fatal(err)
	}
	var typedNil *fixture
	if _, err := New(typedNil); !errors.Is(err, ErrRejected) {
		t.Fatal(err)
	}
	foreign := validFixture()
	foreign.scope.Product = "mail"
	if _, err := New(foreign); !errors.Is(err, ErrRejected) {
		t.Fatal(err)
	}
	f := validFixture()
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	consume := func(Result) error { called = true; return nil }
	var nilCommand *ExportCard
	if err := d.Perform(t.Context(), nilCommand, consume); !errors.Is(err, ErrRejected) {
		t.Fatal(err)
	}
	f.scope.Subject = "other"
	if err := d.Perform(t.Context(), ExportCard{ID: "private-id"}, consume); !errors.Is(err, ErrRejected) || called {
		t.Fatal("foreign identity admitted", err)
	}
}

func TestResultConsumedInsideCallAndBuffersCleared(t *testing.T) {
	f := validFixture()
	f.body = []byte("private vCard")
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	var received []byte
	err = d.Perform(t.Context(), ExportCard{ID: "private-id"}, func(r Result) error {
		if string(r.Body) != "private vCard" {
			t.Fatal("lost card")
		}
		received = r.Body
		return nil
	})
	if err != nil || !bytes.Equal(received, make([]byte, len(received))) {
		t.Fatal("plaintext retained", err)
	}
	f.body = []byte("private vCard")
	f.onRead = func() { f.scope.Subject = "other" }
	called := false
	err = d.Perform(t.Context(), ExportCard{ID: "private-id"}, func(Result) error { called = true; return nil })
	if !errors.Is(err, ErrRejected) || called || !bytes.Equal(f.body, make([]byte, len(f.body))) {
		t.Fatal("late result escaped", err)
	}
}

func TestCancellationSuppressesLateResultsAndKeepsImportProgress(t *testing.T) {
	f := validFixture()
	f.body = []byte("private")
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	f.onRead = cancel
	called := false
	err = d.Perform(ctx, ExportCard{ID: "private-id"}, func(Result) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called || !bytes.Equal(f.body, make([]byte, len(f.body))) {
		t.Fatal("canceled result escaped", err)
	}
	failure := errors.New("connection lost")
	f.importErr = failure
	err = d.Perform(t.Context(), ImportCSV{Book: "book", Data: []byte("csv")}, func(Result) error { t.Fatal("partial import treated as success"); return nil })
	var partial *PartialImportError
	if !errors.As(err, &partial) || !errors.Is(err, failure) || partial.Imported != 2 || partial.Skipped != 1 {
		t.Fatal("partial progress lost", err)
	}
	wire, jsonErr := json.Marshal(partial)
	if jsonErr != nil || string(wire) != `{"imported":2,"skipped":1}` {
		t.Fatal("wrapped cause exposed in JSON", jsonErr)
	}
	f.onImport = func() { f.scope.Subject = "another" }
	err = d.Perform(t.Context(), ImportCSV{Book: "book", Data: []byte("csv")}, func(Result) error { t.Fatal("foreign import exposed"); return nil })
	if !errors.Is(err, ErrRejected) || errors.As(err, &partial) {
		t.Fatal("partial progress leaked across identity", err)
	}
}

func TestTypedMutationPassesOriginalInputAndErrorFromConsumer(t *testing.T) {
	f := validFixture()
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("consumer failed")
	input := api.ContactInput{FormattedName: "Private", Note: "keep", Emails: []api.ContactEmail{{Email: "user@example.invalid"}}}
	err = d.Perform(t.Context(), CreateContact{Book: "book", Input: input}, func(r Result) error {
		contact, ok := r.Value.(api.Contact)
		if !ok || contact.ID != "created" || contact.FormattedName != input.FormattedName {
			t.Fatal("wrong response")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || f.book != "book" || f.input.Note != input.Note || len(f.input.Emails) != 1 {
		t.Fatal("input changed or consumer error lost", err)
	}
}

type blockingImport struct {
	*fixture
	entered chan struct{}
	resume  chan struct{}
	seen    []byte
}

func (f *blockingImport) ImportCSV(_ context.Context, _ string, data []byte) (int, int, error) {
	close(f.entered)
	<-f.resume
	f.seen = bytes.Clone(data)
	return 1, 0, nil
}

func TestInputOwnedUntilCanceledCallReturns(t *testing.T) {
	f := &blockingImport{fixture: validFixture(), entered: make(chan struct{}), resume: make(chan struct{})}
	d, err := New(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	input := []byte("private csv")
	result := make(chan error, 1)
	go func() {
		result <- d.Perform(ctx, ImportCSV{Book: "book", Data: input}, func(Result) error {
			t.Error("canceled result was consumed")
			return nil
		})
	}()
	<-f.entered
	cancel()
	close(f.resume)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was lost", err)
	}
	if string(f.seen) != "private csv" || string(input) != "private csv" {
		t.Fatal("input changed before the caller regained ownership")
	}
	// The host, not the dispatcher or repository, now clears its owned buffer.
	clear(input)
	clear(f.seen)
}
