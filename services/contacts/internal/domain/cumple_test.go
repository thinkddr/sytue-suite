// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

// Las formas de BDAY que circulan, con los mismos casos que `cumpleanosDe` de la web
// (apps/frontend/apps/contacts/src/client/cumpleanos.test.ts): la ficha de las dos
// plataformas tiene que decir lo mismo de la misma tarjeta.
func TestDecodeContactProyectaElCumpleanosSinAno(t *testing.T) {
	for _, caso := range []struct {
		bday, completo, sinAno string
	}{
		{"BDAY:19900215", "1990-02-15", ""},
		{"BDAY:1990-02-15", "1990-02-15", ""},
		{"BDAY:--0215", "", "--02-15"},
		{"BDAY;VALUE=date:--02-15", "", "--02-15"},
		{"item1.BDAY:--1231", "", "--12-31"},
		{"BDAY:--0229", "", "--02-29"},
		{"BDAY;X-APPLE-OMIT-YEAR=1604:1604-02-15", "", "--02-15"},
		{"BDAY;X-APPLE-OMIT-YEAR=1604:1990-02-15", "1990-02-15", ""},
		{"BDAY:--0231", "", ""},
		{"BDAY:--1301", "", ""},
		{"BDAY:mañana", "", ""},
	} {
		c, err := DecodeContact("BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ana\r\n" + caso.bday + "\r\nEND:VCARD\r\n")
		if err != nil {
			t.Fatalf("%s: %v", caso.bday, err)
		}
		completo := ""
		if c.Birthday != nil {
			completo = c.Birthday.Format(bdayLayout)
		}
		if completo != caso.completo || c.BirthdayNoYear != caso.sinAno {
			t.Errorf("%s → %q / %q; quiero %q / %q", caso.bday, completo, c.BirthdayNoYear, caso.completo, caso.sinAno)
		}
	}
}

func TestNormalizeBirthdayNoYear(t *testing.T) {
	for entra, sale := range map[string]string{"--02-15": "--02-15", "--0215": "--02-15", "\t--1231\n": "--12-31", "--02-29": "--02-29"} {
		if s, err := NormalizeBirthdayNoYear(entra); err != nil || s != sale {
			t.Errorf("%q → %q, %v; quiero %q", entra, s, err, sale)
		}
	}
	for _, malo := range []string{"", "02-15", "--2-15", "--04-31", "--00-10", "--12-00", "--1-1-1", "--ab-cd", "1990-02-15"} {
		if s, err := NormalizeBirthdayNoYear(malo); !errors.Is(err, ErrInvalidContact) {
			t.Errorf("%q → %q, %v; quiero ErrInvalidContact", malo, s, err)
		}
	}
}

// Quien edita por la API manda el cumpleaños sin año que leyó: si no ha cambiado, el
// BDAY se queda como estaba (el de Apple, con su año de relleno); si ha cambiado, se
// escribe en la forma de vCard 4, «--MMDD»; y vuelve a leerse igual.
func TestEncodeContactEscribeElCumpleanosSinAno(t *testing.T) {
	apple := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nBDAY;X-APPLE-OMIT-YEAR=1604:1604-12-10\r\nEND:VCARD\r\n"
	for _, caso := range []struct {
		base, sinAno, valor, omitir string
	}{
		{"", "--02-15", "--0215", ""},
		{apple, "--12-10", "1604-12-10", "1604"},
		{apple, "--12-11", "--1211", ""},
		{"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nBDAY:1990-05-04\r\nEND:VCARD\r\n", "--05-04", "--0504", ""},
	} {
		data, err := EncodeContact(Contact{UID: "u1", FormattedName: "Ada", BirthdayNoYear: caso.sinAno}, caso.base)
		if err != nil {
			t.Fatal(err)
		}
		card, err := parseSingle(data)
		if err != nil {
			t.Fatal(err)
		}
		fs := card[vcard.FieldBirthday]
		if len(fs) != 1 || fs[0].Value != caso.valor || fs[0].Params.Get(paramOmitirAno) != caso.omitir {
			t.Errorf("%s sobre %q: BDAY = %+v; quiero %q", caso.sinAno, strings.TrimSpace(caso.base), fs, caso.valor)
			continue
		}
		vuelta, err := DecodeContact(data)
		if err != nil {
			t.Fatal(err)
		}
		if vuelta.Birthday != nil || vuelta.BirthdayNoYear != caso.sinAno {
			t.Errorf("%s no vuelve igual: %v / %q", caso.sinAno, vuelta.Birthday, vuelta.BirthdayNoYear)
		}
	}
}

// Google Contacts exporta el cumpleaños sin año como «--MM-DD» y lo vuelve a leer así.
// El CSV propio lo lleva con la guarda contra fórmulas y vuelve a leerse igual.
func TestCSVConCumpleanosSinAno(t *testing.T) {
	if f, s := parseCSVBirthday("--02-15"); f != nil || s != "--02-15" {
		t.Errorf("importar «--02-15» → %v / %q", f, s)
	}
	if f, s := parseCSVBirthday("1990-02-15"); f == nil || s != "" {
		t.Errorf("importar «1990-02-15» → %v / %q", f, s)
	}
	var b strings.Builder
	if err := WriteContactsCSV(&b, []Contact{{FormattedName: "Ana", BirthdayNoYear: "--02-15"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), ",'--02-15,") {
		t.Fatalf("el CSV no lleva el cumpleaños sin año, con su guarda:\n%s", b.String())
	}
	cs, _, err := ParseContactsCSV(strings.NewReader(b.String()))
	if err != nil || len(cs) != 1 || cs[0].BirthdayNoYear != "--02-15" || cs[0].Birthday != nil {
		t.Errorf("el CSV propio no vuelve igual: %+v, %v", cs, err)
	}
}
