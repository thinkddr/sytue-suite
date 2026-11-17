// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Una tarjeta de Apple o de Google con sitios web, relaciones y aniversario se proyecta
// con lo que una persona sabe leer, y lo que no (un «ftp://», una referencia
// `urn:uuid:`, un aniversario sin año) queda fuera de la proyección.
func TestDecodeContactProyectaWebRelacionesYAniversario(t *testing.T) {
	card := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:x\r\nFN:Ada\r\n" +
		"URL;TYPE=work:https://acme.example\r\nitem1.URL:http://ada.example\r\nURL:ftp://viejo.example\r\n" +
		"RELATED;TYPE=spouse;VALUE=text:William\\, conde\r\nRELATED;TYPE=friend:Mary\r\nRELATED:urn:uuid:1234\r\n" +
		"ANNIVERSARY:20120609\r\nEND:VCARD\r\n"
	c, err := DecodeContact(card)
	if err != nil {
		t.Fatal(err)
	}
	if want := []URL{{URL: "https://acme.example", Type: "work"}, {URL: "http://ada.example", Type: "other"}}; !reflect.DeepEqual(c.URLs, want) {
		t.Errorf("urls = %+v", c.URLs)
	}
	if want := []Related{{Name: "William, conde", Type: "spouse"}, {Name: "Mary", Type: "friend"}}; !reflect.DeepEqual(c.Related, want) {
		t.Errorf("related = %+v", c.Related)
	}
	if c.Anniversary == nil || c.Anniversary.Format(time.DateOnly) != "2012-06-09" {
		t.Errorf("anniversary = %v", c.Anniversary)
	}

	parcial, err := DecodeContact("BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nANNIVERSARY:--0609\r\nEND:VCARD\r\n")
	if err != nil || parcial.Anniversary != nil {
		t.Errorf("un aniversario sin año no es una fecha: %v, %v", parcial.Anniversary, err)
	}
}

// Editar por la API sustituye lo proyectado —lo que no se reenvía se quita— y conserva
// lo que la proyección no enseña: quien edita en la app nativa no lo ve y no puede
// reenviarlo.
func TestEncodeContactRehaceLoProyectadoYConservaLoOculto(t *testing.T) {
	base := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:x\r\nFN:Ada\r\n" +
		"URL:https://vieja.example\r\nURL:ftp://viejo.example\r\n" +
		"RELATED;VALUE=text:Mary\r\nRELATED:urn:uuid:1234\r\nANNIVERSARY:--0609\r\nEND:VCARD\r\n"
	boda := time.Date(2012, 6, 9, 0, 0, 0, 0, time.UTC)
	c := Contact{
		UID: "x", FormattedName: "Ada",
		URLs:    []URL{{URL: "https://nueva.example", Type: "home"}},
		Related: []Related{{Name: "Ana, la de Lugo", Type: "sibling"}},
	}
	out, err := EncodeContact(c, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, debe := range []string{"https://nueva.example", "ftp://viejo.example", "urn:uuid:1234", "ANNIVERSARY:--0609", "Ana\\, la de Lugo"} {
		if !strings.Contains(out, debe) {
			t.Errorf("falta %q en\n%s", debe, out)
		}
	}
	for _, no := range []string{"vieja.example", "Mary"} {
		if strings.Contains(out, no) {
			t.Errorf("sobra %q en\n%s", no, out)
		}
	}
	// Ida y vuelta: lo escrito se lee igual.
	vuelta, err := DecodeContact(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(vuelta.URLs, c.URLs) || !reflect.DeepEqual(vuelta.Related, c.Related) {
		t.Errorf("ida y vuelta: %+v / %+v", vuelta.URLs, vuelta.Related)
	}

	// Una fecha completa sustituye al aniversario parcial.
	c.Anniversary = &boda
	out, err = EncodeContact(c, base)
	if err != nil || !strings.Contains(out, "ANNIVERSARY:2012-06-09") || strings.Contains(out, "--0609") {
		t.Errorf("aniversario: %v\n%s", err, out)
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"ejemplo.com":           "https://ejemplo.com",
		"ejemplo.com:8080/x":    "https://ejemplo.com:8080/x",
		" HTTP://Ejemplo.com/a": "HTTP://Ejemplo.com/a", //nolint:gocritic // el espacio inicial es lo que se prueba
		"https://[::1]:8080/":   "https://[::1]:8080/",
	} {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("%q → %q, %v; quiero %q", in, got, err, want)
		}
	}
	for _, malo := range []string{"", "javascript:alert(1)", "mailto:a@b.c", "https://", "a b.com", "ftp://x.example", "https://banco.example@otro.example"} {
		if _, err := NormalizeURL(malo); err == nil {
			t.Errorf("%q: quiero error", malo)
		}
	}
}
