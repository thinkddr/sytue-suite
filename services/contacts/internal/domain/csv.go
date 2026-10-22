// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// Importacion/exportacion CSV de contactos (clean-room, stdlib encoding/csv).
// Mapea el modelo Contact <-> los dos dialectos CSV mas comunes:
//
//   - Google Contacts: columnas de correo/telefono/direccion NUMERADAS y con
//     etiqueta ("E-mail 1 - Value", "E-mail 1 - Type", "Phone 1 - Value",
//     "Address 1 - Street", ...). Un mismo "Value" puede traer varios correos
//     separados por " ::: ".
//   - Outlook (Windows/Outlook.com): columnas FIJAS y con nombre ("E-mail
//     Address", "E-mail 2 Address", "Home Phone", "Business Phone", "Mobile
//     Phone", "Home Street", "Business Street", "Company", "Job Title", ...).
//
// El dialecto se detecta por las cabeceras (no por el contenido). Se reusa el
// MISMO modelo Contact que alimenta el round-trip vCard (N correos/telefonos con
// etiqueta, direccion estructurada, cumpleanos, organizacion, notas).

const (
	csvDialectGoogle  = "google"
	csvDialectOutlook = "outlook"
	// csvMaxRows acota el numero de contactos leidos de un CSV (defensa: streaming
	// con limite de filas ademas del limite de tamano de la capa HTTP).
	csvMaxRows = 50_000
	// csvMaxColumns acota el barrido de columnas numeradas de Google (E-mail N,
	// Phone N, Address N) para no depender de una cabecera con N ilimitado.
	csvMaxColumns = 50
)

// ParseContactsCSV lee un CSV de Google o de Outlook y devuelve los contactos
// mapeados al modelo, mas cuantas filas se OMITIERON (corruptas o sin nombre).
// Filas invalidas no abortan la importacion: se saltan y se contabilizan. Si las
// cabeceras no corresponden a ningun dialecto conocido devuelve ErrInvalidCSV.
func ParseContactsCSV(r io.Reader) (contacts []Contact, skipped int, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolera filas con distinto numero de campos
	cr.LazyQuotes = true    // tolera comillas mal balanceadas de exportadores torpes
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err == io.EOF {
		return nil, 0, nil // fichero vacio: 0 importados, 0 omitidos
	}
	if err != nil {
		return nil, 0, fmt.Errorf("%w: cabecera ilegible: %w", ErrInvalidCSV, err)
	}
	idx := headerIndex(header)
	dialect := detectDialect(idx)
	if dialect == "" {
		return nil, 0, fmt.Errorf("%w: cabeceras no reconocidas (ni Google ni Outlook)", ErrInvalidCSV)
	}

	for len(contacts) < csvMaxRows {
		rec, rerr := cr.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			// Una fila MALFORMADA (csv.ParseError) se salta sin abortar; un error
			// del lector subyacente (p. ej. cuerpo demasiado grande por el
			// MaxBytesReader de la capa HTTP) SI aborta.
			var pe *csv.ParseError
			if errors.As(rerr, &pe) {
				skipped++
				continue
			}
			return contacts, skipped, rerr
		}
		c, ok := rowToContact(dialect, idx, rec)
		if !ok {
			skipped++ // fila sin nombre ni datos utiles
			continue
		}
		contacts = append(contacts, c)
	}
	return contacts, skipped, nil
}

// WriteContactsCSV serializa contactos a CSV en formato Google Contacts (el mas
// portable: lo importan Google, Outlook y la mayoria de clientes). El numero de
// columnas de correo/telefono/direccion se ajusta al maximo real del lote para
// no perder datos en el round-trip. NEUTRALIZA la inyeccion de formulas CSV:
// toda celda que empiece por = + - @ (o tab/CR) se prefija con una comilla para
// que Excel/Sheets no la ejecute (OWASP CSV injection).
func WriteContactsCSV(w io.Writer, contacts []Contact) error {
	cw := csv.NewWriter(w)

	maxE, maxP, maxA := 1, 1, 0
	for _, c := range contacts {
		maxE = max(maxE, len(c.Emails))
		maxP = max(maxP, len(c.Phones))
		maxA = max(maxA, len(c.Addresses))
	}

	header := []string{
		"Name", "Given Name", "Additional Name", "Family Name", "Name Prefix",
		"Name Suffix", "Nickname", "Birthday", "Notes", "Organization Name", "Organization Title",
	}
	for i := 1; i <= maxE; i++ {
		header = append(header, fmt.Sprintf("E-mail %d - Type", i), fmt.Sprintf("E-mail %d - Value", i))
	}
	for i := 1; i <= maxP; i++ {
		header = append(header, fmt.Sprintf("Phone %d - Type", i), fmt.Sprintf("Phone %d - Value", i))
	}
	for i := 1; i <= maxA; i++ {
		p := fmt.Sprintf("Address %d - ", i)
		header = append(header, p+"Type", p+"Street", p+"City", p+"PO Box",
			p+"Region", p+"Postal Code", p+"Country", p+"Extended Address")
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, c := range contacts {
		bday := ""
		if c.Birthday != nil {
			bday = c.Birthday.Format(bdayLayout)
		}
		rec := []string{
			validation.CSVSafe(c.DisplayName()), validation.CSVSafe(c.GivenName), validation.CSVSafe(c.AdditionalName), validation.CSVSafe(c.FamilyName),
			validation.CSVSafe(c.HonorificPrefix), validation.CSVSafe(c.HonorificSuffix), validation.CSVSafe(c.Nickname), validation.CSVSafe(bday),
			validation.CSVSafe(c.Note), validation.CSVSafe(c.Organization), validation.CSVSafe(c.Title),
		}
		for i := 0; i < maxE; i++ {
			if i < len(c.Emails) {
				rec = append(rec, validation.CSVSafe(string(c.Emails[i].Type)), validation.CSVSafe(c.Emails[i].Email))
			} else {
				rec = append(rec, "", "")
			}
		}
		for i := 0; i < maxP; i++ {
			if i < len(c.Phones) {
				rec = append(rec, validation.CSVSafe(string(c.Phones[i].Type)), validation.CSVSafe(c.Phones[i].Phone))
			} else {
				rec = append(rec, "", "")
			}
		}
		for i := 0; i < maxA; i++ {
			if i < len(c.Addresses) {
				a := c.Addresses[i]
				rec = append(rec, validation.CSVSafe(string(a.Type)), validation.CSVSafe(a.Street), validation.CSVSafe(a.Locality), validation.CSVSafe(a.POBox),
					validation.CSVSafe(a.Region), validation.CSVSafe(a.PostalCode), validation.CSVSafe(a.Country), validation.CSVSafe(a.Extended))
			} else {
				rec = append(rec, "", "", "", "", "", "", "", "")
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ---------------------------------------------------------------------------
// Deteccion de dialecto y mapeo de filas
// ---------------------------------------------------------------------------

// headerIndex normaliza las cabeceras (minusculas, sin espacios sobrantes, sin
// BOM) y las mapea a su indice de columna.
func headerIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		key := normalizeHeader(h)
		if key == "" {
			continue
		}
		if _, dup := idx[key]; !dup { // ante duplicados gana la primera columna
			idx[key] = i
		}
	}
	return idx
}

// normalizeHeader baja a minusculas, quita el BOM UTF-8 inicial (U+FEFF, que
// anteponen Excel/Outlook a la primera cabecera) y colapsa espacios.
func normalizeHeader(h string) string {
	h = strings.TrimPrefix(h, "\uFEFF")
	return strings.ToLower(strings.Join(strings.Fields(h), " "))
}

// detectDialect decide el dialecto por la forma de las cabeceras de correo:
// Google numera ("e-mail 1 - value"), Outlook usa columnas fijas ("e-mail
// address"). Devuelve "" si no reconoce ninguno.
func detectDialect(idx map[string]int) string {
	has := func(k string) bool { _, ok := idx[k]; return ok }
	if has("e-mail 1 - value") || has("e-mail 1 - label") || has("e-mail 1 - type") || has("given name") {
		return csvDialectGoogle
	}
	if has("e-mail address") || has("e-mail 2 address") || has("mobile phone") ||
		has("business phone") || has("home phone") {
		return csvDialectOutlook
	}
	return ""
}

// rowToContact mapea una fila al modelo segun el dialecto. Devuelve false si la
// fila no tiene un nombre utilizable (se omitira).
func rowToContact(dialect string, idx map[string]int, rec []string) (Contact, bool) {
	get := func(key string) string {
		if i, ok := idx[key]; ok && i < len(rec) {
			return desan(strings.TrimSpace(rec[i]))
		}
		return ""
	}

	var c Contact
	switch dialect {
	case csvDialectGoogle:
		c = googleRow(get, idx)
	case csvDialectOutlook:
		c = outlookRow(get)
	}

	if c.FormattedName == "" {
		c.FormattedName = strings.TrimSpace(strings.Join(nonBlank(
			c.GivenName, c.AdditionalName, c.FamilyName,
		), " "))
	}
	if c.FormattedName == "" {
		c.FormattedName = c.Organization
	}
	if strings.TrimSpace(c.FormattedName) == "" {
		return Contact{}, false
	}
	ensurePrimary(len(c.Emails), func(i int) *bool { return &c.Emails[i].IsPrimary })
	ensurePrimary(len(c.Phones), func(i int) *bool { return &c.Phones[i].IsPrimary })
	ensurePrimary(len(c.Addresses), func(i int) *bool { return &c.Addresses[i].IsPrimary })
	return c, true
}

// googleRow mapea una fila del CSV de Google. Soporta el formato clasico ("Given
// Name"/"Family Name", "Organization 1 - Name") y el moderno ("First Name"/"Last
// Name", "Organization Name").
func googleRow(get func(string) string, idx map[string]int) Contact {
	c := Contact{
		FormattedName:   get("name"),
		GivenName:       firstNonBlank(get("given name"), get("first name")),
		FamilyName:      firstNonBlank(get("family name"), get("last name")),
		AdditionalName:  firstNonBlank(get("additional name"), get("middle name")),
		HonorificPrefix: get("name prefix"),
		HonorificSuffix: get("name suffix"),
		Nickname:        get("nickname"),
		Note:            get("notes"),
		Organization:    firstNonBlank(get("organization name"), get("organization 1 - name")),
		Title:           firstNonBlank(get("organization title"), get("organization 1 - title")),
	}
	c.Birthday = parseCSVDate(get("birthday"))

	// Correos/telefonos/direcciones numerados: "E-mail N - Value" + Type/Label.
	for n := 1; n <= csvMaxColumns; n++ {
		valKey := fmt.Sprintf("e-mail %d - value", n)
		if _, ok := idx[valKey]; !ok {
			break
		}
		val := get(valKey)
		typ := firstNonBlank(get(fmt.Sprintf("e-mail %d - type", n)), get(fmt.Sprintf("e-mail %d - label", n)))
		for _, e := range splitMulti(val) {
			c.Emails = append(c.Emails, Email{Email: e, Type: emailType(csvLabel(typ))})
		}
	}
	for n := 1; n <= csvMaxColumns; n++ {
		valKey := fmt.Sprintf("phone %d - value", n)
		if _, ok := idx[valKey]; !ok {
			break
		}
		val := get(valKey)
		typ := firstNonBlank(get(fmt.Sprintf("phone %d - type", n)), get(fmt.Sprintf("phone %d - label", n)))
		for _, p := range splitMulti(val) {
			c.Phones = append(c.Phones, Phone{Phone: p, Type: phoneType(csvLabel(typ))})
		}
	}
	for n := 1; n <= csvMaxColumns; n++ {
		p := fmt.Sprintf("address %d - ", n)
		if _, ok := idx[p+"street"]; !ok {
			if _, ok2 := idx[p+"formatted"]; !ok2 {
				break
			}
		}
		addr := Address{
			Street:     firstNonBlank(get(p+"street"), get(p+"formatted")),
			Locality:   get(p + "city"),
			Region:     get(p + "region"),
			PostalCode: get(p + "postal code"),
			Country:    get(p + "country"),
			POBox:      get(p + "po box"),
			Extended:   get(p + "extended address"),
			Type:       addressType(csvLabel(firstNonBlank(get(p+"type"), get(p+"label")))),
		}
		if !addr.IsZero() {
			c.Addresses = append(c.Addresses, addr)
		}
	}
	return c
}

// outlookRow mapea una fila del CSV de Outlook (columnas fijas con nombre).
func outlookRow(get func(string) string) Contact {
	c := Contact{
		GivenName:       get("first name"),
		FamilyName:      get("last name"),
		AdditionalName:  get("middle name"),
		HonorificPrefix: get("title"), // Outlook "Title" = tratamiento (Mr/Ms)
		HonorificSuffix: get("suffix"),
		Nickname:        get("nickname"),
		Note:            get("notes"),
		Organization:    get("company"),
		Title:           get("job title"),
	}
	c.Birthday = parseCSVDate(get("birthday"))

	addEmail := func(v string, t EmailType) {
		for _, e := range splitMulti(v) {
			c.Emails = append(c.Emails, Email{Email: e, Type: t})
		}
	}
	addEmail(get("e-mail address"), EmailHome)
	addEmail(get("e-mail 2 address"), EmailOther)
	addEmail(get("e-mail 3 address"), EmailOther)

	addPhone := func(v string, t PhoneType) {
		if v = strings.TrimSpace(v); v != "" {
			c.Phones = append(c.Phones, Phone{Phone: v, Type: t})
		}
	}
	addPhone(get("mobile phone"), PhoneCell)
	addPhone(get("home phone"), PhoneHome)
	addPhone(get("home phone 2"), PhoneHome)
	addPhone(get("business phone"), PhoneWork)
	addPhone(get("business phone 2"), PhoneWork)
	addPhone(get("home fax"), PhoneFax)
	addPhone(get("business fax"), PhoneFax)
	addPhone(get("other phone"), PhoneOther)
	addPhone(get("pager"), PhoneOther)

	addAddr := func(prefix string, t AddressType) {
		addr := Address{
			Street:     get(prefix + "street"),
			Locality:   get(prefix + "city"),
			Region:     get(prefix + "state"),
			PostalCode: get(prefix + "postal code"),
			Country:    get(prefix + "country"),
			POBox:      get(prefix + "po box"),
			Type:       t,
		}
		if !addr.IsZero() {
			c.Addresses = append(c.Addresses, addr)
		}
	}
	addAddr("home ", AddressHome)
	addAddr("business ", AddressWork)
	addAddr("other ", AddressOther)
	return c
}

// ---------------------------------------------------------------------------
// Helpers CSV
// ---------------------------------------------------------------------------

// desan revierte la comilla-guardia de validation.CSVSafe al importar, para que el round-trip
// sea sin perdida: solo la quita si protegia un caracter de formula.
func desan(s string) string {
	if len(s) >= 2 && s[0] == '\'' {
		switch s[1] {
		case '=', '+', '-', '@', '\t', '\r':
			return s[1:]
		}
	}
	return s
}

// splitMulti separa el valor multi-correo de Google (" ::: ") y descarta vacios.
func splitMulti(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ":::") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// csvLabel normaliza una etiqueta TYPE de Google/Outlook ("* Home", "Work",
// "Mobile") a una palabra que emailType/phoneType/addressType sepan mapear.
func csvLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "* ")
	switch {
	case strings.Contains(s, "mobile"), strings.Contains(s, "cell"):
		return "cell"
	case strings.Contains(s, "work"), strings.Contains(s, "business"):
		return "work"
	case strings.Contains(s, "home"):
		return "home"
	case strings.Contains(s, "fax"):
		return "fax"
	}
	return s
}

// parseCSVDate parsea el cumpleanos en los formatos mas comunes de Google/Outlook.
// Devuelve nil si esta vacio o es ilegible (no invalida la fila).
func parseCSVDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	for _, layout := range []string{
		"2006-01-02", "2006/01/02", "01/02/2006", "1/2/2006",
		"02/01/2006", "2/1/2006", "20060102", "January 2, 2006", "Jan 2, 2006",
	} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t
		}
	}
	return nil
}

// nonBlank devuelve los argumentos no vacios (tras trim) preservando el orden.
func nonBlank(vs ...string) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// firstNonBlank devuelve el primer argumento no vacio (tras trim), o "".
func firstNonBlank(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
