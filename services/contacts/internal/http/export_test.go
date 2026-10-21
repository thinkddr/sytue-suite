// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

func TestExportEntregaTablaYVCard(t *testing.T) {
	libro := uuid.New()
	f := &fakeStore{
		books:    []domain.AddressBook{{ID: libro, Name: "personal"}},
		contacts: []domain.Contact{{UID: "a", FormattedName: "Ana Núñez", Emails: []domain.Email{{Email: "ana@x.es"}}}},
		cards: []domain.CardObject{
			{AddressBookID: libro, Data: "BEGIN:VCARD\r\nFN:Ana Núñez\r\nEND:VCARD\r\n"},
			{AddressBookID: libro, Data: "BEGIN:VCARD\nFN:Luis\nEND:VCARD"},
		},
	}
	leer := func(formato string) map[string]string {
		rec := httptest.NewRecorder()
		newHandlers(f).Export(rec, req("GET", "/api/v1/export?format="+formato, "", nil))
		out := map[string]string{}
		for parte, err := range portabilidad.Leer(rec.Result()) {
			if err != nil {
				t.Fatalf("%s: %v", formato, err)
			}
			b, _ := io.ReadAll(parte)
			out[parte.Ruta] = string(b)
		}
		return out
	}

	xml := leer("xml")
	if !strings.Contains(xml["contacts.xml"], "<formatted_name>Ana Núñez</formatted_name>") {
		t.Fatalf("tabla XML: %q", xml["contacts.xml"])
	}
	if strings.Count(xml["contacts.vcf"], "BEGIN:VCARD") != 2 || !strings.HasSuffix(xml["contacts.vcf"], "END:VCARD\r\n") {
		t.Fatalf("vCard: %q", xml["contacts.vcf"])
	}
	// En CSV va el de Google Contacts, que se importa tal cual en Google y Outlook.
	if csv := leer("csv")["contacts.csv"]; !strings.HasPrefix(csv, "Name,Given Name") {
		t.Fatalf("CSV: %q", csv)
	}
}
