// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateContactsFixture struct {
	ContactsData
	scope   storage.Scope
	contact func(context.Context, string) (Contact, error)
	create  func(context.Context, string, ContactInput) (Contact, error)
	photo   func(context.Context, string) ([]byte, error)
	page    func(context.Context, string, string) (ContactPage, error)
}

func (f privateContactsFixture) PrivateStorageScope() storage.Scope { return f.scope }

func (f privateContactsFixture) Contact(ctx context.Context, id string) (Contact, error) {
	return f.contact(ctx, id)
}

func (f privateContactsFixture) Create(ctx context.Context, book string, in ContactInput) (Contact, error) {
	return f.create(ctx, book, in)
}

func (f privateContactsFixture) Photo(ctx context.Context, uid string) ([]byte, error) {
	return f.photo(ctx, uid)
}

func (f privateContactsFixture) InBookPage(ctx context.Context, book, cursor string) (ContactPage, error) {
	return f.page(ctx, book, cursor)
}

func TestPrivateContactsPreservesFailuresPayloadsAndOriginalClient(t *testing.T) {
	calls := 0
	original := NewContacts("https://contacts.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"legacy","formatted_name":"legacy"}`)), Request: r}, nil
	})})
	missing := errors.New("private keys unavailable")
	privateNote := "name and note must never reach legacy HTTP"
	image := []byte{0, 1, 2, 3, 4, 0xff}
	fixture := privateContactsFixture{
		scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"},
		contact: func(_ context.Context, id string) (Contact, error) {
			if id != "contact" {
				t.Fatal("changed private contact ID")
			}
			return Contact{}, missing
		},
		create: func(_ context.Context, book string, in ContactInput) (Contact, error) {
			if book != "private-book" || in.Note != privateNote {
				t.Fatal("private payload changed")
			}
			return Contact{ID: "new-private", Note: in.Note}, nil
		},
		photo: func(_ context.Context, uid string) ([]byte, error) {
			if uid != "urn:uuid:contact" {
				t.Fatal("private UID changed before dispatch")
			}
			return bytes.Clone(image), nil
		},
	}
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Contact(t.Context(), "contact"); !errors.Is(err, missing) || calls != 0 {
		t.Fatal("private failure reached legacy", err, calls)
	}
	created, err := private.Create(t.Context(), "private-book", ContactInput{FormattedName: "private name", Note: privateNote})
	if err != nil || created.ID != "new-private" || created.Note != privateNote || calls != 0 {
		t.Fatal("private write lost data or reached HTTP", err, calls)
	}
	photo, err := private.Photo(t.Context(), "urn:uuid:contact")
	if err != nil || !bytes.Equal(photo, image) || calls != 0 {
		t.Fatal("private photo did not retain exact bytes", err, calls)
	}
	if _, err = private.c.Do(t.Context(), Request{Method: http.MethodPost, Path: "/future-operation", Body: []byte(privateNote)}); !errors.Is(err, ErrPrivateContactsOperation) || calls != 0 {
		t.Fatal("unimplemented private operation reached HTTP", err, calls)
	}
	legacy, err := original.Contact(t.Context(), "contact")
	if err != nil || legacy.ID != "legacy" || calls != 1 {
		t.Fatal("private facade changed original client", err, calls)
	}
	if _, err = private.WithPrivateData(fixture); !errors.Is(err, ErrPrivateContactsOperation) {
		t.Fatal("nested private facade accepted", err)
	}
	for _, scope := range []storage.Scope{{}, {Tenant: "org", Subject: "owner", Product: "drive"}} {
		fixture.scope = scope
		if _, err = original.WithPrivateData(fixture); !errors.Is(err, ErrPrivateContactsOperation) {
			t.Fatal("unscoped repository accepted", err)
		}
	}
	if _, err = original.WithPrivateData(nil); !errors.Is(err, ErrPrivateContactsOperation) {
		t.Fatal("nil private repository accepted", err)
	}
}

func TestPrivateContactsBookInventoryHasNoLegacyFallbackOrPartialSuccess(t *testing.T) {
	for _, fault := range []string{"complete", "missing-book", "later-failure", "cycle"} {
		t.Run(fault, func(t *testing.T) {
			requests, pages := 0, 0
			original := NewContacts("https://contacts.example", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected legacy read")
			})})
			unavailable := errors.New("private history unavailable")
			private, err := original.WithPrivateData(privateContactsFixture{scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"}, page: func(_ context.Context, book, cursor string) (ContactPage, error) {
				pages++
				if book != "book" {
					t.Fatal("book scope changed")
				}
				if fault == "missing-book" {
					return ContactPage{}, &Error{Status: http.StatusNotFound}
				}
				if cursor == "" {
					return ContactPage{Contacts: []Contact{{ID: "first"}}, NextCursor: "private-next"}, nil
				}
				if fault == "later-failure" {
					return ContactPage{}, unavailable
				}
				if fault == "cycle" {
					return ContactPage{Contacts: []Contact{{ID: "second"}}, NextCursor: "private-next"}, nil
				}
				return ContactPage{Contacts: []Contact{{ID: "second"}}}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			contacts, err := private.InBook(t.Context(), "book")
			if requests != 0 {
				t.Fatal("private inventory fell back to HTTP", requests)
			}
			if fault == "complete" {
				if err != nil || len(contacts) != 2 || pages != 2 {
					t.Fatal("private inventory was truncated", contacts, err, pages)
				}
			} else {
				if err == nil || contacts != nil {
					t.Fatal("incomplete inventory became success", contacts, err)
				}
				if fault == "missing-book" {
					var missing *Error
					if !errors.As(err, &missing) || missing.Status != http.StatusNotFound || pages != 1 {
						t.Fatal("missing private book tried legacy fallback", err)
					}
				}
				if fault == "later-failure" && !errors.Is(err, unavailable) {
					t.Fatal("private page failure changed", err)
				}
			}
		})
	}
}
