// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"errors"
	"reflect"
	"testing"

	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// La API acepta sitios web, relaciones y aniversario: completa el «https://» que la
// gente no escribe, descarta las filas vacías del editor y rechaza (400) lo que no es
// un sitio web abrible con un clic.
func TestToContactConWebRelacionesYAniversario(t *testing.T) {
	c, err := contactRequest{
		FormattedName: "Ana",
		URLs: []domain.URL{
			{URL: " ejemplo.com/ana ", Type: "WORK"}, {URL: ""}, {URL: "http://blog.example", Type: "raro"},
		},
		Related:     []domain.Related{{Name: " Luis ", Type: "Spouse"}, {Name: "  "}, {Name: "Eva", Type: "jefa"}},
		Anniversary: "2012-06-09",
	}.toContact()
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.URL{{URL: "https://ejemplo.com/ana", Type: "work"}, {URL: "http://blog.example", Type: "other"}}; !reflect.DeepEqual(c.URLs, want) {
		t.Errorf("urls = %+v", c.URLs)
	}
	if want := []domain.Related{{Name: "Luis", Type: "spouse"}, {Name: "Eva"}}; !reflect.DeepEqual(c.Related, want) {
		t.Errorf("related = %+v", c.Related)
	}
	if c.Anniversary == nil || c.Anniversary.Format("2006-01-02") != "2012-06-09" {
		t.Errorf("anniversary = %v", c.Anniversary)
	}
	for _, malo := range []contactRequest{
		{FormattedName: "Ana", URLs: []domain.URL{{URL: "javascript:alert(1)"}}},
		{FormattedName: "Ana", URLs: []domain.URL{{URL: "ftp://viejo.example"}}},
		{FormattedName: "Ana", URLs: []domain.URL{{URL: "https:// con espacios"}}},
		{FormattedName: "Ana", Related: []domain.Related{{Name: "dos\nlíneas"}}},
		{FormattedName: "Ana", Anniversary: "junio"},
	} {
		if _, err := malo.toContact(); !errors.Is(err, domain.ErrInvalidContact) {
			t.Errorf("%+v: %v; quiero ErrInvalidContact (400)", malo, err)
		}
	}
}
