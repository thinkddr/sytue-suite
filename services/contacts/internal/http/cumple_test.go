// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// La API acepta el cumpleaños sin año en `birthday_no_year`, lo normaliza a «--MM-DD»
// y lo descarta si además llega una fecha completa: un contacto no tiene dos.
func TestToContactConCumpleanosSinAno(t *testing.T) {
	for _, caso := range []struct {
		req            contactRequest
		completo, sinA string
	}{
		{contactRequest{FormattedName: "Ana", BirthdayNoYear: "--0215"}, "", "--02-15"},
		{contactRequest{FormattedName: "Ana", BirthdayNoYear: "--02-29"}, "", "--02-29"},
		{contactRequest{FormattedName: "Ana", Birthday: "1990-02-15", BirthdayNoYear: "--03-01"}, "1990-02-15", ""},
		{contactRequest{FormattedName: "Ana"}, "", ""},
	} {
		c, err := caso.req.toContact()
		if err != nil {
			t.Fatalf("%+v: %v", caso.req, err)
		}
		completo := ""
		if c.Birthday != nil {
			completo = c.Birthday.Format("2006-01-02")
		}
		if completo != caso.completo || c.BirthdayNoYear != caso.sinA {
			t.Errorf("%+v → %q / %q", caso.req, completo, c.BirthdayNoYear)
		}
	}
	for _, malo := range []string{"--04-31", "02-15", "mañana"} {
		if _, err := (contactRequest{FormattedName: "Ana", BirthdayNoYear: malo}).toContact(); !errors.Is(err, domain.ErrInvalidContact) {
			t.Errorf("%q: %v; quiero ErrInvalidContact (400)", malo, err)
		}
	}
}
