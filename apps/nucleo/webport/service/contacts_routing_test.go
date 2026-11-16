// SPDX-License-Identifier: Apache-2.0
package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestContactsSourceTransportRejectsMailAndCRUDAndKeepsRoomsProduct(t *testing.T) {
	identity := fixtureIdentity()
	identity.Scope.Product = "contacts"
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(webport.Method, []byte) ([]byte, error) { return nil, webport.ErrRejected }}
	owner, err := webport.OpenStorage(t.Context(), port, webport.NewSources())
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewServiceTransport(owner, "contacts")
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"mail", "drive"} {
		if _, err = NewServiceTransport(owner, service); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("cross product transport accepted", service, err)
		}
	}
	header, _ := storage.EncodeScopeHeader(identity.Scope)
	epoch := "11111111-1111-4111-8111-111111111111"
	id := "22222222-2222-4222-8222-222222222222"
	request := func(method, path string) *http.Request {
		r, e := http.NewRequestWithContext(t.Context(), method, path, http.NoBody)
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set(storage.ScopeHeader, header)
		return r
	}
	for _, tc := range []struct{ method, path, operation string }{
		{"GET", "/migration", "contacts.source.state"}, {"POST", "/migration", "contacts.source.begin"}, {"GET", "/migration/" + epoch + "/rows", "contacts.source.page"}, {"GET", "/migration/" + epoch + "/blobs/card/" + id, "contacts.source.open"},
	} {
		out, e := transport.classify(request(tc.method, "https://contacts.invalid/api/v1/private-storage"+tc.path))
		if e != nil || out.Operation != tc.operation {
			t.Fatal("original source route lost", tc, e)
		}
	}
	for _, path := range []string{"/api/v1/contacts", "/api/v1/private-storage/ingress/body", "/api/v1/private-storage/migration?after=evil", "/api/v1/private-storage/migration/" + epoch + "/rows?after=invalid", "/api/v1/private-storage/migration/" + epoch + "/blobs/mime_message/" + id, "/api/v1/private-storage/migration/" + epoch + "/blobs/card/not-uuid", "/api/v1/private-storage/migration/" + epoch + "/blobs/card/00000000-0000-0000-0000-000000000000"} {
		if _, e := transport.classify(request("GET", "https://contacts.invalid"+path)); !errors.Is(e, webport.ErrRejected) {
			t.Fatal("unrestricted Contacts route accepted", path, e)
		}
	}
	rooms, err := NewServiceTransport(owner, "rooms")
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range []string{"mail", "contacts"} {
		out, e := rooms.classify(request("GET", "https://rooms.invalid/api/v1/matrix/storage/"+product+"/authority"))
		if product == "contacts" && (e != nil || out.Operation != "rooms.authority") {
			t.Fatal("Contacts authority lost", e)
		}
		if product == "mail" && !errors.Is(e, webport.ErrRejected) {
			t.Fatal("Mail authority crossed Contacts scope", e)
		}
	}
	identity.Scope.Product = "mail"
	mailOwner, err := webport.OpenStorage(t.Context(), storagePort{sourcePort: sourcePort{id: strings.Repeat("b", 64), identity: identity}, handle: func(webport.Method, []byte) ([]byte, error) { return nil, webport.ErrRejected }}, webport.NewSources())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewServiceTransport(mailOwner, "contacts"); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("Mail owner constructed Contacts transport", err)
	}
}
