// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
)

func TestEncodeDecodeContactRoundTrip(t *testing.T) {
	in := Contact{
		UID:           "11111111-2222-3333-4444-555555555555",
		FormattedName: "Ada Lovelace",
		FamilyName:    "Lovelace",
		GivenName:     "Ada",
		Organization:  "Analytical Engine",
		Title:         "Matemática",
		Nickname:      "Ada",
		Note:          "Primera programadora",
		Emails: []Email{
			{Email: "ada@example.eu", Type: EmailWork, IsPrimary: true},
			{Email: "ada.home@example.eu", Type: EmailHome},
		},
		Phones: []Phone{
			{Phone: "+34123456789", Type: PhoneCell, IsPrimary: true},
		},
	}

	data, err := EncodeContact(in, "")
	if err != nil {
		t.Fatalf("EncodeContact: %v", err)
	}
	if !strings.Contains(data, "VERSION:4.0") {
		t.Errorf("la vCard no declara VERSION:4.0:\n%s", data)
	}
	if !strings.Contains(data, "FN:Ada Lovelace") {
		t.Errorf("falta FN en la vCard:\n%s", data)
	}

	out, err := DecodeContact(data)
	if err != nil {
		t.Fatalf("DecodeContact: %v", err)
	}
	if out.FormattedName != in.FormattedName {
		t.Errorf("FN = %q, quiero %q", out.FormattedName, in.FormattedName)
	}
	if out.FamilyName != "Lovelace" || out.GivenName != "Ada" {
		t.Errorf("N mal decodificado: family=%q given=%q", out.FamilyName, out.GivenName)
	}
	if out.UID != in.UID {
		t.Errorf("UID = %q, quiero %q", out.UID, in.UID)
	}
	if len(out.Emails) != 2 {
		t.Fatalf("emails = %d, quiero 2", len(out.Emails))
	}
	if out.PrimaryEmail() != "ada@example.eu" {
		t.Errorf("correo principal = %q, quiero ada@example.eu", out.PrimaryEmail())
	}
	if len(out.Phones) != 1 || out.Phones[0].Phone != "+34123456789" {
		t.Errorf("teléfonos mal decodificados: %+v", out.Phones)
	}
}

func TestEncodeDecodeContactAddressesBirthdayRoundTrip(t *testing.T) {
	bday := time.Date(1906, 12, 9, 0, 0, 0, 0, time.UTC)
	in := Contact{
		UID:           "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		FormattedName: "Grace Hopper",
		GivenName:     "Grace",
		FamilyName:    "Hopper",
		Birthday:      &bday,
		Emails: []Email{
			{Email: "grace@work.eu", Type: EmailWork},
			{Email: "grace@home.eu", Type: EmailHome, IsPrimary: true},
			{Email: "grace@other.eu", Type: EmailOther},
		},
		Phones: []Phone{
			{Phone: "+34600000001", Type: PhoneCell, IsPrimary: true},
			{Phone: "+34910000002", Type: PhoneWork},
		},
		Addresses: []Address{
			{Street: "1 Trafalgar Sq", Locality: "London", PostalCode: "WC2N", Country: "UK", Type: AddressWork, IsPrimary: true},
			{Street: "Calle Mayor 1", Locality: "Madrid", Region: "Madrid", PostalCode: "28001", Country: "ES", Type: AddressHome},
		},
	}

	// Base con una ADR y un BDAY ajenos: al ser campos gestionados deben
	// regenerarse desde el modelo, no duplicarse (round-trip sin fugas).
	base := "BEGIN:VCARD\r\nVERSION:4.0\r\nADR;TYPE=home:;;Vieja;Nowhere;;00000;XX\r\nBDAY:1999-01-01\r\nEND:VCARD\r\n"
	data, err := EncodeContact(in, base)
	if err != nil {
		t.Fatalf("EncodeContact: %v", err)
	}
	if n := strings.Count(data, "ADR"); n != 2 {
		t.Errorf("se esperaban 2 ADR (sin duplicar la de base), hay %d:\n%s", n, data)
	}
	if strings.Count(data, "BDAY") != 1 {
		t.Errorf("se esperaba 1 BDAY (sin duplicar la de base):\n%s", data)
	}

	out, err := DecodeContact(data)
	if err != nil {
		t.Fatalf("DecodeContact: %v", err)
	}
	if len(out.Emails) != 3 || len(out.Phones) != 2 {
		t.Fatalf("emails=%d phones=%d, quiero 3 y 2", len(out.Emails), len(out.Phones))
	}
	if out.PrimaryEmail() != "grace@home.eu" {
		t.Errorf("correo principal = %q, quiero grace@home.eu", out.PrimaryEmail())
	}
	if out.Emails[2].Type != EmailOther {
		t.Errorf("tipo del tercer correo = %q, quiero other", out.Emails[2].Type)
	}
	if out.Phones[1].Type != PhoneWork {
		t.Errorf("tipo del segundo telefono = %q, quiero work", out.Phones[1].Type)
	}
	if out.Birthday == nil || !out.Birthday.Equal(bday) {
		t.Errorf("cumpleanos = %v, quiero %v", out.Birthday, bday)
	}
	if len(out.Addresses) != 2 {
		t.Fatalf("direcciones = %d, quiero 2:\n%s", len(out.Addresses), data)
	}
	a0 := out.Addresses[0]
	if a0.Street != "1 Trafalgar Sq" || a0.Locality != "London" || a0.PostalCode != "WC2N" ||
		a0.Country != "UK" || a0.Type != AddressWork || !a0.IsPrimary {
		t.Errorf("primera direccion mal round-trip: %+v", a0)
	}
	a1 := out.Addresses[1]
	if a1.Street != "Calle Mayor 1" || a1.Locality != "Madrid" || a1.Region != "Madrid" ||
		a1.PostalCode != "28001" || a1.Country != "ES" || a1.Type != AddressHome {
		t.Errorf("segunda direccion mal round-trip: %+v", a1)
	}
}

// TestEncodeContactNoBorraLoQueLaAPIEsconde: la API REST no proyecta una PHOTO
// embebida ni un BDAY sin año, así que quien edita por ella no puede reenviarlos; al
// guardar tienen que seguir ahí. Lo que SÍ se proyecta —una foto por URL, un
// cumpleaños completo— se borra al no mandarlo, que es como se quita.
func TestEncodeContactNoBorraLoQueLaAPIEsconde(t *testing.T) {
	c := Contact{UID: "u1", FormattedName: "Ada"}
	oculta := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nPHOTO:data:image/png;base64,iVBORw0KGgo=\r\nBDAY:--0215\r\nEND:VCARD\r\n"
	data, err := EncodeContact(c, oculta)
	if err != nil {
		t.Fatal(err)
	}
	card, err := parseSingle(data)
	if err != nil {
		t.Fatal(err)
	}
	for campo, quiero := range map[string]string{
		vcard.FieldPhoto: "data:image/png;base64,iVBORw0KGgo=", vcard.FieldBirthday: "--0215",
	} {
		if fs := card[campo]; len(fs) != 1 || fs[0].Value != quiero {
			t.Errorf("al guardar sin tocarlo, %s se pierde o se duplica: %v\n%s", campo, fs, data)
		}
	}
	vista := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nPHOTO:https://fotos.test/ada.png\r\nBDAY:1990-05-04\r\nEND:VCARD\r\n"
	if data, err = EncodeContact(c, vista); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, "PHOTO") || strings.Contains(data, "BDAY") {
		t.Errorf("lo proyectado y no reenviado se queda; así no se puede quitar:\n%s", data)
	}
}

func TestEncodeContactRequiresName(t *testing.T) {
	if _, err := EncodeContact(Contact{}, ""); err == nil {
		t.Fatal("se esperaba error por FN vacío")
	}
}

func TestEncodeDecodeGroupRoundTrip(t *testing.T) {
	in := Group{
		UID:     "group-uid-0001",
		Name:    "Equipo",
		Members: []string{"uid-a", "uid-b"},
	}
	data, err := EncodeGroup(in, "")
	if err != nil {
		t.Fatalf("EncodeGroup: %v", err)
	}
	if !strings.Contains(strings.ToUpper(data), "KIND:GROUP") {
		t.Errorf("la vCard de grupo no declara KIND:group:\n%s", data)
	}
	if KindOf(data) != KindGroup {
		t.Errorf("KindOf = %q, quiero group", KindOf(data))
	}
	out, err := DecodeGroup(data)
	if err != nil {
		t.Fatalf("DecodeGroup: %v", err)
	}
	if out.Name != "Equipo" {
		t.Errorf("nombre = %q, quiero Equipo", out.Name)
	}
	if len(out.Members) != 2 {
		t.Fatalf("miembros = %d, quiero 2", len(out.Members))
	}
}

func TestDecodeContactRejectsGroup(t *testing.T) {
	data, err := EncodeGroup(Group{UID: "g1", Name: "G", Members: []string{"x"}}, "")
	if err != nil {
		t.Fatalf("EncodeGroup: %v", err)
	}
	if _, err := DecodeContact(data); err == nil {
		t.Fatal("DecodeContact debería rechazar una vCard KIND=group")
	}
}

func TestValidName(t *testing.T) {
	cases := map[string]bool{
		"contacts":  true,
		"work-book": true,
		"":          false,
		".":         false,
		"..":        false,
		"a/b":       false,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, quiero %v", name, got, want)
		}
	}
}

func TestResourceName(t *testing.T) {
	if got := ResourceName("abc-123"); got != "abc-123.vcf" {
		t.Errorf("ResourceName = %q, quiero abc-123.vcf", got)
	}
	if got := ResourceName("already.vcf"); got != "already.vcf" {
		t.Errorf("ResourceName no debe duplicar la extensión: %q", got)
	}
	if got := ResourceName("a/b"); strings.ContainsAny(got, "/") {
		t.Errorf("ResourceName debe sanear separadores: %q", got)
	}
}
