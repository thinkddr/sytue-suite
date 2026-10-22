// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseGoogleCSV(t *testing.T) {
	// CSV de Google: correos/telefonos/direcciones numerados; un "Value" con dos
	// correos separados por " ::: ".
	csv := "Name,Given Name,Family Name,Organization Name,Organization Title,Birthday," +
		"E-mail 1 - Type,E-mail 1 - Value,Phone 1 - Type,Phone 1 - Value," +
		"Address 1 - Type,Address 1 - Street,Address 1 - City,Address 1 - Region,Address 1 - Postal Code,Address 1 - Country\r\n" +
		"Ada Lovelace,Ada,Lovelace,Sytue,Ingeniera,1815-12-10," +
		"Home,ada@home.example ::: ada@work.example,Mobile,+34600000000," +
		"Home,Calle Uno 1,Madrid,Madrid,28001,España\r\n"

	got, skipped, err := ParseContactsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseContactsCSV: %v", err)
	}
	if skipped != 0 || len(got) != 1 {
		t.Fatalf("got %d contactos, %d omitidos; want 1,0", len(got), skipped)
	}
	c := got[0]
	if c.FormattedName != "Ada Lovelace" || c.GivenName != "Ada" || c.FamilyName != "Lovelace" {
		t.Errorf("nombre mal mapeado: %+v", c)
	}
	if c.Organization != "Sytue" || c.Title != "Ingeniera" {
		t.Errorf("org/title mal mapeado: %q / %q", c.Organization, c.Title)
	}
	if c.Birthday == nil || c.Birthday.Format(bdayLayout) != "1815-12-10" {
		t.Errorf("cumpleaños mal mapeado: %v", c.Birthday)
	}
	if len(c.Emails) != 2 || c.Emails[0].Email != "ada@home.example" || c.Emails[1].Email != "ada@work.example" {
		t.Errorf("correos multi-valor mal mapeados: %+v", c.Emails)
	}
	if len(c.Phones) != 1 || c.Phones[0].Phone != "+34600000000" || c.Phones[0].Type != PhoneCell {
		t.Errorf("teléfono mal mapeado: %+v", c.Phones)
	}
	if len(c.Addresses) != 1 || c.Addresses[0].Locality != "Madrid" || c.Addresses[0].PostalCode != "28001" {
		t.Errorf("dirección mal mapeada: %+v", c.Addresses)
	}
}

func TestParseOutlookCSV(t *testing.T) {
	// CSV de Outlook: columnas fijas con nombre (no numeradas).
	csv := "First Name,Last Name,Company,Job Title,E-mail Address,E-mail 2 Address,Mobile Phone,Business Phone," +
		"Home Street,Home City,Home State,Home Postal Code,Home Country\r\n" +
		"Grace,Hopper,Navy,Contralmirante,grace@home.example,grace@navy.example,+1202555,+1202777," +
		"1 Main St,Arlington,VA,22201,USA\r\n"

	got, skipped, err := ParseContactsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseContactsCSV: %v", err)
	}
	if skipped != 0 || len(got) != 1 {
		t.Fatalf("got %d contactos, %d omitidos; want 1,0", len(got), skipped)
	}
	c := got[0]
	if c.FormattedName != "Grace Hopper" { // compuesto de First+Last
		t.Errorf("FN compuesto mal: %q", c.FormattedName)
	}
	if c.Organization != "Navy" || c.Title != "Contralmirante" {
		t.Errorf("org/title mal: %q / %q", c.Organization, c.Title)
	}
	if len(c.Emails) != 2 || c.Emails[0].Email != "grace@home.example" {
		t.Errorf("correos Outlook mal: %+v", c.Emails)
	}
	// Mobile -> cell, Business -> work.
	if len(c.Phones) != 2 || c.Phones[0].Type != PhoneCell || c.Phones[1].Type != PhoneWork {
		t.Errorf("teléfonos Outlook por tipo mal: %+v", c.Phones)
	}
	if len(c.Addresses) != 1 || c.Addresses[0].Region != "VA" || c.Addresses[0].Type != AddressHome {
		t.Errorf("dirección Outlook mal: %+v", c.Addresses)
	}
}

func TestParseCSVSkipsInvalidRows(t *testing.T) {
	// Cabecera Google; fila válida + fila SIN nombre ni datos utilizables.
	csv := "Name,Given Name,Family Name,E-mail 1 - Value\r\n" +
		"Ada Lovelace,Ada,Lovelace,ada@example.com\r\n" +
		",,,huerfano@example.com\r\n" // sin nombre: se omite

	got, skipped, err := ParseContactsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseContactsCSV: %v", err)
	}
	if len(got) != 1 || skipped != 1 {
		t.Fatalf("got %d contactos, %d omitidos; want 1,1", len(got), skipped)
	}
}

func TestParseCSVUnrecognized(t *testing.T) {
	_, _, err := ParseContactsCSV(strings.NewReader("Col A,Col B\r\n1,2\r\n"))
	if !errors.Is(err, ErrInvalidCSV) {
		t.Fatalf("err = %v, want ErrInvalidCSV", err)
	}
}

func TestParseCSVEmpty(t *testing.T) {
	got, skipped, err := ParseContactsCSV(strings.NewReader(""))
	if err != nil || len(got) != 0 || skipped != 0 {
		t.Fatalf("CSV vacío: got %d/%d err=%v; want 0/0/nil", len(got), skipped, err)
	}
}

func TestWriteCSVNeutralizesInjection(t *testing.T) {
	contacts := []Contact{{
		FormattedName: "=SUM(A1)", // fórmula: debe neutralizarse al exportar
		Note:          "+budget",
		Emails:        []Email{{Email: "ada@example.com", Type: EmailHome, IsPrimary: true}},
		Phones:        []Phone{{Phone: "+34600000000", Type: PhoneCell}},
	}}
	var sb strings.Builder
	if err := WriteContactsCSV(&sb, contacts); err != nil {
		t.Fatalf("WriteContactsCSV: %v", err)
	}
	out := sb.String()
	// Celdas peligrosas prefijadas con comilla (no las ejecuta Excel/Sheets).
	if !strings.Contains(out, "'=SUM(A1)") {
		t.Errorf("FN con fórmula sin neutralizar:\n%s", out)
	}
	if !strings.Contains(out, "'+budget") {
		t.Errorf("nota con '+' sin neutralizar:\n%s", out)
	}
	if !strings.Contains(out, "'+34600000000") {
		t.Errorf("teléfono con '+' sin neutralizar:\n%s", out)
	}
	// El correo normal NO se prefija.
	if strings.Contains(out, "'ada@example.com") {
		t.Errorf("correo normal prefijado por error:\n%s", out)
	}
}

func TestCSVRoundTrip(t *testing.T) {
	orig := []Contact{{
		FormattedName: "Ada Lovelace",
		GivenName:     "Ada",
		FamilyName:    "Lovelace",
		Organization:  "Sytue",
		Title:         "Ingeniera",
		Emails: []Email{
			{Email: "ada@home.example", Type: EmailHome, IsPrimary: true},
			{Email: "ada@work.example", Type: EmailWork},
		},
		Phones: []Phone{
			{Phone: "+34600000000", Type: PhoneCell, IsPrimary: true}, // '+' ejercita el desan
			{Phone: "915550000", Type: PhoneWork},
		},
		Addresses: []Address{
			{Street: "Calle Uno 1", Locality: "Madrid", Region: "Madrid", PostalCode: "28001", Country: "España", Type: AddressHome, IsPrimary: true},
		},
	}}

	var sb strings.Builder
	if err := WriteContactsCSV(&sb, orig); err != nil {
		t.Fatalf("WriteContactsCSV: %v", err)
	}
	got, skipped, err := ParseContactsCSV(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if skipped != 0 || len(got) != 1 {
		t.Fatalf("round-trip got %d/%d; want 1/0", len(got), skipped)
	}
	c := got[0]
	if c.FormattedName != "Ada Lovelace" || c.GivenName != "Ada" || c.FamilyName != "Lovelace" {
		t.Errorf("nombre no round-trip: %+v", c)
	}
	if c.Organization != "Sytue" || c.Title != "Ingeniera" {
		t.Errorf("org/title no round-trip: %q/%q", c.Organization, c.Title)
	}
	if len(c.Emails) != 2 || c.Emails[0].Email != "ada@home.example" || c.Emails[0].Type != EmailHome ||
		c.Emails[1].Email != "ada@work.example" || c.Emails[1].Type != EmailWork {
		t.Errorf("correos no round-trip: %+v", c.Emails)
	}
	if len(c.Phones) != 2 || c.Phones[0].Phone != "+34600000000" || c.Phones[0].Type != PhoneCell ||
		c.Phones[1].Type != PhoneWork {
		t.Errorf("teléfonos no round-trip (¿desan del '+'?): %+v", c.Phones)
	}
	if len(c.Addresses) != 1 {
		t.Fatalf("dirección no round-trip: %+v", c.Addresses)
	}
	a := c.Addresses[0]
	if a.Street != "Calle Uno 1" || a.Locality != "Madrid" || a.PostalCode != "28001" || a.Country != "España" {
		t.Errorf("componentes de dirección no round-trip: %+v", a)
	}
}

func TestCSVRoundTripInjectionLossless(t *testing.T) {
	// Un nombre que parece fórmula se exporta con la comilla-guardia y se importa
	// SIN ella (sin pérdida): el guard es solo para el motor de hojas de cálculo.
	orig := []Contact{{FormattedName: "=cmd|calc", GivenName: "=cmd|calc"}}
	var sb strings.Builder
	if err := WriteContactsCSV(&sb, orig); err != nil {
		t.Fatalf("WriteContactsCSV: %v", err)
	}
	got, _, err := ParseContactsCSV(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(got) != 1 || got[0].FormattedName != "=cmd|calc" {
		t.Fatalf("round-trip con inyección no es sin pérdida: %+v", got)
	}
}
