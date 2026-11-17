// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privatePhotoByIDFixture struct {
	privateContactsFixture
	read func(context.Context, string, string) ([]byte, error)
}

func (f privatePhotoByIDFixture) PhotoByID(ctx context.Context, id, etag string) ([]byte, error) {
	return f.read(ctx, id, etag)
}

func TestPrivatePhotoByIDRequiresCapabilityAndNeverFallsBack(t *testing.T) {
	httpCalls, uidCalls := 0, 0
	original := NewContacts("https://contacts.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		httpCalls++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("legacy"))), Request: r}, nil
	})})
	fixture := privateContactsFixture{
		scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"},
		photo: func(context.Context, string) ([]byte, error) { uidCalls++; return []byte("uid"), nil },
	}
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []*Contacts{nil, original, private} {
		if photo, err := client.PhotoByID(t.Context(), "contact", "revision"); photo != nil || !errors.Is(err, ErrPrivateContactsOperation) {
			t.Fatal("missing private capability returned a photo", err)
		}
	}
	if httpCalls != 0 || uidCalls != 0 {
		t.Fatal("ID-based private read fell back to UID or HTTP")
	}
	legacy, err := original.Photo(t.Context(), "legacy-uid")
	if err != nil || string(legacy) != "legacy" || httpCalls != 1 {
		t.Fatal("legacy UID photo changed", err)
	}
	legacy, err = private.Photo(t.Context(), "uid")
	if err != nil || string(legacy) != "uid" || uidCalls != 1 {
		t.Fatal("existing private UID photo changed", err)
	}
}

func TestPrivatePhotoByIDDispatchesExactIDRevisionAndError(t *testing.T) {
	photo := []byte{0, 1, 2, 0xff}
	failure := errors.New("private read failed")
	fixture := privatePhotoByIDFixture{
		privateContactsFixture: privateContactsFixture{scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"}},
		read: func(_ context.Context, id, etag string) ([]byte, error) {
			if id != "contact" || etag != "revision" {
				t.Fatal("private ID or revision changed")
			}
			return bytes.Clone(photo), nil
		},
	}
	private, err := NewContacts("https://contacts.example", nil, nil).WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	got, err := private.PhotoByID(t.Context(), "contact", "revision")
	if err != nil || !bytes.Equal(got, photo) {
		t.Fatal("private photo bytes changed", err)
	}
	fixture.read = func(context.Context, string, string) ([]byte, error) { return []byte("partial"), failure }
	private, err = NewContacts("https://contacts.example", nil, nil).WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := private.PhotoByID(t.Context(), "contact", "revision"); got != nil || !errors.Is(err, failure) {
		t.Fatal("private read failure changed or fell back", err)
	}
}

func TestPhotoForContactKeepsPrivateAndLegacyRoutesSeparate(t *testing.T) {
	httpCalls, uidCalls, idCalls := 0, 0, 0
	original := NewContacts("https://contacts.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		httpCalls++
		if r.URL.Query().Get("uid") != "shared" {
			t.Fatal("legacy endpoint received the wrong UID")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("legacy"))), Request: r}, nil
	})})
	base := privateContactsFixture{
		scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"},
		photo: func(context.Context, string) ([]byte, error) { uidCalls++; return []byte("wrong UID"), nil },
	}
	person := Contact{ID: "person-a", AddressBookID: "book-a", UID: "shared", ETag: "r1"}
	legacy, err := original.PhotoForContact(t.Context(), person)
	if err != nil || string(legacy) != "legacy" || httpCalls != 1 {
		t.Fatal("legacy route changed", err)
	}
	missing, err := original.WithPrivateData(base)
	if err != nil {
		t.Fatal(err)
	}
	if photo, err := missing.PhotoForContact(t.Context(), person); photo != nil || !errors.Is(err, ErrPrivateContactsOperation) {
		t.Fatal("missing capability did not fail closed", err)
	}
	private, err := original.WithPrivateData(privatePhotoByIDFixture{privateContactsFixture: base, read: func(_ context.Context, id, etag string) ([]byte, error) {
		idCalls++
		if id != person.ID || etag != person.ETag {
			t.Fatal("private route changed ID or revision")
		}
		return []byte("private"), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	photo, err := private.PhotoForContact(t.Context(), person)
	if err != nil || string(photo) != "private" || idCalls != 1 || uidCalls != 0 || httpCalls != 1 {
		t.Fatal("private photo used a different route", err)
	}
	for _, invalid := range []Contact{
		{ID: "", AddressBookID: "book-a", ETag: "r1"},
		{ID: "person-a", AddressBookID: "", ETag: "r1"},
		{ID: "person-a", AddressBookID: "book-a", ETag: ""},
	} {
		if got, err := private.PhotoForContact(t.Context(), invalid); got != nil || !errors.Is(err, ErrPrivateContactsOperation) {
			t.Fatal("incomplete private contact was accepted", err)
		}
	}
	if idCalls != 1 || uidCalls != 0 || httpCalls != 1 {
		t.Fatal("invalid contact escaped to a provider")
	}
}

func TestPhotoForContactErasesLateAndFailedPrivateBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	late := []byte("sensitive")
	failure := errors.New("failed")
	fixture := privatePhotoByIDFixture{
		privateContactsFixture: privateContactsFixture{scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "contacts"}},
		read:                   func(context.Context, string, string) ([]byte, error) { cancel(); return late, nil },
	}
	private, err := NewContacts("https://contacts.example", nil, nil).WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	person := Contact{ID: "person", AddressBookID: "book", ETag: "revision"}
	if got, err := private.PhotoForContact(ctx, person); got != nil || !errors.Is(err, context.Canceled) || !bytes.Equal(late, make([]byte, len(late))) {
		t.Fatal("late private bytes survived cancellation", err)
	}
	partial := []byte("partial")
	fixture.read = func(context.Context, string, string) ([]byte, error) { return partial, failure }
	private, err = NewContacts("https://contacts.example", nil, nil).WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := private.PhotoForContact(t.Context(), person); got != nil || !errors.Is(err, failure) || !bytes.Equal(partial, make([]byte, len(partial))) {
		t.Fatal("failed private bytes survived", err)
	}
}
