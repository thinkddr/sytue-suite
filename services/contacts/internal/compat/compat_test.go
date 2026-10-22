// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la traducción PURA a los dialectos de la Google People API y de
// Microsoft Graph (contacts): contacto con y sin organización, correos y teléfonos
// con su etiqueta, paginación en los dos formatos y parámetros no soportados.
package compat

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

var (
	contactID = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	etag      = uuid.MustParse("55555555-5555-4555-8555-555555555555")
	created   = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
)

// fullContact es un contacto con nombre completo, organización, dos correos y
// tres teléfonos de etiquetas distintas.
func fullContact() domain.Contact {
	bday := time.Date(1990, 3, 15, 0, 0, 0, 0, time.UTC)
	return domain.Contact{
		ID: contactID, UID: "uid-1", FormattedName: "Ada Lovelace",
		FamilyName: "Lovelace", GivenName: "Ada", AdditionalName: "Byron",
		HonorificPrefix: "Sra.", Nickname: "Ada", Note: "Analista",
		Organization: "Sytue", Title: "Ingeniera", Birthday: &bday,
		Emails: []domain.Email{
			{Email: "ada@ex.com", Type: domain.EmailWork, IsPrimary: true},
			{Email: "ada@casa.com", Type: domain.EmailHome},
		},
		Phones: []domain.Phone{
			{Phone: "+34600000000", Type: domain.PhoneCell, IsPrimary: true},
			{Phone: "+34910000000", Type: domain.PhoneWork},
			{Phone: "+34930000000", Type: domain.PhoneHome},
		},
		ETag: etag, CreatedAt: created, UpdatedAt: created,
	}
}

// bareContact es un contacto mínimo: solo nombre, sin organización ni canales.
func bareContact() domain.Contact {
	return domain.Contact{
		ID: contactID, UID: "uid-2", FormattedName: "Bob", GivenName: "Bob",
		ETag: etag, CreatedAt: created, UpdatedAt: created,
	}
}

func TestPersonFromContact(t *testing.T) {
	tests := []struct {
		name                                  string
		contact                               domain.Contact
		wantEmails, wantPhones, wantOrgBlocks int
		wantDisplayName                       string
	}{
		{
			name: "completo", contact: fullContact(),
			wantEmails: 2, wantPhones: 3, wantOrgBlocks: 1, wantDisplayName: "Ada Lovelace",
		},
		{
			name: "sin organización ni canales", contact: bareContact(),
			wantDisplayName: "Bob",
		},
		{
			name: "solo puesto, sin empresa", contact: func() domain.Contact {
				c := bareContact()
				c.Title = "Ingeniera"
				return c
			}(),
			wantOrgBlocks: 1, wantDisplayName: "Bob",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := PersonFromContact(tc.contact)
			if p.ResourceName != "people/"+contactID.String() {
				t.Fatalf("resourceName = %q", p.ResourceName)
			}
			if len(p.Names) != 1 || p.Names[0].DisplayName != tc.wantDisplayName {
				t.Fatalf("names = %+v", p.Names)
			}
			if len(p.EmailAddresses) != tc.wantEmails {
				t.Errorf("correos = %d, quiero %d", len(p.EmailAddresses), tc.wantEmails)
			}
			if len(p.PhoneNumbers) != tc.wantPhones {
				t.Errorf("teléfonos = %d, quiero %d", len(p.PhoneNumbers), tc.wantPhones)
			}
			if len(p.Organizations) != tc.wantOrgBlocks {
				t.Errorf("organizations = %d, quiero %d", len(p.Organizations), tc.wantOrgBlocks)
			}
		})
	}
}

func TestPersonFieldDetails(t *testing.T) {
	p := PersonFromContact(fullContact())
	if p.Names[0].FamilyName != "Lovelace" || p.Names[0].GivenName != "Ada" ||
		p.Names[0].MiddleName != "Byron" || p.Names[0].HonorificPrefix != "Sra." {
		t.Fatalf("nombre estructurado: %+v", p.Names[0])
	}
	if p.EmailAddresses[0].Value != "ada@ex.com" || p.EmailAddresses[0].Type != "work" ||
		!p.EmailAddresses[0].Metadata.Primary {
		t.Fatalf("correo principal: %+v", p.EmailAddresses[0])
	}
	if p.EmailAddresses[1].Metadata.Primary {
		t.Fatal("solo un correo puede ser principal")
	}
	if p.PhoneNumbers[0].Type != "mobile" {
		t.Fatalf("un TEL de tipo cell es mobile en People: %+v", p.PhoneNumbers[0])
	}
	if p.Organizations[0].Name != "Sytue" || p.Organizations[0].Title != "Ingeniera" {
		t.Fatalf("organización: %+v", p.Organizations[0])
	}
	if p.ETag != etag.String() {
		t.Fatalf("etag = %q", p.ETag)
	}
}

func TestGraphFromContact(t *testing.T) {
	g := GraphFromContact(fullContact())
	if g.ID != contactID.String() || g.DisplayName != "Ada Lovelace" ||
		g.GivenName != "Ada" || g.Surname != "Lovelace" {
		t.Fatalf("contacto de Graph: %+v", g)
	}
	if g.CompanyName != "Sytue" || g.JobTitle != "Ingeniera" {
		t.Fatalf("empresa/puesto: %+v", g)
	}
	if len(g.EmailAddresses) != 2 || g.EmailAddresses[0].Address != "ada@ex.com" ||
		g.EmailAddresses[0].Name != "Ada Lovelace" {
		t.Fatalf("correos: %+v", g.EmailAddresses)
	}
	if g.MobilePhone != "+34600000000" {
		t.Errorf("mobilePhone = %q", g.MobilePhone)
	}
	if len(g.BusinessPhones) != 1 || g.BusinessPhones[0] != "+34910000000" {
		t.Errorf("businessPhones = %v", g.BusinessPhones)
	}
	if len(g.HomePhones) != 1 || g.HomePhones[0] != "+34930000000" {
		t.Errorf("homePhones = %v", g.HomePhones)
	}
	if g.Birthday != "1990-03-15T00:00:00Z" {
		t.Errorf("birthday = %q", g.Birthday)
	}

	bare := GraphFromContact(bareContact())
	if bare.CompanyName != "" || bare.JobTitle != "" || bare.MobilePhone != "" ||
		len(bare.EmailAddresses) != 0 || len(bare.BusinessPhones) != 0 || bare.Birthday != "" {
		t.Fatalf("un contacto mínimo no inventa campos: %+v", bare)
	}
}

// TestGraphPhonesNeverLost: Graph solo tiene tres huecos de teléfono; ninguno de
// los números del contacto puede quedarse por el camino.
func TestGraphPhonesNeverLost(t *testing.T) {
	c := bareContact()
	c.Phones = []domain.Phone{
		{Phone: "1", Type: domain.PhoneCell},
		{Phone: "2", Type: domain.PhoneCell},
		{Phone: "3", Type: domain.PhoneFax},
		{Phone: "4", Type: domain.PhoneOther},
		{Phone: "5", Type: domain.PhoneHome},
	}
	g := GraphFromContact(c)
	got := len(g.BusinessPhones) + len(g.HomePhones)
	if g.MobilePhone != "" {
		got++
	}
	if got != len(c.Phones) {
		t.Fatalf("se han perdido teléfonos: %+v", g)
	}
}

func TestSplitResourceName(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "people/" + contactID.String(), want: contactID.String()},
		{in: contactID.String(), want: contactID.String()},
		{in: " people/me ", want: "me"},
	}
	for _, tc := range tests {
		if got := SplitResourceName(tc.in); got != tc.want {
			t.Errorf("SplitResourceName(%q) = %q, quiero %q", tc.in, got, tc.want)
		}
	}
}

func TestCheckParams(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{name: "soportados", query: "pageSize=10&pageToken=abc"},
		{name: "proyección y sistema", query: "personFields=names,emailAddresses&prettyPrint=true&alt=json"},
		{name: "orden no emulado", query: "sortOrder=LAST_MODIFIED_DESCENDING", wantErr: true},
		{name: "sincronización incremental", query: "requestSyncToken=true", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("query de prueba inválida: %v", err)
			}
			err = CheckParams(q, "pageSize", "pageToken", "sources")
			if tc.wantErr != (err != nil) {
				t.Fatalf("CheckParams = %v (quiero error = %v)", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrUnsupportedParam) {
				t.Fatalf("quiero ErrUnsupportedParam, tengo %v", err)
			}
		})
	}
}

func TestPageTokenRoundTrip(t *testing.T) {
	tok := EncodePageToken(100)
	off, err := DecodePageToken(tok)
	if err != nil || off != 100 {
		t.Fatalf("token %q -> (%d, %v)", tok, off, err)
	}
	if off, err := DecodePageToken(""); err != nil || off != 0 {
		t.Fatalf("token vacío = primera página, tengo (%d, %v)", off, err)
	}
	if _, err := DecodePageToken("de-otra-api"); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("token ajeno debe ser ErrInvalidPageToken, tengo %v", err)
	}
}

func TestPageSizeAndNextLink(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{{raw: "", want: 100}, {raw: "0", want: 100}, {raw: "x", want: 100}, {raw: "7", want: 7}, {raw: "9000", want: 250}}
	for _, tc := range tests {
		if got := PageSize(tc.raw, 100, 250); got != tc.want {
			t.Errorf("PageSize(%q) = %d, quiero %d", tc.raw, got, tc.want)
		}
	}
	link := NextLink("https://contacts.sytue.eu/", "/compat/graph/v1.0/me/contacts", 10, 10)
	want := "https://contacts.sytue.eu/compat/graph/v1.0/me/contacts?%24skip=10&%24top=10"
	if link != want {
		t.Fatalf("nextLink = %q, quiero %q", link, want)
	}
	if NextLink("https://x", "/y", 10, 0) != "" {
		t.Fatal("sin más páginas no debe haber nextLink")
	}
}

func TestCollectionsNeverNil(t *testing.T) {
	if c := NewConnections(nil, "").Connections; c == nil {
		t.Error("connections no puede ser nulo")
	}
	if v := NewGraphCollection[GraphContact](nil, "").Value; v == nil {
		t.Error("value de una colección OData no puede ser nulo")
	}
}
